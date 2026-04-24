package workflowedit

import (
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// --- fake SaveFS ---

type fakeFS struct {
	writeErr error
	statErr  error
	info     fakeFileInfo
}

func (f *fakeFS) WriteAtomic(_ string, _ []byte, _ os.FileMode) error { return f.writeErr }
func (f *fakeFS) Stat(_ string) (os.FileInfo, error) {
	if f.statErr != nil {
		return nil, f.statErr
	}
	return f.info, nil
}

type fakeFileInfo struct {
	modTime time.Time
	size    int64
}

func (fi fakeFileInfo) Name() string       { return "config.json" }
func (fi fakeFileInfo) Size() int64        { return fi.size }
func (fi fakeFileInfo) Mode() os.FileMode  { return 0o600 }
func (fi fakeFileInfo) ModTime() time.Time { return fi.modTime }
func (fi fakeFileInfo) IsDir() bool        { return false }
func (fi fakeFileInfo) Sys() any           { return nil }

// --- fake EditorRunner ---

type fakeEditorRunner struct {
	runCount int
	lastPath string
}

func (f *fakeEditorRunner) Run(filePath string, _ ExecCallback) tea.Cmd {
	f.runCount++
	f.lastPath = filePath
	return nil
}

// --- model constructors ---

// newTestModel returns a Model with fake dependencies and no workflow loaded.
func newTestModel() Model {
	return New(&fakeFS{}, &fakeEditorRunner{}, "/testproject", "/testworkflow")
}

// newLoadedModel returns a Model with the given steps already loaded.
func newLoadedModel(steps ...workflowmodel.Step) Model {
	m := newTestModel()
	m.doc.Steps = steps
	m.diskDoc.Steps = make([]workflowmodel.Step, len(steps))
	copy(m.diskDoc.Steps, steps)
	m.loaded = true
	return m
}

// newLoadedModelWithWidth applies a WindowSizeMsg to a loaded model.
func newLoadedModelWithWidth(width, height int, steps ...workflowmodel.Step) Model {
	m := newLoadedModel(steps...)
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(Model)
}

// applyKey delivers a KeyMsg to the model and returns the next model.
func applyKey(m Model, km tea.KeyMsg) Model {
	next, _ := m.Update(km)
	return next.(Model)
}

// --- key helpers ---

func keyDown() tea.KeyMsg  { return tea.KeyMsg{Type: tea.KeyDown} }
func keyUp() tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyUp} }
func keyTab() tea.KeyMsg   { return tea.KeyMsg{Type: tea.KeyTab} }
func keyEsc() tea.KeyMsg   { return tea.KeyMsg{Type: tea.KeyEsc} }
func keyEnter() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }
func keyDel() tea.KeyMsg   { return tea.KeyMsg{Type: tea.KeyDelete} }

func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func keyCtrlN() tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyCtrlN} }
func keyCtrlO() tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyCtrlO} }
func keyCtrlS() tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyCtrlS} }
func keyCtrlQ() tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyCtrlQ} }
func keyF10() tea.KeyMsg      { return tea.KeyMsg{Type: tea.KeyF10} }
func keyAltUp() tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyUp, Alt: true} }
func keyShiftTab() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyShiftTab} }

// --- step factories ---

func sampleStep(name string) workflowmodel.Step {
	return workflowmodel.Step{
		Name:    name,
		Kind:    workflowmodel.StepKindShell,
		Command: []string{"echo"},
	}
}

func sampleClaudeStep(name string) workflowmodel.Step {
	return workflowmodel.Step{
		Name:        name,
		Kind:        workflowmodel.StepKindClaude,
		IsClaudeSet: true,
		Model:       "claude-sonnet-4-6",
		PromptFile:  "prompts/" + name + ".md",
	}
}
