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

// arkRelPath 是带 Steam 前缀的、ARK 客户端自带的 steam_api64.dll 相对路径。
const arkRelPath = `steamapps\common\ARK\Engine\Binaries\ThirdParty\Steamworks\Steamv132\Win64\steam_api64.dll`

// FindSteamAPI 返回本机 ARK 客户端自带的 steam_api64.dll 路径，用来以 ARK 的身份调用 Steam 接口
// （见 AddFavorite）。找不到时说明这台电脑没装 ARK。
func FindSteamAPI() (string, error) {
	dir, err := steamPath()
	if err != nil {
		return "", err
	}
	return findIn(candidateDirs(dir), os.Stat)
}

// LibraryDirs 返回本机所有 Steam 库目录（Steam 本体目录在前）。
func LibraryDirs() ([]string, error) {
	dir, err := steamPath()
	if err != nil {
		return nil, err
	}
	return candidateDirs(dir), nil
}

// candidateDirs 是可能装有 ARK 的 Steam 库目录：Steam 本体目录在前，然后是
// libraryfolders.vdf 里登记的其他库。
func candidateDirs(steamPath string) []string {
	return append([]string{steamPath}, libraryPaths(steamPath)...)
}

// findIn 按顺序在各库目录里找 ARK 客户端的 steam_api64.dll。stat 可注入，便于测试。
func findIn(dirs []string, stat func(string) (os.FileInfo, error)) (string, error) {
	for _, dir := range dirs {
		p := filepath.Join(dir, arkRelPath)
		if fi, err := stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("没有找到 ARK 客户端，请确认这台电脑通过 Steam 装了 ARK")
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
// 读不到时返回空，不影响调用方继续用 Steam 本体目录找。
func libraryPaths(steamPath string) []string {
	data, err := os.ReadFile(filepath.Join(steamPath, "steamapps", "libraryfolders.vdf"))
	if err != nil {
		return nil
	}
	return parseLibraryPaths(string(data), steamPath)
}

// parseLibraryPaths 解析 libraryfolders.vdf 里的 "path" 字段。vdf 里的反斜杠是转义的
// （`"F:\\Steam"`），Steam 本体所在的目录跳过（调用方已经找过）。
func parseLibraryPaths(vdf, skip string) []string {
	var out []string
	for _, m := range libraryPathRe.FindAllStringSubmatch(vdf, -1) {
		p := strings.ReplaceAll(m[1], `\\`, `\`)
		if p == "" || strings.EqualFold(p, skip) {
			continue
		}
		dup := false
		for _, e := range out {
			if strings.EqualFold(e, p) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, p)
		}
	}
	return out
}
