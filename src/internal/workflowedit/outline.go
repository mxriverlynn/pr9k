package workflowedit

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// sectionKey identifies a named section in the outline tree.
type sectionKey int

const (
	sectionEnv          sectionKey = iota
	sectionContainerEnv sectionKey = iota
	sectionStatusLine   sectionKey = iota
	sectionInitialize   sectionKey = iota
	sectionIteration    sectionKey = iota
	sectionFinalize     sectionKey = iota
)

// outlineRowKind classifies each row in the flat outline list.
type outlineRowKind int

const (
	rowKindSectionHeader outlineRowKind = iota
	rowKindStep
	rowKindEnvItem
	rowKindContainerEnvItem
	rowKindAddRow
)

// outlineRow is one entry in the flattened outline list.
type outlineRow struct {
	kind    outlineRowKind
	section sectionKey
	stepIdx int    // valid when kind == rowKindStep
	label   string // display label (env key, containerEnv key, etc.)
}

// outlinePanel is the left-hand structured outline pane.
type outlinePanel struct {
	vp        viewport.Model
	cursor    int
	width     int
	height    int
	scrolls   int // incremented per scroll event; aids test assertions
	collapsed map[sectionKey]bool
}

func newOutlinePanel(width, height int) outlinePanel {
	return outlinePanel{
		vp:        viewport.New(width, height),
		width:     width,
		height:    height,
		collapsed: make(map[sectionKey]bool),
	}
}

// buildOutlineRows returns the complete flat row list for the given doc and
// collapse state. Phase sections are always shown; top-level env/containerEnv/
// statusLine sections appear only when they have content.
func buildOutlineRows(doc workflowmodel.WorkflowDoc, collapsed map[sectionKey]bool) []outlineRow {
	var rows []outlineRow

	// top-level env section (shown only when non-empty)
	if len(doc.Env) > 0 {
		rows = append(rows, outlineRow{kind: rowKindSectionHeader, section: sectionEnv})
		if !collapsed[sectionEnv] {
			for _, k := range doc.Env {
				rows = append(rows, outlineRow{kind: rowKindEnvItem, section: sectionEnv, label: k})
			}
			rows = append(rows, outlineRow{kind: rowKindAddRow, section: sectionEnv})
		}
	}

	// top-level containerEnv section (shown only when non-empty)
	if len(doc.ContainerEnv) > 0 {
		rows = append(rows, outlineRow{kind: rowKindSectionHeader, section: sectionContainerEnv})
		if !collapsed[sectionContainerEnv] {
			// Stable iteration order: collect keys and sort.
			keys := sortedKeys(doc.ContainerEnv)
			for _, k := range keys {
				label := k + " = " + doc.ContainerEnv[k]
				rows = append(rows, outlineRow{kind: rowKindContainerEnvItem, section: sectionContainerEnv, label: label})
			}
			rows = append(rows, outlineRow{kind: rowKindAddRow, section: sectionContainerEnv})
		}
	}

	// top-level statusLine section (shown only when present)
	if doc.StatusLine != nil {
		rows = append(rows, outlineRow{kind: rowKindSectionHeader, section: sectionStatusLine})
	}

	// Phase sections — always shown with their steps.
	rows = appendPhaseRows(rows, doc, sectionInitialize, workflowmodel.StepPhaseInitialize, collapsed)
	rows = appendPhaseRows(rows, doc, sectionIteration, workflowmodel.StepPhaseIteration, collapsed)
	rows = appendPhaseRows(rows, doc, sectionFinalize, workflowmodel.StepPhaseFinalize, collapsed)

	return rows
}

// appendPhaseRows adds a phase section header, its step rows, and its + Add row
// to rows (respecting the collapsed state).
func appendPhaseRows(rows []outlineRow, doc workflowmodel.WorkflowDoc, sk sectionKey, phase workflowmodel.StepPhase, collapsed map[sectionKey]bool) []outlineRow {
	rows = append(rows, outlineRow{kind: rowKindSectionHeader, section: sk})
	if !collapsed[sk] {
		for i, s := range doc.Steps {
			if s.Phase == phase {
				rows = append(rows, outlineRow{kind: rowKindStep, section: sk, stepIdx: i})
			}
		}
		rows = append(rows, outlineRow{kind: rowKindAddRow, section: sk})
	}
	return rows
}

// cursorStepIdx returns the Steps index for the row at cursor, or -1.
func cursorStepIdx(rows []outlineRow, cursor int) int {
	if cursor < 0 || cursor >= len(rows) {
		return -1
	}
	if rows[cursor].kind == rowKindStep {
		return rows[cursor].stepIdx
	}
	return -1
}

// ShortcutLine returns the shortcut hints appropriate for the current outline
// state (D-11). It needs the doc to determine the current row kind.
func (o outlinePanel) ShortcutLine(reorderMode bool, doc workflowmodel.WorkflowDoc) string {
	if reorderMode {
		return "↑/↓  move  ·  Enter  commit  ·  Esc  cancel"
	}
	rows := buildOutlineRows(doc, o.collapsed)
	if o.cursor >= 0 && o.cursor < len(rows) {
		switch rows[o.cursor].kind {
		case rowKindSectionHeader:
			return "↑/↓  navigate  ·  Space  collapse  ·  a  add"
		case rowKindAddRow:
			return "↑/↓  navigate  ·  Enter  add"
		}
	}
	return "↑/↓  navigate  ·  Tab  detail  ·  Del  delete  ·  r  reorder  ·  Alt+↑/↓  move"
}

// render dispatches to renderBordered when the pane has been sized (D18–D25),
// or falls back to the flat text render for unsized models.
func (o outlinePanel) render(doc workflowmodel.WorkflowDoc, cursor int, reorderMode bool) string {
	if o.width > 0 {
		return o.renderBordered(doc, cursor, reorderMode)
	}
	return o.renderFlat(doc, cursor, reorderMode)
}

// sectionLabel returns the display label for a section header including item count.
func sectionLabel(sk sectionKey, doc workflowmodel.WorkflowDoc) string {
	switch sk {
	case sectionEnv:
		return fmt.Sprintf("env (%d)", len(doc.Env))
	case sectionContainerEnv:
		return fmt.Sprintf("containerEnv (%d)", len(doc.ContainerEnv))
	case sectionStatusLine:
		return "statusLine"
	case sectionInitialize:
		return fmt.Sprintf("initialize (%d)", countPhaseSteps(doc, workflowmodel.StepPhaseInitialize))
	case sectionIteration:
		return fmt.Sprintf("iteration (%d)", countPhaseSteps(doc, workflowmodel.StepPhaseIteration))
	case sectionFinalize:
		return fmt.Sprintf("finalize (%d)", countPhaseSteps(doc, workflowmodel.StepPhaseFinalize))
	}
	return ""
}

// addRowLabel returns the label for the + Add affordance row of a section.
func addRowLabel(sk sectionKey) string {
	switch sk {
	case sectionEnv:
		return "Add env var"
	case sectionContainerEnv:
		return "Add container env"
	case sectionInitialize, sectionIteration, sectionFinalize:
		return "Add step"
	}
	return "Add"
}

// countPhaseSteps counts steps in doc that match the given phase.
func countPhaseSteps(doc workflowmodel.WorkflowDoc, phase workflowmodel.StepPhase) int {
	n := 0
	for _, s := range doc.Steps {
		if s.Phase == phase {
			n++
		}
	}
	return n
}

// sortedKeys returns map keys in sorted order for stable rendering.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
