package workflow

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

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
