//go:build windows

package steam

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// arkRelPath 是带 Steam 前缀的 ARK 客户端主程序相对路径。
const arkRelPath = `steamapps\common\ARK\ShooterGame\Binaries\Win64\ShooterGame.exe`

// FindARK 返回本机 ARK 客户端的 ShooterGame.exe 路径。
//
// 直接启动这个程序（带 -ExecCmds）不经过 Steam，游戏启动后立即执行游戏内控制台的
// open 命令。已实测可行：客户端会直接向 127.0.0.1:<端口> 发游戏报文。
// 反过来 +connect / steam://run 走的是 Steam 的查询流程，对 127.0.0.1 会报
// "Unable to query server info for invite"，一个包都不会发。
func FindARK() (string, error) {
	steamPath, err := steamPath()
	if err != nil {
		return "", err
	}
	libs := []string{steamPath}
	libs = append(libs, libraryPaths(steamPath)...)
	for _, dir := range libs {
		p := filepath.Join(dir, arkRelPath)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("没有找到 ARK 客户端（ShooterGame.exe），请确认这台电脑装了 ARK")
}

// steamPath 读取 Steam 安装目录。用户级的 ActiveProcess 里带着 steamclient.dll 的路径，
// 没登录过时退回 32 位视图下的 InstallPath。
func steamPath() (string, error) {
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam\ActiveProcess`, registry.QUERY_VALUE); err == nil {
		defer k.Close()
		if dll, _, err := k.GetStringValue("SteamClientDll"); err == nil && dll != "" {
			return filepath.Dir(dll), nil
		}
	}
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(root, `Software\Valve\Steam`, registry.QUERY_VALUE|registry.WOW64_32KEY)
		if err != nil {
			continue
		}
		p, _, err := k.GetStringValue("InstallPath")
		k.Close()
		if err == nil && p != "" {
			return p, nil
		}
	}
	return "", errors.New("没有找到 Steam 安装目录")
}

var libraryPathRe = regexp.MustCompile(`"path"\s+"([^"]+)"`)

// libraryPaths 从 libraryfolders.vdf 里读出其他 Steam 库目录（游戏不一定装在 Steam 本体目录里）。
func libraryPaths(steamPath string) []string {
	data, err := os.ReadFile(filepath.Join(steamPath, "steamapps", "libraryfolders.vdf"))
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range libraryPathRe.FindAllStringSubmatch(string(data), -1) {
		p := strings.ReplaceAll(m[1], `\`, `\`)
		if p != steamPath {
			out = append(out, p)
		}
	}
	return out
}
