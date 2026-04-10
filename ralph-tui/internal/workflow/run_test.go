package workflow

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
)

// --- Test doubles ---

type fakeExecutor struct {
	runStepCalls       []runStepCall
	runStepErrors      []error // per-call errors; nil entries mean success
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
	idx := len(f.runStepCalls)
	f.runStepCalls = append(f.runStepCalls, runStepCall{name, command})
	if idx < len(f.runStepErrors) && f.runStepErrors[idx] != nil {
		return f.runStepErrors[idx]
	}
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
	current, total int
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
	defer func() { _ = log.Close() }()

	runner := NewRunner(log, workingDir)
	defer func() { _ = runner.Close() }()

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

// TestRun_QuitFromIterationOrchestrateClosesAndSkipsFinalization verifies that
// when Orchestrate returns ActionQuit during an iteration, Run closes the
// executor and skips finalization and the completion summary.
func TestRun_QuitFromIterationOrchestrateClosesAndSkipsFinalization(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	actions <- ui.ActionQuit
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		captureResults: oneIssueResults("testuser", "42", "abc123"),
		runStepErrors:  []error{errors.New("step failed")},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("iter-step"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	if !executor.closed {
		t.Error("expected executor to be closed on quit")
	}
	for _, call := range executor.runStepCalls {
		if call.name == "final1" {
			t.Error("finalization step should not have run after iteration quit")
		}
	}
	for _, line := range executor.logLines {
		if strings.Contains(line, "Ralph completed") {
			t.Errorf("expected no completion summary on quit, found: %q", line)
		}
	}
}

// TestRun_QuitFromFinalizationOrchestrateClosesWithoutSummary verifies that
// when Orchestrate returns ActionQuit during finalization, Run closes the
// executor without writing the completion summary.
func TestRun_QuitFromFinalizationOrchestrateClosesWithoutSummary(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	actions <- ui.ActionQuit
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		captureResults: oneIssueResults("testuser", "42", "abc123"),
		// index 0 = iteration step (succeeds), index 1 = finalize step (fails)
		runStepErrors: []error{nil, errors.New("finalize failed")},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("iter-step"),
		FinalizeSteps: nonClaudeSteps("final-step"),
	}

	Run(executor, header, kh, cfg)

	if !executor.closed {
		t.Error("expected executor to be closed on finalization quit")
	}
	for _, line := range executor.logLines {
		if strings.Contains(line, "Ralph completed") {
			t.Errorf("expected no completion summary on quit, found: %q", line)
		}
	}
}

// TestBuildIterationSteps_ClaudeStep verifies that a claude step produces the
// correct CLI command with the expected flags and prompt content.
func TestBuildIterationSteps_ClaudeStep(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "test-prompt.txt"), []byte("do something"), 0644); err != nil {
		t.Fatal(err)
	}

	step := steps.Step{
		Name:        "test-step",
		IsClaude:    true,
		Model:       "claude-opus-4-6",
		PromptFile:  "test-prompt.txt",
		PrependVars: true,
	}

	resolved, err := buildIterationSteps(dir, []steps.Step{step}, "42", "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved step, got %d", len(resolved))
	}

	rs := resolved[0]
	if rs.Name != "test-step" {
		t.Errorf("expected name %q, got %q", "test-step", rs.Name)
	}
	if len(rs.Command) < 7 || rs.Command[0] != "claude" {
		t.Fatalf("unexpected command: %v", rs.Command)
	}
	if rs.Command[1] != "--permission-mode" || rs.Command[2] != "acceptEdits" {
		t.Errorf("expected --permission-mode acceptEdits, got %v %v", rs.Command[1], rs.Command[2])
	}
	if rs.Command[3] != "--model" || rs.Command[4] != "claude-opus-4-6" {
		t.Errorf("expected --model claude-opus-4-6, got %v %v", rs.Command[3], rs.Command[4])
	}
	if rs.Command[5] != "-p" {
		t.Errorf("expected -p flag, got %q", rs.Command[5])
	}
	prompt := rs.Command[6]
	if !strings.Contains(prompt, "ISSUENUMBER=42") {
		t.Errorf("expected ISSUENUMBER=42 in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "STARTINGSHA=abc123") {
		t.Errorf("expected STARTINGSHA=abc123 in prompt, got %q", prompt)
	}
}

// TestBuildIterationSteps_ClaudeStepMissingPromptFile verifies that a claude
// step with a missing prompt file returns a non-nil error containing the step name.
func TestBuildIterationSteps_ClaudeStepMissingPromptFile(t *testing.T) {
	dir := t.TempDir()

	step := steps.Step{
		Name:       "bad-step",
		IsClaude:   true,
		Model:      "some-model",
		PromptFile: "nonexistent.txt",
	}

	_, err := buildIterationSteps(dir, []steps.Step{step}, "42", "abc123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "bad-step") {
		t.Errorf("expected error to contain step name %q, got %q", "bad-step", err.Error())
	}
}

// TestRun_BuildIterationStepsErrorLogsAndContinuesToFinalization verifies that
// when buildIterationSteps fails, Run logs "Error preparing steps", skips the
// iteration, still runs finalization, and reports 0 iterations in the summary.
func TestRun_BuildIterationStepsErrorLogsAndContinuesToFinalization(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: "42"},
			{output: "abc123"},
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	claudeStep := steps.Step{
		Name:       "bad-claude",
		IsClaude:   true,
		Model:      "some-model",
		PromptFile: "nonexistent.txt",
	}

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         []steps.Step{claudeStep},
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	foundErr := false
	for _, line := range executor.logLines {
		if strings.Contains(line, "Error preparing steps") {
			foundErr = true
		}
	}
	if !foundErr {
		t.Errorf("expected 'Error preparing steps' in log, got %v", executor.logLines)
	}

	for _, call := range executor.runStepCalls {
		if call.name == "bad-claude" {
			t.Errorf("iteration step %q should not have run", call.name)
		}
	}

	ranFinal := false
	for _, call := range executor.runStepCalls {
		if call.name == "final1" {
			ranFinal = true
		}
	}
	if !ranFinal {
		t.Error("expected finalization to run after buildIterationSteps error")
	}

	found0 := false
	for _, line := range executor.logLines {
		if strings.Contains(line, "0 iteration(s)") {
			found0 = true
		}
	}
	if !found0 {
		t.Errorf("expected '0 iteration(s)' in completion summary, got %v", executor.logLines)
	}
}

// TestBuildFinalizeSteps_ClaudeStep verifies that a finalize claude step
// produces the correct CLI command and uses empty issueID/sha.
func TestBuildFinalizeSteps_ClaudeStep(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "finalize-prompt.txt"), []byte("finalize this"), 0644); err != nil {
		t.Fatal(err)
	}

	step := steps.Step{
		Name:        "finalize-claude",
		IsClaude:    true,
		Model:       "claude-sonnet-4-6",
		PromptFile:  "finalize-prompt.txt",
		PrependVars: true,
	}

	resolved, err := buildFinalizeSteps(dir, []steps.Step{step})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved step, got %d", len(resolved))
	}

	rs := resolved[0]
	if rs.Name != "finalize-claude" {
		t.Errorf("expected name %q, got %q", "finalize-claude", rs.Name)
	}
	if len(rs.Command) < 7 || rs.Command[0] != "claude" {
		t.Fatalf("unexpected command: %v", rs.Command)
	}
	if rs.Command[5] != "-p" {
		t.Errorf("expected -p flag at index 5, got %q", rs.Command[5])
	}
	prompt := rs.Command[6]
	if !strings.HasPrefix(prompt, "ISSUENUMBER=\nSTARTINGSHA=\n") {
		t.Errorf("expected empty ISSUENUMBER and STARTINGSHA in prompt, got %q", prompt)
	}
}

// TestFinalHeader_SetStepStateRoutesToSetFinalizeStepState verifies that
// finalHeader.SetStepState delegates to RunHeader.SetFinalizeStepState and
// does not call SetStepState.
func TestFinalHeader_SetStepStateRoutesToSetFinalizeStepState(t *testing.T) {
	h := &fakeRunHeader{}
	fh := &finalHeader{h: h}

	fh.SetStepState(2, ui.StepActive)

	if len(h.finalizeStepCalls) != 1 {
		t.Fatalf("expected 1 SetFinalizeStepState call, got %d", len(h.finalizeStepCalls))
	}
	if h.finalizeStepCalls[0].idx != 2 || h.finalizeStepCalls[0].state != ui.StepActive {
		t.Errorf("expected finalizeStepCalls[0]={2, StepActive}, got %+v", h.finalizeStepCalls[0])
	}
	if len(h.stepStateCalls) != 0 {
		t.Errorf("expected 0 SetStepState calls, got %d", len(h.stepStateCalls))
	}
}

// TestRun_BuildFinalizeStepsErrorSkipsFinalizationButWritesSummary verifies
// that when buildFinalizeSteps fails, Run skips finalization Orchestrate but
// still writes the completion summary and closes the executor.
func TestRun_BuildFinalizeStepsErrorSkipsFinalizationButWritesSummary(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: ""}, // no issue → skip iteration
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	claudeFinalStep := steps.Step{
		Name:       "bad-finalize",
		IsClaude:   true,
		Model:      "some-model",
		PromptFile: "nonexistent.txt",
	}

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("step1"),
		FinalizeSteps: []steps.Step{claudeFinalStep},
	}

	Run(executor, header, kh, cfg)

	for _, call := range executor.runStepCalls {
		if call.name == "bad-finalize" {
			t.Error("finalization step should not have run when buildFinalizeSteps errors")
		}
	}

	found := false
	for _, line := range executor.logLines {
		if strings.Contains(line, "Ralph completed") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected completion summary in log, got %v", executor.logLines)
	}

	if !executor.closed {
		t.Error("expected executor to be closed")
	}
}

// TestRun_SetFinalizationCalledWithStepNames verifies that SetFinalization
// receives the names of all finalize steps in order.
func TestRun_SetFinalizationCalledWithStepNames(t *testing.T) {
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
		FinalizeSteps: nonClaudeSteps("final-a", "final-b", "final-c"),
	}

	Run(executor, header, kh, cfg)

	if len(header.finalizationCalls) == 0 {
		t.Fatal("expected SetFinalization to be called")
	}
	names := header.finalizationCalls[0].names
	wantNames := []string{"final-a", "final-b", "final-c"}
	if len(names) != len(wantNames) {
		t.Fatalf("expected %d finalize step names, got %d: %v", len(wantNames), len(names), names)
	}
	for i, want := range wantNames {
		if names[i] != want {
			t.Errorf("name[%d]: want %q, got %q", i, want, names[i])
		}
	}
}

// --- Signal handling integration tests ---

// TestRun_ForceQuit_ClosesExecutorAndReturns verifies that when ForceQuit is
// called (simulating an OS signal), Run terminates cleanly and closes the
// executor (flushing the log writer).
func TestRun_ForceQuit_ClosesExecutorAndReturns(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		captureResults: oneIssueResults("testuser", "42", "abc123"),
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("step1", "step2"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	// Simulate OS signal arriving before Run processes any step.
	kh.ForceQuit()

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()

	select {
	case <-done:
		// Run returned cleanly after ForceQuit.
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after ForceQuit")
	}

	if !executor.closed {
		t.Error("expected executor.Close() to be called after ForceQuit")
	}
}

// TestRun_ForceQuit_SkipsRemainingSteps verifies that when ForceQuit is called
// (simulating an OS signal), iteration steps after the quit are not executed.
func TestRun_ForceQuit_SkipsRemainingSteps(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		captureResults: oneIssueResults("testuser", "42", "abc123"),
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("step1", "step2", "step3"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	// ForceQuit before any step runs — all steps should be skipped.
	kh.ForceQuit()
	Run(executor, header, kh, cfg)

	for _, call := range executor.runStepCalls {
		if call.name == "step1" || call.name == "step2" || call.name == "step3" {
			t.Errorf("expected no iteration steps to run after ForceQuit, but %q ran", call.name)
		}
	}
}

// T5 — ForceQuit before finalization starts skips all finalize steps but still
// closes the executor. ActionQuit is preserved through the skipped iteration
// loop (no issue found) and consumed by the finalization Orchestrate's
// pre-step drain.
func TestRun_ForceQuit_DuringFinalization_SkipsAllFinalizeSteps(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: ""}, // no issue → iteration loop exits without calling Orchestrate
		},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("iter-step"),
		FinalizeSteps: nonClaudeSteps("final1", "final2", "final3"),
	}

	// ActionQuit bypasses the iteration loop (no issue found, so no Orchestrate there)
	// and is consumed by the finalization Orchestrate's pre-step drain.
	kh.ForceQuit()
	Run(executor, header, kh, cfg)

	for _, call := range executor.runStepCalls {
		if call.name == "final1" || call.name == "final2" || call.name == "final3" {
			t.Errorf("expected no finalize steps to run after ForceQuit, but %q ran", call.name)
		}
	}
	if !executor.closed {
		t.Error("expected executor.Close() to be called after ForceQuit during finalization")
	}
}

// --- Unbounded mode tests ---

// TestRun_UnboundedLoopRunsUntilNoIssue verifies that when Iterations == 0,
// the loop runs until get_next_issue returns empty, then exits.
func TestRun_UnboundedLoopRunsUntilNoIssue(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"}, // get_gh_user
			{output: "10"},       // iteration 1: get_next_issue
			{output: "sha1"},     // iteration 1: git rev-parse HEAD
			{output: "20"},       // iteration 2: get_next_issue
			{output: "sha2"},     // iteration 2: git rev-parse HEAD
			{output: ""},         // iteration 3: no issue → exit
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    0,
		Steps:         nonClaudeSteps("step1"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	// step1 should have run exactly twice (once per issue), final1 once
	iterCount := 0
	for _, call := range executor.runStepCalls {
		if call.name == "step1" {
			iterCount++
		}
	}
	if iterCount != 2 {
		t.Errorf("expected step1 to run 2 times, got %d", iterCount)
	}
}

// TestRun_UnboundedLoopRunsFinalizationAfterExhausted verifies finalization
// runs after the unbounded loop exits when no more issues are found.
func TestRun_UnboundedLoopRunsFinalizationAfterExhausted(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: "10"},
			{output: "sha1"},
			{output: "20"},
			{output: "sha2"},
			{output: ""},
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    0,
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
		t.Error("expected finalization to run after unbounded loop exhausted issues")
	}
}

// TestRun_UnboundedLoopSetIterationPassesTotalZero verifies that SetIteration
// is called with total == 0 in unbounded mode.
func TestRun_UnboundedLoopSetIterationPassesTotalZero(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: "10"},
			{output: "sha1"},
			{output: ""},
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    0,
		Steps:         nonClaudeSteps("step1"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	if len(header.iterationCalls) == 0 {
		t.Fatal("expected at least one SetIteration call")
	}
	for _, call := range header.iterationCalls {
		if call.total != 0 {
			t.Errorf("expected SetIteration total == 0 in unbounded mode, got %d", call.total)
		}
	}
}

// TestRun_UnboundedLoopLogLinesHaveNoTotal verifies log lines use "Iteration N —"
// format (no "/M") in unbounded mode.
func TestRun_UnboundedLoopLogLinesHaveNoTotal(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: "10"},
			{output: "sha1"},
			{output: "20"},
			{output: "sha2"},
			{output: ""},
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    0,
		Steps:         nonClaudeSteps("step1"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	for _, line := range executor.logLines {
		if strings.Contains(line, "Iteration 1/") || strings.Contains(line, "Iteration 2/") {
			t.Errorf("unbounded log line should not contain total, got: %q", line)
		}
	}

	found1 := false
	found2 := false
	for _, line := range executor.logLines {
		if strings.Contains(line, "Iteration 1 —") {
			found1 = true
		}
		if strings.Contains(line, "Iteration 2 —") {
			found2 = true
		}
	}
	if !found1 {
		t.Error("expected log line containing 'Iteration 1 —'")
	}
	if !found2 {
		t.Error("expected log line containing 'Iteration 2 —'")
	}
}

// TestRun_BoundedModeCapLimitsIterations verifies that when more issues are
// available than Iterations, the loop stops at the cap.
func TestRun_BoundedModeCapLimitsIterations(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: "10"}, // iteration 1
			{output: "sha1"},
			{output: "20"}, // iteration 2
			{output: "sha2"},
			// 3rd issue is available but should never be fetched
			{output: "30"},
			{output: "sha3"},
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

	iterCount := 0
	for _, call := range executor.runStepCalls {
		if call.name == "step1" {
			iterCount++
		}
	}
	if iterCount != 2 {
		t.Errorf("expected exactly 2 iterations with bounded cap, got %d", iterCount)
	}
}

// --- iterationLabel unit tests ---

// TestIterationLabel_BoundedMode verifies "Iteration N/M" format when total > 0.
func TestIterationLabel_BoundedMode(t *testing.T) {
	cases := []struct {
		i, total int
		want     string
	}{
		{1, 3, "Iteration 1/3"},
		{2, 0, "Iteration 2"},
		{1, 1, "Iteration 1/1"},
	}
	for _, c := range cases {
		got := iterationLabel(c.i, c.total)
		if got != c.want {
			t.Errorf("iterationLabel(%d, %d) = %q, want %q", c.i, c.total, got, c.want)
		}
	}
}

// TestRun_StepsPendingSetBeforeIteration verifies that all step indices are
// set to StepPending before the first StepActive call in each iteration.
func TestRun_StepsPendingSetBeforeIteration(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: oneIssueResults("testuser", "42", "abc123"),
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("s1", "s2", "s3"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	pendingBeforeActive := map[int]bool{}
	for _, call := range header.stepStateCalls {
		if call.state == ui.StepPending {
			pendingBeforeActive[call.idx] = true
		} else if call.state == ui.StepActive {
			break
		}
	}
	for _, idx := range []int{0, 1, 2} {
		if !pendingBeforeActive[idx] {
			t.Errorf("expected StepPending for index %d before first StepActive", idx)
		}
	}
}

// --- Additional unbounded mode tests ---

// TestRun_UnboundedQuitFromOrchestrateClosesAndSkipsFinalization verifies that
// when Orchestrate returns ActionQuit during an unbounded-mode iteration, Run
// closes the executor and skips finalization and the completion summary.
func TestRun_UnboundedQuitFromOrchestrateClosesAndSkipsFinalization(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	actions <- ui.ActionQuit
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		captureResults: oneIssueResults("testuser", "42", "abc123"),
		runStepErrors:  []error{errors.New("step failed")},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    0,
		Steps:         nonClaudeSteps("iter-step"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	if !executor.closed {
		t.Error("expected executor to be closed on quit in unbounded mode")
	}
	for _, call := range executor.runStepCalls {
		if call.name == "final1" {
			t.Error("finalization step should not have run after iteration quit in unbounded mode")
		}
	}
	for _, line := range executor.logLines {
		if strings.Contains(line, "Ralph completed") {
			t.Errorf("expected no completion summary on quit, found: %q", line)
		}
	}
}

// TestRun_UnboundedCompletionSummaryShowsActualCount verifies that after the
// unbounded loop processes N issues and exits, the completion summary reports N.
func TestRun_UnboundedCompletionSummaryShowsActualCount(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: "10"},   // iteration 1: issue found
			{output: "sha1"}, // iteration 1: sha
			{output: "20"},   // iteration 2: issue found
			{output: "sha2"}, // iteration 2: sha
			{output: ""},     // iteration 3: no issue → exit
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    0,
		Steps:         nonClaudeSteps("step1"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	found := false
	for _, line := range executor.logLines {
		if strings.Contains(line, "2 iteration(s)") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected completion summary with '2 iteration(s)' in unbounded mode, got %v", executor.logLines)
	}
}

// TestRun_UnboundedForceQuitClosesExecutorAndReturns verifies that when
// ForceQuit is called before Run starts, Run terminates cleanly and closes
// the executor in unbounded mode.
func TestRun_UnboundedForceQuitClosesExecutorAndReturns(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		captureResults: oneIssueResults("testuser", "42", "abc123"),
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    0,
		Steps:         nonClaudeSteps("step1", "step2"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	kh.ForceQuit()

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()

	select {
	case <-done:
		// Run returned cleanly after ForceQuit.
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after ForceQuit in unbounded mode")
	}

	if !executor.closed {
		t.Error("expected executor.Close() to be called after ForceQuit in unbounded mode")
	}
}

// TestRun_UnboundedBuildIterationStepsErrorLogsAndContinuesToFinalization
// verifies that when buildIterationSteps fails in unbounded mode, Run logs
// "Error preparing steps", breaks the loop, runs finalization, and reports
// 0 iterations in the summary.
func TestRun_UnboundedBuildIterationStepsErrorLogsAndContinuesToFinalization(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: "42"},
			{output: "abc123"},
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	claudeStep := steps.Step{
		Name:       "bad-claude",
		IsClaude:   true,
		Model:      "some-model",
		PromptFile: "nonexistent.txt",
	}

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    0,
		Steps:         []steps.Step{claudeStep},
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	foundErr := false
	for _, line := range executor.logLines {
		if strings.Contains(line, "Error preparing steps") {
			foundErr = true
		}
	}
	if !foundErr {
		t.Errorf("expected 'Error preparing steps' in log, got %v", executor.logLines)
	}

	ranFinal := false
	for _, call := range executor.runStepCalls {
		if call.name == "final1" {
			ranFinal = true
		}
	}
	if !ranFinal {
		t.Error("expected finalization to run after buildIterationSteps error in unbounded mode")
	}

	found0 := false
	for _, line := range executor.logLines {
		if strings.Contains(line, "0 iteration(s)") {
			found0 = true
		}
	}
	if !found0 {
		t.Errorf("expected '0 iteration(s)' in completion summary, got %v", executor.logLines)
	}
}

// TestRun_UnboundedNoIssueFoundLogFormat verifies the "No issue found" log line
// uses "Iteration N" format (no "/M") in unbounded mode.
func TestRun_UnboundedNoIssueFoundLogFormat(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"},
			{output: "10"},   // iteration 1: issue found
			{output: "sha1"}, // iteration 1: sha
			{output: "20"},   // iteration 2: issue found
			{output: "sha2"}, // iteration 2: sha
			{output: ""},     // iteration 3: no issue → exit
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    0,
		Steps:         nonClaudeSteps("step1"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	foundNoIssue := false
	for _, line := range executor.logLines {
		if strings.Contains(line, "Iteration 3") && strings.Contains(line, "No issue found") {
			foundNoIssue = true
		}
		if strings.Contains(line, "Iteration 3/") {
			t.Errorf("unbounded 'no issue' log line should not contain total, got: %q", line)
		}
	}
	if !foundNoIssue {
		t.Errorf("expected log line with 'Iteration 3' and 'No issue found', got %v", executor.logLines)
	}
}
