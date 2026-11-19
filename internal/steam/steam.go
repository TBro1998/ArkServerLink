// Package steam 检测本机 Steam 客户端，用来在 Steam 自带的浏览器里完成登录。
package steam

import "net/url"

// OpenURL 返回在 Steam 客户端里打开 target 的地址。
// 注意：Steam 只把 steamcommunity.com / store.steampowered.com 等自家页面留在客户端里打开，
// 其他域名会转交系统浏览器，所以 target 应当直接是 Steam 的页面（例如 OpenID 登录页）。
func OpenURL(target string) string { return "steam://openurl/" + target }

// ArkAppID 是 ARK: Survival Evolved 在 Steam 上的 App ID。
const ArkAppID = "346110"

// ConnectURL 是让 Steam 启动 ARK 并连接指定地址的地址，地址要用游戏端口。
//
// 不用 steam://connect：2023 年 9 月之后 Steam 的加入流程对 ARK 会报
// "app id specified by the server is invalid"。steam://run/<appid>//<参数>
// 直接把参数交给游戏进程，效果和启动时带 +connect 一样，等同于游戏内控制台
// 的 open 命令。注意 ARK 已经在运行时 Steam 会忽略参数，游戏停在主菜单。
func ConnectURL(addr string) string {
	return "steam://run/" + ArkAppID + "//+connect%20" + url.PathEscape(addr)
}
