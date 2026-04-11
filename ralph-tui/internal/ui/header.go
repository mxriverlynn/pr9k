package ui

import (
	"fmt"
	"strconv"
	"strings"
)

// StepState represents the display state of a single workflow step.
type StepState int

const (
	StepPending StepState = iota
	StepActive
	StepDone
	StepFailed
	StepSkipped // displayed as "[-] <name>"
)

// HeaderCols is the number of checkbox columns per row; constant to fit 80-column terminals.
const HeaderCols = 4

// StatusHeader manages the pointer-mutable string state for the TUI status header.
// Glyph reads the exported fields via pointer on each render cycle — callers
// update state by mutating struct fields directly (e.g. RenderIterationLine, SetStepState).
//
// Layout:
//
//	IterationLine    →  Text(&h.IterationLine)
//	Rows[r][0..3]   →  HBox(Text(&h.Rows[r][0]), ..., Text(&h.Rows[r][3]))  // one row per HeaderCols steps
type StatusHeader struct {
	IterationLine string               // e.g. "Iteration 2/5 — Issue #42", "Initializing 1/2: Splash", "Finalizing 1/3: Deferred work"
	Rows          [][HeaderCols]string // row count computed at startup; each row has HeaderCols slots
	stepNames     []string             // current phase's step name list
}

// NewStatusHeader constructs a header sized to fit the largest phase.
// Call this once at startup, after validation, with the max step count across
// all three phases (initialize, iteration, finalize).
func NewStatusHeader(maxStepsAcrossPhases int) *StatusHeader {
	rowCount := (maxStepsAcrossPhases + HeaderCols - 1) / HeaderCols // ceil division
	if rowCount < 1 {
		rowCount = 1
	}
	return &StatusHeader{
		Rows: make([][HeaderCols]string, rowCount),
	}
}

const initializeHeaderFormat = "Initializing {{STEP_NUM}}/{{STEP_COUNT}}: {{STEP_NAME}}"
const finalizeHeaderFormat = "Finalizing {{STEP_NUM}}/{{STEP_COUNT}}: {{STEP_NAME}}"

// RenderInitializeLine updates the iteration line for the initialize phase.
// Example output: "Initializing 1/2: Splash".
func (h *StatusHeader) RenderInitializeLine(stepNum, stepCount int, stepName string) {
	h.IterationLine = substitute(initializeHeaderFormat, map[string]string{
		"STEP_NUM":   strconv.Itoa(stepNum),
		"STEP_COUNT": strconv.Itoa(stepCount),
		"STEP_NAME":  stepName,
	})
}

// RenderIterationLine updates the iteration line for the iteration phase.
// When maxIter == 0, the total is omitted (unbounded mode).
// When issueID is empty, the issue suffix is omitted.
// Example outputs: "Iteration 2/5 — Issue #42", "Iteration 3".
func (h *StatusHeader) RenderIterationLine(iter, maxIter int, issueID string) {
	var b strings.Builder
	if maxIter > 0 {
		fmt.Fprintf(&b, "Iteration %d/%d", iter, maxIter)
	} else {
		fmt.Fprintf(&b, "Iteration %d", iter)
	}
	if issueID != "" {
		fmt.Fprintf(&b, " — Issue #%s", issueID)
	}
	h.IterationLine = b.String()
}

// RenderFinalizeLine updates the iteration line for the finalize phase.
// Example output: "Finalizing 1/3: Deferred work".
func (h *StatusHeader) RenderFinalizeLine(stepNum, stepCount int, stepName string) {
	h.IterationLine = substitute(finalizeHeaderFormat, map[string]string{
		"STEP_NUM":   strconv.Itoa(stepNum),
		"STEP_COUNT": strconv.Itoa(stepCount),
		"STEP_NAME":  stepName,
	})
}

// substitute replaces all {{KEY}} tokens in template with the corresponding
// value from vals. Keys not present in vals are left as-is.
func substitute(template string, vals map[string]string) string {
	pairs := make([]string, 0, len(vals)*2)
	for k, v := range vals {
		pairs = append(pairs, "{{"+k+"}}", v)
	}
	return strings.NewReplacer(pairs...).Replace(template)
}

// SetPhaseSteps replaces the current step name list and re-renders all
// checkbox slots. Call at the start of each phase (initialize, iteration,
// finalize) to swap the header to the new phase's step set.
//
// Panics if len(names) exceeds the allocated grid capacity — this is a bug
// indicator, not a user-reachable path (NewStatusHeader is sized to the
// largest phase, so overflow means the caller passed the wrong max).
func (h *StatusHeader) SetPhaseSteps(names []string) {
	totalSlots := len(h.Rows) * HeaderCols
	if len(names) > totalSlots {
		panic(fmt.Sprintf("ui: phase has %d steps, exceeds allocated grid capacity %d", len(names), totalSlots))
	}
	h.stepNames = append(h.stepNames[:0], names...)
	for r := 0; r < len(h.Rows); r++ {
		for c := 0; c < HeaderCols; c++ {
			idx := r*HeaderCols + c
			if idx < len(names) {
				h.Rows[r][c] = checkboxLabel(StepPending, names[idx])
			} else {
				h.Rows[r][c] = "" // trailing empty slots render as blank padding
			}
		}
	}
}

// SetStepState updates the checkbox label for step idx in the current phase.
// Out-of-range idx is a no-op.
func (h *StatusHeader) SetStepState(idx int, state StepState) {
	if idx < 0 || idx >= len(h.stepNames) {
		return
	}
	r, c := idx/HeaderCols, idx%HeaderCols
	h.Rows[r][c] = checkboxLabel(state, h.stepNames[idx])
}

func checkboxLabel(state StepState, name string) string {
	switch state {
	case StepActive:
		return fmt.Sprintf("[▸] %s", name)
	case StepDone:
		return fmt.Sprintf("[✓] %s", name)
	case StepFailed:
		return fmt.Sprintf("[✗] %s", name)
	case StepSkipped:
		return fmt.Sprintf("[-] %s", name)
	default:
		return fmt.Sprintf("[ ] %s", name)
	}
}
