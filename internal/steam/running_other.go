//go:build !windows

package steam

// Running 只在 Windows 上检测，其他平台一律走系统浏览器。
func Running() bool { return false }

// ArkRunning 只在 Windows 上检测。
func ArkRunning() bool { return false }
