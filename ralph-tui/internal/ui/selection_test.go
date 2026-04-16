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
		_, ok := MouseToViewport(msg, topRow, leftCol, vp)
		if ok {
			t.Error("expected ok=false for click above topRow")
		}
	})

	t.Run("below_viewport_bottom", func(t *testing.T) {
		msg := tea.MouseMsg{X: 10, Y: topRow + vp.Height}
		_, ok := MouseToViewport(msg, topRow, leftCol, vp)
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
	p, ok := MouseToViewport(msg, topRow, leftCol, vp)
	if !ok {
		t.Fatal("expected ok=true for click inside viewport")
	}
	wantVisualRow := vp.YOffset + (7 - topRow) // 10 + 2 = 12
	wantCol := 15 - leftCol                     // 14
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
