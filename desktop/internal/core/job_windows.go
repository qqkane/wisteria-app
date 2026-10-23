//go:build windows

package core

// keepCoreInJob puts a freshly started core subprocess into a kill-on-close
// job object so a core can never outlive the GUI: if the app crashes or is
// killed from the Task Manager, teardown never runs, and without this the
// core keeps running as an orphan (holding the SOCKS port, logging traffic).
// The job handle is intentionally never closed — its lifetime IS the
// cleanup mechanism; Windows closes it when the GUI process dies.

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func keepCoreInJob(pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	return nil
}
