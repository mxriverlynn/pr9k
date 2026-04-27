package workflowedit

import (
	"io"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// --- validate function helpers ---

// noFindings is a validateFn that returns zero findings (validation passes).
var noFindings = func(_ workflowmodel.WorkflowDoc, _ string, _ map[string][]byte) []findingResult {
	return nil
}

// fatalFindings is a validateFn that returns one fatal finding.
var fatalFindings = func(_ workflowmodel.WorkflowDoc, _ string, _ map[string][]byte) []findingResult {
	return []findingResult{{text: "config error: schema: step model is required", isFatal: true}}
}

// warnFindings is a validateFn that returns one non-fatal warning.
var warnFindings = func(_ workflowmodel.WorkflowDoc, _ string, _ map[string][]byte) []findingResult {
	return []findingResult{{text: "config warning: schema: captureAs has no consumer", isFatal: false}}
}

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

// fakeEditorRunner records invocations and optionally invokes the callback.
// If invokeCallback is true, Run immediately invokes cb(nil) and returns a
// tea.Cmd whose result is the return value of cb.
type fakeEditorRunner struct {
	runCount       int
	lastPath       string
	invokeCallback bool // when true, Run calls cb(nil) and wraps it in a Cmd
}

func (f *fakeEditorRunner) Run(filePath string, cb ExecCallback) tea.Cmd {
	f.runCount++
	f.lastPath = filePath
	if f.invokeCallback {
		return func() tea.Msg { return cb(nil) }
	}
	return nil
}

// --- model constructors ---

// newTestModel returns a Model with fake dependencies and no workflow loaded.
func newTestModel() Model {
	return New(&fakeFS{}, &fakeEditorRunner{}, "/testproject", "/testworkflow")
}

// newTestModelWithLog returns a Model with fake dependencies and the given writer
// as the session-event log destination.
func newTestModelWithLog(w io.Writer) Model {
	return newTestModel().WithLog(w)
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

// applyMsg delivers an arbitrary message to the model and returns the next model.
func applyMsg(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
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
func keyCtrlE() tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyCtrlE} }
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
