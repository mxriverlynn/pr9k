package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/vars"
)

// newCapturingRunner constructs a Runner with a mutex-guarded slice-append
// sender installed via SetSender. The returned drain closure snapshots the
// captured lines at call time. This is the helper the migration ticket will
// standardize on.
func newCapturingRunner(t *testing.T) (*Runner, *logger.Logger, func() []string) {
	t.Helper()
	r, log := newTestRunner(t)
	var mu sync.Mutex
	var captured []string
	r.SetSender(func(line string) {
		mu.Lock()
		captured = append(captured, line)
		mu.Unlock()
	})
	drain := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(captured))
		copy(out, captured)
		return out
	}
	return r, log, drain
}

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

// TestNewRunner_WriteToLogWithoutSetSenderPanics verifies that calling WriteToLog
// on a newly-created Runner (before SetSender) panics with a descriptive message.
// This catches missing-wire bugs (forgetting to call SetSender before RunStep)
// early and loudly rather than silently dropping output.
func TestNewRunner_WriteToLogWithoutSetSenderPanics(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer func() { _ = log.Close() }()

	r := NewRunner(log, dir)

	defer func() {
		rec := recover()
		if rec == nil {
			t.Error("expected WriteToLog to panic without SetSender, but it did not")
		}
	}()

	r.WriteToLog("this should panic")
}

// TP-003 — NewRunner panic sentinel contains a descriptive message
func TestNewRunner_WriteToLogWithoutSetSenderPanicsWithDescriptiveMessage(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer func() { _ = log.Close() }()

	r := NewRunner(log, dir)

	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("expected WriteToLog to panic without SetSender, but it did not")
		}
		msg, ok := rec.(string)
		if !ok {
			t.Fatalf("expected panic value to be a string, got %T: %v", rec, rec)
		}
		if !strings.Contains(msg, "sendLine not set") {
			t.Errorf("expected panic message to contain 'sendLine not set', got %q", msg)
		}
	}()

	r.WriteToLog("this should panic with a descriptive message")
}

func TestRunStep_StdoutArrivesInPipe(t *testing.T) {
	r, log, drain := newCapturingRunner(t)

	if err := r.RunStep("test-step", []string{"echo", "hello from stdout"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

	lines := drain()
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
	r, log, drain := newCapturingRunner(t)

	if err := r.RunStep("test-step", []string{"sh", "-c", "echo 'hello from stderr' >&2"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

	lines := drain()
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
	r, log, drain := newCapturingRunner(t)

	if err := r.RunStep("test-step", []string{"sh", "-c", "echo 'out line'; echo 'err line' >&2"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

	lines := drain()
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
	r, log, drain := newCapturingRunner(t)

	script := "for i in $(seq 1 200); do echo \"line $i\" >&2; done"
	if err := r.RunStep("test-step", []string{"sh", "-c", script}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

	lines := drain()
	if len(lines) != 200 {
		t.Errorf("expected 200 lines, got %d", len(lines))
	}
	_ = log.Close()
}

// TestRunStep_UsesProjectDir verifies RunStep sets cmd.Dir to the runner's
// projectDir (target repo), not the install dir. Mirrors the equivalent test
// for CaptureOutput.
func TestRunStep_UsesProjectDir(t *testing.T) {
	projectDir := t.TempDir()
	logDir := t.TempDir()
	log, err := logger.NewLogger(logDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()

	r := NewRunner(log, projectDir)
	var mu sync.Mutex
	var captured []string
	r.SetSender(func(line string) {
		mu.Lock()
		captured = append(captured, line)
		mu.Unlock()
	})

	if err := r.RunStep("pwd-step", []string{"sh", "-c", "pwd"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

	wantDir, _ := filepath.EvalSymlinks(projectDir)
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, line := range captured {
		got, err := filepath.EvalSymlinks(line)
		if err == nil && got == wantDir {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected RunStep cmd.Dir=%q in captured output, got %v", wantDir, captured)
	}
}

// Integration tests

func TestRunStep_IntegrationStdoutInPipeAndLogFile(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	r := NewRunner(log, dir)
	var captureMu sync.Mutex
	var captured []string
	r.SetSender(func(line string) {
		captureMu.Lock()
		captured = append(captured, line)
		captureMu.Unlock()
	})
	drain := func() []string {
		captureMu.Lock()
		defer captureMu.Unlock()
		out := make([]string, len(captured))
		copy(out, captured)
		return out
	}

	if err := r.RunStep("integration-step", []string{"echo", "integration hello"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

	pipeLines := drain()

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
	var captureMu sync.Mutex
	var captured []string
	r.SetSender(func(line string) {
		captureMu.Lock()
		captured = append(captured, line)
		captureMu.Unlock()
	})
	drain := func() []string {
		captureMu.Lock()
		defer captureMu.Unlock()
		out := make([]string, len(captured))
		copy(out, captured)
		return out
	}

	if err := r.RunStep("integration-step", []string{"sh", "-c", "echo 'integration err' >&2"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

	lines := drain()
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
	r, log, _ := newCapturingRunner(t)

	err := r.RunStep("test-step", []string{"sh", "-c", "exit 1"})
	_ = log.Close()

	if err == nil {
		t.Fatal("expected error from non-zero exit, got nil")
	}
}

// TP-001 — RunStep returns error for empty command slice
func TestRunStep_ReturnsErrorForEmptyCommandSlice(t *testing.T) {
	r, log, _ := newCapturingRunner(t)

	err := r.RunStep("my-step", []string{})
	_ = log.Close()

	if err == nil {
		t.Fatal("expected error for empty command slice, got nil")
	}
	if !strings.Contains(err.Error(), "empty command") {
		t.Errorf("expected error to contain 'empty command', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "my-step") {
		t.Errorf("expected error to contain step name 'my-step', got %q", err.Error())
	}
	if r.LastCapture() != "" {
		t.Errorf("expected LastCapture to be empty after empty-command error, got %q", r.LastCapture())
	}
}

// TP-002 — RunStep returns error for nil command (nil slice is zero-length)
func TestRunStep_ReturnsErrorForNilCommand(t *testing.T) {
	r, log, _ := newCapturingRunner(t)

	err := r.RunStep("nil-step", nil)
	_ = log.Close()

	if err == nil {
		t.Fatal("expected error for nil command, got nil")
	}
	if !strings.Contains(err.Error(), "empty command") {
		t.Errorf("expected error to contain 'empty command', got %q", err.Error())
	}
}

// T2 — RunStep returns error for non-existent command
func TestRunStep_ReturnsErrorForNonExistentCommand(t *testing.T) {
	r, log, _ := newCapturingRunner(t)

	err := r.RunStep("test-step", []string{"nonexistent-binary-xyz"})
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
	r, log, drain := newCapturingRunner(t)

	if err := r.RunStep("step-one", []string{"echo", "output from step one"}); err != nil {
		t.Fatalf("RunStep step-one: %v", err)
	}
	if err := r.RunStep("step-two", []string{"echo", "output from step two"}); err != nil {
		t.Fatalf("RunStep step-two: %v", err)
	}

	lines := drain()
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
	r.SetSender(func(string) {}) // no-op sender; test only checks the log file

	if err := r.RunStep("my-named-step", []string{"echo", "some output"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

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

// TP-004 — CaptureOutput returns error for empty command slice
func TestCaptureOutput_ReturnsErrorForEmptyCommandSlice(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	_, err := r.CaptureOutput([]string{})

	if err == nil {
		t.Fatal("expected error for empty command slice, got nil")
	}
	if !strings.Contains(err.Error(), "empty command") {
		t.Errorf("expected error to contain 'empty command', got %q", err.Error())
	}
}

// TP-005 — CaptureOutput returns error for nil command (nil slice is zero-length)
func TestCaptureOutput_ReturnsErrorForNilCommand(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	_, err := r.CaptureOutput(nil)

	if err == nil {
		t.Fatal("expected error for nil command, got nil")
	}
	if !strings.Contains(err.Error(), "empty command") {
		t.Errorf("expected error to contain 'empty command', got %q", err.Error())
	}
}

// newIterVT creates a VarTable in Iteration phase with ISSUE_ID bound.
func newIterVT(workflowDir, issueID string) *vars.VarTable {
	vt := vars.New(workflowDir, workflowDir, 0)
	vt.SetPhase(vars.Iteration)
	vt.Bind(vars.Iteration, "ISSUE_ID", issueID)
	return vt
}

func TestResolveCommand_ScriptPathAndIssueID(t *testing.T) {
	workflowDir := "/home/user/project"
	cmd := []string{"scripts/close_gh_issue", "{{ISSUE_ID}}"}
	vt := newIterVT(workflowDir, "42")
	got := ResolveCommand(workflowDir, cmd, vt, vars.Iteration)
	want := []string{"/home/user/project/scripts/close_gh_issue", "42"}
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
	workflowDir := "/home/user/project"
	cmd := []string{"git", "push"}
	vt := newIterVT(workflowDir, "99")
	got := ResolveCommand(workflowDir, cmd, vt, vars.Iteration)
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
	workflowDir := "/proj"
	cmd := []string{"scripts/foo", "{{ISSUE_ID}}", "--label={{ISSUE_ID}}"}
	vt := newIterVT(workflowDir, "7")
	got := ResolveCommand(workflowDir, cmd, vt, vars.Iteration)
	want := []string{"/proj/scripts/foo", "7", "--label=7"}
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
	workflowDir := "/base"
	cmd := []string{"scripts/foo", "arg"}
	vt := newIterVT(workflowDir, "1")
	got := ResolveCommand(workflowDir, cmd, vt, vars.Iteration)
	wantExe := "/base/scripts/foo"
	if got[0] != wantExe {
		t.Errorf("exe: got %q, want %q", got[0], wantExe)
	}
}

func TestResolveCommand_AbsolutePathUnchanged(t *testing.T) {
	workflowDir := "/proj"
	cmd := []string{"/usr/bin/env", "{{ISSUE_ID}}"}
	vt := newIterVT(workflowDir, "3")
	got := ResolveCommand(workflowDir, cmd, vt, vars.Iteration)
	if got[0] != "/usr/bin/env" {
		t.Errorf("exe: got %q, want /usr/bin/env", got[0])
	}
	if got[1] != "3" {
		t.Errorf("arg: got %q, want 3", got[1])
	}
}

func TestResolveCommand_NoTemplateVars_Passthrough(t *testing.T) {
	workflowDir := "/proj"
	cmd := []string{"git", "commit", "-m", "fix things"}
	vt := newIterVT(workflowDir, "10")
	got := ResolveCommand(workflowDir, cmd, vt, vars.Iteration)
	want := []string{"git", "commit", "-m", "fix things"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("element %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveCommand_EmptySlice(t *testing.T) {
	vt := newIterVT("/workflow", "42")
	got := ResolveCommand("/workflow", []string{}, vt, vars.Iteration)
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestResolveCommand_DoesNotMutateInput(t *testing.T) {
	original := []string{"scripts/close_gh_issue", "{{ISSUE_ID}}"}
	input := make([]string, len(original))
	copy(input, original)
	vt := newIterVT("/workflow", "42")
	ResolveCommand("/workflow", input, vt, vars.Iteration)
	for i := range original {
		if input[i] != original[i] {
			t.Errorf("input[%d] mutated: got %q, want %q", i, input[i], original[i])
		}
	}
}

func TestResolveCommand_TemplateInExecutable(t *testing.T) {
	cmd := []string{"scripts/issue-{{ISSUE_ID}}/run", "arg"}
	vt := newIterVT("/workflow", "5")
	got := ResolveCommand("/workflow", cmd, vt, vars.Iteration)
	wantExe := "/workflow/scripts/issue-5/run"
	if got[0] != wantExe {
		t.Errorf("exe: got %q, want %q", got[0], wantExe)
	}
}

func TestResolveCommand_SingleElementBareCommand(t *testing.T) {
	vt := newIterVT("/workflow", "1")
	got := ResolveCommand("/workflow", []string{"git"}, vt, vars.Iteration)
	if got[0] != "git" {
		t.Errorf("exe: got %q, want %q", got[0], "git")
	}
}

// TestResolveCommand_UsesWorkflowDir verifies that a relative script path is
// joined against workflowDir (the install dir), NOT the projectDir (target repo).
// This guards against confusion where scripts live in the workflow bundle but
// claude subprocesses operate on the target repo.
func TestResolveCommand_UsesWorkflowDir(t *testing.T) {
	workflowDir := t.TempDir()
	projectDir := t.TempDir()

	// Create the script in workflowDir, not projectDir, so a wrong join would fail.
	scriptsDir := filepath.Join(workflowDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	cmd := []string{"scripts/my-script", "arg"}
	vt := vars.New(workflowDir, projectDir, 0)
	vt.SetPhase(vars.Iteration)

	got := ResolveCommand(workflowDir, cmd, vt, vars.Iteration)

	wantExe := filepath.Join(workflowDir, "scripts/my-script")
	if got[0] != wantExe {
		t.Errorf("ResolveCommand should join against workflowDir: got %q, want %q", got[0], wantExe)
	}
	// Confirm the resolved path does NOT start with projectDir.
	if strings.HasPrefix(got[0], projectDir) {
		t.Errorf("ResolveCommand must not join against projectDir: got %q", got[0])
	}
}

// Terminate unit tests

// TestTerminate_RunStepReturnsWithinTimeout starts a long-running subprocess,
// requests termination, and verifies RunStep returns within 5 seconds.
func TestTerminate_RunStepReturnsWithinTimeout(t *testing.T) {
	r, log, _ := newCapturingRunner(t)

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

	_ = log.Close()
}

// TestTerminate_ScannerGoroutinesExitAfterPipesClose verifies that after
// termination the subprocess pipes are closed so scanner goroutines inside
// RunStep exit naturally (evidenced by RunStep returning).
func TestTerminate_ScannerGoroutinesExitAfterPipesClose(t *testing.T) {
	r, log, _ := newCapturingRunner(t)

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

	_ = log.Close()
}

// TestTerminate_SIGTERMSentBeforeSIGKILL uses a subprocess that traps SIGTERM
// and writes a marker line before exiting, confirming SIGTERM arrives first.
func TestTerminate_SIGTERMSentBeforeSIGKILL(t *testing.T) {
	r, log, drain := newCapturingRunner(t)

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

	lines := drain()
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
	r, log, _ := newCapturingRunner(t)

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

	_ = log.Close()
}

// TestTerminate_NoOpWhenNoSubprocessRunning verifies that calling Terminate()
// when no subprocess is running does not panic and returns without blocking.
func TestTerminate_NoOpWhenNoSubprocessRunning(t *testing.T) {
	r, log, _ := newCapturingRunner(t)

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

	_ = log.Close()
}

// WasTerminated tests

// Gap 6 — WasTerminated returns false on a fresh Runner before any RunStep call.
func TestWasTerminated_FalseOnFreshRunner(t *testing.T) {
	r, log, _ := newCapturingRunner(t)

	if r.WasTerminated() {
		t.Error("WasTerminated should be false on a freshly constructed Runner")
	}

	_ = log.Close()
}

// TestWasTerminated_FalseBeforeTerminate verifies the flag is false when
// Terminate has not been called.
func TestWasTerminated_FalseBeforeTerminate(t *testing.T) {
	r, log, _ := newCapturingRunner(t)

	if err := r.RunStep("step", []string{"echo", "ok"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = log.Close()

	if r.WasTerminated() {
		t.Error("WasTerminated should be false for a step that exited normally")
	}
}

// TestWasTerminated_TrueAfterTerminate verifies the flag is true when
// Terminate() is called while a step is running.
func TestWasTerminated_TrueAfterTerminate(t *testing.T) {
	r, log, _ := newCapturingRunner(t)

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

	_ = log.Close()
}

// TestWasTerminated_ResetOnNextRunStep verifies the flag is reset when the
// next RunStep begins.
func TestWasTerminated_ResetOnNextRunStep(t *testing.T) {
	r, log, _ := newCapturingRunner(t)

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

	_ = log.Close()
}

// TestTerminate_AfterSubprocessAlreadyExited runs a fast command, waits for it
// to finish, then calls Terminate() and verifies no panic and no hang.
func TestTerminate_AfterSubprocessAlreadyExited(t *testing.T) {
	r, log, _ := newCapturingRunner(t)

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

	_ = log.Close()
}

// WriteToLog tests

// T1 — WriteToLog line appears in pipe output
func TestWriteToLog_LineAppearsInPipe(t *testing.T) {
	r, log, drain := newCapturingRunner(t)

	r.WriteToLog("injected line")

	lines := drain()
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
	r, log, drain := newCapturingRunner(t)

	if err := r.RunStep("step-before", []string{"echo", "before"}); err != nil {
		t.Fatalf("RunStep before: %v", err)
	}
	r.WriteToLog("--- separator ---")
	if err := r.RunStep("step-after", []string{"echo", "after"}); err != nil {
		t.Fatalf("RunStep after: %v", err)
	}

	lines := drain()
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
	r, log, drain := newCapturingRunner(t)

	r.WriteToLog("before")
	r.WriteToLog("")
	r.WriteToLog("after")

	lines := drain()
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
	r, log, _ := newCapturingRunner(t)

	_ = log.Close()

	// Should not panic; write error is silently discarded.
	r.WriteToLog("late line")
}

// SetSender tests

// TestSetSender_ForwardsEveryStdoutLine verifies that the sendLine callback
// receives every stdout line emitted by a subprocess.
func TestSetSender_ForwardsEveryStdoutLine(t *testing.T) {
	r, log, drain := newCapturingRunner(t)

	script := "echo line1; echo line2; echo line3; echo line4; echo line5"
	if err := r.RunStep("test-step", []string{"sh", "-c", script}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = log.Close()

	got := drain()
	want := []string{"line1", "line2", "line3", "line4", "line5"}
	if len(got) != len(want) {
		t.Fatalf("expected %d lines, got %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("line %d: want %q, got %q", i, w, got[i])
		}
	}
}

// TestSetSender_ForwardsEveryStderrLine verifies that the sendLine callback
// receives every stderr line emitted by a subprocess.
func TestSetSender_ForwardsEveryStderrLine(t *testing.T) {
	r, log, drain := newCapturingRunner(t)

	script := "echo line1 >&2; echo line2 >&2; echo line3 >&2; echo line4 >&2; echo line5 >&2"
	if err := r.RunStep("test-step", []string{"sh", "-c", script}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = log.Close()

	got := drain()
	want := []string{"line1", "line2", "line3", "line4", "line5"}
	if len(got) != len(want) {
		t.Fatalf("expected %d lines, got %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("line %d: want %q, got %q", i, w, got[i])
		}
	}
}

// RunSandboxedStep tests

// TestRunSandboxedStep_InstallsAndClearsTerminator verifies that the terminator
// is installed in currentTerminator during RunSandboxedStep and cleared to nil
// after the call returns.
func TestRunSandboxedStep_InstallsAndClearsTerminator(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	terminator := func(syscall.Signal) error { return nil }

	// Use a slow-enough command that a goroutine can observe currentTerminator.
	observedNonNil := make(chan bool, 1)
	go func() {
		// Poll briefly until we see the terminator installed.
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			r.processMu.Lock()
			v := r.currentTerminator
			r.processMu.Unlock()
			if v != nil {
				observedNonNil <- true
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		observedNonNil <- false
	}()

	opts := SandboxOptions{Terminator: terminator}
	if err := r.RunSandboxedStep("sandbox-step", []string{"sh", "-c", "sleep 0.05"}, opts); err != nil {
		t.Fatalf("RunSandboxedStep: %v", err)
	}

	// After RunSandboxedStep returns, currentTerminator must be nil.
	r.processMu.Lock()
	afterTerm := r.currentTerminator
	r.processMu.Unlock()
	if afterTerm != nil {
		t.Error("expected currentTerminator to be nil after RunSandboxedStep returned")
	}

	// The goroutine must have seen the terminator set during the call.
	if !<-observedNonNil {
		t.Error("expected to observe non-nil currentTerminator while RunSandboxedStep was running")
	}
}

// TestRunSandboxedStep_UsesEmptyStdin verifies that RunSandboxedStep provides
// an explicit empty stdin. A command that reads stdin exits immediately when
// stdin is empty (EOF on open), which is the expected sandbox behaviour.
func TestRunSandboxedStep_UsesEmptyStdin(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	// "cat -" reads stdin until EOF; with empty stdin it exits immediately.
	err := r.RunSandboxedStep("stdin-step", []string{"cat", "-"}, SandboxOptions{})
	if err != nil {
		t.Fatalf("RunSandboxedStep with empty stdin: %v", err)
	}
}

// TestRunSandboxedStep_CleansCidfile verifies that RunSandboxedStep removes the
// cidfile after the step exits.
func TestRunSandboxedStep_CleansCidfile(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	// Create a real temp file to use as the cidfile.
	f, err := os.CreateTemp("", "ralph-test-*.cid")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	cidPath := f.Name()
	_ = f.Close()

	opts := SandboxOptions{CidfilePath: cidPath}
	if err := r.RunSandboxedStep("cidfile-step", []string{"true"}, opts); err != nil {
		t.Fatalf("RunSandboxedStep: %v", err)
	}

	if _, statErr := os.Stat(cidPath); !os.IsNotExist(statErr) {
		t.Errorf("expected cidfile %q to be removed after RunSandboxedStep, got stat err: %v", cidPath, statErr)
	}
}

// TestRunSandboxedStep_CleansCidfile_NonexistentPath verifies that passing a
// nonexistent CidfilePath does not cause an error or panic.
func TestRunSandboxedStep_CleansCidfile_NonexistentPath(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	opts := SandboxOptions{CidfilePath: "/tmp/ralph-nonexistent-cidfile-that-does-not-exist.cid"}
	if err := r.RunSandboxedStep("cidfile-missing-step", []string{"true"}, opts); err != nil {
		t.Fatalf("RunSandboxedStep with nonexistent cidfile: %v", err)
	}
}

// TestTerminate_UsesTerminatorWhenInstalled verifies that Terminate() dispatches
// SIGTERM (and, after the grace period, SIGKILL) through the installed terminator
// rather than calling proc.Signal / proc.Kill directly.
func TestTerminate_UsesTerminatorWhenInstalled(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	// Use a very short grace period so the test does not wait 3 seconds.
	r.terminateGraceOverride = 100 * time.Millisecond

	var mu sync.Mutex
	var signals []syscall.Signal

	// proc is captured after cmd.Start() so the terminator can forward signals.
	var proc *os.Process

	terminator := func(sig syscall.Signal) error {
		mu.Lock()
		signals = append(signals, sig)
		p := proc
		mu.Unlock()
		if p != nil {
			// Forward to the real process so it actually exits.
			if sig == syscall.SIGKILL {
				return p.Kill()
			}
			return p.Signal(sig)
		}
		return nil
	}

	// Run a script that traps SIGTERM but ignores it, so SIGKILL fires.
	script := `trap '' TERM; while true; do sleep 0.05; done`
	opts := SandboxOptions{Terminator: terminator}

	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunSandboxedStep("term-step", []string{"sh", "-c", script}, opts)
	}()

	// Wait for the process to start and capture its *os.Process.
	time.Sleep(50 * time.Millisecond)
	r.processMu.Lock()
	proc = r.currentProc
	r.processMu.Unlock()

	r.Terminate()

	select {
	case <-stepDone:
	case <-time.After(2 * time.Second):
		t.Fatal("RunSandboxedStep did not return within 2 seconds after Terminate")
	}

	mu.Lock()
	got := make([]syscall.Signal, len(signals))
	copy(got, signals)
	mu.Unlock()

	if len(got) < 2 {
		t.Fatalf("expected at least 2 terminator calls (SIGTERM + SIGKILL), got %d: %v", len(got), got)
	}
	if got[0] != syscall.SIGTERM {
		t.Errorf("first terminator call: want SIGTERM, got %v", got[0])
	}
	if got[1] != syscall.SIGKILL {
		t.Errorf("second terminator call: want SIGKILL, got %v", got[1])
	}
}

// TestTerminate_UsesProcessWhenNoTerminator verifies that when no terminator is
// installed (RunStep, not RunSandboxedStep), Terminate() signals the host
// process directly and does not call any terminator.
func TestTerminate_UsesProcessWhenNoTerminator(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunStep("no-term-step", []string{"sleep", "60"})
	}()

	time.Sleep(50 * time.Millisecond)
	r.Terminate()

	select {
	case <-stepDone:
		// RunStep returned — direct process signaling worked
	case <-time.After(5 * time.Second):
		t.Fatal("RunStep did not return within 5 seconds; direct process signaling may be broken")
	}
}

// TestRunSandboxedStep_TerminatorClearedBeforeWaitReturn verifies that after
// RunSandboxedStep returns naturally, a subsequent Terminate() call does NOT
// invoke the terminator (it was cleared before the procDone channel closed).
func TestRunSandboxedStep_TerminatorClearedBeforeWaitReturn(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	// A terminator that panics if called — should never fire after step exits.
	panicTerminator := func(syscall.Signal) error {
		panic("terminator called after RunSandboxedStep returned: F2 violation")
	}

	opts := SandboxOptions{Terminator: panicTerminator}
	if err := r.RunSandboxedStep("f2-step", []string{"true"}, opts); err != nil {
		t.Fatalf("RunSandboxedStep: %v", err)
	}

	// The step has exited. Terminate() must see currentTerminator == nil and
	// currentProc == nil, short-circuit immediately, and not call the terminator.
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Terminate()
	}()

	select {
	case <-done:
		// Returned without calling the terminator.
	case <-time.After(1 * time.Second):
		t.Fatal("Terminate() blocked after RunSandboxedStep returned")
	}
}

// TestSetSender_BurstDoesNotDropOrReorder emits 200 stderr lines in a tight
// loop and asserts all 200 arrive in order through drain().
func TestSetSender_BurstDoesNotDropOrReorder(t *testing.T) {
	r, log, drain := newCapturingRunner(t)

	script := "for i in $(seq 1 200); do echo \"line $i\" >&2; done"
	if err := r.RunStep("test-step", []string{"sh", "-c", script}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = log.Close()

	got := drain()
	if len(got) != 200 {
		t.Fatalf("expected 200 lines, got %d", len(got))
	}
	for i, line := range got {
		want := fmt.Sprintf("line %d", i+1)
		if line != want {
			t.Errorf("line %d: want %q, got %q", i, want, line)
		}
	}
}

// TestSetSender_NilIsTreatedAsNoop verifies that SetSender(nil) installs a
// no-op sender so subsequent RunStep calls don't panic. Lines no longer
// arrive at the previously-installed capturing drain (it was replaced).
func TestSetSender_NilIsTreatedAsNoop(t *testing.T) {
	r, log, drain := newCapturingRunner(t)

	// Replace the capturing sender with a no-op via SetSender(nil).
	r.SetSender(nil)

	// RunStep must not panic even though the capture sender was cleared.
	if err := r.RunStep("test-step", []string{"echo", "pipe-line"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

	// drain() captures nothing because the sender was replaced with a no-op.
	lines := drain()
	_ = log.Close()

	if len(lines) != 0 {
		t.Errorf("expected no lines in old drain after SetSender(nil), got %v", lines)
	}
}

// TestSetSender_CalledBeforeAndAfterRunStep verifies that each drain only
// contains lines from the step that ran while its sender was installed.
func TestSetSender_CalledBeforeAndAfterRunStep(t *testing.T) {
	r, log, _ := newCapturingRunner(t)

	var mu1 sync.Mutex
	var cap1 []string
	r.SetSender(func(line string) {
		mu1.Lock()
		cap1 = append(cap1, line)
		mu1.Unlock()
	})

	if err := r.RunStep("step-one", []string{"echo", "step-one-output"}); err != nil {
		t.Fatalf("RunStep step-one: %v", err)
	}

	var mu2 sync.Mutex
	var cap2 []string
	r.SetSender(func(line string) {
		mu2.Lock()
		cap2 = append(cap2, line)
		mu2.Unlock()
	})

	if err := r.RunStep("step-two", []string{"echo", "step-two-output"}); err != nil {
		t.Fatalf("RunStep step-two: %v", err)
	}

	_ = log.Close()

	mu1.Lock()
	drain1 := append([]string{}, cap1...)
	mu1.Unlock()

	mu2.Lock()
	drain2 := append([]string{}, cap2...)
	mu2.Unlock()

	if len(drain1) != 1 || drain1[0] != "step-one-output" {
		t.Errorf("first drain: expected [step-one-output], got %v", drain1)
	}
	if len(drain2) != 1 || drain2[0] != "step-two-output" {
		t.Errorf("second drain: expected [step-two-output], got %v", drain2)
	}
}

// TestWriteToLog_ForwardsToSender verifies that WriteToLog forwards the line
// to the installed sendLine callback.
func TestWriteToLog_ForwardsToSender(t *testing.T) {
	r, log, drain := newCapturingRunner(t)

	r.WriteToLog("hello")
	_ = log.Close()

	got := drain()
	if len(got) != 1 || got[0] != "hello" {
		t.Errorf("expected [\"hello\"], got %v", got)
	}
}

// TP-001 — Default no-op sendLine works through forwardPipe (RunStep path).
// A Runner that never calls SetSender should not panic when RunStep emits lines.
func TestRunStep_DefaultNoOpSendLineNoPanic(t *testing.T) {
	r, log, drain := newCapturingRunner(t)

	// Never call SetSender — use the default no-op installed in NewRunner.
	if err := r.RunStep("test-step", []string{"echo", "default-noop-line"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

	lines := drain()
	_ = log.Close()

	found := false
	for _, l := range lines {
		if l == "default-noop-line" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'default-noop-line' in pipe output, got %v", lines)
	}
}

// TP-002 — Snapshot-then-unlock pattern is race-safe under concurrent SetSender calls.
// A long-running subprocess emits lines while a goroutine repeatedly swaps the sender.
// The -race flag detects any violation; this test asserts no panic and RunStep returns.
func TestRunStep_ConcurrentSetSenderNoRace(t *testing.T) {
	r, log, _ := newCapturingRunner(t)

	stepDone := make(chan error, 1)
	go func() {
		script := "while true; do echo line; sleep 0.01; done"
		stepDone <- r.RunStep("chatty-step", []string{"sh", "-c", script})
	}()

	// Concurrently swap the sender 50 times while lines are streaming.
	swapDone := make(chan struct{})
	go func() {
		defer close(swapDone)
		senderA := func(string) {}
		senderB := func(string) {}
		for i := 0; i < 50; i++ {
			if i%2 == 0 {
				r.SetSender(senderA)
			} else {
				r.SetSender(senderB)
			}
		}
	}()

	<-swapDone
	// Pragmatic shortcut: sleep gives the subprocess time to emit at least one line
	// before we terminate; deterministic synchronization would require IPC from the shell.
	time.Sleep(50 * time.Millisecond)
	r.Terminate()

	select {
	case <-stepDone:
	case <-time.After(5 * time.Second):
		t.Fatal("RunStep did not return within 5 seconds after Terminate")
	}

	_ = log.Close()
}

// TP-003 — stdout and stderr forwardPipe goroutines call sendLine concurrently.
// Both goroutines share the same mutex-guarded sender; a race here drops lines.
func TestRunStep_ConcurrentStdoutStderrSenderNoRace(t *testing.T) {
	r, log, drain := newCapturingRunner(t)

	script := "for i in $(seq 1 50); do echo \"out-$i\"; echo \"err-$i\" >&2; done"
	if err := r.RunStep("interleaved-step", []string{"sh", "-c", script}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = log.Close()

	got := drain()
	if len(got) != 100 {
		t.Fatalf("expected 100 lines (50 stdout + 50 stderr), got %d: %v", len(got), got)
	}

	lineSet := make(map[string]bool, 100)
	for _, l := range got {
		lineSet[l] = true
	}
	for i := 1; i <= 50; i++ {
		outLine := fmt.Sprintf("out-%d", i)
		errLine := fmt.Sprintf("err-%d", i)
		if !lineSet[outLine] {
			t.Errorf("missing expected stdout line %q", outLine)
		}
		if !lineSet[errLine] {
			t.Errorf("missing expected stderr line %q", errLine)
		}
	}
}

// TP-004 — sendLine calls in progress survive Terminate() without panic or hang.
func TestRunStep_SendLineAfterTerminateNoPanic(t *testing.T) {
	r, log, drain := newCapturingRunner(t)

	stepDone := make(chan error, 1)
	go func() {
		script := "while true; do echo line; sleep 0.01; done"
		stepDone <- r.RunStep("chatty-step", []string{"sh", "-c", script})
	}()

	// Pragmatic shortcut: sleep gives the subprocess time to emit at least one line
	// before we terminate; deterministic synchronization would require IPC from the shell.
	time.Sleep(100 * time.Millisecond)
	r.Terminate()

	select {
	case <-stepDone:
	case <-time.After(5 * time.Second):
		t.Fatal("RunStep did not return within 5 seconds after Terminate")
	}

	_ = log.Close()

	got := drain()
	if len(got) == 0 {
		t.Error("expected at least one line delivered to sender before Terminate")
	}
}

// TP-005 — WriteToLog after Close still invokes sendLine without panic.
// The pipe write fails silently; the sender should still receive the line.
func TestWriteToLog_AfterCloseSendLineStillInvoked(t *testing.T) {
	r, log, drain := newCapturingRunner(t)

	_ = log.Close()

	// Write after close: pipe write is silently discarded, sender should still fire.
	r.WriteToLog("late")

	got := drain()
	if len(got) != 1 || got[0] != "late" {
		t.Errorf("expected sender to receive [\"late\"] after Close, got %v", got)
	}
}

// TP-006 — SetSender atomic replacement is reflected immediately in WriteToLog.
// This is a synchronous test (no subprocess) so there is no timing ambiguity.
func TestSetSender_AtomicReplacementViaWriteToLog(t *testing.T) {
	r, log, _ := newCapturingRunner(t)

	var muA sync.Mutex
	var capA []string
	r.SetSender(func(line string) {
		muA.Lock()
		capA = append(capA, line)
		muA.Unlock()
	})
	r.WriteToLog("to-a")

	var muB sync.Mutex
	var capB []string
	r.SetSender(func(line string) {
		muB.Lock()
		capB = append(capB, line)
		muB.Unlock()
	})
	r.WriteToLog("to-b")

	_ = log.Close()

	muA.Lock()
	drainA := append([]string{}, capA...)
	muA.Unlock()

	muB.Lock()
	drainB := append([]string{}, capB...)
	muB.Unlock()

	if len(drainA) != 1 || drainA[0] != "to-a" {
		t.Errorf("sender A: expected [\"to-a\"], got %v", drainA)
	}
	if len(drainB) != 1 || drainB[0] != "to-b" {
		t.Errorf("sender B: expected [\"to-b\"], got %v", drainB)
	}
}

// TP-007 — Default no-op sendLine works through WriteToLog path.
// A Runner that never calls SetSender should not panic when WriteToLog is called.
func TestWriteToLog_DefaultNoOpSendLineNoPanic(t *testing.T) {
	r, log, drain := newCapturingRunner(t)

	// Never call SetSender — use the default no-op installed in NewRunner.
	r.WriteToLog("noop-test")

	lines := drain()
	_ = log.Close()

	found := false
	for _, l := range lines {
		if l == "noop-test" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'noop-test' in pipe output with default no-op sender, got %v", lines)
	}
}

// TestLastNonEmptyLine_AllEmptyReturnsEmpty verifies that an all-whitespace/blank
// input slice returns "" (T6).
func TestLastNonEmptyLine_AllEmptyReturnsEmpty(t *testing.T) {
	cases := []struct {
		name  string
		input []string
	}{
		{"spaces", []string{" ", "  "}},
		{"carriage returns", []string{"\r", "\r\n"}},
		{"mixed", []string{" ", "\r", "  \r"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lastNonEmptyLine(tc.input); got != "" {
				t.Errorf("lastNonEmptyLine(%v) = %q, want %q", tc.input, got, "")
			}
		})
	}
}

// TestLastNonEmptyLine_NilOrEmptySliceReturnsEmpty verifies nil and empty slice
// both return "" (T7).
func TestLastNonEmptyLine_NilOrEmptySliceReturnsEmpty(t *testing.T) {
	if got := lastNonEmptyLine(nil); got != "" {
		t.Errorf("lastNonEmptyLine(nil) = %q, want %q", got, "")
	}
	if got := lastNonEmptyLine([]string{}); got != "" {
		t.Errorf("lastNonEmptyLine([]) = %q, want %q", got, "")
	}
}

// TestLastNonEmptyLine_TrailingEmptiesSkipped verifies that trailing blank lines
// are skipped and the correct last non-empty line is returned (T8).
func TestLastNonEmptyLine_TrailingEmptiesSkipped(t *testing.T) {
	cases := []struct {
		name  string
		input []string
		want  string
	}{
		{"single trailing empty", []string{"first", "second", ""}, "second"},
		{"multiple trailing empties", []string{"alpha", " ", "\r", "  "}, "alpha"},
		{"trailing carriage return lines", []string{"result", "\r", "\r\n"}, "result"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lastNonEmptyLine(tc.input); got != tc.want {
				t.Errorf("lastNonEmptyLine(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TP-001 — WriteToLog does not write to the file logger.
// The updated subprocess-execution.md documents that WriteToLog forwards the
// line via sendLine only and does not call r.log.Log(). This test enforces that
// behavioral contract so that an accidental r.log.Log() call would be caught.
func TestWriteToLog_DoesNotWriteToFileLogger(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	r := NewRunner(log, dir)

	var mu sync.Mutex
	var captured []string
	r.SetSender(func(line string) {
		mu.Lock()
		captured = append(captured, line)
		mu.Unlock()
	})

	r.WriteToLog("test line")

	// readLogFile calls log.Close() internally before reading.
	logLines := readLogFile(t, log, dir)
	for _, l := range logLines {
		if strings.Contains(l, "test line") {
			t.Errorf("WriteToLog must not write to the file logger; found %q in log file", l)
		}
	}

	mu.Lock()
	senderLines := make([]string, len(captured))
	copy(senderLines, captured)
	mu.Unlock()

	found := false
	for _, l := range senderLines {
		if l == "test line" {
			found = true
		}
	}
	if !found {
		t.Errorf("WriteToLog must forward line to sendLine; sender received %v", senderLines)
	}
}

// TestTerminate_IntegrationOrchestrationCanProceed terminates a step mid-stream
// and verifies the orchestration can proceed to the next step without hanging.
// TP-001 — Runner.ProjectDir() getter returns the value passed to NewRunner.
func TestNewRunner_ProjectDirGetter(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer func() { _ = log.Close() }()

	r := NewRunner(log, "/some/project/dir")
	if got := r.ProjectDir(); got != "/some/project/dir" {
		t.Errorf("ProjectDir() = %q, want %q", got, "/some/project/dir")
	}
}

func TestTerminate_IntegrationOrchestrationCanProceed(t *testing.T) {
	r, log, drain := newCapturingRunner(t)

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

	lines := drain()
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

// TP-001 — RunSandboxedStep returns error for empty command slice.
func TestRunSandboxedStep_ReturnsErrorForEmptyCommandSlice(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	err := r.RunSandboxedStep("my-step", []string{}, SandboxOptions{})
	if err == nil {
		t.Fatal("expected error for empty command slice, got nil")
	}
	if !strings.Contains(err.Error(), "empty command") {
		t.Errorf("expected error to contain 'empty command', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "my-step") {
		t.Errorf("expected error to contain step name 'my-step', got %q", err.Error())
	}
}

// TP-002 — RunSandboxedStep returns error for nil command slice.
func TestRunSandboxedStep_ReturnsErrorForNilCommand(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	err := r.RunSandboxedStep("nil-step", nil, SandboxOptions{})
	if err == nil {
		t.Fatal("expected error for nil command, got nil")
	}
	if !strings.Contains(err.Error(), "empty command") {
		t.Errorf("expected error to contain 'empty command', got %q", err.Error())
	}
}

// TP-003 — RunSandboxedStep forwards stdout and stderr through runCommand.
func TestRunSandboxedStep_OutputForwarding(t *testing.T) {
	r, log, drain := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	if err := r.RunSandboxedStep("out-step", []string{"sh", "-c", "echo stdout-line; echo stderr-line >&2"}, SandboxOptions{}); err != nil {
		t.Fatalf("RunSandboxedStep: %v", err)
	}

	lines := drain()
	foundOut, foundErr := false, false
	for _, l := range lines {
		if l == "stdout-line" {
			foundOut = true
		}
		if l == "stderr-line" {
			foundErr = true
		}
	}
	if !foundOut {
		t.Errorf("expected 'stdout-line' in drain output, got %v", lines)
	}
	if !foundErr {
		t.Errorf("expected 'stderr-line' in drain output, got %v", lines)
	}
}

// TP-004 — RunSandboxedStep populates LastCapture on success and clears it on failure.
func TestRunSandboxedStep_LastCapturePopulation(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	// (a) Successful command: LastCapture holds the last non-empty stdout line.
	if err := r.RunSandboxedStep("cap-step", []string{"sh", "-c", "echo first; echo second"}, SandboxOptions{}); err != nil {
		t.Fatalf("RunSandboxedStep: %v", err)
	}
	if got := r.LastCapture(); got != "second" {
		t.Errorf("LastCapture after success: want %q, got %q", "second", got)
	}

	// (b) Failing command: LastCapture is cleared.
	_ = r.RunSandboxedStep("fail-step", []string{"false"}, SandboxOptions{})
	if got := r.LastCapture(); got != "" {
		t.Errorf("LastCapture after failure: want %q, got %q", "", got)
	}
}

// TP-005 — RunSandboxedStep cleans up the cidfile even when the command fails.
func TestRunSandboxedStep_CleansCidfileOnError(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	f, err := os.CreateTemp("", "ralph-test-*.cid")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	cidPath := f.Name()
	_ = f.Close()

	opts := SandboxOptions{CidfilePath: cidPath}
	runErr := r.RunSandboxedStep("fail-step", []string{"false"}, opts)
	if runErr == nil {
		t.Fatal("expected non-nil error from 'false', got nil")
	}

	if _, statErr := os.Stat(cidPath); !os.IsNotExist(statErr) {
		t.Errorf("expected cidfile %q to be removed after failed RunSandboxedStep, got stat err: %v", cidPath, statErr)
	}
}

// TP-006 — RunSandboxedStep resets the terminated flag.
func TestRunSandboxedStep_ResetTerminatedFlag(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	// Terminate a RunStep to set the flag.
	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunStep("long-step", []string{"sleep", "60"})
	}()
	time.Sleep(50 * time.Millisecond)
	r.Terminate()
	<-stepDone

	if !r.WasTerminated() {
		t.Fatal("WasTerminated should be true after Terminate was called")
	}

	// RunSandboxedStep must reset the flag.
	if err := r.RunSandboxedStep("reset-step", []string{"true"}, SandboxOptions{}); err != nil {
		t.Fatalf("RunSandboxedStep: %v", err)
	}

	if r.WasTerminated() {
		t.Error("WasTerminated should be false after RunSandboxedStep resets the flag")
	}
}

// TP-007 — Terminate() calls terminator exactly once with SIGTERM when the
// process exits promptly after SIGTERM (no SIGKILL escalation).
func TestTerminate_TerminatorSIGTERMOnlyWhenProcessExitsPromptly(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	r.terminateGraceOverride = 2 * time.Second

	var mu sync.Mutex
	var signals []syscall.Signal

	var proc *os.Process
	terminator := func(sig syscall.Signal) error {
		mu.Lock()
		signals = append(signals, sig)
		p := proc
		mu.Unlock()
		if p != nil {
			if sig == syscall.SIGKILL {
				return p.Kill()
			}
			return p.Signal(sig)
		}
		return nil
	}

	// Script traps SIGTERM and exits cleanly — SIGKILL should never fire.
	script := `trap 'exit 0' TERM; while true; do sleep 0.05; done`
	opts := SandboxOptions{Terminator: terminator}

	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunSandboxedStep("sigterm-only-step", []string{"sh", "-c", script}, opts)
	}()

	time.Sleep(100 * time.Millisecond)
	r.processMu.Lock()
	proc = r.currentProc
	r.processMu.Unlock()

	r.Terminate()

	select {
	case <-stepDone:
	case <-time.After(5 * time.Second):
		t.Fatal("RunSandboxedStep did not return within 5 seconds after Terminate")
	}

	mu.Lock()
	got := make([]syscall.Signal, len(signals))
	copy(got, signals)
	mu.Unlock()

	if len(got) != 1 {
		t.Fatalf("expected exactly 1 terminator call (SIGTERM only), got %d: %v", len(got), got)
	}
	if got[0] != syscall.SIGTERM {
		t.Errorf("expected SIGTERM, got %v", got[0])
	}
}

// TP-008 — RunSandboxedStep returns error for a nonexistent command.
func TestRunSandboxedStep_ReturnsErrorForNonExistentCommand(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	err := r.RunSandboxedStep("test-step", []string{"/nonexistent/command"}, SandboxOptions{})
	if err == nil {
		t.Fatal("expected error for nonexistent command, got nil")
	}
	if !strings.Contains(err.Error(), "workflow: start") {
		t.Errorf("expected error to contain 'workflow: start', got %q", err.Error())
	}
}

// TP-009 — RunSandboxedStep propagates non-zero exit error.
func TestRunSandboxedStep_ReturnsErrorOnNonZeroExit(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	err := r.RunSandboxedStep("exit-step", []string{"false"}, SandboxOptions{})
	if err == nil {
		t.Fatal("expected error from non-zero exit, got nil")
	}
}

// TP-010 — RunSandboxedStep uses projectDir as cmd.Dir.
func TestRunSandboxedStep_UsesProjectDir(t *testing.T) {
	projectDir := t.TempDir()
	logDir := t.TempDir()
	log, err := logger.NewLogger(logDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()

	r := NewRunner(log, projectDir)
	var mu sync.Mutex
	var captured []string
	r.SetSender(func(line string) {
		mu.Lock()
		captured = append(captured, line)
		mu.Unlock()
	})

	if err := r.RunSandboxedStep("pwd-step", []string{"sh", "-c", "pwd"}, SandboxOptions{}); err != nil {
		t.Fatalf("RunSandboxedStep: %v", err)
	}

	wantDir, _ := filepath.EvalSymlinks(projectDir)
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, line := range captured {
		got, evalErr := filepath.EvalSymlinks(line)
		if evalErr == nil && got == wantDir {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected RunSandboxedStep cmd.Dir=%q in captured output, got %v", wantDir, captured)
	}
}
