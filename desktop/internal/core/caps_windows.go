//go:build windows

package core

// Windows counterpart of caps_other.go: the setcap capability route is a
// Unix mechanism. Windows TUN elevates the whole app instead, so service.go
// never reaches this stub through the Linux branch — it exists so the
// package typechecks on windows.

import "errors"

func tunGrantHint(singbox string) (string, error) {
	return "", errors.New("TUN требует прав администратора — перезапустите приложение от администратора")
}
