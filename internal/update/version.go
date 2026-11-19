// Package update 实现联机助手的自动更新：查询最新版本（网站接口，失败时直接查 GitHub）、
// 选择下载线路（GitHub 直连或加速站）、下载并校验 sha256、启动安装包静默安装后重启客户端。
package update

import (
	"strconv"
	"strings"
)

// semver 是解析后的版本号，只支持 MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]。
type semver struct {
	nums [3]int
	pre  []string
}

func parseVersion(v string) (semver, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "+")
	var s semver
	core, pre, hasPre := strings.Cut(v, "-")
	if hasPre {
		if pre == "" {
			return s, false
		}
		s.pre = strings.Split(pre, ".")
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return s, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return s, false
		}
		s.nums[i] = n
	}
	return s, true
}

// Newer 判断 latest 是否比 current 新。任一版本号无法解析时返回 false（例如开发构建）。
func Newer(latest, current string) bool {
	a, ok1 := parseVersion(latest)
	b, ok2 := parseVersion(current)
	if !ok1 || !ok2 {
		return false
	}
	return compare(a, b) > 0
}

func compare(a, b semver) int {
	for i := range a.nums {
		if a.nums[i] != b.nums[i] {
			return sign(a.nums[i] - b.nums[i])
		}
	}
	// 正式版比同号的预发布版新
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := comparePre(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	return sign(len(a.pre) - len(b.pre))
}

// comparePre 按 semver 规则比较预发布标识：数字按数值比，数字小于字母。
func comparePre(x, y string) int {
	nx, ex := strconv.Atoi(x)
	ny, ey := strconv.Atoi(y)
	switch {
	case ex == nil && ey == nil:
		return sign(nx - ny)
	case ex == nil:
		return -1
	case ey == nil:
		return 1
	}
	return strings.Compare(x, y)
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}
