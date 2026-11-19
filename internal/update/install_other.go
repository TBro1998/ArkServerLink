//go:build !windows

package update

import "errors"

// RunInstaller 目前只支持 Windows 安装包。
func RunInstaller(installer, dir string) error {
	return errors.New("当前系统不支持自动更新，请到官网下载")
}
