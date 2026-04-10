package ui

import (
	"strings"
	"testing"
)

// --- NewStatusHeader row-count computation ---

func TestNewStatusHeader_RowCount(t *testing.T) {
	cases := []struct {
		maxSteps int
		wantRows int
	}{
		{0, 1},  // minimum 1 row
		{1, 1},  // 1 step → 1 row
		{4, 1},  // exactly 4 → 1 row
		{5, 2},  // 5 → ceil(5/4) = 2 rows
		{8, 2},  // 8 → ceil(8/4) = 2 rows
		{9, 3},  // 9 → ceil(9/4) = 3 rows
		{20, 5}, // 20 → ceil(20/4) = 5 rows
	}
	for _, tc := range cases {
		h := NewStatusHeader(tc.maxSteps)
		if len(h.Rows) != tc.wantRows {
			t.Errorf("NewStatusHeader(%d): got %d rows, want %d", tc.maxSteps, len(h.Rows), tc.wantRows)
		}
	}
}

// --- SetPhaseSteps ---

func TestSetPhaseSteps_ShortPhase(t *testing.T) {
	h := NewStatusHeader(8)
	names := []string{"Step A", "Step B", "Step C"}
	h.SetPhaseSteps(names)

	if h.Rows[0][0] != "[ ] Step A" {
		t.Errorf("Rows[0][0] = %q, want %q", h.Rows[0][0], "[ ] Step A")
	}
	if h.Rows[0][1] != "[ ] Step B" {
		t.Errorf("Rows[0][1] = %q, want %q", h.Rows[0][1], "[ ] Step B")
	}
	if h.Rows[0][2] != "[ ] Step C" {
		t.Errorf("Rows[0][2] = %q, want %q", h.Rows[0][2], "[ ] Step C")
	}
	// trailing slot in the same row should be blank
	if h.Rows[0][3] != "" {
		t.Errorf("Rows[0][3] should be empty, got %q", h.Rows[0][3])
	}
	// second row should be entirely blank
	for c, v := range h.Rows[1] {
		if v != "" {
			t.Errorf("Rows[1][%d] should be empty, got %q", c, v)
		}
	}
}

func TestSetPhaseSteps_LongPhase(t *testing.T) {
	h := NewStatusHeader(8)
	names := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	h.SetPhaseSteps(names)

	for r := range 2 {
		for c := range HeaderCols {
			idx := r*HeaderCols + c
			want := "[ ] " + names[idx]
			if h.Rows[r][c] != want {
				t.Errorf("Rows[%d][%d] = %q, want %q", r, c, h.Rows[r][c], want)
			}
		}
	}
}

func TestSetPhaseSteps_PhaseTransitionClearsTrailingSlots(t *testing.T) {
	// Start with a longer phase, then switch to a shorter one.
	h := NewStatusHeader(8)
	longNames := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	h.SetPhaseSteps(longNames)

	shortNames := []string{"X", "Y", "Z"}
	h.SetPhaseSteps(shortNames)

	if h.Rows[0][0] != "[ ] X" {
		t.Errorf("Rows[0][0] = %q, want %q", h.Rows[0][0], "[ ] X")
	}
	if h.Rows[0][1] != "[ ] Y" {
		t.Errorf("Rows[0][1] = %q, want %q", h.Rows[0][1], "[ ] Y")
	}
	if h.Rows[0][2] != "[ ] Z" {
		t.Errorf("Rows[0][2] = %q, want %q", h.Rows[0][2], "[ ] Z")
	}
	// trailing slot in row 0 and all of row 1 must be blank (no stale names)
	if h.Rows[0][3] != "" {
		t.Errorf("Rows[0][3] should be empty after phase transition, got %q", h.Rows[0][3])
	}
	for c, v := range h.Rows[1] {
		if v != "" {
			t.Errorf("Rows[1][%d] should be empty after phase transition, got %q", c, v)
		}
	}
}

func TestSetPhaseSteps_OverflowPanics(t *testing.T) {
	h := NewStatusHeader(4) // 1 row = 4 slots
	names := make([]string, 5)
	for i := range names {
		names[i] = "step"
	}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for overflow SetPhaseSteps, got none")
		}
	}()
	h.SetPhaseSteps(names)
}

// --- SetStepState ---

func TestSetStepState_UpdatesRightRowCol(t *testing.T) {
	h := NewStatusHeader(8)
	names := []string{"Feature work", "Test planning", "Test writing", "Code review",
		"Review fixes", "Close issue", "Update docs", "Git push"}
	h.SetPhaseSteps(names)

	h.SetStepState(0, StepDone)
	h.SetStepState(1, StepDone)
	h.SetStepState(2, StepDone)
	h.SetStepState(3, StepActive)
	// steps 4-7 remain pending from SetPhaseSteps

	cases := []struct {
		row, col int
		want     string
	}{
		{0, 0, "[✓] Feature work"},
		{0, 1, "[✓] Test planning"},
		{0, 2, "[✓] Test writing"},
		{0, 3, "[▸] Code review"},
		{1, 0, "[ ] Review fixes"},
		{1, 1, "[ ] Close issue"},
		{1, 2, "[ ] Update docs"},
		{1, 3, "[ ] Git push"},
	}
	for _, c := range cases {
		got := h.Rows[c.row][c.col]
		if got != c.want {
			t.Errorf("Rows[%d][%d] = %q, want %q", c.row, c.col, got, c.want)
		}
	}
}

func TestSetStepState_FailedStep(t *testing.T) {
	h := NewStatusHeader(4)
	h.SetPhaseSteps([]string{"Alpha", "Beta", "Gamma"})
	h.SetStepState(1, StepFailed)

	if h.Rows[0][1] != "[✗] Beta" {
		t.Errorf("Rows[0][1] = %q, want %q", h.Rows[0][1], "[✗] Beta")
	}
}

func TestSetStepState_SkippedStep(t *testing.T) {
	h := NewStatusHeader(4)
	h.SetPhaseSteps([]string{"Alpha", "Beta", "Gamma"})
	h.SetStepState(2, StepSkipped)

	if h.Rows[0][2] != "[-] Gamma" {
		t.Errorf("Rows[0][2] = %q, want %q", h.Rows[0][2], "[-] Gamma")
	}
}

func TestSetStepState_OutOfBoundsIsNoOp(t *testing.T) {
	h := NewStatusHeader(4)
	h.SetPhaseSteps([]string{"A", "B", "C"})

	rowsBefore := h.Rows[0]
	h.SetStepState(-1, StepDone)
	h.SetStepState(3, StepDone) // idx 3 is beyond len(stepNames)==3
	h.SetStepState(99, StepDone)

	if h.Rows[0] != rowsBefore {
		t.Errorf("Rows[0] changed after out-of-bounds SetStepState: got %v, want %v", h.Rows[0], rowsBefore)
	}
}

// --- RenderInitializeLine ---

func TestRenderInitializeLine(t *testing.T) {
	cases := []struct {
		stepNum, stepCount int
		stepName           string
		want               string
	}{
		{1, 2, "Splash", "Initializing 1/2: Splash"},
		{2, 2, "Setup", "Initializing 2/2: Setup"},
		{1, 1, "Long step name with spaces", "Initializing 1/1: Long step name with spaces"},
	}
	for _, tc := range cases {
		h := NewStatusHeader(4)
		h.RenderInitializeLine(tc.stepNum, tc.stepCount, tc.stepName)
		if h.IterationLine != tc.want {
			t.Errorf("RenderInitializeLine(%d, %d, %q): got %q, want %q",
				tc.stepNum, tc.stepCount, tc.stepName, h.IterationLine, tc.want)
		}
	}
}

func TestRenderInitializeLine_WritesToIterationLine(t *testing.T) {
	h := NewStatusHeader(4)
	h.RenderInitializeLine(1, 3, "Bootstrap")
	if h.IterationLine == "" {
		t.Error("RenderInitializeLine did not write to IterationLine")
	}
}

// --- RenderIterationLine ---

func TestRenderIterationLine_Bounded(t *testing.T) {
	h := NewStatusHeader(8)
	h.RenderIterationLine(2, 5, "42")

	want := "Iteration 2/5 — Issue #42"
	if h.IterationLine != want {
		t.Errorf("got %q, want %q", h.IterationLine, want)
	}
}

func TestRenderIterationLine_Unbounded(t *testing.T) {
	h := NewStatusHeader(8)
	h.RenderIterationLine(3, 0, "42")

	want := "Iteration 3 — Issue #42"
	if h.IterationLine != want {
		t.Errorf("got %q, want %q", h.IterationLine, want)
	}
}

func TestRenderIterationLine_EmptyIssueID(t *testing.T) {
	h := NewStatusHeader(8)
	h.RenderIterationLine(1, 5, "")

	want := "Iteration 1/5"
	if h.IterationLine != want {
		t.Errorf("got %q, want %q", h.IterationLine, want)
	}
}

func TestRenderIterationLine_UnboundedEmptyIssueID(t *testing.T) {
	h := NewStatusHeader(8)
	h.RenderIterationLine(4, 0, "")

	want := "Iteration 4"
	if h.IterationLine != want {
		t.Errorf("got %q, want %q", h.IterationLine, want)
	}
}

func TestRenderIterationLine_WritesToIterationLine(t *testing.T) {
	h := NewStatusHeader(8)
	h.RenderIterationLine(1, 3, "99")
	if h.IterationLine == "" {
		t.Error("RenderIterationLine did not write to IterationLine")
	}
}

// --- RenderFinalizeLine ---

func TestRenderFinalizeLine(t *testing.T) {
	cases := []struct {
		stepNum, stepCount int
		stepName           string
		want               string
	}{
		{1, 3, "Deferred work", "Finalizing 1/3: Deferred work"},
		{3, 3, "Final push", "Finalizing 3/3: Final push"},
		{2, 5, "Long finalize step name", "Finalizing 2/5: Long finalize step name"},
	}
	for _, tc := range cases {
		h := NewStatusHeader(4)
		h.RenderFinalizeLine(tc.stepNum, tc.stepCount, tc.stepName)
		if h.IterationLine != tc.want {
			t.Errorf("RenderFinalizeLine(%d, %d, %q): got %q, want %q",
				tc.stepNum, tc.stepCount, tc.stepName, h.IterationLine, tc.want)
		}
	}
}

func TestRenderFinalizeLine_WritesToIterationLine(t *testing.T) {
	h := NewStatusHeader(4)
	h.RenderFinalizeLine(1, 2, "Cleanup")
	if h.IterationLine == "" {
		t.Error("RenderFinalizeLine did not write to IterationLine")
	}
}

// --- Input immutability ---

func TestSetPhaseSteps_DoesNotMutateInput(t *testing.T) {
	h := NewStatusHeader(4)
	names := []string{"Alpha", "Beta", "Gamma"}
	h.SetPhaseSteps(names)

	// Mutate the caller's slice after the call.
	names[0] = "MUTATED"

	// The header's internal state should still reflect the original names.
	if h.Rows[0][0] != "[ ] Alpha" {
		t.Errorf("Rows[0][0] = %q after mutating input, want %q", h.Rows[0][0], "[ ] Alpha")
	}
}

// --- Phase transition interactions ---

func TestSetStepState_AfterPhaseTransition(t *testing.T) {
	// Start with a longer phase (8 steps), then switch to a shorter one (3 steps).
	// An index valid in the old phase but out-of-bounds in the new phase must be a no-op.
	h := NewStatusHeader(8)
	h.SetPhaseSteps([]string{"A", "B", "C", "D", "E", "F", "G", "H"})
	h.SetPhaseSteps([]string{"X", "Y", "Z"})

	rowsBefore := h.Rows[0]
	h.SetStepState(5, StepDone) // index 5 was valid in the old phase, invalid now

	if h.Rows[0] != rowsBefore {
		t.Errorf("Rows[0] changed after out-of-bounds SetStepState (post phase transition): got %v, want %v", h.Rows[0], rowsBefore)
	}
}

// --- SetPhaseSteps edge cases ---

func TestSetPhaseSteps_ExactlyOneFullRow(t *testing.T) {
	h := NewStatusHeader(4)
	names := []string{"Step1", "Step2", "Step3", "Step4"}
	h.SetPhaseSteps(names)

	for c, name := range names {
		want := "[ ] " + name
		if h.Rows[0][c] != want {
			t.Errorf("Rows[0][%d] = %q, want %q", c, h.Rows[0][c], want)
		}
	}
}

func TestSetPhaseSteps_ZeroSteps(t *testing.T) {
	h := NewStatusHeader(4)
	h.SetPhaseSteps([]string{"Prev1", "Prev2"}) // load a previous phase
	h.SetPhaseSteps([]string{})                 // now clear with zero steps

	for c, v := range h.Rows[0] {
		if v != "" {
			t.Errorf("Rows[0][%d] = %q, want empty after zero-step SetPhaseSteps", c, v)
		}
	}
}

// --- NewStatusHeader edge cases ---

func TestNewStatusHeader_NegativeInput(t *testing.T) {
	h := NewStatusHeader(-1)
	if len(h.Rows) != 1 {
		t.Errorf("NewStatusHeader(-1): got %d rows, want 1", len(h.Rows))
	}
}

// T1: SetStepState before SetPhaseSteps is called is a no-op (stepNames is nil).
func TestSetStepState_BeforeSetPhaseSteps_IsNoOp(t *testing.T) {
	h := NewStatusHeader(4)
	// Do NOT call SetPhaseSteps — stepNames is nil.
	rowsBefore := h.Rows[0]
	h.SetStepState(0, StepSkipped)
	if h.Rows[0] != rowsBefore {
		t.Errorf("Rows[0] changed without SetPhaseSteps: got %v, want %v", h.Rows[0], rowsBefore)
	}
}

// T2: StepSkipped at a second-row grid position uses correct row/col arithmetic.
func TestSetStepState_SkippedStep_SecondRow(t *testing.T) {
	h := NewStatusHeader(8)
	names := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	h.SetPhaseSteps(names)
	// index 5 → row 1, col 1
	h.SetStepState(5, StepSkipped)
	if h.Rows[1][1] != "[-] F" {
		t.Errorf("Rows[1][1] = %q, want %q", h.Rows[1][1], "[-] F")
	}
}

// --- RenderCompletionLine ---

// T1: RenderCompletionLine formats the summary string correctly.
func TestRenderCompletionLine(t *testing.T) {
	cases := []struct {
		iterationsRun, finalizeCount int
		want                         string
	}{
		{3, 2, "Ralph completed after 3 iteration(s) and 2 finalizing tasks."},
		{1, 0, "Ralph completed after 1 iteration(s) and 0 finalizing tasks."},
	}
	for _, tc := range cases {
		h := NewStatusHeader(4)
		h.RenderCompletionLine(tc.iterationsRun, tc.finalizeCount)
		if h.IterationLine != tc.want {
			t.Errorf("RenderCompletionLine(%d, %d): got %q, want %q",
				tc.iterationsRun, tc.finalizeCount, h.IterationLine, tc.want)
		}
	}
}

// T4: RenderCompletionLine overwrites a previous IterationLine value.
func TestRenderCompletionLine_OverwritesPreviousIterationLine(t *testing.T) {
	h := NewStatusHeader(8)
	h.RenderIterationLine(1, 5, "42")
	h.RenderCompletionLine(1, 3)

	if !strings.Contains(h.IterationLine, "Ralph completed") {
		t.Errorf("expected IterationLine to contain %q, got %q", "Ralph completed", h.IterationLine)
	}
	if strings.Contains(h.IterationLine, "Iteration") {
		t.Errorf("expected IterationLine to not contain %q after RenderCompletionLine, got %q", "Iteration", h.IterationLine)
	}
}

// --- substitute helper ---

// T3a: substitute with an empty vals map returns the template string unchanged.
func TestSubstitute_EmptyValsReturnsTemplateUnchanged(t *testing.T) {
	tmpl := "Initializing {{STEP_NUM}}/{{STEP_COUNT}}: {{STEP_NAME}}"
	got := substitute(tmpl, map[string]string{})
	if got != tmpl {
		t.Errorf("substitute with empty vals: got %q, want %q", got, tmpl)
	}
}

// T3b: substitute leaves unknown {{KEY}} tokens in the template as-is.
func TestSubstitute_UnknownKeyLeftAsIs(t *testing.T) {
	tmpl := "Hello {{KNOWN}} and {{UNKNOWN}}"
	got := substitute(tmpl, map[string]string{"KNOWN": "world"})
	want := "Hello world and {{UNKNOWN}}"
	if got != want {
		t.Errorf("substitute with unknown key: got %q, want %q", got, want)
	}
}

// T3: An unrecognized StepState value falls through to the default (pending) display.
func TestSetStepState_UnknownStateFallsToDefault(t *testing.T) {
	h := NewStatusHeader(4)
	h.SetPhaseSteps([]string{"Alpha", "Beta", "Gamma"})
	h.SetStepState(0, StepState(99))
	if h.Rows[0][0] != "[ ] Alpha" {
		t.Errorf("Rows[0][0] = %q, want %q", h.Rows[0][0], "[ ] Alpha")
	}
}
