package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip 将测试请求交给自定义函数，以便验证代理发往 Cursor 上游的内容。
// request 表示代理构造的上游请求；返回模拟的 HTTP 响应或错误。
func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

// TestLoadConfigEnvironmentOverridesYAML 验证宝塔环境变量优先于 YAML 配置。
// lyh用cursor修改 2026-08-01：锁定部署配置优先级，防止服务器仍使用文件中的旧 Token 或监听地址。
func TestLoadConfigEnvironmentOverridesYAML(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("token: yaml-token\nlisten_addr: 0.0.0.0:9000\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	environment := map[string]string{
		envToken:      "env-token",
		envListenAddr: "127.0.0.1:8041",
	}

	cfg, err := loadConfig(configPath, func(key string) string { return environment[key] })
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Token != "env-token" {
		t.Fatalf("expected env token, got %q", cfg.Token)
	}
	if cfg.ListenAddr != "127.0.0.1:8041" {
		t.Fatalf("expected env listen address, got %q", cfg.ListenAddr)
	}
}

// TestLoadConfigWithoutYAML 验证仅设置 Cursor Token 环境变量即可在宝塔启动。
// lyh用cursor修改 2026-08-01：覆盖无密钥文件部署场景，并确认默认只监听回环地址。
func TestLoadConfigWithoutYAML(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.yaml")
	cfg, err := loadConfig(missingPath, func(key string) string {
		if key == envToken {
			return "env-token"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.ListenAddr != defaultListenAddr {
		t.Fatalf("expected default listen address %q, got %q", defaultListenAddr, cfg.ListenAddr)
	}
}

// TestHealthEndpoint 验证宝塔健康检查不会调用任何 Cursor 上游。
// lyh用cursor修改 2026-08-01：确保健康检查可独立反映本地进程存活状态。
func TestHealthEndpoint(t *testing.T) {
	handler := newServerApp(appConfig{Token: "test-token"}, nil, nil)
	request := httptest.NewRequest(http.MethodGet, healthPath, nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if strings.TrimSpace(response.Body.String()) != "ok" {
		t.Fatalf("expected health body ok, got %q", response.Body.String())
	}
}

// TestUnknownRoute 验证未登记路径不会被转发至 Cursor 上游。
// lyh用cursor修改 2026-08-01：限制代理入口为明确路由，避免宝塔域名成为开放转发器。
func TestUnknownRoute(t *testing.T) {
	handler := newServerApp(appConfig{Token: "test-token"}, nil, nil)
	request := httptest.NewRequest(http.MethodPost, "/unknown", strings.NewReader("payload"))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.Code)
	}
}

// TestProxyPreservesRequestData 验证请求体、查询参数和业务请求头会被完整送往已配置的 Cursor 上游。
// lyh用cursor修改 2026-08-01：防止 Linux 部署改造破坏 Tab 流量转发协议。
func TestProxyPreservesRequestData(t *testing.T) {
	var capturedRequest *http.Request
	var capturedBody string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		capturedRequest = request
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read upstream body: %v", err)
		}
		capturedBody = string(body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("upstream-ok")),
		}, nil
	})}
	const route = "/aiserver.v1.AiService/StreamCpp"
	handler := newServerApp(
		appConfig{Token: "server-token"},
		client,
		map[string]string{route: "https://api4.cursor.sh:443" + route},
	)
	request := httptest.NewRequest(http.MethodPost, route+"?mode=test", strings.NewReader("request-payload"))
	request.Header.Set("Content-Type", "application/proto")
	request.Header.Set("X-Cursor-Test", "kept")
	request.Header.Set("Authorization", "Bearer client-token")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if capturedRequest == nil {
		t.Fatal("expected upstream request")
	}
	if capturedRequest.URL.RawQuery != "mode=test" {
		t.Fatalf("expected query to be preserved, got %q", capturedRequest.URL.RawQuery)
	}
	if capturedBody != "request-payload" {
		t.Fatalf("expected request body to be preserved, got %q", capturedBody)
	}
	if capturedRequest.Header.Get("X-Cursor-Test") != "kept" {
		t.Fatalf("expected custom header to be preserved")
	}
	if capturedRequest.Header.Get("Authorization") != "Bearer server-token" {
		t.Fatalf("expected configured authorization header, got %q", capturedRequest.Header.Get("Authorization"))
	}
	if capturedRequest.Header.Get("x-cursor-checksum") == "" {
		t.Fatal("expected cursor checksum header")
	}
}
