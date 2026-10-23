package main

// RestartAsAdmin relaunches the app via the UAC prompt ("runas") and quits
// the current (unelevated) instance. Used by the TUN settings row.

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

var shell32 = windows.NewLazySystemDLL("shell32.dll")

func (a *App) RestartAsAdmin() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	exePtr, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	ret, _, callErr := shell32.NewProc("ShellExecuteW").Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(exePtr)),
		0, 0,
		1, // SW_SHOWNORMAL
	)
	if ret <= 32 {
		return fmt.Errorf("перезапуск отменён: %v", callErr)
	}
	a.logf("перезапуск от администратора...")
	runtime.Quit(a.ctx)
	return nil
}
