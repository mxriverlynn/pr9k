package ui

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// Category 1: CopyToClipboard unit tests
// =============================================================================

// C1-01: CopyToClipboard with empty string succeeds (copyFn returns nil).
func TestCopyToClipboard_EmptyString_Succeeds(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	var invoked bool
	copyFn = func(text string) error {
		invoked = true
		return nil
	}
	isTTYFn = func() bool { return false }

	err := copyToClipboard("")
	if err != nil {
		t.Fatalf("expected nil error for empty string, got %v", err)
	}
	if !invoked {
		t.Error("copyFn should be called even for empty string")
	}
}

// C1-02: CopyToClipboard with multi-byte UTF-8 text — base64 encoding in OSC 52
// path is correct.
func TestCopyToClipboard_MultiByteUTF8_OSC52Base64Correct(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return errors.New("no clipboard") }
	isTTYFn = func() bool { return true }

	origStderr := stderrWriter
	var buf bytes.Buffer
	stderrWriter = &buf
	t.Cleanup(func() { stderrWriter = origStderr })

	const text = "こんにちは" // 5 runes, 15 bytes
	err := copyToClipboard(text)
	if err != nil {
		t.Fatalf("expected nil error on OSC 52 path, got %v", err)
	}

	expected := fmt.Sprintf("\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(text)))
	if buf.String() != expected {
		t.Errorf("OSC 52 output:\ngot:  %q\nwant: %q", buf.String(), expected)
	}
}

// C1-03: CopyToClipboard success path does not write to stderrWriter.
func TestCopyToClipboard_Success_NoStderrOutput(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return nil }
	isTTYFn = func() bool { return false }

	origStderr := stderrWriter
	var buf bytes.Buffer
	stderrWriter = &buf
	t.Cleanup(func() { stderrWriter = origStderr })

	err := copyToClipboard("hello")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if buf.Len() > 0 {
		t.Errorf("expected no stderr output on success path, got %q", buf.String())
	}
}

// C1-04: CopyToClipboard with very large text (>64 KB) — OSC 52 fallback writes
// the full base64-encoded payload without truncation.
func TestCopyToClipboard_LargeText_OSC52WritesFullPayload(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return errors.New("no clipboard") }
	isTTYFn = func() bool { return true }

	origStderr := stderrWriter
	var buf bytes.Buffer
	stderrWriter = &buf
	t.Cleanup(func() { stderrWriter = origStderr })

	text := strings.Repeat("A", 70*1024) // 70 KB
	err := copyToClipboard(text)
	if err != nil {
		t.Fatalf("expected nil error on OSC 52 path, got %v", err)
	}

	expected := fmt.Sprintf("\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(text)))
	if buf.String() != expected {
		t.Errorf("OSC 52 output length: got %d bytes, want %d bytes", len(buf.String()), len(expected))
	}
}

// =============================================================================
// Category 2: copySelectedText helper
// =============================================================================

// C2-01: copySelectedText("") returns nil cmd directly.
func TestCopySelectedText_Empty_ReturnsNilCmd(t *testing.T) {
	cmd := copySelectedText("")
	if cmd != nil {
		t.Errorf("expected nil cmd for empty string, got non-nil")
	}
}

// C2-02: copySelectedText with successful copy — returned cmd produces
// LogLinesMsg with "[copied N chars]".
func TestCopySelectedText_Success_ReturnsConfirmationMsg(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return nil }
	isTTYFn = func() bool { return false }

	const text = "hello world"
	cmd := copySelectedText(text)
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
	want := fmt.Sprintf("[copied %d chars]", len(text))
	if ll.Lines[0] != want {
		t.Errorf("got %q, want %q", ll.Lines[0], want)
	}
}

// C2-03: copySelectedText with failing copy — returned cmd produces LogLinesMsg
// with "[copy failed: ...]".
func TestCopySelectedText_Failure_ReturnsErrorMsg(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return errors.New("no clipboard") }
	isTTYFn = func() bool { return false }

	cmd := copySelectedText("some text")
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

// C2-04: copySelectedText must not call copyFn before cmd() is invoked —
// the clipboard write must run inside the tea.Cmd closure (M1 async fix).
func TestCopySelectedText_CopyFn_NotCalledBeforeCmdInvocation(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	var invoked bool
	copyFn = func(text string) error {
		invoked = true
		return nil
	}
	isTTYFn = func() bool { return false }

	cmd := copySelectedText("hello")
	if invoked {
		t.Error("copyFn must not be called before cmd() is invoked (clipboard write must be async)")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for non-empty text")
	}
	// Only after invoking cmd() should the copy happen.
	cmd()
	if !invoked {
		t.Error("copyFn must be called when cmd() is invoked")
	}
}

// =============================================================================
// Category 3: model.go routing edge cases
// =============================================================================

// C3-01: y from ModeSelect when SelectedText() returns multi-line text —
// clipboard payload is correct and contains exactly one newline.
func TestKeys_Y_MultiLineSelection_ClipboardPayloadCorrect(t *testing.T) {
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

	// Directly construct a committed cross-raw-line selection.
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
	if !strings.Contains(selText, "\n") {
		t.Skip("selection does not span multiple lines — test precondition not met")
	}

	_, cmd := m.Update(keyMsg("y"))
	if cmd != nil {
		cmd()
	}

	if captured != selText {
		t.Errorf("clipboard payload: got %q, want %q", captured, selText)
	}
	if !strings.Contains(captured, "\n") {
		t.Errorf("expected newline in multi-line clipboard payload, got %q", captured)
	}
}

// C3-02: y followed immediately by another y — second y is a no-op because the
// model is no longer in ModeSelect.
func TestKeys_Y_DoubleY_SecondIsNoOp(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	var copyCount int
	copyFn = func(text string) error {
		copyCount++
		return nil
	}
	isTTYFn = func() bool { return false }

	m, _ := newSelectTestModel(t, ModeNormal)
	next, _ := m.Update(LogLinesMsg{Lines: []string{"hello"}})
	m = next.(Model)
	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)

	// Move cursor to create a non-empty selection.
	for range 3 {
		next, _ = m.Update(keyMsg("l"))
		m = next.(Model)
	}
	m.log.sel.committed = true
	m.log.sel.active = false

	// First y — exits ModeSelect, may copy (if selection is non-empty).
	next, cmd1 := m.Update(keyMsg("y"))
	m = next.(Model)
	if cmd1 != nil {
		cmd1()
	}

	if m.keys.handler.Mode() == ModeSelect {
		t.Fatal("expected to exit ModeSelect after first y")
	}
	copyCountAfterFirst := copyCount
	modeAfterFirst := m.keys.handler.Mode()

	// Second y — in ModeNormal, y is unmapped; must be a no-op.
	next, cmd2 := m.Update(keyMsg("y"))
	m = next.(Model)
	if cmd2 != nil {
		cmd2()
	}

	if copyCount != copyCountAfterFirst {
		t.Errorf("second y triggered copy: copyCount went from %d to %d",
			copyCountAfterFirst, copyCount)
	}
	if m.keys.handler.Mode() != modeAfterFirst {
		t.Errorf("second y changed mode from %v to %v", modeAfterFirst, m.keys.handler.Mode())
	}
}

// C3-03: Enter from ModeSelect entered via Done mode — prevMode restored to
// Done, not Normal.
func TestKeys_Enter_FromDoneMode_RestoresDone(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return nil }
	isTTYFn = func() bool { return false }

	m, _ := newSelectTestModel(t, ModeDone)
	next, _ := m.Update(LogLinesMsg{Lines: []string{"done output"}})
	m = next.(Model)

	// Enter ModeSelect from ModeDone by pressing v.
	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)
	if m.keys.handler.Mode() != ModeSelect {
		t.Fatal("expected ModeSelect after v from ModeDone")
	}

	// Press Enter to copy and exit.
	next, _ = m.Update(enterKeyMsg())
	m = next.(Model)

	if m.keys.handler.Mode() != ModeDone {
		t.Errorf("expected ModeDone after Enter exits ModeSelect, got %v",
			m.keys.handler.Mode())
	}
}

// C3-04: y with selection that spans a single character — feedback reads
// "[copied 1 chars]".
func TestKeys_Y_SingleCharSelection_FeedbackCorrect(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return nil }
	isTTYFn = func() bool { return false }

	m, _ := newSelectTestModel(t, ModeNormal)
	next, _ := m.Update(LogLinesMsg{Lines: []string{"abc"}})
	m = next.(Model)
	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)

	// Move cursor one position to create a single-character selection.
	next, _ = m.Update(keyMsg("l"))
	m = next.(Model)
	m.log.sel.committed = true
	m.log.sel.active = false

	selText := m.log.SelectedText()
	if len(selText) != 1 {
		t.Skipf("selection is %q (len %d), not a single char — precondition not met",
			selText, len(selText))
	}

	_, cmd := m.Update(keyMsg("y"))
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	ll, ok := msg.(LogLinesMsg)
	if !ok {
		t.Fatalf("expected LogLinesMsg, got %T", msg)
	}
	if len(ll.Lines) != 1 {
		t.Fatalf("expected 1 feedback line, got %d", len(ll.Lines))
	}
	want := fmt.Sprintf("[copied %d chars]", len(selText))
	if ll.Lines[0] != want {
		t.Errorf("got %q, want %q", ll.Lines[0], want)
	}
}

// =============================================================================
// Category 4: handleSelect key dispatch
// =============================================================================

// C4-01: handleSelect y key does NOT call copyFn directly — only transitions
// the mode. The clipboard write is performed by model.go's routing block, which
// has access to logModel.SelectedText().
func TestHandleSelect_Y_DoesNotCallCopyFn(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	var invoked bool
	copyFn = func(text string) error {
		invoked = true
		return nil
	}
	isTTYFn = func() bool { return false }

	m, _ := newSelectTestModel(t, ModeNormal)
	next, _ := m.Update(LogLinesMsg{Lines: []string{"hello"}})
	m = next.(Model)
	// Enter ModeSelect via the normal flow.
	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)
	if m.keys.handler.Mode() != ModeSelect {
		t.Fatal("expected ModeSelect")
	}

	// Call keysModel.Update directly, bypassing model.go's routing block.
	// handleSelect should only transition the mode — not call copyFn.
	_, _ = m.keys.Update(keyMsg("y"))

	if invoked {
		t.Error("handleSelect must not call copyFn directly; copy is handled by model.go")
	}
	// Mode should have transitioned away from ModeSelect.
	if m.keys.handler.Mode() == ModeSelect {
		t.Error("expected mode to exit ModeSelect after y via handleSelect")
	}
}

// C4-02: handleSelect enter key restores prevMode, not hardcoded ModeNormal.
// When ModeSelect was entered from ModeDone, Enter must restore ModeDone.
func TestHandleSelect_Enter_RestoresPrevMode(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return nil }
	isTTYFn = func() bool { return false }

	m, _ := newSelectTestModel(t, ModeDone)
	next, _ := m.Update(LogLinesMsg{Lines: []string{"done output"}})
	m = next.(Model)
	// Enter ModeSelect from ModeDone.
	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)
	if m.keys.handler.Mode() != ModeSelect {
		t.Fatal("expected ModeSelect after v from ModeDone")
	}

	// Call handleSelect via keysModel.Update with Enter.
	_, _ = m.keys.Update(enterKeyMsg())

	// prevMode was ModeDone — handleSelect must restore it.
	if m.keys.handler.Mode() != ModeDone {
		t.Errorf("expected ModeDone after Enter via handleSelect, got %v",
			m.keys.handler.Mode())
	}
}

// C4-03: After y exits ModeSelect, the shortcut footer updates to prevMode's
// shortcuts — verifies updateShortcutLineLocked is called on mode transition.
func TestHandleSelect_Y_ShortcutFooterUpdates(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return nil }
	isTTYFn = func() bool { return false }

	m, _ := newSelectTestModel(t, ModeNormal)
	next, _ := m.Update(LogLinesMsg{Lines: []string{"hello"}})
	m = next.(Model)
	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)

	selectShortcut := m.keys.handler.ShortcutLine()

	// Press y — exits ModeSelect.
	next, _ = m.Update(keyMsg("y"))
	m = next.(Model)

	normalShortcut := m.keys.handler.ShortcutLine()

	if m.keys.handler.Mode() == ModeSelect {
		t.Error("expected to exit ModeSelect after y")
	}
	// ShortcutLine must change when the mode changes.
	if selectShortcut == normalShortcut {
		t.Errorf("shortcut line did not change after exiting ModeSelect; got %q for both modes",
			normalShortcut)
	}
}

// =============================================================================
// Category 5: test seam safety
// =============================================================================

// C5-01: resetClipboardFns restores the original copyFn after the test.
func TestResetClipboardFns_RestoresCopyFn(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	// Outer cleanup so the real clipboard.WriteAll is always restored.
	t.Cleanup(func() {
		copyFn = origCopy
		isTTYFn = origTTY
	})

	var sentinelACalled, sentinelBCalled bool
	sentinelA := func(_ string) error { sentinelACalled = true; return nil }
	sentinelB := func(_ string) error { sentinelBCalled = true; return nil }

	// Install sentinelA as the "original" that resetClipboardFns will restore.
	copyFn = sentinelA

	t.Run("inner", func(inner *testing.T) {
		// Schedule restoration of sentinelA via resetClipboardFns.
		resetClipboardFns(inner, sentinelA, origTTY)
		// Replace with sentinelB inside the sub-test.
		copyFn = sentinelB
	})
	// Sub-test has completed; t.Cleanup ran and should have restored sentinelA.
	_ = copyFn("probe")
	if !sentinelACalled {
		t.Error("resetClipboardFns did not restore copyFn: sentinelA not invoked after cleanup")
	}
	if sentinelBCalled {
		t.Error("sentinelB still in place after cleanup — copyFn was not restored")
	}
}

// C5-02: stderrWriter is restored after a test that redirects it via
// t.Cleanup (the pattern used by TP-107-06 and TP-107-07).
func TestResetClipboardFns_RestoresStderrWriter(t *testing.T) {
	origStderr := stderrWriter

	t.Run("redirect-and-cleanup", func(inner *testing.T) {
		var buf bytes.Buffer
		stderrWriter = &buf
		inner.Cleanup(func() { stderrWriter = origStderr })

		// Verify the redirect is in effect inside the sub-test.
		fmt.Fprint(stderrWriter, "probe")
		if buf.String() != "probe" {
			inner.Error("stderr redirect not working inside sub-test")
		}
	})

	// After the sub-test, stderrWriter must be the original.
	if stderrWriter != origStderr {
		t.Error("stderrWriter not restored after sub-test cleanup")
	}
}

// C5-03: Audit — package-level vars (copyFn, isTTYFn, stderrWriter) are not
// mutex-protected and must not be mutated from parallel tests.
//
// This test documents the constraint. The real enforcement is via `go test
// -race ./...` in CI: any test that calls t.Parallel() and swaps these vars
// will trigger the race detector.
func TestClipboardSeams_NoParallelMutation(t *testing.T) {
	// Enforcement: code review + CI race detector (-race flag).
	// clipboard_copy_test.go must not call t.Parallel() in any test that
	// swaps copyFn, isTTYFn, or stderrWriter.
}

// =============================================================================
// Category 6: feedback LogLinesMsg integration
// =============================================================================

// C6-01: Feedback "[copied N chars]" line appears in the log viewport after the
// cmd from model.Update is executed and the resulting LogLinesMsg is fed back
// through Update.
func TestFeedback_CopiedLine_AppearsInLogViewport(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return nil }
	isTTYFn = func() bool { return false }

	m, _ := newSelectTestModel(t, ModeNormal)
	next, _ := m.Update(LogLinesMsg{Lines: []string{"hello"}})
	m = next.(Model)
	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)

	for range 3 {
		next, _ = m.Update(keyMsg("l"))
		m = next.(Model)
	}
	m.log.sel.committed = true
	m.log.sel.active = false

	selText := m.log.SelectedText()
	if selText == "" {
		t.Skip("selection empty — precondition not met")
	}

	next, cmd := m.Update(keyMsg("y"))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected non-nil cmd after y with non-empty selection")
	}

	// Execute the cmd to obtain the LogLinesMsg, then feed it back.
	feedbackMsg := cmd()
	ll, ok := feedbackMsg.(LogLinesMsg)
	if !ok {
		t.Fatalf("expected LogLinesMsg from cmd, got %T", feedbackMsg)
	}
	next, _ = m.Update(ll)
	m = next.(Model)

	want := fmt.Sprintf("[copied %d chars]", len(selText))
	if !strings.Contains(m.log.View(), want) {
		t.Errorf("feedback line %q not found in log viewport:\n%s", want, m.log.View())
	}
}

// C6-02: Feedback "[copy failed: ...]" line appears in the log viewport after
// the cmd is executed and the resulting LogLinesMsg is fed back through Update.
func TestFeedback_CopyFailedLine_AppearsInLogViewport(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return errors.New("no clipboard") }
	isTTYFn = func() bool { return false }

	m, _ := newSelectTestModel(t, ModeNormal)
	next, _ := m.Update(LogLinesMsg{Lines: []string{"fail test"}})
	m = next.(Model)
	next, _ = m.Update(keyMsg("v"))
	m = next.(Model)

	for range 3 {
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
	if cmd == nil {
		t.Fatal("expected non-nil cmd after y with non-empty selection")
	}

	feedbackMsg := cmd()
	ll, ok := feedbackMsg.(LogLinesMsg)
	if !ok {
		t.Fatalf("expected LogLinesMsg from cmd, got %T", feedbackMsg)
	}
	next, _ = m.Update(ll)
	m = next.(Model)

	if !strings.Contains(m.log.View(), "copy failed") {
		t.Errorf("feedback line 'copy failed' not found in log viewport:\n%s", m.log.View())
	}
}

// C6-03: The N in "[copied N chars]" counts bytes, not runes.
// Go's len(string) counts UTF-8 bytes; for multi-byte text such as CJK
// characters the byte count will exceed the visible glyph count.
// This test documents and verifies the current behavior.
func TestFeedback_CopiedNChars_CountsBytes(t *testing.T) {
	origCopy, origTTY := copyFn, isTTYFn
	resetClipboardFns(t, origCopy, origTTY)

	copyFn = func(text string) error { return nil }
	isTTYFn = func() bool { return false }

	// "日本語" = 3 runes, 9 bytes.
	const text = "日本語"
	byteCount := len(text)         // 9
	runeCount := len([]rune(text)) // 3
	if byteCount == runeCount {
		t.Skip("test text has equal byte and rune count — cannot distinguish the two")
	}

	cmd := copySelectedText(text)
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

	wantBytes := fmt.Sprintf("[copied %d chars]", byteCount) // current behavior
	wantRunes := fmt.Sprintf("[copied %d chars]", runeCount) // hypothetical rune-count behavior

	got := ll.Lines[0]
	if got == wantRunes && got != wantBytes {
		// Implementation was changed to use rune count. Update P1 in test-plan.md.
		t.Logf("NOTE: implementation now uses rune count: %q", got)
	}
	if got != wantBytes {
		t.Errorf("got %q, want byte-count form %q", got, wantBytes)
	}
}
