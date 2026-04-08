package ui

import (
	"strings"
	"testing"
)

var testStepNames = [8]string{
	"Feature work",
	"Test planning",
	"Test writing",
	"Code review",
	"Review fixes",
	"Close issue",
	"Update docs",
	"Git push",
}

func TestStatusHeader_IterationLine(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	h.SetIteration(2, 5, "42", "Add widget support")

	want := "Iteration 2/5 — Issue #42: Add widget support"
	if h.IterationLine != want {
		t.Errorf("got %q, want %q", h.IterationLine, want)
	}
}

func TestStatusHeader_StepCheckboxStates(t *testing.T) {
	h := NewStatusHeader(testStepNames)

	// Steps 0-2 done, step 3 active, rest pending
	h.SetStepState(0, StepDone)
	h.SetStepState(1, StepDone)
	h.SetStepState(2, StepDone)
	h.SetStepState(3, StepActive)

	cases := []struct {
		got  string
		want string
	}{
		{h.Row1[0], "[✓] Feature work"},
		{h.Row1[1], "[✓] Test planning"},
		{h.Row1[2], "[✓] Test writing"},
		{h.Row1[3], "[▸] Code review"},
		{h.Row2[0], "[ ] Review fixes"},
		{h.Row2[1], "[ ] Close issue"},
		{h.Row2[2], "[ ] Update docs"},
		{h.Row2[3], "[ ] Git push"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestStatusHeader_FailedStep(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	h.SetStepState(2, StepFailed)

	if h.Row1[2] != "[✗] Test writing" {
		t.Errorf("failed step: got %q, want %q", h.Row1[2], "[✗] Test writing")
	}
}

func TestStatusHeader_FinalizationMode(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	finalSteps := []string{"Deferred work", "Lessons learned", "Final git push"}
	h.SetFinalization(1, 3, finalSteps)

	if !strings.HasPrefix(h.IterationLine, "Finalizing") {
		t.Errorf("expected IterationLine to start with 'Finalizing', got %q", h.IterationLine)
	}
	if !strings.Contains(h.IterationLine, "1/3") {
		t.Errorf("expected '1/3' in IterationLine, got %q", h.IterationLine)
	}
	if h.Row1[0] != "[ ] Deferred work" {
		t.Errorf("finalize step 0: got %q, want %q", h.Row1[0], "[ ] Deferred work")
	}
	if h.Row1[1] != "[ ] Lessons learned" {
		t.Errorf("finalize step 1: got %q, want %q", h.Row1[1], "[ ] Lessons learned")
	}
	if h.Row1[2] != "[ ] Final git push" {
		t.Errorf("finalize step 2: got %q, want %q", h.Row1[2], "[ ] Final git push")
	}
	if h.Row1[3] != "" {
		t.Errorf("finalize step 3 should be empty, got %q", h.Row1[3])
	}
	// Row2 unused for 3 finalization steps
	for i, v := range h.Row2 {
		if v != "" {
			t.Errorf("Row2[%d] should be empty for 3 finalization steps, got %q", i, v)
		}
	}
}

func TestStatusHeader_TwoRowsOfFour(t *testing.T) {
	h := NewStatusHeader(testStepNames)

	// Row1 holds steps 0-3 (all pending on init)
	for i, name := range testStepNames[:4] {
		want := "[ ] " + name
		if h.Row1[i] != want {
			t.Errorf("Row1[%d] = %q, want %q", i, h.Row1[i], want)
		}
	}
	// Row2 holds steps 4-7 (all pending on init)
	for i, name := range testStepNames[4:] {
		want := "[ ] " + name
		if h.Row2[i] != want {
			t.Errorf("Row2[%d] = %q, want %q", i, h.Row2[i], want)
		}
	}
}

// T1 — SetStepState ignores out-of-bounds index
func TestStatusHeader_SetStepState_OutOfBounds(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	row1Before := h.Row1
	row2Before := h.Row2

	h.SetStepState(-1, StepDone)
	h.SetStepState(8, StepDone)

	if h.Row1 != row1Before {
		t.Errorf("Row1 changed after out-of-bounds SetStepState: got %v, want %v", h.Row1, row1Before)
	}
	if h.Row2 != row2Before {
		t.Errorf("Row2 changed after out-of-bounds SetStepState: got %v, want %v", h.Row2, row2Before)
	}
}

// T2 — SetFinalizeStepState is no-op before SetFinalization
func TestStatusHeader_SetFinalizeStepState_BeforeSetFinalization(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	row1Before := h.Row1
	row2Before := h.Row2

	h.SetFinalizeStepState(0, StepDone)

	if h.Row1 != row1Before {
		t.Errorf("Row1 changed after SetFinalizeStepState before SetFinalization: got %v, want %v", h.Row1, row1Before)
	}
	if h.Row2 != row2Before {
		t.Errorf("Row2 changed after SetFinalizeStepState before SetFinalization: got %v, want %v", h.Row2, row2Before)
	}
}

// T3 — SetFinalizeStepState ignores out-of-bounds index after SetFinalization
func TestStatusHeader_SetFinalizeStepState_OutOfBounds(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	finalSteps := []string{"Deferred work", "Lessons learned", "Final git push"}
	h.SetFinalization(1, 3, finalSteps)
	row1Before := h.Row1
	row2Before := h.Row2

	h.SetFinalizeStepState(3, StepDone)
	h.SetFinalizeStepState(-1, StepDone)

	if h.Row1 != row1Before {
		t.Errorf("Row1 changed after out-of-bounds SetFinalizeStepState: got %v, want %v", h.Row1, row1Before)
	}
	if h.Row2 != row2Before {
		t.Errorf("Row2 changed after out-of-bounds SetFinalizeStepState: got %v, want %v", h.Row2, row2Before)
	}
}

// T4 — SetFinalizeStepState updates Row2 for idx >= 4
func TestStatusHeader_SetFinalizeStepState_Row2Update(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	finalSteps := []string{"Step A", "Step B", "Step C", "Step D", "Step E", "Step F"}
	h.SetFinalization(1, 6, finalSteps)

	h.SetFinalizeStepState(5, StepDone)

	if h.Row2[1] != "[✓] Step F" {
		t.Errorf("Row2[1] = %q, want %q", h.Row2[1], "[✓] Step F")
	}
	if h.Row2[0] != "[ ] Step E" {
		t.Errorf("Row2[0] = %q, want %q (should still be pending)", h.Row2[0], "[ ] Step E")
	}
}

// T5 — SetFinalization with >4 steps spans both rows
func TestStatusHeader_SetFinalization_SixSteps(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	finalSteps := []string{"Step A", "Step B", "Step C", "Step D", "Step E", "Step F"}
	h.SetFinalization(1, 6, finalSteps)

	for i := range 4 {
		want := "[ ] " + finalSteps[i]
		if h.Row1[i] != want {
			t.Errorf("Row1[%d] = %q, want %q", i, h.Row1[i], want)
		}
	}
	if h.Row2[0] != "[ ] Step E" {
		t.Errorf("Row2[0] = %q, want %q", h.Row2[0], "[ ] Step E")
	}
	if h.Row2[1] != "[ ] Step F" {
		t.Errorf("Row2[1] = %q, want %q", h.Row2[1], "[ ] Step F")
	}
	if h.Row2[2] != "" {
		t.Errorf("Row2[2] should be empty, got %q", h.Row2[2])
	}
	if h.Row2[3] != "" {
		t.Errorf("Row2[3] should be empty, got %q", h.Row2[3])
	}
}

// T6 — SetFinalization overwrites prior iteration state
func TestStatusHeader_SetFinalization_OverwritesIterationState(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	h.SetStepState(0, StepDone)
	h.SetStepState(1, StepDone)
	h.SetStepState(4, StepActive)

	finalSteps := []string{"Deferred work", "Lessons learned", "Final git push"}
	h.SetFinalization(1, 3, finalSteps)

	if h.Row1[0] != "[ ] Deferred work" {
		t.Errorf("Row1[0] = %q, want %q", h.Row1[0], "[ ] Deferred work")
	}
	if h.Row1[1] != "[ ] Lessons learned" {
		t.Errorf("Row1[1] = %q, want %q", h.Row1[1], "[ ] Lessons learned")
	}
	if h.Row1[2] != "[ ] Final git push" {
		t.Errorf("Row1[2] = %q, want %q", h.Row1[2], "[ ] Final git push")
	}
	if h.Row1[3] != "" {
		t.Errorf("Row1[3] should be empty, got %q", h.Row1[3])
	}
	for i, v := range h.Row2 {
		if v != "" {
			t.Errorf("Row2[%d] should be empty, got %q", i, v)
		}
	}
}

// T7 — SetFinalization with 0 steps
func TestStatusHeader_SetFinalization_ZeroSteps(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	h.SetFinalization(0, 0, []string{})

	want := "Finalizing 0/0"
	if h.IterationLine != want {
		t.Errorf("IterationLine = %q, want %q", h.IterationLine, want)
	}
	for i, v := range h.Row1 {
		if v != "" {
			t.Errorf("Row1[%d] should be empty, got %q", i, v)
		}
	}
	for i, v := range h.Row2 {
		if v != "" {
			t.Errorf("Row2[%d] should be empty, got %q", i, v)
		}
	}
}
