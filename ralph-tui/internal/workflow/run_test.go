package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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
	mu        sync.Mutex
	logLines  []string
	closed    bool
}

func (f *callbackFakeExecutor) RunStep(name string, cmd []string) error {
	if f.runStepFn != nil {
		return f.runStepFn(name, cmd)
	}
	return nil
}

func (f *callbackFakeExecutor) WasTerminated() bool { return false }

func (f *callbackFakeExecutor) WriteToLog(line string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLines = append(f.logLines, line)
}

func (f *callbackFakeExecutor) CaptureOutput(_ []string) (string, error) {
	return "", nil
}

func (f *callbackFakeExecutor) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *callbackFakeExecutor) getLogLines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.logLines)
}

func (f *callbackFakeExecutor) wasClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

type fakeRunHeader struct {
	mu             sync.Mutex
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
	h.mu.Lock()
	defer h.mu.Unlock()
	h.phaseStepCalls = append(h.phaseStepCalls, phaseStepCall{label, names})
}

func (h *fakeRunHeader) SetStepState(idx int, state ui.StepState) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stepStateCalls = append(h.stepStateCalls, stepStateCall{idx, state})
}

func (h *fakeRunHeader) getPhaseStepCalls() []phaseStepCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.phaseStepCalls)
}

func (h *fakeRunHeader) getStepStateCalls() []stepStateCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.stepStateCalls)
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

// T23 — executeStep error recovery: retry re-executes the step.
// When a step fails and HandleStepError returns ActionRetry, the step must be
// re-executed. The retry separator must be logged. On success the second time,
// the step is marked done.
func TestRun_ExecuteStep_RetryReExecutesStep(t *testing.T) {
	callCount := 0
	executor := &callbackFakeExecutor{
		runStepFn: func(name string, cmd []string) error {
			callCount++
			if callCount == 1 {
				return errors.New("step failed")
			}
			return nil
		},
	}

	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)
	header := &fakeRunHeader{}

	cfg := RunConfig{
		ProjectDir: t.TempDir(),
		Iterations: 1,
		Config: &steps.WorkflowConfig{
			Loop: []steps.Step{commandStep("step1", "echo", "one")},
		},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()

	time.Sleep(50 * time.Millisecond)
	actions <- ui.ActionRetry

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after ActionRetry")
	}

	if callCount != 2 {
		t.Errorf("expected RunStep called twice (initial + retry), got %d", callCount)
	}

	logLines := executor.getLogLines()
	found := false
	for _, line := range logLines {
		if strings.Contains(line, "(retry)") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected retry separator in log, got: %v", logLines)
	}

	stepStateCalls := header.getStepStateCalls()
	lastDone := false
	for _, call := range stepStateCalls {
		if call.idx == 0 && call.state == ui.StepDone {
			lastDone = true
		}
	}
	if !lastDone {
		t.Errorf("expected step to be marked done after retry, state calls: %v", stepStateCalls)
	}
}

// T24 — executeStep error recovery: quit propagates quit and closes executor.
// When a step fails and the user chooses ActionQuit, Run() must return and
// call Close().
func TestRun_ExecuteStep_QuitClosesExecutor(t *testing.T) {
	callCount := 0
	executor := &callbackFakeExecutor{
		runStepFn: func(name string, cmd []string) error {
			callCount++
			return errors.New("always fails")
		},
	}

	actions := make(chan ui.StepAction, 10)
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

	time.Sleep(50 * time.Millisecond)
	actions <- ui.ActionQuit

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after ActionQuit")
	}

	if !executor.wasClosed() {
		t.Error("expected executor.Close() to be called after ActionQuit")
	}
	if callCount != 1 {
		t.Errorf("expected only step1 to be attempted, got %d RunStep calls", callCount)
	}
}

// T25 — executeStep error recovery: continue skips to next step.
// When a step fails and the user chooses ActionContinue, the failed step is
// skipped and the next step runs.
func TestRun_ExecuteStep_ContinueSkipsToNextStep(t *testing.T) {
	step2Ran := false
	executor := &callbackFakeExecutor{
		runStepFn: func(name string, cmd []string) error {
			if name == "step1" {
				return errors.New("step1 fails")
			}
			if name == "step2" {
				step2Ran = true
			}
			return nil
		},
	}

	actions := make(chan ui.StepAction, 10)
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

	time.Sleep(50 * time.Millisecond)
	actions <- ui.ActionContinue

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after ActionContinue")
	}

	if !step2Ran {
		t.Error("expected step2 to run after ActionContinue on step1 failure")
	}
}

// T27 — BuildPrompt failure logs error and continues.
// When a Claude step's prompt file is missing, executeStep must log an error
// and return without entering error recovery. The next step must still run.
func TestRun_BuildPromptFailure_LogsErrorAndContinues(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir: t.TempDir(),
		Iterations: 1,
		Config: &steps.WorkflowConfig{
			Loop: []steps.Step{
				{Name: "missing-step", PromptFile: "nonexistent.txt"},
				commandStep("next-step", "echo", "ran"),
			},
		},
	}

	Run(executor, header, kh, cfg)

	found := false
	for _, line := range executor.logLines {
		if strings.Contains(line, "missing-step") && strings.Contains(line, "could not build prompt") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error log for missing prompt, got: %v", executor.logLines)
	}

	if len(executor.runStepCalls) == 0 {
		t.Fatal("expected next-step to run after BuildPrompt failure")
	}
	if executor.runStepCalls[0].name != "next-step" {
		t.Errorf("expected next-step to run after BuildPrompt failure, got %q", executor.runStepCalls[0].name)
	}
}

// T28 — Completion summary reports correct iteration count.
// iterationsRun is not incremented when exitLoopIfEmpty fires mid-iteration.
// The completion message must reflect only fully-completed iterations.
func TestRun_CompletionSummary_ReportsCorrectIterationCount(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{
			{output: "42"}, // iteration 1: non-empty → continue
			{output: ""},   // iteration 2: empty → exit loop
		},
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

	found := false
	for _, line := range executor.logLines {
		if strings.Contains(line, "completed after 1 iteration(s)") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected completion message with 1 iteration, got log: %v", executor.logLines)
	}
}

// T29 — executor.Close() called on normal completion.
// After all phases complete normally, executor.Close() must be called exactly once.
func TestRun_ExecutorClose_CalledOnNormalCompletion(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir: t.TempDir(),
		Iterations: 1,
		Config: &steps.WorkflowConfig{
			Loop: []steps.Step{commandStep("do-work", "echo", "work")},
		},
	}

	Run(executor, header, kh, cfg)

	if !executor.closed {
		t.Error("expected executor.Close() to be called after normal completion")
	}
}

// T32 — Run calls SetPhaseSteps with correct phase labels.
// Run must call SetPhaseSteps with "Pre-loop", "Iteration 1/N", ..., "Post-loop"
// labels in order.
func TestRun_SetPhaseSteps_CalledWithCorrectLabels(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir: t.TempDir(),
		Iterations: 2,
		Config: &steps.WorkflowConfig{
			PreLoop:  []steps.Step{commandStep("pre-step", "echo", "pre")},
			Loop:     []steps.Step{commandStep("loop-step", "echo", "loop")},
			PostLoop: []steps.Step{commandStep("post-step", "echo", "post")},
		},
	}

	Run(executor, header, kh, cfg)

	phaseStepCalls := header.getPhaseStepCalls()
	labels := make([]string, len(phaseStepCalls))
	for i, call := range phaseStepCalls {
		labels[i] = call.label
	}

	want := []string{"Pre-loop", "Iteration 1/2", "Iteration 2/2", "Post-loop"}
	if !equalStringSlices(labels, want) {
		t.Errorf("SetPhaseSteps labels:\n  got  %v\n  want %v", labels, want)
	}
}

// T33 — Pre-loop quit prevents loop and post-loop.
// If ActionQuit arrives while a pre-loop step is running, loop and post-loop
// phases must not execute.
func TestRun_PreLoopQuit_PreventsLoopAndPostLoop(t *testing.T) {
	preStepStarted := make(chan struct{})
	preStepUnblock := make(chan struct{})
	var ranSteps []string

	executor := &callbackFakeExecutor{
		runStepFn: func(name string, cmd []string) error {
			ranSteps = append(ranSteps, name)
			if name == "pre-step" {
				close(preStepStarted)
				<-preStepUnblock
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
			PreLoop:  []steps.Step{commandStep("pre-step", "echo", "pre")},
			Loop:     []steps.Step{commandStep("loop-step", "echo", "loop")},
			PostLoop: []steps.Step{commandStep("post-step", "echo", "post")},
		},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()

	<-preStepStarted
	actions <- ui.ActionQuit
	close(preStepUnblock)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after ActionQuit during pre-loop")
	}

	for _, name := range ranSteps {
		if name == "loop-step" {
			t.Errorf("loop-step should not have run after pre-loop quit")
		}
		if name == "post-step" {
			t.Errorf("post-step should not have run after pre-loop quit")
		}
	}
}

// T34 — exitLoopIfEmpty treats whitespace-only output as empty.
// strings.TrimSpace means "\n  \n" is treated the same as "" for loop exit.
func TestRun_ExitLoopIfEmpty_WhitespaceOnlyTreatedAsEmpty(t *testing.T) {
	executor := &fakeExecutor{
		captureResults: []captureResult{{output: "  \n  "}},
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

	if executor.captureIdx != 1 {
		t.Errorf("expected 1 CaptureOutput call (loop exited on whitespace-only), got %d", executor.captureIdx)
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
