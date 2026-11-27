package graphics

import (
	"strconv"
	"strings"
)

// 控制台命令：玩家在游戏里按 Tab 打开控制台粘贴执行，重启游戏后恢复默认。ASA 只有这种方式，
// ASE 也可以用（每个选项的 Console，只读参数只能写进 ini）。
// 多条命令用 " | " 连成一行，控制台会依次执行。

// GammaMin、GammaMax 是伽马的可选范围，游戏默认 2.2。
const (
	GammaMin = 1.0
	GammaMax = 5.0
)

// Command 返回所选选项的控制台命令，没有控制台写法的选项跳过。gamma 不在 [GammaMin, GammaMax] 里时不调伽马。
func Command(cat Catalog, ids []string, gamma float64) string {
	var parts []string
	for _, o := range cat.Ordered(ids) {
		parts = append(parts, cat.ConsoleLines(o)...)
	}
	if g := GammaCommand(gamma); g != "" {
		parts = append(parts, g)
	}
	return strings.Join(parts, " | ")
}

// GammaCommand 返回调整伽马的控制台命令（ASE、ASA 通用），超出范围时为空。
func GammaCommand(gamma float64) string {
	if gamma < GammaMin || gamma > GammaMax {
		return ""
	}
	return "gamma " + strconv.FormatFloat(gamma, 'f', -1, 64)
}
