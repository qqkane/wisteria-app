//go:build windows

package core

import (
	"os/exec"
	"syscall"
)

const exeSuffix = ".exe"

// hideConsole keeps the core processes invisible: no flashing conhost window.
const createNoWindow = 0x08000000

func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
