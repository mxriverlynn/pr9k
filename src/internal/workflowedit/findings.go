package workflowedit

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// findingResult is a single validation finding in a form workflowedit can hold
// without importing internal/validator directly (D-4). makeValidateCmd converts
// []validator.Error → []findingResult before dispatching validateCompleteMsg.
type findingResult struct {
	text     string
	isFatal  bool
	stepName string // step name for field-jump on Enter; "" if finding is file-level
}

// findingEntry is an element in the rendered findings panel.
type findingEntry struct {
	key     string // stable key for acknowledgment tracking (== text)
	text    string // display line
	isFatal bool
	stepIdx int // index into doc.Steps (-1 when finding has no step reference)
}

// findingsPanel is a scrollable list of validation findings with per-entry
// acknowledgment tracking. Its viewport state survives rebuilds.
type findingsPanel struct {
	entries []findingEntry
	ackSet  map[string]bool // keyed by findingEntry.key
	vp      viewport.Model
}

// buildFindingsPanel constructs a findings panel from items, looking up each
// item's stepName in steps to produce a step index for field-jump navigation.
// Acknowledgment state and the viewport scroll position from prev are preserved.
func buildFindingsPanel(items []findingResult, steps []workflowmodel.Step, prev findingsPanel) findingsPanel {
	entries := make([]findingEntry, len(items))
	for i, item := range items {
		stepIdx := -1
		if item.stepName != "" {
			for j, s := range steps {
				if s.Name == item.stepName {
					stepIdx = j
					break
				}
			}
		}
		entries[i] = findingEntry{
			key:     item.text,
			text:    item.text,
			isFatal: item.isFatal,
			stepIdx: stepIdx,
		}
	}

	// Carry over acknowledgment for entries that still appear in the new panel.
	newAckSet := make(map[string]bool, len(entries))
	if prev.ackSet != nil {
		for _, e := range entries {
			if prev.ackSet[e.key] {
				newAckSet[e.key] = true
			}
		}
	}

	fp := findingsPanel{
		entries: entries,
		ackSet:  newAckSet,
		vp:      prev.vp, // preserve scroll position
	}
	fp.syncViewport()
	return fp
}

// syncViewport sets the viewport content to the current entries.
func (fp *findingsPanel) syncViewport() {
	lines := make([]string, len(fp.entries))
	for i, e := range fp.entries {
		lines[i] = e.text
	}
	fp.vp.SetContent(strings.Join(lines, "\n"))
}

// firstStepIdx returns the step index of the first entry that has a reference,
// or -1 if none do. Used by the Enter key handler to jump to a referenced step.
func (fp findingsPanel) firstStepIdx() int {
	for _, e := range fp.entries {
		if e.stepIdx >= 0 {
			return e.stepIdx
		}
	}
	return -1
}
