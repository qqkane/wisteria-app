//go:build windows

package core

// IsElevated: does the process run with administrator rights? The TUN mode
// needs them to create the wintun adapter and install auto-routes.

import "golang.org/x/sys/windows"

func IsElevated() bool {
	t, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false
	}
	defer t.Close()
	return t.IsElevated()
}
