package ui

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// copyFn is the package-level clipboard write function. Swapped out in tests
// to capture the payload without invoking the real clipboard daemon.
//
// NOTE: copyFn, isTTYFn, and stderrWriter are not mutex-protected and must NOT
// be mutated from parallel tests. No test that calls t.Parallel() may modify
// these vars. Enforcement is via go test -race ./... in CI.
var copyFn = clipboard.WriteAll

// isTTYFn reports whether stderr is attached to a terminal. Swapped out in
// tests to force the tty and non-tty code paths without a real terminal.
var isTTYFn = func() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// stderrWriter is the writer used for the OSC 52 fallback sequence. In
// production this is os.Stderr; in tests it is redirected to a bytes.Buffer
// so the escape sequence can be captured and verified without a real terminal.
var stderrWriter io.Writer = os.Stderr

// copyToClipboard writes text to the system clipboard.
//
//  1. Calls copyFn(text). Returns nil on success (covers local macOS/Linux-with-X/Windows).
//  2. If copyFn fails AND stderr is a terminal, writes an OSC 52 escape sequence
//     to stderr so that clipboard-capable terminals (iTerm2, Kitty, Windows Terminal,
//     etc.) can deliver the content even in headless/SSH environments.
//     OSC 52 delivery is best-effort: it depends on the terminal forwarding the
//     sequence in alt-screen mode and has no acknowledgement protocol, so this
//     path always returns nil.
//  3. If copyFn fails AND stderr is NOT a terminal, returns the underlying error
//     so the caller can emit a diagnostic log line.
func copyToClipboard(text string) error {
	err := copyFn(text)
	if err == nil {
		return nil
	}
	if isTTYFn() {
		// OSC 52 fallback: write "\x1b]52;c;<base64>\x07" to stderr.
		// Terminals that support OSC 52 will inject the payload into the
		// clipboard on the client side, even over SSH.
		encoded := base64.StdEncoding.EncodeToString([]byte(text))
		if _, werr := fmt.Fprintf(stderrWriter, "\x1b]52;c;%s\x07", encoded); werr != nil {
			return werr
		}
		return nil
	}
	return err
}

// copySelectedText performs the clipboard copy for a committed selection and
// returns a tea.Cmd that appends the appropriate feedback log line. Called by
// model.go after y/Enter exits ModeSelect. text is the raw selected text
// (already extracted by model.go before ClearSelection is called).
//
// If text is empty, no copy is attempted and nil is returned (silent no-op).
// The clipboard write runs inside the returned tea.Cmd closure so it does not
// block the Bubble Tea Update goroutine.
func copySelectedText(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	return func() tea.Msg {
		err := copyToClipboard(text)
		var line string
		if err == nil {
			line = fmt.Sprintf("[copied %d chars]", len(text))
		} else {
			line = "[copy failed: install xclip/xsel or run in a terminal that supports OSC 52]"
		}
		return LogLinesMsg{Lines: []string{line}}
	}
}
