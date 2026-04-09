package ui

import (
	"reflect"
	"strings"
	"testing"
)

var testStepNames = []string{
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

// 3 finalization steps → rowSize=(3+1)/2=2 → Row1=[2], Row2=[1]
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
	if len(h.Row1) != 2 {
		t.Errorf("Row1 len = %d, want 2", len(h.Row1))
	}
	if len(h.Row2) != 1 {
		t.Errorf("Row2 len = %d, want 1", len(h.Row2))
	}
	if h.Row1[0] != "[ ] Deferred work" {
		t.Errorf("finalize step 0: got %q, want %q", h.Row1[0], "[ ] Deferred work")
	}
	if h.Row1[1] != "[ ] Lessons learned" {
		t.Errorf("finalize step 1: got %q, want %q", h.Row1[1], "[ ] Lessons learned")
	}
	if h.Row2[0] != "[ ] Final git push" {
		t.Errorf("finalize step 2: got %q, want %q", h.Row2[0], "[ ] Final git push")
	}
}

// 8 steps → rowSize=4 → Row1=[4], Row2=[4]
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
	row1Before := append([]string(nil), h.Row1...)
	row2Before := append([]string(nil), h.Row2...)

	h.SetStepState(-1, StepDone)
	h.SetStepState(8, StepDone)

	if !reflect.DeepEqual(h.Row1, row1Before) {
		t.Errorf("Row1 changed after out-of-bounds SetStepState: got %v, want %v", h.Row1, row1Before)
	}
	if !reflect.DeepEqual(h.Row2, row2Before) {
		t.Errorf("Row2 changed after out-of-bounds SetStepState: got %v, want %v", h.Row2, row2Before)
	}
}

// T2 — SetFinalizeStepState is no-op before SetFinalization
func TestStatusHeader_SetFinalizeStepState_BeforeSetFinalization(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	row1Before := append([]string(nil), h.Row1...)
	row2Before := append([]string(nil), h.Row2...)

	h.SetFinalizeStepState(0, StepDone)

	if !reflect.DeepEqual(h.Row1, row1Before) {
		t.Errorf("Row1 changed after SetFinalizeStepState before SetFinalization: got %v, want %v", h.Row1, row1Before)
	}
	if !reflect.DeepEqual(h.Row2, row2Before) {
		t.Errorf("Row2 changed after SetFinalizeStepState before SetFinalization: got %v, want %v", h.Row2, row2Before)
	}
}

// T3 — SetFinalizeStepState ignores out-of-bounds index after SetFinalization
// 3 finalization steps → len=3, valid indices 0-2; idx=3 is out-of-bounds
func TestStatusHeader_SetFinalizeStepState_OutOfBounds(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	finalSteps := []string{"Deferred work", "Lessons learned", "Final git push"}
	h.SetFinalization(1, 3, finalSteps)
	row1Before := append([]string(nil), h.Row1...)
	row2Before := append([]string(nil), h.Row2...)

	h.SetFinalizeStepState(3, StepDone)
	h.SetFinalizeStepState(-1, StepDone)

	if !reflect.DeepEqual(h.Row1, row1Before) {
		t.Errorf("Row1 changed after out-of-bounds SetFinalizeStepState: got %v, want %v", h.Row1, row1Before)
	}
	if !reflect.DeepEqual(h.Row2, row2Before) {
		t.Errorf("Row2 changed after out-of-bounds SetFinalizeStepState: got %v, want %v", h.Row2, row2Before)
	}
}

// T4 — SetFinalizeStepState updates Row2 for idx >= rowSize
// 6 finalization steps → rowSize=(6+1)/2=3 → Row1=[A,B,C], Row2=[D,E,F]
// idx=5 → Row2[5-3=2] = "[✓] Step F"
func TestStatusHeader_SetFinalizeStepState_Row2Update(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	finalSteps := []string{"Step A", "Step B", "Step C", "Step D", "Step E", "Step F"}
	h.SetFinalization(1, 6, finalSteps)

	h.SetFinalizeStepState(5, StepDone)

	if h.Row2[2] != "[✓] Step F" {
		t.Errorf("Row2[2] = %q, want %q", h.Row2[2], "[✓] Step F")
	}
	if h.Row2[1] != "[ ] Step E" {
		t.Errorf("Row2[1] = %q, want %q (should still be pending)", h.Row2[1], "[ ] Step E")
	}
}

// T5 — SetFinalization with 6 steps spans both rows
// rowSize=(6+1)/2=3 → Row1=[A,B,C] (3 entries), Row2=[D,E,F] (3 entries)
func TestStatusHeader_SetFinalization_SixSteps(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	finalSteps := []string{"Step A", "Step B", "Step C", "Step D", "Step E", "Step F"}
	h.SetFinalization(1, 6, finalSteps)

	if len(h.Row1) != 3 {
		t.Errorf("Row1 len = %d, want 3", len(h.Row1))
	}
	if len(h.Row2) != 3 {
		t.Errorf("Row2 len = %d, want 3", len(h.Row2))
	}
	for i := range 3 {
		want := "[ ] " + finalSteps[i]
		if h.Row1[i] != want {
			t.Errorf("Row1[%d] = %q, want %q", i, h.Row1[i], want)
		}
	}
	if h.Row2[0] != "[ ] Step D" {
		t.Errorf("Row2[0] = %q, want %q", h.Row2[0], "[ ] Step D")
	}
	if h.Row2[1] != "[ ] Step E" {
		t.Errorf("Row2[1] = %q, want %q", h.Row2[1], "[ ] Step E")
	}
	if h.Row2[2] != "[ ] Step F" {
		t.Errorf("Row2[2] = %q, want %q", h.Row2[2], "[ ] Step F")
	}
}

// T6 — SetFinalization overwrites prior iteration state
// 3 finalization steps → rowSize=2 → Row1=[Deferred, Lessons], Row2=[Final push]
func TestStatusHeader_SetFinalization_OverwritesIterationState(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	h.SetStepState(0, StepDone)
	h.SetStepState(1, StepDone)
	h.SetStepState(4, StepActive)

	finalSteps := []string{"Deferred work", "Lessons learned", "Final git push"}
	h.SetFinalization(1, 3, finalSteps)

	if len(h.Row1) != 2 {
		t.Errorf("Row1 len = %d, want 2", len(h.Row1))
	}
	if len(h.Row2) != 1 {
		t.Errorf("Row2 len = %d, want 1", len(h.Row2))
	}
	if h.Row1[0] != "[ ] Deferred work" {
		t.Errorf("Row1[0] = %q, want %q", h.Row1[0], "[ ] Deferred work")
	}
	if h.Row1[1] != "[ ] Lessons learned" {
		t.Errorf("Row1[1] = %q, want %q", h.Row1[1], "[ ] Lessons learned")
	}
	if h.Row2[0] != "[ ] Final git push" {
		t.Errorf("Row2[0] = %q, want %q", h.Row2[0], "[ ] Final git push")
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

// Dynamic step count: 5 steps → rowSize=(5+1)/2=3 → Row1=[3], Row2=[2]
func TestStatusHeader_DynamicStepCount_Five(t *testing.T) {
	names := []string{"Step A", "Step B", "Step C", "Step D", "Step E"}
	h := NewStatusHeader(names)

	if len(h.Row1) != 3 {
		t.Errorf("Row1 len = %d, want 3", len(h.Row1))
	}
	if len(h.Row2) != 2 {
		t.Errorf("Row2 len = %d, want 2", len(h.Row2))
	}
	for i, name := range names[:3] {
		want := "[ ] " + name
		if h.Row1[i] != want {
			t.Errorf("Row1[%d] = %q, want %q", i, h.Row1[i], want)
		}
	}
	for i, name := range names[3:] {
		want := "[ ] " + name
		if h.Row2[i] != want {
			t.Errorf("Row2[%d] = %q, want %q", i, h.Row2[i], want)
		}
	}
}

// Dynamic step count: 1 step → rowSize=1 → Row1=[1], Row2=[]
func TestStatusHeader_DynamicStepCount_One(t *testing.T) {
	names := []string{"Only step"}
	h := NewStatusHeader(names)

	if len(h.Row1) != 1 {
		t.Errorf("Row1 len = %d, want 1", len(h.Row1))
	}
	if len(h.Row2) != 0 {
		t.Errorf("Row2 len = %d, want 0", len(h.Row2))
	}
	if h.Row1[0] != "[ ] Only step" {
		t.Errorf("Row1[0] = %q, want %q", h.Row1[0], "[ ] Only step")
	}
}

// Dynamic step count: 0 steps → Row1=[], Row2=[]; SetStepState(0,...) is no-op
func TestStatusHeader_DynamicStepCount_Zero(t *testing.T) {
	h := NewStatusHeader([]string{})

	if len(h.Row1) != 0 {
		t.Errorf("Row1 len = %d, want 0", len(h.Row1))
	}
	if len(h.Row2) != 0 {
		t.Errorf("Row2 len = %d, want 0", len(h.Row2))
	}
	// Must not panic
	h.SetStepState(0, StepDone)
}

// SetStepState boundary with 5 steps: last valid idx=4 → Row2[4-3=1]
func TestStatusHeader_SetStepState_DynamicBoundary(t *testing.T) {
	names := []string{"Step A", "Step B", "Step C", "Step D", "Step E"}
	h := NewStatusHeader(names) // rowSize=3, Row1=[A,B,C], Row2=[D,E]

	h.SetStepState(4, StepDone)
	if h.Row2[1] != "[✓] Step E" {
		t.Errorf("Row2[1] = %q, want %q", h.Row2[1], "[✓] Step E")
	}

	// idx=5 is out of bounds — must be a no-op
	row2Before := append([]string(nil), h.Row2...)
	h.SetStepState(5, StepDone)
	if !reflect.DeepEqual(h.Row2, row2Before) {
		t.Errorf("Row2 changed after out-of-bounds SetStepState(5,...): got %v, want %v", h.Row2, row2Before)
	}
}

// SetFinalization with 3 names → rowSize=2 → Row1=[2], Row2=[1]; exercise SetFinalizeStepState
func TestStatusHeader_SetFinalization_DynamicCount(t *testing.T) {
	h := NewStatusHeader(testStepNames)
	steps := []string{"Phase A", "Phase B", "Phase C"}
	h.SetFinalization(1, 3, steps)

	if len(h.Row1) != 2 {
		t.Errorf("Row1 len = %d, want 2", len(h.Row1))
	}
	if len(h.Row2) != 1 {
		t.Errorf("Row2 len = %d, want 1", len(h.Row2))
	}

	h.SetFinalizeStepState(0, StepDone)
	h.SetFinalizeStepState(1, StepActive)
	h.SetFinalizeStepState(2, StepDone)

	if h.Row1[0] != "[✓] Phase A" {
		t.Errorf("Row1[0] = %q, want %q", h.Row1[0], "[✓] Phase A")
	}
	if h.Row1[1] != "[▸] Phase B" {
		t.Errorf("Row1[1] = %q, want %q", h.Row1[1], "[▸] Phase B")
	}
	if h.Row2[0] != "[✓] Phase C" {
		t.Errorf("Row2[0] = %q, want %q", h.Row2[0], "[✓] Phase C")
	}
}

// Input slice isolation: mutating the original slice after NewStatusHeader must not affect the header
func TestStatusHeader_InputSliceIsolation(t *testing.T) {
	names := []string{"Step A", "Step B", "Step C", "Step D"}
	h := NewStatusHeader(names)

	names[0] = "MUTATED"

	if h.Row1[0] != "[ ] Step A" {
		t.Errorf("Row1[0] = %q after mutating input slice, want %q", h.Row1[0], "[ ] Step A")
	}
}
