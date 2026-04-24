package workflowedit

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// pathPickerModel holds the state for the DialogPathPicker dialog.
// Stored as dialogState.payload when DialogPathPicker is active.
type pathPickerModel struct {
	input    string   // current text in the path input field
	matches  []string // nil = not yet scanned; empty = scanned, no results
	matchIdx int      // cycling position within matches
}

// pathCompletionMsg is dispatched when an async completePath scan finishes.
type pathCompletionMsg struct {
	matches []string
	err     error
}

// newPathPicker returns a pathPickerModel pre-filled with defaultPath.
func newPathPicker(defaultPath string) pathPickerModel {
	return pathPickerModel{input: defaultPath}
}

// completePath returns a tea.Cmd that scans for path completions
// asynchronously. The UI goroutine is never blocked (D-25).
func completePath(prefix string) tea.Cmd {
	return func() tea.Msg {
		matches, err := scanMatches(prefix)
		return pathCompletionMsg{matches: matches, err: err}
	}
}

// scanMatches returns filesystem entries whose names start with the basename
// of prefix. A leading "~" is expanded via os.UserHomeDir. Hidden entries
// (names starting with ".") are excluded unless prefix's basename itself
// starts with ".". Results are sorted alphabetically; directories get a
// trailing "/" appended.
func scanMatches(prefix string) ([]string, error) {
	// ~ expansion
	if prefix == "~" || strings.HasPrefix(prefix, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		if prefix == "~" {
			prefix = home + "/"
		} else {
			prefix = home + prefix[1:]
		}
	}

	var dir, base string
	if strings.HasSuffix(prefix, "/") {
		dir = prefix
		base = ""
	} else {
		dir = filepath.Dir(prefix)
		base = filepath.Base(prefix)
	}

	showHidden := strings.HasPrefix(base, ".")

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var matches []string
	for _, e := range entries {
		name := e.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasPrefix(name, base) {
			continue
		}
		candidate := filepath.Join(dir, name)
		if e.IsDir() {
			candidate += "/"
		}
		matches = append(matches, candidate)
	}
	sort.Strings(matches)
	return matches, nil
}
