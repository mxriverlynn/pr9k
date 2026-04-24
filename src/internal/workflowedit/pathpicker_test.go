package workflowedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTabComplete_AsyncPattern — Tab with no existing matches dispatches a
// tea.Cmd rather than computing matches synchronously in Update.
// Guards against blocking filesystem calls on the UI goroutine.
func TestTabComplete_AsyncPattern(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogPathPicker, payload: pathPickerModel{input: "/tmp/"}}
	_, cmd := m.Update(keyTab())
	if cmd == nil {
		t.Fatal("Tab on path picker with no matches should return async cmd, not nil")
	}
}

// TestTabComplete_HiddenFilesExcluded — entries whose names start with "."
// are not returned when the input prefix does not start with ".".
func TestTabComplete_HiddenFilesExcluded(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "visible.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".hidden"), 0755); err != nil {
		t.Fatal(err)
	}
	matches, err := scanMatches(dir + "/")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range matches {
		base := filepath.Base(strings.TrimSuffix(m, "/"))
		if strings.HasPrefix(base, ".") {
			t.Errorf("hidden entry %q should not appear in matches", m)
		}
	}
	if len(matches) == 0 {
		t.Error("expected at least one visible match")
	}
}

// TestTabComplete_HiddenFilesShownWhenPrefixStartsDot — entries whose names
// start with "." are included when the input prefix's basename starts with ".".
func TestTabComplete_HiddenFilesShownWhenPrefixStartsDot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".dotdir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "visible.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	// Use string concat (not filepath.Join) so the trailing "." is preserved;
	// filepath.Join would clean it away, losing the hidden-file signal.
	matches, err := scanMatches(dir + "/.")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("expected hidden entries when prefix starts with dot")
	}
	for _, m := range matches {
		base := filepath.Base(strings.TrimSuffix(m, "/"))
		if !strings.HasPrefix(base, ".") {
			t.Errorf("non-hidden entry %q should not appear when prefix starts with dot", m)
		}
	}
}

// TestTabComplete_TildeExpansion — a leading "~" is expanded to $HOME before
// the filesystem scan, so matches are returned as absolute paths.
func TestTabComplete_TildeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "foobar"), 0755); err != nil {
		t.Fatal(err)
	}
	matches, err := scanMatches("~/foo")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "foobar") + "/"
	for _, m := range matches {
		if m == want {
			return
		}
	}
	t.Errorf("want %q in matches, got %v", want, matches)
}

// TestTabComplete_CyclingForward — repeated Tab presses cycle through the
// existing match list in forward order without dispatching a new cmd.
func TestTabComplete_CyclingForward(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogPathPicker, payload: pathPickerModel{
		input:    "/tmp/foo1",
		matches:  []string{"/tmp/foo1", "/tmp/foo2", "/tmp/foo3"},
		matchIdx: 0,
	}}

	got1, cmd := m.Update(keyTab())
	if cmd != nil {
		t.Error("Tab with existing matches should not dispatch cmd")
	}
	p1 := got1.(Model).dialog.payload.(pathPickerModel)
	if p1.input != "/tmp/foo2" {
		t.Errorf("want /tmp/foo2 after first Tab, got %q", p1.input)
	}

	got2, _ := got1.(Model).Update(keyTab())
	p2 := got2.(Model).dialog.payload.(pathPickerModel)
	if p2.input != "/tmp/foo3" {
		t.Errorf("want /tmp/foo3 after second Tab, got %q", p2.input)
	}
}

// TestTabComplete_CyclingBackward — Shift+Tab cycles backward (wrapping) without
// dispatching a new cmd.
func TestTabComplete_CyclingBackward(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogPathPicker, payload: pathPickerModel{
		input:    "/tmp/foo1",
		matches:  []string{"/tmp/foo1", "/tmp/foo2", "/tmp/foo3"},
		matchIdx: 0,
	}}
	got, cmd := m.Update(keyShiftTab())
	if cmd != nil {
		t.Error("Shift+Tab with existing matches should not dispatch cmd")
	}
	p := got.(Model).dialog.payload.(pathPickerModel)
	if p.input != "/tmp/foo3" {
		t.Errorf("want /tmp/foo3 (wrap-around), got %q", p.input)
	}
}

// TestTabComplete_EmptyMatch_NoChange — when pathCompletionMsg carries no
// matches, the input field stays unchanged.
func TestTabComplete_EmptyMatch_NoChange(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogPathPicker, payload: pathPickerModel{input: "/tmp/noexist"}}
	got := applyMsg(m, pathCompletionMsg{matches: []string{}})
	p := got.dialog.payload.(pathPickerModel)
	if p.input != "/tmp/noexist" {
		t.Errorf("input should be unchanged for empty matches, got %q", p.input)
	}
}

// TestTabComplete_SingleMatch_FillsCompletely — a single-match completion
// result fills the input completely.
func TestTabComplete_SingleMatch_FillsCompletely(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogPathPicker, payload: pathPickerModel{input: "/tmp/fo"}}
	got := applyMsg(m, pathCompletionMsg{matches: []string{"/tmp/foobar"}})
	p := got.dialog.payload.(pathPickerModel)
	if p.input != "/tmp/foobar" {
		t.Errorf("want /tmp/foobar, got %q", p.input)
	}
}
