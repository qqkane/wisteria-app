//go:build !windows

package core

import (
	"os/exec"
	"syscall"
)

const exeSuffix = ""

// hideConsole puts the core into its own process group: it stops receiving
// the app's terminal signals (Ctrl+C) and can be killed as a group later.
// (The Windows counterpart hides the console window; the process-group
// equivalent of the Job Object's kill-on-close would need PDEATHSIG, which
// fires on the death of the forking *thread*, not the process - not worth
// the foot-gun while CommandContext already kills the core on stop.)
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
