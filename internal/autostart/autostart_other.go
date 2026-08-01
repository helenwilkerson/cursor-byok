//go:build !windows

package autostart

import "fmt"

// lyh用cursor修改 2026-08-01：非 Windows 平台返回明确的不支持状态，避免跨平台构建依赖 Windows 注册表。
// getPlatformStatus 返回当前平台不支持开机启动管理的状态。
func getPlatformStatus() (Status, error) {
	return Status{Supported: false}, nil
}

// lyh用cursor修改 2026-08-01：非 Windows 平台拒绝写入启动项，避免界面显示虚假的成功状态。
// setPlatformEnabled 拒绝在尚未实现的平台上修改开机启动状态。
// enabled 表示调用方期望的状态；返回不支持该能力的明确错误。
func setPlatformEnabled(enabled bool) (Status, error) {
	_ = enabled
	return Status{Supported: false}, fmt.Errorf("当前系统暂不支持开机启动设置")
}
