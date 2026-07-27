package netproxy

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"cursor/internal/logger"

	"golang.org/x/net/http/httpproxy"
)

const (
	proxyCacheTTL     = time.Second
	alwaysNoProxyList = "localhost,127.0.0.1,::1"
)

var (
	installOnce             sync.Once
	initialDefaultTransport = cloneDefaultTransport()
	defaultResolver         = &proxyResolver{}
	proxyTransports         sync.Map
	appProxyMu             sync.RWMutex
	appProxyConfig          AppProxyConfig
)

// lyh用cursor修改 2026-07-27：提供运行时可更新的应用内出口代理配置，避免只能依赖环境变量或系统代理。
type AppProxyConfig struct {
	Enabled bool
	Mode    string
	URL     string
}

// InstallDefaultTransport makes clients with a nil Transport use the same
// proxy resolution as clients created through this package.
func InstallDefaultTransport() {
	installOnce.Do(func() {
		http.DefaultTransport = NewTransport(nil)
		defaultResolver.logCurrentSnapshot("default transport installed")
	})
}

// NewHTTPClient creates an HTTP client that follows environment and OS proxy
// settings while preserving the caller's timeout choice.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: NewTransport(nil),
		Timeout:   timeout,
	}
}

// lyh用cursor修改 2026-07-27：同步用户配置中的出口代理策略，让所有复用 netproxy 的 HTTP 客户端即时切换出口。
// SetAppProxyConfig 更新应用内出口代理配置。
// cfg.Mode 支持 configured、system 和 direct；configured 会优先使用 cfg.URL。
// 已有空闲连接会被关闭，确保后续请求使用新的代理策略。
func SetAppProxyConfig(cfg AppProxyConfig) {
	next := AppProxyConfig{
		Enabled: cfg.Enabled,
		Mode:    strings.ToLower(strings.TrimSpace(cfg.Mode)),
		URL:     strings.TrimSpace(cfg.URL),
	}
	appProxyMu.Lock()
	changed := appProxyConfig != next
	appProxyConfig = next
	appProxyMu.Unlock()
	if changed {
		defaultResolver.logCurrentSnapshot("app proxy config updated")
	}
}

// NewTransport clones the given transport and installs proxy resolution on it.
// When base is nil, it clones Go's original default transport.
func NewTransport(base *http.Transport) *http.Transport {
	transport := initialDefaultTransport.Clone()
	if base != nil {
		transport = base.Clone()
	}
	transport.Proxy = ProxyForRequest
	proxyTransports.Store(transport, struct{}{})
	return transport
}

// ProxyForRequest resolves the proxy URL for a single request.
func ProxyForRequest(req *http.Request) (*url.URL, error) {
	if req == nil || req.URL == nil {
		return nil, nil
	}
	return defaultResolver.proxyForURL(req.URL)
}

// CurrentStatus returns the latest proxy resolver snapshot without exposing
// proxy credentials.
func CurrentStatus() Status {
	return statusFromSnapshot(defaultResolver.currentSnapshot())
}

type proxyResolver struct {
	mu       sync.Mutex
	snapshot proxySnapshot
}

type proxySnapshot struct {
	expiresAt      time.Time
	source         string
	active         bool
	description    string
	key            string
	httpProxy      string
	httpsProxy     string
	configuredURL  string
	proxyFunc      func(*url.URL) (*url.URL, error)
	systemBypass   []string
	excludeSimple  bool
	pacIgnored     bool
	loadErrMessage string
}

// Status is a sanitized summary of the proxy resolver's current decision.
type Status struct {
	Source                 string `json:"source"`
	Active                 bool   `json:"active"`
	UsingSystemProxy       bool   `json:"usingSystemProxy"`
	UsingEnvProxy          bool   `json:"usingEnvProxy"`
	UsingConfiguredProxy   bool   `json:"usingConfiguredProxy"`
	HTTPProxy              string `json:"httpProxy"`
	HTTPSProxy             string `json:"httpsProxy"`
	ConfiguredProxyURL     string `json:"configuredProxyURL"`
	Description            string `json:"description"`
	PACIgnored             bool   `json:"pacIgnored"`
	LoadError              string `json:"loadError"`
}

type systemProxyConfig struct {
	Source         string
	HTTPProxy      string
	HTTPSProxy     string
	SOCKSProxy     string
	Bypass         []string
	ExcludeSimple  bool
	PACEnabled     bool
	PACURL         string
	LoadErrMessage string
}

func (resolver *proxyResolver) proxyForURL(reqURL *url.URL) (*url.URL, error) {
	snapshot := resolver.currentSnapshot()
	if snapshot.proxyFunc == nil {
		return nil, nil
	}
	if shouldBypassSystemProxy(reqURL, snapshot.systemBypass, snapshot.excludeSimple) {
		return nil, nil
	}
	return snapshot.proxyFunc(reqURL)
}

func (resolver *proxyResolver) currentSnapshot() proxySnapshot {
	now := time.Now()
	resolver.mu.Lock()
	defer resolver.mu.Unlock()

	if !resolver.snapshot.expiresAt.IsZero() && now.Before(resolver.snapshot.expiresAt) {
		return resolver.snapshot
	}
	next := buildProxySnapshot(now)
	if next.key != resolver.snapshot.key {
		logProxySnapshot(next)
		closeIdleProxyConnections()
	}
	resolver.snapshot = next
	return next
}

func (resolver *proxyResolver) logCurrentSnapshot(prefix string) {
	resolver.mu.Lock()
	next := buildProxySnapshot(time.Now())
	if prefix != "" {
		next.description = strings.TrimSpace(prefix + "; " + next.description)
	}
	logProxySnapshot(next)
	resolver.snapshot = next
	closeIdleProxyConnections()
	resolver.mu.Unlock()
}

// appProxyMode 返回当前应用内出口策略，用于决定是否允许环境变量代理参与回退。
func appProxyMode() string {
	appProxyMu.RLock()
	mode := strings.ToLower(strings.TrimSpace(appProxyConfig.Mode))
	appProxyMu.RUnlock()
	return mode
}

func buildProxySnapshot(now time.Time) proxySnapshot {
	// lyh用cursor修改 2026-07-27：优先使用应用内出口代理，避免 Cursor 外网请求被系统代理状态影响。
	if snapshot, ok := appProxySnapshot(now); ok {
		return snapshot
	}

	if appProxyMode() != "system" {
		if cfg, ok := envProxyConfig(); ok {
			return snapshotFromConfig(now, "env", cfg, nil, false, false, "")
		}
	}

	systemConfig := loadSystemProxyConfig()
	httpProxy := systemConfig.HTTPProxy
	httpsProxy := systemConfig.HTTPSProxy
	if systemConfig.SOCKSProxy != "" {
		if httpProxy == "" {
			httpProxy = systemConfig.SOCKSProxy
		}
		if httpsProxy == "" {
			httpsProxy = systemConfig.SOCKSProxy
		}
	}
	if httpProxy == "" && httpsProxy == "" {
		return proxySnapshot{
			expiresAt:      now.Add(proxyCacheTTL),
			source:         directSource(systemConfig),
			description:    directDescription(systemConfig),
			key:            directKey(systemConfig),
			pacIgnored:     systemConfig.PACEnabled,
			loadErrMessage: systemConfig.LoadErrMessage,
		}
	}

	cfg := httpproxy.Config{
		HTTPProxy:  httpProxy,
		HTTPSProxy: httpsProxy,
		NoProxy:    joinNoProxy(alwaysNoProxyList, envNoProxy(), strings.Join(systemConfig.Bypass, ",")),
	}
	return snapshotFromConfig(now, "system", cfg, systemConfig.Bypass, systemConfig.ExcludeSimple, systemConfig.PACEnabled, systemConfig.LoadErrMessage)
}

func snapshotFromConfig(now time.Time, source string, cfg httpproxy.Config, bypass []string, excludeSimple bool, pacIgnored bool, loadErr string) proxySnapshot {
	proxyFunc := cfg.ProxyFunc()
	httpDesc := sanitizeProxyValue(cfg.HTTPProxy)
	httpsDesc := sanitizeProxyValue(cfg.HTTPSProxy)
	active := httpDesc != "" || httpsDesc != ""
	description := fmt.Sprintf("source=%s http=%s https=%s", source, displayProxyDesc(httpDesc), displayProxyDesc(httpsDesc))
	if cfg.NoProxy != "" {
		description += " no_proxy=configured"
	}
	if excludeSimple {
		description += " exclude_simple=true"
	}
	if pacIgnored {
		description += " pac=ignored"
	}
	if loadErr != "" {
		description += " load_error=" + loadErr
	}
	return proxySnapshot{
		expiresAt:      now.Add(proxyCacheTTL),
		source:         source,
		active:         active,
		description:    description,
		key:            strings.Join([]string{source, httpDesc, httpsDesc, cfg.NoProxy, fmt.Sprint(excludeSimple), fmt.Sprint(pacIgnored), loadErr}, "|"),
		httpProxy:      httpDesc,
		httpsProxy:     httpsDesc,
		proxyFunc:      proxyFunc,
		systemBypass:   cleanBypassRules(bypass),
		excludeSimple:  excludeSimple,
		pacIgnored:     pacIgnored,
		loadErrMessage: loadErr,
	}
}

// lyh用cursor修改 2026-07-27：将应用内出口代理策略置于环境变量和系统代理之前，保证 v2rayN 本地端口优先生效。
// appProxySnapshot 根据当前应用内代理配置生成代理解析快照。
// now 用于计算快照缓存过期时间。
// 返回值包含代理快照，以及本次是否由应用内配置接管代理解析。
func appProxySnapshot(now time.Time) (proxySnapshot, bool) {
	appProxyMu.RLock()
	cfg := appProxyConfig
	appProxyMu.RUnlock()
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	switch mode {
	case "direct":
		return proxySnapshot{
			expiresAt:   now.Add(proxyCacheTTL),
			source:      "direct",
			description: "source=direct app_proxy=direct",
			key:         "app|direct",
		}, true
	case "system", "":
		return proxySnapshot{}, false
	case "configured":
		if !cfg.Enabled {
			return proxySnapshot{}, false
		}
	}
	proxyURL := strings.TrimSpace(cfg.URL)
	if proxyURL == "" {
		return proxySnapshot{}, false
	}
	proxyConfig := httpproxy.Config{
		HTTPProxy:  proxyURL,
		HTTPSProxy: proxyURL,
		NoProxy:    joinNoProxy(alwaysNoProxyList, envNoProxy()),
	}
	snapshot := snapshotFromConfig(now, "configured", proxyConfig, nil, false, false, "")
	snapshot.configuredURL = sanitizeProxyValue(proxyURL)
	snapshot.key = strings.Join([]string{snapshot.key, snapshot.configuredURL}, "|")
	return snapshot, true
}

func envProxyConfig() (httpproxy.Config, bool) {
	allProxy := firstEnv("ALL_PROXY", "all_proxy")
	httpProxy := firstEnv("HTTP_PROXY", "http_proxy")
	httpsProxy := firstEnv("HTTPS_PROXY", "https_proxy")
	if httpProxy == "" {
		httpProxy = allProxy
	}
	if httpsProxy == "" {
		httpsProxy = allProxy
	}
	hasProxy := httpProxy != "" || httpsProxy != "" || allProxy != ""
	return httpproxy.Config{
		HTTPProxy:  httpProxy,
		HTTPSProxy: httpsProxy,
		NoProxy:    joinNoProxy(alwaysNoProxyList, envNoProxy()),
		CGI:        os.Getenv("REQUEST_METHOD") != "",
	}, hasProxy
}

func envNoProxy() string {
	return firstEnv("NO_PROXY", "no_proxy")
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func directSource(cfg systemProxyConfig) string {
	if cfg.PACEnabled {
		return "system-pac-ignored"
	}
	if cfg.LoadErrMessage != "" {
		return "direct"
	}
	return "none"
}

func directDescription(cfg systemProxyConfig) string {
	parts := []string{"source=direct"}
	if cfg.PACEnabled {
		parts = append(parts, "pac=ignored")
	}
	if cfg.LoadErrMessage != "" {
		parts = append(parts, "load_error="+cfg.LoadErrMessage)
	}
	return strings.Join(parts, " ")
}

func directKey(cfg systemProxyConfig) string {
	return strings.Join([]string{"direct", fmt.Sprint(cfg.PACEnabled), cfg.PACURL, cfg.LoadErrMessage}, "|")
}

func logProxySnapshot(snapshot proxySnapshot) {
	if snapshot.description == "" {
		snapshot.description = "source=direct"
	}
	logger.Infof("net proxy: %s", snapshot.description)
}

func statusFromSnapshot(snapshot proxySnapshot) Status {
	source := strings.TrimSpace(snapshot.source)
	if source == "" || source == "none" || source == "system-pac-ignored" {
		source = "direct"
	}
	// lyh用cursor修改 2026-07-27：在代理状态中暴露 configured 命中信息，便于确认请求是否走 v2rayN。
	return Status{
		Source:               source,
		Active:               snapshot.active,
		UsingSystemProxy:     snapshot.active && snapshot.source == "system",
		UsingEnvProxy:        snapshot.active && snapshot.source == "env",
		UsingConfiguredProxy: snapshot.active && snapshot.source == "configured",
		HTTPProxy:            snapshot.httpProxy,
		HTTPSProxy:           snapshot.httpsProxy,
		ConfiguredProxyURL:   snapshot.configuredURL,
		Description:          snapshot.description,
		PACIgnored:           snapshot.pacIgnored,
		LoadError:            snapshot.loadErrMessage,
	}
}

func closeIdleProxyConnections() {
	proxyTransports.Range(func(key any, _ any) bool {
		if transport, ok := key.(*http.Transport); ok && transport != nil {
			transport.CloseIdleConnections()
		}
		return true
	})
}

func cloneDefaultTransport() *http.Transport {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		clone := transport.Clone()
		clone.Proxy = nil
		return clone
	}
	return &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func joinNoProxy(parts ...string) string {
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		for _, value := range strings.FieldsFunc(part, func(r rune) bool {
			return r == ',' || r == ';'
		}) {
			value = strings.TrimSpace(value)
			if value == "" || strings.EqualFold(value, "<local>") {
				continue
			}
			items = append(items, value)
		}
	}
	return strings.Join(items, ",")
}

func cleanBypassRules(rules []string) []string {
	cleaned := make([]string, 0, len(rules))
	for _, rule := range rules {
		for _, value := range strings.FieldsFunc(rule, func(r rune) bool {
			return r == ',' || r == ';'
		}) {
			value = strings.ToLower(strings.TrimSpace(value))
			if value == "" {
				continue
			}
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func shouldBypassSystemProxy(reqURL *url.URL, rules []string, excludeSimple bool) bool {
	if reqURL == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(reqURL.Hostname()), "."))
	if host == "" {
		return false
	}
	if excludeSimple && isSimpleHostname(host) {
		return true
	}
	for _, rule := range rules {
		if ruleMatchesHost(rule, host) {
			return true
		}
	}
	return false
}

func isSimpleHostname(host string) bool {
	if host == "" || strings.Contains(host, ".") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return false
	}
	return !strings.Contains(host, ":")
}

func ruleMatchesHost(rule string, host string) bool {
	rule = strings.TrimSpace(strings.TrimSuffix(rule, "."))
	if rule == "" {
		return false
	}
	if rule == "*" {
		return true
	}
	if strings.EqualFold(rule, "<local>") {
		return isSimpleHostname(host)
	}
	if strings.Contains(rule, "://") {
		if parsed, err := url.Parse(rule); err == nil {
			rule = strings.TrimSpace(parsed.Hostname())
		}
	}
	if strings.Contains(rule, ":") {
		if ruleHost, _, err := net.SplitHostPort(rule); err == nil {
			rule = ruleHost
		}
	}
	rule = strings.ToLower(strings.TrimSuffix(rule, "."))
	if strings.ContainsAny(rule, "*?") {
		matched, err := path.Match(rule, host)
		return err == nil && matched
	}
	if strings.HasPrefix(rule, ".") {
		return strings.HasSuffix(host, rule)
	}
	if strings.HasPrefix(rule, "*.") {
		return strings.HasSuffix(host, strings.TrimPrefix(rule, "*"))
	}
	return host == rule || strings.HasSuffix(host, "."+rule)
}

func sanitizeProxyValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		parsed, err = url.Parse("http://" + raw)
	}
	if err != nil || parsed == nil || parsed.Host == "" {
		return "invalid"
	}
	scheme := strings.TrimSpace(parsed.Scheme)
	if scheme == "" {
		scheme = "http"
	}
	return scheme + "://" + parsed.Host
}

func displayProxyDesc(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func normalizeProxyAddress(defaultScheme string, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		return raw
	}
	if defaultScheme == "" {
		defaultScheme = "http"
	}
	return defaultScheme + "://" + raw
}
