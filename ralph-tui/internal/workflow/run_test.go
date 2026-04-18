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

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/claudestream"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/sandbox"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/statusline"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/vars"
)

// --- Test doubles ---

type fakeExecutor struct {
	runStepCalls            []runStepCall
	runStepErrors           []error          // per-call errors; nil entries mean success
	runStepCaptures         []string         // per-call LastCapture values (indexed by call order)
	runStepFullCaptureModes []ui.CaptureMode // per-call captureMode passed to RunStepFull
	lastCapture             string
	logLines                []string
	projectDir              string
	runSandboxedStepCalls   []runSandboxedStepCall
	runSandboxedStepErrors  []error // per-call errors; nil entries mean success
	// lastStatsReturn holds per-call return values for LastStats(). Index 0 is
	// returned on the first call, index 1 on the second, etc. If the call index
	// exceeds the slice length, a zero StepStats is returned.
	lastStatsReturn []claudestream.StepStats
	// lastStatsCalls counts how many times LastStats() has been called.
	lastStatsCalls int
	// onLog, when non-nil, is invoked for every line passed to WriteToLog or
	// WriteRunSummary. Tests use it to observe the log stream from another
	// goroutine without racing on logLines. The callback runs synchronously on
	// the writer goroutine, so happens-before the receiver of any channel it
	// sends to.
	onLog func(line string)
	// writeRunSummaryCalls counts how many times WriteRunSummary has been called.
	writeRunSummaryCalls int
}

type runStepCall struct {
	name    string
	command []string
}

type runSandboxedStepCall struct {
	name    string
	command []string
	opts    SandboxOptions
}

func (f *fakeExecutor) RunStep(name string, command []string) error {
	return f.RunStepFull(name, command, ui.CaptureLastLine)
}

func (f *fakeExecutor) RunStepFull(name string, command []string, captureMode ui.CaptureMode) error {
	idx := len(f.runStepCalls)
	f.runStepCalls = append(f.runStepCalls, runStepCall{name, command})
	f.runStepFullCaptureModes = append(f.runStepFullCaptureModes, captureMode)
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

func (f *fakeExecutor) RunSandboxedStep(name string, command []string, opts SandboxOptions) error {
	idx := len(f.runSandboxedStepCalls)
	f.runSandboxedStepCalls = append(f.runSandboxedStepCalls, runSandboxedStepCall{name, command, opts})
	if idx < len(f.runSandboxedStepErrors) && f.runSandboxedStepErrors[idx] != nil {
		return f.runSandboxedStepErrors[idx]
	}
	return nil
}

func (f *fakeExecutor) WriteToLog(line string) {
	f.logLines = append(f.logLines, line)
	if f.onLog != nil {
		f.onLog(line)
	}
}

func (f *fakeExecutor) WriteRunSummary(line string) {
	f.writeRunSummaryCalls++
	f.logLines = append(f.logLines, line)
	if f.onLog != nil {
		f.onLog(line)
	}
}

func (f *fakeExecutor) LastCapture() string {
	return f.lastCapture
}

func (f *fakeExecutor) LastStats() claudestream.StepStats {
	idx := f.lastStatsCalls
	f.lastStatsCalls++
	if idx < len(f.lastStatsReturn) {
		return f.lastStatsReturn[idx]
	}
	return claudestream.StepStats{}
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

// indexOf returns the index of the first occurrence of target in slice, or -1.
func indexOf(slice []string, target string) int {
	for i, v := range slice {
		if v == target {
			return i
		}
	}
	return -1
}

// TestBuildStep_ClaudeStepIteration verifies that a claude iteration step
// produces a docker run argv wrapping the claude CLI with the expected flags
// and prompt content. Assertions search for tokens by value rather than fixed
// index because the docker preamble sits before the claude argv.
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
	exec := &fakeExecutor{projectDir: dir}
	resolved, err := buildStep(dir, step, vt, vars.Iteration, nil, nil, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Name != "test-step" {
		t.Errorf("expected name %q, got %q", "test-step", resolved.Name)
	}
	if len(resolved.Command) == 0 || resolved.Command[0] != "docker" {
		t.Fatalf("expected command[0] == %q, got %v", "docker", resolved.Command)
	}
	imageIdx := indexOf(resolved.Command, sandbox.ImageTag)
	if imageIdx < 0 {
		t.Fatalf("image tag %q not found in command: %v", sandbox.ImageTag, resolved.Command)
	}
	if resolved.Command[imageIdx+1] != "claude" {
		t.Errorf("expected %q after image tag, got %q", "claude", resolved.Command[imageIdx+1])
	}
	if resolved.Command[imageIdx+2] != "--permission-mode" || resolved.Command[imageIdx+3] != "bypassPermissions" {
		t.Errorf("expected --permission-mode bypassPermissions after image, got %v %v",
			resolved.Command[imageIdx+2], resolved.Command[imageIdx+3])
	}
	modelIdx := indexOf(resolved.Command, "--model")
	if modelIdx < 0 || resolved.Command[modelIdx+1] != "claude-opus-4-6" {
		t.Errorf("expected --model claude-opus-4-6, not found or wrong value")
	}
	promptIdx := indexOf(resolved.Command, "-p")
	if promptIdx < 0 || resolved.Command[promptIdx+1] != "do something" {
		t.Errorf("expected -p %q, not found or wrong value", "do something")
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
	exec := &fakeExecutor{projectDir: dir}
	resolved, err := buildStep(dir, step, vt, vars.Iteration, nil, nil, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "implement issue 42 from sha abc123"
	promptIdx := indexOf(resolved.Command, "-p")
	if promptIdx < 0 || promptIdx+1 >= len(resolved.Command) {
		t.Fatalf("-p flag not found in command: %v", resolved.Command)
	}
	if got := resolved.Command[promptIdx+1]; got != want {
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
	exec := &fakeExecutor{projectDir: dir}
	_, err := buildStep(dir, step, vt, vars.Iteration, nil, nil, exec)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "bad-step") {
		t.Errorf("expected error to contain step name %q, got %q", "bad-step", err.Error())
	}
}

// TestBuildStep_ClaudeStepFinalize verifies that a finalize claude step
// produces a docker-wrapped argv with the correct model flag.
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
	exec := &fakeExecutor{projectDir: dir}
	resolved, err := buildStep(dir, step, vt, vars.Finalize, nil, nil, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Name != "finalize-claude" {
		t.Errorf("expected name %q, got %q", "finalize-claude", resolved.Name)
	}
	if len(resolved.Command) == 0 || resolved.Command[0] != "docker" {
		t.Fatalf("expected docker command, got %v", resolved.Command)
	}
	modelIdx := indexOf(resolved.Command, "--model")
	if modelIdx < 0 || resolved.Command[modelIdx+1] != "claude-sonnet-4-6" {
		t.Errorf("expected --model claude-sonnet-4-6, not found or wrong value")
	}
	promptIdx := indexOf(resolved.Command, "-p")
	if promptIdx < 0 || resolved.Command[promptIdx+1] != "finalize this" {
		t.Errorf("expected -p %q, not found or wrong value", "finalize this")
	}
}

// TestBuildStep_ClaudeStep_SandboxBindMount verifies that the resolved command
// includes a bind-mount of the project directory to the container workspace.
func TestBuildStep_ClaudeStep_SandboxBindMount(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "p.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	projectDir := "/tmp/fake-repo"
	step := steps.Step{Name: "s", IsClaude: true, Model: "m", PromptFile: "p.txt"}
	vt := vars.New(dir, dir, 0)
	vt.SetPhase(vars.Iteration)
	exec := &fakeExecutor{projectDir: projectDir}
	resolved, err := buildStep(dir, step, vt, vars.Iteration, nil, nil, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mountVal := fmt.Sprintf("type=bind,source=%s,target=%s", projectDir, sandbox.ContainerRepoPath)
	mountIdx := indexOf(resolved.Command, "--mount")
	found := false
	for i := mountIdx; i < len(resolved.Command); i++ {
		if resolved.Command[i] == mountVal {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected bind-mount %q in command %v", mountVal, resolved.Command)
	}
}

// TestBuildStep_ClaudeStep_SandboxOptionsCidfile verifies that the resolved
// step carries a non-empty CidfilePath under os.TempDir() matching the
// ralph-*.cid pattern.
func TestBuildStep_ClaudeStep_SandboxOptionsCidfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "p.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	step := steps.Step{Name: "s", IsClaude: true, Model: "m", PromptFile: "p.txt"}
	vt := vars.New(dir, dir, 0)
	vt.SetPhase(vars.Iteration)
	exec := &fakeExecutor{projectDir: dir}
	resolved, err := buildStep(dir, step, vt, vars.Iteration, nil, nil, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resolved.IsClaude {
		t.Error("expected resolved.IsClaude to be true")
	}
	if resolved.CidfilePath == "" {
		t.Fatal("expected non-empty CidfilePath")
	}
	if !strings.HasPrefix(resolved.CidfilePath, os.TempDir()) {
		t.Errorf("expected CidfilePath under os.TempDir() %q, got %q", os.TempDir(), resolved.CidfilePath)
	}
	base := filepath.Base(resolved.CidfilePath)
	if !strings.HasPrefix(base, "ralph-") || !strings.HasSuffix(base, ".cid") {
		t.Errorf("expected CidfilePath base to match ralph-*.cid, got %q", base)
	}
}

// TestBuildStep_ClaudeStep_EnvPassthrough verifies that env vars listed in the
// step file's env allowlist are included as -e flags when set on the host and
// omitted when unset.
func TestBuildStep_ClaudeStep_EnvPassthrough(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "p.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	step := steps.Step{Name: "s", IsClaude: true, Model: "m", PromptFile: "p.txt"}
	vt := vars.New(dir, dir, 0)
	vt.SetPhase(vars.Iteration)
	exec := &fakeExecutor{projectDir: dir}

	// With GITHUB_TOKEN set: expect -e GITHUB_TOKEN in command.
	t.Setenv("GITHUB_TOKEN", "tok123")
	resolved, err := buildStep(dir, step, vt, vars.Iteration, []string{"GITHUB_TOKEN"}, nil, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsSequence(resolved.Command, "-e", "GITHUB_TOKEN") {
		t.Errorf("expected -e GITHUB_TOKEN in command when env var is set; got %v", resolved.Command)
	}

	// With GITHUB_TOKEN unset: expect no -e GITHUB_TOKEN entry.
	// Unset directly (os.Unsetenv) so the key is absent, not empty.
	if err := os.Unsetenv("GITHUB_TOKEN"); err != nil {
		t.Fatalf("os.Unsetenv: %v", err)
	}
	resolved2, err := buildStep(dir, step, vt, vars.Iteration, []string{"GITHUB_TOKEN"}, nil, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if containsSequence(resolved2.Command, "-e", "GITHUB_TOKEN") {
		t.Errorf("expected no -e GITHUB_TOKEN when env var is unset; got %v", resolved2.Command)
	}
}

// TestBuildStep_ClaudeStep_DispatchesToSandboxedRunner verifies that Run
// dispatches IsClaude steps to RunSandboxedStep and non-claude steps to RunStep.
func TestBuildStep_ClaudeStep_DispatchesToSandboxedRunner(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "p.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	exec := &fakeExecutor{projectDir: dir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	claudeStep := steps.Step{Name: "claude-step", IsClaude: true, Model: "m", PromptFile: "p.txt"}
	shellStep := steps.Step{Name: "shell-step", IsClaude: false, Command: []string{"echo", "hi"}}

	cfg := RunConfig{
		WorkflowDir: dir,
		Iterations:  1,
		Steps:       []steps.Step{claudeStep, shellStep},
	}
	Run(exec, header, kh, cfg)

	if len(exec.runSandboxedStepCalls) != 1 {
		t.Errorf("expected 1 RunSandboxedStep call (for claude step), got %d", len(exec.runSandboxedStepCalls))
	} else if exec.runSandboxedStepCalls[0].name != "claude-step" {
		t.Errorf("expected RunSandboxedStep called for %q, got %q", "claude-step", exec.runSandboxedStepCalls[0].name)
	}

	if len(exec.runStepCalls) != 1 {
		t.Errorf("expected 1 RunStep call (for shell step), got %d", len(exec.runStepCalls))
	} else if exec.runStepCalls[0].name != "shell-step" {
		t.Errorf("expected RunStep called for %q, got %q", "shell-step", exec.runStepCalls[0].name)
	}
}

// containsSequence reports whether slice contains a and b as consecutive elements.
func containsSequence(slice []string, a, b string) bool {
	for i := 0; i+1 < len(slice); i++ {
		if slice[i] == a && slice[i+1] == b {
			return true
		}
	}
	return false
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

// TestLastCapture_ResetBetweenCalls verifies that a successful RunStep followed
// by a failing RunStep returns "" from LastCapture, not stale data from the
// first call. This guards the reset-on-failure contract of lastCapture.
func TestLastCapture_ResetBetweenCalls(t *testing.T) {
	logDir := t.TempDir()
	log, err := logger.NewLogger(logDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()

	runner := NewRunner(log, logDir)
	runner.SetSender(func(string) {})

	// First call succeeds and populates LastCapture.
	if err := runner.RunStep("step1", []string{"sh", "-c", "echo captured-value"}); err != nil {
		t.Fatalf("first RunStep: %v", err)
	}
	if got := runner.LastCapture(); got != "captured-value" {
		t.Fatalf("after first RunStep: got %q, want %q", got, "captured-value")
	}

	// Second call fails; LastCapture must be cleared, not stale.
	_ = runner.RunStep("step2", []string{"sh", "-c", "echo something; exit 1"})
	if got := runner.LastCapture(); got != "" {
		t.Errorf("after failed RunStep: LastCapture = %q, want empty (must not retain stale value)", got)
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

// TP-001: TestStepDispatcher_ClaudeStep_RoutesToRunSandboxedStep verifies that
// stepDispatcher.RunStep dispatches IsClaude=true steps to RunSandboxedStep
// with a SandboxOptions containing the step's CidfilePath, and does NOT call
// the underlying executor's RunStep.
func TestStepDispatcher_ClaudeStep_RoutesToRunSandboxedStep(t *testing.T) {
	exec := &fakeExecutor{}
	current := ui.ResolvedStep{
		Name:        "claude-step",
		IsClaude:    true,
		CidfilePath: "test.cid",
		Command:     []string{"docker", "run", "image"},
	}
	d := &stepDispatcher{exec: exec, current: current}

	if err := d.RunStep("claude-step", current.Command); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(exec.runSandboxedStepCalls) != 1 {
		t.Fatalf("expected 1 RunSandboxedStep call, got %d", len(exec.runSandboxedStepCalls))
	}
	if exec.runSandboxedStepCalls[0].opts.CidfilePath != "test.cid" {
		t.Errorf("expected CidfilePath %q, got %q", "test.cid", exec.runSandboxedStepCalls[0].opts.CidfilePath)
	}
	if len(exec.runStepCalls) != 0 {
		t.Errorf("expected 0 RunStep calls for IsClaude=true, got %d", len(exec.runStepCalls))
	}
}

// TP-002: TestStepDispatcher_NonClaudeStep_RoutesToRunStep verifies that
// stepDispatcher.RunStep delegates IsClaude=false steps to the underlying
// executor's RunStep and does NOT call RunSandboxedStep.
func TestStepDispatcher_NonClaudeStep_RoutesToRunStep(t *testing.T) {
	exec := &fakeExecutor{}
	current := ui.ResolvedStep{
		Name:     "shell-step",
		IsClaude: false,
		Command:  []string{"echo", "hi"},
	}
	d := &stepDispatcher{exec: exec, current: current}

	if err := d.RunStep("shell-step", current.Command); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(exec.runStepCalls) != 1 {
		t.Fatalf("expected 1 RunStep call, got %d", len(exec.runStepCalls))
	}
	if len(exec.runSandboxedStepCalls) != 0 {
		t.Errorf("expected 0 RunSandboxedStep calls for IsClaude=false, got %d", len(exec.runSandboxedStepCalls))
	}
}

// TP-003: TestRunSandboxedStep_AutoConstructsTerminatorFromCidfilePath verifies
// that runCommand installs a non-nil currentTerminator when opts.CidfilePath is
// non-empty and opts.Terminator is nil (the auto-construction path via
// sandbox.NewTerminator). After the step exits, currentTerminator must be nil.
func TestRunSandboxedStep_AutoConstructsTerminatorFromCidfilePath(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	cidfile, err := os.CreateTemp("", "ralph-tp003-*.cid")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	cidPath := cidfile.Name()
	_ = cidfile.Close()

	observedNonNil := make(chan bool, 1)
	go func() {
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

	opts := SandboxOptions{CidfilePath: cidPath} // Terminator intentionally nil
	if err := r.RunSandboxedStep("auto-term-step", []string{"sh", "-c", "sleep 0.05"}, opts); err != nil {
		t.Fatalf("RunSandboxedStep: %v", err)
	}

	r.processMu.Lock()
	afterTerm := r.currentTerminator
	r.processMu.Unlock()
	if afterTerm != nil {
		t.Error("expected currentTerminator to be nil after RunSandboxedStep returned")
	}

	if !<-observedNonNil {
		t.Error("expected to observe non-nil currentTerminator while RunSandboxedStep was running with auto-constructed terminator")
	}
}

// TP-004: TestBuildStep_ClaudeStep_EnvAllowlistMergesBuiltinAndUser verifies
// that buildStep produces -e flags for both a sandbox.BuiltinEnvAllowlist entry
// (ANTHROPIC_API_KEY) and a user-supplied env var (CUSTOM_VAR).
func TestBuildStep_ClaudeStep_EnvAllowlistMergesBuiltinAndUser(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "p.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ANTHROPIC_API_KEY", "builtin-key")
	t.Setenv("CUSTOM_VAR", "custom-val")

	step := steps.Step{Name: "s", IsClaude: true, Model: "m", PromptFile: "p.txt"}
	vt := vars.New(dir, dir, 0)
	vt.SetPhase(vars.Iteration)
	exec := &fakeExecutor{projectDir: dir}

	resolved, err := buildStep(dir, step, vt, vars.Iteration, []string{"CUSTOM_VAR"}, nil, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containsSequence(resolved.Command, "-e", "ANTHROPIC_API_KEY") {
		t.Errorf("expected -e ANTHROPIC_API_KEY (builtin) in command; got %v", resolved.Command)
	}
	if !containsSequence(resolved.Command, "-e", "CUSTOM_VAR") {
		t.Errorf("expected -e CUSTOM_VAR (user env) in command; got %v", resolved.Command)
	}
}

// TP-005: TestBuildStep_NonClaudeStep_ZeroValuesCidfileAndIsClaude verifies
// that a non-claude step resolves with IsClaude=false, CidfilePath="", and
// Command equal to the original step command.
func TestBuildStep_NonClaudeStep_ZeroValuesCidfileAndIsClaude(t *testing.T) {
	dir := t.TempDir()
	step := steps.Step{
		Name:     "echo-step",
		IsClaude: false,
		Command:  []string{"echo", "hi"},
	}
	vt := vars.New(dir, dir, 0)
	vt.SetPhase(vars.Iteration)
	exec := &fakeExecutor{projectDir: dir}

	resolved, err := buildStep(dir, step, vt, vars.Iteration, nil, nil, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.IsClaude {
		t.Error("expected IsClaude=false for non-claude step")
	}
	if resolved.CidfilePath != "" {
		t.Errorf("expected empty CidfilePath for non-claude step, got %q", resolved.CidfilePath)
	}
	if len(resolved.Command) != 2 || resolved.Command[0] != "echo" || resolved.Command[1] != "hi" {
		t.Errorf("expected command [echo hi], got %v", resolved.Command)
	}
}

// TP-006: TestStepDispatcher_ClaudeStep_ForwardsCidfilePathToSandboxOptions
// verifies that the CidfilePath from the current ResolvedStep flows through
// stepDispatcher.RunStep into the SandboxOptions passed to RunSandboxedStep.
func TestStepDispatcher_ClaudeStep_ForwardsCidfilePathToSandboxOptions(t *testing.T) {
	exec := &fakeExecutor{}
	wantCid := "/tmp/ralph-xyz.cid"
	current := ui.ResolvedStep{
		Name:        "claude-step",
		IsClaude:    true,
		CidfilePath: wantCid,
		Command:     []string{"docker", "run"},
	}
	d := &stepDispatcher{exec: exec, current: current}

	if err := d.RunStep("claude-step", current.Command); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(exec.runSandboxedStepCalls) != 1 {
		t.Fatalf("expected 1 RunSandboxedStep call, got %d", len(exec.runSandboxedStepCalls))
	}
	got := exec.runSandboxedStepCalls[0].opts.CidfilePath
	if got != wantCid {
		t.Errorf("expected CidfilePath %q forwarded to SandboxOptions, got %q", wantCid, got)
	}
}

// TP-007: TestStepDispatcher_DelegatesWasTerminatedAndWriteToLog verifies
// that stepDispatcher.WasTerminated and WriteToLog delegate to the wrapped
// executor unchanged.
func TestStepDispatcher_DelegatesWasTerminatedAndWriteToLog(t *testing.T) {
	exec := &fakeExecutor{}
	d := &stepDispatcher{exec: exec, current: ui.ResolvedStep{}}

	wasTerminated := d.WasTerminated()
	if wasTerminated != exec.WasTerminated() {
		t.Errorf("WasTerminated: got %v, want %v", wasTerminated, exec.WasTerminated())
	}

	d.WriteToLog("test line")
	if len(exec.logLines) != 1 || exec.logLines[0] != "test line" {
		t.Errorf("WriteToLog: expected logLines=[%q], got %v", "test line", exec.logLines)
	}
}

// TP-008: TestRun_InitializePhase_PassesEnvThroughBuildStep verifies that
// RunConfig.Env is threaded through Run's initialize phase into buildStep,
// producing -e flags for the custom env var in the sandboxed step command.
func TestRun_InitializePhase_PassesEnvThroughBuildStep(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "init-prompt.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CUSTOM_VAR", "v")

	exec := &fakeExecutor{projectDir: dir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	claudeInitStep := steps.Step{
		Name:       "init-claude",
		IsClaude:   true,
		Model:      "m",
		PromptFile: "init-prompt.txt",
	}

	cfg := RunConfig{
		WorkflowDir:     dir,
		Iterations:      1,
		InitializeSteps: []steps.Step{claudeInitStep},
		Steps:           nonClaudeSteps("shell-step"),
		Env:             []string{"CUSTOM_VAR"},
	}

	Run(exec, header, kh, cfg)

	if len(exec.runSandboxedStepCalls) != 1 {
		t.Fatalf("expected 1 RunSandboxedStep call (initialize claude), got %d", len(exec.runSandboxedStepCalls))
	}
	argv := exec.runSandboxedStepCalls[0].command
	if !containsSequence(argv, "-e", "CUSTOM_VAR") {
		t.Errorf("expected -e CUSTOM_VAR in initialize step command argv; got %v", argv)
	}
}

// TP-009: TestBuildStep_ClaudeStep_EnvAllowlistDefensiveCopy verifies that
// buildStep does not mutate sandbox.BuiltinEnvAllowlist when appending the
// user-supplied env slice. The original slice length and contents must be
// unchanged after the call.
func TestBuildStep_ClaudeStep_EnvAllowlistDefensiveCopy(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "p.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	originalLen := len(sandbox.BuiltinEnvAllowlist)
	originalCopy := make([]string, originalLen)
	copy(originalCopy, sandbox.BuiltinEnvAllowlist)

	step := steps.Step{Name: "s", IsClaude: true, Model: "m", PromptFile: "p.txt"}
	vt := vars.New(dir, dir, 0)
	vt.SetPhase(vars.Iteration)
	exec := &fakeExecutor{projectDir: dir}

	if _, err := buildStep(dir, step, vt, vars.Iteration, []string{"CUSTOM_VAR", "ANOTHER_VAR"}, nil, exec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sandbox.BuiltinEnvAllowlist) != originalLen {
		t.Errorf("BuiltinEnvAllowlist length changed: was %d, now %d (defensive copy mutated original)",
			originalLen, len(sandbox.BuiltinEnvAllowlist))
	}
	for i, v := range originalCopy {
		if sandbox.BuiltinEnvAllowlist[i] != v {
			t.Errorf("BuiltinEnvAllowlist[%d] changed: was %q, now %q", i, v, sandbox.BuiltinEnvAllowlist[i])
		}
	}
}

// TP-001: TestBuildStep_NonClaudeStep_CaptureModeMapping verifies that the
// string captureMode field on a non-claude Step is correctly mapped to the
// ui.CaptureMode enum in the returned ResolvedStep.
func TestBuildStep_NonClaudeStep_CaptureModeMapping(t *testing.T) {
	cases := []struct {
		input string
		want  ui.CaptureMode
	}{
		{"fullStdout", ui.CaptureFullStdout},
		{"lastLine", ui.CaptureLastLine},
		{"", ui.CaptureLastLine},
	}
	for _, tc := range cases {
		t.Run("captureMode="+tc.input, func(t *testing.T) {
			dir := t.TempDir()
			step := steps.Step{
				Name:        "s",
				IsClaude:    false,
				Command:     []string{"echo", "x"},
				CaptureMode: tc.input,
			}
			vt := vars.New(dir, dir, 0)
			vt.SetPhase(vars.Iteration)
			exec := &fakeExecutor{projectDir: dir}

			resolved, err := buildStep(dir, step, vt, vars.Iteration, nil, nil, exec)
			if err != nil {
				t.Fatalf("buildStep: %v", err)
			}
			if resolved.CaptureMode != tc.want {
				t.Errorf("CaptureMode = %v, want %v", resolved.CaptureMode, tc.want)
			}
		})
	}
}

// TP-002: TestStepDispatcher_NonClaudeStep_ThreadsCaptureMode verifies that
// stepDispatcher.RunStep passes d.current.CaptureMode through to the underlying
// executor's RunStepFull for non-claude steps.
func TestStepDispatcher_NonClaudeStep_ThreadsCaptureMode(t *testing.T) {
	cases := []struct {
		name string
		mode ui.CaptureMode
	}{
		{"fullStdout", ui.CaptureFullStdout},
		{"lastLine (zero)", ui.CaptureLastLine},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec := &fakeExecutor{}
			current := ui.ResolvedStep{
				Name:        "shell-step",
				IsClaude:    false,
				Command:     []string{"echo", "hi"},
				CaptureMode: tc.mode,
			}
			d := &stepDispatcher{exec: exec, current: current}

			if err := d.RunStep("shell-step", current.Command); err != nil {
				t.Fatalf("RunStep: %v", err)
			}

			if len(exec.runStepFullCaptureModes) != 1 {
				t.Fatalf("expected 1 RunStepFull call, got %d", len(exec.runStepFullCaptureModes))
			}
			if exec.runStepFullCaptureModes[0] != tc.mode {
				t.Errorf("captureMode = %v, want %v", exec.runStepFullCaptureModes[0], tc.mode)
			}
		})
	}
}

// TP-003: TestRun_FullStdout_CaptureAsBindsMultiLine verifies the headline
// feature: a step with captureMode="fullStdout" and captureAs="OUT" binds the
// full multi-line stdout payload (not just the last line) into the var table.
// Verified via LastCapture() after the run and the capture log written to the
// log stream.
func TestRun_FullStdout_CaptureAsBindsMultiLine(t *testing.T) {
	runner, log, drain := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps: []steps.Step{{
			Name:        "capture-step",
			IsClaude:    false,
			Command:     []string{"sh", "-c", "printf 'a\\nb\\nc'"},
			CaptureAs:   "OUT",
			CaptureMode: "fullStdout",
		}},
	}

	Run(runner, header, kh, cfg)

	got := runner.LastCapture()
	want := "a\nb\nc"
	if got != want {
		t.Errorf("LastCapture() = %q, want %q", got, want)
	}

	// writeCaptureLog should have emitted the bound value to the log stream.
	lines := drain()
	captureLogged := false
	for _, l := range lines {
		if strings.Contains(l, "OUT") && strings.Contains(l, "a") {
			captureLogged = true
			break
		}
	}
	if !captureLogged {
		t.Errorf("expected capture log entry for OUT in log stream; got lines: %v", lines)
	}
}

// TP-009: Compile-time assertion that *Runner satisfies StepExecutor.
// Mirrors the existing ui.HeartbeatReader assertion in workflow.go.
var _ StepExecutor = (*Runner)(nil)

// TP-010: TestRunStep_CurrentTerminatorStaysNilDuringExecution verifies that
// RunStep installs no terminator — currentTerminator remains nil throughout
// the duration of a RunStep call. This confirms the opts==nil guard in
// runCommand skips terminator installation for non-sandboxed steps.
func TestRunStep_CurrentTerminatorStaysNilDuringExecution(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()

	sawNonNil := make(chan bool, 1)
	go func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			r.processMu.Lock()
			v := r.currentTerminator
			r.processMu.Unlock()
			if v != nil {
				sawNonNil <- true
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		sawNonNil <- false
	}()

	if err := r.RunStep("plain-step", []string{"sh", "-c", "sleep 0.05"}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}

	if got := <-sawNonNil; got {
		t.Error("currentTerminator was set during RunStep — expected nil for non-sandboxed steps")
	}
}

// TP-011: TestStepDispatcher_ClaudeStep_PropagatesRunSandboxedStepError verifies
// that an error returned by RunSandboxedStep flows through stepDispatcher.RunStep
// to the caller unchanged. If this propagation were broken (e.g., error silently
// swallowed), Orchestrate's runStepWithErrorHandling would treat a failed claude
// step as successful, bypassing error-mode recovery entirely.
func TestStepDispatcher_ClaudeStep_PropagatesRunSandboxedStepError(t *testing.T) {
	wantErr := errors.New("sandbox: container exited with code 1")
	exec := &fakeExecutor{
		runSandboxedStepErrors: []error{wantErr},
	}
	current := ui.ResolvedStep{
		Name:     "claude-step",
		IsClaude: true,
		Command:  []string{"docker", "run", "image"},
	}
	d := &stepDispatcher{exec: exec, current: current}

	gotErr := d.RunStep("claude-step", current.Command)
	if gotErr != wantErr {
		t.Errorf("expected error %v, got %v", wantErr, gotErr)
	}
}

// TP-012: TestRun_FinalizePhase_ClaudeStep_DispatchesToRunSandboxedStep verifies
// that a claude step in the finalize phase is dispatched to RunSandboxedStep
// (not RunStep), confirming the stepDispatcher wiring is consistent across all
// three phases.
func TestRun_FinalizePhase_ClaudeStep_DispatchesToRunSandboxedStep(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "final-prompt.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	exec := &fakeExecutor{projectDir: dir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	claudeFinalStep := steps.Step{
		Name:       "claude-final",
		IsClaude:   true,
		Model:      "m",
		PromptFile: "final-prompt.txt",
	}

	cfg := RunConfig{
		WorkflowDir:   dir,
		Iterations:    1,
		Steps:         nonClaudeSteps("shell-step"),
		FinalizeSteps: []steps.Step{claudeFinalStep},
	}

	Run(exec, header, kh, cfg)

	if len(exec.runSandboxedStepCalls) != 1 {
		t.Fatalf("expected 1 RunSandboxedStep call (finalize claude), got %d", len(exec.runSandboxedStepCalls))
	}
	if exec.runSandboxedStepCalls[0].name != "claude-final" {
		t.Errorf("expected RunSandboxedStep called for %q, got %q", "claude-final", exec.runSandboxedStepCalls[0].name)
	}
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

// TestRun_FinalizeCaptureAsIgnored documents that the finalize phase does not
// bind captureAs values into the VarTable, unlike the initialize and iteration
// phases. A finalize step with CaptureAs set will not make its captured value
// available to subsequent finalize steps via {{VAR}} substitution. This
// asymmetry is intentional — finalize steps run after all iteration work is
// complete and do not need to pass state forward.
func TestRun_FinalizeCaptureAsIgnored(t *testing.T) {
	// Step 1 (finalize): CaptureAs="FINAL_VAR", fakeExecutor returns "captured-value".
	// Step 2 (finalize): command contains {{FINAL_VAR}}; if CaptureAs were honoured
	// the substituted value would be "captured-value", but since it is not, the
	// variable resolves to the empty string and the arg is "".
	executor := &fakeExecutor{
		runStepCaptures: []string{"captured-value", ""},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	captureStep := steps.Step{
		Name:      "capture-final",
		IsClaude:  false,
		Command:   []string{"echo", "captured-value"},
		CaptureAs: "FINAL_VAR",
	}
	useStep := steps.Step{
		Name:     "use-final",
		IsClaude: false,
		Command:  []string{"echo", "{{FINAL_VAR}}"},
	}

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("iter-step"),
		FinalizeSteps: []steps.Step{captureStep, useStep},
	}

	Run(executor, header, kh, cfg)

	// Find the "use-final" RunStep call and verify {{FINAL_VAR}} was NOT substituted
	// — it should resolve to empty string because finalize does not call vt.Bind.
	var useFinalCall *runStepCall
	for i := range executor.runStepCalls {
		if executor.runStepCalls[i].name == "use-final" {
			c := executor.runStepCalls[i]
			useFinalCall = &c
			break
		}
	}
	if useFinalCall == nil {
		t.Fatal("expected 'use-final' step to have run")
	}
	for _, arg := range useFinalCall.command {
		if arg == "captured-value" {
			t.Errorf("finalize CaptureAs must not bind into VarTable: expected {{FINAL_VAR}} to resolve to empty string, but got %q in command %v", arg, useFinalCall.command)
		}
	}
}

// SUGG-004: TestBuildStep_ClaudeStep_NilUserEnv_OnlyBuiltinsInCommand verifies
// that buildStep with a nil user-env slice does not include any user-supplied
// env var in the command. This is the common default configuration where
// ralph-steps.json has no top-level env field.
func TestBuildStep_ClaudeStep_NilUserEnv_OnlyBuiltinsInCommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "p.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set a custom env var that would appear if user-env were non-nil.
	t.Setenv("CUSTOM_USER_VAR", "should-not-appear")
	t.Setenv("ANTHROPIC_API_KEY", "builtin-key")

	step := steps.Step{Name: "s", IsClaude: true, Model: "m", PromptFile: "p.txt"}
	vt := vars.New(dir, dir, 0)
	vt.SetPhase(vars.Iteration)
	exec := &fakeExecutor{projectDir: dir}

	resolved, err := buildStep(dir, step, vt, vars.Iteration, nil, nil, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The builtin must appear.
	if !containsSequence(resolved.Command, "-e", "ANTHROPIC_API_KEY") {
		t.Errorf("expected -e ANTHROPIC_API_KEY (builtin) in command; got %v", resolved.Command)
	}

	// The user var must NOT appear — nil env means no user additions.
	if containsSequence(resolved.Command, "-e", "CUSTOM_USER_VAR") {
		t.Errorf("expected CUSTOM_USER_VAR to be absent with nil user-env; got %v", resolved.Command)
	}
}

// --- TP-W4: runStats.add unit tests ---

// TP-W4a: TestRunStats_ZeroValue verifies that a freshly allocated runStats has
// all zero fields, confirming the zero-value contract relied upon by Run.
func TestRunStats_ZeroValue(t *testing.T) {
	var rs runStats
	if rs.invocations != 0 {
		t.Errorf("expected invocations=0, got %d", rs.invocations)
	}
	if rs.retries != 0 {
		t.Errorf("expected retries=0, got %d", rs.retries)
	}
	if rs.total.InputTokens != 0 || rs.total.OutputTokens != 0 ||
		rs.total.CacheCreationTokens != 0 || rs.total.CacheReadTokens != 0 ||
		rs.total.NumTurns != 0 || rs.total.TotalCostUSD != 0 || rs.total.DurationMS != 0 {
		t.Errorf("expected all total fields to be zero, got %+v", rs.total)
	}
}

// TP-W4b: TestRunStats_Add_AccumulatesAllFields verifies that calling add twice
// sums every StepStats field independently.
func TestRunStats_Add_AccumulatesAllFields(t *testing.T) {
	var rs runStats

	s1 := claudestream.StepStats{
		InputTokens:         10,
		OutputTokens:        20,
		CacheCreationTokens: 3,
		CacheReadTokens:     4,
		NumTurns:            2,
		TotalCostUSD:        0.01,
		DurationMS:          500,
	}
	s2 := claudestream.StepStats{
		InputTokens:         5,
		OutputTokens:        15,
		CacheCreationTokens: 1,
		CacheReadTokens:     2,
		NumTurns:            1,
		TotalCostUSD:        0.005,
		DurationMS:          300,
	}

	rs.add(s1, false)
	rs.add(s2, false)

	if rs.total.InputTokens != 15 {
		t.Errorf("InputTokens: want 15, got %d", rs.total.InputTokens)
	}
	if rs.total.OutputTokens != 35 {
		t.Errorf("OutputTokens: want 35, got %d", rs.total.OutputTokens)
	}
	if rs.total.CacheCreationTokens != 4 {
		t.Errorf("CacheCreationTokens: want 4, got %d", rs.total.CacheCreationTokens)
	}
	if rs.total.CacheReadTokens != 6 {
		t.Errorf("CacheReadTokens: want 6, got %d", rs.total.CacheReadTokens)
	}
	if rs.total.NumTurns != 3 {
		t.Errorf("NumTurns: want 3, got %d", rs.total.NumTurns)
	}
	if rs.total.TotalCostUSD != 0.015 {
		t.Errorf("TotalCostUSD: want 0.015, got %f", rs.total.TotalCostUSD)
	}
	if rs.total.DurationMS != 800 {
		t.Errorf("DurationMS: want 800, got %d", rs.total.DurationMS)
	}
	if rs.invocations != 2 {
		t.Errorf("invocations: want 2, got %d", rs.invocations)
	}
}

// TP-W4c: TestRunStats_Add_RetryIncrement verifies that add increments retries
// only when isRetry is true, and does not increment it otherwise.
func TestRunStats_Add_RetryIncrement(t *testing.T) {
	var rs runStats
	s := claudestream.StepStats{InputTokens: 1}

	rs.add(s, false) // not a retry
	if rs.retries != 0 {
		t.Errorf("after non-retry add: want retries=0, got %d", rs.retries)
	}

	rs.add(s, true) // retry
	if rs.retries != 1 {
		t.Errorf("after retry add: want retries=1, got %d", rs.retries)
	}

	rs.add(s, false) // not a retry
	if rs.retries != 1 {
		t.Errorf("after second non-retry add: want retries=1 (unchanged), got %d", rs.retries)
	}

	if rs.invocations != 3 {
		t.Errorf("want invocations=3, got %d", rs.invocations)
	}
}

// --- TP-W1: stepDispatcher stats folding tests ---

// TP-W1a: TestStepDispatcher_ClaudeStep_FoldsStatsIntoRunStats verifies that
// when a claude step succeeds, LastStats() is called and the returned StepStats
// are folded into the shared runStats via add.
func TestStepDispatcher_ClaudeStep_FoldsStatsIntoRunStats(t *testing.T) {
	wantStats := claudestream.StepStats{
		InputTokens:  50,
		OutputTokens: 30,
		NumTurns:     2,
		TotalCostUSD: 0.02,
		DurationMS:   1000,
	}
	exec := &fakeExecutor{
		lastStatsReturn: []claudestream.StepStats{wantStats},
	}
	rs := &runStats{}
	current := ui.ResolvedStep{
		Name:     "claude-step",
		IsClaude: true,
		Command:  []string{"docker", "run"},
	}
	d := &stepDispatcher{exec: exec, current: current, stats: rs}

	if err := d.RunStep("claude-step", current.Command); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exec.lastStatsCalls != 1 {
		t.Errorf("expected LastStats() called once, got %d", exec.lastStatsCalls)
	}
	if rs.invocations != 1 {
		t.Errorf("expected invocations=1, got %d", rs.invocations)
	}
	if rs.total.InputTokens != wantStats.InputTokens {
		t.Errorf("InputTokens: want %d, got %d", wantStats.InputTokens, rs.total.InputTokens)
	}
	if rs.total.OutputTokens != wantStats.OutputTokens {
		t.Errorf("OutputTokens: want %d, got %d", wantStats.OutputTokens, rs.total.OutputTokens)
	}
	if rs.total.NumTurns != wantStats.NumTurns {
		t.Errorf("NumTurns: want %d, got %d", wantStats.NumTurns, rs.total.NumTurns)
	}
	if rs.total.TotalCostUSD != wantStats.TotalCostUSD {
		t.Errorf("TotalCostUSD: want %f, got %f", wantStats.TotalCostUSD, rs.total.TotalCostUSD)
	}
}

// TP-W1b: TestStepDispatcher_ClaudeStep_FoldsStatsOnError verifies that stats
// are folded into runStats even when RunSandboxedStep returns an error (D21:
// "the spend was real"). The error is still propagated to the caller.
func TestStepDispatcher_ClaudeStep_FoldsStatsOnError(t *testing.T) {
	wantStats := claudestream.StepStats{InputTokens: 99, TotalCostUSD: 0.05}
	wantErr := errors.New("sandbox: container exited non-zero")
	exec := &fakeExecutor{
		runSandboxedStepErrors: []error{wantErr},
		lastStatsReturn:        []claudestream.StepStats{wantStats},
	}
	rs := &runStats{}
	current := ui.ResolvedStep{
		Name:     "failing-claude-step",
		IsClaude: true,
		Command:  []string{"docker", "run"},
	}
	d := &stepDispatcher{exec: exec, current: current, stats: rs}

	gotErr := d.RunStep("failing-claude-step", current.Command)
	if gotErr != wantErr {
		t.Fatalf("expected error %v, got %v", wantErr, gotErr)
	}

	// Stats must be folded even on error.
	if exec.lastStatsCalls != 1 {
		t.Errorf("expected LastStats() called once even on error, got %d", exec.lastStatsCalls)
	}
	if rs.invocations != 1 {
		t.Errorf("expected invocations=1 even on error, got %d", rs.invocations)
	}
	if rs.total.InputTokens != wantStats.InputTokens {
		t.Errorf("InputTokens must be folded on error: want %d, got %d", wantStats.InputTokens, rs.total.InputTokens)
	}
}

// --- TP-W2: stepDispatcher prevFailed retry tracking tests ---

// TP-W2a: TestStepDispatcher_ClaudeStep_RetryCountsOnSecondCallAfterError
// verifies that when a claude step errors on the first call and succeeds on the
// second call (as happens during a retry loop), the second call's stats are
// folded with isRetry=true, incrementing runStats.retries.
func TestStepDispatcher_ClaudeStep_RetryCountsOnSecondCallAfterError(t *testing.T) {
	wantErr := errors.New("exit 1")
	exec := &fakeExecutor{
		runSandboxedStepErrors: []error{wantErr, nil}, // first call fails, second succeeds
		lastStatsReturn: []claudestream.StepStats{
			{InputTokens: 10},
			{InputTokens: 20},
		},
	}
	rs := &runStats{}
	current := ui.ResolvedStep{
		Name:     "claude-step",
		IsClaude: true,
		Command:  []string{"docker", "run"},
	}
	d := &stepDispatcher{exec: exec, current: current, stats: rs}

	// First call errors.
	gotErr := d.RunStep("claude-step", current.Command)
	if gotErr != wantErr {
		t.Fatalf("first call: expected error %v, got %v", wantErr, gotErr)
	}
	if d.prevFailed != true {
		t.Error("expected prevFailed=true after first error")
	}

	// Second call succeeds. Because prevFailed=true, this is counted as a retry.
	if err := d.RunStep("claude-step", current.Command); err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}

	if rs.invocations != 2 {
		t.Errorf("expected invocations=2, got %d", rs.invocations)
	}
	if rs.retries != 1 {
		t.Errorf("expected retries=1, got %d", rs.retries)
	}
	// prevFailed is cleared after a successful second call.
	if d.prevFailed != false {
		t.Error("expected prevFailed=false after successful second call")
	}
}

// TP-W2b: TestStepDispatcher_NonClaudeStep_ResetsRetryTracking verifies that
// when a non-claude step runs (even after prevFailed is set), prevFailed is
// cleared to false, so a subsequent claude step is not miscounted as a retry.
func TestStepDispatcher_NonClaudeStep_ResetsRetryTracking(t *testing.T) {
	exec := &fakeExecutor{
		runSandboxedStepErrors: []error{errors.New("fail"), nil},
		lastStatsReturn: []claudestream.StepStats{
			{InputTokens: 10}, // first claude call (failed)
			{InputTokens: 20}, // third call (claude again, should not count as retry)
		},
	}
	rs := &runStats{}

	// Step 1: claude step errors → prevFailed = true.
	d := &stepDispatcher{exec: exec, current: ui.ResolvedStep{
		Name: "claude-step", IsClaude: true, Command: []string{"docker", "run"},
	}, stats: rs}
	_ = d.RunStep("claude-step", d.current.Command)
	if !d.prevFailed {
		t.Fatal("expected prevFailed=true after first error")
	}

	// Step 2: non-claude step → prevFailed must reset to false.
	d.current = ui.ResolvedStep{Name: "shell-step", IsClaude: false, Command: []string{"echo"}}
	if err := d.RunStep("shell-step", d.current.Command); err != nil {
		t.Fatalf("non-claude step: unexpected error: %v", err)
	}
	if d.prevFailed {
		t.Error("expected prevFailed=false after non-claude step")
	}

	// Step 3: claude step succeeds. Because prevFailed=false, this is NOT a retry.
	d.current = ui.ResolvedStep{Name: "claude-step", IsClaude: true, Command: []string{"docker", "run"}}
	if err := d.RunStep("claude-step", d.current.Command); err != nil {
		t.Fatalf("third call: unexpected error: %v", err)
	}

	if rs.retries != 0 {
		t.Errorf("expected retries=0 (non-claude step reset prevFailed), got %d", rs.retries)
	}
}

// --- TP-W3: stepDispatcher forwards ArtifactPath and CaptureMode ---

// TP-W3: TestStepDispatcher_ClaudeStep_ForwardsArtifactPathAndCaptureMode
// verifies that ArtifactPath and CaptureMode from current are forwarded to the
// SandboxOptions passed to RunSandboxedStep.
func TestStepDispatcher_ClaudeStep_ForwardsArtifactPathAndCaptureMode(t *testing.T) {
	exec := &fakeExecutor{}
	wantPath := "/logs/test-stamp/iter01-01-my-step.jsonl"
	current := ui.ResolvedStep{
		Name:         "my-step",
		IsClaude:     true,
		Command:      []string{"docker", "run"},
		ArtifactPath: wantPath,
		CaptureMode:  ui.CaptureResult,
	}
	d := &stepDispatcher{exec: exec, current: current}

	if err := d.RunStep("my-step", current.Command); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(exec.runSandboxedStepCalls) != 1 {
		t.Fatalf("expected 1 RunSandboxedStep call, got %d", len(exec.runSandboxedStepCalls))
	}
	opts := exec.runSandboxedStepCalls[0].opts
	if opts.ArtifactPath != wantPath {
		t.Errorf("ArtifactPath: want %q, got %q", wantPath, opts.ArtifactPath)
	}
	if opts.CaptureMode != ui.CaptureResult {
		t.Errorf("CaptureMode: want CaptureResult, got %v", opts.CaptureMode)
	}
}

// --- TP-W5: artifactPath helper in Run ---

// claudeIterStep creates a temporary workflow directory with prompts/name.txt and
// returns a claude step suitable for use in the iteration phase of Run.
func claudeIterStep(t *testing.T, dir, name string) steps.Step {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	promptFile := name + ".txt"
	if err := os.WriteFile(filepath.Join(dir, "prompts", promptFile), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	return steps.Step{Name: name, IsClaude: true, Model: "m", PromptFile: promptFile}
}

// TP-W5a: TestRun_ClaudeStep_ArtifactPathInSandboxOptions verifies that when
// RunStamp is non-empty, a claude step in the iteration phase receives an
// ArtifactPath of the form
// <projectDir>/logs/<runStamp>/iter<iter>-<stepIdx>-<slug>.jsonl.
func TestRun_ClaudeStep_ArtifactPathInSandboxOptions(t *testing.T) {
	dir := t.TempDir()
	stepName := "feature-work"
	claudeStep := claudeIterStep(t, dir, stepName)

	exec := &fakeExecutor{projectDir: dir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	runStamp := "ralph-2026-04-14-173022.123"
	cfg := RunConfig{
		WorkflowDir: dir,
		Iterations:  1,
		Steps:       []steps.Step{claudeStep},
		RunStamp:    runStamp,
	}

	Run(exec, header, kh, cfg)

	if len(exec.runSandboxedStepCalls) != 1 {
		t.Fatalf("expected 1 RunSandboxedStep call, got %d", len(exec.runSandboxedStepCalls))
	}

	// Iteration 1, step index j=0 → j+1=1; iter%02d=01, step%02d=01.
	wantPath := filepath.Join(dir, "logs", runStamp, "iter01-01-feature-work.jsonl")
	gotPath := exec.runSandboxedStepCalls[0].opts.ArtifactPath
	if gotPath != wantPath {
		t.Errorf("ArtifactPath: want %q, got %q", wantPath, gotPath)
	}
}

// TP-W5b: TestRun_ClaudeStep_EmptyRunStamp_NoArtifactPath verifies that when
// RunStamp is empty, ArtifactPath in SandboxOptions is "" (persistence disabled).
func TestRun_ClaudeStep_EmptyRunStamp_NoArtifactPath(t *testing.T) {
	dir := t.TempDir()
	claudeStep := claudeIterStep(t, dir, "my-step")

	exec := &fakeExecutor{projectDir: dir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: dir,
		Iterations:  1,
		Steps:       []steps.Step{claudeStep},
		RunStamp:    "", // empty → no artifact path
	}

	Run(exec, header, kh, cfg)

	if len(exec.runSandboxedStepCalls) != 1 {
		t.Fatalf("expected 1 RunSandboxedStep call, got %d", len(exec.runSandboxedStepCalls))
	}
	if gotPath := exec.runSandboxedStepCalls[0].opts.ArtifactPath; gotPath != "" {
		t.Errorf("expected empty ArtifactPath when RunStamp is empty, got %q", gotPath)
	}
}

// TP-W5c: TestRun_InitializePhase_ArtifactPathPrefix verifies that claude steps
// in the initialize phase receive an ArtifactPath with the "initialize-" prefix.
func TestRun_InitializePhase_ArtifactPathPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "init-step.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	claudeInitStep := steps.Step{Name: "init-step", IsClaude: true, Model: "m", PromptFile: "init-step.txt"}

	exec := &fakeExecutor{projectDir: dir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	runStamp := "ralph-2026-04-14-173022.123"
	cfg := RunConfig{
		WorkflowDir:     dir,
		Iterations:      1,
		InitializeSteps: []steps.Step{claudeInitStep},
		Steps:           nonClaudeSteps("shell-step"),
		RunStamp:        runStamp,
	}

	Run(exec, header, kh, cfg)

	if len(exec.runSandboxedStepCalls) != 1 {
		t.Fatalf("expected 1 RunSandboxedStep call (initialize), got %d", len(exec.runSandboxedStepCalls))
	}
	gotPath := exec.runSandboxedStepCalls[0].opts.ArtifactPath
	wantPath := filepath.Join(dir, "logs", runStamp, "initialize-01-init-step.jsonl")
	if gotPath != wantPath {
		t.Errorf("initialize phase ArtifactPath: want %q, got %q", wantPath, gotPath)
	}
}

// TP-W5d: TestRun_FinalizePhase_ArtifactPathPrefix verifies that claude steps
// in the finalize phase receive an ArtifactPath with the "finalize-" prefix.
func TestRun_FinalizePhase_ArtifactPathPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "final-step.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	claudeFinalStep := steps.Step{Name: "final-step", IsClaude: true, Model: "m", PromptFile: "final-step.txt"}

	exec := &fakeExecutor{projectDir: dir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	runStamp := "ralph-2026-04-14-173022.123"
	cfg := RunConfig{
		WorkflowDir:   dir,
		Iterations:    1,
		Steps:         nonClaudeSteps("shell-step"),
		FinalizeSteps: []steps.Step{claudeFinalStep},
		RunStamp:      runStamp,
	}

	Run(exec, header, kh, cfg)

	if len(exec.runSandboxedStepCalls) != 1 {
		t.Fatalf("expected 1 RunSandboxedStep call (finalize), got %d", len(exec.runSandboxedStepCalls))
	}
	gotPath := exec.runSandboxedStepCalls[0].opts.ArtifactPath
	wantPath := filepath.Join(dir, "logs", runStamp, "finalize-01-final-step.jsonl")
	if gotPath != wantPath {
		t.Errorf("finalize phase ArtifactPath: want %q, got %q", wantPath, gotPath)
	}
}

// --- TP-W6: CaptureMode = CaptureResult on claude steps ---

// TP-W6a: TestRun_ClaudeStep_CaptureModeIsResult verifies that a claude step
// dispatched by Run receives CaptureMode=CaptureResult in its SandboxOptions.
func TestRun_ClaudeStep_CaptureModeIsResult(t *testing.T) {
	dir := t.TempDir()
	claudeStep := claudeIterStep(t, dir, "my-claude-step")

	exec := &fakeExecutor{projectDir: dir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: dir,
		Iterations:  1,
		Steps:       []steps.Step{claudeStep},
	}

	Run(exec, header, kh, cfg)

	if len(exec.runSandboxedStepCalls) != 1 {
		t.Fatalf("expected 1 RunSandboxedStep call, got %d", len(exec.runSandboxedStepCalls))
	}
	gotMode := exec.runSandboxedStepCalls[0].opts.CaptureMode
	if gotMode != ui.CaptureResult {
		t.Errorf("CaptureMode: want CaptureResult, got %v", gotMode)
	}
}

// TP-W6b: TestRun_NonClaudeStep_CaptureModeDefaultsToLastLine verifies that
// non-claude steps do not set CaptureMode in SandboxOptions (they call RunStep,
// not RunSandboxedStep). The fakeExecutor receives no RunSandboxedStep calls.
func TestRun_NonClaudeStep_CaptureModeDefaultsToLastLine(t *testing.T) {
	exec := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       nonClaudeSteps("shell-step"),
	}

	Run(exec, header, kh, cfg)

	if len(exec.runSandboxedStepCalls) != 0 {
		t.Errorf("expected 0 RunSandboxedStep calls for non-claude step, got %d", len(exec.runSandboxedStepCalls))
	}
	if len(exec.runStepCalls) != 1 {
		t.Fatalf("expected 1 RunStep call for non-claude step, got %d", len(exec.runStepCalls))
	}
}

// --- D13 2c: run-level cumulative summary tests ---

// TestRun_RunSummary_EmittedForClaudeSteps verifies that after a run with claude
// steps, the run-level cumulative summary line is written to the log body (D13 2c).
// The summary must appear before the CompletionSummary line and contain the key
// token/cost fragments from the accumulated StepStats.
func TestRun_RunSummary_EmittedForClaudeSteps(t *testing.T) {
	dir := t.TempDir()
	claudeStep := claudeIterStep(t, dir, "feature-work")

	wantStats := claudestream.StepStats{
		NumTurns:     3,
		InputTokens:  120,
		OutputTokens: 60,
		TotalCostUSD: 0.0123456,
		DurationMS:   5000,
	}
	exec := &fakeExecutor{
		projectDir:      dir,
		lastStatsReturn: []claudestream.StepStats{wantStats},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: dir,
		Iterations:  1,
		Steps:       []steps.Step{claudeStep},
	}

	Run(exec, header, kh, cfg)

	// Find the run summary line in logLines (it must start with "total claude spend").
	runSummaryIdx := -1
	completionIdx := -1
	for i, line := range exec.logLines {
		if strings.HasPrefix(line, "total claude spend") {
			runSummaryIdx = i
		}
		if strings.HasPrefix(line, "Ralph completed after") {
			completionIdx = i
		}
	}

	if runSummaryIdx < 0 {
		t.Fatalf("run summary line not found in log body; lines: %v", exec.logLines)
	}
	if completionIdx < 0 {
		t.Fatalf("completion summary line not found in log body; lines: %v", exec.logLines)
	}
	if runSummaryIdx >= completionIdx {
		t.Errorf("run summary (idx %d) must appear before completion summary (idx %d)", runSummaryIdx, completionIdx)
	}
	if exec.writeRunSummaryCalls != 1 {
		t.Errorf("WriteRunSummary called %d times, want 1", exec.writeRunSummaryCalls)
	}

	line := exec.logLines[runSummaryIdx]
	for _, fragment := range []string{"1 step invocation", "120/60 tokens", "$0.0123456"} {
		if !strings.Contains(line, fragment) {
			t.Errorf("run summary %q missing fragment %q", line, fragment)
		}
	}
}

// TestRun_RunSummary_NotEmittedForNonClaudeSteps verifies that when no claude
// steps run, no run-level summary line is written to the log body (D13 2c:
// FinalizeRun returns nil for zero invocations).
func TestRun_RunSummary_NotEmittedForNonClaudeSteps(t *testing.T) {
	exec := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       nonClaudeSteps("shell-step"),
	}

	Run(exec, header, kh, cfg)

	for _, line := range exec.logLines {
		if strings.HasPrefix(line, "total claude spend") {
			t.Errorf("unexpected run summary line in non-claude run: %q", line)
		}
	}
}

// TestRun_RunSummary_MultipleClaudeStepsAccumulate verifies that stats from
// multiple claude step invocations are accumulated into the run summary (D13 2c,
// D21: each invocation contributes to the cumulative total).
func TestRun_RunSummary_MultipleClaudeStepsAccumulate(t *testing.T) {
	dir := t.TempDir()
	step1 := claudeIterStep(t, dir, "step-one")
	step2 := claudeIterStep(t, dir, "step-two")

	stats1 := claudestream.StepStats{InputTokens: 100, TotalCostUSD: 0.01}
	stats2 := claudestream.StepStats{InputTokens: 200, TotalCostUSD: 0.02}
	exec := &fakeExecutor{
		projectDir:      dir,
		lastStatsReturn: []claudestream.StepStats{stats1, stats2},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: dir,
		Iterations:  1,
		Steps:       []steps.Step{step1, step2},
	}

	Run(exec, header, kh, cfg)

	var runSummary string
	for _, line := range exec.logLines {
		if strings.HasPrefix(line, "total claude spend") {
			runSummary = line
			break
		}
	}
	if runSummary == "" {
		t.Fatalf("run summary not found in log body; lines: %v", exec.logLines)
	}
	// Two invocations.
	if !strings.Contains(runSummary, "2 step invocations") {
		t.Errorf("run summary %q should reflect 2 invocations", runSummary)
	}
	// Combined cost.
	if !strings.Contains(runSummary, "$0.0300000") {
		t.Errorf("run summary %q should reflect combined cost $0.03", runSummary)
	}
}

// --- StatusRunner / buildState tests ---

// fakeStatusRunner records PushState and Trigger calls for assertions.
type fakeStatusRunner struct {
	pushStates []statusline.State
	triggers   int
}

func (r *fakeStatusRunner) PushState(s statusline.State) {
	r.pushStates = append(r.pushStates, s)
}

func (r *fakeStatusRunner) Trigger() {
	r.triggers++
}

// TestBuildState_PopulatesAllFields verifies that buildState reads all
// built-in variables from the VarTable and returns a correctly populated State.
func TestBuildState_PopulatesAllFields(t *testing.T) {
	vt := vars.New("/workflow", "/project", 5)
	vt.SetIteration(2)
	vt.SetStep(3, 7, "my-step")
	vt.SetPhase(vars.Iteration)
	vt.Bind(vars.Initialize, "FOO", "bar")

	state := buildState(vt, vars.Iteration, "sess-123", "0.6.0")

	if state.SessionID != "sess-123" {
		t.Errorf("SessionID: want %q, got %q", "sess-123", state.SessionID)
	}
	if state.Version != "0.6.0" {
		t.Errorf("Version: want %q, got %q", "0.6.0", state.Version)
	}
	if state.Phase != "iteration" {
		t.Errorf("Phase: want %q, got %q", "iteration", state.Phase)
	}
	if state.Iteration != 2 {
		t.Errorf("Iteration: want 2, got %d", state.Iteration)
	}
	if state.MaxIterations != 5 {
		t.Errorf("MaxIterations: want 5, got %d", state.MaxIterations)
	}
	if state.StepNum != 3 {
		t.Errorf("StepNum: want 3, got %d", state.StepNum)
	}
	if state.StepCount != 7 {
		t.Errorf("StepCount: want 7, got %d", state.StepCount)
	}
	if state.StepName != "my-step" {
		t.Errorf("StepName: want %q, got %q", "my-step", state.StepName)
	}
	if state.WorkflowDir != "/workflow" {
		t.Errorf("WorkflowDir: want %q, got %q", "/workflow", state.WorkflowDir)
	}
	if state.ProjectDir != "/project" {
		t.Errorf("ProjectDir: want %q, got %q", "/project", state.ProjectDir)
	}
	if state.Captures["FOO"] != "bar" {
		t.Errorf("Captures[FOO]: want %q, got %q", "bar", state.Captures["FOO"])
	}
}

// TestBuildState_PhaseStrings verifies the three phase string conversions.
func TestBuildState_PhaseStrings(t *testing.T) {
	vt := vars.New("/wd", "/pd", 1)
	cases := []struct {
		phase vars.Phase
		want  string
	}{
		{vars.Initialize, "initialize"},
		{vars.Iteration, "iteration"},
		{vars.Finalize, "finalize"},
	}
	for _, tc := range cases {
		s := buildState(vt, tc.phase, "", "")
		if s.Phase != tc.want {
			t.Errorf("phase %v: want %q, got %q", tc.phase, tc.want, s.Phase)
		}
	}
}

// TestBuildState_CapturesIsDefensiveCopy verifies that mutating the returned
// Captures map does not affect subsequent buildState calls or the VarTable.
func TestBuildState_CapturesIsDefensiveCopy(t *testing.T) {
	vt := vars.New("/wd", "/pd", 1)
	vt.SetPhase(vars.Initialize)
	vt.Bind(vars.Initialize, "KEY", "original")

	s1 := buildState(vt, vars.Initialize, "", "")

	// Mutate the returned map.
	s1.Captures["KEY"] = "mutated"
	s1.Captures["NEW"] = "injected"

	// A second call must return fresh copies from the VarTable.
	s2 := buildState(vt, vars.Initialize, "", "")
	if s2.Captures["KEY"] != "original" {
		t.Errorf("Captures[KEY] after mutation: want %q, got %q", "original", s2.Captures["KEY"])
	}
	if _, ok := s2.Captures["NEW"]; ok {
		t.Error("Captures[NEW] should not appear in second buildState call")
	}
}

// TestBuildState_IterationPhaseIncludesIterCaptures verifies that iteration
// captures shadow persistent ones and are included only for Iteration phase.
func TestBuildState_IterationPhaseIncludesIterCaptures(t *testing.T) {
	vt := vars.New("/wd", "/pd", 1)
	vt.SetPhase(vars.Initialize)
	vt.Bind(vars.Initialize, "PERSISTENT", "pval")
	vt.SetPhase(vars.Iteration)
	vt.Bind(vars.Iteration, "ITER_VAR", "ival")

	// Iteration phase includes both.
	iterState := buildState(vt, vars.Iteration, "", "")
	if iterState.Captures["PERSISTENT"] != "pval" {
		t.Errorf("Iteration phase: want PERSISTENT=%q, got %q", "pval", iterState.Captures["PERSISTENT"])
	}
	if iterState.Captures["ITER_VAR"] != "ival" {
		t.Errorf("Iteration phase: want ITER_VAR=%q, got %q", "ival", iterState.Captures["ITER_VAR"])
	}

	// Finalize phase excludes iteration-scoped captures.
	finalState := buildState(vt, vars.Finalize, "", "")
	if finalState.Captures["PERSISTENT"] != "pval" {
		t.Errorf("Finalize phase: want PERSISTENT=%q, got %q", "pval", finalState.Captures["PERSISTENT"])
	}
	if _, ok := finalState.Captures["ITER_VAR"]; ok {
		t.Error("Finalize phase must not include iteration-scoped ITER_VAR")
	}
}

// TestRun_StatusRunner_InitialPushNoTrigger verifies that the initial PushState
// call happens immediately (before any mutation) and does not fire a Trigger.
func TestRun_StatusRunner_InitialPushNoTrigger(t *testing.T) {
	runner := &fakeStatusRunner{}
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       nonClaudeSteps("step1"),
		Runner:      runner,
	}

	Run(executor, header, kh, cfg)

	if len(runner.pushStates) == 0 {
		t.Fatal("expected at least one PushState call")
	}
	// The first push must have phase "initialize" (zero-state before any mutation).
	if runner.pushStates[0].Phase != "initialize" {
		t.Errorf("initial push: want phase %q, got %q", "initialize", runner.pushStates[0].Phase)
	}
	// Triggers must equal pushes-1 (initial push has no trigger).
	if runner.triggers != len(runner.pushStates)-1 {
		t.Errorf("triggers: want %d (pushes-1), got %d", len(runner.pushStates)-1, runner.triggers)
	}
}

// TestRun_StatusRunner_PushAtEveryMutationSite runs a one-iteration workflow
// with one initialize step (captureAs), one iteration step (captureAs), and
// one finalize step. It asserts that PushState+Trigger fires at every
// VarTable mutation site and that phases are correct at each site.
func TestRun_StatusRunner_PushAtEveryMutationSite(t *testing.T) {
	runner := &fakeStatusRunner{}
	executor := &fakeExecutor{
		runStepCaptures: []string{"init-val", "iter-val", ""},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:     t.TempDir(),
		Iterations:      1,
		InitializeSteps: []steps.Step{captureStep("init", "INIT_VAR")},
		Steps:           []steps.Step{captureStep("iter", "ITER_RESULT")},
		FinalizeSteps:   nonClaudeSteps("final"),
		Runner:          runner,
	}

	Run(executor, header, kh, cfg)

	// Expected push sequence (index: description):
	// 0: initial (no trigger)
	// 1: SetPhase(Initialize)
	// 2: SetStep init-1
	// 3: Bind INIT_VAR
	// 4: ResetIteration
	// 5: SetIteration(1)
	// 6: SetPhase(Iteration)
	// 7: SetStep iter-1
	// 8: Bind ITER_RESULT
	// 9: SetPhase(Finalize)
	// 10: SetStep final-1
	// Total: 11 pushes, 10 triggers
	const wantPushes = 11
	const wantTriggers = 10
	if len(runner.pushStates) != wantPushes {
		t.Errorf("PushState calls: want %d, got %d", wantPushes, len(runner.pushStates))
	}
	if runner.triggers != wantTriggers {
		t.Errorf("Trigger calls: want %d, got %d", wantTriggers, runner.triggers)
	}

	// Spot-check phases at key indices (if enough pushes were recorded).
	phaseAt := func(idx int) string {
		if idx < len(runner.pushStates) {
			return runner.pushStates[idx].Phase
		}
		return "<missing>"
	}
	if phaseAt(0) != "initialize" {
		t.Errorf("push[0] phase: want %q, got %q", "initialize", phaseAt(0))
	}
	if phaseAt(1) != "initialize" {
		t.Errorf("push[1] phase (SetPhase Init): want %q, got %q", "initialize", phaseAt(1))
	}
	if phaseAt(4) != "iteration" {
		t.Errorf("push[4] phase (ResetIteration): want %q, got %q", "iteration", phaseAt(4))
	}
	if phaseAt(6) != "iteration" {
		t.Errorf("push[6] phase (SetPhase Iteration): want %q, got %q", "iteration", phaseAt(6))
	}
	if phaseAt(9) != "finalize" {
		t.Errorf("push[9] phase (SetPhase Finalize): want %q, got %q", "finalize", phaseAt(9))
	}
	if phaseAt(10) != "finalize" {
		t.Errorf("push[10] phase (SetStep finalize): want %q, got %q", "finalize", phaseAt(10))
	}
}

// TestRun_StatusRunner_NilRunnerNoPanic verifies that Run does not panic when
// Runner is nil (the common case when no status line is configured).
func TestRun_StatusRunner_NilRunnerNoPanic(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       nonClaudeSteps("step1"),
		Runner:      nil, // explicit nil
	}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Run panicked with nil Runner: %v", r)
		}
	}()
	Run(executor, header, kh, cfg)
}

// TestRun_StatusRunner_CaptureReflectedInNextPush verifies that after a Bind
// call the following State snapshot includes the captured value.
func TestRun_StatusRunner_CaptureReflectedInNextPush(t *testing.T) {
	runner := &fakeStatusRunner{}
	executor := &fakeExecutor{
		runStepCaptures: []string{"captured-value"},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:     t.TempDir(),
		Iterations:      1,
		InitializeSteps: []steps.Step{captureStep("get-val", "MY_VAR")},
		Runner:          runner,
	}

	Run(executor, header, kh, cfg)

	// Find the push that follows the Bind call (index 3 in the expected sequence:
	// 0=initial, 1=SetPhase, 2=SetStep, 3=Bind).
	const bindPushIdx = 3
	if len(runner.pushStates) <= bindPushIdx {
		t.Fatalf("not enough pushes: want at least %d, got %d", bindPushIdx+1, len(runner.pushStates))
	}
	got := runner.pushStates[bindPushIdx].Captures["MY_VAR"]
	if got != "captured-value" {
		t.Errorf("push[%d] Captures[MY_VAR]: want %q, got %q", bindPushIdx, "captured-value", got)
	}
}

// TP-004: buildState returns zero numeric fields when SetIteration and SetStep
// have not been called. strconv.Atoi("") returns (0, err); the err is discarded
// and 0 is the contract for cold-start state.
func TestBuildState_NumericFieldDefaults(t *testing.T) {
	vt := vars.New("/wf", "/pj", 0)
	s := buildState(vt, vars.Initialize, "", "")

	if s.Iteration != 0 {
		t.Errorf("Iteration: want 0, got %d", s.Iteration)
	}
	if s.StepNum != 0 {
		t.Errorf("StepNum: want 0, got %d", s.StepNum)
	}
	if s.StepCount != 0 {
		t.Errorf("StepCount: want 0, got %d", s.StepCount)
	}
	if s.MaxIterations != 0 {
		t.Errorf("MaxIterations: want 0, got %d", s.MaxIterations)
	}
	if s.StepName != "" {
		t.Errorf("StepName: want %q, got %q", "", s.StepName)
	}
	if s.Captures == nil {
		t.Error("Captures: must not be nil even when empty")
	}
}

// TP-018: buildState with empty sessionID and version passes them verbatim.
func TestBuildState_EmptySessionIDAndVersion(t *testing.T) {
	vt := vars.New("/wf", "/pj", 1)
	s := buildState(vt, vars.Initialize, "", "")

	if s.SessionID != "" {
		t.Errorf("SessionID: want %q, got %q", "", s.SessionID)
	}
	if s.Version != "" {
		t.Errorf("Version: want %q, got %q", "", s.Version)
	}
	if s.Captures == nil {
		t.Error("Captures: must not be nil")
	}
}

// TP-019: buildState scalars are populated even when Captures is empty.
func TestBuildState_ScalarsPopulatedWithEmptyCaptures(t *testing.T) {
	vt := vars.New("/wf", "/pj", 3)
	vt.SetIteration(1)
	vt.SetStep(1, 1, "foo")
	s := buildState(vt, vars.Finalize, "s", "v")

	if s.Captures == nil {
		t.Error("Captures: must not be nil when no user captures exist")
	}
	if len(s.Captures) != 0 {
		t.Errorf("Captures: expected empty, got %v", s.Captures)
	}
	if s.StepNum != 1 {
		t.Errorf("StepNum: want 1, got %d", s.StepNum)
	}
	if s.StepName != "foo" {
		t.Errorf("StepName: want %q, got %q", "foo", s.StepName)
	}
}

// TP-005: finalize-phase pushes include persistent captures (INIT_VAR) but not
// iteration-scoped ones (ITER_RESULT).
func TestRun_StatusRunner_FinalizePhaseExcludesIterCaptures(t *testing.T) {
	runner := &fakeStatusRunner{}
	executor := &fakeExecutor{
		runStepCaptures: []string{"init-value", "iter-value", ""},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:     t.TempDir(),
		Iterations:      1,
		InitializeSteps: []steps.Step{captureStep("init-step", "INIT_VAR")},
		Steps:           []steps.Step{captureStep("iter-step", "ITER_RESULT")},
		FinalizeSteps:   nonClaudeSteps("final"),
		Runner:          runner,
	}

	Run(executor, header, kh, cfg)

	// Find the first push with Phase=="finalize".
	finIdx := -1
	for i, s := range runner.pushStates {
		if s.Phase == "finalize" {
			finIdx = i
			break
		}
	}
	if finIdx < 0 {
		t.Fatal("no push with Phase=finalize found")
	}

	fin := runner.pushStates[finIdx]
	if fin.Captures["INIT_VAR"] != "init-value" {
		t.Errorf("finalize push: expected INIT_VAR=%q, got %q", "init-value", fin.Captures["INIT_VAR"])
	}
	if _, ok := fin.Captures["ITER_RESULT"]; ok {
		t.Error("finalize push: ITER_RESULT must not appear in finalize captures")
	}
}

// TP-006: ActionQuit during initialize — balance invariant triggers==pushes-1 holds,
// no iteration or finalize pushes appear.
func TestRun_StatusRunner_QuitDuringInitialize_BalanceInvariant(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	actions <- ui.ActionQuit
	kh := ui.NewKeyHandler(func() {}, actions)

	runner := &fakeStatusRunner{}
	executor := &fakeExecutor{
		runStepErrors: []error{errors.New("init failed")},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir:     t.TempDir(),
		Iterations:      1,
		InitializeSteps: nonClaudeSteps("init-step"),
		Steps:           nonClaudeSteps("iter-step"),
		FinalizeSteps:   nonClaudeSteps("final"),
		Runner:          runner,
	}

	Run(executor, header, kh, cfg)

	if runner.triggers != len(runner.pushStates)-1 {
		t.Errorf("balance invariant: triggers=%d, pushes=%d (want triggers==pushes-1)",
			runner.triggers, len(runner.pushStates))
	}
	for _, s := range runner.pushStates {
		if s.Phase == "iteration" || s.Phase == "finalize" {
			t.Errorf("quit during initialize: unexpected push with phase=%q", s.Phase)
		}
	}
}

// TP-007: ActionQuit during first iteration — balance invariant holds, no
// finalize pushes appear.
func TestRun_StatusRunner_QuitDuringIteration_BalanceInvariant(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	actions <- ui.ActionQuit
	kh := ui.NewKeyHandler(func() {}, actions)

	runner := &fakeStatusRunner{}
	executor := &fakeExecutor{
		runStepErrors: []error{errors.New("step failed")},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    3,
		Steps:         nonClaudeSteps("iter-step"),
		FinalizeSteps: nonClaudeSteps("final"),
		Runner:        runner,
	}

	Run(executor, header, kh, cfg)

	if runner.triggers != len(runner.pushStates)-1 {
		t.Errorf("balance invariant: triggers=%d, pushes=%d (want triggers==pushes-1)",
			runner.triggers, len(runner.pushStates))
	}
	for _, s := range runner.pushStates {
		if s.Phase == "finalize" {
			t.Error("quit during iteration: unexpected finalize push")
		}
	}
	// At least one push with phase=="iteration" must have appeared.
	hasIter := false
	for _, s := range runner.pushStates {
		if s.Phase == "iteration" {
			hasIter = true
		}
	}
	if !hasIter {
		t.Error("expected at least one iteration-phase push before quit")
	}
}

// TP-008: breakLoopIfEmpty — balance invariant holds; finalize pushes appear
// after the break.
func TestRun_StatusRunner_BreakLoopIfEmpty_BalanceInvariant(t *testing.T) {
	runner := &fakeStatusRunner{}
	executor := &fakeExecutor{
		runStepCaptures: []string{""}, // first iteration breaks immediately
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    3,
		Steps:         []steps.Step{breakStep("get-issue", "ISSUE_ID")},
		FinalizeSteps: nonClaudeSteps("final"),
		Runner:        runner,
	}

	Run(executor, header, kh, cfg)

	if runner.triggers != len(runner.pushStates)-1 {
		t.Errorf("balance invariant: triggers=%d, pushes=%d (want triggers==pushes-1)",
			runner.triggers, len(runner.pushStates))
	}
	hasFinal := false
	for _, s := range runner.pushStates {
		if s.Phase == "finalize" {
			hasFinal = true
		}
	}
	if !hasFinal {
		t.Error("expected finalize-phase push after breakLoopIfEmpty exit")
	}
}

// TP-009: ResetIteration clears iteration-scoped captures visible in pushes.
// The push after Bind for iteration 2 must contain ITER_VAR == "second", and
// no push for iteration 2 must contain ITER_VAR == "first".
func TestRun_StatusRunner_ResetIterationClearsCaptures(t *testing.T) {
	runner := &fakeStatusRunner{}
	executor := &fakeExecutor{
		runStepCaptures: []string{"first", "second"},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  2,
		Steps:       []steps.Step{captureStep("iter", "ITER_VAR")},
		Runner:      runner,
	}

	Run(executor, header, kh, cfg)

	if runner.triggers != len(runner.pushStates)-1 {
		t.Errorf("balance invariant: triggers=%d, pushes=%d", runner.triggers, len(runner.pushStates))
	}

	// Iteration 2's bind push must contain ITER_VAR == "second".
	found := false
	for _, s := range runner.pushStates {
		if s.Phase == "iteration" && s.Captures["ITER_VAR"] == "second" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a push with ITER_VAR=second (iteration 2 bind) but none found")
	}

	// No push from iteration 2 onward should contain ITER_VAR == "first".
	// After ResetIteration, the previous iteration's captures are cleared.
	seenSecond := false
	for _, s := range runner.pushStates {
		if s.Phase == "iteration" && s.Captures["ITER_VAR"] == "second" {
			seenSecond = true
		}
		if seenSecond && s.Captures["ITER_VAR"] == "first" {
			t.Error("found ITER_VAR=first in a push after iteration 2 started (ResetIteration should have cleared it)")
			break
		}
	}
}

// TP-020: ActionQuit during finalize — balance invariant holds.
func TestRun_StatusRunner_QuitDuringFinalize_BalanceInvariant(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	runner := &fakeStatusRunner{}
	finalizeSeen := make(chan struct{}, 1)
	executor := &fakeExecutor{
		// iter-step succeeds, final-step fails → enters error mode
		runStepErrors: []error{nil, errors.New("finalize failed")},
		onLog: func(line string) {
			// writePhaseBanner("Finalizing") writes exactly "Finalizing" as the heading.
			// Signal as soon as the finalize phase banner appears so ActionQuit lands
			// in the buffered actions channel before Orchestrate starts the step.
			if line == "Finalizing" {
				select {
				case finalizeSeen <- struct{}{}:
				default:
				}
			}
		},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    1,
		Steps:         nonClaudeSteps("iter-step"),
		FinalizeSteps: nonClaudeSteps("final-step"),
		Runner:        runner,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()

	select {
	case <-finalizeSeen:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not reach finalize phase in time")
	}
	actions <- ui.ActionQuit

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete after ActionQuit")
	}

	if runner.triggers != len(runner.pushStates)-1 {
		t.Errorf("balance invariant: triggers=%d, pushes=%d (want triggers==pushes-1)",
			runner.triggers, len(runner.pushStates))
	}
}

// TP-021: unbounded iteration (Iterations==0) with break-on-empty on first call.
// Exactly one iteration's pushes fire, finalize pushes appear, balance invariant holds.
func TestRun_StatusRunner_UnboundedIterationBreaksOnEmpty(t *testing.T) {
	runner := &fakeStatusRunner{}
	executor := &fakeExecutor{
		runStepCaptures: []string{""}, // immediately empty → break
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    0, // unbounded
		Steps:         []steps.Step{breakStep("get-issue", "ISSUE_ID")},
		FinalizeSteps: nonClaudeSteps("final"),
		Runner:        runner,
	}

	Run(executor, header, kh, cfg)

	if runner.triggers != len(runner.pushStates)-1 {
		t.Errorf("balance invariant: triggers=%d, pushes=%d", runner.triggers, len(runner.pushStates))
	}
	hasFinal := false
	for _, s := range runner.pushStates {
		if s.Phase == "finalize" {
			hasFinal = true
		}
	}
	if !hasFinal {
		t.Error("expected finalize-phase push after unbounded break-on-empty")
	}
	// Only one iteration should have occurred.
	iterCount := 0
	for _, s := range runner.pushStates {
		if s.Phase == "iteration" && s.StepNum > 0 {
			iterCount++
		}
	}
	if iterCount > 5 {
		t.Errorf("expected at most one iteration's worth of SetStep pushes, got %d", iterCount)
	}
}

// TP-022: the initial PushState call precedes any SetPhase push and has no
// corresponding Trigger. The very first push has no trigger; the second does.
func TestRun_StatusRunner_InitialPushPrecedesSetPhasePush(t *testing.T) {
	runner := &fakeStatusRunner{}
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       nonClaudeSteps("iter-step"),
		Runner:      runner,
	}

	Run(executor, header, kh, cfg)

	if len(runner.pushStates) < 2 {
		t.Fatalf("expected at least 2 pushes, got %d", len(runner.pushStates))
	}
	// First push: phase "initialize", no trigger for it.
	if runner.pushStates[0].Phase != "initialize" {
		t.Errorf("push[0] phase: want %q, got %q", "initialize", runner.pushStates[0].Phase)
	}
	// Triggers == pushes - 1 (initial push has no trigger).
	if runner.triggers != len(runner.pushStates)-1 {
		t.Errorf("balance invariant: triggers=%d, pushes=%d", runner.triggers, len(runner.pushStates))
	}
}

// TP-023: zero-step workflow — all step lists empty, Iterations==1. The exact
// push count equals the fixed-site minimum and the balance invariant holds.
func TestRun_StatusRunner_ZeroStepWorkflow_BalanceInvariant(t *testing.T) {
	runner := &fakeStatusRunner{}
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Runner:      runner,
	}

	Run(executor, header, kh, cfg)

	if runner.triggers != len(runner.pushStates)-1 {
		t.Errorf("balance invariant: triggers=%d, pushes=%d", runner.triggers, len(runner.pushStates))
	}
	// At minimum: initial + SetPhase(Init) + ResetIteration + SetIteration + SetPhase(Iter) + SetPhase(Finalize)
	const minPushes = 6
	if len(runner.pushStates) < minPushes {
		t.Errorf("expected at least %d pushes for zero-step workflow, got %d", minPushes, len(runner.pushStates))
	}
}

// TP-024: finalize-only phase — empty init and iteration steps, one finalize step.
// A finalize push with Phase=="finalize" and correct StepNum must appear.
func TestRun_StatusRunner_FinalizeOnlyPhase(t *testing.T) {
	runner := &fakeStatusRunner{}
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    1,
		FinalizeSteps: nonClaudeSteps("only-final"),
		Runner:        runner,
	}

	Run(executor, header, kh, cfg)

	if runner.triggers != len(runner.pushStates)-1 {
		t.Errorf("balance invariant: triggers=%d, pushes=%d", runner.triggers, len(runner.pushStates))
	}
	// Find a SetStep push in finalize with StepName=="only-final".
	found := false
	for _, s := range runner.pushStates {
		if s.Phase == "finalize" && s.StepName == "only-final" {
			found = true
			if s.StepNum != 1 {
				t.Errorf("finalize SetStep push: want StepNum=1, got %d", s.StepNum)
			}
		}
	}
	if !found {
		t.Error("expected a finalize push with StepName=only-final")
	}
}

// --- TP-001: buildStep delivers containerEnv to docker argv (iteration phase) ---

// TestBuildStep_ContainerEnvDelivered_IterationPhase (TP-001) verifies end-to-end
// that a non-nil containerEnv map flows through buildStep into the docker argv as
// consecutive -e KEY=VALUE pairs in sorted key order, placed after CLAUDE_CONFIG_DIR.
func TestBuildStep_ContainerEnvDelivered_IterationPhase(t *testing.T) {
	workflowDir := t.TempDir()
	promptsDir := filepath.Join(workflowDir, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "step.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	profileDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", profileDir)

	exec := &fakeExecutor{projectDir: t.TempDir()}
	vt := vars.New(workflowDir, exec.projectDir, 3)
	vt.SetPhase(vars.Iteration)
	vt.SetIteration(1)
	vt.SetStep(1, 1, "step1")

	s := steps.Step{
		Name:       "step1",
		IsClaude:   true,
		Model:      "sonnet",
		PromptFile: "step.txt",
	}
	containerEnv := map[string]string{"GOCACHE": "/tmp/gc", "GOPATH": "/tmp/go"}

	resolved, err := buildStep(workflowDir, s, vt, vars.Iteration, nil, containerEnv, exec)
	if err != nil {
		t.Fatalf("buildStep error: %v", err)
	}

	// Both entries must appear as -e KEY=VALUE pairs.
	gcacheIdx := findConsecutive(resolved.Command, "-e", "GOCACHE=/tmp/gc")
	gopathIdx := findConsecutive(resolved.Command, "-e", "GOPATH=/tmp/go")
	if gcacheIdx < 0 {
		t.Fatal("-e GOCACHE=/tmp/gc not found in command")
	}
	if gopathIdx < 0 {
		t.Fatal("-e GOPATH=/tmp/go not found in command")
	}
	// GOCACHE < GOPATH alphabetically — sorted order.
	if gcacheIdx >= gopathIdx {
		t.Errorf("GOCACHE (idx %d) must come before GOPATH (idx %d) in sorted order", gcacheIdx, gopathIdx)
	}
	// containerEnv must appear after CLAUDE_CONFIG_DIR.
	claudeConfigIdx := findConsecutivePrefix(resolved.Command, "-e", "CLAUDE_CONFIG_DIR=")
	if claudeConfigIdx < 0 {
		t.Fatal("CLAUDE_CONFIG_DIR not found in command")
	}
	if claudeConfigIdx >= gcacheIdx {
		t.Errorf("CLAUDE_CONFIG_DIR (idx %d) must come before containerEnv GOCACHE (idx %d)", claudeConfigIdx, gcacheIdx)
	}
}

// --- TP-002: containerEnv reaches all three phases ---

// TestBuildStep_ContainerEnvDelivered_AllPhases (TP-002) verifies that a non-nil
// containerEnv map is threaded through buildStep for all three phase call sites
// (initialize, iteration, finalize), so a regression dropping the parameter from
// any phase is caught.
func TestBuildStep_ContainerEnvDelivered_AllPhases(t *testing.T) {
	workflowDir := t.TempDir()
	promptsDir := filepath.Join(workflowDir, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "step.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	profileDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", profileDir)

	exec := &fakeExecutor{projectDir: t.TempDir()}
	containerEnv := map[string]string{"MY_KEY": "my_value"}
	s := steps.Step{
		Name:       "step",
		IsClaude:   true,
		Model:      "sonnet",
		PromptFile: "step.txt",
	}

	phaseTests := []struct {
		name  string
		phase vars.Phase
	}{
		{"Initialize", vars.Initialize},
		{"Iteration", vars.Iteration},
		{"Finalize", vars.Finalize},
	}

	for _, tc := range phaseTests {
		t.Run(tc.name, func(t *testing.T) {
			vt := vars.New(workflowDir, exec.projectDir, 3)
			vt.SetPhase(tc.phase)
			if tc.phase == vars.Iteration {
				vt.SetIteration(1)
			}
			vt.SetStep(1, 1, s.Name)

			resolved, err := buildStep(workflowDir, s, vt, tc.phase, nil, containerEnv, exec)
			if err != nil {
				t.Fatalf("buildStep error: %v", err)
			}

			if findConsecutive(resolved.Command, "-e", "MY_KEY=my_value") < 0 {
				t.Errorf("phase %s: -e MY_KEY=my_value not found in command %v", tc.name, resolved.Command)
			}
		})
	}
}

// findConsecutive returns the index of flag in args where args[i]==flag and
// args[i+1]==value, or -1 if not found.
func findConsecutive(args []string, flag, value string) int {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return i
		}
	}
	return -1
}

// findConsecutivePrefix returns the index where args[i]==flag and
// strings.HasPrefix(args[i+1], prefix), or -1 if not found.
func findConsecutivePrefix(args []string, flag, prefix string) int {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && strings.HasPrefix(args[i+1], prefix) {
			return i
		}
	}
	return -1
}
