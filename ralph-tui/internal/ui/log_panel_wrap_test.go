package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

// --- Category 1: rewrap — rawOffset accuracy ---

// TestRewrap_RawOffset_WordBoundary verifies rawOffset values after a
// word-boundary wrap. "the quick brown fox" at width 10 wraps after "the quick"
// (9 chars); the consumed space at byte 9 advances the offset to 10.
func TestRewrap_RawOffset_WordBoundary(t *testing.T) {
	rawLine := "the quick brown fox"
	m := newLogModel(10, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{rawLine}})

	if len(m.visualLines) < 2 {
		t.Fatalf("expected at least 2 visual lines, got %d: %v",
			len(m.visualLines), collectTexts(m.visualLines))
	}
	if m.visualLines[0].rawOffset != 0 {
		t.Errorf("segment 0 rawOffset: want 0, got %d", m.visualLines[0].rawOffset)
	}
	// "the quick" = 9 bytes + 1 consumed space → next segment at offset 10.
	if m.visualLines[1].rawOffset != 10 {
		t.Errorf("segment 1 rawOffset: want 10, got %d (segment texts: %v)",
			m.visualLines[1].rawOffset, collectTexts(m.visualLines))
	}
}

// TestRewrap_RawOffset_HardWrap verifies rawOffset values for a hard-wrapped
// 30-char no-space token at width 10. No separators are consumed, so offsets
// advance by exactly 10 per segment.
func TestRewrap_RawOffset_HardWrap(t *testing.T) {
	rawLine := strings.Repeat("a", 30)
	m := newLogModel(10, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{rawLine}})

	if len(m.visualLines) != 3 {
		t.Fatalf("expected 3 visual lines for 30-char token at width 10, got %d: %v",
			len(m.visualLines), collectTexts(m.visualLines))
	}
	for i, want := range []int{0, 10, 20} {
		if m.visualLines[i].rawOffset != want {
			t.Errorf("segment %d rawOffset: want %d, got %d", i, want, m.visualLines[i].rawOffset)
		}
	}
}

// TestRewrap_RawOffset_HyphenBreak verifies that rawOffset advances correctly
// when ansi.Wrap breaks at a hyphen. Hyphens stay in the segment text (not
// consumed), so only spaces in the space-skipping loop advance the offset.
func TestRewrap_RawOffset_HyphenBreak(t *testing.T) {
	rawLine := "foo-bar-baz"
	m := newLogModel(7, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{rawLine}})

	if len(m.visualLines) < 2 {
		t.Skip("ansi.Wrap did not break 'foo-bar-baz' at width 7 — hyphen-break path not exercised")
	}
	if m.visualLines[0].rawOffset != 0 {
		t.Errorf("segment 0 rawOffset: want 0, got %d", m.visualLines[0].rawOffset)
	}
	// For each boundary: rawOffset[i+1] == rawOffset[i] + len(seg[i]) + spaces_consumed.
	// Hyphens are NOT consumed (only spaces are skipped by the rewrap loop).
	for i := 0; i < len(m.visualLines)-1; i++ {
		seg := m.visualLines[i]
		next := m.visualLines[i+1]
		pos := seg.rawOffset + len(seg.text)
		for pos < len(rawLine) && rawLine[pos] == ' ' {
			pos++
		}
		if next.rawOffset != pos {
			t.Errorf("segment %d→%d: expected rawOffset %d, got %d (seg text=%q)",
				i, i+1, pos, next.rawOffset, seg.text)
		}
	}
}

// TestRewrap_MultipleRawLines_MixedWrapping verifies rawIdx tracking across
// three raw lines with different wrapping behavior: one short (no wrap), one
// that wraps into 2 segments, and one empty.
func TestRewrap_MultipleRawLines_MixedWrapping(t *testing.T) {
	lines := []string{
		"hello",               // rawIdx=0: 1 segment (5 chars ≤ 10)
		"the quick brown fox", // rawIdx=1: 2 segments at width 10
		"",                    // rawIdx=2: 1 segment (empty)
	}
	m := newLogModel(10, 20)
	m, _ = m.Update(LogLinesMsg{Lines: lines})

	if len(m.visualLines) != 4 {
		t.Fatalf("expected 4 visual lines (1+2+1), got %d: %v",
			len(m.visualLines), collectTexts(m.visualLines))
	}
	wantRawIdx := []int{0, 1, 1, 2}
	for i, want := range wantRawIdx {
		if m.visualLines[i].rawIdx != want {
			t.Errorf("visualLines[%d].rawIdx: want %d, got %d", i, want, m.visualLines[i].rawIdx)
		}
	}
}

// --- Category 2: rewrap — edge cases ---

// TestRewrap_TrailingSpaces verifies that trailing spaces on a line that fits
// entirely within the viewport width are preserved in the visual segment.
func TestRewrap_TrailingSpaces(t *testing.T) {
	rawLine := "hello     "
	m := newLogModel(80, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{rawLine}})

	if len(m.visualLines) != 1 {
		t.Fatalf("expected 1 visual line for %q at width 80, got %d", rawLine, len(m.visualLines))
	}
	if m.visualLines[0].text != rawLine {
		t.Errorf("segment text: want %q, got %q", rawLine, m.visualLines[0].text)
	}
}

// TestRewrap_ConsecutiveSpaces verifies that multiple consecutive spaces at a
// wrap boundary are all skipped by the offset advancement loop. "a     b" at
// width 3 breaks after "a"; the five spaces at bytes 1–5 must all be consumed,
// leaving segment 1 with rawOffset 6.
func TestRewrap_ConsecutiveSpaces(t *testing.T) {
	rawLine := "a     b" // 'a' + 5 spaces + 'b' = 7 bytes
	m := newLogModel(3, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{rawLine}})

	if len(m.visualLines) < 2 {
		t.Fatalf("expected at least 2 visual lines for %q at width 3, got %d: %v",
			rawLine, len(m.visualLines), collectTexts(m.visualLines))
	}
	if m.visualLines[0].rawOffset != 0 {
		t.Errorf("segment 0 rawOffset: want 0, got %d", m.visualLines[0].rawOffset)
	}
	if m.visualLines[1].rawOffset != 6 {
		t.Errorf("segment 1 rawOffset: want 6 (after 'a' + 5 consumed spaces), got %d",
			m.visualLines[1].rawOffset)
	}
	if m.visualLines[1].text != "b" {
		t.Errorf("segment 1 text: want %q, got %q", "b", m.visualLines[1].text)
	}
}

// TestRewrap_Width1 verifies that a 3-character line at width 1 produces three
// single-character segments (extreme narrow-width hard-wrap).
func TestRewrap_Width1(t *testing.T) {
	m := newLogModel(1, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"abc"}})

	if len(m.visualLines) != 3 {
		t.Fatalf("expected 3 visual lines for 'abc' at width 1, got %d: %v",
			len(m.visualLines), collectTexts(m.visualLines))
	}
	for i, want := range []string{"a", "b", "c"} {
		if m.visualLines[i].text != want {
			t.Errorf("segment %d text: want %q, got %q", i, want, m.visualLines[i].text)
		}
	}
}

// TestRewrap_SingleCharLine verifies that a single-character raw line produces
// exactly one visual segment with rawOffset 0.
func TestRewrap_SingleCharLine(t *testing.T) {
	m := newLogModel(80, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"x"}})

	if len(m.visualLines) != 1 {
		t.Fatalf("expected 1 visual line for 'x' at width 80, got %d", len(m.visualLines))
	}
	if m.visualLines[0].rawOffset != 0 {
		t.Errorf("rawOffset: want 0, got %d", m.visualLines[0].rawOffset)
	}
	if m.visualLines[0].text != "x" {
		t.Errorf("text: want %q, got %q", "x", m.visualLines[0].text)
	}
}

// TestRewrap_OnlySpaces verifies that a line consisting entirely of spaces does
// not panic and produces at least one visual segment.
func TestRewrap_OnlySpaces(t *testing.T) {
	m := newLogModel(3, 20)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("rewrap panicked on all-spaces input: %v", r)
		}
	}()
	m, _ = m.Update(LogLinesMsg{Lines: []string{"     "}})
	if len(m.visualLines) < 1 {
		t.Fatalf("expected at least 1 visual line for all-spaces input, got 0")
	}
}

// --- Category 3: renderContent correctness ---

// TestRenderContent_JoinsSegmentsWithNewline verifies that renderContent
// joins visual-line segments with "\n" and wraps them in logContentStyle.
func TestRenderContent_JoinsSegmentsWithNewline(t *testing.T) {
	m := newLogModel(10, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"the quick brown fox"}})

	if len(m.visualLines) < 2 {
		t.Fatalf("expected at least 2 visual lines, got %d", len(m.visualLines))
	}
	rendered := m.renderContent()
	plain := stripANSI(rendered)
	if !strings.Contains(plain, "the quick\nbrown fox") {
		t.Errorf("renderContent: want segments joined by newline, plain=%q", plain)
	}
}

// --- Category 4: ring buffer eviction + wrapping interaction ---

// TestRingEviction_RawIdx_CorrectAfterEviction verifies that visualLines rawIdx
// values are 0-based after ring buffer eviction (rewrap iterates the trimmed
// m.lines slice, so rawIdx must reflect the new slice indices, not pre-eviction
// positions).
func TestRingEviction_RawIdx_CorrectAfterEviction(t *testing.T) {
	m := newLogModel(80, 20)
	total := logRingBufferCap + 100
	lines := make([]string, total)
	for i := range lines {
		lines[i] = "line"
	}
	m, _ = m.Update(LogLinesMsg{Lines: lines})

	if len(m.lines) != logRingBufferCap {
		t.Fatalf("expected %d lines after eviction, got %d", logRingBufferCap, len(m.lines))
	}
	// All rawIdx values must be in-bounds for the trimmed slice.
	for i, vl := range m.visualLines {
		if vl.rawIdx < 0 || vl.rawIdx >= len(m.lines) {
			t.Errorf("visualLines[%d].rawIdx=%d out of range [0, %d)",
				i, vl.rawIdx, len(m.lines))
		}
	}
	if len(m.visualLines) > 0 && m.visualLines[0].rawIdx != 0 {
		t.Errorf("visualLines[0].rawIdx: want 0 after eviction, got %d", m.visualLines[0].rawIdx)
	}
}

// TestRingEviction_WrappedLines_CountCorrect verifies that after ring buffer
// eviction, the total visual line count equals 2 × len(m.lines) when every raw
// line wraps into exactly 2 segments.
func TestRingEviction_WrappedLines_CountCorrect(t *testing.T) {
	// "hello world" at width 10: "hello" (5) + space consumed + "world" (5) = 2 segments.
	m := newLogModel(10, 20)
	total := logRingBufferCap + 100
	lines := make([]string, total)
	for i := range lines {
		lines[i] = "hello world"
	}
	m, _ = m.Update(LogLinesMsg{Lines: lines})

	wantVisual := 2 * len(m.lines)
	if len(m.visualLines) != wantVisual {
		t.Errorf("expected %d visual lines after eviction (2×%d raw), got %d",
			wantVisual, len(m.lines), len(m.visualLines))
	}
}

// --- Category 5: SetSize edge cases ---

// TestSetSize_HeightOnly_NoRewrap verifies that calling SetSize with the same
// width but a different height does not change the visual line count (the
// early-return optimization in SetSize skips rewrap for height-only changes).
func TestSetSize_HeightOnly_NoRewrap(t *testing.T) {
	m := newLogModel(10, 5)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"the quick brown fox"}})
	countBefore := len(m.visualLines)

	m.SetSize(10, 10) // same width, different height

	if len(m.visualLines) != countBefore {
		t.Errorf("expected visual line count unchanged after height-only resize, before=%d after=%d",
			countBefore, len(m.visualLines))
	}
}

// TestSetSize_EmptyContent_NoSnapshotPanic verifies that SetSize on an empty
// logModel (no lines, no visualLines) does not panic. The snapshot guard
// `len(m.visualLines) > 0` must fire before any index dereference.
func TestSetSize_EmptyContent_NoSnapshotPanic(t *testing.T) {
	m := newLogModel(0, 0)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SetSize panicked on empty logModel: %v", r)
		}
	}()
	m.SetSize(80, 20)
}

// TestSetSize_YOffset_OutOfRange verifies that SetSize does not panic when
// viewport.YOffset has been set to a value >= len(visualLines). The guard
// `m.viewport.YOffset < len(m.visualLines)` in SetSize must prevent the
// out-of-bounds dereference.
func TestSetSize_YOffset_OutOfRange(t *testing.T) {
	m := newLogModel(10, 5)
	m, _ = m.Update(LogLinesMsg{Lines: []string{"hello", "world", "foo"}})

	// Force an out-of-range offset directly (bypasses viewport clamping).
	m.viewport.YOffset = len(m.visualLines) + 10

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SetSize panicked with out-of-range YOffset: %v", r)
		}
	}()
	m.SetSize(5, 5)
}

// TestSetSize_WidenRestoresScrollPosition verifies that widening the viewport
// (producing fewer visual lines per raw line) still restores scroll position to
// the same raw line. The existing tests only narrow the viewport.
func TestSetSize_WidenRestoresScrollPosition(t *testing.T) {
	// "word1 word2" wraps to 2 visual lines at width 10 but fits in 1 at width 40.
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "word1 word2"
	}
	m := newLogModel(10, 5)
	m, _ = m.Update(LogLinesMsg{Lines: lines})

	// Scroll to middle.
	mid := len(m.visualLines) / 2
	if mid >= len(m.visualLines) {
		mid = len(m.visualLines) - 1
	}
	m.viewport.SetYOffset(mid)
	if m.viewport.YOffset >= len(m.visualLines) {
		t.Skip("viewport clamped mid offset — not enough content to test")
	}
	snapRawIdx := m.visualLines[m.viewport.YOffset].rawIdx

	// Widen: fewer visual lines per raw line.
	m.SetSize(40, 5)

	if m.viewport.YOffset >= len(m.visualLines) {
		t.Fatalf("YOffset %d out of range after widen (len=%d)", m.viewport.YOffset, len(m.visualLines))
	}
	topAfter := m.visualLines[m.viewport.YOffset].rawIdx
	if topAfter != snapRawIdx {
		t.Errorf("raw line changed after widen: before rawIdx=%d, after rawIdx=%d",
			snapRawIdx, topAfter)
	}
}

// --- Category 6: auto-scroll behavior with wrapping ---

// TestAutoScroll_AtBottom_StaysAfterWrap verifies that when the viewport is at
// the bottom, adding a line that wraps into multiple segments keeps the viewport
// at the bottom (the wasAtBottom + GotoBottom() path handles the extra visual
// lines).
func TestAutoScroll_AtBottom_StaysAfterWrap(t *testing.T) {
	m := newLogModel(10, 5)
	fill := make([]string, 20)
	for i := range fill {
		fill[i] = "line"
	}
	m, _ = m.Update(LogLinesMsg{Lines: fill})
	m.viewport.GotoBottom()

	m, _ = m.Update(LogLinesMsg{Lines: []string{"the quick brown fox"}})

	if !m.viewport.AtBottom() {
		t.Error("expected viewport to stay at bottom after wrapping line added while at bottom")
	}
}

// TestAutoScroll_ScrolledUp_StaysAfterWrap verifies that adding a wrapping line
// while scrolled up does not change YOffset (auto-scroll must not fire when
// wasAtBottom was false).
func TestAutoScroll_ScrolledUp_StaysAfterWrap(t *testing.T) {
	m := newLogModel(10, 5)
	fill := make([]string, 20)
	for i := range fill {
		fill[i] = "line"
	}
	m, _ = m.Update(LogLinesMsg{Lines: fill})
	m.viewport.GotoTop()
	offsetBefore := m.viewport.YOffset

	m, _ = m.Update(LogLinesMsg{Lines: []string{"the quick brown fox"}})

	if m.viewport.YOffset != offsetBefore {
		t.Errorf("expected YOffset unchanged when scrolled up, before=%d after=%d",
			offsetBefore, m.viewport.YOffset)
	}
}

// --- Category 7: ANSI content handling ---

// TestRewrap_ANSI_EscapeSequences verifies that wrapping a line containing ANSI
// color codes does not panic and that the visible text is preserved across the
// resulting segments. ansi.Wrap is ANSI-aware; this test exercises the
// integration.
func TestRewrap_ANSI_EscapeSequences(t *testing.T) {
	rawLine := "\x1b[31mred text goes here\x1b[0m"
	m := newLogModel(10, 20)
	m, _ = m.Update(LogLinesMsg{Lines: []string{rawLine}})

	if len(m.visualLines) == 0 {
		t.Fatal("expected at least 1 visual line for ANSI-colored text")
	}
	// Collect plain text from all segments.
	var combined strings.Builder
	for _, vl := range m.visualLines {
		combined.WriteString(stripANSI(vl.text))
	}
	// The visible words must be present in the concatenated plain text.
	got := combined.String()
	if !strings.Contains(got, "red") {
		t.Errorf("expected plain text to contain 'red', got combined segments: %q", got)
	}
	if !strings.Contains(got, "here") {
		t.Errorf("expected plain text to contain 'here', got combined segments: %q", got)
	}
}

// --- Category 8: integration with Model ---

// TestModel_WindowSizeMsg_TriggersRewrap verifies the end-to-end path:
// Model.Update(tea.WindowSizeMsg) calls m.log.SetSize, which triggers rewrap.
// A line that fits at vpWidth=80 but wraps at vpWidth=40 must produce more
// visual lines after the narrowing WindowSizeMsg.
func TestModel_WindowSizeMsg_TriggersRewrap(t *testing.T) {
	m := newTestModel(t)

	// 35 'a's + space + 35 'b's = 71 chars: fits at width 80, wraps at width 40.
	line := strings.Repeat("a", 35) + " " + strings.Repeat("b", 35)

	// Size to Width=82 → vpWidth = 82-2 = 80. Send content.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 82, Height: 24})
	m = next.(Model)
	next, _ = m.Update(LogLinesMsg{Lines: []string{line}})
	m = next.(Model)
	countAt80 := len(m.log.visualLines)

	// Resize to Width=42 → vpWidth=40. WindowSizeMsg must trigger rewrap.
	next, _ = m.Update(tea.WindowSizeMsg{Width: 42, Height: 24})
	m = next.(Model)
	countAt40 := len(m.log.visualLines)

	if countAt40 <= countAt80 {
		t.Errorf("expected more visual lines after WindowSizeMsg narrows viewport: at80=%d at40=%d",
			countAt80, countAt40)
	}
}

// --- Category 9: visualLine struct invariants ---

// TestVisualLine_RawField_MatchesText verifies the invariant that raw == text
// for every visualLine produced from plain-text ring buffer content. This
// invariant is documented in the visualLine struct and must hold before the
// selection/copy ticket changes it.
func TestVisualLine_RawField_MatchesText(t *testing.T) {
	m := newLogModel(10, 20)
	lines := []string{
		"short line",
		"the quick brown fox",
		"",
		strings.Repeat("x", 25),
	}
	m, _ = m.Update(LogLinesMsg{Lines: lines})

	for i, vl := range m.visualLines {
		if vl.raw != vl.text {
			t.Errorf("visualLines[%d]: raw=%q != text=%q — invariant violated",
				i, vl.raw, vl.text)
		}
	}
}
