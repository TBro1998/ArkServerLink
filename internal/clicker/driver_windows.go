//go:build windows

package clicker

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procSendInput                = user32.NewProc("SendInput")
	procMapVirtualKeyW           = user32.NewProc("MapVirtualKeyW")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procRegisterHotKey           = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey         = user32.NewProc("UnregisterHotKey")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procPostThreadMessageW       = user32.NewProc("PostThreadMessageW")
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseLeftDown  = 0x0002
	mouseLeftUp    = 0x0004
	mouseRightDown = 0x0008
	mouseRightUp   = 0x0010

	keyScanCode = 0x0008
	keyUp       = 0x0002

	modNoRepeat = 0x4000
	wmHotkey    = 0x0312
	wmQuit      = 0x0012
)

// gameExes 是 ASE 和 ASA 客户端的进程名。
var gameExes = []string{"ShooterGame.exe", "ArkAscended.exe"}

// input 对应 Win32 的 INPUT（64 位下 40 字节：类型 4 字节、对齐 4 字节、联合体 32 字节）。
type input struct {
	typ   uint32
	_     uint32
	union [32]byte
}

type mouseInput struct {
	dx, dy    int32
	mouseData uint32
	flags     uint32
	time      uint32
	extraInfo uintptr
}

type keybdInput struct {
	vk, scan  uint16
	flags     uint32
	time      uint32
	extraInfo uintptr
}

type winDriver struct{}

func newDriver() driver { return winDriver{} }

func (winDriver) press(action string, vk uint16, down bool) error {
	var in input
	switch action {
	case ActionLeft, ActionRight:
		in.typ = inputMouse
		var f uint32
		switch {
		case action == ActionLeft && down:
			f = mouseLeftDown
		case action == ActionLeft:
			f = mouseLeftUp
		case down:
			f = mouseRightDown
		default:
			f = mouseRightUp
		}
		*(*mouseInput)(unsafe.Pointer(&in.union[0])) = mouseInput{flags: f}
	case ActionKey:
		// 游戏读的是扫描码，只发虚拟键码有的游戏收不到
		scan, _, _ := procMapVirtualKeyW.Call(uintptr(vk), 0) // MAPVK_VK_TO_VSC
		f := uint32(keyScanCode)
		if !down {
			f |= keyUp
		}
		in.typ = inputKeyboard
		*(*keybdInput)(unsafe.Pointer(&in.union[0])) = keybdInput{scan: uint16(scan), flags: f}
	default:
		return errors.New("未知的点击对象")
	}
	n, _, err := procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
	if n != 1 {
		return err
	}
	return nil
}

func (winDriver) gameFocused() bool {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return false
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return false
	}
	name := filepath.Base(windows.UTF16ToString(buf[:size]))
	for _, exe := range gameExes {
		if strings.EqualFold(name, exe) {
			return true
		}
	}
	return false
}

// registerHotkey 在一个专用的系统线程上注册热键并跑消息循环（WM_HOTKEY 只发给注册它的线程）。
func (winDriver) registerHotkey(vk uint16, fn func()) (func(), error) {
	type result struct {
		tid uint32
		err error
	}
	ready := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		tid := windows.GetCurrentThreadId()
		const id = 1
		if ok, _, err := procRegisterHotKey.Call(0, id, modNoRepeat, uintptr(vk)); ok == 0 {
			ready <- result{err: err}
			return
		}
		defer procUnregisterHotKey.Call(0, id)
		ready <- result{tid: tid}
		var msg struct {
			hwnd    uintptr
			message uint32
			wParam  uintptr
			lParam  uintptr
			time    uint32
			pt      [2]int32
			_       uint32
		}
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(r) <= 0 { // WM_QUIT 或出错
				return
			}
			if msg.message == wmHotkey && msg.wParam == id {
				fn()
			}
		}
	}()
	res := <-ready
	if res.err != nil {
		return nil, res.err
	}
	return func() {
		procPostThreadMessageW.Call(uintptr(res.tid), wmQuit, 0, 0)
	}, nil
}
