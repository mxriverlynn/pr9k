package ui

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"

	"github.com/atotto/clipboard"
	"golang.org/x/term"
)

// copyFn is the package-level clipboard write function. Swapped out in tests
// to capture the payload without invoking the real clipboard daemon.
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

// CopyToClipboard writes text to the system clipboard.
//
//  1. Calls copyFn(text). Returns nil on success (covers local macOS/Linux-with-X/Windows).
//  2. If copyFn fails AND stderr is a terminal, writes an OSC 52 escape sequence
//     to stderr so that clipboard-capable terminals (iTerm2, Kitty, Windows Terminal,
//     etc.) can deliver the content even in headless/SSH environments.
//     OSC 52 has no acknowledgement protocol, so this path always returns nil
//     (best-effort delivery).
//  3. If copyFn fails AND stderr is NOT a terminal, returns the underlying error
//     so the caller can emit a diagnostic log line.
func CopyToClipboard(text string) error {
	err := copyFn(text)
	if err == nil {
		return nil
	}
	if isTTYFn() {
		// OSC 52 fallback: write "\x1b]52;c;<base64>\x07" to stderr.
		// Terminals that support OSC 52 will inject the payload into the
		// clipboard on the client side, even over SSH.
		encoded := base64.StdEncoding.EncodeToString([]byte(text))
		fmt.Fprintf(stderrWriter, "\x1b]52;c;%s\x07", encoded)
		return nil
	}
	return err
}
