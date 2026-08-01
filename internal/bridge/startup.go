package bridge

import "cursor/internal/autostart"

// StartupService 向主界面提供操作系统开机启动管理能力。
type StartupService struct{}

// lyh用cursor修改 2026-08-01：创建独立开机启动服务，避免将操作系统配置职责混入代理服务。
// NewStartupService 创建不保存界面缓存的开机启动服务。
// 返回值用于注册为 Wails 服务，所有状态均实时读取操作系统。
func NewStartupService() *StartupService {
	return &StartupService{}
}

// lyh用cursor修改 2026-08-01：向主界面返回操作系统中的真实启动项状态，避免界面缓存造成误导。
// GetStatus 读取当前程序是否已注册为当前用户的开机启动项。
// 返回值包含平台支持状态和当前程序路径对应的启用状态。
func (s *StartupService) GetStatus() (autostart.Status, error) {
	_ = s
	return autostart.GetStatus()
}

// lyh用cursor修改 2026-08-01：通过独立桥接服务修改启动项，保持界面与平台注册逻辑职责分离。
// SetEnabled 根据 enabled 添加或删除当前用户的开机启动项。
// enabled 为 true 时添加，false 时删除；返回修改后的操作系统实际状态。
func (s *StartupService) SetEnabled(enabled bool) (autostart.Status, error) {
	_ = s
	return autostart.SetEnabled(enabled)
}
