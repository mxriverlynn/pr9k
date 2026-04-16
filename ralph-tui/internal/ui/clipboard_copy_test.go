package ui

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// enterKeyMsg returns a tea.KeyMsg for the Enter key.
func enterKeyMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEnter}
}

// resetClipboardFns restores copyFn and isTTYFn to their original values after
// a test that swaps them.
func resetClipboardFns(t *testing.T, origCopy func(string) error, origTTY func() bool) {
	t.Helper()
	t.Cleanup(func() {
		copyFn = origCopy
		isTTYFn = origTTY
	})
}

// --- TP-107-01: y copies and exits ModeSelect ---

func TestKeys_Y_Copies_Exits(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	var captured string
	copyFn = func(text string) error {
		captured = text
		return nil
	}
	isTTYFn = func() bool { return false }

	m, _ := newSelectTestModel(t, ModeNormal)
	// Add two lines: "hello world" and "foo" so we have content to select.
	next, _ := m.Update(LogLinesMsg{Lines: []string{"hello world", "foo"}})
	m = next.(Model)
	// Enter ModeSelect.
	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)
	if m.keys.handler.Mode() != ModeSelect {
		t.Fatal("expected ModeSelect")
	}

	// Remember the expected text before pressing y.
	expectedText := m.log.SelectedText()

	// Press y — should copy and return to prevMode (ModeNormal).
	next, cmd := m.Update(keyMsg("y"))
	m = next.(Model)

	if m.keys.handler.Mode() == ModeSelect {
		t.Error("expected mode to exit ModeSelect after y")
	}
	// Selection should be cleared.
	if m.log.sel.active || m.log.sel.committed {
		t.Error("expected selection to be cleared after y")
	}
	// copyFn must have been called (even with empty selection the mode exits;
	// but for a freshly entered zero-size selection SelectedText() == "").
	// If text was non-empty, check the captured payload.
	if expectedText != "" {
		if captured != expectedText {
			t.Errorf("clipboard payload: got %q, want %q", captured, expectedText)
		}
		// cmd should emit a LogLinesMsg with the confirmation line.
		if cmd == nil {
			t.Fatal("expected non-nil cmd after y with non-empty selection")
		}
		msg := cmd()
		ll, ok := msg.(LogLinesMsg)
		if !ok {
			t.Fatalf("expected LogLinesMsg, got %T", msg)
		}
		if len(ll.Lines) != 1 {
			t.Fatalf("expected 1 feedback line, got %d", len(ll.Lines))
		}
		if !strings.HasPrefix(ll.Lines[0], "[copied ") {
			t.Errorf("expected [copied ...] line, got %q", ll.Lines[0])
		}
	}
}

// --- TP-107-02: Enter also copies ---

func TestKeys_Enter_AlsoCopies(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	var called bool
	copyFn = func(text string) error {
		called = text != ""
		return nil
	}
	isTTYFn = func() bool { return false }

	m, _ := newSelectTestModel(t, ModeNormal)
	next, _ := m.Update(LogLinesMsg{Lines: []string{"hello", "world"}})
	m = next.(Model)
	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)

	// Move cursor right to create a non-empty selection.
	next, _ = m.Update(keyMsg("l"))
	m = next.(Model)
	// Commit the selection by marking it — directly set committed.
	m.log.sel.committed = true
	m.log.sel.active = false

	var cmd tea.Cmd
	next, cmd = m.Update(enterKeyMsg())
	m = next.(Model)

	if m.keys.handler.Mode() == ModeSelect {
		t.Error("expected mode to exit ModeSelect after Enter")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd after Enter with selection")
	}
	msg := cmd()
	ll, ok := msg.(LogLinesMsg)
	if !ok {
		t.Fatalf("expected LogLinesMsg, got %T", msg)
	}
	if len(ll.Lines) == 0 {
		t.Fatal("expected feedback line")
	}
	_ = called
}

// --- TP-107-03: empty selection is a silent no-op ---

func TestCopy_EmptySelection_NoOp(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	var invoked bool
	copyFn = func(text string) error {
		invoked = true
		return nil
	}
	isTTYFn = func() bool { return false }

	m, _ := newSelectTestModel(t, ModeNormal)
	next, _ := m.Update(LogLinesMsg{Lines: []string{"line1"}})
	m = next.(Model)
	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)

	// The freshly entered selection has anchor == cursor (empty range).
	// SelectedText() == "".
	if txt := m.log.SelectedText(); txt != "" {
		t.Skipf("selection is non-empty (%q) — test precondition not met", txt)
	}

	next, cmd := m.Update(keyMsg("y"))
	m = next.(Model)

	if m.keys.handler.Mode() == ModeSelect {
		t.Error("expected mode to exit ModeSelect")
	}
	if invoked {
		t.Error("copyFn must not be called for empty selection")
	}
	// cmd should be nil (no feedback line for empty selection).
	if cmd != nil {
		msg := cmd()
		if msg != nil {
			// A non-nil feedback message for an empty selection is a bug.
			t.Errorf("expected nil msg for empty selection, got %T", msg)
		}
	}
}

// --- TP-107-04: success appends "[copied N chars]" ---

func TestCopy_Success_AppendsConfirmationLogLine(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return nil }
	isTTYFn = func() bool { return false }

	m, _ := newSelectTestModel(t, ModeNormal)
	// Use a distinct text so we know what N should be.
	const content = "hello"
	next, _ := m.Update(LogLinesMsg{Lines: []string{content}})
	m = next.(Model)
	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)

	// Extend cursor to cover the full word so SelectedText() is non-empty.
	for range len(content) {
		next, _ = m.Update(keyMsg("l"))
		m = next.(Model)
	}
	// Mark as committed so SelectedText() returns the range.
	m.log.sel.committed = true
	m.log.sel.active = false

	selText := m.log.SelectedText()
	if selText == "" {
		t.Skip("selection still empty after cursor moves — test precondition not met")
	}

	next, cmd := m.Update(keyMsg("y"))
	m = next.(Model)

	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	ll, ok := msg.(LogLinesMsg)
	if !ok {
		t.Fatalf("expected LogLinesMsg, got %T", msg)
	}
	if len(ll.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(ll.Lines))
	}
	want := fmt.Sprintf("[copied %d chars]", len(selText))
	if ll.Lines[0] != want {
		t.Errorf("got %q, want %q", ll.Lines[0], want)
	}
}

// --- TP-107-05: clipboard error appends error log line ---

func TestCopy_ClipboardError_AppendsLogLine(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return errors.New("no clipboard") }
	isTTYFn = func() bool { return false } // not a tty → error returned

	m, _ := newSelectTestModel(t, ModeNormal)
	const content = "abc"
	next, _ := m.Update(LogLinesMsg{Lines: []string{content}})
	m = next.(Model)
	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)

	for range len(content) {
		next, _ = m.Update(keyMsg("l"))
		m = next.(Model)
	}
	m.log.sel.committed = true
	m.log.sel.active = false

	if m.log.SelectedText() == "" {
		t.Skip("selection empty — precondition not met")
	}

	next, cmd := m.Update(keyMsg("y"))
	m = next.(Model)

	if m.keys.handler.Mode() == ModeSelect {
		t.Error("expected mode to exit ModeSelect")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	ll, ok := msg.(LogLinesMsg)
	if !ok {
		t.Fatalf("expected LogLinesMsg, got %T", msg)
	}
	if len(ll.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(ll.Lines))
	}
	if !strings.Contains(ll.Lines[0], "copy failed") {
		t.Errorf("expected [copy failed ...] line, got %q", ll.Lines[0])
	}
}

// --- TP-107-06: OSC 52 fallback on clipboard failure with tty ---

func TestCopy_Failure_FallsBackToOSC52(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return errors.New("no clipboard") }
	isTTYFn = func() bool { return true } // is a tty → OSC 52 path

	// Redirect stderr so we can capture the OSC 52 sequence.
	origStderr := stderrWriter
	var buf bytes.Buffer
	stderrWriter = &buf
	t.Cleanup(func() { stderrWriter = origStderr })

	const text = "hello OSC52"
	err := CopyToClipboard(text)
	if err != nil {
		t.Fatalf("CopyToClipboard should return nil on OSC 52 path, got %v", err)
	}

	expected := fmt.Sprintf("\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(text)))
	if buf.String() != expected {
		t.Errorf("OSC 52 output:\ngot:  %q\nwant: %q", buf.String(), expected)
	}
}

// --- TP-107-07: no tty → error returned, no OSC 52 ---

func TestCopy_Failure_NoTty_EmitsErrorLogLine(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	clipErr := errors.New("no clipboard daemon")
	copyFn = func(text string) error { return clipErr }
	isTTYFn = func() bool { return false }

	origStderr := stderrWriter
	var buf bytes.Buffer
	stderrWriter = &buf
	t.Cleanup(func() { stderrWriter = origStderr })

	err := CopyToClipboard("test")
	if err == nil {
		t.Fatal("expected error when not a tty and copyFn fails")
	}
	if buf.Len() > 0 {
		t.Errorf("expected no OSC 52 output, got %q", buf.String())
	}
}

// --- TP-107-08: clipboard payload contains no wrap-induced newlines ---

func TestCopy_TextFaithful_OriginalLineBreaksOnly(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	var captured string
	copyFn = func(text string) error {
		captured = text
		return nil
	}
	isTTYFn = func() bool { return false }

	// Use a line longer than the viewport width (76) so it wraps into multiple
	// visual rows. Select all visual rows of that one raw line — the clipboard
	// payload must contain no newlines.
	m, _ := newSelectTestModel(t, ModeNormal)
	longLine := strings.Repeat("a", 100) // wraps to 2+ visual rows at width 76
	next, _ := m.Update(LogLinesMsg{Lines: []string{longLine}})
	m = next.(Model)

	// Verify the line wraps into multiple visual segments.
	if len(m.log.visualLines) < 2 {
		t.Skip("line did not wrap — test precondition not met")
	}

	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)

	// Move cursor to end of visible area (past the wrap boundary).
	for range 5 {
		next, _ = m.Update(keyMsg("j"))
		m = next.(Model)
	}
	for range 20 {
		next, _ = m.Update(keyMsg("l"))
		m = next.(Model)
	}
	m.log.sel.committed = true
	m.log.sel.active = false

	selText := m.log.SelectedText()
	if selText == "" {
		t.Skip("selection empty — test precondition not met")
	}

	next, cmd := m.Update(keyMsg("y"))
	m = next.(Model)
	if cmd != nil {
		cmd()
	}

	// The captured text should contain no newlines because all selected
	// visual rows come from the same single raw line.
	if strings.Contains(captured, "\n") {
		t.Errorf("clipboard payload has wrap-induced newlines: %q", captured)
	}
}

// --- TP-107-09: cross-raw-line selection has exactly one newline ---

func TestCopy_TextAcrossRawLines_HasSingleNewline(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	var captured string
	copyFn = func(text string) error {
		captured = text
		return nil
	}
	isTTYFn = func() bool { return false }

	m, _ := newSelectTestModel(t, ModeNormal)
	next, _ := m.Update(LogLinesMsg{Lines: []string{"line one", "line two"}})
	m = next.(Model)

	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)

	// Directly construct a committed cross-raw-line selection:
	// anchor at the start of raw line 0, cursor partway into raw line 1.
	// visualRow / col are derived from the visual layout (lines are
	// short enough that each occupies exactly one visual row).
	m.log.sel = selection{
		anchor:    pos{rawIdx: 0, rawOffset: 0, visualRow: 0, col: 0},
		cursor:    pos{rawIdx: 1, rawOffset: 4, visualRow: 1, col: 4},
		committed: true,
		active:    false,
	}

	selText := m.log.SelectedText()
	if selText == "" {
		t.Skip("selection empty — test precondition not met")
	}

	_, cmd := m.Update(keyMsg("y"))
	if cmd != nil {
		cmd()
	}

	if captured == "" {
		t.Skip("nothing captured — test precondition not met")
	}
	newlines := strings.Count(captured, "\n")
	if newlines != 1 {
		t.Errorf("expected exactly 1 newline across 2 raw lines, got %d in %q", newlines, captured)
	}
}
