package ui

import (
	"fmt"
	"testing"
)

// makeLines returns a slice of n distinct lines of the form "line-0", "line-1", ...
func makeLines(n int) []string {
	lines := make([]string, n)
	for i := range n {
		lines[i] = fmt.Sprintf("line-%d", i)
	}
	return lines
}

// setCommittedSelection sets a committed selection on m covering rawIdx
// anchor..cursor (same rawOffset=0). Uses a direct field assignment since there
// is no public constructor for committed selections.
//
// Note: this helper does not accept *testing.T and therefore cannot call
// t.Helper(). If validation (e.g. bounds checks) is added in the future,
// thread *testing.T through the signature so error lines point at the
// call site rather than at this helper.
func setCommittedSelection(m *logModel, anchorRawIdx, cursorRawIdx int) {
	p := func(rawIdx int) pos {
		return pos{rawIdx: rawIdx, rawOffset: 0}
	}
	m.sel = selection{
		anchor:    p(anchorRawIdx),
		cursor:    p(cursorRawIdx),
		committed: true,
	}
}

// --- TP-106-01: ring eviction decrements selection rawIdx ---

// TestLogModel_RingEviction_DecrementsSelectionRawIdx verifies that when new
// lines cause ring-buffer eviction, both selection.anchor.rawIdx and
// selection.cursor.rawIdx are decremented by the eviction count, and
// SelectedText() still returns the original text.
func TestLogModel_RingEviction_DecrementsSelectionRawIdx(t *testing.T) {
	m := newLogModel(80, 20)

	// Fill the buffer to near capacity.
	fill := logRingBufferCap - 10
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(fill)})

	// Set a committed selection covering the last two raw lines currently in
	// the buffer. After the buffer is trimmed, rawIdx should decrement.
	anchorRawIdx := fill - 2
	cursorRawIdx := fill - 1
	setCommittedSelection(&m, anchorRawIdx, cursorRawIdx)

	// Remember the text the selection covers before eviction.
	wantText := m.SelectedText()
	if wantText == "" {
		t.Fatal("precondition: SelectedText must be non-empty before eviction")
	}

	// Push k lines so that fill + k > cap, triggering eviction.
	// fill = cap - 10, so k = 15 gives fill + k = cap + 5 → evicted = 5.
	k := 15
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(k)})

	evicted := fill + k - logRingBufferCap
	if evicted <= 0 {
		t.Fatalf("precondition: expected eviction, got evicted=%d (fill=%d k=%d cap=%d)",
			evicted, fill, k, logRingBufferCap)
	}

	wantAnchor := anchorRawIdx - evicted
	wantCursor := cursorRawIdx - evicted

	if m.sel.anchor.rawIdx != wantAnchor {
		t.Errorf("anchor.rawIdx: want %d, got %d", wantAnchor, m.sel.anchor.rawIdx)
	}
	if m.sel.cursor.rawIdx != wantCursor {
		t.Errorf("cursor.rawIdx: want %d, got %d", wantCursor, m.sel.cursor.rawIdx)
	}

	// The selected text should be unchanged (same raw content, just at a new index).
	gotText := m.SelectedText()
	if gotText != wantText {
		t.Errorf("SelectedText after eviction: want %q, got %q", wantText, gotText)
	}
}

// --- TP-106-02: ring eviction clears selection when anchor rawIdx underflows ---

// TestLogModel_RingEviction_ClearsSelectionWhenRawIdxUnderflow verifies that
// when eviction removes the line the selection is anchored to, the entire
// selection is cleared (zeroed out) rather than pointing at wrong content.
func TestLogModel_RingEviction_ClearsSelectionWhenRawIdxUnderflow(t *testing.T) {
	m := newLogModel(80, 20)

	// Fill the buffer to capacity.
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(logRingBufferCap)})

	// Anchor the selection at rawIdx 0 — the oldest line in the buffer.
	setCommittedSelection(&m, 0, 1)

	if m.sel == (selection{}) {
		t.Fatal("precondition: selection must be set")
	}

	// Adding any new line evicts rawIdx 0 and causes underflow.
	m, _ = m.Update(LogLinesMsg{Lines: []string{"new line"}})

	if m.sel != (selection{}) {
		t.Errorf("expected selection cleared after anchor eviction, got anchor.rawIdx=%d cursor.rawIdx=%d",
			m.sel.anchor.rawIdx, m.sel.cursor.rawIdx)
	}
}

// --- TP-106-03: ring eviction decrements cursor too, not just anchor ---

// TestLogModel_RingEviction_DecrementsCursorTooNotJustAnchor verifies that
// both anchor and cursor are decremented, and that if only the cursor underflows
// (anchor survives) the whole selection is still cleared — no half-valid ranges.
func TestLogModel_RingEviction_DecrementsCursorTooNotJustAnchor(t *testing.T) {
	m := newLogModel(80, 20)

	// Fill to capacity.
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(logRingBufferCap)})

	// Anchor survives (rawIdx 5), but cursor is at rawIdx 2 — will underflow
	// when 3 lines are evicted.
	setCommittedSelection(&m, 2, 5)

	// Add 3 lines → evict 3 lines. cursor.rawIdx (2 - 3 = -1) underflows.
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(3)})

	if m.sel != (selection{}) {
		t.Errorf("expected selection cleared when cursor underflows, got anchor=%d cursor=%d",
			m.sel.anchor.rawIdx, m.sel.cursor.rawIdx)
	}
}

// --- TP-106-04: auto-scroll suppressed during active selection ---

// TestLogModel_AutoScroll_SuppressedDuringSelection verifies that when a
// selection is visible and the viewport is at the bottom, appending new lines
// does NOT auto-scroll (YOffset stays where it was).
func TestLogModel_AutoScroll_SuppressedDuringSelection(t *testing.T) {
	m := newLogModel(80, 5)

	// Populate with enough lines to fill the viewport.
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(20)})

	// Scroll to bottom and confirm.
	m.viewport.GotoBottom()
	if !m.viewport.AtBottom() {
		t.Fatal("precondition: viewport must be at bottom")
	}

	// Set a committed non-empty selection.
	setCommittedSelection(&m, 10, 11)

	if !m.sel.visible() {
		t.Fatal("precondition: selection must be visible")
	}

	yBefore := m.viewport.YOffset

	// Append new lines — should not trigger GotoBottom because selection is visible.
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(5)})

	if m.viewport.YOffset != yBefore {
		t.Errorf("expected YOffset=%d (no autoscroll), got %d", yBefore, m.viewport.YOffset)
	}
}

// --- TP-106-05: auto-scroll resumes after selection is cleared ---

// TestLogModel_AutoScroll_ResumesAfterSelectionCleared verifies that after
// the selection is cleared, subsequent LogLinesMsg again triggers GotoBottom
// when the viewport was at the bottom.
func TestLogModel_AutoScroll_ResumesAfterSelectionCleared(t *testing.T) {
	m := newLogModel(80, 5)

	// Populate and scroll to bottom.
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(20)})
	m.viewport.GotoBottom()

	// Set a committed selection and confirm auto-scroll is suppressed.
	setCommittedSelection(&m, 10, 11)
	yBefore := m.viewport.YOffset
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(5)})
	if m.viewport.YOffset != yBefore {
		t.Errorf("suppression check: expected YOffset unchanged, got %d (was %d)",
			m.viewport.YOffset, yBefore)
	}

	// Clear the selection and scroll back to bottom.
	m = m.ClearSelection()
	m.viewport.GotoBottom()
	if !m.viewport.AtBottom() {
		t.Fatal("expected viewport at bottom after clear + GotoBottom")
	}

	yBeforeClear := m.viewport.YOffset

	// Now appending should auto-scroll.
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(10)})

	if m.viewport.YOffset <= yBeforeClear {
		t.Errorf("expected auto-scroll after selection cleared: YOffset %d → %d (should have increased)",
			yBeforeClear, m.viewport.YOffset)
	}
}

// --- TP-106-06: visual coords recomputed after rewrap ---

// TestLogModel_NewLogLines_RecomputeVisualCoords_AfterRewrap verifies that
// after new lines arrive (triggering rewrap), selection.anchor.visualRow and
// cursor.visualRow are updated to reflect the current visualLines positions
// while rawIdx and rawOffset remain unchanged.
func TestLogModel_NewLogLines_RecomputeVisualCoords_AfterRewrap(t *testing.T) {
	m := newLogModel(80, 20)

	// Add two lines so rawIdx 0 and 1 exist.
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello", "world"}})

	// Set a committed selection spanning the two lines.
	// Deliberately set visualRow to a stale/wrong value to confirm recompute fixes it.
	m.sel = selection{
		anchor:    pos{rawIdx: 0, rawOffset: 0, visualRow: 99, col: 0},
		cursor:    pos{rawIdx: 1, rawOffset: 0, visualRow: 99, col: 0},
		committed: true,
	}

	// Push more lines — triggers rewrap and recomputeSelectionVisualCoords.
	m, _ = m.Update(LogLinesMsg{Lines: []string{"extra"}})

	// rawIdx and rawOffset should be unchanged.
	if m.sel.anchor.rawIdx != 0 || m.sel.anchor.rawOffset != 0 {
		t.Errorf("anchor raw coords changed: got rawIdx=%d rawOffset=%d",
			m.sel.anchor.rawIdx, m.sel.anchor.rawOffset)
	}
	if m.sel.cursor.rawIdx != 1 || m.sel.cursor.rawOffset != 0 {
		t.Errorf("cursor raw coords changed: got rawIdx=%d rawOffset=%d",
			m.sel.cursor.rawIdx, m.sel.cursor.rawOffset)
	}

	// visualRow should now be correct (0 and 1 for lines "hello" and "world").
	if m.sel.anchor.visualRow != 0 {
		t.Errorf("anchor.visualRow: want 0, got %d", m.sel.anchor.visualRow)
	}
	if m.sel.cursor.visualRow != 1 {
		t.Errorf("cursor.visualRow: want 1, got %d", m.sel.cursor.visualRow)
	}
}
