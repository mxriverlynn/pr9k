package ui

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Test doubles ---

type stubRunner struct {
	results       []error // one per RunStep call; nil = success
	callCount     int
	wasTerminated bool
	logLines      []string
}

func (s *stubRunner) RunStep(_ string, _ []string) error {
	var err error
	if s.callCount < len(s.results) {
		err = s.results[s.callCount]
	}
	s.callCount++
	return err
}

func (s *stubRunner) WasTerminated() bool    { return s.wasTerminated }
func (s *stubRunner) WriteToLog(line string) { s.logLines = append(s.logLines, line) }

// callbackStubRunner is a StepRunner whose RunStep behaviour is controlled by
// a callback. Use this when you need precise timing control in tests.
type callbackStubRunner struct {
	onRunStep       func(name string) error
	wasTerminatedFn func() bool
	logLines        []string
}

func (c *callbackStubRunner) RunStep(name string, _ []string) error {
	return c.onRunStep(name)
}
func (c *callbackStubRunner) WasTerminated() bool    { return c.wasTerminatedFn() }
func (c *callbackStubRunner) WriteToLog(line string) { c.logLines = append(c.logLines, line) }

type spyHeader struct {
	mu    sync.Mutex
	calls []struct {
		idx   int
		state StepState
	}
}

func (h *spyHeader) SetStepState(idx int, state StepState) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, struct {
		idx   int
		state StepState
	}{idx, state})
}

func (h *spyHeader) lastStateFor(idx int) (StepState, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := len(h.calls) - 1; i >= 0; i-- {
		if h.calls[i].idx == idx {
			return h.calls[i].state, true
		}
	}
	return 0, false
}

// newOrchestrateTest returns the common test scaffolding.
func newOrchestrateTest(t *testing.T, results ...error) (*stubRunner, *spyHeader, *KeyHandler, []ResolvedStep) {
	t.Helper()
	stub := &stubRunner{results: results}
	spy := &spyHeader{}
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)
	steps := []ResolvedStep{{Name: "step0", Command: []string{"x"}}}
	return stub, spy, h, steps
}

// runOrchestrate starts Orchestrate in a goroutine and returns a channel
// that receives the final StepAction when it returns.
func runOrchestrate(steps []ResolvedStep, runner StepRunner, header StepHeader, h *KeyHandler) <-chan StepAction {
	ch := make(chan StepAction, 1)
	go func() {
		ch <- Orchestrate(steps, runner, header, h)
	}()
	return ch
}

// --- Error state: checkbox and shortcut bar ---

// T1 — Non-zero exit triggers StepFailed state on the header.
func TestOrchestrate_NonZeroExit_SetsStepFailed(t *testing.T) {
	stub, spy, h, steps := newOrchestrateTest(t, errors.New("exit 1"))
	done := runOrchestrate(steps, stub, spy, h)

	time.Sleep(30 * time.Millisecond) // let orchestration reach blocked state

	state, ok := spy.lastStateFor(0)
	if !ok || state != StepFailed {
		t.Errorf("expected StepFailed for step 0, got %v (ok=%v)", state, ok)
	}

	h.Actions <- ActionContinue
	<-done
}

// T2 — Non-zero exit switches shortcut bar to error mode.
func TestOrchestrate_NonZeroExit_SetsErrorShortcuts(t *testing.T) {
	stub, spy, h, steps := newOrchestrateTest(t, errors.New("exit 1"))
	done := runOrchestrate(steps, stub, spy, h)

	time.Sleep(30 * time.Millisecond) // let orchestration reach blocked state

	if h.ShortcutLine() != ErrorShortcuts {
		t.Errorf("expected ErrorShortcuts, got %q", h.ShortcutLine())
	}

	h.Actions <- ActionContinue
	<-done
}

// --- Continue ---

// T3 — ActionContinue advances to the next step (two-step sequence).
func TestOrchestrate_Continue_AdvancesToNextStep(t *testing.T) {
	stub := &stubRunner{results: []error{errors.New("fail"), nil}}
	spy := &spyHeader{}
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)
	steps := []ResolvedStep{
		{Name: "step0", Command: []string{"x"}},
		{Name: "step1", Command: []string{"x"}},
	}

	done := runOrchestrate(steps, stub, spy, h)

	time.Sleep(30 * time.Millisecond) // blocked on error for step 0

	h.Actions <- ActionContinue

	select {
	case result := <-done:
		if result != ActionContinue {
			t.Errorf("expected ActionContinue result, got %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Orchestrate did not return after ActionContinue")
	}

	// Step 1 should have been activated and completed.
	state1, ok := spy.lastStateFor(1)
	if !ok || state1 != StepDone {
		t.Errorf("expected step 1 to be StepDone, got %v (ok=%v)", state1, ok)
	}
}

// --- Retry ---

// T4 — ActionRetry causes the step to be re-executed.
func TestOrchestrate_Retry_ReExecutesStep(t *testing.T) {
	stub, spy, h, steps := newOrchestrateTest(t, errors.New("fail"), nil)
	done := runOrchestrate(steps, stub, spy, h)

	time.Sleep(30 * time.Millisecond)

	h.Actions <- ActionRetry

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Orchestrate did not return after ActionRetry")
	}

	if stub.callCount != 2 {
		t.Errorf("expected step to run twice (retry), got callCount=%d", stub.callCount)
	}
	_ = spy
}

// T5 — ActionRetry writes a separator containing "(retry)" to the log.
func TestOrchestrate_Retry_WritesSeparatorWithRetrySuffix(t *testing.T) {
	stub := &stubRunner{results: []error{errors.New("fail"), nil}}
	spy := &spyHeader{}
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)
	steps := []ResolvedStep{{Name: "my-step", Command: []string{"x"}}}

	done := runOrchestrate(steps, stub, spy, h)

	time.Sleep(30 * time.Millisecond)
	h.Actions <- ActionRetry

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Orchestrate did not return after ActionRetry")
	}

	found := false
	for _, line := range stub.logLines {
		if strings.Contains(line, "(retry)") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a log line containing '(retry)', got %v", stub.logLines)
	}
}

// --- User-initiated skip does NOT trigger error state ---

// T6 — When WasTerminated() is true, non-zero exit is treated as a skip,
// not a failure — step is marked done and error mode is not entered.
func TestOrchestrate_UserInitiatedSkip_DoesNotTriggerErrorState(t *testing.T) {
	stub := &stubRunner{
		results:       []error{errors.New("killed")},
		wasTerminated: true,
	}
	spy := &spyHeader{}
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)
	steps := []ResolvedStep{{Name: "step0", Command: []string{"x"}}}

	done := runOrchestrate(steps, stub, spy, h)

	select {
	case result := <-done:
		if result != ActionContinue {
			t.Errorf("expected ActionContinue, got %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Orchestrate should have returned immediately for user-initiated skip")
	}

	// Must not have set StepFailed.
	for _, c := range spy.calls {
		if c.idx == 0 && c.state == StepFailed {
			t.Error("step 0 must not be set to StepFailed for a user-initiated skip")
		}
	}

	// Mode must remain normal (error mode must not have been entered).
	if h.mode != ModeNormal {
		t.Errorf("expected ModeNormal after user-initiated skip, got %v", h.mode)
	}
}

// --- Orchestration blocks until action is received ---

// T7 — Orchestration goroutine blocks on the Actions channel during error state.
func TestOrchestrate_BlocksOnActionChannelDuringErrorState(t *testing.T) {
	stub, spy, h, steps := newOrchestrateTest(t, errors.New("exit 1"))
	done := runOrchestrate(steps, stub, spy, h)

	time.Sleep(30 * time.Millisecond) // let orchestration reach blocking point

	// Must still be blocked.
	select {
	case <-done:
		t.Fatal("Orchestrate returned before an action was sent")
	default:
		// correct — still blocking
	}

	h.Actions <- ActionContinue

	select {
	case <-done:
		// unblocked correctly
	case <-time.After(time.Second):
		t.Fatal("Orchestrate did not return after action was sent")
	}
}

// --- Quit ---

// --- Happy path ---

// Gap 1 — All steps succeed without error; both are marked StepDone.
func TestOrchestrate_AllStepsSucceed_ReturnsActionContinue(t *testing.T) {
	stub := &stubRunner{results: []error{nil, nil}}
	spy := &spyHeader{}
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)
	steps := []ResolvedStep{
		{Name: "step0", Command: []string{"x"}},
		{Name: "step1", Command: []string{"x"}},
	}

	done := runOrchestrate(steps, stub, spy, h)

	select {
	case result := <-done:
		if result != ActionContinue {
			t.Errorf("expected ActionContinue, got %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Orchestrate did not return for all-success run")
	}

	for i := range 2 {
		state, ok := spy.lastStateFor(i)
		if !ok || state != StepDone {
			t.Errorf("expected step %d to be StepDone, got %v (ok=%v)", i, state, ok)
		}
	}
}

// Gap 2 — Empty steps slice returns immediately with ActionContinue.
func TestOrchestrate_EmptySteps_ReturnsActionContinue(t *testing.T) {
	stub := &stubRunner{}
	spy := &spyHeader{}
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)

	done := runOrchestrate([]ResolvedStep{}, stub, spy, h)

	select {
	case result := <-done:
		if result != ActionContinue {
			t.Errorf("expected ActionContinue, got %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Orchestrate did not return for empty steps")
	}

	if stub.callCount != 0 {
		t.Errorf("expected no RunStep calls for empty steps, got %d", stub.callCount)
	}
}

// --- Retry fails again ---

// Gap 3 — Retry fails again; user continues on the second failure.
func TestOrchestrate_RetryFailsAgain_ThenContinue(t *testing.T) {
	stub := &stubRunner{results: []error{errors.New("fail1"), errors.New("fail2")}}
	spy := &spyHeader{}
	actions := make(chan StepAction, 2)
	h := NewKeyHandler(nil, actions)
	steps := []ResolvedStep{{Name: "step0", Command: []string{"x"}}}

	done := runOrchestrate(steps, stub, spy, h)

	time.Sleep(30 * time.Millisecond) // blocked on first error
	h.Actions <- ActionRetry

	time.Sleep(30 * time.Millisecond) // blocked on second error
	h.Actions <- ActionContinue

	select {
	case result := <-done:
		if result != ActionContinue {
			t.Errorf("expected ActionContinue, got %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Orchestrate did not return after retry-fail-continue")
	}

	if stub.callCount != 2 {
		t.Errorf("expected 2 RunStep calls (original + retry), got %d", stub.callCount)
	}

	state, ok := spy.lastStateFor(0)
	if !ok || state != StepFailed {
		t.Errorf("expected step 0 to remain StepFailed after continue, got %v (ok=%v)", state, ok)
	}
}

// --- Quit mid-sequence ---

// Gap 4 — Quitting on the first failed step prevents subsequent steps from running.
func TestOrchestrate_QuitMidSequence_SkipsRemainingSteps(t *testing.T) {
	stub := &stubRunner{results: []error{errors.New("fail"), nil}}
	spy := &spyHeader{}
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)
	steps := []ResolvedStep{
		{Name: "step0", Command: []string{"x"}},
		{Name: "step1", Command: []string{"x"}},
	}

	done := runOrchestrate(steps, stub, spy, h)

	time.Sleep(30 * time.Millisecond) // blocked on step 0 error
	h.Actions <- ActionQuit

	select {
	case result := <-done:
		if result != ActionQuit {
			t.Errorf("expected ActionQuit, got %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Orchestrate did not return after ActionQuit")
	}

	if stub.callCount != 1 {
		t.Errorf("expected only 1 RunStep call (step 1 skipped), got %d", stub.callCount)
	}
}

// --- Mode restoration ---

// Gap 5 — After ActionContinue, mode and shortcut bar are restored to Normal.
func TestOrchestrate_Continue_RestoresModeToNormal(t *testing.T) {
	stub := &stubRunner{results: []error{errors.New("fail")}}
	spy := &spyHeader{}
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)
	steps := []ResolvedStep{{Name: "step0", Command: []string{"x"}}}

	done := runOrchestrate(steps, stub, spy, h)

	time.Sleep(30 * time.Millisecond) // blocked in error state
	h.Actions <- ActionContinue

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Orchestrate did not return after ActionContinue")
	}

	if h.ShortcutLine() != NormalShortcuts {
		t.Errorf("expected NormalShortcuts after continue, got %q", h.ShortcutLine())
	}
	if h.mode != ModeNormal {
		t.Errorf("expected ModeNormal after continue, got %v", h.mode)
	}
}

// --- Quit ---

// TestOrchestrate_PendingQuitBeforeFirstStep_ReturnsActionQuitImmediately
// verifies that when ActionQuit is already in the channel before Orchestrate
// starts, it returns ActionQuit without running any steps.
func TestOrchestrate_PendingQuitBeforeFirstStep_ReturnsActionQuitImmediately(t *testing.T) {
	stub := &stubRunner{results: []error{nil, nil}}
	spy := &spyHeader{}
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)
	twoSteps := []ResolvedStep{
		{Name: "step0", Command: []string{"x"}},
		{Name: "step1", Command: []string{"x"}},
	}

	// Pre-inject ActionQuit before Orchestrate starts (simulates OS signal arriving
	// between iterations, before any step of the new iteration has begun).
	h.Actions <- ActionQuit

	done := runOrchestrate(twoSteps, stub, spy, h)

	select {
	case result := <-done:
		if result != ActionQuit {
			t.Errorf("expected ActionQuit, got %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Orchestrate did not return after pre-injected ActionQuit")
	}

	if stub.callCount != 0 {
		t.Errorf("expected no RunStep calls when quit was pending before first step, got %d", stub.callCount)
	}
}

// TestOrchestrate_ForceQuitAfterStep_SkipsNextStep verifies that when
// ActionQuit is injected after step0 completes (simulating ForceQuit called
// during step0 which was terminated), step1 is not run.
func TestOrchestrate_ForceQuitAfterStep_SkipsNextStep(t *testing.T) {
	stepStarted := make(chan struct{})
	stepUnblock := make(chan struct{})
	callCount := 0

	// Use a channel-based stub to control timing precisely.
	cbRunner := &callbackStubRunner{
		onRunStep: func(name string) error {
			if name == "step0" {
				close(stepStarted)
				<-stepUnblock
				callCount++
				return errors.New("terminated")
			}
			callCount++
			return nil
		},
		wasTerminatedFn: func() bool { return true },
	}

	spy := &spyHeader{}
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)
	twoSteps := []ResolvedStep{
		{Name: "step0", Command: []string{"x"}},
		{Name: "step1", Command: []string{"x"}},
	}

	done := runOrchestrate(twoSteps, cbRunner, spy, h)

	// Wait for step0 to start, then inject ActionQuit and unblock step0.
	<-stepStarted
	h.Actions <- ActionQuit
	close(stepUnblock)

	select {
	case result := <-done:
		if result != ActionQuit {
			t.Errorf("expected ActionQuit, got %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Orchestrate did not return after ForceQuit during step0")
	}

	if callCount != 1 {
		t.Errorf("expected only step0 to run, got callCount=%d", callCount)
	}
}

// T3 — A non-quit action (ActionContinue) pre-injected into the channel is
// silently consumed by the pre-step drain; both steps run and Orchestrate
// returns ActionContinue.
func TestOrchestrate_PreStepDrain_ConsumesNonQuitAction_BothStepsRun(t *testing.T) {
	stub := &stubRunner{results: []error{nil, nil}}
	spy := &spyHeader{}
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)
	twoSteps := []ResolvedStep{
		{Name: "step0", Command: []string{"x"}},
		{Name: "step1", Command: []string{"x"}},
	}

	// Pre-inject a stale ActionContinue — must not cause an early exit.
	h.Actions <- ActionContinue

	done := runOrchestrate(twoSteps, stub, spy, h)

	select {
	case result := <-done:
		if result != ActionContinue {
			t.Errorf("expected ActionContinue, got %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Orchestrate did not return after consuming stale ActionContinue")
	}

	if stub.callCount != 2 {
		t.Errorf("expected both steps to run (callCount=2), got %d", stub.callCount)
	}
}

// T4 — ActionQuit injected after step0 succeeds skips step1 and returns ActionQuit.
func TestOrchestrate_ForceQuitAfterSuccessfulStep_SkipsNextStep(t *testing.T) {
	step0Done := make(chan struct{})
	step0Unblock := make(chan struct{})
	callCount := 0

	cbRunner := &callbackStubRunner{
		onRunStep: func(name string) error {
			callCount++
			if name == "step0" {
				close(step0Done)
				<-step0Unblock
			}
			return nil // both steps succeed
		},
		wasTerminatedFn: func() bool { return false },
	}

	spy := &spyHeader{}
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)
	twoSteps := []ResolvedStep{
		{Name: "step0", Command: []string{"x"}},
		{Name: "step1", Command: []string{"x"}},
	}

	done := runOrchestrate(twoSteps, cbRunner, spy, h)

	// Wait for step0 to start, inject ActionQuit, then let step0 finish.
	<-step0Done
	h.Actions <- ActionQuit
	close(step0Unblock)

	select {
	case result := <-done:
		if result != ActionQuit {
			t.Errorf("expected ActionQuit, got %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Orchestrate did not return after ForceQuit between successful steps")
	}

	if callCount != 1 {
		t.Errorf("expected only step0 to run, got callCount=%d", callCount)
	}
}

// TestOrchestrate_Quit_ReturnsActionQuit passes through the quit action.
func TestOrchestrate_Quit_ReturnsActionQuit(t *testing.T) {
	stub, spy, h, steps := newOrchestrateTest(t, errors.New("exit 1"))
	done := runOrchestrate(steps, stub, spy, h)

	time.Sleep(30 * time.Millisecond)

	h.Actions <- ActionQuit

	select {
	case result := <-done:
		if result != ActionQuit {
			t.Errorf("expected ActionQuit, got %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Orchestrate did not return after ActionQuit")
	}
}
