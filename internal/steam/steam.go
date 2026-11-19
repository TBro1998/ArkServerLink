// Package steam 检测本机 Steam 客户端，用来在 Steam 自带的浏览器里完成登录。
package steam

// OpenURL 返回在 Steam 客户端里打开 target 的地址。
// 注意：Steam 只把 steamcommunity.com / store.steampowered.com 等自家页面留在客户端里打开，
// 其他域名会转交系统浏览器，所以 target 应当直接是 Steam 的页面（例如 OpenID 登录页）。
func OpenURL(target string) string { return "steam://openurl/" + target }
