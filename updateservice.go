package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"arkfgf.com/link/internal/arkfgf"
	"arkfgf.com/link/internal/update"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// 更新状态。
const (
	UpdateIdle        = "idle"        // 还没检查过
	UpdateChecking    = "checking"    // 正在检查
	UpdateLatest      = "latest"      // 已是最新
	UpdateAvailable   = "available"   // 有新版本
	UpdateDownloading = "downloading" // 正在测速 / 下载
	UpdateInstalling  = "installing"  // 已启动安装包，客户端即将退出
	UpdateError       = "error"       // 检查或下载失败，见 Error
)

// UpdateState 是自动更新的状态，变化时通过 update:changed 事件推给前端。
type UpdateState struct {
	Current   string                `json:"current"`
	Phase     string                `json:"phase"`
	Latest    *arkfgf.ClientRelease `json:"latest"`
	Available bool                  `json:"available"` // Latest 比当前版本新
	CanAuto   bool                  `json:"can_auto"`  // 安装版可以自动安装；便携版和开发构建只能去官网下载
	Mirrors   []arkfgf.Mirror       `json:"mirrors"`
	Mirror    string                `json:"mirror"` // 选择的线路，见 update.Prefs
	Source    string                `json:"source"` // 正在使用的线路名称
	Done      int64                 `json:"done"`
	Total     int64                 `json:"total"`
	Error     string                `json:"error"`
	CheckedAt time.Time             `json:"checked_at"`
	// DownloadPage 是官网下载页
	DownloadPage string `json:"download_page"`
}

// UpdateService 暴露给前端：检查更新、选择下载线路、下载并安装。
type UpdateService struct {
	api       *arkfgf.Client
	version   string
	prefsPath string
	http      *http.Client

	mu     sync.Mutex
	state  UpdateState
	cancel context.CancelFunc // 正在下载时有值
}

const (
	updateFirstCheck = 5 * time.Second
	updateInterval   = 6 * time.Hour
)

func newUpdateService(api *arkfgf.Client, version string) *UpdateService {
	s := &UpdateService{
		api:     api,
		version: version,
		// 下载不设总超时（大文件 + 慢线路），停滞由 update.Download 检测
		http: &http.Client{},
	}
	_, canAuto := update.InstallDir()
	s.state = UpdateState{
		Current:      version,
		Phase:        UpdateIdle,
		CanAuto:      canAuto,
		Mirrors:      update.DefaultMirrors,
		DownloadPage: siteURL(api.BaseURL()) + "/download",
	}
	if p, err := update.PrefsPath(); err == nil {
		s.prefsPath = p
		s.state.Mirror = update.LoadPrefs(p).Mirror
	}
	return s
}

// siteURL 由接口地址推出网站地址：https://arkfgf.com/api/v1 → https://arkfgf.com
func siteURL(api string) string {
	return strings.TrimSuffix(strings.TrimRight(api, "/"), "/api/v1")
}

func (s *UpdateService) ServiceName() string { return "UpdateService" }

func (s *UpdateService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	go func() {
		t := time.NewTimer(updateFirstCheck)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			s.check(ctx, false)
			t.Reset(updateInterval)
		}
	}()
	return nil
}

func (s *UpdateService) ServiceShutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// State 返回当前状态。
func (s *UpdateService) State() UpdateState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Check 立即检查更新（玩家手动点「检查更新」）。
func (s *UpdateService) Check(ctx context.Context) UpdateState {
	s.check(ctx, true)
	return s.State()
}

func (s *UpdateService) check(ctx context.Context, manual bool) {
	s.mu.Lock()
	if s.state.Phase == UpdateDownloading || s.state.Phase == UpdateInstalling || s.state.Phase == UpdateChecking {
		s.mu.Unlock()
		return
	}
	prev := s.state.Phase
	s.state.Phase = UpdateChecking
	s.state.Error = ""
	s.mu.Unlock()
	s.emit()

	rel, err := s.fetch(ctx)

	s.mu.Lock()
	s.state.CheckedAt = time.Now()
	switch {
	case err != nil && !manual:
		// 自动检查失败不打扰玩家，保持之前的状态
		s.state.Phase = prev
	case err != nil:
		s.state.Phase = UpdateError
		s.state.Error = "检查更新失败: " + err.Error()
	default:
		if len(rel.Mirrors) > 0 {
			s.state.Mirrors = rel.Mirrors
		}
		s.state.Latest = rel.Stable
		s.state.Available = rel.Stable != nil && update.Newer(rel.Stable.Version, s.version)
		if s.state.Available {
			s.state.Phase = UpdateAvailable
		} else {
			s.state.Phase = UpdateLatest
		}
	}
	s.mu.Unlock()
	s.emit()
}

// fetch 先查网站接口（有缓存、国内可访问），失败时直接查 GitHub。
func (s *UpdateService) fetch(ctx context.Context) (*arkfgf.ClientReleases, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	rel, err := s.api.ClientReleases(ctx)
	if err == nil {
		return rel, nil
	}
	if ctx.Err() != nil {
		return nil, err
	}
	gh, ghErr := update.FetchGitHub(ctx, s.http, "ArkServerLink/"+s.version)
	if ghErr != nil {
		return nil, err // 报网站的错误，更好理解
	}
	return gh, nil
}

// SetMirror 选择下载线路：空串为自动，"direct" 为 GitHub 直连，其余为加速站前缀。
func (s *UpdateService) SetMirror(mirror string) error {
	s.mu.Lock()
	s.state.Mirror = mirror
	path := s.prefsPath
	s.mu.Unlock()
	s.emit()
	if path == "" {
		return nil
	}
	return update.SavePrefs(path, update.Prefs{Mirror: mirror})
}

// Install 下载新版安装包、校验后静默安装，安装包装完会重新打开客户端。
// 进度通过 update:changed 事件推送；会断开当前房间。
func (s *UpdateService) Install() error {
	dir, ok := update.InstallDir()
	if !ok {
		return errors.New("当前不是安装版，请到官网下载新版本")
	}
	s.mu.Lock()
	if s.state.Phase == UpdateDownloading || s.state.Phase == UpdateInstalling {
		s.mu.Unlock()
		return nil
	}
	rel := s.state.Latest
	asset := rel.Asset(arkfgf.AssetInstaller)
	if !s.state.Available || asset == nil {
		s.mu.Unlock()
		return errors.New("没有可安装的新版本")
	}
	sources := update.Sources(asset.URL, s.state.Mirrors, s.state.Mirror)
	auto := s.state.Mirror == ""
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.state.Phase = UpdateDownloading
	s.state.Error = ""
	s.state.Source = ""
	s.state.Done, s.state.Total = 0, asset.Size
	s.mu.Unlock()
	s.emit()

	go func() {
		defer cancel()
		if auto {
			s.setSource("正在测速选择线路…")
			sources = update.Rank(ctx, s.http, sources, 8*time.Second)
		}
		path, err := update.Download(ctx, s.http, *asset, sources, update.CacheDir(), func(p update.Progress) {
			s.mu.Lock()
			s.state.Source, s.state.Done, s.state.Total = p.Source, p.Done, p.Total
			s.mu.Unlock()
			s.emit()
		})
		if err == nil {
			s.mu.Lock()
			s.state.Phase = UpdateInstalling
			s.mu.Unlock()
			s.emit()
			if err = update.RunInstaller(path, dir); err == nil {
				// 给前端一点时间显示「正在安装」，然后退出（顺带断开房间、结束 easytier-core），
				// 安装包等文件解除占用后再覆盖
				time.Sleep(800 * time.Millisecond)
				if app := application.Get(); app != nil {
					app.Quit()
				}
				return
			}
		}
		s.mu.Lock()
		s.cancel = nil
		if errors.Is(err, context.Canceled) {
			s.state.Phase = UpdateAvailable
		} else {
			s.state.Phase = UpdateError
			s.state.Error = "更新失败: " + err.Error()
		}
		s.mu.Unlock()
		s.emit()
	}()
	return nil
}

// CancelInstall 取消正在进行的下载。
func (s *UpdateService) CancelInstall() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil && s.state.Phase == UpdateDownloading {
		s.cancel()
	}
}

// OpenDownloadPage 用浏览器打开官网下载页。
func (s *UpdateService) OpenDownloadPage() error {
	return application.Get().Browser.OpenURL(s.State().DownloadPage)
}

func (s *UpdateService) setSource(name string) {
	s.mu.Lock()
	s.state.Source = name
	s.mu.Unlock()
	s.emit()
}

func (s *UpdateService) emit() {
	if app := application.Get(); app != nil {
		app.Event.Emit(EventUpdateChanged, s.State())
	}
}
