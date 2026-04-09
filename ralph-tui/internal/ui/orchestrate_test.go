package ui

import (
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

// newHandleStepErrorTest creates common scaffolding for HandleStepError tests.
// It returns a stub runner (not terminated by default), a spy header, and a
// key handler backed by a buffered actions channel.
func newHandleStepErrorTest(t *testing.T, wasTerminated bool) (*stubRunner, *spyHeader, *KeyHandler) {
	t.Helper()
	stub := &stubRunner{wasTerminated: wasTerminated}
	spy := &spyHeader{}
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)
	return stub, spy, h
}

// --- HandleStepError tests ---

// T10 — ActionRetry: HandleStepError returns ActionRetry and mode returns to Normal.
func TestHandleStepError_Retry_ReturnsActionRetryAndRestoresNormalMode(t *testing.T) {
	stub, spy, h := newHandleStepErrorTest(t, false)

	done := make(chan StepAction, 1)
	go func() {
		done <- HandleStepError(stub, spy, h, 0)
	}()

	time.Sleep(30 * time.Millisecond) // let goroutine reach blocked state

	h.Actions <- ActionRetry

	select {
	case result := <-done:
		if result != ActionRetry {
			t.Errorf("expected ActionRetry, got %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("HandleStepError did not return after ActionRetry")
	}

	if h.mode != ModeNormal {
		t.Errorf("expected ModeNormal after ActionRetry, got %v", h.mode)
	}

	state, ok := spy.lastStateFor(0)
	if !ok || state != StepFailed {
		t.Errorf("expected StepFailed for step 0, got %v (ok=%v)", state, ok)
	}
}

// T11 — ActionContinue: HandleStepError returns ActionContinue.
func TestHandleStepError_Continue_ReturnsActionContinue(t *testing.T) {
	stub, spy, h := newHandleStepErrorTest(t, false)

	done := make(chan StepAction, 1)
	go func() {
		done <- HandleStepError(stub, spy, h, 1)
	}()

	time.Sleep(30 * time.Millisecond)

	h.Actions <- ActionContinue

	select {
	case result := <-done:
		if result != ActionContinue {
			t.Errorf("expected ActionContinue, got %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("HandleStepError did not return after ActionContinue")
	}

	if h.mode != ModeNormal {
		t.Errorf("expected ModeNormal after ActionContinue, got %v", h.mode)
	}
}

// T12 — ActionQuit: HandleStepError returns ActionQuit.
func TestHandleStepError_Quit_ReturnsActionQuit(t *testing.T) {
	stub, spy, h := newHandleStepErrorTest(t, false)

	done := make(chan StepAction, 1)
	go func() {
		done <- HandleStepError(stub, spy, h, 2)
	}()

	time.Sleep(30 * time.Millisecond)

	h.Actions <- ActionQuit

	select {
	case result := <-done:
		if result != ActionQuit {
			t.Errorf("expected ActionQuit, got %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("HandleStepError did not return after ActionQuit")
	}
}

// T13 — Terminated step: HandleStepError treats user-terminated step as
// continue — marks it StepDone, returns ActionContinue without entering error mode.
func TestHandleStepError_TerminatedStep_TreatedAsContinue(t *testing.T) {
	stub, spy, h := newHandleStepErrorTest(t, true /* wasTerminated */)

	result := HandleStepError(stub, spy, h, 0)

	if result != ActionContinue {
		t.Errorf("expected ActionContinue for terminated step, got %v", result)
	}

	state, ok := spy.lastStateFor(0)
	if !ok || state != StepDone {
		t.Errorf("expected StepDone for terminated step, got %v (ok=%v)", state, ok)
	}

	if h.mode != ModeNormal {
		t.Errorf("expected ModeNormal (error mode must not be entered), got %v", h.mode)
	}
}
