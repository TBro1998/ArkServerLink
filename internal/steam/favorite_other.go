//go:build !windows

package steam

import "errors"

// ArkAppID 是 ARK: Survival Evolved 的 Steam App ID。
const ArkAppID = 346110

// FavoriteArg 见 Windows 版。
const FavoriteArg = "--add-ark-favorite"

// AddFavorite 只在 Windows 上实现。
func AddFavorite(gamePort, queryPort int) error { return errors.New("只支持 Windows") }

// RunFavoriteChild 只在 Windows 上实现。
func RunFavoriteChild(args []string) int { return 1 }
