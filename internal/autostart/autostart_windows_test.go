//go:build windows

package autostart

import "testing"

// lyh用cursor修改 2026-08-01：覆盖当前路径、旧路径和参数异常场景，防止启动项状态误判。
// TestStartupCommandsMatch 验证 Windows 启动命令仅在完整匹配当前程序与专用参数时有效。
func TestStartupCommandsMatch(t *testing.T) {
	executablePath := `C:\Program Files\Cursor Assistant\Cursor助手.exe`

	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{name: "current command", command: `"C:\Program Files\Cursor Assistant\Cursor助手.exe" --autostart`, want: true},
		{name: "case insensitive", command: `"c:\program files\cursor assistant\cursor助手.exe" --autostart`, want: true},
		{name: "missing startup argument", command: `"C:\Program Files\Cursor Assistant\Cursor助手.exe"`, want: false},
		{name: "stale executable", command: `"D:\Cursor助手.exe" --autostart`, want: false},
		{name: "extra argument", command: `"C:\Program Files\Cursor Assistant\Cursor助手.exe" --autostart --other`, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := startupCommandsMatch(test.command, executablePath); got != test.want {
				t.Fatalf("startupCommandsMatch(%q, %q) = %v, want %v", test.command, executablePath, got, test.want)
			}
		})
	}
}
