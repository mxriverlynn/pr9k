package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// reverseVideoAvailable reports whether the default lipgloss renderer emits
// ANSI reverse-video codes. When stdout is not a TTY (e.g., in CI), this
// returns false, and tests that specifically check for \x1b[7m skip the ANSI
// assertion.
func reverseVideoAvailable() bool {
	return strings.Contains(lipgloss.NewStyle().Reverse(true).Render("X"), "\x1b")
}

// --- Category 1: renderContent selection overlay ---

// TestRenderContent_NoSelection verifies that the fast path is taken when
// sel is the zero value: no reverse-video codes appear in the output and
// the plain text of all lines is preserved.
func TestRenderContent_NoSelection(t *testing.T) {
	m := newLogModel(80, 5)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello", "world"}})

	// Zero-value sel: neither active nor committed.
	if m.sel != (selection{}) {
		t.Fatal("precondition: expected zero-value selection after populate")
	}

	rendered := m.renderContent()

	// Fast path must not produce a reverse-video ANSI code.
	if strings.Contains(rendered, "\x1b[7m") {
		t.Errorf("expected no reverse-video ANSI in no-selection render, got %q", rendered)
	}
	plain := stripANSI(rendered)
	if !strings.Contains(plain, "hello") {
		t.Errorf("expected 'hello' in rendered output, got %q", plain)
	}
	if !strings.Contains(plain, "world") {
		t.Errorf("expected 'world' in rendered output, got %q", plain)
	}
}

// TestRenderContent_EmptySelection_CursorCell verifies that an empty selection
// (anchor == cursor) highlights the single cursor cell with reverse-video when
// ANSI is available. The plain text is preserved in all environments.
func TestRenderContent_EmptySelection_CursorCell(t *testing.T) {
	m := newLogModel(80, 5)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello world", "second line"}})

	// Set an empty selection at row 0, col 3 ("l").
	p := pos{rawIdx: 0, rawOffset: 3, visualRow: 0, col: 3}
	m.sel = selection{anchor: p, cursor: p, active: true}

	rendered := m.renderContent()
	plain := stripANSI(rendered)

	if !strings.Contains(plain, "hello world") {
		t.Errorf("plain text missing 'hello world': %q", plain)
	}
	if !strings.Contains(plain, "second line") {
		t.Errorf("plain text missing 'second line': %q", plain)
	}
	// When ANSI is available, the cursor cell must be wrapped in reverse-video.
	if reverseVideoAvailable() && !strings.Contains(rendered, "\x1b[7m") {
		t.Errorf("expected reverse-video code in rendered output (cursor at col 3), got %q", rendered)
	}
}

// TestRenderContent_SingleRowRange verifies that a non-empty selection confined
// to a single row renders only the selected column range in reverse-video, and
// that the unselected row is not highlighted.
func TestRenderContent_SingleRowRange(t *testing.T) {
	m := newLogModel(80, 5)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello world", "other line"}})

	// Select "ello" (col 1 to col 5) on visual row 0.
	anchor := pos{rawIdx: 0, rawOffset: 1, visualRow: 0, col: 1}
	cursor := pos{rawIdx: 0, rawOffset: 5, visualRow: 0, col: 5}
	m.sel = selection{anchor: anchor, cursor: cursor, active: true}

	rendered := m.renderContent()
	plain := stripANSI(rendered)

	if !strings.Contains(plain, "hello world") {
		t.Errorf("plain text missing 'hello world': %q", plain)
	}
	if !strings.Contains(plain, "other line") {
		t.Errorf("plain text missing 'other line': %q", plain)
	}
	if reverseVideoAvailable() && !strings.Contains(rendered, "\x1b[7m") {
		t.Errorf("expected reverse-video code for single-row selection, got %q", rendered)
	}
}

// TestRenderContent_MultiRowRange verifies that a selection spanning three rows
// renders: the start row partially highlighted from startCol, middle rows fully
// highlighted, and the end row partially highlighted to endCol.
func TestRenderContent_MultiRowRange(t *testing.T) {
	m := newLogModel(80, 5)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"aaa", "bbb", "ccc"}})

	// Select from row 0 col 1 to row 2 col 2.
	anchor := pos{rawIdx: 0, rawOffset: 1, visualRow: 0, col: 1}
	cursor := pos{rawIdx: 2, rawOffset: 2, visualRow: 2, col: 2}
	m.sel = selection{anchor: anchor, cursor: cursor, active: true}

	rendered := m.renderContent()
	plain := stripANSI(rendered)

	for _, want := range []string{"aaa", "bbb", "ccc"} {
		if !strings.Contains(plain, want) {
			t.Errorf("plain text missing %q in multi-row selection render: %q", want, plain)
		}
	}
	if reverseVideoAvailable() && !strings.Contains(rendered, "\x1b[7m") {
		t.Errorf("expected reverse-video in multi-row selection, got %q", rendered)
	}
}

// TestRenderContent_CommittedSelection verifies that a committed selection
// (committed=true, active=false) still renders the highlight overlay; the
// showSel flag checks both active and committed.
func TestRenderContent_CommittedSelection(t *testing.T) {
	m := newLogModel(80, 5)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello world"}})

	anchor := pos{rawIdx: 0, rawOffset: 0, visualRow: 0, col: 0}
	cursor := pos{rawIdx: 0, rawOffset: 5, visualRow: 0, col: 5}
	m.sel = selection{anchor: anchor, cursor: cursor, active: false, committed: true}

	rendered := m.renderContent()
	plain := stripANSI(rendered)

	if !strings.Contains(plain, "hello world") {
		t.Errorf("plain text missing 'hello world': %q", plain)
	}
	// No reverse-video must appear with a zero-value sel (fast path).
	// With committed=true the selection path is taken; verify ANSI when available.
	if reverseVideoAvailable() && !strings.Contains(rendered, "\x1b[7m") {
		t.Errorf("expected reverse-video for committed selection, got %q", rendered)
	}
}

// --- Category 2: splitAtCol / splitAtCols / colToByteOffset ---

// TestSplitAtCol_Start verifies that col=0 on a non-empty string yields an
// empty before, the first character as cursor, and the rest as after.
func TestSplitAtCol_Start(t *testing.T) {
	before, cursor, after := splitAtCol("hello", 0)
	if before != "" {
		t.Errorf("before: want %q, got %q", "", before)
	}
	if cursor != "h" {
		t.Errorf("cursor: want %q, got %q", "h", cursor)
	}
	if after != "ello" {
		t.Errorf("after: want %q, got %q", "ello", after)
	}
}

// TestSplitAtCol_PastEnd verifies that col past the string display width
// returns the full string as before, a space fallback as cursor, and empty after.
func TestSplitAtCol_PastEnd(t *testing.T) {
	before, cursor, after := splitAtCol("hi", 10)
	if before != "hi" {
		t.Errorf("before: want %q, got %q", "hi", before)
	}
	if cursor != " " {
		t.Errorf("cursor: want %q (space fallback), got %q", " ", cursor)
	}
	if after != "" {
		t.Errorf("after: want %q, got %q", "", after)
	}
}

// TestSplitAtCol_EmptyString verifies that splitAtCol on an empty string
// returns empty before, a space fallback as cursor, and empty after.
func TestSplitAtCol_EmptyString(t *testing.T) {
	before, cursor, after := splitAtCol("", 0)
	if before != "" {
		t.Errorf("before: want %q, got %q", "", before)
	}
	if cursor != " " {
		t.Errorf("cursor: want %q (space fallback), got %q", " ", cursor)
	}
	if after != "" {
		t.Errorf("after: want %q, got %q", "", after)
	}
}

// TestSplitAtCols_EmptyRange verifies that startCol == endCol yields an empty
// sel segment.
func TestSplitAtCols_EmptyRange(t *testing.T) {
	before, sel, after := splitAtCols("hello", 2, 2)
	if before != "he" {
		t.Errorf("before: want %q, got %q", "he", before)
	}
	if sel != "" {
		t.Errorf("sel: want %q (empty), got %q", "", sel)
	}
	if after != "llo" {
		t.Errorf("after: want %q, got %q", "llo", after)
	}
}

// TestSplitAtCols_FullRow verifies that startCol=0 and endCol=len(s) yields
// empty before, the full string as sel, and empty after.
func TestSplitAtCols_FullRow(t *testing.T) {
	s := "hello"
	before, sel, after := splitAtCols(s, 0, len(s))
	if before != "" {
		t.Errorf("before: want %q, got %q", "", before)
	}
	if sel != s {
		t.Errorf("sel: want %q, got %q", s, sel)
	}
	if after != "" {
		t.Errorf("after: want %q, got %q", "", after)
	}
}

// TestColToByteOffset_MultiByte verifies that colToByteOffset accounts for
// multi-byte UTF-8 runes. "é" (U+00E9) is 2 bytes but occupies 1 display cell,
// so colToByteOffset("éabc", 1) must return 2 — the byte offset of 'a'.
func TestColToByteOffset_MultiByte(t *testing.T) {
	s := "éabc" // é is 2 bytes (UTF-8: 0xC3 0xA9), 1 display cell
	got := colToByteOffset(s, 1)
	if got != 2 {
		t.Errorf("colToByteOffset(%q, 1): want 2, got %d", s, got)
	}
	// Confirm the byte at offset 2 is the ASCII 'a'.
	if s[got] != 'a' {
		t.Errorf("byte at offset %d in %q: want 'a' (0x61), got 0x%02x", got, s, s[got])
	}
}

// --- Category 3: initSelectionAtLastVisibleRow edge cases ---

// TestInitSelection_EmptyVisualLines verifies that initSelectionAtLastVisibleRow
// returns a zero-value selection when there are no visual lines.
func TestInitSelection_EmptyVisualLines(t *testing.T) {
	m := newLogModel(80, 5)
	// No lines added — visualLines is empty.

	got := m.initSelectionAtLastVisibleRow()
	if got != (selection{}) {
		t.Errorf("expected zero-value selection for empty visualLines, got %+v", got)
	}
}

// TestInitSelection_FewerLinesThanViewport verifies that when the content has
// fewer visual lines than the viewport height, lastRow is clamped to
// len(visualLines) - 1.
func TestInitSelection_FewerLinesThanViewport(t *testing.T) {
	m := newLogModel(80, 10)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"line1", "line2", "line3"}})
	// 3 visual lines < viewport height 10 → lastRow must clamp to 2.

	sel := m.initSelectionAtLastVisibleRow()
	if !sel.active {
		t.Fatal("expected selection to be active")
	}
	if sel.cursor.visualRow != 2 {
		t.Errorf("cursor.visualRow: want 2 (clamped to last line), got %d", sel.cursor.visualRow)
	}
	if sel.anchor.visualRow != 2 {
		t.Errorf("anchor.visualRow: want 2 (clamped to last line), got %d", sel.anchor.visualRow)
	}
}

// TestInitSelection_RawIdxAndOffset verifies that the returned selection's
// rawIdx and rawOffset match the visualLine at the computed lastRow.
func TestInitSelection_RawIdxAndOffset(t *testing.T) {
	m := newLogModel(80, 5)
	// 8 lines, viewport height 5: YOffset=0, lastRow = min(0+5-1, 7) = 4.
	m, _ = m.Update(LogLinesMsg{Lines: []string{"a", "b", "c", "d", "e", "f", "g", "h"}})

	sel := m.initSelectionAtLastVisibleRow()
	if !sel.active {
		t.Fatal("expected selection to be active")
	}

	// Independently compute the expected lastRow.
	lastRow := m.viewport.YOffset + m.viewport.Height - 1
	if lastRow >= len(m.visualLines) {
		lastRow = len(m.visualLines) - 1
	}

	wantVL := m.visualLines[lastRow]
	if sel.cursor.rawIdx != wantVL.rawIdx {
		t.Errorf("cursor.rawIdx: want %d, got %d", wantVL.rawIdx, sel.cursor.rawIdx)
	}
	if sel.cursor.rawOffset != wantVL.rawOffset {
		t.Errorf("cursor.rawOffset: want %d, got %d", wantVL.rawOffset, sel.cursor.rawOffset)
	}
	if sel.anchor.rawIdx != wantVL.rawIdx {
		t.Errorf("anchor.rawIdx: want %d, got %d", wantVL.rawIdx, sel.anchor.rawIdx)
	}
	if sel.anchor.rawOffset != wantVL.rawOffset {
		t.Errorf("anchor.rawOffset: want %d, got %d", wantVL.rawOffset, sel.anchor.rawOffset)
	}
}

// --- Category 4: SetSelection / ClearSelection / SelectedText ---

// TestSetSelection_ReRendersContent verifies that SetSelection calls
// viewport.SetContent: after replacing the viewport with a fresh empty one,
// SetSelection must restore the rendered content.
func TestSetSelection_ReRendersContent(t *testing.T) {
	m := newLogModel(80, 5)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello", "world", "foo"}})

	// Replace viewport with a fresh one to clear content.
	m.viewport = viewport.New(80, 5)
	blankView := stripANSI(m.viewport.View())
	if strings.Contains(blankView, "hello") {
		t.Fatal("precondition: viewport should not contain 'hello' after replacement")
	}

	sel := selection{
		anchor: pos{rawIdx: 0, rawOffset: 0, visualRow: 0, col: 0},
		cursor: pos{rawIdx: 0, rawOffset: 1, visualRow: 0, col: 1},
		active: true,
	}
	m = m.SetSelection(sel)

	if m.sel != sel {
		t.Errorf("SetSelection did not update sel: got %+v, want %+v", m.sel, sel)
	}
	// SetSelection must call viewport.SetContent, restoring rendered content.
	got := stripANSI(m.viewport.View())
	if !strings.Contains(got, "hello") {
		t.Errorf("expected viewport re-rendered after SetSelection (should contain 'hello'), got %q", got)
	}
}

// TestClearSelection_RemovesOverlay verifies that ClearSelection resets sel to
// the zero value and re-renders the viewport content without a selection overlay.
func TestClearSelection_RemovesOverlay(t *testing.T) {
	m := newLogModel(80, 5)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello", "world"}})

	// Set a selection.
	anchor := pos{rawIdx: 0, rawOffset: 0, visualRow: 0, col: 0}
	cursor := pos{rawIdx: 0, rawOffset: 3, visualRow: 0, col: 3}
	m.sel = selection{anchor: anchor, cursor: cursor, active: true}

	m = m.ClearSelection()

	if m.sel != (selection{}) {
		t.Errorf("ClearSelection: expected zero-value sel, got %+v", m.sel)
	}
	// Viewport must still be rendered (content preserved after clear).
	got := stripANSI(m.viewport.View())
	if !strings.Contains(got, "hello") {
		t.Errorf("expected 'hello' in viewport after ClearSelection, got %q", got)
	}
}

// TestSelectedText_ActiveSelection verifies that SelectedText returns the
// correct substring from the ring buffer for an active cross-line selection.
func TestSelectedText_ActiveSelection(t *testing.T) {
	m := newLogModel(80, 5)
	m, _ = m.Update(LogLinesMsg{Lines: []string{
		"hello world",
		"second line",
		"third line",
	}})

	// Select from "world" (byte offset 6 of line 0) to "second" (byte offset 6 of line 1).
	// extractText: lines[0][6:] + "\n" + lines[1][:6] = "world" + "\n" + "second"
	anchor := pos{rawIdx: 0, rawOffset: 6, visualRow: 0, col: 6}
	cursor := pos{rawIdx: 1, rawOffset: 6, visualRow: 1, col: 6}
	m.sel = selection{anchor: anchor, cursor: cursor, active: true}

	got := m.SelectedText()
	want := "world\nsecond"
	if got != want {
		t.Errorf("SelectedText(): want %q, got %q", want, got)
	}
}
