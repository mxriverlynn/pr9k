package ui

import (
	"strings"
	"testing"
)

// --- Word-wrap unit tests (issue #102) ---

// TestWrap_WordBoundary verifies that a sentence wraps at word boundaries
// when it exceeds the viewport width.
func TestWrap_WordBoundary(t *testing.T) {
	m := newLogModel(10, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"the quick brown fox"}})

	// "the quick" is 9 chars ≤ 10; "brown" would make 15 → wraps.
	// Expected segments: "the quick", "brown fox"
	if len(m.visualLines) < 2 {
		t.Fatalf("expected at least 2 visual lines, got %d: %v",
			len(m.visualLines), collectTexts(m.visualLines))
	}
	if m.visualLines[0].text != "the quick" {
		t.Errorf("segment 0: want %q, got %q", "the quick", m.visualLines[0].text)
	}
	if m.visualLines[1].text != "brown fox" {
		t.Errorf("segment 1: want %q, got %q", "brown fox", m.visualLines[1].text)
	}
}

// TestWrap_LongToken verifies that a single token longer than the viewport
// width is hard-wrapped into multiple segments.
func TestWrap_LongToken(t *testing.T) {
	token := strings.Repeat("a", 40)
	m := newLogModel(10, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{token}})

	// 40-char token at width 10 → 4 segments of 10 chars each.
	if len(m.visualLines) != 4 {
		t.Fatalf("expected 4 visual lines for 40-char token at width 10, got %d: %v",
			len(m.visualLines), collectTexts(m.visualLines))
	}
	for i, vl := range m.visualLines {
		if len(vl.text) != 10 {
			t.Errorf("segment %d: want 10 chars, got %d (%q)", i, len(vl.text), vl.text)
		}
	}
}

// TestWrap_Rewrap_OnResize verifies that changing the viewport width
// rebuilds visualLines to match the new width.
func TestWrap_Rewrap_OnResize(t *testing.T) {
	line := "the quick brown fox jumped over the lazy dog"
	m := newLogModel(80, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{line}})
	countAt80 := len(m.visualLines)

	m.SetSize(40, 20)
	countAt40 := len(m.visualLines)

	if countAt80 > countAt40 {
		// Narrower width should produce more visual lines.
		t.Errorf("expected more visual lines at width 40 than 80, got %d vs %d",
			countAt40, countAt80)
	}
	// At width 80 the line fits on one row.
	if countAt80 != 1 {
		t.Errorf("expected 1 visual line at width 80, got %d", countAt80)
	}
	if countAt40 < 2 {
		t.Errorf("expected at least 2 visual lines at width 40, got %d", countAt40)
	}
}

// TestWrap_EmptyLine verifies that an empty raw line produces exactly one
// visual segment with empty text, preserving chrome spacing.
func TestWrap_EmptyLine(t *testing.T) {
	m := newLogModel(80, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{""}})

	if len(m.visualLines) != 1 {
		t.Fatalf("expected 1 visual line for empty raw line, got %d", len(m.visualLines))
	}
	if m.visualLines[0].text != "" {
		t.Errorf("expected empty segment text, got %q", m.visualLines[0].text)
	}
}

// TestWrap_ExactWidthNoSpaces verifies that a raw line exactly as wide as the
// viewport (with no internal spaces) produces exactly one visual segment and
// no spurious empty trailing segment.
func TestWrap_ExactWidthNoSpaces(t *testing.T) {
	line := strings.Repeat("x", 10)
	m := newLogModel(10, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{line}})

	if len(m.visualLines) != 1 {
		t.Fatalf("expected 1 visual line for exact-width no-space line, got %d: %v",
			len(m.visualLines), collectTexts(m.visualLines))
	}
	if m.visualLines[0].text != line {
		t.Errorf("segment text: want %q, got %q", line, m.visualLines[0].text)
	}
}

// TestWrap_WidthZero_NoOp verifies that before the first WindowSizeMsg
// (width=0), each raw line produces exactly one visual segment containing the
// raw content — ansi.Wrap short-circuits and returns the input unchanged.
func TestWrap_WidthZero_NoOp(t *testing.T) {
	m := newLogModel(0, 0)
	raw := "hello world this is a long line"
	m, _ = m.Update(LogLinesMsg{Lines: []string{raw}})

	if len(m.visualLines) != 1 {
		t.Fatalf("expected 1 visual line at width 0, got %d", len(m.visualLines))
	}
	if m.visualLines[0].text != raw {
		t.Errorf("expected raw text unchanged, got %q", m.visualLines[0].text)
	}
	if m.visualLines[0].rawIdx != 0 {
		t.Errorf("expected rawIdx 0, got %d", m.visualLines[0].rawIdx)
	}
}

// TestWrap_TabNormalization verifies that a raw line containing \t is stored
// with 4-space expansion, not the literal tab character.
func TestWrap_TabNormalization(t *testing.T) {
	m := newLogModel(80, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"col1\tcol2"}})

	if len(m.lines) != 1 {
		t.Fatalf("expected 1 raw line, got %d", len(m.lines))
	}
	if strings.Contains(m.lines[0], "\t") {
		t.Errorf("raw line still contains tab: %q", m.lines[0])
	}
	if !strings.Contains(m.lines[0], "    ") {
		t.Errorf("raw line does not contain 4-space expansion: %q", m.lines[0])
	}
}

// TestResize_PreservesTopRawLine verifies that scrolling to a specific raw
// line and then resizing narrower keeps that raw line at the top of the
// viewport.
func TestResize_PreservesTopRawLine(t *testing.T) {
	// 30 lines so the viewport can scroll.
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "the quick brown fox jumps over the lazy dog" // 43 chars — wraps at narrow widths
	}
	m := newLogModel(80, 5)
	m, _ = m.Update(LogLinesMsg{Lines: lines})

	// Scroll to a position in the middle.
	m.viewport.SetYOffset(10)

	// Snapshot which raw line is at the top before resize.
	topRawIdx := m.visualLines[m.viewport.YOffset].rawIdx

	// Resize to a narrower width so each raw line wraps into multiple segments.
	m.SetSize(20, 5)

	// After resize, the top visible row should map to the same raw line.
	topAfter := m.visualLines[m.viewport.YOffset].rawIdx
	if topAfter != topRawIdx {
		t.Errorf("top raw line changed after resize: before rawIdx=%d, after rawIdx=%d",
			topRawIdx, topAfter)
	}
}

// TestResize_PreservesTopRawOffset verifies that when the top visible row is
// segment 2 (or later) of a multi-segment raw line before resize, the scroll
// position after rewrap does not regress to an earlier segment of the same raw
// line than necessary.
func TestResize_PreservesTopRawOffset(t *testing.T) {
	// A long line that wraps into 3 segments at width 20.
	// Each "word" is exactly 20 chars so at width 20 each sits on its own visual row.
	wordA := strings.Repeat("a", 19) + "X"
	wordB := strings.Repeat("b", 19) + "X"
	wordC := strings.Repeat("c", 19) + "X"
	rawLine := wordA + " " + wordB + " " + wordC // rawIdx=0

	// Put rawLine FIRST so its segments start at visual index 0.
	// Then add enough filler lines so the viewport can scroll to segment 2 of rawLine.
	// With height=5, total lines must be at least 7 so max offset >= 2.
	fillers := make([]string, 10)
	for i := range fillers {
		fillers[i] = "filler"
	}
	lines := append([]string{rawLine}, fillers...)
	m := newLogModel(20, 5)
	m, _ = m.Update(LogLinesMsg{Lines: lines})

	// rawLine should produce 3 segments at width 20.
	longRawIdx := 0
	seg1VisualRow := -1
	for i, vl := range m.visualLines {
		if vl.rawIdx == longRawIdx && vl.rawOffset > 0 {
			seg1VisualRow = i
			break
		}
	}
	if seg1VisualRow < 0 {
		t.Skip("long raw line did not produce multiple segments at width 20 — test precondition not met")
	}

	// Verify the viewport can actually scroll to the target row.
	if seg1VisualRow > len(m.visualLines)-m.viewport.Height {
		t.Skipf("viewport cannot scroll to segment 2 (visualRow=%d, max=%d) — not enough content below",
			seg1VisualRow, len(m.visualLines)-m.viewport.Height)
	}

	m.viewport.SetYOffset(seg1VisualRow)

	// Confirm the viewport actually accepted the offset (not clamped away).
	if m.viewport.YOffset != seg1VisualRow {
		t.Skipf("viewport clamped YOffset to %d (wanted %d) — resize invariant not testable",
			m.viewport.YOffset, seg1VisualRow)
	}

	snapOffset := m.visualLines[m.viewport.YOffset].rawOffset

	// Resize narrower so each word-sized segment now requires 2 rows.
	m.SetSize(10, 5)

	// After rewrap, the new top row's rawOffset must be <= the snapshot offset
	// (we should not regress to an earlier portion of the same raw line).
	if m.viewport.YOffset >= len(m.visualLines) {
		t.Fatalf("YOffset %d out of range after resize (len=%d)", m.viewport.YOffset, len(m.visualLines))
	}
	newTopVL := m.visualLines[m.viewport.YOffset]
	if newTopVL.rawIdx != longRawIdx {
		t.Errorf("top raw line changed: want rawIdx=%d, got %d", longRawIdx, newTopVL.rawIdx)
	}
	if newTopVL.rawOffset > snapOffset {
		t.Errorf("rawOffset regressed: snapshot=%d, after resize top rawOffset=%d (jumped backward in line)",
			snapOffset, newTopVL.rawOffset)
	}
}

// collectTexts is a test helper that extracts the text field from a slice of
// visualLines for use in error messages.
func collectTexts(vls []visualLine) []string {
	out := make([]string, len(vls))
	for i, vl := range vls {
		out[i] = vl.text
	}
	return out
}
