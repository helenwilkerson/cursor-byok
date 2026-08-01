//go:build windows

package autostart

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const (
	startupRegistryPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	startupValueName    = "CursorAssistant"
)

// lyh用cursor修改 2026-08-01：以当前可执行程序路径校验 Windows 启动项，避免旧安装路径被误判为已启用。
// getPlatformStatus 读取 HKCU 启动项并确认其命令是否指向当前可执行程序。
// 返回当前 Windows 用户的真实启动状态；注册表访问失败时返回错误。
func getPlatformStatus() (Status, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return Status{Supported: true}, fmt.Errorf("获取当前程序路径失败: %w", err)
	}

	key, err := registry.OpenKey(registry.CURRENT_USER, startupRegistryPath, registry.QUERY_VALUE)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return Status{Supported: true}, nil
	}
	if err != nil {
		return Status{Supported: true}, fmt.Errorf("打开 Windows 开机启动项失败: %w", err)
	}
	defer key.Close()

	command, _, err := key.GetStringValue(startupValueName)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return Status{Supported: true}, nil
	}
	if err != nil {
		return Status{Supported: true}, fmt.Errorf("读取 Windows 开机启动项失败: %w", err)
	}

	return Status{
		Supported: true,
		Enabled:   startupCommandsMatch(command, executablePath),
	}, nil
}

// lyh用cursor修改 2026-08-01：在 HKCU Run 中增删启动项，使普通用户无需管理员权限即可控制开机启动。
// setPlatformEnabled 根据 enabled 写入或删除当前用户的 Windows 开机启动项。
// enabled 为 true 时写入当前可执行程序的绝对路径，为 false 时删除对应值；返回写入后的实际状态。
func setPlatformEnabled(enabled bool) (Status, error) {
	key, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		startupRegistryPath,
		registry.QUERY_VALUE|registry.SET_VALUE,
	)
	if err != nil {
		return Status{Supported: true}, fmt.Errorf("打开 Windows 开机启动项失败: %w", err)
	}
	defer key.Close()

	if !enabled {
		if err := key.DeleteValue(startupValueName); err != nil && !errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return Status{Supported: true}, fmt.Errorf("删除 Windows 开机启动项失败: %w", err)
		}
		return Status{Supported: true}, nil
	}

	executablePath, err := os.Executable()
	if err != nil {
		return Status{Supported: true}, fmt.Errorf("获取当前程序路径失败: %w", err)
	}
	startupCommand := quoteWindowsCommandPath(executablePath) + " " + launchArgument
	if err := key.SetStringValue(startupValueName, startupCommand); err != nil {
		return Status{Supported: true}, fmt.Errorf("写入 Windows 开机启动项失败: %w", err)
	}
	return Status{Supported: true, Enabled: true}, nil
}

// lyh用cursor修改 2026-08-01：统一生成带引号的可执行程序命令，确保安装路径包含空格时仍能启动。
// quoteWindowsCommandPath 为可能包含空格的可执行程序路径生成安全的启动命令。
// executablePath 表示当前程序绝对路径；返回始终使用双引号包裹的 Windows 命令。
func quoteWindowsCommandPath(executablePath string) string {
	return `"` + strings.TrimSpace(executablePath) + `"`
}

// lyh用cursor修改 2026-08-01：同时校验程序路径和专用参数，避免旧启动项或其他命令被误判为当前配置。
// startupCommandsMatch 判断注册表命令是否仅指向当前可执行程序。
// command 表示注册表中的启动命令，executablePath 表示当前程序路径；返回是否匹配。
func startupCommandsMatch(command string, executablePath string) bool {
	normalizedCommand := strings.TrimSpace(command)
	expectedCommand := quoteWindowsCommandPath(executablePath) + " " + launchArgument
	return strings.EqualFold(normalizedCommand, expectedCommand)
}
