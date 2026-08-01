// Package autostart 管理桌面应用的当前用户开机启动状态。
package autostart

import "os"

const launchArgument = "--autostart"

// lyh用cursor修改 2026-08-01：使用统一状态结构向前端返回平台支持情况和系统实际启用状态。
// Status 表示操作系统中的实际开机启动状态。
type Status struct {
	Supported bool `json:"supported"`
	Enabled   bool `json:"enabled"`
}

// lyh用cursor修改 2026-08-01：通过统一入口读取系统启动项，避免界面缓存与操作系统实际状态不一致。
// GetStatus 读取当前可执行程序是否已注册为当前用户的开机启动项。
// 返回值中的 Supported 表示当前操作系统是否支持该能力，Enabled 表示启动项是否指向当前程序。
func GetStatus() (Status, error) {
	return getPlatformStatus()
}

// lyh用cursor修改 2026-08-01：集中管理系统启动项写入和删除，避免平台逻辑扩散到界面桥接层。
// SetEnabled 根据 enabled 添加或删除当前可执行程序的开机启动项。
// enabled 为 true 时注册当前程序，为 false 时移除启动项；返回操作后的系统实际状态。
func SetEnabled(enabled bool) (Status, error) {
	return setPlatformEnabled(enabled)
}

// lyh用cursor修改 2026-08-01：区分系统登录启动与用户手动启动，使开机启动时仅驻留托盘而不打扰用户。
// IsLaunch 判断当前进程是否由本应用写入的开机启动项拉起。
// 返回 true 表示命令行包含专用开机启动参数，应用应隐藏主窗口启动。
func IsLaunch() bool {
	for _, argument := range os.Args[1:] {
		if argument == launchArgument {
			return true
		}
	}
	return false
}
