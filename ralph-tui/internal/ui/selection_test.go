package ui

import (
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// TestSelection_Normalize verifies that normalized() always returns (start, end)
// in reading order by (rawIdx, rawOffset). When the anchor is further in the
// document than the cursor, they are swapped; equal positions are returned
// unchanged.
func TestSelection_Normalize(t *testing.T) {
	t.Run("anchor_below_cursor_swapped", func(t *testing.T) {
		s := selection{
			anchor: pos{rawIdx: 3, rawOffset: 5},
			cursor: pos{rawIdx: 1, rawOffset: 2},
		}
		start, end := s.normalized()
		if start.rawIdx != 1 || start.rawOffset != 2 {
			t.Errorf("start: want {rawIdx:1 rawOffset:2}, got {rawIdx:%d rawOffset:%d}", start.rawIdx, start.rawOffset)
		}
		if end.rawIdx != 3 || end.rawOffset != 5 {
			t.Errorf("end: want {rawIdx:3 rawOffset:5}, got {rawIdx:%d rawOffset:%d}", end.rawIdx, end.rawOffset)
		}
	})

	t.Run("same_rawIdx_anchor_offset_greater", func(t *testing.T) {
		s := selection{
			anchor: pos{rawIdx: 2, rawOffset: 10},
			cursor: pos{rawIdx: 2, rawOffset: 3},
		}
		start, end := s.normalized()
		if start.rawOffset != 3 || end.rawOffset != 10 {
			t.Errorf("want start.rawOffset=3 end.rawOffset=10, got %d %d", start.rawOffset, end.rawOffset)
		}
	})

	t.Run("equal_positions_unchanged", func(t *testing.T) {
		s := selection{
			anchor: pos{rawIdx: 1, rawOffset: 4},
			cursor: pos{rawIdx: 1, rawOffset: 4},
		}
		start, end := s.normalized()
		if start.rawIdx != 1 || start.rawOffset != 4 || end.rawIdx != 1 || end.rawOffset != 4 {
			t.Errorf("expected equal positions, got start={%d,%d} end={%d,%d}",
				start.rawIdx, start.rawOffset, end.rawIdx, end.rawOffset)
		}
	})
}

// TestSelection_ExtractText_SingleRawLine_MultipleVisual verifies that a
// selection spanning two wrapped visual segments of a single raw line yields
// the raw substring with no intra-line newline, regardless of wrap boundaries.
func TestSelection_ExtractText_SingleRawLine_MultipleVisual(t *testing.T) {
	// Raw line: "hello world foo bar"
	// Suppose visual segments wrap as: "hello world" | "foo bar"
	// A selection from offset 6 to offset 15 spans "world fo" — all in rawIdx 0.
	lines := []string{"hello world foo bar"}
	start := pos{rawIdx: 0, rawOffset: 6}
	end := pos{rawIdx: 0, rawOffset: 15}
	got := extractText(lines, start, end)
	want := "world foo"
	if got != want {
		t.Errorf("extractText: want %q, got %q", want, got)
	}
}

// TestSelection_ExtractText_AcrossRawLines verifies that a selection spanning
// two raw lines yields the selected text from each line joined by exactly one
// newline, preserving the original ring-buffer newline structure.
func TestSelection_ExtractText_AcrossRawLines(t *testing.T) {
	lines := []string{"first line", "second line", "third line"}
	// Select from offset 6 of line 0 to offset 6 of line 1.
	start := pos{rawIdx: 0, rawOffset: 6}
	end := pos{rawIdx: 1, rawOffset: 6}
	got := extractText(lines, start, end)
	want := "line\nsecond"
	if got != want {
		t.Errorf("extractText: want %q, got %q", want, got)
	}
}

// TestExtractText_PartialRawLine_SingleLine verifies that a selection wholly
// inside one raw line (not starting at offset 0 and not reaching the end)
// returns only the substring, with no newlines.
func TestExtractText_PartialRawLine_SingleLine(t *testing.T) {
	lines := []string{"abcdefghij"}
	start := pos{rawIdx: 0, rawOffset: 3}
	end := pos{rawIdx: 0, rawOffset: 7}
	got := extractText(lines, start, end)
	want := "defg"
	if got != want {
		t.Errorf("extractText: want %q, got %q", want, got)
	}
}

// TestMouseToViewport_OutsideArea verifies that a click above topRow or below
// the bottom of the viewport returns ok=false.
func TestMouseToViewport_OutsideArea(t *testing.T) {
	vp := viewport.New(80, 20)
	vp.YOffset = 0

	topRow := 5
	leftCol := 1

	t.Run("above_topRow", func(t *testing.T) {
		msg := tea.MouseMsg{X: 10, Y: topRow - 1}
		_, ok := mouseToViewport(msg, topRow, leftCol, vp)
		if ok {
			t.Error("expected ok=false for click above topRow")
		}
	})

	t.Run("below_viewport_bottom", func(t *testing.T) {
		msg := tea.MouseMsg{X: 10, Y: topRow + vp.Height}
		_, ok := mouseToViewport(msg, topRow, leftCol, vp)
		if ok {
			t.Error("expected ok=false for click below viewport bottom")
		}
	})
}

// TestMouseToViewport_InsideArea verifies that a click inside the viewport
// content area returns ok=true with the correct visualRow and col values.
// visualRow = vp.YOffset + (msg.Y - topRow), col = msg.X - leftCol.
func TestMouseToViewport_InsideArea(t *testing.T) {
	vp := viewport.New(80, 20)
	vp.YOffset = 10

	topRow := 5
	leftCol := 1

	// Click at screen row 7 (2 rows below topRow) and screen col 15.
	msg := tea.MouseMsg{X: 15, Y: 7}
	p, ok := mouseToViewport(msg, topRow, leftCol, vp)
	if !ok {
		t.Fatal("expected ok=true for click inside viewport")
	}
	wantVisualRow := vp.YOffset + (7 - topRow) // 10 + 2 = 12
	wantCol := 15 - leftCol                    // 14
	if p.visualRow != wantVisualRow {
		t.Errorf("visualRow: want %d, got %d", wantVisualRow, p.visualRow)
	}
	if p.col != wantCol {
		t.Errorf("col: want %d, got %d", wantCol, p.col)
	}
}

// TestVisualColToRawOffset_AsciiOnly verifies that for a plain ASCII segment,
// each display cell corresponds to exactly one byte, so the returned byte
// offset equals col.
func TestVisualColToRawOffset_AsciiOnly(t *testing.T) {
	rawLine := "hello world"
	// segmentStart=0, col=5 → offset should be 5 (pointing at ' ')
	got := visualColToRawOffset(rawLine, 0, 5)
	if got != 5 {
		t.Errorf("want offset 5, got %d", got)
	}
	// segmentStart=6, col=3 → offset should be 6+3=9 (pointing at 'l')
	got = visualColToRawOffset(rawLine, 6, 3)
	if got != 9 {
		t.Errorf("want offset 9, got %d", got)
	}
}

// TestVisualColToRawOffset_ExceedsSegment verifies that when col is greater
// than the total display cells available from segmentStart to end of rawLine,
// the function returns len(rawLine) (the segment's end offset).
func TestVisualColToRawOffset_ExceedsSegment(t *testing.T) {
	rawLine := "hello"
	got := visualColToRawOffset(rawLine, 0, 100)
	if got != len(rawLine) {
		t.Errorf("want len(rawLine)=%d, got %d", len(rawLine), got)
	}
}

// TestVisualColToRawOffset_MultiCellGraphemes verifies that wide runes (CJK
// characters, emoji) advance the cell count by their display width (typically
// 2 cells), so that col is a display-cell index rather than a byte index.
func TestVisualColToRawOffset_MultiCellGraphemes(t *testing.T) {
	// '中' is a 3-byte CJK character with display width 2.
	// rawLine: "中a" → cells: [0,1]=中, [2]=a
	rawLine := "中a"
	midLen := len("中") // 3

	// col=0 → before '中' → offset 0
	got := visualColToRawOffset(rawLine, 0, 0)
	if got != 0 {
		t.Errorf("col=0: want 0, got %d", got)
	}

	// col=2 → after '中' (2 cells consumed) → offset 3 (start of 'a')
	got = visualColToRawOffset(rawLine, 0, 2)
	if got != midLen {
		t.Errorf("col=2: want %d, got %d", midLen, got)
	}

	// col=3 → after 'a' → offset 4 = len(rawLine)
	got = visualColToRawOffset(rawLine, 0, 3)
	if got != len(rawLine) {
		t.Errorf("col=3: want %d, got %d", len(rawLine), got)
	}

	// col=1 falls inside the wide char → returns offset 0 (before '中')
	got = visualColToRawOffset(rawLine, 0, 1)
	if got != 0 {
		t.Errorf("col=1 (inside wide char): want 0, got %d", got)
	}
}

// TestSelection_Visible verifies the three states:
// - empty inactive/uncommitted selection → false
// - active selection (mid-drag) → true regardless of range
// - committed non-empty selection → true
// - committed empty (anchor == cursor) → false
func TestSelection_Visible(t *testing.T) {
	t.Run("zero_value_not_visible", func(t *testing.T) {
		var s selection
		if s.visible() {
			t.Error("zero-value selection should not be visible")
		}
	})

	t.Run("active_mid_drag_visible", func(t *testing.T) {
		s := selection{
			anchor: pos{rawIdx: 0, rawOffset: 0},
			cursor: pos{rawIdx: 0, rawOffset: 0}, // same cell
			active: true,
		}
		if !s.visible() {
			t.Error("active selection should be visible even if anchor == cursor")
		}
	})

	t.Run("committed_non_empty_visible", func(t *testing.T) {
		s := selection{
			anchor:    pos{rawIdx: 0, rawOffset: 0},
			cursor:    pos{rawIdx: 0, rawOffset: 5},
			committed: true,
		}
		if !s.visible() {
			t.Error("committed non-empty selection should be visible")
		}
	})

	t.Run("committed_empty_not_visible", func(t *testing.T) {
		s := selection{
			anchor:    pos{rawIdx: 1, rawOffset: 3},
			cursor:    pos{rawIdx: 1, rawOffset: 3},
			committed: true,
		}
		if s.visible() {
			t.Error("committed selection with anchor==cursor should not be visible")
		}
	})
}

// TestSelection_Normalize_SameRawIdxEqualOffsets verifies that when anchor and
// cursor have the same rawIdx and the same rawOffset, normalized() returns the
// anchor as start (the <= comparison does not swap equal positions).
func TestSelection_Normalize_SameRawIdxEqualOffsets(t *testing.T) {
	s := selection{
		anchor: pos{rawIdx: 2, rawOffset: 5},
		cursor: pos{rawIdx: 2, rawOffset: 5},
	}
	start, end := s.normalized()
	// When equal, the anchor is returned as start (because anchor.rawOffset <= cursor.rawOffset is true).
	if start != s.anchor || end != s.cursor {
		t.Errorf("equal positions: want anchor as start, got start=%+v end=%+v", start, end)
	}
}

// TestSelection_Normalize_RawIdxPriorityOverRawOffset verifies that rawIdx
// dominates the ordering comparison: when anchor.rawIdx < cursor.rawIdx, the
// anchor is always the start even if anchor.rawOffset > cursor.rawOffset.
func TestSelection_Normalize_RawIdxPriorityOverRawOffset(t *testing.T) {
	s := selection{
		anchor: pos{rawIdx: 1, rawOffset: 99},
		cursor: pos{rawIdx: 3, rawOffset: 0},
	}
	start, end := s.normalized()
	if start.rawIdx != 1 || start.rawOffset != 99 {
		t.Errorf("start: want {rawIdx:1 rawOffset:99}, got {rawIdx:%d rawOffset:%d}", start.rawIdx, start.rawOffset)
	}
	if end.rawIdx != 3 || end.rawOffset != 0 {
		t.Errorf("end: want {rawIdx:3 rawOffset:0}, got {rawIdx:%d rawOffset:%d}", end.rawIdx, end.rawOffset)
	}
}

// TestExtractText_ThreeOrMoreRawLines verifies that a selection spanning three
// raw lines includes all intermediate lines in the output, emitting a newline
// before each intermediate and before the final line segment.
func TestExtractText_ThreeOrMoreRawLines(t *testing.T) {
	lines := []string{"line zero", "line one", "line two", "line three"}
	start := pos{rawIdx: 0, rawOffset: 5}
	end := pos{rawIdx: 3, rawOffset: 4}
	got := extractText(lines, start, end)
	want := "zero\nline one\nline two\nline"
	if got != want {
		t.Errorf("extractText 3+ lines: want %q, got %q", want, got)
	}
}

// TestExtractText_FullRawLine verifies that selecting from offset 0 to
// len(line) on a single raw line returns the entire line without modification.
func TestExtractText_FullRawLine(t *testing.T) {
	line := "full line content"
	lines := []string{line}
	start := pos{rawIdx: 0, rawOffset: 0}
	end := pos{rawIdx: 0, rawOffset: len(line)}
	got := extractText(lines, start, end)
	if got != line {
		t.Errorf("extractText full line: want %q, got %q", line, got)
	}
}

// TestExtractText_EmptyMiddleLine verifies that an empty string in a middle
// raw line produces consecutive newlines (\n\n) in the extracted output.
func TestExtractText_EmptyMiddleLine(t *testing.T) {
	lines := []string{"first", "", "last"}
	start := pos{rawIdx: 0, rawOffset: 0}
	end := pos{rawIdx: 2, rawOffset: 4}
	got := extractText(lines, start, end)
	want := "first\n\nlast"
	if got != want {
		t.Errorf("extractText empty middle: want %q, got %q", want, got)
	}
}

// TestExtractText_StartAtEndOfLine verifies that when start.rawOffset equals
// len(lines[startIdx]), the extracted start segment is empty but no panic occurs,
// and the result is a newline followed by the beginning of the next line.
func TestExtractText_StartAtEndOfLine(t *testing.T) {
	lines := []string{"hello", "world"}
	start := pos{rawIdx: 0, rawOffset: len("hello")}
	end := pos{rawIdx: 1, rawOffset: 5}
	got := extractText(lines, start, end)
	want := "\nworld"
	if got != want {
		t.Errorf("extractText start at end: want %q, got %q", want, got)
	}
}

// TestExtractText_EndAtOffsetZero verifies that when end.rawOffset is 0, the
// extracted segment from the final line is empty and the trailing newline before
// it is still emitted.
func TestExtractText_EndAtOffsetZero(t *testing.T) {
	lines := []string{"hello", "world"}
	start := pos{rawIdx: 0, rawOffset: 0}
	end := pos{rawIdx: 1, rawOffset: 0}
	got := extractText(lines, start, end)
	want := "hello\n"
	if got != want {
		t.Errorf("extractText end at offset 0: want %q, got %q", want, got)
	}
}

// TestMouseToViewport_ExactBottomBoundary verifies that a click at exactly
// topRow + vp.Height - 1 is inside the viewport (ok=true), confirming the
// boundary check uses > not >=.
func TestMouseToViewport_ExactBottomBoundary(t *testing.T) {
	vp := viewport.New(80, 20)
	topRow := 5
	leftCol := 1
	msg := tea.MouseMsg{X: 10, Y: topRow + vp.Height - 1}
	_, ok := mouseToViewport(msg, topRow, leftCol, vp)
	if !ok {
		t.Error("expected ok=true for click at exact bottom boundary")
	}
}

// TestMouseToViewport_ExactTopBoundary verifies that a click at exactly topRow
// is inside the viewport (ok=true) and returns visualRow == vp.YOffset.
func TestMouseToViewport_ExactTopBoundary(t *testing.T) {
	vp := viewport.New(80, 20)
	vp.YOffset = 7
	topRow := 5
	leftCol := 1
	msg := tea.MouseMsg{X: 10, Y: topRow}
	p, ok := mouseToViewport(msg, topRow, leftCol, vp)
	if !ok {
		t.Error("expected ok=true for click at exact top boundary")
	}
	if p.visualRow != vp.YOffset {
		t.Errorf("visualRow: want %d, got %d", vp.YOffset, p.visualRow)
	}
}

// TestMouseToViewport_NegativeCol verifies that when msg.X < leftCol the
// function still returns ok=true (row is valid) with a negative col value.
// Callers are responsible for clamping negative col values.
func TestMouseToViewport_NegativeCol(t *testing.T) {
	vp := viewport.New(80, 20)
	topRow := 5
	leftCol := 10
	msg := tea.MouseMsg{X: 3, Y: topRow + 2} // X < leftCol
	p, ok := mouseToViewport(msg, topRow, leftCol, vp)
	if !ok {
		t.Error("expected ok=true even when col would be negative (row is valid)")
	}
	if p.col >= 0 {
		t.Errorf("expected negative col, got %d", p.col)
	}
}

// TestVisualColToRawOffset_ColZero verifies that col=0 always returns
// segmentStart, regardless of segment content.
func TestVisualColToRawOffset_ColZero(t *testing.T) {
	rawLine := "hello"
	got := visualColToRawOffset(rawLine, 0, 0)
	if got != 0 {
		t.Errorf("segmentStart=0 col=0: want 0, got %d", got)
	}
	got = visualColToRawOffset(rawLine, 3, 0)
	if got != 3 {
		t.Errorf("segmentStart=3 col=0: want 3, got %d", got)
	}
}

// TestVisualColToRawOffset_EmptyRawLine verifies that an empty rawLine with
// any col returns 0 without panicking.
func TestVisualColToRawOffset_EmptyRawLine(t *testing.T) {
	rawLine := ""
	for _, col := range []int{0, 1, 100} {
		got := visualColToRawOffset(rawLine, 0, col)
		if got != 0 {
			t.Errorf("empty rawLine col=%d: want 0, got %d", col, got)
		}
	}
}

// TestVisualColToRawOffset_SegmentStartAtEndOfRawLine verifies that when
// segmentStart equals len(rawLine), the function returns len(rawLine)
// without panicking regardless of col.
func TestVisualColToRawOffset_SegmentStartAtEndOfRawLine(t *testing.T) {
	rawLine := "hello"
	segmentStart := len(rawLine)
	got := visualColToRawOffset(rawLine, segmentStart, 5)
	if got != len(rawLine) {
		t.Errorf("segmentStart at end: want %d, got %d", len(rawLine), got)
	}
}

// TestVisualColToRawOffset_MultiByteNonWide verifies that a multi-byte but
// single-cell character (like "é", 2 bytes, 1 display cell) advances the byte
// offset by 2 but the cell count by only 1, unlike CJK wide characters.
func TestVisualColToRawOffset_MultiByteNonWide(t *testing.T) {
	// "é" is 2 bytes (U+00E9) with display width 1.
	rawLine := "éa"
	eLen := len("é") // 2

	// col=0 → before 'é' → segmentStart + 0
	got := visualColToRawOffset(rawLine, 0, 0)
	if got != 0 {
		t.Errorf("col=0: want 0, got %d", got)
	}

	// col=1 → after 'é' (1 cell consumed, 2 bytes) → offset 2
	got = visualColToRawOffset(rawLine, 0, 1)
	if got != eLen {
		t.Errorf("col=1: want %d (after é), got %d", eLen, got)
	}

	// col=2 → after 'a' → offset 3 = len(rawLine)
	got = visualColToRawOffset(rawLine, 0, 2)
	if got != len(rawLine) {
		t.Errorf("col=2: want %d, got %d", len(rawLine), got)
	}
}

// TestSelection_Contains verifies that contains() uses a half-open convention
// ([start.col, end.col)) and handles single-row and multi-row ranges correctly.
func TestSelection_Contains(t *testing.T) {
	t.Run("single_row_inside", func(t *testing.T) {
		s := selection{
			anchor: pos{visualRow: 3, col: 2},
			cursor: pos{visualRow: 3, col: 7},
		}
		// col 2..6 are in range; col 7 is excluded (half-open)
		for col := 2; col < 7; col++ {
			if !s.contains(3, col) {
				t.Errorf("col %d should be inside single-row selection", col)
			}
		}
		if s.contains(3, 7) {
			t.Error("col 7 (end) should be excluded by half-open convention")
		}
		if s.contains(3, 1) {
			t.Error("col 1 (before start) should be outside")
		}
	})

	t.Run("single_row_wrong_row", func(t *testing.T) {
		s := selection{
			anchor: pos{visualRow: 3, col: 0},
			cursor: pos{visualRow: 3, col: 5},
		}
		if s.contains(2, 2) || s.contains(4, 2) {
			t.Error("different rows should be outside a single-row selection")
		}
	})

	t.Run("multi_row_start_row", func(t *testing.T) {
		s := selection{
			anchor: pos{visualRow: 2, col: 4},
			cursor: pos{visualRow: 5, col: 3},
		}
		// On the start row, only cols >= 4 are in range.
		if !s.contains(2, 4) || !s.contains(2, 100) {
			t.Error("start row: cols >= anchor.col should be inside")
		}
		if s.contains(2, 3) {
			t.Error("start row: col < anchor.col should be outside")
		}
	})

	t.Run("multi_row_middle_rows", func(t *testing.T) {
		s := selection{
			anchor: pos{visualRow: 2, col: 4},
			cursor: pos{visualRow: 5, col: 3},
		}
		for row := 3; row <= 4; row++ {
			for _, col := range []int{0, 1, 50} {
				if !s.contains(row, col) {
					t.Errorf("middle row %d col %d should be inside", row, col)
				}
			}
		}
	})

	t.Run("multi_row_end_row", func(t *testing.T) {
		s := selection{
			anchor: pos{visualRow: 2, col: 4},
			cursor: pos{visualRow: 5, col: 3},
		}
		// On the end row, cols < 3 are in range; col 3 is excluded.
		if !s.contains(5, 0) || !s.contains(5, 2) {
			t.Error("end row: cols < cursor.col should be inside")
		}
		if s.contains(5, 3) {
			t.Error("end row: col == cursor.col should be excluded (half-open)")
		}
	})

	t.Run("outside_row_range", func(t *testing.T) {
		s := selection{
			anchor: pos{visualRow: 2, col: 0},
			cursor: pos{visualRow: 5, col: 5},
		}
		if s.contains(1, 0) || s.contains(6, 0) {
			t.Error("rows outside the selection range should return false")
		}
	})
}

// TestSelection_Contains_EmptySelection verifies that a selection where anchor
// equals cursor contains no cells — the half-open range [col, col) is empty.
func TestSelection_Contains_EmptySelection(t *testing.T) {
	s := selection{
		anchor: pos{visualRow: 2, col: 5},
		cursor: pos{visualRow: 2, col: 5},
	}
	for _, col := range []int{0, 4, 5, 6, 100} {
		if s.contains(2, col) {
			t.Errorf("empty selection: col %d should not be contained", col)
		}
	}
}

// TestSelection_Contains_ReversedAnchorCursor verifies that contains() works
// correctly when the anchor is after the cursor in document order, because
// contains uses normalized() internally. rawIdx drives the normalization order,
// so both fields must be set for the swap to happen.
func TestSelection_Contains_ReversedAnchorCursor(t *testing.T) {
	s := selection{
		anchor: pos{rawIdx: 5, rawOffset: 0, visualRow: 5, col: 3},
		cursor: pos{rawIdx: 2, rawOffset: 0, visualRow: 2, col: 1},
	}
	// Normalized: start={row:2,col:1}, end={row:5,col:3}
	// Middle rows (3, 4) should be fully included.
	if !s.contains(3, 0) || !s.contains(4, 50) {
		t.Error("reversed anchor/cursor: middle rows should be contained")
	}
	// Start row: col >= 1 included
	if !s.contains(2, 1) || s.contains(2, 0) {
		t.Error("reversed anchor/cursor: start row boundary incorrect")
	}
	// End row: col < 3 included
	if !s.contains(5, 2) || s.contains(5, 3) {
		t.Error("reversed anchor/cursor: end row boundary incorrect")
	}
}

// TestSelection_Contains_ColZeroIncluded verifies that a single-row selection
// starting at col=0 includes col=0 (the half-open range [0, N) is non-empty).
func TestSelection_Contains_ColZeroIncluded(t *testing.T) {
	s := selection{
		anchor: pos{visualRow: 1, col: 0},
		cursor: pos{visualRow: 1, col: 5},
	}
	if !s.contains(1, 0) {
		t.Error("col 0 should be included in a selection starting at col 0")
	}
	if s.contains(1, 5) {
		t.Error("col 5 (end) should be excluded by half-open convention")
	}
}

// TestSelection_Visible_ActiveAndCommitted verifies that when both active and
// committed are true, visible() returns true (active takes priority on the
// first branch), even if the range is non-empty.
func TestSelection_Visible_ActiveAndCommitted(t *testing.T) {
	s := selection{
		anchor:    pos{rawIdx: 0, rawOffset: 0},
		cursor:    pos{rawIdx: 0, rawOffset: 10},
		active:    true,
		committed: true,
	}
	if !s.visible() {
		t.Error("active+committed selection should be visible")
	}
}

// TestSelection_Visible_CommittedReversed verifies that visible() correctly
// normalizes a committed selection whose anchor is after the cursor before
// checking for emptiness.
func TestSelection_Visible_CommittedReversed(t *testing.T) {
	s := selection{
		anchor:    pos{rawIdx: 3, rawOffset: 5},
		cursor:    pos{rawIdx: 0, rawOffset: 2},
		committed: true,
	}
	if !s.visible() {
		t.Error("committed selection with reversed anchor/cursor should be visible (non-empty)")
	}
}

// TestExtractText_BoundsGuards verifies that extractText returns "" for any
// out-of-range rawIdx or rawOffset rather than panicking. This guards against
// stale coordinates after ring-buffer eviction or a rewrap.
func TestExtractText_BoundsGuards(t *testing.T) {
	lines := []string{"hello", "world"}

	cases := []struct {
		name  string
		start pos
		end   pos
	}{
		{"rawIdx_negative", pos{rawIdx: -1, rawOffset: 0}, pos{rawIdx: 0, rawOffset: 3}},
		{"rawIdx_out_of_range", pos{rawIdx: 0, rawOffset: 0}, pos{rawIdx: 5, rawOffset: 0}},
		{"rawOffset_negative_single_line", pos{rawIdx: 0, rawOffset: -1}, pos{rawIdx: 0, rawOffset: 3}},
		{"rawOffset_past_end_single_line", pos{rawIdx: 0, rawOffset: 0}, pos{rawIdx: 0, rawOffset: 100}},
		{"rawOffset_start_past_end_single_line", pos{rawIdx: 0, rawOffset: 4}, pos{rawIdx: 0, rawOffset: 2}},
		{"rawOffset_past_end_start_multiline", pos{rawIdx: 0, rawOffset: 100}, pos{rawIdx: 1, rawOffset: 3}},
		{"rawOffset_past_end_end_multiline", pos{rawIdx: 0, rawOffset: 0}, pos{rawIdx: 1, rawOffset: 100}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractText(lines, tc.start, tc.end)
			if got != "" {
				t.Errorf("expected \"\", got %q", got)
			}
		})
	}
}

// TestExtractText_InputImmutability verifies that calling extractText does not
// mutate the lines slice passed by the caller.
func TestExtractText_InputImmutability(t *testing.T) {
	original := []string{"alpha", "beta", "gamma"}
	snapshot := make([]string, len(original))
	copy(snapshot, original)

	start := pos{rawIdx: 0, rawOffset: 1}
	end := pos{rawIdx: 2, rawOffset: 3}
	_ = extractText(original, start, end)

	for i, s := range original {
		if s != snapshot[i] {
			t.Errorf("lines[%d] mutated: was %q, now %q", i, snapshot[i], s)
		}
	}
}

// TestZeroValue_NoPanic verifies that zero-value pos{} and selection{} do not
// panic when passed to normalized(), visible(), and contains(). This documents
// safe behavior with uninitialized structs.
func TestZeroValue_NoPanic(t *testing.T) {
	var s selection
	// None of these should panic.
	_, _ = s.normalized()
	_ = s.visible()
	_ = s.contains(0, 0)
}
