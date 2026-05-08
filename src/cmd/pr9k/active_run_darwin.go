package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// isProcessAlive returns true if the process with the given PID is alive AND
// its executable matches the current os.Executable() (EvalSymlinks-canonicalized).
// Uses `ps -p <pid> -o comm=` to read the process image path.
// Any binary-path read error → treat as dead → false (D-5).
func isProcessAlive(pid int, _ string) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}

	out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "comm=").Output()
	if err != nil {
		// binary-path read failure → treat as dead (D-5)
		return false
	}

	exePath := strings.TrimSpace(string(out))
	if exePath == "" {
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
