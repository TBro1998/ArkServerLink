//go:build !windows

package steam

import (
	"errors"
	"os/exec"
)

// FindARK 只在 Windows 上实现。
func FindARK() (string, error) { return "", errors.New("只支持 Windows") }

// ARKCommand 只在 Windows 上实现，其他平台返回的命令启动时会失败。
func ARKCommand(exe string, port int) *exec.Cmd { return exec.Command(exe) }
