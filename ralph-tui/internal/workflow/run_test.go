package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/vars"
)

// --- Test doubles ---

type fakeExecutor struct {
	runStepCalls    []runStepCall
	runStepErrors   []error  // per-call errors; nil entries mean success
	runStepCaptures []string // per-call LastCapture values (indexed by call order)
	lastCapture     string
	logLines        []string
	closed          bool
}

type runStepCall struct {
	name    string
	command []string
}

func (f *fakeExecutor) RunStep(name string, command []string) error {
	idx := len(f.runStepCalls)
	f.runStepCalls = append(f.runStepCalls, runStepCall{name, command})
	if idx < len(f.runStepErrors) && f.runStepErrors[idx] != nil {
		f.lastCapture = ""
		return f.runStepErrors[idx]
	}
	if idx < len(f.runStepCaptures) {
		f.lastCapture = f.runStepCaptures[idx]
	} else {
		f.lastCapture = ""
	}
	return nil
}

func (f *fakeExecutor) WasTerminated() bool { return false }

func (f *fakeExecutor) WriteToLog(line string) {
	f.logLines = append(f.logLines, line)
}

func (f *fakeExecutor) LastCapture() string {
	return f.lastCapture
}

func (f *fakeExecutor) Close() error {
	f.closed = true
	return nil
}

type fakeRunHeader struct {
	renderIterationCalls []renderIterCall
	stepStateCalls       []stepStateCall
	phaseStepsCalls      [][]string
}

type renderIterCall struct {
	iter, maxIter int
	issueID       string
}

type stepStateCall struct {
	idx   int
	state ui.StepState
}

func (h *fakeRunHeader) RenderInitializeLine(stepNum, stepCount int, stepName string) {}
func (h *fakeRunHeader) RenderIterationLine(iter, maxIter int, issueID string) {
	h.renderIterationCalls = append(h.renderIterationCalls, renderIterCall{iter, maxIter, issueID})
}
func (h *fakeRunHeader) RenderFinalizeLine(stepNum, stepCount int, stepName string) {}

func (h *fakeRunHeader) SetPhaseSteps(names []string) {
	cp := make([]string, len(names))
	copy(cp, names)
	h.phaseStepsCalls = append(h.phaseStepsCalls, cp)
}

func (h *fakeRunHeader) SetStepState(idx int, state ui.StepState) {
	h.stepStateCalls = append(h.stepStateCalls, stepStateCall{idx, state})
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

// captureStep creates a non-claude step with CaptureAs set. The step runs
// "echo <name>" so that real runners produce the name as output.
func captureStep(name, captureAs string) steps.Step {
	return steps.Step{
		Name:      name,
		IsClaude:  false,
		Command:   []string{"echo", name},
		CaptureAs: captureAs,
	}
}

// breakStep creates a non-claude step with CaptureAs and BreakLoopIfEmpty.
func breakStep(name, captureAs string) steps.Step {
	return steps.Step{
		Name:             name,
		IsClaude:         false,
		Command:          []string{"echo", name},
		CaptureAs:        captureAs,
		BreakLoopIfEmpty: true,
	}
}

// --- Unit tests ---

// TestRun_SingleIterationAllStepsSucceed verifies each step is called in order
// for a single iteration followed by finalization.
func TestRun_SingleIterationAllStepsSucceed(t *testing.T) {
	executor := &fakeExecutor{}
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

// TestRun_TwoIterationsAllStepsSucceed verifies the loop executes twice.
func TestRun_TwoIterationsAllStepsSucceed(t *testing.T) {
	executor := &fakeExecutor{}
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

	if len(header.renderIterationCalls) != 2 {
		t.Fatalf("expected 2 RenderIterationLine calls, got %d", len(header.renderIterationCalls))
	}
	// issueID is empty at iteration start (populated by step captureAs, not hardcoded)
	if header.renderIterationCalls[0].issueID != "" {
		t.Errorf("iteration 1: want empty issueID at start, got %q", header.renderIterationCalls[0].issueID)
	}
}

// TestRun_BreakLoopIfEmptyCapture verifies the loop exits when a step with
// BreakLoopIfEmpty produces empty capture, and iterationsRun reflects the
// iteration that triggered the break.
func TestRun_BreakLoopIfEmptyCapture(t *testing.T) {
	// runStepCaptures[0] is "" (empty) for the break step, so loop exits immediately.
	executor := &fakeExecutor{
		runStepCaptures: []string{""},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	breakIter := breakStep("get-issue", "ISSUE_ID")

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    3,
		Steps:         []steps.Step{breakIter, nonClaudeSteps("work")[0]},
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	result := Run(executor, header, kh, cfg)

	// "work" step should not have run (loop broke before it)
	for _, call := range executor.runStepCalls {
		if call.name == "work" {
			t.Errorf("work step should not have run after breakLoopIfEmpty triggered")
		}
	}

	// final1 should still run
	found := false
	for _, call := range executor.runStepCalls {
		if call.name == "final1" {
			found = true
		}
	}
	if !found {
		t.Error("expected finalization to run even after early loop exit")
	}

	if result.IterationsRun != 1 {
		t.Errorf("expected IterationsRun=1 (break on first iteration), got %d", result.IterationsRun)
	}
}

// TestRun_BreakLoopIfEmptyNonEmptyCapture verifies the loop continues when
// BreakLoopIfEmpty is set but capture is non-empty, and breaks when empty.
// iterationsRun reflects the iteration that triggered the break.
func TestRun_BreakLoopIfEmptyNonEmptyCapture(t *testing.T) {
	// Iteration 1: get-issue → "issue-42" (non-empty → continue), work → ""
	// Iteration 2: get-issue → "" (empty AND BreakLoopIfEmpty → break)
	// Final: final1
	// Total: 4 RunStep calls
	executor := &fakeExecutor{
		runStepCaptures: []string{"issue-42", "", ""},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	breakIter := breakStep("get-issue", "ISSUE_ID")

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    3,
		Steps:         []steps.Step{breakIter, nonClaudeSteps("work")[0]},
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	result := Run(executor, header, kh, cfg)

	count := len(executor.runStepCalls)
	if count != 4 {
		t.Errorf("expected 4 RunStep calls (iter1: get-issue+work, iter2: get-issue breaks, final1), got %d: %v", count, executor.runStepCalls)
	}

	if result.IterationsRun != 2 {
		t.Errorf("expected IterationsRun=2 (break on second iteration), got %d", result.IterationsRun)
	}
}

// TestRun_BreakLoopIfEmptyStepFails verifies that when a step with
// BreakLoopIfEmpty fails (non-zero exit), the break does not fire — normal
// error-mode path takes over instead.
func TestRun_BreakLoopIfEmptyStepFails(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		// step 0 (get-issue) fails; step 1 (work) succeeds
		runStepErrors:   []error{errors.New("exit 1"), nil},
		runStepCaptures: []string{"", ""},
	}
	header := &fakeRunHeader{}

	breakIter := breakStep("get-issue", "ISSUE_ID")

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         []steps.Step{breakIter, nonClaudeSteps("work")[0]},
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()

	// Let Orchestrate reach the blocked error-mode state, then send ActionContinue.
	time.Sleep(30 * time.Millisecond)
	actions <- ui.ActionContinue

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete after ActionContinue")
	}

	// "work" step must have run — the break did not fire despite empty capture,
	// because the step failed (StepFailed, not StepDone).
	found := false
	for _, call := range executor.runStepCalls {
		if call.name == "work" {
			found = true
		}
	}
	if !found {
		t.Error("work step should have run: breakLoopIfEmpty must not fire when the step fails")
	}
}

// TestRun_InitializeStepsRunBeforeIterationSteps verifies initialize steps
// execute before any iteration steps.
func TestRun_InitializeStepsRunBeforeIterationSteps(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:      t.TempDir(),
		Iterations:      1,
		InitializeSteps: nonClaudeSteps("init1", "init2"),
		Steps:           nonClaudeSteps("iter1"),
		FinalizeSteps:   nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	wantNames := []string{"init1", "init2", "iter1", "final1"}
	if len(executor.runStepCalls) != len(wantNames) {
		t.Fatalf("expected %d RunStep calls, got %d: %v", len(wantNames), len(executor.runStepCalls), executor.runStepCalls)
	}
	for i, want := range wantNames {
		if executor.runStepCalls[i].name != want {
			t.Errorf("call %d: want %q, got %q", i, want, executor.runStepCalls[i].name)
		}
	}
}

// TestRun_InitializeCaptureAvailableInIteration verifies that a captureAs
// binding from the initialize phase is available in subsequent iteration steps
// via VarTable substitution.
func TestRun_InitializeCaptureAvailableInIteration(t *testing.T) {
	dir := t.TempDir()

	// init step captures "myuser" as GITHUB_USER
	// iteration step command includes {{GITHUB_USER}} which should be substituted
	executor := &fakeExecutor{
		runStepCaptures: []string{"myuser"}, // init step capture
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	initStep := captureStep("get-user", "GITHUB_USER")
	iterStep := steps.Step{
		Name:     "get-issue",
		IsClaude: false,
		Command:  []string{"echo", "{{GITHUB_USER}}"},
	}

	cfg := RunConfig{
		ProjectDir:      dir,
		Iterations:      1,
		InitializeSteps: []steps.Step{initStep},
		Steps:           []steps.Step{iterStep},
	}

	Run(executor, header, kh, cfg)

	if len(executor.runStepCalls) < 2 {
		t.Fatalf("expected at least 2 RunStep calls, got %d", len(executor.runStepCalls))
	}

	// The iteration step's command should have {{GITHUB_USER}} substituted to "myuser"
	iterCall := executor.runStepCalls[1]
	found := false
	for _, arg := range iterCall.command {
		if arg == "myuser" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'myuser' in iteration step command ({{GITHUB_USER}} substituted), got %v", iterCall.command)
	}
}

// TestRun_FinalizationRunsAfterIterationLoop verifies finalization steps run
// after a successful iteration loop.
func TestRun_FinalizationRunsAfterIterationLoop(t *testing.T) {
	executor := &fakeExecutor{}
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

// TestRun_FinalizationRunsWhenLoopBreaksEarly verifies finalization still runs
// even when the iteration loop exits early via breakLoopIfEmpty.
func TestRun_FinalizationRunsWhenLoopBreaksEarly(t *testing.T) {
	executor := &fakeExecutor{
		runStepCaptures: []string{""}, // break step captures empty → exit loop
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         []steps.Step{breakStep("get-issue", "ISSUE_ID")},
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
		t.Error("expected finalization to run even when loop breaks early")
	}
}

// TestRun_ClosedAfterCompletion verifies executor.Close() is called after all
// work completes.
func TestRun_ClosedAfterCompletion(t *testing.T) {
	executor := &fakeExecutor{}
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

// TestRun_StepBuildErrorSkipsIterationAndContinuesToFinalization verifies that
// when building a step fails, Run logs "Error preparing steps", skips the
// remaining iteration steps, still runs finalization.
func TestRun_StepBuildErrorSkipsIterationAndContinuesToFinalization(t *testing.T) {
	executor := &fakeExecutor{}
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
		t.Error("expected finalization to run after step build error")
	}
}

// TestRun_QuitFromIterationOrchestrateClosesAndSkipsFinalization verifies that
// when Orchestrate returns ActionQuit during an iteration, Run closes the
// executor and skips finalization.
func TestRun_QuitFromIterationOrchestrateClosesAndSkipsFinalization(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	actions <- ui.ActionQuit
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		runStepErrors: []error{errors.New("step failed")},
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
}

// TestRun_QuitFromFinalizationOrchestrateClosesWithoutSummary verifies that
// when Orchestrate returns ActionQuit during finalization, Run closes the
// executor.
func TestRun_QuitFromFinalizationOrchestrateClosesWithoutSummary(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	actions <- ui.ActionQuit
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
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
}

// TestRun_QuitFromInitializeOrchestrateClosesEarly verifies that
// ActionQuit during the initialize phase closes the executor immediately.
func TestRun_QuitFromInitializeOrchestrateClosesEarly(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	actions <- ui.ActionQuit
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		runStepErrors: []error{errors.New("init failed")},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		ProjectDir:      t.TempDir(),
		Iterations:      1,
		InitializeSteps: nonClaudeSteps("init-step"),
		Steps:           nonClaudeSteps("iter-step"),
		FinalizeSteps:   nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	if !executor.closed {
		t.Error("expected executor to be closed on initialize quit")
	}
	for _, call := range executor.runStepCalls {
		if call.name == "iter-step" || call.name == "final1" {
			t.Errorf("step %q should not have run after initialize quit", call.name)
		}
	}
}

// TestRun_InitializeBuildErrorContinuesToNextInitStep verifies that when
// buildStep fails for an initialize step (e.g., missing prompt file), Run logs
// the error and continues to the next initialize step rather than aborting the
// initialize phase. The subsequent initialize step and the iteration loop both
// still run.
func TestRun_InitializeBuildErrorContinuesToNextInitStep(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	badInitStep := steps.Step{
		Name:       "bad-init",
		IsClaude:   true,
		Model:      "some-model",
		PromptFile: "nonexistent.txt",
	}

	cfg := RunConfig{
		ProjectDir:      t.TempDir(),
		Iterations:      1,
		InitializeSteps: []steps.Step{badInitStep, nonClaudeSteps("good-init")[0]},
		Steps:           nonClaudeSteps("iter-step"),
	}

	Run(executor, header, kh, cfg)

	foundErr := false
	for _, line := range executor.logLines {
		if strings.Contains(line, "Error preparing initialize step") {
			foundErr = true
		}
	}
	if !foundErr {
		t.Errorf("expected 'Error preparing initialize step' in log, got %v", executor.logLines)
	}

	ranGoodInit := false
	ranIter := false
	for _, call := range executor.runStepCalls {
		if call.name == "good-init" {
			ranGoodInit = true
		}
		if call.name == "iter-step" {
			ranIter = true
		}
	}
	if !ranGoodInit {
		t.Error("expected good-init step to run after bad-init build error")
	}
	if !ranIter {
		t.Error("expected iteration to run after initialize phase completed")
	}
}

// TestBuildStep_ClaudeStepIteration verifies that a claude iteration step
// produces the correct CLI command with the expected flags and prompt content.
func TestBuildStep_ClaudeStepIteration(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "test-prompt.txt"), []byte("do something"), 0644); err != nil {
		t.Fatal(err)
	}

	step := steps.Step{
		Name:       "test-step",
		IsClaude:   true,
		Model:      "claude-opus-4-6",
		PromptFile: "test-prompt.txt",
	}

	vt := vars.New(dir, 0)
	vt.SetPhase(vars.Iteration)
	vt.Bind(vars.Iteration, "ISSUE_ID", "42")
	vt.Bind(vars.Iteration, "STARTING_SHA", "abc123")
	resolved, err := buildStep(dir, step, vt, vars.Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Name != "test-step" {
		t.Errorf("expected name %q, got %q", "test-step", resolved.Name)
	}
	if len(resolved.Command) < 7 || resolved.Command[0] != "claude" {
		t.Fatalf("unexpected command: %v", resolved.Command)
	}
	if resolved.Command[1] != "--permission-mode" || resolved.Command[2] != "acceptEdits" {
		t.Errorf("expected --permission-mode acceptEdits, got %v %v", resolved.Command[1], resolved.Command[2])
	}
	if resolved.Command[3] != "--model" || resolved.Command[4] != "claude-opus-4-6" {
		t.Errorf("expected --model claude-opus-4-6, got %v %v", resolved.Command[3], resolved.Command[4])
	}
	if resolved.Command[5] != "-p" {
		t.Errorf("expected -p flag, got %q", resolved.Command[5])
	}
	if got := resolved.Command[6]; got != "do something" {
		t.Errorf("expected prompt %q, got %q", "do something", got)
	}
}

// TestBuildStep_ClaudeStepWithVarSubstitution verifies {{VAR}} tokens in a
// prompt file are substituted with VarTable-bound values.
func TestBuildStep_ClaudeStepWithVarSubstitution(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	promptContent := "implement issue {{ISSUE_ID}} from sha {{STARTING_SHA}}"
	if err := os.WriteFile(filepath.Join(dir, "prompts", "subst-prompt.txt"), []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}

	step := steps.Step{
		Name:       "subst-step",
		IsClaude:   true,
		Model:      "claude-opus-4-6",
		PromptFile: "subst-prompt.txt",
	}

	vt := vars.New(dir, 0)
	vt.SetPhase(vars.Iteration)
	vt.Bind(vars.Iteration, "ISSUE_ID", "42")
	vt.Bind(vars.Iteration, "STARTING_SHA", "abc123")
	resolved, err := buildStep(dir, step, vt, vars.Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved.Command) < 7 {
		t.Fatalf("expected at least 7 command elements, got %d: %v", len(resolved.Command), resolved.Command)
	}
	want := "implement issue 42 from sha abc123"
	if got := resolved.Command[6]; got != want {
		t.Errorf("expected substituted prompt %q, got %q", want, got)
	}
}

// TestBuildStep_ClaudeStepMissingPromptFile verifies that a claude step with
// a missing prompt file returns an error containing the step name.
func TestBuildStep_ClaudeStepMissingPromptFile(t *testing.T) {
	dir := t.TempDir()

	step := steps.Step{
		Name:       "bad-step",
		IsClaude:   true,
		Model:      "some-model",
		PromptFile: "nonexistent.txt",
	}

	vt := vars.New(dir, 0)
	vt.SetPhase(vars.Iteration)
	_, err := buildStep(dir, step, vt, vars.Iteration)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "bad-step") {
		t.Errorf("expected error to contain step name %q, got %q", "bad-step", err.Error())
	}
}

// TestBuildStep_ClaudeStepFinalize verifies that a finalize claude step
// produces the correct CLI command.
func TestBuildStep_ClaudeStepFinalize(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "finalize-prompt.txt"), []byte("finalize this"), 0644); err != nil {
		t.Fatal(err)
	}

	step := steps.Step{
		Name:       "finalize-claude",
		IsClaude:   true,
		Model:      "claude-sonnet-4-6",
		PromptFile: "finalize-prompt.txt",
	}

	vt := vars.New(dir, 0)
	vt.SetPhase(vars.Finalize)
	resolved, err := buildStep(dir, step, vt, vars.Finalize)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Name != "finalize-claude" {
		t.Errorf("expected name %q, got %q", "finalize-claude", resolved.Name)
	}
	if len(resolved.Command) < 7 || resolved.Command[0] != "claude" {
		t.Fatalf("unexpected command: %v", resolved.Command)
	}
	if resolved.Command[3] != "--model" || resolved.Command[4] != "claude-sonnet-4-6" {
		t.Errorf("expected --model claude-sonnet-4-6, got %v %v", resolved.Command[3], resolved.Command[4])
	}
	if resolved.Command[6] != "finalize this" {
		t.Errorf("expected prompt %q, got %q", "finalize this", resolved.Command[6])
	}
}

// TestRun_IterationsRunOnNormalCompletion verifies that IterationsRun equals the
// configured iteration count when all iterations complete without a break.
func TestRun_IterationsRunOnNormalCompletion(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir: t.TempDir(),
		Iterations: 2,
		Steps:      nonClaudeSteps("step1"),
	}

	result := Run(executor, header, kh, cfg)

	if result.IterationsRun != 2 {
		t.Errorf("expected IterationsRun=2 on normal completion, got %d", result.IterationsRun)
	}
}

// TestRun_IterationsRunZeroOnInitializeQuit verifies that IterationsRun is zero
// when the workflow quits during the initialize phase before the loop begins.
func TestRun_IterationsRunZeroOnInitializeQuit(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	actions <- ui.ActionQuit
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		runStepErrors: []error{errors.New("init failed")},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		ProjectDir:      t.TempDir(),
		Iterations:      1,
		InitializeSteps: nonClaudeSteps("init-step"),
		Steps:           nonClaudeSteps("iter-step"),
	}

	result := Run(executor, header, kh, cfg)

	if result.IterationsRun != 0 {
		t.Errorf("expected IterationsRun=0 on initialize quit, got %d", result.IterationsRun)
	}
}

// TestRun_IterationsRunOnIterationQuit verifies that IterationsRun reflects the
// iteration index at the time of quit, not zero.
func TestRun_IterationsRunOnIterationQuit(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	actions <- ui.ActionQuit
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		runStepErrors: []error{errors.New("step failed")},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		ProjectDir: t.TempDir(),
		Iterations: 3,
		Steps:      nonClaudeSteps("iter-step"),
	}

	result := Run(executor, header, kh, cfg)

	if result.IterationsRun != 1 {
		t.Errorf("expected IterationsRun=1 on iteration quit, got %d", result.IterationsRun)
	}
}

// TestRun_SetPhaseStepsCalledPerIteration verifies that SetPhaseSteps is called
// once per iteration with the iteration step names, plus once for finalization.
func TestRun_SetPhaseStepsCalledPerIteration(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    2,
		Steps:         nonClaudeSteps("step1", "step2"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	// Expect 3 SetPhaseSteps calls: iteration 1, iteration 2, finalization.
	if len(header.phaseStepsCalls) != 3 {
		t.Fatalf("expected 3 SetPhaseSteps calls, got %d: %v", len(header.phaseStepsCalls), header.phaseStepsCalls)
	}

	wantIterNames := []string{"step1", "step2"}
	for i := 0; i < 2; i++ {
		got := header.phaseStepsCalls[i]
		if len(got) != len(wantIterNames) {
			t.Errorf("phaseStepsCalls[%d]: got %v, want %v", i, got, wantIterNames)
			continue
		}
		for j, name := range wantIterNames {
			if got[j] != name {
				t.Errorf("phaseStepsCalls[%d][%d]: got %q, want %q", i, j, got[j], name)
			}
		}
	}

	wantFinalNames := []string{"final1"}
	got := header.phaseStepsCalls[2]
	if len(got) != len(wantFinalNames) || got[0] != wantFinalNames[0] {
		t.Errorf("phaseStepsCalls[2] (finalization): got %v, want %v", got, wantFinalNames)
	}
}

// TestRun_FinalizationStepStateCallsUseCorrectIndices verifies that finalization
// step state updates use 0-based indices within the finalize phase, not
// continuation indices from the iteration phase.
func TestRun_FinalizationStepStateCallsUseCorrectIndices(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("iter1", "iter2", "iter3"),
		FinalizeSteps: nonClaudeSteps("final1", "final2"),
	}

	Run(executor, header, kh, cfg)

	// Each step produces SetStepState(idx, Active) and SetStepState(idx, Done).
	// Iteration: 3 steps × 2 calls = 6 calls (indices 0, 1, 2).
	// Finalization: 2 steps × 2 calls = 4 calls (indices must be 0, 1 — not 3, 4).
	totalCalls := len(header.stepStateCalls)
	if totalCalls < 10 {
		t.Fatalf("expected at least 10 step state calls (6 iter + 4 final), got %d", totalCalls)
	}

	finalCalls := header.stepStateCalls[totalCalls-4:]
	for _, call := range finalCalls {
		if call.idx > 1 {
			t.Errorf("finalization step state call used index %d, want 0 or 1 (not a continuation of iteration indices)", call.idx)
		}
	}
}

// TestRun_FinalizationPhaseStepsSetAfterBreak verifies that SetPhaseSteps is
// still called with finalization step names even when the iteration loop exits
// early via breakLoopIfEmpty.
func TestRun_FinalizationPhaseStepsSetAfterBreak(t *testing.T) {
	executor := &fakeExecutor{
		runStepCaptures: []string{""}, // break step captures empty → exit loop
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:    t.TempDir(),
		Iterations:    3,
		Steps:         []steps.Step{breakStep("get-issue", "ISSUE_ID")},
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	// phaseStepsCalls: [0] = iteration names, [1] = finalization names.
	if len(header.phaseStepsCalls) < 2 {
		t.Fatalf("expected at least 2 SetPhaseSteps calls (iteration + finalization), got %d", len(header.phaseStepsCalls))
	}

	last := header.phaseStepsCalls[len(header.phaseStepsCalls)-1]
	if len(last) != 1 || last[0] != "final1" {
		t.Errorf("last SetPhaseSteps call (finalization): got %v, want [final1]", last)
	}
}

// TestRun_InitializeDoesNotCallSetPhaseSteps verifies that the initialize phase
// does not call header.SetPhaseSteps — the first phaseStepsCalls entry is the
// iteration step names, not the initialize step names.
func TestRun_InitializeDoesNotCallSetPhaseSteps(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		ProjectDir:      t.TempDir(),
		Iterations:      1,
		InitializeSteps: nonClaudeSteps("init1", "init2"),
		Steps:           nonClaudeSteps("iter-step"),
		FinalizeSteps:   nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	if len(header.phaseStepsCalls) == 0 {
		t.Fatal("expected at least one SetPhaseSteps call")
	}

	// The first SetPhaseSteps call must be for the iteration phase, not initialize.
	first := header.phaseStepsCalls[0]
	if len(first) != 1 || first[0] != "iter-step" {
		t.Errorf("phaseStepsCalls[0]: got %v, want [iter-step] (initialize must not call SetPhaseSteps)", first)
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

// TestLastCapture_LastNonEmptyStdoutLine verifies Runner.LastCapture returns
// the last non-empty stdout line after a successful RunStep.
func TestLastCapture_LastNonEmptyStdoutLine(t *testing.T) {
	logDir := t.TempDir()
	log, err := logger.NewLogger(logDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()

	runner := NewRunner(log, logDir)
	collect := collectLines(t, runner)

	// Print three lines; last non-empty should be "third".
	if err := runner.RunStep("test", []string{"sh", "-c", "printf 'first\nsecond\nthird\n'"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = runner.Close()
	_ = collect()

	if got := runner.LastCapture(); got != "third" {
		t.Errorf("LastCapture: got %q, want %q", got, "third")
	}
}

// TestLastCapture_EmptyOnFailure verifies LastCapture returns "" when RunStep fails.
func TestLastCapture_EmptyOnFailure(t *testing.T) {
	logDir := t.TempDir()
	log, err := logger.NewLogger(logDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()

	runner := NewRunner(log, logDir)
	collect := collectLines(t, runner)

	_ = runner.RunStep("test", []string{"sh", "-c", "echo something; exit 1"})
	_ = runner.Close()
	_ = collect()

	if got := runner.LastCapture(); got != "" {
		t.Errorf("LastCapture after failure: got %q, want empty string", got)
	}
}

// TestLastCapture_StripsTrailingCarriageReturn verifies that lines ending with
// \r are stripped before being returned as the capture value.
func TestLastCapture_StripsTrailingCarriageReturn(t *testing.T) {
	logDir := t.TempDir()
	log, err := logger.NewLogger(logDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()

	runner := NewRunner(log, logDir)
	collect := collectLines(t, runner)

	// Print a line with a trailing \r (CRLF-style, common in some scripts).
	if err := runner.RunStep("test", []string{"printf", "hello\r\n"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = runner.Close()
	_ = collect()

	if got := runner.LastCapture(); got != "hello" {
		t.Errorf("LastCapture: got %q, want %q", got, "hello")
	}
}

// TestLastCapture_StderrNotCaptured verifies that output written only to stderr
// does not appear in LastCapture. The forwardAndCapture function is wired to
// stdout only; stderr is handled by forward, which does not accumulate lines.
func TestLastCapture_StderrNotCaptured(t *testing.T) {
	logDir := t.TempDir()
	log, err := logger.NewLogger(logDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()

	runner := NewRunner(log, logDir)
	collect := collectLines(t, runner)

	if err := runner.RunStep("test", []string{"sh", "-c", "echo stderr-only >&2"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	_ = runner.Close()
	_ = collect()

	if got := runner.LastCapture(); got != "" {
		t.Errorf("LastCapture: got %q, want empty string (stderr should not be captured)", got)
	}
}

// TestRun_Integration_FullFlow runs the orchestration end-to-end with fake
// scripts and real subprocesses — verifying the full path from initialize phase
// through iteration and finalization.
func TestRun_Integration_FullFlow(t *testing.T) {
	projectDir := t.TempDir()
	workingDir := t.TempDir()

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

	initSteps := []steps.Step{
		{Name: "Get GitHub user", IsClaude: false, Command: []string{"scripts/get_gh_user"}, CaptureAs: "GITHUB_USER"},
	}
	iterSteps := []steps.Step{
		{Name: "Get next issue", IsClaude: false, Command: []string{"scripts/get_next_issue", "{{GITHUB_USER}}"}, CaptureAs: "ISSUE_ID", BreakLoopIfEmpty: true},
		{Name: "Echo iter", IsClaude: false, Command: []string{"echo", "iteration step done"}},
	}
	finalSteps := []steps.Step{
		{Name: "Echo final", IsClaude: false, Command: []string{"echo", "finalization done"}},
	}

	cfg := RunConfig{
		ProjectDir:      projectDir,
		Iterations:      1,
		InitializeSteps: initSteps,
		Steps:           iterSteps,
		FinalizeSteps:   finalSteps,
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
