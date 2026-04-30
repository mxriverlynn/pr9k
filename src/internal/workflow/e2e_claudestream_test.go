package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

// e2eFixturesDir returns the path to the shared claudestream testdata/ fixtures
// relative to this test file's location.
func e2eFixturesDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile is .../src/internal/workflow/e2e_claudestream_test.go
	// Hop sideways into the claudestream package's testdata directory.
	return filepath.Join(filepath.Dir(thisFile), "..", "claudestream", "testdata")
}

// fakeClaude writes a shell script to t.TempDir()/fake-claude.sh that cats the
// fixture file to stdout and exits with exitCode. Optional stderrLines are written
// to stderr (one per line, without the [stderr] prefix — the workflow adds that).
// Returns the argv slice to pass as the command.
// Skips the test on Windows with a clear message.
func fakeClaude(t *testing.T, fixturePath string, exitCode int, stderrLines ...string) []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("e2e claudestream tests require a POSIX shell — skipping on Windows")
	}

	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")
	for _, line := range stderrLines {
		fmt.Fprintf(&sb, "printf '%%s\\n' %s >&2\n", shellQuote(line))
	}
	fmt.Fprintf(&sb, "cat %s\n", shellQuote(fixturePath))
	fmt.Fprintf(&sb, "exit %d\n", exitCode)

	scriptPath := filepath.Join(t.TempDir(), "fake-claude.sh")
	if err := os.WriteFile(scriptPath, []byte(sb.String()), 0o700); err != nil {
		t.Fatalf("fakeClaude: write script: %v", err)
	}
	return []string{"sh", scriptPath}
}

// shellQuote wraps s in single quotes, safe for POSIX sh.
// Assumes s contains no single-quote characters (fixture paths and test strings never do).
func shellQuote(s string) string {
	return "'" + s + "'"
}

// newE2ERunner creates a capturing Runner for e2e tests.
// Returns the runner, a drain function (snapshots collected sendLine lines), and a cleanup function.
func newE2ERunner(t *testing.T) (*Runner, func() []string) {
	t.Helper()
	r, _, drain := newCapturingRunner(t)
	return r, drain
}

// artifactPath creates a parent directory and returns a .jsonl path within it.
func artifactPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "step.jsonl")
}

// readArtifact reads and returns the content of the artifact file as a slice of
// non-empty lines.
func readArtifact(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readArtifact: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// linesContaining returns a new slice of lines that contain substr.
func linesContaining(lines []string, substr string) []string {
	var out []string
	for _, l := range lines {
		if strings.Contains(l, substr) {
			out = append(out, l)
		}
	}
	return out
}

// Test 1: Success path end-to-end.
// Verifies that RunSandboxedStep with the smoke-success fixture:
//   - sets LastCapture() to the result.result value
//   - writes the fixture lines + ralph_end sentinel to the artifact
//   - emits the assistant text and finalize summary line via sendLine
func TestE2E_SuccessPath(t *testing.T) {
	fixtPath := filepath.Join(e2eFixturesDir(t), "smoke-success.ndjson")
	artifact := artifactPath(t)
	argv := fakeClaude(t, fixtPath, 0)

	runner, drain := newE2ERunner(t)

	err := runner.RunSandboxedStep("e2e-success", argv, SandboxOptions{
		ArtifactPath: artifact,
		CaptureMode:  ui.CaptureResult,
	})
	if err != nil {
		t.Fatalf("RunSandboxedStep: unexpected error: %v", err)
	}

	// Assert LastCapture() equals the fixture's result.result.
	got := runner.LastCapture()
	want := "Hello there, friend."
	if got != want {
		t.Errorf("LastCapture() = %q, want %q", got, want)
	}

	// Assert the artifact exists and its last line is the ralph_end sentinel.
	artifactLines := readArtifact(t, artifact)
	if len(artifactLines) == 0 {
		t.Fatal("artifact file is empty")
	}
	lastLine := artifactLines[len(artifactLines)-1]
	const wantSentinel = `{"type":"ralph_end","ok":true,"schema":"v1"}`
	if lastLine != wantSentinel {
		t.Errorf("artifact last line = %q, want sentinel %q", lastLine, wantSentinel)
	}

	// The fixture has 4 NDJSON lines; artifact should be fixture + sentinel = 5 lines.
	wantLines := 5
	if len(artifactLines) != wantLines {
		t.Errorf("artifact has %d lines, want %d", len(artifactLines), wantLines)
	}

	// Assert the sendLine slice contains the assistant text and finalize summary.
	lines := drain()
	if len(linesContaining(lines, "Hello there, friend.")) == 0 {
		t.Errorf("sendLine output missing assistant text; got lines: %v", lines)
	}
	// Finalize format: "N turns · in/out tokens ..."
	if len(linesContaining(lines, "turns ·")) == 0 {
		t.Errorf("sendLine output missing finalize summary; got lines: %v", lines)
	}
}

// Test 2: is_error=true path.
// Verifies that RunSandboxedStep with the auth-failure fixture:
//   - returns a non-nil error containing the truncated result and session_id
//   - sets LastCapture() to empty string
//   - writes the sentinel (result event was observed, even though is_error=true)
func TestE2E_IsErrorPath(t *testing.T) {
	fixtPath := filepath.Join(e2eFixturesDir(t), "smoke-auth-failure.ndjson")
	artifact := artifactPath(t)
	argv := fakeClaude(t, fixtPath, 1)

	runner, _ := newE2ERunner(t)

	err := runner.RunSandboxedStep("e2e-auth-failure", argv, SandboxOptions{
		ArtifactPath: artifact,
		CaptureMode:  ui.CaptureResult,
	})

	// Assert error is non-nil and contains is_error=true marker and session_id.
	if err == nil {
		t.Fatal("RunSandboxedStep: expected error for is_error=true fixture, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "is_error=true") {
		t.Errorf("error message missing 'is_error=true': %q", errMsg)
	}
	// Session ID from smoke-auth-failure.ndjson.
	const sessionID = "004fdbf6-7f5f-4fdb-aa5a-e43c0a50c42d"
	if !strings.Contains(errMsg, sessionID) {
		t.Errorf("error message missing session ID %q: %q", sessionID, errMsg)
	}

	// Assert LastCapture() is empty.
	if got := runner.LastCapture(); got != "" {
		t.Errorf("LastCapture() = %q after is_error, want empty string", got)
	}

	// Assert sentinel WAS written (D26: sentinel signals result arrived, not success).
	artifactLines := readArtifact(t, artifact)
	const wantSentinel = `{"type":"ralph_end","ok":true,"schema":"v1"}`
	found := false
	for _, l := range artifactLines {
		if l == wantSentinel {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("artifact missing sentinel line; artifact lines: %v", artifactLines)
	}
}

// Test 3: Retry truncates JSONL (O_TRUNC semantics).
// Verifies that re-invoking RunSandboxedStep with the same ArtifactPath overwrites
// the previous attempt's bytes, leaving only the second invocation's content.
func TestE2E_RetryTruncatesArtifact(t *testing.T) {
	fixtDir := e2eFixturesDir(t)
	artifact := artifactPath(t)

	runner, _ := newE2ERunner(t)

	// First invocation: auth-failure fixture.
	argvFail := fakeClaude(t, filepath.Join(fixtDir, "smoke-auth-failure.ndjson"), 1)
	_ = runner.RunSandboxedStep("e2e-retry-1", argvFail, SandboxOptions{
		ArtifactPath: artifact,
		CaptureMode:  ui.CaptureResult,
	})

	firstLines := readArtifact(t, artifact)
	if len(firstLines) == 0 {
		t.Fatal("artifact is empty after first invocation")
	}

	// Second invocation: success fixture, same artifact path.
	argvSuccess := fakeClaude(t, filepath.Join(fixtDir, "smoke-success.ndjson"), 0)
	err := runner.RunSandboxedStep("e2e-retry-2", argvSuccess, SandboxOptions{
		ArtifactPath: artifact,
		CaptureMode:  ui.CaptureResult,
	})
	if err != nil {
		t.Fatalf("second RunSandboxedStep: unexpected error: %v", err)
	}

	secondLines := readArtifact(t, artifact)

	// The artifact should now contain ONLY the success fixture content (4 lines + sentinel).
	// Any auth-failure content from the first invocation must be gone.
	const wantSentinel = `{"type":"ralph_end","ok":true,"schema":"v1"}`
	if secondLines[len(secondLines)-1] != wantSentinel {
		t.Errorf("artifact last line after retry = %q, want sentinel", secondLines[len(secondLines)-1])
	}

	// The auth-failure session ID must not appear in the second artifact.
	const authSessionID = "004fdbf6-7f5f-4fdb-aa5a-e43c0a50c42d"
	for _, l := range secondLines {
		if strings.Contains(l, authSessionID) {
			t.Errorf("artifact still contains auth-failure session ID after retry; line: %q", l)
		}
	}

	// The success session ID must be present.
	const successSessionID = "765dadfd-4a06-4a83-b590-c95b81f8dca9"
	found := false
	for _, l := range secondLines {
		if strings.Contains(l, successSessionID) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("artifact missing success session ID %q after retry; lines: %v", successSessionID, secondLines)
	}
}

// Test 4: No-result crash path.
// Verifies that when fake-claude emits only the system init line and exits 1:
//   - RunSandboxedStep returns a non-nil error containing "no result event"
//   - The .jsonl artifact has NO ralph_end sentinel line
func TestE2E_NoResultCrashPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("e2e claudestream tests require a POSIX shell — skipping on Windows")
	}

	// Write an inline fixture with only the system init line.
	initLine := `{"type":"system","subtype":"init","cwd":"/tmp","session_id":"crash-test-session","tools":[],"mcp_servers":[],"model":"claude-test","permissionMode":"default","apiKeySource":"none","claude_code_version":"0.0.0","output_style":"default","agents":[],"skills":[],"plugins":[]}`
	tmpDir := t.TempDir()
	crashFixt := filepath.Join(tmpDir, "crash.ndjson")
	if err := os.WriteFile(crashFixt, []byte(initLine+"\n"), 0o644); err != nil {
		t.Fatalf("write crash fixture: %v", err)
	}

	artifact := artifactPath(t)
	argv := fakeClaude(t, crashFixt, 1)

	runner, _ := newE2ERunner(t)
	err := runner.RunSandboxedStep("e2e-crash", argv, SandboxOptions{
		ArtifactPath: artifact,
		CaptureMode:  ui.CaptureResult,
	})

	if err == nil {
		t.Fatal("RunSandboxedStep: expected error for crash path, got nil")
	}
	if !strings.Contains(err.Error(), "no result event") {
		t.Errorf("error message missing 'no result event': %q", err.Error())
	}

	// Artifact should exist (the init line was written) but contain NO sentinel.
	artifactLines := readArtifact(t, artifact)
	if len(artifactLines) == 0 {
		t.Fatal("artifact is empty — expected at least the system init line")
	}
	const sentinel = `{"type":"ralph_end","ok":true,"schema":"v1"}`
	for _, l := range artifactLines {
		if l == sentinel {
			t.Errorf("artifact unexpectedly contains sentinel line after crash (no result event)")
		}
	}
}

// Test 5: Stderr passthrough (D20, D27).
// Verifies that stderr lines are forwarded via sendLine with "[stderr] " prefix
// and do NOT appear in the .jsonl artifact.
func TestE2E_StderrPassthrough(t *testing.T) {
	fixtPath := filepath.Join(e2eFixturesDir(t), "smoke-success.ndjson")
	artifact := artifactPath(t)

	stderrLines := []string{
		"diagnostic line one",
		"diagnostic line two",
		"diagnostic line three",
	}
	argv := fakeClaude(t, fixtPath, 0, stderrLines...)

	runner, drain := newE2ERunner(t)

	err := runner.RunSandboxedStep("e2e-stderr", argv, SandboxOptions{
		ArtifactPath: artifact,
		CaptureMode:  ui.CaptureResult,
	})
	if err != nil {
		t.Fatalf("RunSandboxedStep: unexpected error: %v", err)
	}

	lines := drain()

	// Assert each stderr line is present with "[stderr] " prefix.
	for _, want := range stderrLines {
		prefixed := "[stderr] " + want
		if len(linesContaining(lines, prefixed)) == 0 {
			t.Errorf("sendLine output missing %q; got lines: %v", prefixed, lines)
		}
	}

	// Assert the .jsonl artifact does NOT contain any stderr text.
	artifactLines := readArtifact(t, artifact)
	for _, stderrLine := range stderrLines {
		for _, artifactLine := range artifactLines {
			if strings.Contains(artifactLine, stderrLine) {
				t.Errorf("artifact contains stderr text %q: %q", stderrLine, artifactLine)
			}
		}
	}

	// Assert no [malformed-json] entries appeared in the artifact
	// (stderr lines must not have been fed to the pipeline parser).
	for _, l := range artifactLines {
		if strings.Contains(l, "malformed-json") || strings.Contains(l, "malformed_json") {
			t.Errorf("artifact unexpectedly contains malformed-json entry: %q", l)
		}
	}
}

// Ensure e2e tests are race-safe: the capturingRunner uses a mutex internally.
// This test verifies that two concurrent RunSandboxedStep calls on independent
// runners do not data-race on shared state.
func TestE2E_ConcurrentRunnersNoRace(t *testing.T) {
	fixtPath := filepath.Join(e2eFixturesDir(t), "smoke-success.ndjson")

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			argv := fakeClaude(t, fixtPath, 0)
			artifact := artifactPath(t)
			runner, _ := newE2ERunner(t)
			if err := runner.RunSandboxedStep("e2e-concurrent", argv, SandboxOptions{
				ArtifactPath: artifact,
				CaptureMode:  ui.CaptureResult,
			}); err != nil {
				t.Errorf("concurrent RunSandboxedStep: %v", err)
			}
		}()
	}
	wg.Wait()
}
