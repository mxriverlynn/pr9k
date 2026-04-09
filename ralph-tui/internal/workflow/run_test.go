package workflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
)

// --- Test doubles ---

type fakeExecutor struct {
	runStepCalls   []runStepCall
	captureResults []captureResult
	captureIdx     int
	logLines       []string
	closed         bool
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

func (f *fakeExecutor) CaptureOutput(_ []string) (string, error) {
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

// callbackFakeExecutor lets tests control RunStep behaviour with a callback for
// precise timing control.
type callbackFakeExecutor struct {
	runStepFn func(name string, cmd []string) error
	logLines  []string
	closed    bool
}

func (f *callbackFakeExecutor) RunStep(name string, cmd []string) error {
	if f.runStepFn != nil {
		return f.runStepFn(name, cmd)
	}
	return nil
}

func (f *callbackFakeExecutor) WasTerminated() bool    { return false }
func (f *callbackFakeExecutor) WriteToLog(line string) { f.logLines = append(f.logLines, line) }
func (f *callbackFakeExecutor) CaptureOutput(_ []string) (string, error) {
	return "", nil
}
func (f *callbackFakeExecutor) Close() error {
	f.closed = true
	return nil
}

type fakeRunHeader struct {
	phaseStepCalls []phaseStepCall
	stepStateCalls []stepStateCall
}

type phaseStepCall struct {
	label string
	names []string
}

type stepStateCall struct {
	idx   int
	state ui.StepState
}

func (h *fakeRunHeader) SetPhaseSteps(label string, names []string) {
	h.phaseStepCalls = append(h.phaseStepCalls, phaseStepCall{label, names})
}

func (h *fakeRunHeader) SetStepState(idx int, state ui.StepState) {
	h.stepStateCalls = append(h.stepStateCalls, stepStateCall{idx, state})
}

// newTestKeyHandler creates a KeyHandler suitable for tests where all steps succeed.
func newTestKeyHandler() *ui.KeyHandler {
	actions := make(chan ui.StepAction, 10)
	return ui.NewKeyHandler(func() {}, actions)
}

// commandStep creates a simple non-capture command step.
func commandStep(name string, args ...string) steps.Step {
	return steps.Step{Name: name, Command: args}
}

// captureStep creates a command step that captures its output into a variable.
func captureStep(name, outputVar string, args ...string) steps.Step {
	return steps.Step{Name: name, Command: args, OutputVariable: outputVar}
}

// exitLoopStep creates a loop step that captures output and exits the loop if empty.
func exitLoopStep(name, outputVar string, args ...string) steps.Step {
	return steps.Step{Name: name, Command: args, OutputVariable: outputVar, ExitLoopIfEmpty: true}
}

// --- Unit tests ---

// Test 1: pre-loop step executes and captures variable.
// A pre-loop command step with outputVariable must call CaptureOutput; the
// captured value must be available (substituted) in later loop steps.
func TestRun_PreLoopStep_CapturesVariable(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{{output: "myuser"}},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir: t.TempDir(),
		Iterations: 1,
		Config: &steps.WorkflowConfig{
			PreLoop: []steps.Step{captureStep("get-user", "GH_USER", "scripts/get_gh_user")},
			Loop:    []steps.Step{commandStep("use-user", "echo", "{{GH_USER}}")},
		},
	}

	Run(executor, header, kh, cfg)

	// The loop step must have received the substituted variable value.
	if len(executor.runStepCalls) == 0 {
		t.Fatal("expected RunStep to be called for the loop step")
	}
	cmd := executor.runStepCalls[0].command
	if !sliceContains(cmd, "myuser") {
		t.Errorf("expected loop step command to contain %q (substituted GH_USER), got %v", "myuser", cmd)
	}
}

// Test 2: loop step uses pre-loop variable.
// A variable captured in pre-loop must flow into loop step commands via substitution.
func TestRun_LoopStepUsesPreLoopVariable(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "testuser"}, // pre-loop GH_USER
			{output: "42"},       // loop ISSUE_ID (get-issue uses {{GH_USER}})
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir: t.TempDir(),
		Iterations: 1,
		Config: &steps.WorkflowConfig{
			PreLoop: []steps.Step{captureStep("get-user", "GH_USER", "scripts/get_gh_user")},
			Loop: []steps.Step{
				captureStep("get-issue", "ISSUE_ID", "scripts/get_next_issue", "{{GH_USER}}"),
				commandStep("do-work", "echo", "working on {{ISSUE_ID}}"),
			},
		},
	}

	Run(executor, header, kh, cfg)

	// do-work (the only RunStep call) must receive the substituted ISSUE_ID.
	if len(executor.runStepCalls) == 0 {
		t.Fatal("expected do-work RunStep call")
	}
	cmd := executor.runStepCalls[0].command
	if !sliceContains(cmd, "working on 42") {
		t.Errorf("expected do-work command to contain %q, got %v", "working on 42", cmd)
	}
}

// Test 3: loop variables reset each iteration.
// ISSUE_ID captured in iteration 1 must be cleared before iteration 2 so that
// the fresh capture in iteration 2 is used by subsequent steps.
func TestRun_LoopVariablesResetEachIteration(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "42"}, // iteration 1 ISSUE_ID
			{output: "99"}, // iteration 2 ISSUE_ID
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir: t.TempDir(),
		Iterations: 2,
		Config: &steps.WorkflowConfig{
			Loop: []steps.Step{
				captureStep("get-issue", "ISSUE_ID", "scripts/get_next_issue"),
				commandStep("do-work", "echo", "issue={{ISSUE_ID}}"),
			},
		},
	}

	Run(executor, header, kh, cfg)

	if len(executor.runStepCalls) < 2 {
		t.Fatalf("expected 2 do-work RunStep calls, got %d", len(executor.runStepCalls))
	}
	if !sliceContains(executor.runStepCalls[0].command, "issue=42") {
		t.Errorf("iteration 1 do-work: expected %q, got %v", "issue=42", executor.runStepCalls[0].command)
	}
	if !sliceContains(executor.runStepCalls[1].command, "issue=99") {
		t.Errorf("iteration 2 do-work: expected %q, got %v", "issue=99", executor.runStepCalls[1].command)
	}
}

// Test 4: exitLoopIfEmpty breaks the iteration loop.
// When a loop step with exitLoopIfEmpty captures an empty string, the loop must
// stop after that iteration regardless of the configured iteration count.
func TestRun_ExitLoopIfEmpty_BreaksIterationLoop(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{{output: ""}},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir: t.TempDir(),
		Iterations: 3,
		Config: &steps.WorkflowConfig{
			Loop: []steps.Step{exitLoopStep("check-issue", "ISSUE_ID", "scripts/get_next_issue")},
		},
	}

	Run(executor, header, kh, cfg)

	// Only one CaptureOutput call (iteration 1's empty result), no further iterations.
	if executor.captureIdx != 1 {
		t.Errorf("expected 1 CaptureOutput call (loop exited), got %d", executor.captureIdx)
	}
}

// Test 5: exitLoopIfEmpty with non-empty output continues the loop.
func TestRun_ExitLoopIfEmpty_NonEmptyContinues(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "42"},
			{output: "99"},
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir: t.TempDir(),
		Iterations: 2,
		Config: &steps.WorkflowConfig{
			Loop: []steps.Step{exitLoopStep("check-issue", "ISSUE_ID", "scripts/get_next_issue")},
		},
	}

	Run(executor, header, kh, cfg)

	if executor.captureIdx != 2 {
		t.Errorf("expected 2 CaptureOutput calls (both iterations ran), got %d", executor.captureIdx)
	}
}

// Test 6: post-loop always runs after early exit via exitLoopIfEmpty.
func TestRun_PostLoopAlwaysRunsAfterEarlyExit(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{{output: ""}}, // empty → exit loop
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir: t.TempDir(),
		Iterations: 3,
		Config: &steps.WorkflowConfig{
			Loop:     []steps.Step{exitLoopStep("check-issue", "ISSUE_ID", "scripts/get_next_issue")},
			PostLoop: []steps.Step{commandStep("finalize", "echo", "finalizing")},
		},
	}

	Run(executor, header, kh, cfg)

	found := false
	for _, call := range executor.runStepCalls {
		if call.name == "finalize" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected post-loop finalize step to run after early exit, got calls: %v", executor.runStepCalls)
	}
}

// Test 7: post-loop always runs after normal loop completion.
func TestRun_PostLoopAlwaysRunsAfterNormalCompletion(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir: t.TempDir(),
		Iterations: 1,
		Config: &steps.WorkflowConfig{
			Loop:     []steps.Step{commandStep("do-work", "echo", "work")},
			PostLoop: []steps.Step{commandStep("finalize", "echo", "finalizing")},
		},
	}

	Run(executor, header, kh, cfg)

	ran := make(map[string]bool)
	for _, c := range executor.runStepCalls {
		ran[c.name] = true
	}
	if !ran["finalize"] {
		t.Errorf("expected post-loop finalize to run, got steps: %v", executor.runStepCalls)
	}
}

// Test 8: Claude step builds correct command.
// A step with promptFile, model, and permissionMode must produce a command of
// the form ["claude", "--permission-mode", <mode>, "--model", <model>, "-p", <prompt>].
func TestRun_ClaudeStepBuildsCorrectCommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	promptContent := "Do the feature work"
	if err := os.WriteFile(filepath.Join(dir, "prompts", "feature.txt"), []byte(promptContent), 0o644); err != nil {
		t.Fatal(err)
	}

	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir: dir,
		Iterations: 1,
		Config: &steps.WorkflowConfig{
			Loop: []steps.Step{
				{
					Name:           "Feature work",
					PromptFile:     "feature.txt",
					Model:          "opus",
					PermissionMode: "default",
				},
			},
		},
	}

	Run(executor, header, kh, cfg)

	if len(executor.runStepCalls) == 0 {
		t.Fatal("expected RunStep to be called for Claude step")
	}
	cmd := executor.runStepCalls[0].command
	want := []string{"claude", "--permission-mode", "default", "--model", "opus", "-p", promptContent}
	if !equalStringSlices(cmd, want) {
		t.Errorf("Claude step command:\n  got  %v\n  want %v", cmd, want)
	}
}

// Test 9: quit drain stops execution before the next step.
// When ActionQuit arrives while a step is running, the following step must not execute.
func TestRun_QuitDrain_StopsExecutionBeforeNextStep(t *testing.T) {
	step1Started := make(chan struct{})
	step1Unblock := make(chan struct{})
	callCount := 0

	executor := &callbackFakeExecutor{
		runStepFn: func(name string, _ []string) error {
			callCount++
			if callCount == 1 {
				close(step1Started)
				<-step1Unblock
			}
			return nil
		},
	}

	actions := make(chan ui.StepAction, 1)
	kh := ui.NewKeyHandler(func() {}, actions)

	header := &fakeRunHeader{}
	cfg := RunConfig{
		ProjectDir: t.TempDir(),
		Iterations: 1,
		Config: &steps.WorkflowConfig{
			Loop: []steps.Step{
				commandStep("step1", "echo", "one"),
				commandStep("step2", "echo", "two"),
			},
		},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()

	// Wait for step1 to start, inject ActionQuit, then let step1 finish.
	<-step1Started
	actions <- ui.ActionQuit
	close(step1Unblock)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after ActionQuit")
	}

	if callCount != 1 {
		t.Errorf("expected only step1 to run, got %d RunStep calls", callCount)
	}
}

// --- Helpers ---

func sliceContains[T comparable](slice []T, item T) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
