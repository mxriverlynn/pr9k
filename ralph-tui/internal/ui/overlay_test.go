package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestOverlay_ANSIOutsideModalPreserved verifies that ANSI escape sequences
// in base lines outside the modal region are preserved intact after the splice.
func TestOverlay_ANSIOutsideModalPreserved(t *testing.T) {
	// Build a two-line base where line 0 has ANSI coloring before the splice point.
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("RED")
	blue := lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Render("BLUE")
	// Construct base: line 0 has colored text followed by plain text; line 1 is plain.
	baseLine0 := red + "     " + blue
	baseLine1 := "untouched line"
	base := baseLine0 + "\n" + baseLine1

	// Modal occupies the center of line 0 only; line 1 must be untouched.
	modal := "XYZ"
	result := overlay(base, modal, 0, 3) // place at row 0, col 3

	resultLines := strings.Split(result, "\n")
	if len(resultLines) < 2 {
		t.Fatalf("expected 2 result lines, got %d", len(resultLines))
	}

	// Line 1 must be byte-identical to the original.
	if resultLines[1] != baseLine1 {
		t.Errorf("line 1 changed: want %q, got %q", baseLine1, resultLines[1])
	}

	// The modal text must appear in line 0.
	plainLine0 := stripANSI(resultLines[0])
	if !strings.Contains(plainLine0, "XYZ") {
		t.Errorf("modal text not found in line 0: %q", plainLine0)
	}
}

// TestSpliceAt_ANSIColorsInsideModalSurvive verifies that ANSI SGR sequences
// inside the insert string are preserved in the output.
func TestSpliceAt_ANSIColorsInsideModalSurvive(t *testing.T) {
	base := "AAABBBCCC"
	insert := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("GREEN")

	result := spliceAt(base, insert, 3)

	// The result should contain the ANSI escape bytes from the insert.
	if !strings.Contains(result, insert) {
		t.Errorf("colored insert not preserved; result: %q", result)
	}
	plain := stripANSI(result)
	if !strings.Contains(plain, "GREEN") {
		t.Errorf("insert text not visible in plain result: %q", plain)
	}
}

// TestSpliceAt_WideRuneAtBoundary verifies that a wide rune (CJK/emoji)
// straddling the left splice boundary does not produce garbled output and
// does not panic.
func TestSpliceAt_WideRuneAtBoundary(t *testing.T) {
	// "A" (width 1) + "中" (width 2) + "Z" (width 1) = 4 total visible columns.
	// left=2 falls in the middle of "中" (which occupies columns 1–2).
	base := "A中Z"
	insert := "X"

	// Must not panic.
	result := spliceAt(base, insert, 2)

	plain := stripANSI(result)
	if !strings.Contains(plain, "X") {
		t.Errorf("insert 'X' not found in result: %q", plain)
	}
	// 'Z' is past the insert (which has width 1, so right edge = 3), so Z at col 3 survives.
	if !strings.Contains(plain, "Z") {
		t.Errorf("trailing 'Z' not found in result: %q", plain)
	}
}

// TestOverlay_ModalRowsPastBaseClipped verifies that when the modal's top
// offset plus its height exceeds the base frame's row count, the extra rows
// are silently clipped and no panic occurs.
func TestOverlay_ModalRowsPastBaseClipped(t *testing.T) {
	base := "line0\nline1\nline2"
	// Modal starts at row 2 and has 5 lines — rows 4–6 extend past the base.
	modal := "R0\nR1\nR2\nR3\nR4"

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("overlay panicked: %v", r)
		}
	}()

	result := overlay(base, modal, 2, 0)
	lines := strings.Split(result, "\n")

	// Base had 3 lines; result must still have exactly 3.
	if len(lines) != 3 {
		t.Errorf("expected 3 result lines, got %d", len(lines))
	}
	// Row 2 should contain "R0" (the first modal line).
	if !strings.Contains(lines[2], "R0") {
		t.Errorf("modal row 0 not spliced into base row 2: %q", lines[2])
	}
}

// TestOverlay_NegativeTopLeft_ClippedGracefully verifies that negative top and
// left offsets are handled without panicking and that valid modal lines are still
// rendered.
func TestOverlay_NegativeTopLeft_ClippedGracefully(t *testing.T) {
	base := "AAAAAA\nBBBBBB\nCCCCCC"
	modal := "M0\nM1\nM2"

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("overlay panicked with negative offsets: %v", r)
		}
	}()

	// top=-1 means rows -1, 0, 1 in base; row -1 is clipped; rows 0 and 1 receive M1 and M2.
	result := overlay(base, modal, -1, 0)
	lines := strings.Split(result, "\n")

	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
	// row 0 should have M1 (modal line at index 1 = top+1 = 0)
	if !strings.Contains(lines[0], "M1") {
		t.Errorf("expected M1 in row 0: %q", lines[0])
	}
}
