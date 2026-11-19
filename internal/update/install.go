package update

import (
	"os"
	"path/filepath"
)

// InstallDir 返回客户端的安装目录。只有通过安装包安装（目录里有 uninstall.exe）时才能自动更新；
// 便携版和开发构建返回 false，改为引导玩家去官网下载。
func InstallDir() (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	dir := filepath.Dir(exe)
	if _, err := os.Stat(filepath.Join(dir, "uninstall.exe")); err != nil {
		return "", false
	}
	return dir, true
}

// CacheDir 是下载安装包的临时目录。
func CacheDir() string {
	return filepath.Join(os.TempDir(), "ArkServerLink-update")
}
