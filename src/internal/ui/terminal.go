package ui

import (
	"os"

	"golang.org/x/sys/unix"
)

// DefaultTerminalWidth is the fallback width used when the stdout terminal
// size cannot be determined (e.g. stdout is not a TTY, or the ioctl fails).
const DefaultTerminalWidth = 80

// TerminalWidth returns the current stdout terminal width in columns.
// Returns DefaultTerminalWidth when stdout is not a TTY or the size cannot
// be determined. Safe to call from any goroutine.
func TerminalWidth() int {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws == nil || ws.Col == 0 {
		return DefaultTerminalWidth
	}
	return int(ws.Col)
}
