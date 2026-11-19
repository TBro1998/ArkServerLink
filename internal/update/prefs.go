package update

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Prefs 是更新相关的设置，保存在 %AppData%\ArkServerLink\update.json。
type Prefs struct {
	// Mirror 是选择的下载线路（加速站前缀）。空串表示自动测速，"direct" 表示 GitHub 直连。
	Mirror string `json:"mirror"`
}

// MirrorDirect 表示选择 GitHub 直连（加速站前缀为空，和「自动」区分开）。
const MirrorDirect = "direct"

// PrefsPath 返回设置文件路径。
func PrefsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ArkServerLink", "update.json"), nil
}

// LoadPrefs 读取设置，文件不存在或损坏时返回默认值。
func LoadPrefs(path string) Prefs {
	var p Prefs
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &p)
	}
	return p
}

// SavePrefs 保存设置。
func SavePrefs(path string, p Prefs) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
