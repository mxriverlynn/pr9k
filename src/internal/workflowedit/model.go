package workflowedit

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
	"github.com/mxriverlynn/pr9k/src/internal/workflowvalidate"
)

// focusTarget identifies which widget holds keyboard focus.
type focusTarget int

const (
	focusOutline focusTarget = iota
	focusDetail
	focusMenu
)

// Model is the top-level Bubble Tea model for the workflow-builder TUI.
// SaveFS and EditorRunner are injected at construction time so tests can
// substitute doubles without spawning real processes or touching disk.
type Model struct {
	saveFS workflowio.SaveFS
	editor EditorRunner

	projectDir  string
	workflowDir string

	doc        workflowmodel.WorkflowDoc
	diskDoc    workflowmodel.WorkflowDoc
	companions map[string][]byte
	loaded     bool
	dirty      bool

	focus     focusTarget
	prevFocus focusTarget

	dialog   dialogState
	helpOpen bool

	saveInProgress     bool
	validateInProgress bool
	pendingQuit        bool

	outline outlinePanel
	detail  detailPane
	menu    menuBar

	width  int
	height int

	reorderMode     bool
	reorderOrigin   int
	reorderSnapshot []workflowmodel.Step

	saveBanner string // "Saved at HH:MM:SS" after a successful save

	// validateFn overrides the real validator when non-nil; used by tests.
	validateFn func(workflowmodel.WorkflowDoc, string, map[string][]byte) (bool, any)
}

// New constructs a workflow-builder Model with the provided dependency
// injections.
func New(saveFS workflowio.SaveFS, editor EditorRunner, projectDir, workflowDir string) Model {
	const defaultOutlineW, defaultDetailW, defaultH = 40, 60, 20
	return Model{
		saveFS:      saveFS,
		editor:      editor,
		projectDir:  projectDir,
		workflowDir: workflowDir,
		outline:     newOutlinePanel(defaultOutlineW, defaultH),
		detail:      newDetailPane(defaultDetailW, defaultH),
		menu:        newMenuBar(),
	}
}

// Init satisfies tea.Model. No startup commands are needed.
func (m Model) Init() tea.Cmd { return nil }

// Update satisfies tea.Model. The routing order (D-9) is:
//  1. helpOpen  → updateHelpModal
//  2. dialog    → updateDialog
//  3. globalKey → handleGlobalKey
//  4. default   → updateEditView
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch {
	case m.helpOpen:
		return m.updateHelpModal(msg)
	case m.dialog.kind != DialogNone:
		return m.updateDialog(msg)
	case isGlobalKey(msg):
		return m.handleGlobalKey(msg)
	default:
		return m.updateEditView(msg)
	}
}

// isGlobalKey reports whether msg is one of the five global shortcuts:
// F10, Ctrl+N, Ctrl+O, Ctrl+S, Ctrl+Q. Del is intentionally absent (D-10).
func isGlobalKey(msg tea.Msg) bool {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return false
	}
	switch km.Type {
	case tea.KeyF10, tea.KeyCtrlN, tea.KeyCtrlO, tea.KeyCtrlS, tea.KeyCtrlQ:
		return true
	}
	return false
}

// View satisfies tea.Model and returns the full TUI string.
func (m Model) View() string {
	var sb strings.Builder
	sb.WriteString(m.menu.render())
	sb.WriteString("\n")
	if m.helpOpen {
		sb.WriteString(m.renderHelpModal())
	} else if m.dialog.kind != DialogNone {
		sb.WriteString(m.renderDialog())
	} else if !m.loaded {
		sb.WriteString(m.renderEmptyEditor())
	} else {
		sb.WriteString(m.renderEditView())
	}
	sb.WriteString("\n")
	if m.saveBanner != "" {
		sb.WriteString(m.saveBanner)
		sb.WriteString("\n")
	}
	sb.WriteString(m.ShortcutLine())
	return sb.String()
}

// --- rendering helpers ---

func (m Model) renderHelpModal() string {
	return "Help: Ctrl+N new  Ctrl+O open  Ctrl+S save  Ctrl+Q quit  ?  close help"
}

func (m Model) renderDialog() string {
	switch m.dialog.kind {
	case DialogNewChoice:
		return "New Workflow: Copy / Empty / Cancel"
	case DialogPathPicker:
		path, _ := m.dialog.payload.(string)
		return "Open: " + path
	case DialogUnsavedChanges:
		return "Unsaved changes: Save / Cancel / Discard"
	case DialogQuitConfirm:
		return "Quit? Yes / No"
	case DialogFindingsPanel:
		return "Findings: validation errors found"
	case DialogSaveInProgress:
		return "Save in progress — please wait"
	case DialogRemoveConfirm:
		name, _ := m.dialog.payload.(string)
		return fmt.Sprintf("Delete step %q? Delete / Cancel", name)
	case DialogRecovery:
		raw, _ := m.dialog.payload.(string)
		return "Recovery: " + raw
	case DialogError:
		msg, _ := m.dialog.payload.(string)
		return "Error: " + msg
	case DialogFileConflict:
		return "File conflict: overwrite / reload / cancel"
	case DialogCrashTempNotice:
		return "Crash temp files detected"
	case DialogFirstSaveConfirm:
		return "First save: confirm"
	case DialogExternalEditorOpening:
		return "Opening external editor…"
	default:
		return ""
	}
}

func (m Model) renderEmptyEditor() string {
	return HintEmpty
}

func (m Model) renderEditView() string {
	outlineStr := m.outline.render(m.doc.Steps, m.outline.cursor, m.reorderMode)
	var detailStr string
	if len(m.doc.Steps) > 0 && m.outline.cursor < len(m.doc.Steps) {
		detailStr = m.detail.render(m.doc.Steps[m.outline.cursor])
	}
	return outlineStr + " | " + detailStr
}

// --- update helpers ---

func (m Model) updateHelpModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case km.Type == tea.KeyEsc, string(km.Runes) == "?":
		m.helpOpen = false
	}
	return m, nil
}

func (m Model) updateDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.dialog.kind {
	case DialogNewChoice:
		return m.updateDialogNewChoice(msg)
	case DialogPathPicker:
		return m.updateDialogPathPicker(msg)
	case DialogRemoveConfirm:
		return m.updateDialogRemoveConfirm(msg)
	case DialogFindingsPanel:
		return m.updateDialogFindings(msg)
	case DialogQuitConfirm:
		return m.updateDialogQuitConfirm(msg)
	case DialogUnsavedChanges:
		return m.updateDialogUnsavedChanges(msg)
	case DialogSaveInProgress:
		return m.updateDialogSaveInProgress(msg)
	case DialogRecovery:
		return m.updateDialogRecovery(msg)
	default:
		km, ok := msg.(tea.KeyMsg)
		if ok && km.Type == tea.KeyEsc {
			m.dialog = dialogState{}
			m.focus = m.prevFocus
		}
		return m, nil
	}
}

func (m Model) updateDialogNewChoice(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if km.Type == tea.KeyEsc {
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	}
	return m, nil
}

func (m Model) updateDialogPathPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if km.Type == tea.KeyEsc {
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	}
	return m, nil
}

func (m Model) updateDialogRemoveConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case km.Type == tea.KeyEsc, string(km.Runes) == "c":
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	case km.Type == tea.KeyEnter, string(km.Runes) == "d":
		idx := m.outline.cursor
		if idx >= 0 && idx < len(m.doc.Steps) {
			steps := make([]workflowmodel.Step, 0, len(m.doc.Steps)-1)
			steps = append(steps, m.doc.Steps[:idx]...)
			steps = append(steps, m.doc.Steps[idx+1:]...)
			m.doc.Steps = steps
			if m.outline.cursor >= len(m.doc.Steps) && m.outline.cursor > 0 {
				m.outline.cursor--
			}
			m.dirty = true
		}
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	}
	return m, nil
}

func (m Model) updateDialogFindings(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case km.Type == tea.KeyEsc:
		m.dialog = dialogState{}
		m.helpOpen = false
		m.focus = m.prevFocus
	case string(km.Runes) == "?":
		m.helpOpen = true
	}
	return m, nil
}

func (m Model) updateDialogQuitConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case string(km.Runes) == "y", km.Type == tea.KeyEnter:
		return m, tea.Quit
	case string(km.Runes) == "n", km.Type == tea.KeyEsc:
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	}
	return m, nil
}

func (m Model) updateDialogUnsavedChanges(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case km.Type == tea.KeyEsc, string(km.Runes) == "c":
		m.dialog = dialogState{}
		m.pendingQuit = false
		m.focus = m.prevFocus
	case string(km.Runes) == "d":
		return m, tea.Quit
	case string(km.Runes) == "s":
		hasFatals, errs := m.runValidate()
		if hasFatals {
			m.dialog = dialogState{kind: DialogFindingsPanel, payload: errs}
		} else {
			cmd := m.makeSaveCmd()
			m.saveInProgress = true
			m.pendingQuit = true
			m.dialog = dialogState{}
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) updateDialogSaveInProgress(msg tea.Msg) (tea.Model, tea.Cmd) {
	scm, ok := msg.(saveCompleteMsg)
	if !ok {
		return m, nil
	}
	m.dialog = dialogState{}
	m.saveInProgress = false
	if scm.result.Err == nil {
		m.dirty = false
		if scm.result.Snapshot != nil {
			m.diskDoc = m.doc
		}
		if m.pendingQuit {
			m.pendingQuit = false
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) updateDialogRecovery(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if km.Type == tea.KeyEsc || string(km.Runes) == "c" {
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	}
	return m, nil
}

func (m Model) handleGlobalKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	km := msg.(tea.KeyMsg)
	switch km.Type {
	case tea.KeyF10:
		m.prevFocus = m.focus
		m.menu.open = !m.menu.open
	case tea.KeyCtrlN:
		m.prevFocus = m.focus
		m.dialog = dialogState{kind: DialogNewChoice}
	case tea.KeyCtrlO:
		m.prevFocus = m.focus
		defaultPath := filepath.Join(m.projectDir, ".pr9k", "workflow", "config.json")
		m.dialog = dialogState{kind: DialogPathPicker, payload: defaultPath}
	case tea.KeyCtrlS:
		if m.saveInProgress {
			return m, nil
		}
		if !m.loaded {
			return m, nil
		}
		hasFatals, errs := m.runValidate()
		if hasFatals {
			m.prevFocus = m.focus
			m.dialog = dialogState{kind: DialogFindingsPanel, payload: errs}
			return m, nil
		}
		cmd := m.makeSaveCmd()
		m.saveInProgress = true
		return m, cmd
	case tea.KeyCtrlQ:
		if m.saveInProgress {
			m.prevFocus = m.focus
			m.dialog = dialogState{kind: DialogSaveInProgress}
			m.pendingQuit = true
			return m, nil
		}
		m.prevFocus = m.focus
		if m.dirty {
			m.dialog = dialogState{kind: DialogUnsavedChanges}
		} else {
			m.dialog = dialogState{kind: DialogQuitConfirm}
		}
	}
	return m, nil
}

func (m Model) updateEditView(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleEditKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case saveCompleteMsg:
		return m.handleSaveComplete(msg)
	case openFileResultMsg:
		return m.handleOpenFileResult(msg)
	}
	return m, nil
}

func (m Model) handleEditKey(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	if string(km.Runes) == "?" {
		m.helpOpen = true
		return m, nil
	}
	switch m.focus {
	case focusOutline:
		return m.handleOutlineKey(km)
	case focusDetail:
		return m.handleDetailKey(km)
	}
	return m, nil
}

func (m Model) handleOutlineKey(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.reorderMode {
		return m.handleReorderKey(km)
	}
	// Alt+Up/Down moves the step (checked before regular Up/Down).
	if km.Alt {
		switch km.Type {
		case tea.KeyUp:
			m = doMoveStepUp(m)
			m.dirty = true
		case tea.KeyDown:
			m = doMoveStepDown(m)
			m.dirty = true
		}
		return m, nil
	}
	switch km.Type {
	case tea.KeyDown:
		if m.outline.cursor < len(m.doc.Steps)-1 {
			m.outline.cursor++
		}
	case tea.KeyUp:
		if m.outline.cursor > 0 {
			m.outline.cursor--
		}
	case tea.KeyTab:
		m.prevFocus = m.focus
		m.focus = focusDetail
		m.detail.cursor = 0
	case tea.KeyDelete:
		if len(m.doc.Steps) > 0 {
			name := m.doc.Steps[m.outline.cursor].Name
			m.prevFocus = m.focus
			m.dialog = dialogState{kind: DialogRemoveConfirm, payload: name}
		}
	default:
		if string(km.Runes) == "r" {
			m.reorderMode = true
			m.reorderOrigin = m.outline.cursor
			m.reorderSnapshot = make([]workflowmodel.Step, len(m.doc.Steps))
			copy(m.reorderSnapshot, m.doc.Steps)
		}
	}
	return m, nil
}

func (m Model) handleReorderKey(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch km.Type {
	case tea.KeyUp:
		m = doMoveStepUp(m)
	case tea.KeyDown:
		m = doMoveStepDown(m)
	case tea.KeyEnter:
		m.reorderMode = false
		m.dirty = true
	case tea.KeyEsc:
		restored := make([]workflowmodel.Step, len(m.reorderSnapshot))
		copy(restored, m.reorderSnapshot)
		m.doc.Steps = restored
		m.outline.cursor = m.reorderOrigin
		m.reorderMode = false
	}
	return m, nil
}

// doMoveStepUp swaps the step at m.outline.cursor with the one above it.
// Returns a new Model to preserve value semantics.
func doMoveStepUp(m Model) Model {
	i := m.outline.cursor
	if i <= 0 {
		return m
	}
	steps := make([]workflowmodel.Step, len(m.doc.Steps))
	copy(steps, m.doc.Steps)
	steps[i], steps[i-1] = steps[i-1], steps[i]
	m.doc.Steps = steps
	m.outline.cursor--
	return m
}

// doMoveStepDown swaps the step at m.outline.cursor with the one below it.
func doMoveStepDown(m Model) Model {
	i := m.outline.cursor
	if i >= len(m.doc.Steps)-1 {
		return m
	}
	steps := make([]workflowmodel.Step, len(m.doc.Steps))
	copy(steps, m.doc.Steps)
	steps[i], steps[i+1] = steps[i+1], steps[i]
	m.doc.Steps = steps
	m.outline.cursor++
	return m
}

func (m Model) handleDetailKey(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.detail.dropdownOpen {
		return m.handleDropdownKey(km)
	}
	switch km.Type {
	case tea.KeyTab:
		m.prevFocus = m.focus
		m.focus = focusOutline
		m.detail.revealedField = -1 // re-mask on focus-leave
	case tea.KeyDown:
		m.detail.cursor++
	case tea.KeyUp:
		if m.detail.cursor > 0 {
			m.detail.cursor--
		}
	case tea.KeyEnter:
		m.detail.dropdownOpen = true
	default:
		if string(km.Runes) == "r" {
			m.detail.revealedField = m.detail.cursor
		}
	}
	return m, nil
}

func (m Model) handleDropdownKey(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch km.Type {
	case tea.KeyEsc:
		m.detail.dropdownOpen = false
	case tea.KeyEnter:
		m.detail.dropdownOpen = false
	}
	return m, nil
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	ow := outlineWidth(m.width)
	if msg.X < ow {
		m.outline.scrolls++
		var cmd tea.Cmd
		m.outline.vp, cmd = m.outline.vp.Update(msg)
		return m, cmd
	}
	m.detail.scrolls++
	var cmd tea.Cmd
	m.detail.vp, cmd = m.detail.vp.Update(msg)
	return m, cmd
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	ow := outlineWidth(m.width)
	dw := m.width - ow
	if dw < 1 {
		dw = 1
	}
	panelH := m.height - 2 // minus menu row and footer row
	if panelH < 1 {
		panelH = 1
	}
	m.outline.width = ow
	m.outline.height = panelH
	m.outline.vp.Width = ow
	m.outline.vp.Height = panelH
	m.detail.width = dw
	m.detail.height = panelH
	m.detail.vp.Width = dw
	m.detail.vp.Height = panelH
	return m, nil
}

func (m Model) handleSaveComplete(msg saveCompleteMsg) (tea.Model, tea.Cmd) {
	m.saveInProgress = false
	if msg.result.Err == nil {
		m.dirty = false
		if msg.result.Snapshot != nil {
			m.diskDoc = m.doc
		}
		m.saveBanner = "Saved at " + time.Now().Format("15:04:05")
		if m.pendingQuit {
			m.pendingQuit = false
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) handleOpenFileResult(msg openFileResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		raw := string(msg.rawBytes)
		m.prevFocus = m.focus
		m.dialog = dialogState{kind: DialogRecovery, payload: raw}
		m.loaded = false
		return m, nil
	}
	m.doc = msg.doc
	m.diskDoc = msg.diskDoc
	if msg.workflowDir != "" {
		m.workflowDir = msg.workflowDir
	}
	m.loaded = true
	m.dirty = false
	m.outline.cursor = 0
	return m, nil
}

// runValidate runs validation and returns (hasFatals, errorsPayload).
// workflowedit does not import internal/validator directly (D-4).
func (m Model) runValidate() (bool, any) {
	if m.validateFn != nil {
		return m.validateFn(m.doc, m.workflowDir, m.companions)
	}
	errs := workflowvalidate.Validate(m.doc, m.workflowDir, m.companions)
	for _, e := range errs {
		if e.IsFatal() {
			return true, errs
		}
	}
	return false, errs
}

// makeSaveCmd returns a tea.Cmd that performs the async save.
func (m Model) makeSaveCmd() tea.Cmd {
	saveFS := m.saveFS
	workflowDir := m.workflowDir
	diskDoc := m.diskDoc
	doc := m.doc
	companions := m.companions
	return func() tea.Msg {
		result := workflowio.Save(saveFS, workflowDir, diskDoc, doc, companions)
		return saveCompleteMsg{result: result}
	}
}

// outlineWidth computes the outline panel's column count for the given total
// terminal width: 40% of width, clamped to [20, 40] (D-12).
func outlineWidth(totalWidth int) int {
	w := totalWidth * 40 / 100
	if w < 20 {
		w = 20
	}
	if w > 40 {
		w = 40
	}
	return w
}
