package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"

	"arkfgf.com/link/internal/clicker"
	"arkfgf.com/link/internal/graphics"
	"arkfgf.com/link/internal/steam"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ToolsPrefs 是游戏工具的设置，保存在 %AppData%\ArkServerLink\tools.json。
type ToolsPrefs struct {
	Clicker    clicker.Config `json:"clicker"`
	ASAOptions []string       `json:"asa_options"` // 上次在 ASA 画质里选的选项
	Gamma      float64        `json:"gamma"`       // 0 表示不调
}

// ASEGraphicsStatus 是 ASE 画质文件的状态。
type ASEGraphicsStatus struct {
	Found       bool            `json:"found"`
	Path        string          `json:"path"`
	Error       string          `json:"error"`    // 找不到或读不了文件的原因
	Applied     []string        `json:"applied"`  // 文件里已生效的选项
	Outdated    bool            `json:"outdated"` // 是旧版本写入的（选项改过参数或改了名），需要重新应用
	Backup      graphics.Backup `json:"backup"`
	GameRunning bool            `json:"game_running"` // 游戏开着时改动要重启游戏才生效
}

// ToolsService 暴露给前端：画质设置和连点器。
type ToolsService struct {
	prefsPath string

	mu      sync.Mutex
	prefs   ToolsPrefs
	clicker *clicker.Clicker
}

func newToolsService() *ToolsService {
	s := &ToolsService{prefs: ToolsPrefs{Clicker: clicker.DefaultConfig()}}
	if dir, err := os.UserConfigDir(); err == nil {
		s.prefsPath = filepath.Join(dir, "ArkServerLink", "tools.json")
		if b, err := os.ReadFile(s.prefsPath); err == nil {
			_ = json.Unmarshal(b, &s.prefs)
		}
	}
	return s
}

func (s *ToolsService) ServiceName() string { return "ToolsService" }

func (s *ToolsService) ServiceStartup(context.Context, application.ServiceOptions) error {
	s.mu.Lock()
	cfg := s.prefs.Clicker
	s.mu.Unlock()
	c := clicker.New(cfg, func(st clicker.State) {
		if app := application.Get(); app != nil {
			app.Event.Emit(EventClickerChanged, st)
		}
	})
	s.mu.Lock()
	s.clicker = c
	s.mu.Unlock()
	return nil
}

func (s *ToolsService) ServiceShutdown() error {
	s.mu.Lock()
	c := s.clicker
	s.mu.Unlock()
	if c != nil {
		c.Close()
	}
	return nil
}

func (s *ToolsService) savePrefs() {
	if s.prefsPath == "" {
		return
	}
	s.mu.Lock()
	b, err := json.MarshalIndent(s.prefs, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.prefsPath), 0o700); err == nil {
		err = os.WriteFile(s.prefsPath, b, 0o600)
	}
	if err != nil {
		log.Printf("保存游戏工具设置失败: %v", err)
	}
}

// Prefs 返回保存的设置。
func (s *ToolsService) Prefs() ToolsPrefs {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clicker != nil {
		s.prefs.Clicker = s.clicker.Config()
	}
	return s.prefs
}

// ---- 画质 ----

// GraphicsCatalog 返回游戏版本（"ase" / "asa"）的画质选项和档位。
func (s *ToolsService) GraphicsCatalog(game string) graphics.Catalog {
	return graphics.For(graphics.Game(game))
}

func aseConfigPath() (string, error) {
	dirs, err := steam.LibraryDirs()
	if err != nil {
		return "", err
	}
	return graphics.FindASEConfig(dirs, os.Stat)
}

// ASEStatus 找到 ASE 的 BaseDeviceProfiles.ini 并读出已生效的选项。
func (s *ToolsService) ASEStatus() ASEGraphicsStatus {
	st := ASEGraphicsStatus{GameRunning: steam.ArkRunning()}
	p, err := aseConfigPath()
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.Found, st.Path, st.Backup = true, p, graphics.BackupOf(p)
	if st.Applied, st.Outdated, err = graphics.ReadApplied(p, graphics.For(graphics.ASE)); err != nil {
		st.Error = "读取 BaseDeviceProfiles.ini 失败：" + err.Error()
	}
	return st
}

// ASEApply 把所选选项写进 BaseDeviceProfiles.ini（生效前先备份原文件），重启游戏后生效。
func (s *ToolsService) ASEApply(ids []string) (ASEGraphicsStatus, error) {
	p, err := aseConfigPath()
	if err != nil {
		return s.ASEStatus(), err
	}
	if len(ids) == 0 {
		return s.ASEStatus(), errors.New("请至少选择一项；要去掉所有修改请点「还原」")
	}
	if err := graphics.ApplyFile(p, graphics.For(graphics.ASE), ids); err != nil {
		return s.ASEStatus(), err
	}
	return s.ASEStatus(), nil
}

// ASERestore 去掉助手写入的设置，文件里其他内容不动。
func (s *ToolsService) ASERestore() (ASEGraphicsStatus, error) {
	p, err := aseConfigPath()
	if err != nil {
		return s.ASEStatus(), err
	}
	if err := graphics.RestoreFile(p); err != nil {
		return s.ASEStatus(), err
	}
	return s.ASEStatus(), nil
}

// ASERestoreBackup 用生效前的备份整份覆盖 BaseDeviceProfiles.ini。
func (s *ToolsService) ASERestoreBackup() (ASEGraphicsStatus, error) {
	p, err := aseConfigPath()
	if err != nil {
		return s.ASEStatus(), err
	}
	if err := graphics.RestoreBackupFile(p); err != nil {
		return s.ASEStatus(), err
	}
	return s.ASEStatus(), nil
}

// ASACommand 生成 ASA 的控制台命令，并记住这次的选择。gamma 为 0 表示不调。
func (s *ToolsService) ASACommand(ids []string, gamma float64) string {
	s.mu.Lock()
	s.prefs.ASAOptions, s.prefs.Gamma = ids, gamma
	s.mu.Unlock()
	s.savePrefs()
	return graphics.Command(graphics.For(graphics.ASA), ids, gamma)
}

// GammaCommand 返回调整伽马的控制台命令（ASE 在游戏控制台里输入）。
func (s *ToolsService) GammaCommand(gamma float64) string {
	s.mu.Lock()
	s.prefs.Gamma = gamma
	s.mu.Unlock()
	s.savePrefs()
	return graphics.GammaCommand(gamma)
}

// ---- 连点器 ----

// ClickerHotkeys 返回可以选的开关热键。
func (s *ToolsService) ClickerHotkeys() []string { return clicker.Hotkeys }

// ClickerState 返回连点器状态。
func (s *ToolsService) ClickerState() clicker.State {
	if c := s.getClicker(); c != nil {
		return c.State()
	}
	return clicker.State{}
}

// SetClickerConfig 保存连点器设置（正在运行时会先停下）。
func (s *ToolsService) SetClickerConfig(cfg clicker.Config) (clicker.Config, error) {
	c := s.getClicker()
	if c == nil {
		return cfg, errors.New("连点器还没有启动")
	}
	cfg, err := c.SetConfig(cfg)
	if err != nil {
		return cfg, err
	}
	s.mu.Lock()
	s.prefs.Clicker = cfg
	s.mu.Unlock()
	s.savePrefs()
	return cfg, nil
}

// ClickerToggle 打开或关闭连点器（和按热键一样）。
func (s *ToolsService) ClickerToggle() clicker.State {
	c := s.getClicker()
	if c == nil {
		return clicker.State{}
	}
	c.Toggle()
	return c.State()
}

func (s *ToolsService) getClicker() *clicker.Clicker {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clicker
}
