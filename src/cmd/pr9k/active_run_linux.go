package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// isProcessAlive returns true if the process with the given PID is alive AND
// its executable matches the current os.Executable() (EvalSymlinks-canonicalized).
// The expectedBinary parameter (from the state file) is used for logging only;
// /proc/<pid>/exe is the authoritative source on Linux.
// Any binary-path read error → treat as dead → false (D-5).
func isProcessAlive(pid int, _ string) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}

	exePath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		// binary-path read failure → treat as dead (D-5)
		return false
	}

	self, err := os.Executable()
	if err != nil {
		return false
	}

	canon := func(p string) string {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			return resolved
		}
		return p
	}

	return canon(exePath) == canon(self)
}
