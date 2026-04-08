package ui

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// --- Test doubles ---

type stubRunner struct {
	results    []error // one per RunStep call; nil = success
	callCount  int
	wasTerminated bool
	logLines   []string
}

func (s *stubRunner) RunStep(_ string, _ []string) error {
	var err error
	if s.callCount < len(s.results) {
		err = s.results[s.callCount]
	}
	s.callCount++
	return err
}

func (s *stubRunner) WasTerminated() bool { return s.wasTerminated }
func (s *stubRunner) WriteToLog(line string) { s.logLines = append(s.logLines, line) }

type spyHeader struct {
	calls []struct {
		idx   int
		state StepState
	}
}

func (h *spyHeader) SetStepState(idx int, state StepState) {
	h.calls = append(h.calls, struct {
		idx   int
		state StepState
	}{idx, state})
}

func (h *spyHeader) lastStateFor(idx int) (StepState, bool) {
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

	if h.ShortcutLine != ErrorShortcuts {
		t.Errorf("expected ErrorShortcuts, got %q", h.ShortcutLine)
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

	if h.ShortcutLine != NormalShortcuts {
		t.Errorf("expected NormalShortcuts after continue, got %q", h.ShortcutLine)
	}
	if h.mode != ModeNormal {
		t.Errorf("expected ModeNormal after continue, got %v", h.mode)
	}
}

// --- Quit ---

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
