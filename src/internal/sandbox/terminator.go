package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// cidfileWait is the maximum duration to poll for the cidfile to appear.
const cidfileWait = 2 * time.Second

// cidPollInterval is how often to check for the cidfile while polling.
const cidPollInterval = 50 * time.Millisecond

// NewTerminator returns a closure that, when invoked with a signal,
// tries to signal the running container (via `docker kill --signal`)
// and falls back to signaling the host docker CLI process if the
// cidfile has not yet been written.
//
// The closure captures *exec.Cmd (not bare *os.Process) so it can
// check cmd.ProcessState != nil before signaling — guards against
// PID-recycling hazards when Terminate() fires after the process has
// exited (iteration 6 F2).
//
// Behavior per signal invocation:
//  1. If cmd.ProcessState != nil, return nil (process already exited).
//  2. Poll `cidfile` for up to cidfileWait (default 2s).
//  3. If cidfile appears and contains a 64-char lowercase-hex string,
//     run `docker kill --signal=<SIG> <cid>`.
//  4. If cidfile still missing after the wait OR contains a partial
//     write, fall back to cmd.Process.Signal(sig) on the docker CLI —
//     before the container is running, signaling the CLI aborts the
//     launch cleanly (no orphan container can exist).
func NewTerminator(cmd *exec.Cmd, cidfile string) func(syscall.Signal) error {
	return func(sig syscall.Signal) error {
		// Guard: process already exited — no-op to avoid PID reuse hazard.
		if cmd.ProcessState != nil {
			return nil
		}

		cid := pollCidfile(cidfile, cidfileWait)
		if cid != "" {
			return dockerKill(sig, cid)
		}

		// Fallback: signal the docker CLI process directly.
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(sig)
	}
}

// pollCidfile polls for the cidfile up to maxWait, returning the
// container ID if a valid 64-char lowercase-hex string is found.
// Returns "" if the file is missing or contains a partial write.
func pollCidfile(cidfile string, maxWait time.Duration) string {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(cidfile)
		if err == nil {
			cid := strings.TrimSpace(string(data))
			if isValidCID(cid) {
				return cid
			}
		}
		time.Sleep(cidPollInterval)
	}
	return ""
}

// isValidCID returns true if s is a 64-char lowercase-hex string.
func isValidCID(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// dockerKill runs `docker kill --signal=<SIG> <cid>` using the signal number.
func dockerKill(sig syscall.Signal, cid string) error {
	cmd := exec.Command("docker", "kill", fmt.Sprintf("--signal=%d", int(sig)), cid)
	return cmd.Run()
}
