//go:build !windows

package core

// Job objects are a Windows mechanism; on other platforms process groups
// and context cancellation already tie the core's lifetime to the app.

func keepCoreInJob(pid int) error { return nil }
