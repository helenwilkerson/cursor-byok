package client

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"cursor/internal/appdata"
	backend "cursor/internal/backend"
	serverconfig "cursor/internal/backend/server/config"
	"cursor/internal/certs"
	"cursor/internal/logger"
	"cursor/internal/mitm"
	"cursor/internal/netproxy"
)

const (
	// publicAPITimeout 表示当前模块中的 publicAPITimeout 状态值。
	publicAPITimeout = 15 * time.Second
	// backendReadyTimeout 表示等待嵌入式 backend 就绪的最长时间。
	backendReadyTimeout = 15 * time.Second
	// backendHealthCheckInterval 表示轮询 backend 健康检查的间隔。
	backendHealthCheckInterval = 1 * time.Second
	// backendHealthCheckAttemptTimeout 限制单次健康检查耗时，避免一次阻塞吃掉全部启动预算。
	backendHealthCheckAttemptTimeout = 1 * time.Second
)

// ProxyService 定义了当前模块中的 ProxyService 类型。
type ProxyService struct {
	// proxy 表示当前声明中的 proxy。
	proxy *mitm.ProxyServer
	// certManager 用于在代理监听地址变化时重建 MITM 服务。
	certManager *certs.Manager
	// backendHost 表示当前嵌入式 backend 服务。
	backendHost *backend.Host

	// mu 表示当前声明中的 mu。
	mu sync.RWMutex
	// lastError 表示当前声明中的 lastError。
	lastError string
	// cursorSettingsApplied 表示当前是否已完成宿主代理设置注入。
	cursorSettingsApplied bool

	// configMu 表示当前声明中的 configMu。
	configMu sync.Mutex
	// configPath 表示当前声明中的 configPath。
	configPath string
	// store 表示统一的配置存储。
	store *serverconfig.Store
	// caCertPEM 表示当前声明中的 caCertPEM。
	caCertPEM []byte

	// caFileMu 表示当前声明中的 caFileMu。
	caFileMu sync.Mutex
	// caFilePath 表示当前声明中的 caFilePath。
	caFilePath string

	// publicClient 表示当前声明中的 publicClient。
	publicClient *http.Client
	// logsRoot 表示当前声明中的 logsRoot。
	logsRoot string
	// modelTestMu 保护模型测速缓存。
	modelTestMu sync.RWMutex
	// modelTestResults 保存当前进程内的模型测速结果。
	modelTestResults map[string]ModelAdapterTestResult
}

// NewProxyService 创建本地代理服务门面，并初始化配置、证书、后端宿主和网络出口策略。
// proxy 表示已有 MITM 代理实例；为空时会按用户配置延迟创建。
// certManager 用于签发 Cursor 白名单域名的 MITM 证书。
// caCertPEM 表示需要注入或展示给客户端信任链使用的 CA 证书内容。
func NewProxyService(proxy *mitm.ProxyServer, certManager *certs.Manager, caCertPEM []byte) *ProxyService {
	if err := appdata.EnsureAssistantHome(); err != nil {
		logger.Errorf("ensure assistant home failed: %v", err)
	}
	copiedCert := make([]byte, len(caCertPEM))
	copy(copiedCert, caCertPEM)

	service := &ProxyService{
		proxy:            proxy,
		certManager:      certManager,
		configPath:       resolveUserConfigPath(),
		logsRoot:         resolveLogsRootPath(),
		caCertPEM:        copiedCert,
		publicClient:     netproxy.NewHTTPClient(publicAPITimeout),
		modelTestResults: make(map[string]ModelAdapterTestResult),
	}
	service.store = serverconfig.NewStore(service.configPath, service.logsRoot)
	// lyh用cursor修改 2026-07-27：服务创建阶段即加载出口代理配置，保证更新检查和测速等早期请求也能走 v2rayN。
	if cfg, err := service.store.Load(context.Background()); err != nil {
		logger.Errorf("load outbound proxy config failed: %v", err)
	} else {
		service.applyOutboundProxyConfig(cfg)
	}
	host, err := backend.NewHost(service.store)
	if err != nil {
		logger.Errorf("init backend host failed: %v", err)
	} else {
		service.backendHost = host
		service.attachOutboundProxyConfigSubscription(host)
	}
	return service
}

func (s *ProxyService) ensureBackendHost() error {
	if s == nil {
		return nil
	}
	if s.backendHost != nil {
		return nil
	}
	host, err := backend.NewHost(s.store)
	if err != nil {
		return err
	}
	s.backendHost = host
	s.attachOutboundProxyConfigSubscription(host)
	if cfg, loadErr := s.LoadUserConfig(); loadErr != nil {
		logger.Errorf("load outbound proxy config failed: %v", loadErr)
	} else {
		s.applyOutboundProxyConfig(cfg)
	}
	return nil
}

func (s *ProxyService) ensureProxy(cfg serverconfig.Config) error {
	if s == nil {
		return nil
	}
	baseURL := ""
	if s.backendHost != nil {
		baseURL = s.backendHost.BaseURL()
	}
	if baseURL == "" {
		baseURL = "http://" + cfg.BackendListenAddr
	}
	listenAddr := cfg.ProxyListenAddr

	if s.proxy != nil {
		snapshot := s.proxy.Snapshot()
		if snapshot.ListenAddr == listenAddr {
			return s.proxy.UpdateBaseURL(baseURL)
		}
		if snapshot.Running {
			return fmt.Errorf("代理正在运行，不能从 %s 切换到 %s，请先停止服务", snapshot.ListenAddr, listenAddr)
		}
	}

	proxyServer, err := mitm.NewProxyServer(listenAddr, baseURL, "", "", s.certManager)
	if err != nil {
		return err
	}
	s.proxy = proxyServer
	return nil
}

// lyh用cursor修改 2026-07-27：把持久化配置映射为 netproxy 运行时配置，统一控制所有外网出口请求。
// applyOutboundProxyConfig 将用户配置中的出口代理策略应用到统一网络代理解析器。
// cfg 表示当前标准化后的用户配置。
func (s *ProxyService) applyOutboundProxyConfig(cfg serverconfig.Config) {
	_ = s
	netproxy.SetAppProxyConfig(netproxy.AppProxyConfig{
		Enabled: cfg.OutboundProxy.Enabled,
		Mode:    cfg.OutboundProxy.Mode,
		URL:     cfg.OutboundProxy.URL,
	})
}

// lyh用cursor修改 2026-07-27：订阅配置热更新，确保手动编辑配置文件后出口代理策略也能同步到运行时。
// attachOutboundProxyConfigSubscription 将 backend 配置管理器的变更通知连接到 netproxy。
// host 表示当前嵌入式 backend 实例。
func (s *ProxyService) attachOutboundProxyConfigSubscription(host *backend.Host) {
	if s == nil || host == nil || host.ConfigManager() == nil {
		return
	}
	host.ConfigManager().Subscribe(func(cfg serverconfig.Config) {
		s.applyOutboundProxyConfig(cfg)
		s.emitState()
	})
}

func (s *ProxyService) waitForBackend(ctx context.Context) error {
	if s == nil || s.backendHost == nil {
		return nil
	}
	ticker := time.NewTicker(backendHealthCheckInterval)
	defer ticker.Stop()
	var lastErr error
	for {
		healthCtx, healthCancel := context.WithTimeout(ctx, backendHealthCheckAttemptTimeout)
		err := s.backendHost.HealthCheck(healthCtx)
		healthCancel()
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("等待内置后端就绪失败: %w", lastErr)
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
