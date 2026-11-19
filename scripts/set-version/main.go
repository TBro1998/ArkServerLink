// set-version 把版本号写进所有需要它的文件，发布时由 GitHub Actions 按 tag 调用。
//
// 用法（在项目根目录）: go run ./scripts/set-version v0.2.0
//
// 改动的位置：main.go 的 version（User-Agent、检查更新）、build/config.yml 的 info.version、
// build/windows/info.json（exe 的版本资源）、build/windows/nsis/wails_tools.nsh（安装包和「应用和功能」里的版本）。
// 任何一处没匹配上都会报错，避免发出版本号不一致的安装包。
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var semver = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)

type edit struct {
	file    string
	pattern *regexp.Regexp // 第 1 个分组之后、第 2 个分组之前是版本号
	count   int            // 期望匹配的次数
}

var edits = []edit{
	{"main.go", regexp.MustCompile(`(const version = ")[^"]*(")`), 1},
	{"build/config.yml", regexp.MustCompile(`(?m)(^  version: ")[^"]*(")`), 1},
	{"build/windows/info.json", regexp.MustCompile(`("(?:file_version|product_version|FileVersion|ProductVersion)": ")[^"]*(")`), 4},
	{"build/windows/nsis/wails_tools.nsh", regexp.MustCompile(`(!define INFO_PRODUCTVERSION ")[^"]*(")`), 1},
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "用法: go run ./scripts/set-version <版本，例如 v0.2.0>")
		os.Exit(2)
	}
	v := strings.TrimPrefix(os.Args[1], "v")
	if !semver.MatchString(v) {
		fmt.Fprintf(os.Stderr, "set-version: %q 不是 MAJOR.MINOR.PATCH[-预发布] 格式\n", os.Args[1])
		os.Exit(2)
	}
	// Windows 版本资源和 NSIS 的 VIProductVersion 只接受数字，预发布后缀只留在 main.go 和 config.yml
	numeric, _, _ := strings.Cut(v, "-")
	for _, e := range edits {
		val := v
		if strings.HasPrefix(e.file, "build/windows/") {
			val = numeric
		}
		if err := apply(e, val); err != nil {
			fmt.Fprintln(os.Stderr, "set-version:", err)
			os.Exit(1)
		}
		fmt.Printf("%s → %s\n", e.file, val)
	}
}

func apply(e edit, val string) error {
	b, err := os.ReadFile(e.file)
	if err != nil {
		return err
	}
	n := len(e.pattern.FindAllIndex(b, -1))
	if n != e.count {
		return fmt.Errorf("%s: 版本号位置匹配到 %d 处，期望 %d 处", e.file, n, e.count)
	}
	out := e.pattern.ReplaceAll(b, []byte("${1}"+val+"${2}"))
	return os.WriteFile(e.file, out, 0o644)
}
