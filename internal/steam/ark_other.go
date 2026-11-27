//go:build !windows

package steam

import "errors"

// FindSteamAPI 只在 Windows 上实现。
func FindSteamAPI() (string, error) { return "", errors.New("只支持 Windows") }

// LibraryDirs 只在 Windows 上实现。
func LibraryDirs() ([]string, error) { return nil, errors.New("只支持 Windows") }
