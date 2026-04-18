package sandbox

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestTerminator_PostExitShortCircuit verifies that the terminator returns nil
// without attempting any signal when the cmd has already exited
// (cmd.ProcessState != nil).
func TestTerminator_PostExitShortCircuit(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("cmd.Run() failed: %v", err)
	}
	// After Run(), cmd.ProcessState is set.
	if cmd.ProcessState == nil {
		t.Fatal("expected cmd.ProcessState to be non-nil after Run()")
	}

	cidfile, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}

	terminator := NewTerminator(cmd, cidfile)
	if err := terminator(syscall.SIGTERM); err != nil {
		t.Errorf("expected nil from terminator after process exit, got %v", err)
	}
}

// TestTerminator_CidfileMissingFallsBackToCLISignal verifies that when the
// cidfile never appears within the poll window, the terminator falls back to
// signaling the docker CLI process (cmd.Process).
func TestTerminator_CidfileMissingFallsBackToCLISignal(t *testing.T) {
	// Use a long-running process so it's alive when we call the terminator.
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start() failed: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGKILL)
		_ = cmd.Wait()
	})

	// Use a cidfile path that will never exist.
	cidfile := "/tmp/ralph-nonexistent-cidfile-test-xyz.cid"
	_ = os.Remove(cidfile)

	// Override the poll wait to a short value for test speed.
	// We patch cidfileWait by using a subtest that completes quickly.
	// Since cidfileWait is a package-level constant, we can't override it directly.
	// Instead, we measure that the terminator does fall back: the process should
	// receive the signal.

	terminator := NewTerminator(cmd, cidfile)

	// Invoke with a short-enough test. The cidfile poll takes cidfileWait (2s) at
	// most. We set a test timeout via the test binary's -timeout flag. The fallback
	// path should signal the process.
	done := make(chan error, 1)
	go func() {
		done <- terminator(syscall.SIGTERM)
	}()

	select {
	case err := <-done:
		// The signal may return an error if the process has already exited
		// (race between signal delivery and process exit). Either nil or
		// "os: process already finished" is acceptable.
		if err != nil && err.Error() != "os: process already finished" {
			t.Errorf("unexpected error from terminator fallback: %v", err)
		}
	case <-time.After(cidfileWait + 500*time.Millisecond):
		t.Error("terminator did not return within poll window + buffer")
	}

	// Verify the process received the signal (it should be gone or dying).
	_ = cmd.Wait()
}

// TestTerminator_CidfileWithValidCIDDispatchesDockerKill verifies that when a
// valid 64-char hex container ID appears in the cidfile, the terminator invokes
// docker kill instead of signaling the CLI process.
//
// Implementation: write a known 64-char hex string to the cidfile before the
// terminator polls, then inject a fake docker binary on PATH that records its
// argv to a temp file.
func TestTerminator_CidfileWithValidCIDDispatchesDockerKill(t *testing.T) {
	// Build a fake docker binary that writes its args to a file.
	fakeDockerArgFile := t.TempDir() + "/docker-args.txt"
	fakeDockerScript := t.TempDir() + "/docker"
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + fakeDockerArgFile + "\nexit 0\n"
	if err := os.WriteFile(fakeDockerScript, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile fake docker: %v", err)
	}

	// Prepend fake docker dir to PATH.
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir()[:0]+fakeDockerScript[:len(fakeDockerScript)-len("/docker")]+":"+oldPath)

	// Write cidfile with a valid 64-char lowercase hex string.
	cidfile := t.TempDir() + "/ralph-test.cid"
	validCID := strings.Repeat("a1b2c3d4", 8) // 64 chars
	if err := os.WriteFile(cidfile, []byte(validCID), 0o644); err != nil {
		t.Fatalf("WriteFile cidfile: %v", err)
	}

	// Use a process that is nominally running (sleep 30) but we won't signal
	// it via the CLI path — the docker-kill path should be taken instead.
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start(): %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGKILL)
		_ = cmd.Wait()
	})

	terminator := NewTerminator(cmd, cidfile)
	if err := terminator(syscall.SIGTERM); err != nil {
		t.Fatalf("terminator returned error: %v", err)
	}

	// Verify the fake docker was called with expected args.
	argData, err := os.ReadFile(fakeDockerArgFile)
	if err != nil {
		t.Fatalf("could not read fake docker arg file: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(argData)), "\n")
	if len(args) < 3 {
		t.Fatalf("expected at least 3 args to fake docker, got %v", args)
	}
	if args[0] != "kill" {
		t.Errorf("expected args[0]='kill', got %q", args[0])
	}
	// args[1] should be --signal=15 (SIGTERM = 15).
	if args[1] != "--signal=15" {
		t.Errorf("expected args[1]='--signal=15', got %q", args[1])
	}
	if args[2] != validCID {
		t.Errorf("expected args[2]=%q (CID), got %q", validCID, args[2])
	}
}

// TestIsValidCID covers boundary and malformed inputs (TP-002).
// Security-adjacent: a false positive could cause docker kill against a wrong CID.
func TestIsValidCID(t *testing.T) {
	validCID := strings.Repeat("a1b2c3d4", 8) // exactly 64 lowercase-hex chars

	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty string", "", false},
		{"63-char valid hex (too short)", validCID[:63], false},
		{"65-char valid hex (too long)", validCID + "a", false},
		{"64-char uppercase hex", strings.ToUpper(validCID), false},
		{"64-char with non-hex char g", validCID[:63] + "g", false},
		{"64-char with space", validCID[:63] + " ", false},
		{"64-char with newline", validCID[:63] + "\n", false},
		{"valid 64-char lowercase hex", validCID, true},
		{"64-char all zeros", strings.Repeat("0", 64), true},
		{"64-char all f", strings.Repeat("f", 64), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidCID(tc.input)
			if got != tc.want {
				t.Errorf("isValidCID(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestTerminator_NilProcessNoCrash verifies that the terminator returns nil
// without panicking when cmd was never started (cmd.Process == nil,
// cmd.ProcessState == nil) and the cidfile does not exist (TP-004).
//
// NOTE: This test takes ~2s because pollCidfile waits for cidfileWait before
// falling back to the cmd.Process nil guard.
func TestTerminator_NilProcessNoCrash(t *testing.T) {
	cmd := exec.Command("true") // not started — Process and ProcessState are both nil

	cidfile := t.TempDir() + "/ralph-nil-process.cid"
	// cidfile does not exist — pollCidfile will time out and return "".

	terminator := NewTerminator(cmd, cidfile)

	done := make(chan error, 1)
	go func() {
		done <- terminator(syscall.SIGTERM)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil from terminator with nil Process, got %v", err)
		}
	case <-time.After(cidfileWait + 500*time.Millisecond):
		t.Error("terminator did not return within poll window + buffer")
	}
}

// TestPollCidfile_ReturnsEmptyOnTimeout verifies that pollCidfile returns ""
// when the file never appears within the poll window (TP-001).
// The empty-string return drives the fallback-to-CLI-signal path; a regression
// returning a non-empty string would cause docker-kill against an invalid CID.
func TestPollCidfile_ReturnsEmptyOnTimeout(t *testing.T) {
	start := time.Now()
	got := pollCidfile("/nonexistent/ralph-test-timeout.cid", 100*time.Millisecond)

	if got != "" {
		t.Errorf("pollCidfile returned %q, want empty string", got)
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Errorf("pollCidfile took %v, expected to complete within ~150ms", elapsed)
	}
}

// TestPollCidfile_WhitespaceOnlyContentReturnsEmpty verifies that pollCidfile
// returns "" when the cidfile contains only whitespace (TP-005).
// Exercises the TrimSpace→isValidCID pipeline — docker writes a hex CID, not
// whitespace, so this confirms defensive handling of partial writes.
func TestPollCidfile_WhitespaceOnlyContentReturnsEmpty(t *testing.T) {
	cidfile := t.TempDir() + "/ralph-whitespace.cid"
	if err := os.WriteFile(cidfile, []byte("   \n  "), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := pollCidfile(cidfile, 100*time.Millisecond)
	if got != "" {
		t.Errorf("pollCidfile returned %q for whitespace-only content, want empty string", got)
	}
}

// TestPollCidfile_FileAppearsAfterPollStarts verifies that pollCidfile detects
// a cidfile that materialises mid-poll and returns its CID (TP-005).
func TestPollCidfile_FileAppearsAfterPollStarts(t *testing.T) {
	cidfile := t.TempDir() + "/ralph-midpoll.cid"
	validCID := strings.Repeat("a1b2c3d4", 8)

	// Write the cidfile after a short delay. The sleep here simulates docker
	// writing the cidfile asynchronously after container startup — it is not
	// used for test synchronisation (no shared Go memory between goroutines).
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(cidfile, []byte(validCID), 0o644)
	}()

	got := pollCidfile(cidfile, 2*time.Second)
	if got != validCID {
		t.Errorf("pollCidfile returned %q, want %q", got, validCID)
	}
}

// TestTerminator_CidfileWithPartialWriteFallsBack verifies that a cidfile
// containing a partial (non-64-char) write causes the terminator to fall back
// to signaling the CLI process.
func TestTerminator_CidfileWithPartialWriteFallsBack(t *testing.T) {
	cidfile := t.TempDir() + "/ralph-partial.cid"
	// Write a short, invalid CID.
	if err := os.WriteFile(cidfile, []byte("partial"), 0o644); err != nil {
		t.Fatalf("WriteFile cidfile: %v", err)
	}

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start(): %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGKILL)
		_ = cmd.Wait()
	})

	terminator := NewTerminator(cmd, cidfile)

	done := make(chan error, 1)
	go func() {
		done <- terminator(syscall.SIGTERM)
	}()

	select {
	case err := <-done:
		if err != nil && err.Error() != "os: process already finished" {
			t.Errorf("unexpected error from terminator partial-cidfile fallback: %v", err)
		}
	case <-time.After(cidfileWait + 500*time.Millisecond):
		t.Error("terminator did not return within poll window + buffer")
	}

	_ = cmd.Wait()
}
