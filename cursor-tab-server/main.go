package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// lyh用cursor修改 2026-08-01：集中声明宝塔部署环境变量和健康检查路径，避免运行参数散落在启动逻辑中。
const (
	defaultConfigPath = "./config.yaml"
	defaultListenAddr = "127.0.0.1:8041"
	healthPath        = "/healthz"
	envConfigPath     = "CURSOR_TAB_CONFIG"
	envListenAddr     = "CURSOR_TAB_LISTEN_ADDR"
	envToken          = "CURSOR_TAB_TOKEN"
)

var hopByHopHeaders = map[string]struct{}{
	"connection":          {},
	"proxy-connection":    {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"te":                  {},
	"trailer":             {},
	"transfer-encoding":   {},
	"upgrade":             {},
}

var defaultUpstreamTargets = map[string]string{
	"/aiserver.v1.AiService/StreamCpp":                      "https://api4.cursor.sh:443/aiserver.v1.AiService/StreamCpp",
	"/aiserver.v1.AiService/StreamNextCursorPrediction":     "https://api4.cursor.sh:443/aiserver.v1.AiService/StreamNextCursorPrediction",
	"/aiserver.v1.AiService/GetCppEditClassification":       "https://api4.cursor.sh:443/aiserver.v1.AiService/GetCppEditClassification",
	"/aiserver.v1.AiService/RefreshTabContext":              "https://api2.cursor.sh:443/aiserver.v1.AiService/RefreshTabContext",
	"/aiserver.v1.AiService/CppConfig":                      "https://api4.cursor.sh:443/aiserver.v1.AiService/CppConfig",
	"/aiserver.v1.AiService/CppEditHistoryStatus":           "https://api2.cursor.sh:443/aiserver.v1.AiService/CppEditHistoryStatus",
	"/aiserver.v1.AiService/CppAppend":                      "https://api3.cursor.sh:443/aiserver.v1.AiService/CppAppend",
	"/aiserver.v1.AiService/CppEditHistoryAppend":           "https://api3.cursor.sh:443/aiserver.v1.AiService/CppEditHistoryAppend",
	"/aiserver.v1.CppService/AvailableModels":               "https://api3.cursor.sh:443/aiserver.v1.CppService/AvailableModels",
	"/aiserver.v1.CppService/RecordCppFate":                 "https://api2.cursor.sh:443/aiserver.v1.CppService/RecordCppFate",
	"/aiserver.v1.AiService/ReportAiCodeChangeMetrics":      "https://api2.cursor.sh:443/aiserver.v1.AiService/ReportAiCodeChangeMetrics",
	"/aiserver.v1.AiService/WriteGitCommitMessage":          "https://api2.cursor.sh:443/aiserver.v1.AiService/WriteGitCommitMessage",
	"/aiserver.v1.AiService/WriteGitBranchName":             "https://api2.cursor.sh:443/aiserver.v1.AiService/WriteGitBranchName",
	"/aiserver.v1.FileSyncService/FSSyncFile":               "https://api4.cursor.sh:443/aiserver.v1.FileSyncService/FSSyncFile",
	"/aiserver.v1.FileSyncService/FSIsEnabledForUser":       "https://api4.cursor.sh:443/aiserver.v1.FileSyncService/FSIsEnabledForUser",
	"/aiserver.v1.FileSyncService/FSConfig":                 "https://api4.cursor.sh:443/aiserver.v1.FileSyncService/FSConfig",
	"/aiserver.v1.FileSyncService/FSUploadFile":             "https://api4.cursor.sh:443/aiserver.v1.FileSyncService/FSUploadFile",
	"/aiserver.v1.DashboardService/GetEffectiveUserPlugins": "https://api2.cursor.sh:443/aiserver.v1.DashboardService/GetEffectiveUserPlugins",
}

// appConfig 表示服务启动所需的 Cursor 凭据和本地监听地址。
// lyh用cursor修改 2026-08-01：扩展 YAML 配置结构，使宝塔可配置回环监听地址。
type appConfig struct {
	Token      string `yaml:"token"`
	ListenAddr string `yaml:"listen_addr"`
}

type serverApp struct {
	config          appConfig
	client          *http.Client
	upstreamTargets map[string]string
}

// main 加载宝塔或本地运行配置，并启动 Cursor Tab HTTP 转发服务。
// lyh用cursor修改 2026-08-01：允许宝塔通过环境变量注入运行配置，并默认仅监听回环地址以避免服务端口直接暴露。
func main() {
	configPath := resolveConfigPath(os.Getenv)
	cfg, err := loadConfig(configPath, os.Getenv)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	log.Printf("cursor-tab-server 启动 listen_addr=%s config_path=%s", cfg.ListenAddr, configPath)
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           newServerApp(cfg, newHTTPClient(), defaultUpstreamTargets),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		_, _ = fmt.Fprintf(os.Stderr, "监听失败: %v\n", err)
		os.Exit(1)
	}
}

func newServerApp(cfg appConfig, client *http.Client, upstreamTargets map[string]string) http.Handler {
	app := &serverApp{
		config:          cfg,
		client:          client,
		upstreamTargets: cloneUpstreamTargets(upstreamTargets),
	}
	if app.client == nil {
		app.client = newHTTPClient()
	}
	return app
}

// ServeHTTP 提供健康检查并将已登记的 Cursor Tab 请求转发到对应官方上游。
// writer 用于返回健康状态或上游响应；request 表示调用方提交的 HTTP 请求。
// lyh用cursor修改 2026-08-01：增加不访问上游的健康检查端点，供宝塔监控服务存活状态。
func (app *serverApp) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == healthPath {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writer.WriteHeader(http.StatusOK)
		if request.Method != http.MethodHead {
			_, _ = writer.Write([]byte("ok"))
		}
		return
	}
	if err := app.handleProxy(writer, request); err != nil {
		http.Error(writer, err.Error(), http.StatusBadGateway)
	}
}

func (app *serverApp) handleProxy(writer http.ResponseWriter, request *http.Request) error {
	if app == nil {
		return fmt.Errorf("服务实例为空")
	}
	rawTarget, ok := app.upstreamTargets[strings.TrimSpace(request.URL.Path)]
	if !ok {
		http.NotFound(writer, request)
		return nil
	}
	targetURL, err := url.Parse(rawTarget)
	if err != nil {
		return fmt.Errorf("解析上游地址失败: %w", err)
	}
	targetURL.RawQuery = request.URL.RawQuery

	requestBody := []byte{}
	if shouldRequestCarryBody(request.Method) {
		requestBody, err = io.ReadAll(request.Body)
		if err != nil {
			return fmt.Errorf("读取请求体失败: %w", err)
		}
	}

	upstreamRequest, err := http.NewRequestWithContext(request.Context(), request.Method, targetURL.String(), bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("构建上游请求失败: %w", err)
	}
	copyRequestHeaders(upstreamRequest.Header, request.Header)
	authorization := formatBearerAuthorization(app.config.Token)
	upstreamRequest.Header.Set("Authorization", authorization)
	upstreamRequest.Header.Set("x-cursor-checksum", buildCursorChecksum(authorization))
	if !shouldRequestCarryBody(request.Method) {
		upstreamRequest.Header.Del("content-length")
	} else {
		upstreamRequest.Header.Set("content-length", strconv.Itoa(len(requestBody)))
	}
	upstreamRequest.Host = targetURL.Host

	response, err := app.client.Do(upstreamRequest)
	if err != nil {
		log.Printf("上游转发失败 method=%s path=%s target=%s err=%v", request.Method, request.URL.Path, targetURL.String(), err)
		return fmt.Errorf("上游请求失败: %w", err)
	}
	defer response.Body.Close()
	log.Printf("上游响应 method=%s path=%s target_host=%s status=%d", request.Method, request.URL.Path, targetURL.Host, response.StatusCode)

	copyResponseHeaders(writer.Header(), response.Header)
	writer.WriteHeader(response.StatusCode)
	_, err = copyStream(writer, response.Body)
	return err
}

// resolveConfigPath 确定运行时 YAML 配置路径，供宝塔环境变量覆盖默认工作目录配置。
// getenv 用于读取环境变量；返回实际应读取的配置文件路径。
func resolveConfigPath(getenv func(string) string) string {
	if getenv != nil {
		if path := strings.TrimSpace(getenv(envConfigPath)); path != "" {
			return path
		}
	}
	return defaultConfigPath
}

// loadConfig 合并 YAML 与环境变量配置，环境变量优先，允许宝塔仅通过环境变量启动服务。
// path 表示 YAML 配置文件路径；getenv 用于读取宝塔注入的环境变量；返回完成校验的运行配置。
// lyh用cursor修改 2026-08-01：统一配置优先级并允许省略 YAML，避免部署时必须上传包含 Cursor Token 的文件。
func loadConfig(path string, getenv func(string) string) (appConfig, error) {
	cfg := appConfig{ListenAddr: defaultListenAddr}
	contents, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(contents, &cfg); err != nil {
			return appConfig{}, fmt.Errorf("解析配置失败: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return appConfig{}, fmt.Errorf("读取配置失败: %w", err)
	}

	if getenv != nil {
		if token := strings.TrimSpace(getenv(envToken)); token != "" {
			cfg.Token = token
		}
		if listenAddr := strings.TrimSpace(getenv(envListenAddr)); listenAddr != "" {
			cfg.ListenAddr = listenAddr
		}
	}
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.ListenAddr = strings.TrimSpace(cfg.ListenAddr)
	if cfg.Token == "" {
		return appConfig{}, fmt.Errorf("token 不能为空，请设置 %s 或在配置文件中填写 token", envToken)
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultListenAddr
	}
	return cfg, nil
}

func copyRequestHeaders(target http.Header, source http.Header) {
	for key, values := range source {
		lowerKey := strings.ToLower(key)
		if _, exists := hopByHopHeaders[lowerKey]; exists {
			continue
		}
		for _, value := range values {
			target.Add(key, value)
		}
	}
}

func copyResponseHeaders(target http.Header, source http.Header) {
	for key, values := range source {
		lowerKey := strings.ToLower(key)
		if _, exists := hopByHopHeaders[lowerKey]; exists {
			continue
		}
		for _, value := range values {
			target.Add(key, value)
		}
	}
}

func copyStream(writer io.Writer, reader io.Reader) (int64, error) {
	buffer := make([]byte, 32*1024)
	var total int64
	for {
		readCount, readErr := reader.Read(buffer)
		if readCount > 0 {
			chunk := buffer[:readCount]
			written, writeErr := writer.Write(chunk)
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written < len(chunk) {
				return total, io.ErrShortWrite
			}
			if flusher, ok := writer.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return total, nil
			}
			return total, readErr
		}
	}
}

func shouldRequestCarryBody(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet, http.MethodHead, http.MethodDelete:
		return false
	default:
		return true
	}
}

func formatBearerAuthorization(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return value
	}
	return "Bearer " + value
}

func buildCursorChecksum(authorization string) string {
	const (
		checksumTimestampDivisor = 1_000_000
		checksumInitialSeed      = 165
	)
	timestamp := time.Now().UnixMilli() / checksumTimestampDivisor
	timestampBytes := make([]byte, 6)
	timestampBigInt := big.NewInt(timestamp)
	for index := 0; index < len(timestampBytes); index++ {
		shift := uint((len(timestampBytes) - 1 - index) * 8)
		timestampBytes[index] = byte(new(big.Int).Rsh(timestampBigInt, shift).Uint64() & 0xff)
	}
	seed := checksumInitialSeed
	for index := 0; index < len(timestampBytes); index++ {
		current := int(timestampBytes[index]^byte(seed)) + (index % 256)
		current &= 0xff
		timestampBytes[index] = byte(current)
		seed = current
	}
	prefix := strings.TrimRight(base64.StdEncoding.EncodeToString(timestampBytes), "=")
	hashBytes := sha256.Sum256([]byte(strings.TrimSpace(authorization)))
	hash := fmt.Sprintf("%x", hashBytes)
	return prefix + hash[:32]
}

func newHTTPClient() *http.Client {
	return &http.Client{}
}

func cloneUpstreamTargets(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
