package workflowedit

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// PickerKind discriminates the intent of a DialogPathPicker: opening an
// existing workflow (PickerKindOpen, the default) or creating a new one
// (PickerKindNew). The kind drives pre-fill, button label, and guidance text.
type PickerKind int

const (
	PickerKindOpen PickerKind = iota // Ctrl+O: browse to existing config.json
	PickerKindNew                    // File>New: choose where to create a workflow
)

// pathPickerModel holds the state for the DialogPathPicker dialog.
// Stored as dialogState.payload when DialogPathPicker is active.
// pathCompletionGen tracks the current input generation so stale async
// completion results are discarded (D-PR2-20).
type pathPickerModel struct {
	input             string     // current text in the path input field
	matches           []string   // nil = not yet scanned; empty = scanned, no results
	matchIdx          int        // cycling position within matches
	pathCompletionGen uint64     // increments on each mutating keystroke or new async dispatch
	kind              PickerKind // PickerKindOpen (default) or PickerKindNew
}

// pathCompletionMsg is dispatched when an async completePath scan finishes.
// gen must match the model's current pathCompletionGen or the message is discarded.
type pathCompletionMsg struct {
	matches []string
	err     error
	gen     uint64
}

// newPathPicker returns a PickerKindOpen pathPickerModel pre-filled with defaultPath.
func newPathPicker(defaultPath string) pathPickerModel {
	return pathPickerModel{input: defaultPath, kind: PickerKindOpen}
}

// newPathPickerForNew returns a PickerKindNew pathPickerModel whose pre-fill
// is <projectDir>/.pr9k/workflow/ (a directory, not a config.json path).
func newPathPickerForNew(projectDir string) pathPickerModel {
	dir := filepath.Join(projectDir, ".pr9k", "workflow") + "/"
	return pathPickerModel{input: dir, kind: PickerKindNew}
}

// completePath returns a tea.Cmd that scans for path completions
// asynchronously. The UI goroutine is never blocked (D-25).
func completePath(prefix string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		matches, err := scanMatches(prefix)
		return pathCompletionMsg{matches: matches, err: err, gen: gen}
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
