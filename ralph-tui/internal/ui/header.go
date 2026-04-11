package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Status-header color scheme. These are package vars so main.go can bind
// fixed colors by value for static widgets (iteration line, HRules, footer)
// while the grid cells bind their per-cell color fields by pointer for
// dynamic repaints as step state changes.
var (
	// LightGray is the default foreground color for the header and footer
	// chrome: brackets, pending/done/failed/skipped markers, step names,
	// iteration line, shortcut bar, version label, and the box border.
	LightGray = lipgloss.Color("245")
	// ActiveStepFG is the foreground color for the currently running
	// step's brackets and name — white so the active row pops against
	// the light-gray chrome.
	ActiveStepFG = lipgloss.Color("15")
	// ActiveMarkerFG is the foreground color for the active step's
	// marker glyph (▸) so the triangle reads as "this one is running"
	// at a glance, independently of the rest of the cell text.
	ActiveMarkerFG = lipgloss.Color("10")
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
// Each checkbox cell is rendered as three adjacent Lip Gloss spans so the
// marker glyph can be colored independently of the brackets and step
// name.
//
// Rows is kept in sync as the legacy single-string representation
// ("[X] name") for existing test assertions.
type StatusHeader struct {
	IterationLine string // e.g. "Iteration 2/5 — Issue #42", "Initializing 1/2: Splash", "Finalizing 1/3: Deferred work"

	Rows [][HeaderCols]string // legacy single-string labels ("[X] name") — test assertions only

	// Split-cell fields: the checkbox grid is rendered from these.
	Prefixes     [][HeaderCols]string
	Markers      [][HeaderCols]string
	Suffixes     [][HeaderCols]string
	MarkerColors [][HeaderCols]lipgloss.Color
	NameColors   [][HeaderCols]lipgloss.Color

	stepNames []string // current phase's step name list
}

// NewStatusHeader constructs a header sized to fit the largest phase.
// Call this once at startup, after validation, with the max step count across
// all three phases (initialize, iteration, finalize).
func NewStatusHeader(maxStepsAcrossPhases int) *StatusHeader {
	rowCount := max((maxStepsAcrossPhases+HeaderCols-1)/HeaderCols, 1) // ceil division, min 1
	return &StatusHeader{
		Rows:         make([][HeaderCols]string, rowCount),
		Prefixes:     make([][HeaderCols]string, rowCount),
		Markers:      make([][HeaderCols]string, rowCount),
		Suffixes:     make([][HeaderCols]string, rowCount),
		MarkerColors: make([][HeaderCols]lipgloss.Color, rowCount),
		NameColors:   make([][HeaderCols]lipgloss.Color, rowCount),
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
	for r := range len(h.Rows) {
		for c := range HeaderCols {
			idx := r*HeaderCols + c
			if idx < len(names) {
				h.writeCell(r, c, StepPending, names[idx])
			} else {
				h.clearCell(r, c)
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
	h.writeCell(r, c, state, h.stepNames[idx])
}

// writeCell populates every parallel field for a single grid slot that
// has a step assigned to it. Kept private because callers should always
// route through SetPhaseSteps / SetStepState, which provide the row/col
// arithmetic and bounds guards.
func (h *StatusHeader) writeCell(r, c int, state StepState, name string) {
	marker, nameColor, markerColor := cellStyle(state)
	h.Prefixes[r][c] = "["
	h.Markers[r][c] = marker
	h.Suffixes[r][c] = "] " + name
	h.NameColors[r][c] = nameColor
	h.MarkerColors[r][c] = markerColor
	h.Rows[r][c] = "[" + marker + "] " + name
}

// clearCell blanks every parallel field for a trailing/unused slot. The
// color fields are reset to LightGray so any transient render before the
// next SetPhaseSteps picks up the chrome color rather than a stale
// active/green/white from the previous phase.
func (h *StatusHeader) clearCell(r, c int) {
	h.Prefixes[r][c] = ""
	h.Markers[r][c] = ""
	h.Suffixes[r][c] = ""
	h.NameColors[r][c] = LightGray
	h.MarkerColors[r][c] = LightGray
	h.Rows[r][c] = ""
}

// cellStyle returns the marker glyph and per-cell colors for a given step
// state. Active steps get white brackets/name with a green marker so the
// running row pops out of the light-gray chrome; every other state uses
// LightGray for both marker and name. Unknown states fall through to the
// pending default (a space marker with light-gray colors).
func cellStyle(state StepState) (marker string, nameColor, markerColor lipgloss.Color) {
	switch state {
	case StepActive:
		return "▸", ActiveStepFG, ActiveMarkerFG
	case StepDone:
		return "✓", LightGray, LightGray
	case StepFailed:
		return "✗", LightGray, LightGray
	case StepSkipped:
		return "-", LightGray, LightGray
	default:
		return " ", LightGray, LightGray
	}
}
