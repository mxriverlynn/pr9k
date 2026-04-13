package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	projectDir      string
	// onLog, when non-nil, is invoked for every line passed to WriteToLog.
	// Tests use it to observe the log stream from another goroutine without
	// racing on logLines. The callback runs synchronously on the writer
	// goroutine, so happens-before the receiver of any channel it sends to.
	onLog func(line string)
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
	if f.onLog != nil {
		f.onLog(line)
	}
}

func (f *fakeExecutor) LastCapture() string {
	return f.lastCapture
}

func (f *fakeExecutor) ProjectDir() string {
	return f.projectDir
}

type fakeRunHeader struct {
	renderInitializeCalls []renderPhaseCall
	renderIterationCalls  []renderIterCall
	renderFinalizeCalls   []renderPhaseCall
	stepStateCalls        []stepStateCall
	phaseStepsCalls       [][]string
}

type renderPhaseCall struct {
	stepNum, stepCount int
	stepName           string
}

type renderIterCall struct {
	iter, maxIter int
	issueID       string
}

type stepStateCall struct {
	idx   int
	state ui.StepState
}

func (h *fakeRunHeader) RenderInitializeLine(stepNum, stepCount int, stepName string) {
	h.renderInitializeCalls = append(h.renderInitializeCalls, renderPhaseCall{stepNum, stepCount, stepName})
}
func (h *fakeRunHeader) RenderIterationLine(iter, maxIter int, issueID string) {
	h.renderIterationCalls = append(h.renderIterationCalls, renderIterCall{iter, maxIter, issueID})
}
func (h *fakeRunHeader) RenderFinalizeLine(stepNum, stepCount int, stepName string) {
	h.renderFinalizeCalls = append(h.renderFinalizeCalls, renderPhaseCall{stepNum, stepCount, stepName})
}

func (h *fakeRunHeader) SetPhaseSteps(names []string) {
	cp := make([]string, len(names))
	copy(cp, names)
	h.phaseStepsCalls = append(h.phaseStepsCalls, cp)
}

func (h *fakeRunHeader) SetStepState(idx int, state ui.StepState) {
	h.stepStateCalls = append(h.stepStateCalls, stepStateCall{idx, state})
}

// newTestKeyHandler creates a KeyHandler suitable for tests where all steps
// succeed. Run() returns on its own once the workflow finishes, so the handler
// simply provides a buffered actions channel that no one consumes.
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
		WorkflowDir:   t.TempDir(),
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
		WorkflowDir:   t.TempDir(),
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
	if header.renderIterationCalls[0].iter != 1 {
		t.Errorf("iteration 1: want iter=1, got %d", header.renderIterationCalls[0].iter)
	}
	if header.renderIterationCalls[0].maxIter != 2 {
		t.Errorf("iteration 1: want maxIter=2, got %d", header.renderIterationCalls[0].maxIter)
	}
	if header.renderIterationCalls[1].iter != 2 {
		t.Errorf("iteration 2: want iter=2, got %d", header.renderIterationCalls[1].iter)
	}
	if header.renderIterationCalls[1].maxIter != 2 {
		t.Errorf("iteration 2: want maxIter=2, got %d", header.renderIterationCalls[1].maxIter)
	}
}

// TestRun_UnlimitedIterations verifies that Iterations==0 runs until a
// breakLoopIfEmpty step returns empty capture. The loop must not run forever.
func TestRun_UnlimitedIterations(t *testing.T) {
	// Iteration 1: get-issue → "issue-1" (non-empty → continue)
	// Iteration 2: get-issue → "" (empty AND BreakLoopIfEmpty → break)
	// Finalize: final1
	executor := &fakeExecutor{
		runStepCaptures: []string{"issue-1", "", ""},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    0, // unlimited
		Steps:         []steps.Step{breakStep("get-issue", "ISSUE_ID")},
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	result := Run(executor, header, kh, cfg)

	if result.IterationsRun != 2 {
		t.Errorf("expected IterationsRun=2, got %d", result.IterationsRun)
	}

	ranFinal := false
	for _, call := range executor.runStepCalls {
		if call.name == "final1" {
			ranFinal = true
		}
	}
	if !ranFinal {
		t.Error("expected finalization to run after unlimited loop exit")
	}
}

// TestRun_NegativeIterationsRunsZeroIterations verifies that a negative
// Iterations value (e.g. -1) silently produces zero iterations because the
// loop condition `cfg.Iterations == 0 || i <= cfg.Iterations` is false on
// the very first evaluation: -1 != 0 and 1 <= -1 is false. Finalization still
// executes. IterationsRun returns the last value of i that was committed, which
// is 1 because the counter increments before the check fails — however, no step
// actually runs during that iteration since we break immediately.
//
// Note: the caller is responsible for validating Iterations >= 0 before calling
// Run. This test documents the current behaviour so that any future change to
// the loop condition is noticed.
func TestRun_NegativeIterationsRunsZeroIterations(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    -1, // invalid; loop condition is false immediately
		Steps:         nonClaudeSteps("iter-step"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	// No iteration step should have run.
	for _, call := range executor.runStepCalls {
		if call.name == "iter-step" {
			t.Errorf("iter-step must not run when Iterations=-1, but it did")
		}
	}

	// Finalization still runs.
	ranFinal := false
	for _, call := range executor.runStepCalls {
		if call.name == "final1" {
			ranFinal = true
		}
	}
	if !ranFinal {
		t.Error("expected finalization to run even when Iterations=-1")
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
		WorkflowDir:   t.TempDir(),
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
		WorkflowDir:   t.TempDir(),
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
		WorkflowDir:   t.TempDir(),
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
	// After ActionContinue is consumed, remaining steps complete near-instantly.
	// Inject ActionQuit to unblock the completion sequence's final blocking receive.
	time.Sleep(10 * time.Millisecond)
	actions <- ui.ActionQuit

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
		WorkflowDir:     t.TempDir(),
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
		WorkflowDir:     dir,
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
		WorkflowDir:   t.TempDir(),
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
		WorkflowDir:   t.TempDir(),
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
		WorkflowDir:   t.TempDir(),
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

// TestRun_StepBuildErrorAbortsAllRemainingIterations verifies that when
// buildStep fails during the iteration loop with Iterations > 1, the entire
// loop is aborted — not just the current iteration. This is distinct from the
// initialize phase, which continues to the next step on a build error.
func TestRun_StepBuildErrorAbortsAllRemainingIterations(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	badStep := steps.Step{
		Name:       "bad-claude",
		IsClaude:   true,
		Model:      "some-model",
		PromptFile: "nonexistent.txt",
	}

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    3, // three iterations configured; error on first should abort all
		Steps:         []steps.Step{badStep},
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	// Iteration step must never have executed (build failed before RunStep).
	for _, call := range executor.runStepCalls {
		if call.name == "bad-claude" {
			t.Errorf("bad-claude step should not have run (build error)")
		}
	}

	// Finalization still runs.
	ranFinal := false
	for _, call := range executor.runStepCalls {
		if call.name == "final1" {
			ranFinal = true
		}
	}
	if !ranFinal {
		t.Error("expected finalization to run after iteration build error")
	}

	// Only one "Error preparing steps" message should appear (iteration 1 aborts
	// the loop; iterations 2 and 3 never start).
	errCount := 0
	for _, line := range executor.logLines {
		if strings.Contains(line, "Error preparing steps") {
			errCount++
		}
	}
	if errCount != 1 {
		t.Errorf("expected exactly 1 'Error preparing steps' log line (loop aborted after first iteration), got %d", errCount)
	}
}

// TestRun_QuitFromIterationSkipsFinalization verifies that when Orchestrate
// returns ActionQuit during an iteration, Run skips finalization.
func TestRun_QuitFromIterationSkipsFinalization(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	actions <- ui.ActionQuit
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		runStepErrors: []error{errors.New("step failed")},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("iter-step"),
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	for _, call := range executor.runStepCalls {
		if call.name == "final1" {
			t.Error("finalization step should not have run after iteration quit")
		}
	}
}

// TestRun_QuitFromFinalizationReturnsWithoutSummary verifies that when
// Orchestrate returns ActionQuit during finalization, Run returns without
// writing the completion summary.
func TestRun_QuitFromFinalizationReturnsWithoutSummary(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	actions <- ui.ActionQuit
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		// index 0 = iteration step (succeeds), index 1 = finalize step (fails)
		runStepErrors: []error{nil, errors.New("finalize failed")},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("iter-step"),
		FinalizeSteps: nonClaudeSteps("final-step"),
	}

	Run(executor, header, kh, cfg)
}

// TestRun_QuitFromInitializeSkipsRemainingPhases verifies that ActionQuit
// during the initialize phase skips all iteration and finalization steps.
func TestRun_QuitFromInitializeSkipsRemainingPhases(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	actions <- ui.ActionQuit
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		runStepErrors: []error{errors.New("init failed")},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir:     t.TempDir(),
		Iterations:      1,
		InitializeSteps: nonClaudeSteps("init-step"),
		Steps:           nonClaudeSteps("iter-step"),
		FinalizeSteps:   nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

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
		WorkflowDir:     t.TempDir(),
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

	vt := vars.New(dir, dir, 0)
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
	if resolved.Command[1] != "--permission-mode" || resolved.Command[2] != "bypassPermissions" {
		t.Errorf("expected --permission-mode bypassPermissions, got %v %v", resolved.Command[1], resolved.Command[2])
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

	vt := vars.New(dir, dir, 0)
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

	vt := vars.New(dir, dir, 0)
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

	vt := vars.New(dir, dir, 0)
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
		WorkflowDir: t.TempDir(),
		Iterations:  2,
		Steps:       nonClaudeSteps("step1"),
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
		WorkflowDir:     t.TempDir(),
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
		WorkflowDir: t.TempDir(),
		Iterations:  3,
		Steps:       nonClaudeSteps("iter-step"),
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
		WorkflowDir:   t.TempDir(),
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
	for i := range 2 {
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
		WorkflowDir:   t.TempDir(),
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
		WorkflowDir:   t.TempDir(),
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
		WorkflowDir:     t.TempDir(),
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

// TestRun_IterationHeaderUpdatesAfterCaptureAs verifies that RenderIterationLine
// is called at iteration start with an empty issueID, then again with the bound
// ISSUE_ID value after the captureAs step completes.
func TestRun_IterationHeaderUpdatesAfterCaptureAs(t *testing.T) {
	executor := &fakeExecutor{
		runStepCaptures: []string{"42"}, // get-issue captures "42" as ISSUE_ID
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  3,
		Steps:       []steps.Step{captureStep("get-issue", "ISSUE_ID")},
	}

	Run(executor, header, kh, cfg)

	// Iteration 1: start call (issueID="") + captureAs call (issueID="42").
	// Subsequent iterations: start call ("") + captureAs call ("42" from new capture).
	// We verify just the first two calls for iteration 1.
	if len(header.renderIterationCalls) < 2 {
		t.Fatalf("expected at least 2 RenderIterationLine calls for iteration 1, got %d", len(header.renderIterationCalls))
	}

	start := header.renderIterationCalls[0]
	if start.iter != 1 || start.maxIter != 3 || start.issueID != "" {
		t.Errorf("iteration 1 start: want {1, 3, \"\"}, got {%d, %d, %q}", start.iter, start.maxIter, start.issueID)
	}

	after := header.renderIterationCalls[1]
	if after.iter != 1 || after.maxIter != 3 || after.issueID != "42" {
		t.Errorf("iteration 1 after captureAs: want {1, 3, \"42\"}, got {%d, %d, %q}", after.iter, after.maxIter, after.issueID)
	}
}

// TestRun_SecondIterationStartsWithEmptyIssueID verifies that when a new
// iteration begins, RenderIterationLine is called with an empty issueID
// (the iteration table is reset), even after a prior iteration bound ISSUE_ID.
func TestRun_SecondIterationStartsWithEmptyIssueID(t *testing.T) {
	executor := &fakeExecutor{
		runStepCaptures: []string{"42", "99"}, // iter1 captures "42", iter2 captures "99"
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  2,
		Steps:       []steps.Step{captureStep("get-issue", "ISSUE_ID")},
	}

	Run(executor, header, kh, cfg)

	// Expected sequence: iter1-start(""), iter1-capture("42"), iter2-start(""), iter2-capture("99")
	if len(header.renderIterationCalls) != 4 {
		t.Fatalf("expected 4 RenderIterationLine calls (2 per iteration), got %d: %v",
			len(header.renderIterationCalls), header.renderIterationCalls)
	}

	iter2Start := header.renderIterationCalls[2]
	if iter2Start.iter != 2 || iter2Start.issueID != "" {
		t.Errorf("iteration 2 start: want {iter=2, issueID=\"\"}, got {iter=%d, issueID=%q}",
			iter2Start.iter, iter2Start.issueID)
	}
}

// TestRun_NonCapturingIterStepDoesNotRerenderHeader verifies that a step
// without captureAs does not cause an additional RenderIterationLine call.
// Only the iteration-start call should appear.
func TestRun_NonCapturingIterStepDoesNotRerenderHeader(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       nonClaudeSteps("work-step"), // no captureAs
	}

	Run(executor, header, kh, cfg)

	// Exactly 1 call: the iteration-start render. No re-render after non-capturing step.
	if len(header.renderIterationCalls) != 1 {
		t.Errorf("expected 1 RenderIterationLine call (iteration start only), got %d: %v",
			len(header.renderIterationCalls), header.renderIterationCalls)
	}
	if header.renderIterationCalls[0].issueID != "" {
		t.Errorf("iteration start: want empty issueID, got %q", header.renderIterationCalls[0].issueID)
	}
}

// TestRun_InitializeRenderCalledPerStep verifies RenderInitializeLine is called
// once per initialize step with the correct step number and step name.
func TestRun_InitializeRenderCalledPerStep(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:     t.TempDir(),
		Iterations:      1,
		InitializeSteps: nonClaudeSteps("init-a", "init-b"),
		Steps:           nonClaudeSteps("iter-step"),
	}

	Run(executor, header, kh, cfg)

	if len(header.renderInitializeCalls) != 2 {
		t.Fatalf("expected 2 RenderInitializeLine calls, got %d: %v",
			len(header.renderInitializeCalls), header.renderInitializeCalls)
	}
	if got := header.renderInitializeCalls[0]; got.stepNum != 1 || got.stepCount != 2 || got.stepName != "init-a" {
		t.Errorf("init step 1: want {1, 2, \"init-a\"}, got {%d, %d, %q}", got.stepNum, got.stepCount, got.stepName)
	}
	if got := header.renderInitializeCalls[1]; got.stepNum != 2 || got.stepCount != 2 || got.stepName != "init-b" {
		t.Errorf("init step 2: want {2, 2, \"init-b\"}, got {%d, %d, %q}", got.stepNum, got.stepCount, got.stepName)
	}
}

// TestRun_FinalizeRenderCalledPerStep verifies RenderFinalizeLine is called
// once per finalize step with the correct step number and step name.
func TestRun_FinalizeRenderCalledPerStep(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("iter-step"),
		FinalizeSteps: nonClaudeSteps("final-a", "final-b"),
	}

	Run(executor, header, kh, cfg)

	if len(header.renderFinalizeCalls) != 2 {
		t.Fatalf("expected 2 RenderFinalizeLine calls, got %d: %v",
			len(header.renderFinalizeCalls), header.renderFinalizeCalls)
	}
	if got := header.renderFinalizeCalls[0]; got.stepNum != 1 || got.stepCount != 2 || got.stepName != "final-a" {
		t.Errorf("finalize step 1: want {1, 2, \"final-a\"}, got {%d, %d, %q}", got.stepNum, got.stepCount, got.stepName)
	}
	if got := header.renderFinalizeCalls[1]; got.stepNum != 2 || got.stepCount != 2 || got.stepName != "final-b" {
		t.Errorf("finalize step 2: want {2, 2, \"final-b\"}, got {%d, %d, %q}", got.stepNum, got.stepCount, got.stepName)
	}
}

// TestRun_InitializeBuildErrorSkipsRenderInitializeLine verifies that
// RenderInitializeLine is NOT called for an initialize step when buildStep
// fails, but IS called for the subsequent valid step.
func TestRun_InitializeBuildErrorSkipsRenderInitializeLine(t *testing.T) {
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
		WorkflowDir:     t.TempDir(),
		Iterations:      1,
		InitializeSteps: []steps.Step{badInitStep, nonClaudeSteps("good-init")[0]},
		Steps:           nonClaudeSteps("iter-step"),
	}

	Run(executor, header, kh, cfg)

	if len(header.renderInitializeCalls) != 1 {
		t.Fatalf("expected 1 RenderInitializeLine call (bad step skipped), got %d: %v",
			len(header.renderInitializeCalls), header.renderInitializeCalls)
	}
	got := header.renderInitializeCalls[0]
	if got.stepNum != 2 || got.stepCount != 2 || got.stepName != "good-init" {
		t.Errorf("expected {stepNum=2, stepCount=2, stepName=%q}, got {%d, %d, %q}",
			"good-init", got.stepNum, got.stepCount, got.stepName)
	}
}

// TestRun_FinalizeBuildErrorSkipsRenderFinalizeLine verifies that
// RenderFinalizeLine is NOT called for a finalize step when buildStep fails,
// but IS called for the subsequent valid step.
func TestRun_FinalizeBuildErrorSkipsRenderFinalizeLine(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	badFinalStep := steps.Step{
		Name:       "bad-final",
		IsClaude:   true,
		Model:      "some-model",
		PromptFile: "nonexistent.txt",
	}

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("iter-step"),
		FinalizeSteps: []steps.Step{badFinalStep, nonClaudeSteps("good-final")[0]},
	}

	Run(executor, header, kh, cfg)

	foundErr := false
	for _, line := range executor.logLines {
		if strings.Contains(line, "Error preparing finalize step") {
			foundErr = true
		}
	}
	if !foundErr {
		t.Errorf("expected 'Error preparing finalize step' in log, got %v", executor.logLines)
	}

	if len(header.renderFinalizeCalls) != 1 {
		t.Fatalf("expected 1 RenderFinalizeLine call (bad step skipped), got %d: %v",
			len(header.renderFinalizeCalls), header.renderFinalizeCalls)
	}
	got := header.renderFinalizeCalls[0]
	if got.stepNum != 2 || got.stepCount != 2 || got.stepName != "good-final" {
		t.Errorf("expected {stepNum=2, stepCount=2, stepName=%q}, got {%d, %d, %q}",
			"good-final", got.stepNum, got.stepCount, got.stepName)
	}

	foundGoodFinal := false
	for _, call := range executor.runStepCalls {
		if call.name == "good-final" {
			foundGoodFinal = true
			break
		}
	}
	if !foundGoodFinal {
		t.Errorf("expected 'good-final' step to execute via RunStep after bad step, got calls: %v", executor.runStepCalls)
	}
}

// TestRun_LogWidthZero_FallsBackToDefaultTerminalWidth verifies that when LogWidth
// is 0 (or negative), Run uses ui.DefaultTerminalWidth for phase banner underlines.
func TestRun_LogWidthZero_FallsBackToDefaultTerminalWidth(t *testing.T) {
	for _, logWidth := range []int{0, -1} {
		t.Run(fmt.Sprintf("LogWidth=%d", logWidth), func(t *testing.T) {
			executor := &fakeExecutor{}
			header := &fakeRunHeader{}
			kh := newTestKeyHandler()

			cfg := RunConfig{
				WorkflowDir: t.TempDir(),
				Iterations:  1,
				Steps:       nonClaudeSteps("step1"),
				LogWidth:    logWidth,
			}

			Run(executor, header, kh, cfg)

			// Find the phase banner underline: a line composed entirely of '═' runes.
			foundUnderline := false
			for _, line := range executor.logLines {
				if len(line) == 0 {
					continue
				}
				allDouble := true
				for _, r := range line {
					if r != '═' {
						allDouble = false
						break
					}
				}
				if allDouble {
					got := len([]rune(line))
					if got != ui.DefaultTerminalWidth {
						t.Errorf("LogWidth=%d: phase banner underline rune count = %d, want %d (DefaultTerminalWidth)", logWidth, got, ui.DefaultTerminalWidth)
					}
					foundUnderline = true
					break
				}
			}
			if !foundUnderline {
				t.Errorf("LogWidth=%d: no '═' phase banner underline found in log lines: %v", logWidth, executor.logLines)
			}
		})
	}
}

// TestRun_LogWidthPositive_UsesThatWidthForPhaseBanner verifies that when
// LogWidth is set to a positive value, Run uses that width for phase banner
// underlines (mirrors the zero-width fallback test with a real value).
func TestRun_LogWidthPositive_UsesThatWidthForPhaseBanner(t *testing.T) {
	const wantWidth = 40

	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       nonClaudeSteps("step1"),
		LogWidth:    wantWidth,
	}

	Run(executor, header, kh, cfg)

	foundUnderline := false
	for _, line := range executor.logLines {
		if len(line) == 0 {
			continue
		}
		allDouble := true
		for _, r := range line {
			if r != '═' {
				allDouble = false
				break
			}
		}
		if allDouble {
			got := len([]rune(line))
			if got != wantWidth {
				t.Errorf("phase banner underline rune count = %d, want %d (LogWidth)", got, wantWidth)
			}
			foundUnderline = true
			break
		}
	}
	if !foundUnderline {
		t.Errorf("no '═' phase banner underline found in log lines: %v", executor.logLines)
	}
}

// TestRun_CaptureAsNonIssueIDProducesEmptyIssueIDInHeader verifies that when a
// captureAs step binds a variable other than "ISSUE_ID", the re-render of the
// iteration header still uses an empty issueID (because the lookup key is
// hardcoded to "ISSUE_ID").
func TestRun_CaptureAsNonIssueIDProducesEmptyIssueIDInHeader(t *testing.T) {
	executor := &fakeExecutor{
		runStepCaptures: []string{"abc123"},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       []steps.Step{captureStep("get-sha", "STARTING_SHA")},
	}

	Run(executor, header, kh, cfg)

	// 2 calls expected: iteration-start (empty) + re-render after captureAs (also empty)
	if len(header.renderIterationCalls) != 2 {
		t.Fatalf("expected 2 RenderIterationLine calls, got %d: %v",
			len(header.renderIterationCalls), header.renderIterationCalls)
	}
	for i, call := range header.renderIterationCalls {
		if call.issueID != "" {
			t.Errorf("renderIterationCalls[%d]: want empty issueID, got %q", i, call.issueID)
		}
	}
}

// TestRun_QuitFromInitializeProducesZeroIterationAndFinalizeHeaderCalls verifies
// that when ActionQuit fires during the initialize phase, no RenderIterationLine
// or RenderFinalizeLine calls are made, and RenderInitializeLine is called once
// (for the step where quit fires, render occurs before Orchestrate).
func TestRun_QuitFromInitializeProducesZeroIterationAndFinalizeHeaderCalls(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	actions <- ui.ActionQuit
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		runStepErrors: []error{errors.New("init failed")},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir:     t.TempDir(),
		Iterations:      1,
		InitializeSteps: nonClaudeSteps("init-step"),
		Steps:           nonClaudeSteps("iter-step"),
		FinalizeSteps:   nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	if len(header.renderInitializeCalls) != 1 {
		t.Errorf("expected 1 RenderInitializeLine call (before quit), got %d", len(header.renderInitializeCalls))
	}
	if len(header.renderIterationCalls) != 0 {
		t.Errorf("expected 0 RenderIterationLine calls after initialize quit, got %d", len(header.renderIterationCalls))
	}
	if len(header.renderFinalizeCalls) != 0 {
		t.Errorf("expected 0 RenderFinalizeLine calls after initialize quit, got %d", len(header.renderFinalizeCalls))
	}
}

// TestRun_QuitDuringFinalizeRecordsOnlyTheQuittingStepRender verifies that when
// ActionQuit fires during the first finalize step's error mode, RenderFinalizeLine
// is called exactly once (render happens before Orchestrate) and subsequent
// finalize steps are not rendered.
func TestRun_QuitDuringFinalizeRecordsOnlyTheQuittingStepRender(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		// index 0 = iter-step succeeds, index 1 = final-a fails → enters error mode
		runStepErrors: []error{nil, errors.New("finalize failed")},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("iter-step"),
		FinalizeSteps: nonClaudeSteps("final-a", "final-b"),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()

	// Let Orchestrate reach error mode for final-a, then send ActionQuit.
	time.Sleep(30 * time.Millisecond)
	actions <- ui.ActionQuit

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete after ActionQuit")
	}

	if len(header.renderFinalizeCalls) != 1 {
		t.Fatalf("expected 1 RenderFinalizeLine call (quit on first finalize step), got %d: %v",
			len(header.renderFinalizeCalls), header.renderFinalizeCalls)
	}
	got := header.renderFinalizeCalls[0]
	if got.stepNum != 1 || got.stepCount != 2 || got.stepName != "final-a" {
		t.Errorf("expected {stepNum=1, stepCount=2, stepName=%q}, got {%d, %d, %q}",
			"final-a", got.stepNum, got.stepCount, got.stepName)
	}
}

// TestRun_FinalizeRenderCalledAfterBreakLoopIfEmpty verifies that
// RenderFinalizeLine is called for finalize steps even when the iteration loop
// exits early via breakLoopIfEmpty.
func TestRun_FinalizeRenderCalledAfterBreakLoopIfEmpty(t *testing.T) {
	executor := &fakeExecutor{
		runStepCaptures: []string{""}, // break step captures empty → exit loop
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    3,
		Steps:         []steps.Step{breakStep("get-issue", "ISSUE_ID")},
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	if len(header.renderFinalizeCalls) != 1 {
		t.Fatalf("expected 1 RenderFinalizeLine call after early loop break, got %d: %v",
			len(header.renderFinalizeCalls), header.renderFinalizeCalls)
	}
	got := header.renderFinalizeCalls[0]
	if got.stepNum != 1 || got.stepCount != 1 || got.stepName != "final1" {
		t.Errorf("expected {stepNum=1, stepCount=1, stepName=%q}, got {%d, %d, %q}",
			"final1", got.stepNum, got.stepCount, got.stepName)
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

// TestCaptureOutput_UsesProjectDir verifies CaptureOutput sets cmd.Dir to the
// runner's project directory (target repo) for every subprocess.
func TestCaptureOutput_UsesProjectDir(t *testing.T) {
	projectDir := t.TempDir()
	logDir := t.TempDir()
	log, err := logger.NewLogger(logDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()

	runner := NewRunner(log, projectDir)

	out, err := runner.CaptureOutput([]string{"sh", "-c", "pwd"})
	if err != nil {
		t.Fatalf("CaptureOutput: %v", err)
	}

	// Resolve symlinks for comparison (macOS temp dirs may be symlinked).
	wantDir, _ := filepath.EvalSymlinks(projectDir)
	gotDir, _ := filepath.EvalSymlinks(out)

	if gotDir != wantDir {
		t.Errorf("expected CaptureOutput cmd.Dir=%q, got %q", wantDir, gotDir)
	}
}

// TestCaptureOutput_ReturnsTrimmedStdout verifies CaptureOutput returns trimmed
// stdout on success (T1).
func TestCaptureOutput_ReturnsTrimmedStdout(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()

	runner := NewRunner(log, dir)

	out, err := runner.CaptureOutput([]string{"sh", "-c", "echo '  hello  '"})
	if err != nil {
		t.Fatalf("CaptureOutput: %v", err)
	}
	if out != "hello" {
		t.Errorf("expected trimmed output %q, got %q", "hello", out)
	}
}

// TestCaptureOutput_ReturnsErrorForFailingCommand verifies CaptureOutput returns
// a non-nil error when the command exits with a non-zero status (T2).
func TestCaptureOutput_ReturnsErrorForFailingCommand(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()

	runner := NewRunner(log, dir)

	_, err = runner.CaptureOutput([]string{"sh", "-c", "exit 1"})
	if err == nil {
		t.Error("expected non-nil error for failing command, got nil")
	}
}

// TestCaptureOutput_ReturnsErrorForNonExistentCommand verifies CaptureOutput
// returns a non-nil error when the command does not exist (T3).
func TestCaptureOutput_ReturnsErrorForNonExistentCommand(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()

	runner := NewRunner(log, dir)

	_, err = runner.CaptureOutput([]string{"__no_such_binary_exists__"})
	if err == nil {
		t.Error("expected non-nil error for non-existent command, got nil")
	}
}

// TestCaptureOutput_DiscardsStderr verifies CaptureOutput returns only stdout
// and ignores stderr output (T4).
func TestCaptureOutput_DiscardsStderr(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()

	runner := NewRunner(log, dir)

	out, err := runner.CaptureOutput([]string{"sh", "-c", "echo stdout; echo stderr >&2"})
	if err != nil {
		t.Fatalf("CaptureOutput: %v", err)
	}
	if out != "stdout" {
		t.Errorf("expected only stdout %q, got %q", "stdout", out)
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
	var captureMu sync.Mutex
	var captured []string
	runner.SetSender(func(line string) {
		captureMu.Lock()
		captured = append(captured, line)
		captureMu.Unlock()
	})

	// Print three lines; last non-empty should be "third".
	if err := runner.RunStep("test", []string{"sh", "-c", "printf 'first\nsecond\nthird\n'"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

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
	var captureMu sync.Mutex
	var captured []string
	runner.SetSender(func(line string) {
		captureMu.Lock()
		captured = append(captured, line)
		captureMu.Unlock()
	})

	_ = runner.RunStep("test", []string{"sh", "-c", "echo something; exit 1"})

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
	var captureMu sync.Mutex
	var captured []string
	runner.SetSender(func(line string) {
		captureMu.Lock()
		captured = append(captured, line)
		captureMu.Unlock()
	})

	// Print a line with a trailing \r (CRLF-style, common in some scripts).
	if err := runner.RunStep("test", []string{"printf", "hello\r\n"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

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
	var captureMu sync.Mutex
	var captured []string
	runner.SetSender(func(line string) {
		captureMu.Lock()
		captured = append(captured, line)
		captureMu.Unlock()
	})

	if err := runner.RunStep("test", []string{"sh", "-c", "echo stderr-only >&2"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

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
	var captureMu sync.Mutex
	var captured []string
	runner.SetSender(func(line string) {
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

	// Actions channel for KeyHandler.
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)
	// Inject ActionQuit to unblock the completion sequence after all steps finish.
	// Real subprocesses take only milliseconds; 500ms is sufficient margin.
	go func() {
		time.Sleep(500 * time.Millisecond)
		actions <- ui.ActionQuit
	}()

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
		WorkflowDir:     projectDir,
		Iterations:      1,
		InitializeSteps: initSteps,
		Steps:           iterSteps,
		FinalizeSteps:   finalSteps,
	}

	header := &fakeRunHeader{}
	Run(runner, header, kh, cfg)

	collected := drain()
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

// TestRun_BreakLoopIfEmpty_MarksRemainingStepsSkipped verifies that when a
// step with BreakLoopIfEmpty triggers (StepDone + empty capture), all
// subsequent iteration steps are marked StepSkipped in the header.
func TestRun_BreakLoopIfEmpty_MarksRemainingStepsSkipped(t *testing.T) {
	// breakStep at index 0 returns empty capture → triggers break.
	// steps at index 1 and 2 should be marked StepSkipped.
	executor := &fakeExecutor{
		runStepCaptures: []string{""},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps: []steps.Step{
			breakStep("get-issue", "ISSUE_ID"),
			nonClaudeSteps("work")[0],
			nonClaudeSteps("review")[0],
		},
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	// Collect all StepSkipped calls and their indices.
	skippedIdxs := map[int]bool{}
	for _, call := range header.stepStateCalls {
		if call.state == ui.StepSkipped {
			skippedIdxs[call.idx] = true
		}
	}

	if !skippedIdxs[1] {
		t.Error("expected step index 1 (work) to be marked StepSkipped")
	}
	if !skippedIdxs[2] {
		t.Error("expected step index 2 (review) to be marked StepSkipped")
	}
	if skippedIdxs[0] {
		t.Error("trigger step (index 0) must not be marked StepSkipped — it completed as StepDone")
	}
}

// TestRun_BreakLoopIfEmpty_NoSkipWhenNotTriggered verifies that when
// BreakLoopIfEmpty is set but the captured value is non-empty (break does not
// fire), no step is marked StepSkipped.
func TestRun_BreakLoopIfEmpty_NoSkipWhenNotTriggered(t *testing.T) {
	// breakStep returns a non-empty value → no break, full iteration runs.
	executor := &fakeExecutor{
		runStepCaptures: []string{"issue-42", ""},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps: []steps.Step{
			breakStep("get-issue", "ISSUE_ID"),
			nonClaudeSteps("work")[0],
		},
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	for _, call := range header.stepStateCalls {
		if call.state == ui.StepSkipped {
			t.Errorf("no step should be StepSkipped when break does not fire; got StepSkipped at idx %d", call.idx)
		}
	}
}

// TestRun_BreakLoopIfEmpty_LastStepNoRemainingSkips verifies that when the
// breakLoopIfEmpty trigger fires on the last iteration step, no SetStepState
// calls are made (the remaining range is empty — j+1 == len(Steps)).
func TestRun_BreakLoopIfEmpty_LastStepNoRemainingSkips(t *testing.T) {
	// Single-step iteration; the step fires the break with empty capture.
	executor := &fakeExecutor{
		runStepCaptures: []string{""},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    1,
		Steps:         []steps.Step{breakStep("get-issue", "ISSUE_ID")},
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	for _, call := range header.stepStateCalls {
		if call.state == ui.StepSkipped {
			t.Errorf("no StepSkipped calls expected when break fires on last step; got idx %d", call.idx)
		}
	}
}

// TestRun_BreakLoopIfEmpty_MultiIterBreakOnSecond verifies that when
// breakLoopIfEmpty fires on the second iteration, only the remaining steps in
// that iteration are marked StepSkipped — not steps from iteration 1.
func TestRun_BreakLoopIfEmpty_MultiIterBreakOnSecond(t *testing.T) {
	// 2-step iteration, 2 iterations.
	// Iteration 1: get-issue → "issue-42" (non-empty → run work step → continue).
	// Iteration 2: get-issue → "" (empty → break; work step should be StepSkipped).
	executor := &fakeExecutor{
		runStepCaptures: []string{"issue-42", "", "", ""},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  2,
		Steps: []steps.Step{
			breakStep("get-issue", "ISSUE_ID"),
			nonClaudeSteps("work")[0],
		},
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	Run(executor, header, kh, cfg)

	skippedCount := 0
	for _, call := range header.stepStateCalls {
		if call.state == ui.StepSkipped {
			skippedCount++
			if call.idx != 1 {
				t.Errorf("StepSkipped expected only at index 1 (work); got idx %d", call.idx)
			}
		}
	}
	if skippedCount != 1 {
		t.Errorf("expected exactly 1 StepSkipped call (iteration 2, work); got %d", skippedCount)
	}
}

// TestRun_BreakLoopIfEmpty_FailedStepNoSkips verifies that when a step with
// BreakLoopIfEmpty fails (non-zero exit), no steps are marked StepSkipped —
// the StepSkipped marking only activates on successful completion (StepDone).
func TestRun_BreakLoopIfEmpty_FailedStepNoSkips(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		runStepErrors:   []error{errors.New("exit 1"), nil},
		runStepCaptures: []string{"", ""},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    1,
		Steps:         []steps.Step{breakStep("get-issue", "ISSUE_ID"), nonClaudeSteps("work")[0]},
		FinalizeSteps: nonClaudeSteps("final1"),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()

	time.Sleep(30 * time.Millisecond)
	actions <- ui.ActionContinue
	// After ActionContinue is consumed, remaining steps complete near-instantly.
	// Inject ActionQuit to unblock the completion sequence's final blocking receive.
	time.Sleep(10 * time.Millisecond)
	actions <- ui.ActionQuit

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete after ActionContinue")
	}

	for _, call := range header.stepStateCalls {
		if call.state == ui.StepSkipped {
			t.Errorf("no StepSkipped calls expected when break step fails; got idx %d", call.idx)
		}
	}
}

// newCompletionObserver returns a fakeExecutor onLog callback that signals
// on completionSeen as soon as the completion summary line is written to the
// log. The callback runs synchronously on the writer goroutine, so the send
// happens-before the receiving test goroutine observes the log slice.
func newCompletionObserver(completionSeen chan<- string) func(string) {
	return func(line string) {
		if strings.HasPrefix(line, "Ralph completed after ") {
			select {
			case completionSeen <- line:
			default:
			}
		}
	}
}

// TestRun_CompletionSummaryAndReturnsImmediately verifies that after all
// finalize steps complete, Run() writes the completion summary as the final
// line of the main body log and returns on its own without waiting for a
// keypress.
func TestRun_CompletionSummaryAndReturnsImmediately(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	completionSeen := make(chan string, 1)
	executor := &fakeExecutor{onLog: newCompletionObserver(completionSeen)}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    2,
		Steps:         nonClaudeSteps("step1"),
		FinalizeSteps: nonClaudeSteps("final1", "final2"),
	}

	done := make(chan RunResult, 1)
	go func() {
		done <- Run(executor, header, kh, cfg)
	}()

	// Wait for the completion summary line to be written to the log body.
	var got string
	select {
	case got = <-completionSeen:
	case <-time.After(5 * time.Second):
		t.Fatal("completion summary was not written to the log body")
	}

	want := ui.CompletionSummary(2, 2)
	if got != want {
		t.Errorf("completion summary: got %q, want %q", got, want)
	}

	// Run() must return on its own — no ActionQuit injection.
	select {
	case result := <-done:
		if result.IterationsRun != 2 {
			t.Errorf("expected IterationsRun=2, got %d", result.IterationsRun)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after writing the completion summary")
	}

	// Sanity check: the completion summary is the last non-empty line in the
	// main body log (trailing blank separators are allowed).
	last := lastNonBlankLine(executor.logLines)
	if last != want {
		t.Errorf("last non-blank log line: got %q, want %q", last, want)
	}
}

// TestRun_CompletionSummaryWithEmptyFinalize verifies that the completion
// summary reports finalizeCount=0 when FinalizeSteps is empty.
func TestRun_CompletionSummaryWithEmptyFinalize(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	completionSeen := make(chan string, 1)
	executor := &fakeExecutor{onLog: newCompletionObserver(completionSeen)}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       nonClaudeSteps("step1"),
		// FinalizeSteps intentionally empty.
	}

	done := make(chan RunResult, 1)
	go func() {
		done <- Run(executor, header, kh, cfg)
	}()

	var got string
	select {
	case got = <-completionSeen:
	case <-time.After(5 * time.Second):
		t.Fatal("completion summary was not written to the log body")
	}

	want := ui.CompletionSummary(1, 0)
	if got != want {
		t.Errorf("completion summary: got %q, want %q", got, want)
	}

	// Run() must return on its own after writing the completion summary.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after writing the completion summary")
	}
}

// TestRun_CompletionSummary_AfterBreakLoopIfEmpty verifies that when the loop
// exits early via breakLoopIfEmpty (on the first iteration of a 3-iteration
// config), the completion summary reports iterationsRun=1 and the correct
// finalizeCount.
func TestRun_CompletionSummary_AfterBreakLoopIfEmpty(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	completionSeen := make(chan string, 1)
	executor := &fakeExecutor{
		runStepCaptures: []string{""},
		onLog:           newCompletionObserver(completionSeen),
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    3,
		Steps:         []steps.Step{breakStep("get-issue", "ISSUE_ID"), nonClaudeSteps("work")[0]},
		FinalizeSteps: nonClaudeSteps("final1", "final2"),
	}

	done := make(chan RunResult, 1)
	go func() {
		done <- Run(executor, header, kh, cfg)
	}()

	var got string
	select {
	case got = <-completionSeen:
	case <-time.After(5 * time.Second):
		t.Fatal("completion summary was not written to the log body after breakLoopIfEmpty")
	}

	want := ui.CompletionSummary(1, 2)
	if got != want {
		t.Errorf("completion summary: got %q, want %q", got, want)
	}

	// Run() must return on its own after writing the completion summary.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after writing the completion summary")
	}
}

// lastNonBlankLine returns the last non-empty entry in lines, or "" if none.
func lastNonBlankLine(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] != "" {
			return lines[i]
		}
	}
	return ""
}

// indexOfLine returns the index of the first line matching pred, or -1 if
// no line matches. Used by log-ordering assertions.
func indexOfLine(lines []string, pred func(string) bool) int {
	for i, line := range lines {
		if pred(line) {
			return i
		}
	}
	return -1
}

// --- Phase banner and capture log tests ---

// TestRun_LogsPhaseBanners verifies that every phase the workflow enters
// writes a full-width phase banner (heading + underline) to the log body.
func TestRun_LogsPhaseBanners(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:     t.TempDir(),
		Iterations:      1,
		InitializeSteps: nonClaudeSteps("init1"),
		Steps:           nonClaudeSteps("step1"),
		FinalizeSteps:   nonClaudeSteps("final1"),
		LogWidth:        40,
	}

	Run(executor, header, kh, cfg)

	// Each phase banner is a heading line exactly equal to the phase name.
	for _, phase := range []string{"Initializing", "Iterations", "Finalizing"} {
		idx := indexOfLine(executor.logLines, func(l string) bool { return l == phase })
		if idx < 0 {
			t.Errorf("expected phase banner heading %q in log, got %v", phase, executor.logLines)
			continue
		}
		// The line immediately after the heading must be the underline.
		if idx+1 >= len(executor.logLines) {
			t.Errorf("phase %q: missing underline line", phase)
			continue
		}
		underline := executor.logLines[idx+1]
		if len([]rune(underline)) != cfg.LogWidth {
			t.Errorf("phase %q: underline width = %d runes, want %d (line %q)",
				phase, len([]rune(underline)), cfg.LogWidth, underline)
		}
		for _, r := range underline {
			if r != '═' {
				t.Errorf("phase %q: underline contains non-'═' rune %q", phase, r)
				break
			}
		}
	}
}

// TestRun_PhaseBannerOrderingAcrossPhases verifies that the banners appear in
// the expected Initializing → Iterations → Finalizing order.
func TestRun_PhaseBannerOrderingAcrossPhases(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:     t.TempDir(),
		Iterations:      1,
		InitializeSteps: nonClaudeSteps("init1"),
		Steps:           nonClaudeSteps("step1"),
		FinalizeSteps:   nonClaudeSteps("final1"),
		LogWidth:        20,
	}

	Run(executor, header, kh, cfg)

	initIdx := indexOfLine(executor.logLines, func(l string) bool { return l == "Initializing" })
	iterIdx := indexOfLine(executor.logLines, func(l string) bool { return l == "Iterations" })
	finalIdx := indexOfLine(executor.logLines, func(l string) bool { return l == "Finalizing" })

	if initIdx < 0 || iterIdx <= initIdx || finalIdx <= iterIdx {
		t.Errorf("expected Initializing → Iterations → Finalizing ordering, got indices init=%d iter=%d final=%d in %v",
			initIdx, iterIdx, finalIdx, executor.logLines)
	}
}

// TestRun_InitializingPhaseSkippedWhenNoInitSteps verifies that the
// Initializing banner is not written when the config has no initialize steps.
func TestRun_InitializingPhaseSkippedWhenNoInitSteps(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       nonClaudeSteps("step1"),
		LogWidth:    20,
	}

	Run(executor, header, kh, cfg)

	if indexOfLine(executor.logLines, func(l string) bool { return l == "Initializing" }) >= 0 {
		t.Errorf("Initializing banner must not appear when there are no init steps: %v", executor.logLines)
	}
	// The Iterations banner must still appear.
	if indexOfLine(executor.logLines, func(l string) bool { return l == "Iterations" }) < 0 {
		t.Errorf("Iterations banner missing: %v", executor.logLines)
	}
}

// TestRun_FinalizingPhaseSkippedWhenNoFinalizeSteps verifies that the
// Finalizing banner is not written when the config has no finalize steps.
func TestRun_FinalizingPhaseSkippedWhenNoFinalizeSteps(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       nonClaudeSteps("step1"),
		LogWidth:    20,
	}

	Run(executor, header, kh, cfg)

	if indexOfLine(executor.logLines, func(l string) bool { return l == "Finalizing" }) >= 0 {
		t.Errorf("Finalizing banner must not appear when there are no finalize steps: %v", executor.logLines)
	}
}

// TestRun_PhaseBannerUsesDefaultWidthWhenZero verifies that a zero LogWidth
// in RunConfig falls back to ui.DefaultTerminalWidth.
func TestRun_PhaseBannerUsesDefaultWidthWhenZero(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       nonClaudeSteps("step1"),
		// LogWidth intentionally left at 0.
	}

	Run(executor, header, kh, cfg)

	idx := indexOfLine(executor.logLines, func(l string) bool { return l == "Iterations" })
	if idx < 0 || idx+1 >= len(executor.logLines) {
		t.Fatalf("missing Iterations banner: %v", executor.logLines)
	}
	underline := executor.logLines[idx+1]
	if len([]rune(underline)) != ui.DefaultTerminalWidth {
		t.Errorf("default-width underline: got %d runes, want %d", len([]rune(underline)), ui.DefaultTerminalWidth)
	}
}

// TestRun_CaptureLogWrittenAfterCaptureStep verifies that a "Captured VAR =
// value" log line is written to the body after every captureAs step.
func TestRun_CaptureLogWrittenAfterCaptureStep(t *testing.T) {
	executor := &fakeExecutor{
		runStepCaptures: []string{"42"}, // iteration step's captured output
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       []steps.Step{captureStep("get-issue", "ISSUE_ID")},
		LogWidth:    40,
	}

	Run(executor, header, kh, cfg)

	want := `Captured ISSUE_ID = "42"`
	if indexOfLine(executor.logLines, func(l string) bool { return l == want }) < 0 {
		t.Errorf("expected capture log line %q, got %v", want, executor.logLines)
	}
}

// TestRun_CaptureLogWrittenForInitializePhase verifies that captureAs in the
// initialize phase also produces a capture log line.
func TestRun_CaptureLogWrittenForInitializePhase(t *testing.T) {
	executor := &fakeExecutor{
		runStepCaptures: []string{"octocat"}, // init step's captured output
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:     t.TempDir(),
		Iterations:      1,
		InitializeSteps: []steps.Step{captureStep("get-user", "GITHUB_USER")},
		Steps:           nonClaudeSteps("step1"),
		LogWidth:        40,
	}

	Run(executor, header, kh, cfg)

	want := `Captured GITHUB_USER = "octocat"`
	if indexOfLine(executor.logLines, func(l string) bool { return l == want }) < 0 {
		t.Errorf("expected init-phase capture log line %q, got %v", want, executor.logLines)
	}
}

// TestRun_CaptureLogNotWrittenForNonCaptureStep verifies that steps without
// captureAs do not emit a "Captured " log line.
func TestRun_CaptureLogNotWrittenForNonCaptureStep(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       nonClaudeSteps("step1"),
		LogWidth:    40,
	}

	Run(executor, header, kh, cfg)

	if idx := indexOfLine(executor.logLines, func(l string) bool { return strings.HasPrefix(l, "Captured ") }); idx >= 0 {
		t.Errorf("no capture log expected for non-capture step, got %q at index %d", executor.logLines[idx], idx)
	}
}

// TP-002 — Run() flows executor.ProjectDir() into VarTable as PROJECT_DIR.
// Verifies that the two dir arguments to vars.New() are not swapped: a step
// with command ["echo", "{{PROJECT_DIR}}"] must receive the executor's
// projectDir, not cfg.WorkflowDir.
func TestRun_ProjectDirFlowsIntoVarTable(t *testing.T) {
	executor := &fakeExecutor{projectDir: "/my/target/repo"}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: "/install/dir",
		Iterations:  1,
		Steps: []steps.Step{
			{Name: "echo-project", IsClaude: false, Command: []string{"echo", "{{PROJECT_DIR}}"}},
		},
	}

	Run(executor, header, kh, cfg)

	if len(executor.runStepCalls) == 0 {
		t.Fatal("expected at least one RunStep call")
	}
	cmd := executor.runStepCalls[0].command
	found := false
	for _, arg := range cmd {
		if arg == "/my/target/repo" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected command to contain %q (PROJECT_DIR), got %v", "/my/target/repo", cmd)
	}
	for _, arg := range cmd {
		if arg == "/install/dir" {
			t.Errorf("command must not contain WorkflowDir %q where PROJECT_DIR is expected, got %v", "/install/dir", cmd)
		}
	}
}

// TP-006 — Run() seeds WORKFLOW_DIR from cfg.WorkflowDir distinct from PROJECT_DIR.
// Verifies the mirror of TP-002: a step with command ["echo", "{{WORKFLOW_DIR}}"]
// receives cfg.WorkflowDir, not executor.ProjectDir().
func TestRun_WorkflowDirFlowsIntoVarTable(t *testing.T) {
	executor := &fakeExecutor{projectDir: "/target/repo"}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: "/install/dir",
		Iterations:  1,
		Steps: []steps.Step{
			{Name: "echo-workflow", IsClaude: false, Command: []string{"echo", "{{WORKFLOW_DIR}}"}},
		},
	}

	Run(executor, header, kh, cfg)

	if len(executor.runStepCalls) == 0 {
		t.Fatal("expected at least one RunStep call")
	}
	cmd := executor.runStepCalls[0].command
	found := false
	for _, arg := range cmd {
		if arg == "/install/dir" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected command to contain %q (WORKFLOW_DIR), got %v", "/install/dir", cmd)
	}
	for _, arg := range cmd {
		if arg == "/target/repo" {
			t.Errorf("command must not contain ProjectDir %q where WORKFLOW_DIR is expected, got %v", "/target/repo", cmd)
		}
	}
}
