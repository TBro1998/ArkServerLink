//go:build !windows

package easytier

import (
	"os"
	"os/exec"
)

func binName(name string) string { return name }

func configureCmd(*exec.Cmd) {}

func attachProcess(*os.Process) error { return nil }
