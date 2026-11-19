//go:build !windows

package steam

import "errors"

// FindARK 只在 Windows 上实现。
func FindARK() (string, error) { return "", errors.New("只支持 Windows") }
