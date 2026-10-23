//go:build !windows

package main

import "errors"

// RestartAsAdmin is the Windows UAC relaunch; elsewhere it is unavailable
// (TUN helpers differ per platform, so the button stays Windows-only).
func (a *App) RestartAsAdmin() error {
	return errors.New("перезапуск от администратора поддерживается только на Windows")
}
