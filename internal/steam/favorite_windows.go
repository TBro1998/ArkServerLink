//go:build windows

package steam

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ArkAppID 是 ARK: Survival Evolved 的 Steam App ID。
const ArkAppID = 346110

// FavoriteArg 是让助手作为一次性子进程添加 Steam 收藏的命令行参数（见 RunFavoriteChild）。
const FavoriteArg = "--add-ark-favorite"

const createNoWindow = 0x08000000

// AddFavorite 把 127.0.0.1:<queryPort> 加进 ARK 的 Steam 收藏，游戏的服务器列表「收藏」里就能看到它。
//
// 玩家从游戏内的收藏列表加入是实测唯一能稳定进服的方式：Steam 客户端自己的连接
// （steam://connect、+connect、Steam 收藏里点连接）对 ARK 服务器都不可用，
// 控制台 open 127.0.0.1:<端口> 会被服务器以 "Invalid authentication token" 拒绝。
//
// 调用 Steam 接口的进程在 Steam 看来就是正在运行的 ARK，所以在一次性的子进程里调用，结束后才启动游戏。
func AddFavorite(gamePort, queryPort int) error {
	dll, err := FindSteamAPI()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, FavoriteArg, dll, strconv.Itoa(gamePort), strconv.Itoa(queryPort))
	cmd.Env = append(os.Environ(), "SteamAppId="+strconv.Itoa(ArkAppID), "SteamGameId="+strconv.Itoa(ArkAppID))
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(lastLine(stderr.String())); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}

// lastLine 返回 s 的最后一个非空行（steam_api 会往标准输出、错误输出写调试信息）。
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

// RunFavoriteChild 是子进程的入口：args 为 dll 路径、游戏端口、查询端口。返回进程退出码。
func RunFavoriteChild(args []string) int {
	if err := runFavorite(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func runFavorite(args []string) error {
	if len(args) != 3 {
		return errors.New("参数错误")
	}
	game, err1 := strconv.Atoi(args[1])
	query, err2 := strconv.Atoi(args[2])
	if err1 != nil || err2 != nil || game <= 0 || game > 65535 || query <= 0 || query > 65535 {
		return errors.New("端口无效")
	}
	dll, err := syscall.LoadDLL(args[0])
	if err != nil {
		return fmt.Errorf("无法加载 ARK 的 steam_api64.dll: %w", err)
	}
	proc := func(name string) (*syscall.Proc, error) { return dll.FindProc(name) }
	initp, err := proc("SteamAPI_Init")
	if err != nil {
		return err
	}
	shutdown, err := proc("SteamAPI_Shutdown")
	if err != nil {
		return err
	}
	mmp, err := proc("SteamMatchmaking")
	if err != nil {
		return err
	}
	add, err := proc("SteamAPI_ISteamMatchmaking_AddFavoriteGame")
	if err != nil {
		return err
	}
	if r, _, _ := initp.Call(); r&0xff == 0 {
		return errors.New("连接不上 Steam，请先打开并登录 Steam 客户端")
	}
	defer shutdown.Call()
	mm, _, _ := mmp.Call()
	if mm == 0 {
		return errors.New("Steam 接口不可用")
	}
	// AddFavoriteGame(this, appID, ip(主机字节序), connPort, queryPort, flags, lastPlayed)
	const loopback = 0x7F000001
	const flagFavorite = 1
	add.Call(mm, ArkAppID, loopback, uintptr(game), uintptr(query), flagFavorite, uintptr(time.Now().Unix()))
	return nil
}
