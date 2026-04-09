package workflow

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
)

// newTestRunner creates a Runner backed by a temp dir logger for testing.
func newTestRunner(t *testing.T) (*Runner, *logger.Logger) {
	t.Helper()
	dir := t.TempDir()
	log, err := logger.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	return NewRunner(log, dir), log
}

// collectLines starts a goroutine that reads all lines from r.LogReader() until
// EOF. The returned function blocks until EOF and returns the collected lines.
func collectLines(t *testing.T, r *Runner) func() []string {
	t.Helper()
	ch := make(chan string, 1000)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(r.LogReader())
		for scanner.Scan() {
			ch <- scanner.Text()
		}
	}()
	return func() []string {
		var lines []string
		for line := range ch {
			lines = append(lines, line)
		}
		return lines
	}
}

// readLogFile returns all non-empty lines from the single log file written by log.
func readLogFile(t *testing.T, log *logger.Logger, dir string) []string {
	t.Helper()
	if err := log.Close(); err != nil {
		t.Fatalf("log.Close: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "logs"))
	if err != nil {
		t.Fatalf("ReadDir logs: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no log files found")
	}
	data, err := os.ReadFile(filepath.Join(dir, "logs", entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// Unit tests

func TestRunStep_StdoutArrivesInPipe(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	if err := r.RunStep("test-step", []string{"echo", "hello from stdout"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = r.Close()

	lines := collect()
	found := false
	for _, l := range lines {
		if l == "hello from stdout" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'hello from stdout' in pipe output, got %v", lines)
	}
	_ = log.Close()
}

func TestRunStep_StderrArrivesInPipe(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	if err := r.RunStep("test-step", []string{"sh", "-c", "echo 'hello from stderr' >&2"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = r.Close()

	lines := collect()
	found := false
	for _, l := range lines {
		if l == "hello from stderr" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'hello from stderr' in pipe output, got %v", lines)
	}
	_ = log.Close()
}

func TestRunStep_StdoutAndStderrBothArriveInPipe(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	if err := r.RunStep("test-step", []string{"sh", "-c", "echo 'out line'; echo 'err line' >&2"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = r.Close()

	lines := collect()
	sorted := append([]string{}, lines...)
	sort.Strings(sorted)

	want := []string{"err line", "out line"}
	sort.Strings(want)

	if len(sorted) != len(want) {
		t.Fatalf("expected %v, got %v", want, lines)
	}
	for i, w := range want {
		if sorted[i] != w {
			t.Errorf("line %d: want %q, got %q", i, w, sorted[i])
		}
	}
	_ = log.Close()
}

func TestRunStep_AllLinesArrivedBeforeCmdWait(t *testing.T) {
	// Produce 200 lines on stderr; verify all arrive. This implicitly tests that
	// the WaitGroup drains both pipes before cmd.Wait() returns.
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	script := "for i in $(seq 1 200); do echo \"line $i\" >&2; done"
	if err := r.RunStep("test-step", []string{"sh", "-c", script}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = r.Close()

	lines := collect()
	if len(lines) != 200 {
		t.Errorf("expected 200 lines, got %d", len(lines))
	}
	_ = log.Close()
}

// Integration tests

func TestRunStep_IntegrationStdoutInPipeAndLogFile(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	r := NewRunner(log, dir)
	collect := collectLines(t, r)

	if err := r.RunStep("integration-step", []string{"echo", "integration hello"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = r.Close()

	pipeLines := collect()

	// Verify pipe
	foundInPipe := false
	for _, l := range pipeLines {
		if l == "integration hello" {
			foundInPipe = true
		}
	}
	if !foundInPipe {
		t.Errorf("expected 'integration hello' in pipe output, got %v", pipeLines)
	}

	// Verify log file
	logLines := readLogFile(t, log, dir)
	foundInLog := false
	for _, l := range logLines {
		if strings.Contains(l, "integration hello") {
			foundInLog = true
		}
	}
	if !foundInLog {
		t.Errorf("expected 'integration hello' in log file, got %v", logLines)
	}
}

func TestRunStep_IntegrationStderrInPipe(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	r := NewRunner(log, dir)
	collect := collectLines(t, r)

	if err := r.RunStep("integration-step", []string{"sh", "-c", "echo 'integration err' >&2"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = r.Close()

	lines := collect()
	found := false
	for _, l := range lines {
		if l == "integration err" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'integration err' in pipe output, got %v", lines)
	}
	_ = log.Close()
}

// T1 — RunStep returns error on command failure
func TestRunStep_ReturnsErrorOnNonZeroExit(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	err := r.RunStep("test-step", []string{"sh", "-c", "exit 1"})
	_ = r.Close()
	_ = collect()
	_ = log.Close()

	if err == nil {
		t.Fatal("expected error from non-zero exit, got nil")
	}
}

// T2 — RunStep returns error for non-existent command
func TestRunStep_ReturnsErrorForNonExistentCommand(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	err := r.RunStep("test-step", []string{"nonexistent-binary-xyz"})
	_ = r.Close()
	_ = collect()
	_ = log.Close()

	if err == nil {
		t.Fatal("expected error for non-existent command, got nil")
	}
	if !strings.Contains(err.Error(), "workflow: start") {
		t.Errorf("expected error to contain 'workflow: start', got %q", err.Error())
	}
}

// T3 — Multiple sequential RunStep calls share the same pipe
func TestRunStep_MultipleSequentialCallsSharePipe(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	if err := r.RunStep("step-one", []string{"echo", "output from step one"}); err != nil {
		t.Fatalf("RunStep step-one: %v", err)
	}
	if err := r.RunStep("step-two", []string{"echo", "output from step two"}); err != nil {
		t.Fatalf("RunStep step-two: %v", err)
	}
	_ = r.Close()

	lines := collect()
	_ = log.Close()

	foundOne, foundTwo := false, false
	for _, l := range lines {
		if l == "output from step one" {
			foundOne = true
		}
		if l == "output from step two" {
			foundTwo = true
		}
	}
	if !foundOne {
		t.Errorf("expected 'output from step one' in pipe output, got %v", lines)
	}
	if !foundTwo {
		t.Errorf("expected 'output from step two' in pipe output, got %v", lines)
	}
}

// T4 — stepName appears in log file lines
func TestRunStep_StepNameAppearsInLogFile(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	r := NewRunner(log, dir)
	collect := collectLines(t, r)

	if err := r.RunStep("my-named-step", []string{"echo", "some output"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = r.Close()
	_ = collect()

	logLines := readLogFile(t, log, dir)
	found := false
	for _, l := range logLines {
		if strings.Contains(l, "my-named-step") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected step name 'my-named-step' in log file lines, got %v", logLines)
	}
}

// T5 — Close is idempotent
func TestClose_IsIdempotent(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	_ = r.Close()
	_ = collect()
	_ = log.Close()

	// Second close should not panic and should return nil (io.PipeWriter behavior)
	err := r.Close()
	if err != nil {
		t.Errorf("expected nil on second Close(), got %v", err)
	}
}

func TestResolveCommand_ScriptPathAndIssueID(t *testing.T) {
	projectDir := "/home/user/project"
	cmd := []string{"ralph-bash/scripts/close_gh_issue", "{{ISSUE_ID}}"}
	got := ResolveCommand(projectDir, cmd, "42")
	want := []string{"/home/user/project/ralph-bash/scripts/close_gh_issue", "42"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("element %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveCommand_BareCommandPassthrough(t *testing.T) {
	projectDir := "/home/user/project"
	cmd := []string{"git", "push"}
	got := ResolveCommand(projectDir, cmd, "99")
	want := []string{"git", "push"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("element %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveCommand_MultipleIssueIDOccurrences(t *testing.T) {
	projectDir := "/proj"
	cmd := []string{"ralph-bash/scripts/foo", "{{ISSUE_ID}}", "--label={{ISSUE_ID}}"}
	got := ResolveCommand(projectDir, cmd, "7")
	want := []string{"/proj/ralph-bash/scripts/foo", "7", "--label=7"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("element %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveCommand_RelativeScriptPathResolved(t *testing.T) {
	projectDir := "/base"
	cmd := []string{"ralph-bash/scripts/foo", "arg"}
	got := ResolveCommand(projectDir, cmd, "1")
	wantExe := "/base/ralph-bash/scripts/foo"
	if got[0] != wantExe {
		t.Errorf("exe: got %q, want %q", got[0], wantExe)
	}
}

func TestResolveCommand_AbsolutePathUnchanged(t *testing.T) {
	projectDir := "/proj"
	cmd := []string{"/usr/bin/env", "{{ISSUE_ID}}"}
	got := ResolveCommand(projectDir, cmd, "3")
	if got[0] != "/usr/bin/env" {
		t.Errorf("exe: got %q, want /usr/bin/env", got[0])
	}
	if got[1] != "3" {
		t.Errorf("arg: got %q, want 3", got[1])
	}
}

func TestResolveCommand_NoTemplateVars_Passthrough(t *testing.T) {
	projectDir := "/proj"
	cmd := []string{"git", "commit", "-m", "fix things"}
	got := ResolveCommand(projectDir, cmd, "10")
	want := []string{"git", "commit", "-m", "fix things"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("element %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveCommand_EmptySlice(t *testing.T) {
	got := ResolveCommand("/proj", []string{}, "42")
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestResolveCommand_DoesNotMutateInput(t *testing.T) {
	original := []string{"ralph-bash/scripts/close_gh_issue", "{{ISSUE_ID}}"}
	input := make([]string, len(original))
	copy(input, original)
	ResolveCommand("/proj", input, "42")
	for i := range original {
		if input[i] != original[i] {
			t.Errorf("input[%d] mutated: got %q, want %q", i, input[i], original[i])
		}
	}
}

func TestResolveCommand_TemplateInExecutable(t *testing.T) {
	cmd := []string{"scripts/issue-{{ISSUE_ID}}/run", "arg"}
	got := ResolveCommand("/proj", cmd, "5")
	wantExe := "/proj/scripts/issue-5/run"
	if got[0] != wantExe {
		t.Errorf("exe: got %q, want %q", got[0], wantExe)
	}
}

func TestResolveCommand_SingleElementBareCommand(t *testing.T) {
	got := ResolveCommand("/proj", []string{"git"}, "1")
	if got[0] != "git" {
		t.Errorf("exe: got %q, want %q", got[0], "git")
	}
}

// Terminate unit tests

// TestTerminate_RunStepReturnsWithinTimeout starts a long-running subprocess,
// requests termination, and verifies RunStep returns within 5 seconds.
func TestTerminate_RunStepReturnsWithinTimeout(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunStep("long-step", []string{"sleep", "60"})
	}()

	// Give the process time to start before terminating.
	time.Sleep(50 * time.Millisecond)
	r.Terminate()

	select {
	case <-stepDone:
	case <-time.After(5 * time.Second):
		t.Fatal("RunStep did not return within 5 seconds after Terminate")
	}

	_ = r.Close()
	_ = collect()
	_ = log.Close()
}

// TestTerminate_ScannerGoroutinesExitAfterPipesClose verifies that after
// termination the subprocess pipes are closed so scanner goroutines inside
// RunStep exit naturally (evidenced by RunStep returning).
func TestTerminate_ScannerGoroutinesExitAfterPipesClose(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	stepDone := make(chan struct{})
	go func() {
		defer close(stepDone)
		_ = r.RunStep("pipe-step", []string{"sh", "-c", "while true; do echo line; sleep 0.05; done"})
	}()

	time.Sleep(50 * time.Millisecond)
	r.Terminate()

	select {
	case <-stepDone:
		// scanner goroutines exited — RunStep returned
	case <-time.After(5 * time.Second):
		t.Fatal("RunStep did not return after Terminate; scanner goroutines may still be blocked")
	}

	_ = r.Close()
	_ = collect()
	_ = log.Close()
}

// TestTerminate_SIGTERMSentBeforeSIGKILL uses a subprocess that traps SIGTERM
// and writes a marker line before exiting, confirming SIGTERM arrives first.
func TestTerminate_SIGTERMSentBeforeSIGKILL(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	script := `trap 'echo received-sigterm; exit 0' TERM; while true; do sleep 0.05; done`

	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunStep("sigterm-step", []string{"sh", "-c", script})
	}()

	time.Sleep(100 * time.Millisecond)
	r.Terminate()

	select {
	case <-stepDone:
	case <-time.After(5 * time.Second):
		t.Fatal("RunStep did not return within 5 seconds after Terminate")
	}

	_ = r.Close()
	lines := collect()
	_ = log.Close()

	found := false
	for _, l := range lines {
		if l == "received-sigterm" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'received-sigterm' in output (SIGTERM sent first), got %v", lines)
	}
}

// TestTerminate_SIGKILLFallbackWhenSIGTERMIgnored starts a subprocess that
// traps and ignores SIGTERM, calls Terminate(), and verifies RunStep returns
// within 5 seconds (SIGKILL fires after the 3-second timeout).
func TestTerminate_SIGKILLFallbackWhenSIGTERMIgnored(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	// trap '' TERM ignores SIGTERM entirely; the process will only die via SIGKILL.
	script := `trap '' TERM; while true; do sleep 0.1; done`

	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunStep("sigkill-step", []string{"sh", "-c", script})
	}()

	time.Sleep(100 * time.Millisecond)
	r.Terminate()

	select {
	case <-stepDone:
		// RunStep returned — SIGKILL fired after 3s timeout
	case <-time.After(5 * time.Second):
		t.Fatal("RunStep did not return within 5 seconds; SIGKILL fallback may be broken")
	}

	_ = r.Close()
	_ = collect()
	_ = log.Close()
}

// TestTerminate_NoOpWhenNoSubprocessRunning verifies that calling Terminate()
// when no subprocess is running does not panic and returns without blocking.
func TestTerminate_NoOpWhenNoSubprocessRunning(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	// Should not panic and should return immediately.
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Terminate()
	}()

	select {
	case <-done:
		// returned cleanly
	case <-time.After(1 * time.Second):
		t.Fatal("Terminate() blocked when no subprocess was running")
	}

	_ = r.Close()
	_ = collect()
	_ = log.Close()
}

// WasTerminated tests

// Gap 6 — WasTerminated returns false on a fresh Runner before any RunStep call.
func TestWasTerminated_FalseOnFreshRunner(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	if r.WasTerminated() {
		t.Error("WasTerminated should be false on a freshly constructed Runner")
	}

	_ = r.Close()
	_ = collect()
	_ = log.Close()
}

// TestWasTerminated_FalseBeforeTerminate verifies the flag is false when
// Terminate has not been called.
func TestWasTerminated_FalseBeforeTerminate(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	if err := r.RunStep("step", []string{"echo", "ok"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = r.Close()
	_ = collect()
	_ = log.Close()

	if r.WasTerminated() {
		t.Error("WasTerminated should be false for a step that exited normally")
	}
}

// TestWasTerminated_TrueAfterTerminate verifies the flag is true when
// Terminate() is called while a step is running.
func TestWasTerminated_TrueAfterTerminate(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunStep("long-step", []string{"sleep", "60"})
	}()

	time.Sleep(50 * time.Millisecond)
	r.Terminate()

	select {
	case <-stepDone:
	case <-time.After(5 * time.Second):
		t.Fatal("RunStep did not return after Terminate")
	}

	if !r.WasTerminated() {
		t.Error("WasTerminated should be true after Terminate was called")
	}

	_ = r.Close()
	_ = collect()
	_ = log.Close()
}

// TestWasTerminated_ResetOnNextRunStep verifies the flag is reset when the
// next RunStep begins.
func TestWasTerminated_ResetOnNextRunStep(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunStep("long-step", []string{"sleep", "60"})
	}()

	time.Sleep(50 * time.Millisecond)
	r.Terminate()
	<-stepDone

	// Start a normal step — should reset the flag.
	if err := r.RunStep("next-step", []string{"echo", "ok"}); err != nil {
		t.Fatalf("RunStep next-step: %v", err)
	}

	if r.WasTerminated() {
		t.Error("WasTerminated should be false after a normal step follows a terminated one")
	}

	_ = r.Close()
	_ = collect()
	_ = log.Close()
}

// TestTerminate_AfterSubprocessAlreadyExited runs a fast command, waits for it
// to finish, then calls Terminate() and verifies no panic and no hang.
func TestTerminate_AfterSubprocessAlreadyExited(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	if err := r.RunStep("fast-step", []string{"echo", "done"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

	// The subprocess has already exited; Terminate should be safe to call.
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Terminate()
	}()

	select {
	case <-done:
		// returned cleanly
	case <-time.After(1 * time.Second):
		t.Fatal("Terminate() blocked after subprocess already exited")
	}

	_ = r.Close()
	_ = collect()
	_ = log.Close()
}

// WriteToLog tests

// T1 — WriteToLog line appears in pipe output
func TestWriteToLog_LineAppearsInPipe(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	r.WriteToLog("injected line")
	_ = r.Close()

	lines := collect()
	_ = log.Close()

	found := false
	for _, l := range lines {
		if l == "injected line" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'injected line' in pipe output, got %v", lines)
	}
}

// T2 — WriteToLog interleaves correctly with RunStep output
func TestWriteToLog_InterleaveWithRunStep(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	if err := r.RunStep("step-before", []string{"echo", "before"}); err != nil {
		t.Fatalf("RunStep before: %v", err)
	}
	r.WriteToLog("--- separator ---")
	if err := r.RunStep("step-after", []string{"echo", "after"}); err != nil {
		t.Fatalf("RunStep after: %v", err)
	}
	_ = r.Close()

	lines := collect()
	_ = log.Close()

	foundBefore, foundSep, foundAfter := false, false, false
	for _, l := range lines {
		switch l {
		case "before":
			foundBefore = true
		case "--- separator ---":
			foundSep = true
		case "after":
			foundAfter = true
		}
	}
	if !foundBefore {
		t.Errorf("expected 'before' in pipe output, got %v", lines)
	}
	if !foundSep {
		t.Errorf("expected '--- separator ---' in pipe output, got %v", lines)
	}
	if !foundAfter {
		t.Errorf("expected 'after' in pipe output, got %v", lines)
	}
}

// T3 — WriteToLog with empty string writes a blank line (no panic, no no-op)
func TestWriteToLog_EmptyString(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	r.WriteToLog("before")
	r.WriteToLog("")
	r.WriteToLog("after")
	_ = r.Close()

	lines := collect()
	_ = log.Close()

	// Expect three lines: "before", "", "after"
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (including blank), got %d: %v", len(lines), lines)
	}
	if lines[1] != "" {
		t.Errorf("expected blank line at index 1, got %q", lines[1])
	}
}

// T6 — WriteToLog after Close does not panic
func TestWriteToLog_AfterCloseNoPanic(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	_ = r.Close()
	_ = collect()
	_ = log.Close()

	// Should not panic; write error is silently discarded.
	r.WriteToLog("late line")
}

// TestTerminate_IntegrationOrchestrationCanProceed terminates a step mid-stream
// and verifies the orchestration can proceed to the next step without hanging.
func TestTerminate_IntegrationOrchestrationCanProceed(t *testing.T) {
	r, log := newTestRunner(t)
	collect := collectLines(t, r)

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- r.RunStep("long-step", []string{"sh", "-c", "while true; do echo streaming; sleep 0.05; done"})
	}()

	time.Sleep(100 * time.Millisecond)
	r.Terminate()

	select {
	case <-firstDone:
	case <-time.After(5 * time.Second):
		t.Fatal("first step did not return after Terminate")
	}

	// Proceed to a subsequent step — must not hang.
	nextDone := make(chan error, 1)
	go func() {
		nextDone <- r.RunStep("next-step", []string{"echo", "next step ran"})
	}()

	select {
	case <-nextDone:
	case <-time.After(5 * time.Second):
		t.Fatal("next step did not return; orchestration is stuck after Terminate")
	}

	_ = r.Close()
	lines := collect()
	_ = log.Close()

	found := false
	for _, l := range lines {
		if l == "next step ran" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'next step ran' in output after termination, got %v", lines)
	}
}

// CaptureOutput tests

// T7 — capture stdout returns trimmed output
func TestCaptureOutput_CapturesStdout(t *testing.T) {
	out, err := CaptureOutput(context.Background(), []string{"echo", "hello"}, t.TempDir())
	if err != nil {
		t.Fatalf("CaptureOutput: %v", err)
	}
	if out != "hello" {
		t.Errorf("expected %q, got %q", "hello", out)
	}
}

// T8 — capture trims whitespace (trailing newlines etc.)
func TestCaptureOutput_TrimsWhitespace(t *testing.T) {
	out, err := CaptureOutput(context.Background(), []string{"sh", "-c", "printf 'hello\\n\\n'"}, t.TempDir())
	if err != nil {
		t.Fatalf("CaptureOutput: %v", err)
	}
	if out != "hello" {
		t.Errorf("expected %q after trimming, got %q", "hello", out)
	}
}

// T9 — non-zero exit returns error and empty output
func TestCaptureOutput_NonZeroExitReturnsError(t *testing.T) {
	out, err := CaptureOutput(context.Background(), []string{"false"}, t.TempDir())
	if err == nil {
		t.Fatal("expected error from non-zero exit, got nil")
	}
	if out != "" {
		t.Errorf("expected empty output on error, got %q", out)
	}
}

// T10 — empty output from command returns "" with no error
func TestCaptureOutput_EmptyOutputNoError(t *testing.T) {
	out, err := CaptureOutput(context.Background(), []string{"true"}, t.TempDir())
	if err != nil {
		t.Fatalf("CaptureOutput: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty string, got %q", out)
	}
}

// T11 — command runs in the specified directory
func TestCaptureOutput_UsesSpecifiedDir(t *testing.T) {
	dir := t.TempDir()
	out, err := CaptureOutput(context.Background(), []string{"pwd"}, dir)
	if err != nil {
		t.Fatalf("CaptureOutput: %v", err)
	}
	// t.TempDir() may return a symlink; resolve both sides for comparison.
	wantResolved, _ := filepath.EvalSymlinks(dir)
	gotResolved, _ := filepath.EvalSymlinks(out)
	if gotResolved != wantResolved {
		t.Errorf("expected pwd %q, got %q", wantResolved, gotResolved)
	}
}
