package main

import (
	"embed"
	"log"
	"os"

	"arkfgf.com/link/internal/arkfgf"
	"arkfgf.com/link/internal/auth"
	"arkfgf.com/link/internal/deeplink"
	"arkfgf.com/link/internal/steam"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

// version 与 build/config.yml 的 info.version 保持一致，出现在 User-Agent 里，也用于检查更新。
// 发布时由 scripts/set-version 按 tag 改写（见 .github/workflows/release.yml）。
const version = "0.1.0"

// 发给前端的事件。
const (
	EventAuthChanged   = "auth:changed"   // 登录状态变化（登录、退出、令牌失效）
	EventDeepLinkJoin  = "deeplink:join"  // 通过 arkfgf-link://join/<id> 请求加入房间
	EventUpdateChanged = "update:changed" // 自动更新状态变化（检查结果、下载进度）
)

func init() {
	application.RegisterEvent[AuthState](EventAuthChanged)
	application.RegisterEvent[int64](EventDeepLinkJoin)
	application.RegisterEvent[UpdateState](EventUpdateChanged)
}

var mainWindow *application.WebviewWindow

func focusMainWindow() {
	if mainWindow == nil {
		return
	}
	mainWindow.Show()
	mainWindow.Restore()
	mainWindow.Focus()
}

func main() {
	// 一次性子进程：以 ARK 的身份把服务器加进 Steam 收藏（见 steam.AddFavorite）
	if len(os.Args) > 1 && os.Args[1] == steam.FavoriteArg {
		os.Exit(steam.RunFavoriteChild(os.Args[2:]))
	}
	accountAPI, linkAPI := arkfgf.BaseURLsFromEnv()
	api := arkfgf.NewSplit(accountAPI, linkAPI, version)
	authSvc := &AuthService{api: api}
	if store, err := auth.DefaultStore(); err == nil {
		authSvc.store = store
	} else {
		log.Printf("无法定位配置目录，登录状态不会保存: %v", err)
		authSvc.store = auth.NewStore(os.DevNull)
	}
	roomSvc := &RoomService{api: api, auth: authSvc}
	updateSvc := newUpdateService(api, version)

	// 首次启动就是被链接唤起的（Windows 上链接是第一个参数）
	launchID, _ := deeplink.FindJoin(os.Args[1:])
	roomSvc.pendingJoin = launchID

	app := application.New(application.Options{
		Name:        "ARKFGF 联机助手",
		Description: "ARK 无公网联机客户端",
		Services: []application.Service{
			application.NewService(authSvc),
			application.NewService(roomSvc),
			application.NewService(updateSvc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		// 同时只允许一个客户端：两个实例会抢同一个本地转发端口。
		// 已经在运行时再点网站上的「用联机助手加入」，链接通过这里转给已有窗口。
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.arkfgf.link",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if id, ok := deeplink.FindJoin(data.Args); ok {
					roomSvc.requestJoin(id)
				}
				focusMainWindow()
			},
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// macOS / Linux 通过系统事件传链接。Windows 也会发这个事件，但链接已在上面从参数里取过
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(e *application.ApplicationEvent) {
		id, ok := deeplink.ParseJoin(e.Context().URL())
		if ok && id == launchID {
			launchID = 0
			return
		}
		if ok {
			roomSvc.requestJoin(id)
		}
	})

	mainWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "ARKFGF 联机助手",
		Width:            1040,
		Height:           680,
		MinWidth:         860,
		MinHeight:        560,
		BackgroundColour: application.NewRGB(3, 7, 18),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
