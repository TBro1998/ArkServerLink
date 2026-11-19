package update

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// RunInstaller 启动安装包静默安装到 dir，装完后由安装包重新打开客户端（/RELAUNCH，见 project.nsi）。
// 调用方随后要尽快退出：安装包会等客户端退出、文件解除占用后再覆盖。
func RunInstaller(installer, dir string) error {
	// NSIS 的 /D= 必须是最后一个参数且不能加引号（路径里可以有空格），
	// Go 默认的参数转义会给带空格的参数加引号，所以自己拼命令行
	args := "/S /RELAUNCH /D=" + dir
	cmd := exec.Command(installer)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       fmt.Sprintf("%s %s", syscall.EscapeArg(installer), args),
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	err := cmd.Start()
	if errors.Is(err, windows.ERROR_ELEVATION_REQUIRED) {
		// 按机器安装（Program Files）时安装包要管理员权限，CreateProcess 不会弹 UAC，改用 ShellExecute
		return shellExecute(installer, args)
	}
	if err != nil {
		return err
	}
	return cmd.Process.Release()
}

func shellExecute(file, args string) error {
	verb, _ := windows.UTF16PtrFromString("runas")
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	a, err := windows.UTF16PtrFromString(args)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, f, a, nil, windows.SW_HIDE)
}
