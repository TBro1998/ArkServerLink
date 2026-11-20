// Package steam 检测本机 Steam 客户端，用来在 Steam 自带的浏览器里完成登录。
package steam

// OpenURL 返回在 Steam 客户端里打开 target 的地址。
// 注意：Steam 只把 steamcommunity.com / store.steampowered.com 等自家页面留在客户端里打开，
// 其他域名会转交系统浏览器，所以 target 应当直接是 Steam 的页面（例如 OpenID 登录页）。
func OpenURL(target string) string { return "steam://openurl/" + target }

// ConnectURL 返回让 Steam 连接游戏服务器的地址，addr 是服务器的查询地址（ip:查询端口）。
// 和在 Steam 收藏里添加服务器再连接是同一个流程：Steam 先查询 A2S_INFO，再按回复里的
// 游戏端口启动游戏进服（游戏已经开着时交给正在运行的游戏）。给游戏端口会查询失败，
// 报 "Unable to query server info for invite"。
func ConnectURL(addr string) string { return "steam://connect/" + addr }
