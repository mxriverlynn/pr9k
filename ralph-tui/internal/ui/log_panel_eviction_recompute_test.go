package ui

import (
	"testing"
)

// ─── Category 1: recomputeSelectionVisualCoords edge cases ───────────────────

// TestRecomputeSelectionVisualCoords_NoSelection verifies C1-01: calling
// recomputeSelectionVisualCoords on a zero-value selection is a no-op and
// returns the unchanged zero-value. Exercises the early-return guard at
// log_panel.go:254.
func TestRecomputeSelectionVisualCoords_NoSelection(t *testing.T) {
	m := newLogModel(80, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello", "world"}})

	// sel is the zero value — neither active nor committed.
	if m.sel != (selection{}) {
		t.Fatal("precondition: expected zero-value selection after populate")
	}

	result := m.recomputeSelectionVisualCoords()
	if result != (selection{}) {
		t.Errorf("expected zero-value selection returned, got %+v", result)
	}
}

// TestRecomputeSelectionVisualCoords_AnchorNotInVisualLines verifies C1-02:
// when the anchor's rawIdx is not present in any visualLine entry, the function
// returns a zero-value selection. Exercises the !ok1 path at log_panel.go:259.
func TestRecomputeSelectionVisualCoords_AnchorNotInVisualLines(t *testing.T) {
	m := newLogModel(80, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello"}})

	// Set a committed selection with anchor.rawIdx pointing to a non-existent
	// rawIdx (e.g., already evicted or otherwise absent).
	m.sel = selection{
		anchor:    pos{rawIdx: 999, rawOffset: 0},
		cursor:    pos{rawIdx: 0, rawOffset: 0},
		committed: true,
	}

	result := m.recomputeSelectionVisualCoords()
	if result != (selection{}) {
		t.Errorf("expected zero-value selection when anchor rawIdx invalid, got %+v", result)
	}
}

// TestRecomputeSelectionVisualCoords_CursorNotInVisualLines verifies C1-03:
// when the cursor's rawIdx is invalid while the anchor's is valid, the function
// returns a zero-value selection. Exercises the !ok2 path at log_panel.go:259.
func TestRecomputeSelectionVisualCoords_CursorNotInVisualLines(t *testing.T) {
	m := newLogModel(80, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello"}})

	m.sel = selection{
		anchor:    pos{rawIdx: 0, rawOffset: 0},
		cursor:    pos{rawIdx: 999, rawOffset: 0},
		committed: true,
	}

	result := m.recomputeSelectionVisualCoords()
	if result != (selection{}) {
		t.Errorf("expected zero-value selection when cursor rawIdx invalid, got %+v", result)
	}
}

// TestRecomputeSelectionVisualCoords_PreservesFlags verifies C1-04: an active
// (not committed) selection with valid rawIdx values is recomputed with
// active=true, committed=false, and correct visualRow values.
func TestRecomputeSelectionVisualCoords_PreservesFlags(t *testing.T) {
	m := newLogModel(80, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello", "world"}})

	// Set active (not committed) selection with deliberately stale visual coords.
	m.sel = selection{
		anchor:    pos{rawIdx: 0, rawOffset: 0, visualRow: 99, col: 0},
		cursor:    pos{rawIdx: 1, rawOffset: 0, visualRow: 99, col: 0},
		active:    true,
		committed: false,
	}

	result := m.recomputeSelectionVisualCoords()

	if !result.active {
		t.Error("expected active=true to be preserved")
	}
	if result.committed {
		t.Error("expected committed=false to be preserved")
	}
	if result.anchor.visualRow != 0 {
		t.Errorf("anchor.visualRow: want 0, got %d", result.anchor.visualRow)
	}
	if result.cursor.visualRow != 1 {
		t.Errorf("cursor.visualRow: want 1, got %d", result.cursor.visualRow)
	}
}

// ─── Category 2: findVisualPos edge cases ────────────────────────────────────

// TestFindVisualPos_EmptyVisualLines verifies C2-01: findVisualPos returns
// ok=false when visualLines is empty.
func TestFindVisualPos_EmptyVisualLines(t *testing.T) {
	m := newLogModel(80, 20)
	// No lines added — visualLines is empty.
	_, ok := m.findVisualPos(pos{rawIdx: 0, rawOffset: 0})
	if ok {
		t.Error("expected ok=false for empty visualLines")
	}
}

// TestFindVisualPos_PicksLastMatchingSegment verifies C2-02: for a raw line
// that wraps into three visual segments, findVisualPos picks the last segment
// whose rawOffset <= p.rawOffset, not the first.
//
// Width=6 wraps "hello world foo" into:
//
//	seg 0: "hello"  rawOffset=0
//	seg 1: "world"  rawOffset=6
//	seg 2: "foo"    rawOffset=12
func TestFindVisualPos_PicksLastMatchingSegment(t *testing.T) {
	m := newLogModel(6, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello world foo"}})

	if len(m.visualLines) != 3 {
		t.Fatalf("precondition: expected 3 visual segments for width=6, got %d", len(m.visualLines))
	}

	// p.rawOffset=7 falls inside segment 1 (rawOffset 6..11).
	result, ok := m.findVisualPos(pos{rawIdx: 0, rawOffset: 7})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if result.visualRow != 1 {
		t.Errorf("expected visualRow=1 (last segment with rawOffset<=7), got %d", result.visualRow)
	}
}

// TestFindVisualPos_RecomputesColFromRawOffset verifies C2-03: the returned
// pos has col equal to the display cell width from the segment's rawOffset to
// p.rawOffset within the raw line.
//
// rawLine[6:7] == "w" (first byte of "world"), cell width == 1.
func TestFindVisualPos_RecomputesColFromRawOffset(t *testing.T) {
	m := newLogModel(6, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello world foo"}})

	// p is 1 byte into segment 1 (which starts at rawOffset=6).
	result, ok := m.findVisualPos(pos{rawIdx: 0, rawOffset: 7})
	if !ok {
		t.Fatal("expected ok=true")
	}
	// rawLine[6:7] = "w" → lipgloss.Width("w") == 1.
	if result.col != 1 {
		t.Errorf("expected col=1 (cell width of rawLine[6:7]), got %d", result.col)
	}
}

// TestFindVisualPos_RawOffsetPastEndOfLine verifies C2-04: when p.rawOffset
// exceeds len(rawLine), findVisualPos returns ok=true (the rawIdx exists in
// visualLines) but skips col recomputation. The sentinel col value from the
// input is preserved unchanged.
func TestFindVisualPos_RawOffsetPastEndOfLine(t *testing.T) {
	// "hello world foo" has len=15. Width=6 gives 3 segments.
	m := newLogModel(6, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello world foo"}})

	// rawOffset=100 far exceeds len("hello world foo")=15.
	// Sentinel col=42 should be returned unchanged.
	p := pos{rawIdx: 0, rawOffset: 100, col: 42}
	result, ok := m.findVisualPos(p)
	if !ok {
		t.Fatal("expected ok=true: rawIdx=0 exists in visualLines even though rawOffset is past end")
	}
	if result.col != 42 {
		t.Errorf("expected col unchanged at 42 (recomputation skipped when rawOffset>len(rawLine)), got %d", result.col)
	}
}

// ─── Category 3: Eviction adjustment edge cases ──────────────────────────────

// TestEviction_InactiveSelection_NoOp verifies C3-01: when sel is the
// zero-value (neither active nor committed), eviction does not modify it.
// Exercises the guard at log_panel.go:313.
func TestEviction_InactiveSelection_NoOp(t *testing.T) {
	m := newLogModel(80, 20)
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(logRingBufferCap)})

	if m.sel != (selection{}) {
		t.Fatal("precondition: expected zero-value selection")
	}

	// Trigger eviction — the guard should skip adjustment entirely.
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(5)})

	if m.sel != (selection{}) {
		t.Errorf("expected zero-value selection after eviction with inactive sel, got %+v", m.sel)
	}
}

// TestEviction_BoundaryRawIdxZero verifies C3-02: eviction that reduces
// anchor.rawIdx to exactly 0 does not falsely clear the selection. The check
// is < 0, so == 0 is the surviving boundary.
func TestEviction_BoundaryRawIdxZero(t *testing.T) {
	m := newLogModel(80, 20)
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(logRingBufferCap)})

	// anchor.rawIdx=5, cursor.rawIdx=8. Evicting 5 lines → anchor becomes 0.
	setCommittedSelection(&m, 5, 8)

	// Push exactly 5 lines → evict 5.
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(5)})

	if m.sel == (selection{}) {
		t.Fatal("expected selection to survive: anchor.rawIdx should be 0 (not < 0)")
	}
	if m.sel.anchor.rawIdx != 0 {
		t.Errorf("anchor.rawIdx: want 0, got %d", m.sel.anchor.rawIdx)
	}
	if m.sel.cursor.rawIdx != 3 {
		t.Errorf("cursor.rawIdx: want 3, got %d", m.sel.cursor.rawIdx)
	}
}

// TestEviction_MultipleSuccessiveEvictions verifies C3-03: two separate
// eviction batches each correctly decrement rawIdx from the already-decremented
// value, confirming cumulative correctness.
func TestEviction_MultipleSuccessiveEvictions(t *testing.T) {
	m := newLogModel(80, 20)
	fill := logRingBufferCap - 5 // 1995
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(fill)})

	anchorRaw := fill - 2 // 1993
	cursorRaw := fill - 1 // 1994
	setCommittedSelection(&m, anchorRaw, cursorRaw)

	// First batch: push 10 → total 2005 → evict 5.
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(10)})
	wantAnchor1 := anchorRaw - 5 // 1988
	wantCursor1 := cursorRaw - 5 // 1989
	if m.sel.anchor.rawIdx != wantAnchor1 {
		t.Errorf("after first eviction: anchor.rawIdx want %d, got %d", wantAnchor1, m.sel.anchor.rawIdx)
	}
	if m.sel.cursor.rawIdx != wantCursor1 {
		t.Errorf("after first eviction: cursor.rawIdx want %d, got %d", wantCursor1, m.sel.cursor.rawIdx)
	}

	// Second batch: push 10 more → total 2010 → evict 10.
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(10)})
	wantAnchor2 := wantAnchor1 - 10 // 1978
	wantCursor2 := wantCursor1 - 10 // 1979
	if m.sel.anchor.rawIdx != wantAnchor2 {
		t.Errorf("after second eviction: anchor.rawIdx want %d, got %d", wantAnchor2, m.sel.anchor.rawIdx)
	}
	if m.sel.cursor.rawIdx != wantCursor2 {
		t.Errorf("after second eviction: cursor.rawIdx want %d, got %d", wantCursor2, m.sel.cursor.rawIdx)
	}
}

// ─── Category 4: Auto-scroll suppression edge cases ──────────────────────────

// TestAutoScroll_SuppressedDuringActiveSelection verifies C4-01: auto-scroll
// is suppressed when the selection is active (mid-drag, not yet committed).
// Existing tests only exercise committed selections; this covers the active path.
func TestAutoScroll_SuppressedDuringActiveSelection(t *testing.T) {
	m := newLogModel(80, 5)
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(20)})
	m.viewport.GotoBottom()

	if !m.viewport.AtBottom() {
		t.Fatal("precondition: viewport must be at bottom")
	}

	// Set an active (not committed) selection spanning two lines.
	m.sel = selection{
		anchor:    pos{rawIdx: 5, rawOffset: 0},
		cursor:    pos{rawIdx: 6, rawOffset: 0},
		active:    true,
		committed: false,
	}
	if !m.sel.visible() {
		t.Fatal("precondition: active selection must be visible()")
	}

	yBefore := m.viewport.YOffset
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(5)})

	if m.viewport.YOffset != yBefore {
		t.Errorf("expected YOffset=%d (no autoscroll during active selection), got %d",
			yBefore, m.viewport.YOffset)
	}
}

// TestAutoScroll_NotSuppressedByEmptyCommittedSelection verifies C4-02: a
// committed selection where anchor == cursor (visible() returns false) does
// not suppress auto-scroll. Auto-scroll proceeds normally.
func TestAutoScroll_NotSuppressedByEmptyCommittedSelection(t *testing.T) {
	m := newLogModel(80, 5)
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(20)})
	m.viewport.GotoBottom()

	if !m.viewport.AtBottom() {
		t.Fatal("precondition: viewport must be at bottom")
	}

	// Set a committed selection with anchor == cursor (empty range).
	// visible() returns false for an empty committed selection.
	m.sel = selection{
		anchor:    pos{rawIdx: 5, rawOffset: 3},
		cursor:    pos{rawIdx: 5, rawOffset: 3},
		committed: true,
	}
	if m.sel.visible() {
		t.Fatal("precondition: empty committed selection must not be visible()")
	}

	yBefore := m.viewport.YOffset
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(5)})

	if m.viewport.YOffset <= yBefore {
		t.Errorf("expected auto-scroll (YOffset > %d) when selection not visible, got YOffset=%d",
			yBefore, m.viewport.YOffset)
	}
}

// TestAutoScroll_NotTriggeredWhenNotAtBottom verifies C4-03: when the viewport
// is not at the bottom, appending lines does not auto-scroll regardless of
// selection visibility. Confirms the wasAtBottom guard operates independently
// of the selection check.
func TestAutoScroll_NotTriggeredWhenNotAtBottom(t *testing.T) {
	m := newLogModel(80, 5)
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(20)})

	// Scroll to top so wasAtBottom will be false.
	m.viewport.GotoTop()
	if m.viewport.AtBottom() {
		t.Skip("viewport is at bottom even after GotoTop — content fits in viewport; test not applicable")
	}

	yBefore := m.viewport.YOffset

	// Append more lines. wasAtBottom=false → GotoBottom never called.
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(5)})

	if m.viewport.YOffset != yBefore {
		t.Errorf("expected YOffset=%d (no autoscroll when not at bottom), got %d",
			yBefore, m.viewport.YOffset)
	}
}

// ─── Category 5: recompute + eviction interaction ────────────────────────────

// TestRecomputeAfterEviction_VisualRowCorrect verifies C5-01: after eviction
// adjusts rawIdx (without clearing the selection) and the Update pipeline
// completes, recomputeSelectionVisualCoords has replaced the stale visualRow
// values with the correct post-eviction positions.
func TestRecomputeAfterEviction_VisualRowCorrect(t *testing.T) {
	m := newLogModel(80, 20)
	fill := logRingBufferCap - 10 // 1990
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(fill)})

	anchorRaw := fill - 2 // 1988
	cursorRaw := fill - 1 // 1989

	// Deliberately set stale visualRow values to confirm recompute overwrites them.
	m.sel = selection{
		anchor:    pos{rawIdx: anchorRaw, rawOffset: 0, visualRow: 99999},
		cursor:    pos{rawIdx: cursorRaw, rawOffset: 0, visualRow: 99999},
		committed: true,
	}

	// Push 15 → evict 5. anchor: 1988→1983, cursor: 1989→1984.
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(15)})
	const evicted = 5
	wantAnchorRaw := anchorRaw - evicted // 1983
	wantCursorRaw := cursorRaw - evicted // 1984

	if m.sel.anchor.rawIdx != wantAnchorRaw {
		t.Errorf("anchor.rawIdx: want %d, got %d", wantAnchorRaw, m.sel.anchor.rawIdx)
	}
	if m.sel.cursor.rawIdx != wantCursorRaw {
		t.Errorf("cursor.rawIdx: want %d, got %d", wantCursorRaw, m.sel.cursor.rawIdx)
	}
	// Stale value must have been overwritten.
	if m.sel.anchor.visualRow == 99999 {
		t.Error("anchor.visualRow still stale (99999) after eviction + recompute")
	}
	if m.sel.cursor.visualRow == 99999 {
		t.Error("cursor.visualRow still stale (99999) after eviction + recompute")
	}
	// With short lines (no wrapping at width=80), rawIdx == visualRow.
	if m.sel.anchor.visualRow != wantAnchorRaw {
		t.Errorf("anchor.visualRow: want %d, got %d", wantAnchorRaw, m.sel.anchor.visualRow)
	}
	if m.sel.cursor.visualRow != wantCursorRaw {
		t.Errorf("cursor.visualRow: want %d, got %d", wantCursorRaw, m.sel.cursor.visualRow)
	}
}

// TestEvictionClearsBeforeRecompute verifies C5-02: when eviction underflows
// anchor.rawIdx (brings it below 0), the selection is cleared before
// recomputeSelectionVisualCoords runs. Recompute's early-return guard fires on
// the already-cleared zero-value selection and does not resurrect it.
func TestEvictionClearsBeforeRecompute(t *testing.T) {
	m := newLogModel(80, 20)
	m, _ = m.Update(LogLinesMsg{Lines: makeLines(logRingBufferCap)})

	// anchor.rawIdx=0 underflows when 1 line is evicted.
	setCommittedSelection(&m, 0, 5)

	// Push 1 line → evict 1 → anchor.rawIdx = -1 < 0 → selection cleared.
	m, _ = m.Update(LogLinesMsg{Lines: []string{"new line"}})

	if m.sel != (selection{}) {
		t.Errorf("expected zero-value selection after underflow; recompute must not resurrect it: got anchor.rawIdx=%d cursor.rawIdx=%d",
			m.sel.anchor.rawIdx, m.sel.cursor.rawIdx)
	}
}

// ─── Category 6: SetSize + recompute interaction ─────────────────────────────

// TestSetSize_DoesNotRecomputeSelectionVisualCoords verifies C6-01: SetSize
// calls rewrap but does NOT call recomputeSelectionVisualCoords. A selection
// with a deliberately stale visualRow remains stale after the resize.
func TestSetSize_DoesNotRecomputeSelectionVisualCoords(t *testing.T) {
	m := newLogModel(80, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello", "world"}})

	// Set a committed selection with deliberately stale visual coords.
	m.sel = selection{
		anchor:    pos{rawIdx: 0, rawOffset: 0, visualRow: 99},
		cursor:    pos{rawIdx: 1, rawOffset: 0, visualRow: 99},
		committed: true,
	}

	// Resize to a different width — rewrap is triggered but recompute is not.
	m.SetSize(60, 20)

	if m.sel.anchor.visualRow != 99 {
		t.Errorf("expected stale anchor.visualRow=99 after SetSize (no recompute), got %d",
			m.sel.anchor.visualRow)
	}
	if m.sel.cursor.visualRow != 99 {
		t.Errorf("expected stale cursor.visualRow=99 after SetSize (no recompute), got %d",
			m.sel.cursor.visualRow)
	}
}

// TestSetSize_SubsequentLogLinesMsg_CorrectsStaleVisualCoords verifies C6-02:
// after a width-change resize leaves visual coords stale, a subsequent
// LogLinesMsg triggers recomputeSelectionVisualCoords and corrects them.
func TestSetSize_SubsequentLogLinesMsg_CorrectsStaleVisualCoords(t *testing.T) {
	m := newLogModel(80, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello", "world"}})

	m.sel = selection{
		anchor:    pos{rawIdx: 0, rawOffset: 0, visualRow: 99},
		cursor:    pos{rawIdx: 1, rawOffset: 0, visualRow: 99},
		committed: true,
	}
	m.SetSize(60, 20)

	// The following LogLinesMsg triggers recompute, correcting the stale coords.
	m, _ = m.Update(LogLinesMsg{Lines: []string{"extra"}})

	if m.sel.anchor.visualRow != 0 {
		t.Errorf("anchor.visualRow: want 0 after LogLinesMsg recompute, got %d", m.sel.anchor.visualRow)
	}
	if m.sel.cursor.visualRow != 1 {
		t.Errorf("cursor.visualRow: want 1 after LogLinesMsg recompute, got %d", m.sel.cursor.visualRow)
	}
}

// ─── Category 7: Integration with model.go ───────────────────────────────────

// TestModel_LogLinesMsg_DuringModeSelect_PreservesSelection verifies C7-01:
// sending a LogLinesMsg while in ModeSelect preserves the active selection —
// raw coordinates are unchanged and the selection is not cleared.
func TestModel_LogLinesMsg_DuringModeSelect_PreservesSelection(t *testing.T) {
	m := newSelectTestModel(t, ModeNormal)
	populateLog(t, &m, 5)

	// Enter ModeSelect via 'v'.
	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)
	if m.keys.handler.Mode() != ModeSelect {
		t.Fatal("precondition: expected ModeSelect after v")
	}
	if m.log.sel == (selection{}) {
		t.Fatal("precondition: selection must be initialized in ModeSelect")
	}

	rawIdxBefore := m.log.sel.anchor.rawIdx
	rawOffBefore := m.log.sel.anchor.rawOffset

	// Send LogLinesMsg during ModeSelect — selection must survive.
	next, _ = m.Update(LogLinesMsg{Lines: makeLines(3)})
	m = next.(Model)

	if m.log.sel == (selection{}) {
		t.Error("expected selection preserved after LogLinesMsg in ModeSelect")
	}
	if m.log.sel.anchor.rawIdx != rawIdxBefore {
		t.Errorf("anchor.rawIdx changed: want %d, got %d", rawIdxBefore, m.log.sel.anchor.rawIdx)
	}
	if m.log.sel.anchor.rawOffset != rawOffBefore {
		t.Errorf("anchor.rawOffset changed: want %d, got %d", rawOffBefore, m.log.sel.anchor.rawOffset)
	}
}

// TestModel_ExternalSetMode_ClearsSelectionOnNextUpdate verifies C7-02: an
// external call to SetMode(ModeNormal) while in ModeSelect is detected by the
// prevObservedMode guard at model.go:98 on the next Update, clearing the
// selection overlay.
func TestModel_ExternalSetMode_ClearsSelectionOnNextUpdate(t *testing.T) {
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step-one"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	m := NewModel(header, kh, "v0")
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	// Populate the log so ModeSelect can be entered.
	next, _ := m.Update(LogLinesMsg{Lines: makeLines(5)})
	m = next.(Model)

	// Enter ModeSelect via 'v'. After this Update, prevObservedMode = ModeSelect.
	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)
	if m.prevObservedMode != ModeSelect {
		t.Fatal("precondition: prevObservedMode must be ModeSelect after v")
	}
	if m.log.sel == (selection{}) {
		t.Fatal("precondition: selection must be set")
	}

	// External mode change — simulates the orchestration goroutine calling SetMode.
	kh.SetMode(ModeNormal)

	// Next Update: the prevObservedMode guard (model.go:98) fires and clears sel.
	next, _ = m.Update(LogLinesMsg{Lines: makeLines(2)})
	m = next.(Model)

	if m.log.sel != (selection{}) {
		t.Errorf("expected selection cleared after external SetMode(ModeNormal), got anchor.rawIdx=%d",
			m.log.sel.anchor.rawIdx)
	}
}
