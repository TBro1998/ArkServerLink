# ARKFGF 联机助手（ArkServerLink）

给《方舟：生存进化》（ARK: Survival Evolved）玩家用的 Windows 桌面客户端。服主没有公网 IP 时，玩家用它加入 arkfgf.com 上的「联机房」：

1. 浏览 arkfgf.com 的在线联机房（搜索、地图、人数、PvP/PvE、密码、Mod、标签）
2. 用 Steam 登录（系统浏览器完成登录，回调到本机）
3. 点「加入」：向网站申请入场凭证，在后台启动 EasyTier，通过 P2P 打洞和服务器直连
4. 显示「已直连」后，在游戏控制台输入 `open 127.0.0.1:<端口>` 进服

网站房间页的「用联机助手加入」按钮会唤起 `arkfgf-link://join/<房间 ID>`，客户端已在运行时由单实例转交给已有窗口。

流量只走 P2P 直连，不经任何中继；打洞失败的网络连不上，客户端会明确提示。

**下载**：[arkfgf.com/download](https://arkfgf.com/download)（可选择 GitHub 加速线路）或 [GitHub Releases](https://github.com/TBro1998/ArkServerLink/releases)。安装版会自动检查并安装新版本。

## 架构

本仓库是 [ArkServerCommander](https://github.com/TBro1998/ArkServerCommander) 的子模块（`ArkServerCommander/ArkServerLink`）。三方（网站、服主 Commander、玩家客户端）的接口和 EasyTier 组网参数约定在主仓库（未公开）的 `docs/p2p-link.md`，改动前先读它。

```
main.go                 Wails 应用、单实例、arkfgf-link:// 链接处理、事件注册
authservice.go          AuthService：Status / Login / CancelLogin / Logout（暴露给前端）
roomservice.go          RoomService：List / Get / Join / Leave / Status / Logs / TakePendingJoin
updateservice.go        UpdateService：State / Check / SetMirror / Install / CancelInstall / OpenDownloadPage
internal/arkfgf         网站 /api/v1 客户端（房间列表、详情、加入、/me、联机助手版本），错误带 code、中文说明、Retry-After
internal/auth           Steam 回环登录（127.0.0.1 随机端口 + state 校验）、令牌 DPAPI 加密保存
internal/session        加入流程：申请凭证（503 自动重试）→ 选本机端口 → 启动 easytier → 监视直连状态
internal/easytier       easytier-core 子进程（参数、凭证走环境变量、作业对象保证随客户端退出）
internal/deeplink       arkfgf-link://join/<id> 解析
internal/update         自动更新：版本比较、下载线路测速、下载并校验 sha256、启动安装包
frontend/               React 界面（房间列表、详情、连接状态、日志、更新对话框）
dev/                    本地联调：docker-compose（公共节点 + 服主节点）、udpecho、fakeowner
scripts/fetch-easytier  下载固定版本 EasyTier（v2.6.4，校验 sha256）
scripts/set-version     发布时按 tag 改写各处版本号
.github/workflows       release.yml：推送 v* tag 自动构建并发布 Release
```

要点：

- **接口地址**：默认 `https://arkfgf.com/api/v1`，环境变量 `ARKFGF_API` 覆盖（本地联调用）。请求带 `User-Agent: ArkServerLink/<版本>`
- **登录**：监听 `127.0.0.1:0`，打开 `<API>/auth/steam/login?client_port=<端口>&state=<随机>`，等网站跳回 `/callback?token=&state=`（5 分钟超时，state 不对的请求忽略）。令牌保存在 `%AppData%\ArkServerLink\token`，用 DPAPI 加密（只有当前 Windows 用户能解开）。任何接口返回 401 都视为登录失效并清除令牌
- **端口**：本机选 `L`、`L+1`、`Q` 三个空闲 UDP 端口分别转发到服务器的 `game_port`、`game_port+1`、`query_port`，优先与服务器相同（通常是 `open 127.0.0.1:7777`）
- **状态**：`joining`（申请中，含凭证不足等待重试）→ `connecting`（打洞中）→ `connected`（直连，显示延迟）；40 秒仍未直连为 `not_direct`（服务器节点不是 p2p，或找不到服务器节点；此时会查一次房间是否已离线），仍在后台重试；失败为 `error`
- **同一时间只连一个房间**；加入别的房间会先断开当前的。客户端只允许运行一个实例
- 退出客户端（包括崩溃、被任务管理器结束）时 easytier-core 一定随之退出（Windows 作业对象）

## 开发

需要 Go 1.26、Node.js、[Wails v3 CLI](https://v3.wails.io/)（`wails3`）、Docker Desktop（联调用）。

```sh
wails3 task easytier:fetch   # 下载 EasyTier 到 bin/easytier（构建时也会自动执行）
wails3 dev                   # 开发模式（热重载）；连本地网站：ARKFGF_API=http://127.0.0.1:8080/api/v1 wails3 dev
wails3 task test             # Go 单元测试（go test ./...）
```

`go vet ./...` / `go test ./...` 需要 `frontend/dist` 存在（根包嵌入前端），先执行过一次 `wails3 build` 或 `wails3 dev`。

开发时测试链接唤起：`bin/arkserverlink.exe arkfgf-link://join/1`（再执行一次即测试转交给已运行的实例）。协议只有安装包会注册。

### 联调 EasyTier（不需要网站）

```sh
wails3 task devenv:up        # Docker 里启动公共节点 pub（127.0.0.1:21010）和服主节点 srv（网络 room-dev），打印一个凭证
ET_IT_PEER=tcp://127.0.0.1:21010 ET_IT_NETWORK=room-dev ET_IT_CREDENTIAL=<凭证> \
  go test -run Integration ./internal/easytier ./internal/session
wails3 task devenv:down
```

`internal/easytier` 的集成测试直接启动 easytier-core；`internal/session` 的用假网站走完整加入流程（选端口、等直连、通过转发端口和 srv 里的 udpecho 通信）。

### 端到端联调（网站 + 服主 + 客户端）

`dev/fakeowner` 代替 Commander：向网站登记房间（`server_ip` 10.126.126.1，端口 7777/27015），用房间的 `network_name` 重建 `srv`（compose 通过 `ROOM_NETWORK` 传入），然后每 10 秒心跳一次：按 `want_credentials` 用 srv 的 `easytier-cli credential generate` 签发凭证上传，按 `revoke` 吊销并回报 `revoked_ack`。房间 ID 和令牌保存在 `dev/bin/fakeowner.json`，重启后沿用同一个房间。

三个终端：

```sh
wails3 task e2e:web      # 网站后端（主仓库 web/backend，需按主仓库目录结构检出）跑在 127.0.0.1:8080，LINK_PEERS=tcp://127.0.0.1:21010，LINK_RPC 为空
wails3 task e2e:owner    # 启动 pub，fakeowner 登记房间、启动 srv、心跳；打印 arkfgf-link://join/<id>
wails3 task e2e:client   # 构建并以 ARKFGF_API=http://127.0.0.1:8080/api/v1 运行客户端
```

- 本地网站的 Steam 登录会跳到真实的 Steam 再回到 `http://127.0.0.1:8080`，需要能访问 steamcommunity.com（`STEAM_API_KEY` 不设时名字显示为 Player）
- 踢人测试：`curl -X POST -H "Authorization: Bearer <fakeowner.json 里的 token>" -d '{"steam_id":"<SteamID>","ban":true}' http://127.0.0.1:8080/api/v1/link/rooms/<id>/kick`，fakeowner 下一次心跳会吊销凭证，客户端几秒后失去服务器
- fakeowner 退出（Ctrl+C）时发送 `online:false` 心跳；加 `-delete` 则下架房间。`go run ./dev/fakeowner -h` 查看全部参数
- `devenv:up` 会把 srv 改回 `room-dev` 网络，与 fakeowner 的房间网络互斥

## 构建和打包

```sh
wails3 build                 # 生成绑定、构建前端（tsc + vite），输出 bin/arkserverlink.exe
wails3 package               # NSIS 安装包 bin/arkserverlink-amd64-installer.exe（默认按用户安装，不需要管理员）
```

- 安装包把 `bin/easytier/` 下的 `easytier-core.exe`、`easytier-cli.exe`、`Packet.dll` 装到程序旁的 `easytier\`
- 安装包注册 `arkfgf-link` 协议（来自 `build/config.yml` 的 `protocols`，生成到 `build/windows/nsis/wails_tools.nsh`）。改了 `build/config.yml` 的 `info` 或 `protocols` 后执行 `wails3 task common:update:build-assets` 重新生成（不会覆盖 `project.nsi`）
- `wails3 package` 需要 `makensis`（[NSIS](https://nsis.sourceforge.io/)）在 PATH 里。脚本按 UTF-8 读取（Taskfile 传了 `-INPUTCHARSET UTF8`，否则中文在 GBK / cp1252 系统上会乱码或报错）
- 覆盖安装前安装包会等客户端和 easytier-core 退出：静默安装（`/S`）最多等 30 秒，手动安装时提示玩家先关闭客户端。带 `/RELAUNCH` 时装完重新打开客户端
- 本地构建显示的是源码里的版本号；发布时以 tag 为准（见下文），不用手动改

## 发布

推送 `v*` 附注 tag，GitHub Actions（[release.yml](.github/workflows/release.yml)）在 Windows 上构建、测试并发布 Release：

```sh
# --cleanup=whitespace：默认会把 # 开头的行当注释删掉，Markdown 标题就丢了
git tag -a --cleanup=whitespace v0.2.0 -m "v0.2.0" -m "## 更新内容
- ……"
git push origin v0.2.0
```

- `go run ./scripts/set-version <tag>` 按 tag 改写 `main.go`、`build/config.yml`、`build/windows/info.json`、`wails_tools.nsh` 里的版本号（Windows 版本资源只取数字部分）
- 发布的文件名是客户端和下载页识别的约定（`update.AssetKind`）：`ArkServerLink-<版本>-windows-x64-setup.exe`（安装包）、`ArkServerLink-<版本>-windows-x64-portable.zip`（便携版）、`SHA256SUMS.txt`
- tag 正文会写进 Release 说明，客户端的更新对话框和网站下载页都会显示
- 带 `-` 的 tag（如 `v0.2.0-beta.1`）发布为预发布版：下载页可以切换下载，客户端不会自动更新到它

## 自动更新

- 启动 5 秒后检查一次，之后每 6 小时一次。版本信息来自网站 `GET /api/v1/link/client/releases`（网站缓存 GitHub Release，并给出加速站列表）；网站不可用时直接查 GitHub API
- 有新的正式版时顶栏显示「新版本 vX」；没有时顶栏显示当前版本号，点击可手动检查
- 更新对话框里可以选择下载线路：自动（默认）、GitHub 直连或某个加速站，选择保存在 `%AppData%\ArkServerLink\update.json`
- 自动模式先向每条线路请求 256 KB 测速，按速度依次尝试；某条线路 20 秒没有数据、返回错误或 sha256 不对就换下一条。指定线路时它排第一，其余作为备用
- 下载到 `%TEMP%\ArkServerLink-update`，校验 sha256（Release 文件的 digest）后以 `/S /RELAUNCH /D=<安装目录>` 启动安装包，客户端随即退出，装完自动重新打开
- 只有安装版（程序旁有 `uninstall.exe`）能自动安装；便携版和开发构建只提示去官网下载

## 许可

[Apache License 2.0](LICENSE)
