package steam

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const stillActive = 259

// Running 报告 Steam 客户端是否正在运行且已登录账号。
// Steam 运行时会在 HKCU\Software\Valve\Steam\ActiveProcess 写入 pid 和 ActiveUser（未登录为 0）；
// Steam 崩溃时 pid 可能残留，所以再确认这个进程还活着并且确实是 steam.exe。
func Running() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam\ActiveProcess`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	pid, _, err := k.GetIntegerValue("pid")
	if err != nil || pid == 0 {
		return false
	}
	if user, _, err := k.GetIntegerValue("ActiveUser"); err != nil || user == 0 {
		return false
	}

	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil || code != stillActive {
		return false
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return false
	}
	return strings.EqualFold(filepath.Base(windows.UTF16ToString(buf[:size])), "steam.exe")
}
