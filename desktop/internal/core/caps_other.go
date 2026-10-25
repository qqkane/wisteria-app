//go:build !windows

package core

// TUN without root: the sing-box helper needs CAP_NET_ADMIN to create the
// tun device and install auto_route. Two routes grant it: run the whole app
// as root, or give the sing-box binary the file capability once
// (setcap cap_net_admin=+ep) — the kernel then grants the capability to that
// binary no matter who launched it. Detection mirrors what the kernel will
// do, so the UI can offer the setcap route before the user reaches for sudo.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// capNetAdmin is the capability bit TUN setup needs (linux/capability.h).
const capNetAdmin = 12

// TunAvailable reports whether the sing-box helper can set up TUN right
// now: the process is root, or the binary carries cap_net_admin.
func TunAvailable(singbox string) bool {
	_, err := tunGrantHint(singbox)
	return err == nil
}

// tunGrantHint returns "" when TUN can start, otherwise a human hint with
// the exact command that would grant the capability.
func tunGrantHint(singbox string) (string, error) {
	if os.Geteuid() == 0 {
		return "", nil
	}
	if singbox != "" && binaryHasCapNetAdmin(singbox) {
		return "", nil
	}
	target := singbox
	if target == "" {
		target = "<путь к sing-box>"
	}
	return "", fmt.Errorf("TUN требует права root либо capability на бинаре sing-box:\n  sudo setcap cap_net_admin=+ep %s\nпосле этого TUN работает без запуска приложения от root", target)
}

// binaryHasCapNetAdmin parses `getcap -r <bin>` output (getcap is in
// libcap[2]-bin; when absent we conservatively report no capability).
func binaryHasCapNetAdmin(bin string) bool {
	getcap, err := exec.LookPath("getcap")
	if err != nil {
		return false
	}
	out, err := exec.Command(getcap, bin).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "cap_net_admin")
}
