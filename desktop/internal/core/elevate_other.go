//go:build !windows

package core

// IsElevated: on Linux "elevated" means root, which TUN mode needs to
// create the tun device and install auto-routes.

import "os"

func IsElevated() bool { return os.Geteuid() == 0 }
