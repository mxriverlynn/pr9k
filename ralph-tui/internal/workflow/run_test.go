package workflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
)

// --- Test doubles ---

type fakeExecutor struct {
	runStepCalls       []runStepCall
	captureOutputCalls [][]string
	captureResults     []captureResult
	captureIdx         int
	logLines           []string
	closed             bool
}

type runStepCall struct {
	name    string
	command []string
}

type captureResult struct {
	output string
}

func (f *fakeExecutor) RunStep(name string, command []string) error {
	f.runStepCalls = append(f.runStepCalls, runStepCall{name, command})
	return nil
}

func (f *fakeExecutor) WasTerminated() bool { return false }

func (f *fakeExecutor) WriteToLog(line string) {
	f.logLines = append(f.logLines, line)
}

func (f *fakeExecutor) CaptureOutput(command []string) (string, error) {
	f.captureOutputCalls = append(f.captureOutputCalls, command)
	if f.captureIdx < len(f.captureResults) {
		r := f.captureResults[f.captureIdx]
		f.captureIdx++
		return r.output, nil
	}
	return "", nil
}

func (f *fakeExecutor) Close() error {
	f.closed = true
	return nil
}

type fakeRunHeader struct {
	iterationCalls    []iterCall
	stepStateCalls    []stepStateCall
	finalizationCalls []finalizeCall
	finalizeStepCalls []stepStateCall
}

type iterCall struct {
	current, total  int
	issueID, title string
}

type stepStateCall struct {
	idx   int
	state ui.StepState
}

type finalizeCall struct {
	current, total int
	names          []string
}

func (h *fakeRunHeader) SetIteration(current, total int, issueID, issueTitle string) {
	h.iterationCalls = append(h.iterationCalls, iterCall{current, total, issueID, issueTitle})
}

func (h *fakeRunHeader) SetStepState(idx int, state ui.StepState) {
	h.stepStateCalls = append(h.stepStateCalls, stepStateCall{idx, state})
}

func (h *fakeRunHeader) SetFinalization(current, total int, names []string) {
	h.finalizationCalls = append(h.finalizationCalls, finalizeCall{current, total, names})
}

func (h *fakeRunHeader) SetFinalizeStepState(idx int, state ui.StepState) {
	h.finalizeStepCalls = append(h.finalizeStepCalls, stepStateCall{idx, state})
}

// newTestKeyHandler creates a KeyHandler suitable for tests where all steps succeed.
func newTestKeyHandler() *ui.KeyHandler {
	actions := make(chan ui.StepAction, 10)
	return ui.NewKeyHandler(func() {}, actions)
}

// nonClaudeSteps creates simple non-claude steps with echo commands for testing.
func nonClaudeSteps(names ...string) []steps.Step {
	result := make([]steps.Step, len(names))
	for i, name := range names {
		result[i] = steps.Step{
			Name:     name,
			IsClaude: false,
			Command:  []string{"echo", name},
		}
	}
	return result
}

// oneIssueResults returns CaptureOutput results for a single iteration:
// get_gh_user, get_next_issue (with issueID), git rev-parse HEAD (with sha).
func oneIssueResults(username, issueID, sha string) []captureResult {
	return []captureResult{
		{output: username},
		{output: issueID},
		{output: sha},
	}
}

// --- Unit tests ---

// TestRun_SingleIterationAllStepsSucceed verifies each step is called in order
// for a single iteration followed by finalization.
func TestRun_SingleIterationAllStepsSucceed(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: oneIssueResults("testuser", "42", "abc123"),
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("step1", "step2", "step3"),
		FinalizeSteps: nonClaudeSteps("final1", "final2"),
	}

	Run(executor, header, kh, cfg)

	wantNames := []string{"step1", "step2", "step3", "final1", "final2"}
	if len(executor.runStepCalls) != len(wantNames) {
		t.Fatalf("expected %d RunStep calls, got %d: %v", len(wantNames), len(executor.runStepCalls), executor.runStepCalls)
	}
	for i, want := range wantNames {
		if executor.runStepCalls[i].name != want {
			t.Errorf("call %d: want name %q, got %q", i, want, executor.runStepCalls[i].name)
		}
	}
}

// TestRun_TwoIterationsAllStepsSucceed verifies the loop executes twice with
// the correct issue ID per iteration.
func TestRun_TwoIterationsAllStepsSucceed(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"}, // get_gh_user
			{output: "42"},       // iteration 1: get_next_issue
			{output: "abc123"},   // iteration 1: git rev-parse HEAD
			{output: "99"},       // iteration 2: get_next_issue
			{output: "def456"},   // iteration 2: git rev-parse HEAD
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    2,
		Steps:         nonClaudeSteps("step1"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	// 2 iteration steps + 1 finalize step = 3 RunStep calls
	if len(executor.runStepCalls) != 3 {
		t.Fatalf("expected 3 RunStep calls, got %d: %v", len(executor.runStepCalls), executor.runStepCalls)
	}

	if len(header.iterationCalls) != 2 {
		t.Fatalf("expected 2 SetIteration calls, got %d", len(header.iterationCalls))
	}
	if header.iterationCalls[0].issueID != "42" {
		t.Errorf("iteration 1: want issueID %q, got %q", "42", header.iterationCalls[0].issueID)
	}
	if header.iterationCalls[1].issueID != "99" {
		t.Errorf("iteration 2: want issueID %q, got %q", "99", header.iterationCalls[1].issueID)
	}
}

// TestRun_EmptyIssueIDSkipsLoopEarly verifies the loop exits when get_next_issue
// returns an empty string and logs a skip message.
func TestRun_EmptyIssueIDSkipsLoopEarly(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: ""}, // no issue
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    3,
		Steps:         nonClaudeSteps("step1", "step2"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	// No iteration steps should have run
	for _, call := range executor.runStepCalls {
		if call.name == "step1" || call.name == "step2" {
			t.Errorf("iteration step %q should not have run", call.name)
		}
	}

	// Skip message should be logged
	found := false
	for _, line := range executor.logLines {
		if strings.Contains(line, "No issue found") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected skip message in log, got %v", executor.logLines)
	}
}

// TestRun_FinalizationRunsAfterIterationLoop verifies finalization steps run
// after a successful iteration loop.
func TestRun_FinalizationRunsAfterIterationLoop(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: oneIssueResults("testuser", "42", "abc123"),
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("step1"),
		FinalizeSteps: nonClaudeSteps("final1", "final2"),
	}

	Run(executor, header, kh, cfg)

	ran := map[string]bool{}
	for _, call := range executor.runStepCalls {
		ran[call.name] = true
	}
	if !ran["final1"] || !ran["final2"] {
		t.Errorf("expected finalization steps to run, got %v", executor.runStepCalls)
	}
}

// TestRun_FinalizationRunsWhenNoIssueFound verifies finalization still runs
// even when get_next_issue returns empty (early loop exit).
func TestRun_FinalizationRunsWhenNoIssueFound(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: ""}, // no issue
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("step1"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	found := false
	for _, call := range executor.runStepCalls {
		if call.name == "final1" {
			found = true
		}
	}
	if !found {
		t.Error("expected finalization to run even when no issue found")
	}
}

// TestRun_BannerWrittenToLog verifies the banner from ralph-art.txt is written
// to the log at startup.
func TestRun_BannerWrittenToLog(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ralph-bash"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ralph-bash", "ralph-art.txt"),
		[]byte("banner line 1\nbanner line 2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: ""}, // no issue
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    dir,
		Iterations:    1,
		Steps:         nonClaudeSteps("step1"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	found := false
	for _, line := range executor.logLines {
		if line == "banner line 1" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected banner content in log lines, got %v", executor.logLines)
	}
}

// TestRun_GetNextIssueCalledWithUsername verifies get_next_issue is called with
// the GitHub username returned by get_gh_user.
func TestRun_GetNextIssueCalledWithUsername(t *testing.T) {
	dir := t.TempDir()
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "myuser"},
			{output: ""}, // no issue
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    dir,
		Iterations:    1,
		Steps:         nonClaudeSteps("step1"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	if len(executor.captureOutputCalls) < 2 {
		t.Fatalf("expected at least 2 CaptureOutput calls, got %d", len(executor.captureOutputCalls))
	}
	getNextIssueCall := executor.captureOutputCalls[1]
	if len(getNextIssueCall) == 0 || getNextIssueCall[len(getNextIssueCall)-1] != "myuser" {
		t.Errorf("expected get_next_issue called with username 'myuser', got %v", getNextIssueCall)
	}
}

// TestRun_CompletionSummaryShowsActualCount verifies the completion message
// shows the number of iterations actually completed, not the requested count.
func TestRun_CompletionSummaryShowsActualCount(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: "42"},     // 1st iteration: issue found
			{output: "abc123"}, // 1st iteration: sha
			{output: ""},       // 2nd iteration: no issue → exits loop
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    3, // 3 requested, only 1 completes
		Steps:         nonClaudeSteps("step1"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	found := false
	for _, line := range executor.logLines {
		if strings.Contains(line, "1 iteration(s)") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected completion summary with '1 iteration(s)', got %v", executor.logLines)
	}
}

// TestRun_ClosedAfterCompletion verifies executor.Close() is called after all
// work completes.
func TestRun_ClosedAfterCompletion(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: ""}, // no issue
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("step1"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	if !executor.closed {
		t.Error("expected executor to be closed after completion")
	}
}

// --- Integration tests ---

// writeScript creates an executable shell script at path with the given content.
func writeScript(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}

// TestCaptureOutput_UsesWorkingDir verifies CaptureOutput sets cmd.Dir to the
// runner's working directory for every subprocess.
func TestCaptureOutput_UsesWorkingDir(t *testing.T) {
	workingDir := t.TempDir()
	logDir := t.TempDir()
	log, err := logger.NewLogger(logDir)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	runner := NewRunner(log, workingDir)
	defer runner.Close()

	out, err := runner.CaptureOutput([]string{"sh", "-c", "pwd"})
	if err != nil {
		t.Fatalf("CaptureOutput: %v", err)
	}

	// Resolve symlinks for comparison (macOS temp dirs may be symlinked).
	wantDir, _ := filepath.EvalSymlinks(workingDir)
	gotDir, _ := filepath.EvalSymlinks(out)

	if gotDir != wantDir {
		t.Errorf("expected CaptureOutput cmd.Dir=%q, got %q", wantDir, gotDir)
	}
}

// TestRun_Integration_FullFlow runs the orchestration end-to-end with fake
// scripts and real subprocesses — verifying the full path from startup banner
// to completion summary.
func TestRun_Integration_FullFlow(t *testing.T) {
	projectDir := t.TempDir()
	workingDir := t.TempDir()

	// Init a git repo in workingDir so git rev-parse HEAD works.
	gitInit := exec.Command("git", "init", workingDir)
	if err := gitInit.Run(); err != nil {
		t.Skip("git not available, skipping integration test")
	}
	gitCommit := exec.Command("git",
		"-C", workingDir,
		"-c", "user.email=test@test.com",
		"-c", "user.name=test",
		"commit", "--allow-empty", "-m", "init",
	)
	if err := gitCommit.Run(); err != nil {
		t.Skipf("git commit failed, skipping: %v", err)
	}

	// Create banner.
	if err := os.MkdirAll(filepath.Join(projectDir, "ralph-bash"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ralph-bash", "ralph-art.txt"),
		[]byte("Test Banner\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create fake scripts.
	scriptsDir := filepath.Join(projectDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeScript(t, filepath.Join(scriptsDir, "get_gh_user"), "#!/bin/sh\necho testuser\n")
	writeScript(t, filepath.Join(scriptsDir, "get_next_issue"), "#!/bin/sh\necho 42\n")

	// Set up logger and runner.
	logDir := t.TempDir()
	log, err := logger.NewLogger(logDir)
	if err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(log, workingDir)
	collect := collectLines(t, runner)

	// Actions channel for KeyHandler.
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	// Use non-claude steps so no prompt files or claude binary are needed.
	iterSteps := []steps.Step{
		{Name: "Echo iter", IsClaude: false, Command: []string{"echo", "iteration step done"}},
	}
	finalSteps := []steps.Step{
		{Name: "Echo final", IsClaude: false, Command: []string{"echo", "finalization done"}},
	}

	cfg := RunConfig{
		ProjectDir:    projectDir,
		Iterations:    1,
		Steps:         iterSteps,
		FinalizeSteps: finalSteps,
	}

	header := &fakeRunHeader{}
	Run(runner, header, kh, cfg)

	collected := collect()
	_ = log.Close()

	checks := []struct {
		desc    string
		contain string
	}{
		{"banner", "Test Banner"},
		{"iteration step output", "iteration step done"},
		{"finalization step output", "finalization done"},
		{"completion summary", "Ralph completed"},
	}
	for _, c := range checks {
		found := false
		for _, line := range collected {
			if strings.Contains(line, c.contain) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s not found in output (looking for %q); got: %v", c.desc, c.contain, collected)
		}
	}
}
