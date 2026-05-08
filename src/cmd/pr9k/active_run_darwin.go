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
// Uses ps -o command= (full argv) to avoid MAXCOMLEN-16 truncation from comm=.
// Falls back to lsof if ps fails. If both fail and the PID is alive, returns true
// rather than treating a live process as dead (D-5 relaxed for binary-read failure).
func isProcessAlive(pid int, _ string) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}

	exePath := executablePathForPID(pid)
	if exePath == "" {
		// Both methods failed — PID is alive, trust it rather than false-negative.
		return true
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

// executablePathForPID returns the executable path for pid on macOS.
// Tries ps -o command= (full argv, first token) then lsof -a -d txt -Fn.
// Returns "" if both methods fail.
func executablePathForPID(pid int) string {
	out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "command=").Output()
	if err == nil {
		if fields := strings.Fields(string(out)); len(fields) > 0 {
			return fields[0]
		}
	}

	out, err = exec.Command("lsof", "-p", fmt.Sprintf("%d", pid), "-a", "-d", "txt", "-Fn").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "n") {
				return strings.TrimPrefix(line, "n")
			}
		}
	}

	return ""
}
