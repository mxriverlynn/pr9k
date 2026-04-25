package workflowedit

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
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

// bannerState holds the set of active warning banners for the session header.
// Priority order (highest first): read-only > external-workflow > symlink >
// shared-install > unknown-field. Only the highest-priority banner is rendered;
// remaining active banners contribute to the "[N more warnings]" affordance.
type bannerState struct {
	isReadOnly         bool
	isExternalWorkflow bool
	isSymlink          bool
	symlinkTarget      string
	isSharedInstall    bool
	hasUnknownField    bool
}

// activeBanner returns the text of the highest-priority active banner and the
// count of lower-priority active banners (for the "[N more warnings]" affordance).
func (b bannerState) activeBanner() (string, int) {
	all := b.allBannerTexts()
	if len(all) == 0 {
		return "", 0
	}
	return all[0], len(all) - 1
}

func (b bannerState) allBannerTexts() []string {
	var out []string
	if b.isReadOnly {
		out = append(out, "[read-only]")
	}
	if b.isExternalWorkflow {
		out = append(out, "[external workflow]")
	}
	if b.isSymlink {
		target := b.symlinkTarget
		if target != "" {
			out = append(out, "[symlink → "+target+"]")
		} else {
			out = append(out, "[symlink]")
		}
	}
	if b.isSharedInstall {
		out = append(out, "[shared install]")
	}
	if b.hasUnknownField {
		out = append(out, "[unknown fields]")
	}
	return out
}

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

	validateInProgress bool
	saveInProgress     bool
	pendingQuit        bool
	saveSnapshot       *workflowio.SaveSnapshot // nil until first successful save

	// forceSave bypasses the mtime conflict check on the next Ctrl+S (set by
	// DialogFileConflict Overwrite).
	forceSave bool

	// firstSaveConfirmed records that the user acknowledged saving to an
	// external or symlinked workflow for this session (D17/D22).
	firstSaveConfirmed bool

	findingsPanel findingsPanel

	outline outlinePanel
	detail  detailPane
	menu    menuBar

	width  int
	height int

	reorderMode     bool
	reorderOrigin   int
	reorderSnapshot []workflowmodel.Step

	saveBanner string // "Saved at HH:MM:SS" after a successful save

	banners bannerState // active warning banners set from load-pipeline signals

	logW io.Writer // session-event log destination; nil disables logging

	// validateFn overrides the real validator when non-nil; used by tests.
	validateFn func(workflowmodel.WorkflowDoc, string, map[string][]byte) []findingResult
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

// WithLog returns a copy of the Model configured to write session events to w.
func (m Model) WithLog(w io.Writer) Model {
	m.logW = w
	return m
}

// IsDirty reports whether the in-memory document differs from the on-disk
// baseline (last load or last successful save).
func (m Model) IsDirty() bool {
	return workflowmodel.IsDirty(m.diskDoc, m.doc)
}

// WithNoValidation returns a copy of the Model that skips the real validator
// during the save pipeline, delivering zero findings to Update immediately.
func (m Model) WithNoValidation() Model {
	m.validateFn = func(_ workflowmodel.WorkflowDoc, _ string, _ map[string][]byte) []findingResult {
		return nil
	}
	return m
}

// LoadResultMsg returns the tea.Msg that delivers a loaded workflow document
// into the model's Update loop. doc is the in-memory state; diskDoc is the
// save-baseline for dirty detection; companions is the companion-file cache;
// workflowDir overrides the model's workflow directory when non-empty.
func LoadResultMsg(doc, diskDoc workflowmodel.WorkflowDoc, companions map[string][]byte, workflowDir string) tea.Msg {
	return openFileResultMsg{
		doc:         doc,
		diskDoc:     diskDoc,
		companions:  companions,
		workflowDir: workflowDir,
	}
}

// logEvent writes a session-event line to logW if it is set. It never writes
// containerEnv values, env entry values, prompt-file content, or editor
// argument lists (field-exclusion contract R7).
func (m Model) logEvent(event string) {
	if m.logW == nil {
		return
	}
	_, _ = fmt.Fprintln(m.logW, event)
}

// fmtEditorInvoked returns the session-event string for the editor_invoked event.
func fmtEditorInvoked(binary string) string {
	return "editor_invoked binary=" + binary
}

// fmtEditorExited returns the session-event string for the editor_exit event.
func fmtEditorExited(binary string, exitCode int, d time.Duration) string {
	return fmt.Sprintf("editor_exit binary=%s exit_code=%d duration_ms=%d", binary, exitCode, d.Milliseconds())
}

func fmtSymlinkDetected(target string) string {
	return "symlink_detected target=" + target
}

func fmtExternalWorkflowDetected(workflowDir string) string {
	return "external_workflow_detected workflowDir=" + workflowDir
}

func fmtReadOnlyDetected(workflowDir string) string {
	return "read_only_detected workflowDir=" + workflowDir
}

// Init satisfies tea.Model. No startup commands are needed.
func (m Model) Init() tea.Cmd { return nil }

// Update satisfies tea.Model. The routing order (D-9 + D-PR2-19) is:
//  0. typed-message pre-dispatch — validateCompleteMsg | saveCompleteMsg | quitMsg
//  1. helpOpen  → updateHelpModal
//  2. dialog    → updateDialog
//  3. globalKey → handleGlobalKey
//  4. default   → updateEditView
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Tier-0 pre-dispatch: these typed messages bypass dialog/help state so
	// that save completions, load results, and programmatic quit are never
	// swallowed by an active dialog (D-PR2-19).
	switch msg.(type) {
	case validateCompleteMsg, saveCompleteMsg, openFileResultMsg, quitMsg:
		return m.updateAsyncCompletion(msg)
	}

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

// updateAsyncCompletion handles the four pre-dispatched message types that
// bypass dialog/help state (tier-0, D-PR2-19).
func (m Model) updateAsyncCompletion(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case validateCompleteMsg:
		return m.handleValidateComplete(msg)
	case saveCompleteMsg:
		return m.handleSaveComplete(msg)
	case openFileResultMsg:
		return m.handleOpenFileResult(msg)
	case quitMsg:
		return m, tea.Quit
	}
	return m, nil
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
		if picker, ok := m.dialog.payload.(pathPickerModel); ok {
			if picker.kind == PickerKindNew {
				warning := ""
				targetConfig := filepath.Join(strings.TrimSuffix(picker.input, "/"), "config.json")
				if _, err := os.Stat(targetConfig); err == nil {
					warning = " — That path already contains a config.json — overwrite?"
				}
				return "New: " + picker.input + warning + "\nCreate / Cancel"
			}
			return "Open: " + picker.input
		}
		return "Open: "
	case DialogUnsavedChanges:
		return "Unsaved changes: Save / Cancel / Discard"
	case DialogQuitConfirm:
		return "Quit? Yes / No"
	case DialogFindingsPanel:
		return "Findings: validation errors found"
	case DialogAcknowledgeFindings:
		return "Validation warnings: acknowledge and save (Enter/y) or cancel (Esc)"
	case DialogSaveInProgress:
		return "Save in progress — please wait"
	case DialogRemoveConfirm:
		name, _ := m.dialog.payload.(string)
		return fmt.Sprintf("Delete step %q? Delete / Cancel", name)
	case DialogRecovery:
		raw, _ := m.dialog.payload.(string)
		return "Recovery — o  open editor  r  reload  d  discard  c  cancel\n" + raw
	case DialogError:
		msg, _ := m.dialog.payload.(string)
		return "Error: " + msg
	case DialogFileConflict:
		return "File changed on disk — o  overwrite  r  reload  c  cancel"
	case DialogCrashTempNotice:
		path, _ := m.dialog.payload.(string)
		return "Crash temp file detected: " + path + "\nd  discard  l  leave"
	case DialogFirstSaveConfirm:
		return "Save to external/symlinked workflow? y  yes  n  no"
	case DialogExternalEditorOpening:
		return "Opening external editor…"
	case DialogCopyBrokenRef:
		return "Default model reference is broken — copy anyway? y  copy anyway  c  cancel"
	default:
		return ""
	}
}

func (m Model) renderEmptyEditor() string {
	return HintEmpty
}

func (m Model) renderEditView() string {
	header := m.renderSessionHeader()
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	outlineStr := m.outline.render(m.doc, m.outline.cursor, m.reorderMode)
	var detailStr string
	stepIdx := cursorStepIdx(rows, m.outline.cursor)
	if stepIdx >= 0 {
		detailStr = m.detail.render(m.doc.Steps[stepIdx])
	}
	return header + "\n" + outlineStr + " | " + detailStr
}

// renderSessionHeader renders the third row of the edit view:
// target path + dirty glyph + at-most-one banner + "[N more warnings]" affordance.
func (m Model) renderSessionHeader() string {
	path := m.workflowDir
	if m.dirty {
		path += "*"
	}
	banner, extra := m.banners.activeBanner()
	if banner == "" {
		return path
	}
	header := path + " " + banner
	if extra > 0 {
		header += fmt.Sprintf(" [%d more warnings]", extra)
	}
	return header
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
	case DialogAcknowledgeFindings:
		return m.updateDialogAcknowledgeFindings(msg)
	case DialogQuitConfirm:
		return m.updateDialogQuitConfirm(msg)
	case DialogUnsavedChanges:
		return m.updateDialogUnsavedChanges(msg)
	case DialogSaveInProgress:
		return m.updateDialogSaveInProgress(msg)
	case DialogFileConflict:
		return m.updateDialogFileConflict(msg)
	case DialogFirstSaveConfirm:
		return m.updateDialogFirstSaveConfirm(msg)
	case DialogCrashTempNotice:
		return m.updateDialogCrashTempNotice(msg)
	case DialogRecovery:
		return m.updateDialogRecovery(msg)
	case DialogCopyBrokenRef:
		return m.updateDialogCopyBrokenRef(msg)
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
	switch {
	case km.Type == tea.KeyEsc:
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	case string(km.Runes) == "c":
		// Copy from default: pre-copy integrity check (GAP-021).
		// Attempt to load the default bundle; on any error open the
		// Copy-anyway/Cancel dialog (DialogCopyBrokenRef). On success,
		// also open the dialog as the confirm step (copy wiring deferred).
		_, err := workflowmodel.CopyFromDefault(m.workflowDir)
		if err != nil {
			m.dialog = dialogState{kind: DialogCopyBrokenRef}
			return m, nil
		}
		// TODO: copy companions and load the doc (full copy wiring deferred).
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	}
	return m, nil
}

func (m Model) updateDialogPathPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	picker, _ := m.dialog.payload.(pathPickerModel)

	switch msg := msg.(type) {
	case pathCompletionMsg:
		// Discard stale completions (D-PR2-20 generation counter).
		if msg.gen != picker.pathCompletionGen {
			return m, nil
		}
		if len(msg.matches) > 0 {
			picker.matches = msg.matches
			picker.matchIdx = 0
			picker.input = msg.matches[0]
		} else {
			picker.matches = []string{}
		}
		m.dialog.payload = picker
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			m.dialog = dialogState{}
			m.focus = m.prevFocus

		case tea.KeyEnter:
			m.dialog = dialogState{}
			m.focus = m.prevFocus

		case tea.KeyTab:
			if picker.matches == nil {
				picker.pathCompletionGen++
				m.dialog.payload = picker
				return m, completePath(picker.input, picker.pathCompletionGen)
			}
			if len(picker.matches) > 0 {
				picker.matchIdx = (picker.matchIdx + 1) % len(picker.matches)
				picker.input = picker.matches[picker.matchIdx]
				m.dialog.payload = picker
			}

		case tea.KeyShiftTab:
			if picker.matches == nil {
				picker.pathCompletionGen++
				m.dialog.payload = picker
				return m, completePath(picker.input, picker.pathCompletionGen)
			}
			if len(picker.matches) > 0 {
				picker.matchIdx = (picker.matchIdx - 1 + len(picker.matches)) % len(picker.matches)
				picker.input = picker.matches[picker.matchIdx]
				m.dialog.payload = picker
			}

		case tea.KeyBackspace:
			if len(picker.input) > 0 {
				picker.input = picker.input[:len(picker.input)-1]
				picker.matches = nil
				picker.matchIdx = 0
				picker.pathCompletionGen++
				m.dialog.payload = picker
			}

		default:
			if len(msg.Runes) > 0 {
				picker.input += string(msg.Runes)
				picker.matches = nil
				picker.matchIdx = 0
				picker.pathCompletionGen++
				m.dialog.payload = picker
			}
		}
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
		rows := buildOutlineRows(m.doc, m.outline.collapsed)
		idx := cursorStepIdx(rows, m.outline.cursor)
		if idx >= 0 && idx < len(m.doc.Steps) {
			steps := make([]workflowmodel.Step, 0, len(m.doc.Steps)-1)
			steps = append(steps, m.doc.Steps[:idx]...)
			steps = append(steps, m.doc.Steps[idx+1:]...)
			m.doc.Steps = steps
			// Rebuild rows after deletion; clamp cursor to valid range.
			newRows := buildOutlineRows(m.doc, m.outline.collapsed)
			if m.outline.cursor >= len(newRows) && m.outline.cursor > 0 {
				m.outline.cursor = len(newRows) - 1
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
	case km.Type == tea.KeyDown:
		m.findingsPanel.vp.ScrollDown(1)
	case km.Type == tea.KeyUp:
		m.findingsPanel.vp.ScrollUp(1)
	case km.Type == tea.KeyEnter:
		// Jump to the first finding's referenced step (if any).
		stepIdx := m.findingsPanel.firstStepIdx()
		if stepIdx >= 0 && stepIdx < len(m.doc.Steps) {
			m.outline.cursor = findStepCursorByIdx(m, stepIdx)
		}
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	}
	return m, nil
}

func (m Model) updateDialogAcknowledgeFindings(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case km.Type == tea.KeyEsc, string(km.Runes) == "c":
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	case string(km.Runes) == "y", km.Type == tea.KeyEnter:
		// GAP-027/028: Write acknowledged finding keys into ackSet BEFORE save.
		if m.findingsPanel.ackSet == nil {
			m.findingsPanel.ackSet = make(map[string]bool)
		}
		for _, e := range m.findingsPanel.entries {
			m.findingsPanel.ackSet[e.key] = true
		}
		m.dialog = dialogState{}
		m.saveInProgress = true
		return m, m.makeSaveCmd()
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
		hasFatals, items := m.runValidate()
		if hasFatals {
			m.findingsPanel = buildFindingsPanel(items, m.doc.Steps, m.findingsPanel)
			m.dialog = dialogState{kind: DialogFindingsPanel}
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
	// saveCompleteMsg is handled by tier-0 pre-dispatch; this handler only
	// processes key input while save is in progress.
	km, ok := msg.(tea.KeyMsg)
	if ok && km.Type == tea.KeyEsc {
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	}
	return m, nil
}

// updateDialogFileConflict handles the D-41 mtime-mismatch dialog.
// o = overwrite (bypass snapshot check once), r = reload, c/Esc = cancel.
func (m Model) updateDialogFileConflict(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch string(km.Runes) {
	case "o":
		// Overwrite: bypass the next mtime check; user can press Ctrl+S again.
		m.forceSave = true
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	case "r":
		// Reload: discard in-memory state and re-load from disk.
		m.dialog = dialogState{}
		return m, m.makeLoadCmd()
	case "c":
		// Cancel: close dialog; preserve in-memory state.
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	default:
		if km.Type == tea.KeyEsc {
			m.dialog = dialogState{}
			m.focus = m.prevFocus
		}
	}
	return m, nil
}

// updateDialogFirstSaveConfirm handles the D-17/D-22 first-save-to-external dialog.
// y = confirm and save, n/Esc = cancel.
func (m Model) updateDialogFirstSaveConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch string(km.Runes) {
	case "y":
		m.firstSaveConfirmed = true
		m.dialog = dialogState{}
		m.saveInProgress = true
		return m, m.makeSaveCmd()
	case "n":
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	default:
		if km.Type == tea.KeyEsc {
			m.dialog = dialogState{}
			m.focus = m.prevFocus
		}
	}
	return m, nil
}

// updateDialogCrashTempNotice handles the D-42-a crash-temp-file dialog.
// d = discard (with containment + regular-file guard), l/Esc = leave.
func (m Model) updateDialogCrashTempNotice(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch string(km.Runes) {
	case "d":
		path, _ := m.dialog.payload.(string)
		if !safeToDelete(m.workflowDir, path) {
			// Containment or type check failed; show error, do NOT delete.
			m.dialog = dialogState{kind: DialogError, payload: "Cannot delete: path is outside workflow directory or is not a regular file."}
			return m, nil
		}
		_ = os.Remove(path)
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	case "l":
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	default:
		if km.Type == tea.KeyEsc {
			m.dialog = dialogState{}
			m.focus = m.prevFocus
		}
	}
	return m, nil
}

// updateDialogRecovery handles the D-36 malformed-config recovery dialog.
// o = open in external editor (then reload), r = reload, d = discard, c/Esc = discard.
func (m Model) updateDialogRecovery(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch string(km.Runes) {
	case "o":
		// Open in external editor; on exit, attempt reload.
		filePath := filepath.Join(m.workflowDir, "config.json")
		workflowDir := m.workflowDir
		m.dialog = dialogState{kind: DialogExternalEditorOpening}
		return m, m.editor.Run(filePath, func(err error) tea.Msg {
			// After editor exits, attempt reload regardless of editor error.
			result, loadErr := workflowio.Load(workflowDir)
			if loadErr != nil {
				return openFileResultMsg{err: loadErr}
			}
			if result.RecoveryView != nil {
				return openFileResultMsg{
					err:      fmt.Errorf("workflowedit: parse error in config.json"),
					rawBytes: result.RecoveryView,
				}
			}
			return openFileResultMsg{
				doc:         result.Doc,
				diskDoc:     result.Doc,
				companions:  result.Companions,
				workflowDir: workflowDir,
			}
		})
	case "r":
		// Reload from disk.
		m.dialog = dialogState{}
		return m, m.makeLoadCmd()
	case "d", "c":
		// Discard / Cancel: return to empty-editor hint state.
		m.dialog = dialogState{}
		m.loaded = false
		m.focus = m.prevFocus
	default:
		if km.Type == tea.KeyEsc {
			m.dialog = dialogState{}
			m.loaded = false
			m.focus = m.prevFocus
		}
	}
	return m, nil
}

// updateDialogCopyBrokenRef handles the Copy-anyway/Cancel dialog shown when
// the default workflow bundle has broken or missing companion references (GAP-021).
// y = copy anyway, c/Esc = cancel.
func (m Model) updateDialogCopyBrokenRef(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch string(km.Runes) {
	case "y":
		// Copy anyway: close dialog and return to empty editor (full copy wiring deferred).
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	case "c":
		m.dialog = dialogState{}
		m.focus = m.prevFocus
	default:
		if km.Type == tea.KeyEsc {
			m.dialog = dialogState{}
			m.focus = m.prevFocus
		}
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
		m.saveSnapshot = nil // F-98: reset snapshot on session transition
		m.prevFocus = m.focus
		m.dialog = dialogState{kind: DialogNewChoice}
	case tea.KeyCtrlO:
		m.prevFocus = m.focus
		defaultPath := filepath.Join(m.projectDir, ".pr9k", "workflow", "config.json")
		m.dialog = dialogState{kind: DialogPathPicker, payload: newPathPicker(defaultPath)}
	case tea.KeyCtrlS:
		if m.validateInProgress || m.saveInProgress {
			return m, nil
		}
		if !m.loaded {
			return m, nil
		}
		// D-41 snapshot compare: check mtime unless forceSave is set.
		if m.saveSnapshot != nil && !m.forceSave {
			configPath := filepath.Join(m.workflowDir, "config.json")
			fi, err := m.saveFS.Stat(configPath)
			if err == nil && !m.saveSnapshot.ModTime.Equal(fi.ModTime()) {
				m.prevFocus = m.focus
				m.dialog = dialogState{kind: DialogFileConflict}
				return m, nil
			}
		}
		m.forceSave = false // clear after use
		m.validateInProgress = true
		return m, m.makeValidateCmd()
	case tea.KeyCtrlQ:
		if m.saveInProgress || m.validateInProgress {
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
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
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
		if m.outline.cursor < len(rows)-1 {
			m.outline.cursor++
		}
	case tea.KeyUp:
		if m.outline.cursor > 0 {
			m.outline.cursor--
		}
	case tea.KeyTab:
		// Tab switches to detail only when cursor is on a step row.
		if cursorStepIdx(rows, m.outline.cursor) >= 0 {
			m.prevFocus = m.focus
			m.focus = focusDetail
			m.detail.cursor = 0
		}
	case tea.KeyDelete:
		stepIdx := cursorStepIdx(rows, m.outline.cursor)
		if stepIdx >= 0 {
			name := m.doc.Steps[stepIdx].Name
			m.prevFocus = m.focus
			m.dialog = dialogState{kind: DialogRemoveConfirm, payload: name}
		}
	case tea.KeyEnter:
		// Enter on an add row creates a new empty item in that section.
		if m.outline.cursor < len(rows) && rows[m.outline.cursor].kind == rowKindAddRow {
			m = doAddItemInSection(m, rows[m.outline.cursor].section)
		}
	default:
		switch string(km.Runes) {
		case "r":
			stepIdx := cursorStepIdx(rows, m.outline.cursor)
			if stepIdx >= 0 {
				m.reorderMode = true
				m.reorderOrigin = m.outline.cursor
				m.reorderSnapshot = make([]workflowmodel.Step, len(m.doc.Steps))
				copy(m.reorderSnapshot, m.doc.Steps)
			}
		case " ":
			// Space toggles collapse of the section the cursor is in.
			if m.outline.cursor < len(rows) {
				m = doToggleCollapse(m, rows, m.outline.cursor)
			}
		case "a":
			// 'a' on a section header triggers add for that section.
			if m.outline.cursor < len(rows) && rows[m.outline.cursor].kind == rowKindSectionHeader {
				m = doAddItemInSection(m, rows[m.outline.cursor].section)
			}
		}
	}
	return m, nil
}

// doToggleCollapse toggles the collapsed state of the section containing the
// row at cursorPos. If collapsing while the cursor is inside the section
// (not on the header), the cursor is moved to the section header.
func doToggleCollapse(m Model, rows []outlineRow, cursorPos int) Model {
	if cursorPos >= len(rows) {
		return m
	}
	sk := rows[cursorPos].section
	curRowKind := rows[cursorPos].kind

	if m.outline.collapsed == nil {
		m.outline.collapsed = make(map[sectionKey]bool)
	}
	m.outline.collapsed[sk] = !m.outline.collapsed[sk]

	// If we just collapsed and cursor was inside (not on header), move to header.
	if m.outline.collapsed[sk] && curRowKind != rowKindSectionHeader {
		newRows := buildOutlineRows(m.doc, m.outline.collapsed)
		for i, row := range newRows {
			if row.kind == rowKindSectionHeader && row.section == sk {
				m.outline.cursor = i
				break
			}
		}
	}
	return m
}

// doAddItemInSection appends a new empty item to the given section.
// For phase sections, a new empty Step is appended with the matching Phase.
// For env/containerEnv sections, a new empty entry is added.
func doAddItemInSection(m Model, sk sectionKey) Model {
	switch sk {
	case sectionInitialize:
		m.doc.Steps = append(m.doc.Steps, workflowmodel.Step{Phase: workflowmodel.StepPhaseInitialize})
		m.dirty = true
		m.outline.cursor = findAddedStepCursor(m)
	case sectionIteration:
		m.doc.Steps = append(m.doc.Steps, workflowmodel.Step{Phase: workflowmodel.StepPhaseIteration})
		m.dirty = true
		m.outline.cursor = findAddedStepCursor(m)
	case sectionFinalize:
		m.doc.Steps = append(m.doc.Steps, workflowmodel.Step{Phase: workflowmodel.StepPhaseFinalize})
		m.dirty = true
		m.outline.cursor = findAddedStepCursor(m)
	case sectionEnv:
		m.doc.Env = append(m.doc.Env, "")
		m.dirty = true
	case sectionContainerEnv:
		if m.doc.ContainerEnv == nil {
			m.doc.ContainerEnv = make(map[string]string)
		}
		// Add a placeholder key; the detail pane (WU-PR2-9b) handles editing.
		m.dirty = true
	}
	return m
}

// findAddedStepCursor returns the flat-row index of the last step in doc.Steps.
func findAddedStepCursor(m Model) int {
	newIdx := len(m.doc.Steps) - 1
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	for i, row := range rows {
		if row.kind == rowKindStep && row.stepIdx == newIdx {
			return i
		}
	}
	return m.outline.cursor
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

// doMoveStepUp swaps the step at cursor with the previous step in doc.Steps.
// Returns a new Model to preserve value semantics.
func doMoveStepUp(m Model) Model {
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	i := cursorStepIdx(rows, m.outline.cursor)
	if i <= 0 {
		return m
	}
	steps := make([]workflowmodel.Step, len(m.doc.Steps))
	copy(steps, m.doc.Steps)
	steps[i], steps[i-1] = steps[i-1], steps[i]
	m.doc.Steps = steps
	// Move cursor to the swapped step's new position.
	m.outline.cursor = findStepCursorByIdx(m, i-1)
	return m
}

// doMoveStepDown swaps the step at cursor with the next step in doc.Steps.
func doMoveStepDown(m Model) Model {
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	i := cursorStepIdx(rows, m.outline.cursor)
	if i < 0 || i >= len(m.doc.Steps)-1 {
		return m
	}
	steps := make([]workflowmodel.Step, len(m.doc.Steps))
	copy(steps, m.doc.Steps)
	steps[i], steps[i+1] = steps[i+1], steps[i]
	m.doc.Steps = steps
	m.outline.cursor = findStepCursorByIdx(m, i+1)
	return m
}

// findStepCursorByIdx returns the flat-row index for doc.Steps[stepIdx].
func findStepCursorByIdx(m Model, stepIdx int) int {
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	for i, row := range rows {
		if row.kind == rowKindStep && row.stepIdx == stepIdx {
			return i
		}
	}
	return m.outline.cursor
}

func (m Model) handleDetailKey(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Dispatch to sub-mode handlers first.
	if m.detail.dropdownOpen {
		return m.handleChoiceListKey(km)
	}
	if m.detail.modelSuggFocus {
		return m.handleModelSuggKey(km)
	}
	if m.detail.editing {
		return m.handleEditingKey(km)
	}

	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	stepIdx := cursorStepIdx(rows, m.outline.cursor)

	switch km.Type {
	case tea.KeyTab:
		m.prevFocus = m.focus
		m.focus = focusOutline
		m.detail.revealedField = -1 // re-mask on focus-leave (D-47)
		m.detail.modelSuggFocus = false

	case tea.KeyDown:
		if stepIdx >= 0 && stepIdx < len(m.doc.Steps) {
			fields := buildDetailFields(m.doc.Steps[stepIdx])
			if m.detail.cursor < len(fields) && fields[m.detail.cursor].kind == fieldKindModelSuggest {
				// First Down from model field moves focus into suggestion list.
				m.detail.modelSuggFocus = true
				m.detail.modelSuggIdx = 0
				return m, nil
			}
		}
		m.detail.cursor++

	case tea.KeyUp:
		if m.detail.cursor > 0 {
			m.detail.cursor--
		}

	case tea.KeyEnter:
		if stepIdx >= 0 && stepIdx < len(m.doc.Steps) {
			fields := buildDetailFields(m.doc.Steps[stepIdx])
			if m.detail.cursor < len(fields) {
				f := fields[m.detail.cursor]
				switch f.kind {
				case fieldKindChoice:
					m.detail.dropdownOpen = true
					m.detail.choiceOptions = f.choices
					current := fieldValue(m.doc.Steps[stepIdx], f)
					m.detail.choiceIdx = 0
					for i, c := range f.choices {
						if c == current {
							m.detail.choiceIdx = i
							break
						}
					}
				default:
					// Text, numeric, model-suggest, secret-mask all use text editing.
					m.detail.editing = true
					m.detail.editBuf = fieldValue(m.doc.Steps[stepIdx], f)
					m.detail.editMsg = ""
				}
			}
		}

	default:
		if string(km.Runes) == "r" && stepIdx >= 0 && stepIdx < len(m.doc.Steps) {
			fields := buildDetailFields(m.doc.Steps[stepIdx])
			if m.detail.cursor < len(fields) && fields[m.detail.cursor].kind == fieldKindSecretMask {
				if m.detail.revealedField == m.detail.cursor {
					m.detail.revealedField = -1 // toggle: re-mask
				} else {
					m.detail.revealedField = m.detail.cursor
				}
			}
		}
	}
	return m, nil
}

// handleChoiceListKey handles keyboard input while a choice-list dropdown is open.
func (m Model) handleChoiceListKey(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch km.Type {
	case tea.KeyEsc:
		m.detail.dropdownOpen = false
	case tea.KeyDown:
		if m.detail.choiceIdx < len(m.detail.choiceOptions)-1 {
			m.detail.choiceIdx++
		}
	case tea.KeyUp:
		if m.detail.choiceIdx > 0 {
			m.detail.choiceIdx--
		}
	case tea.KeyEnter:
		m = commitChoiceSelection(m)
		m.detail.dropdownOpen = false
	default:
		// Typed char: jump to first option starting with that character.
		if len(km.Runes) == 1 {
			ch := strings.ToLower(string(km.Runes))
			for i, opt := range m.detail.choiceOptions {
				if strings.HasPrefix(strings.ToLower(opt), ch) {
					m.detail.choiceIdx = i
					break
				}
			}
		}
	}
	return m, nil
}

// handleEditingKey handles keyboard input while a text or numeric field is
// in editing mode. Field kind is checked to dispatch numeric behaviour.
func (m Model) handleEditingKey(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	stepIdx := cursorStepIdx(rows, m.outline.cursor)
	isNumeric := false
	if stepIdx >= 0 && stepIdx < len(m.doc.Steps) {
		fields := buildDetailFields(m.doc.Steps[stepIdx])
		if m.detail.cursor < len(fields) && fields[m.detail.cursor].kind == fieldKindNumeric {
			isNumeric = true
		}
	}

	switch km.Type {
	case tea.KeyEsc:
		m.detail.editing = false
		m.detail.editBuf = ""
		m.detail.editMsg = ""
		return m, nil

	case tea.KeyEnter:
		m = commitDetailEdit(m)
		m.detail.editing = false
		m.detail.editMsg = ""
		return m, nil

	case tea.KeyBackspace:
		if len(m.detail.editBuf) > 0 {
			runes := []rune(m.detail.editBuf)
			m.detail.editBuf = string(runes[:len(runes)-1])
		}
		return m, nil
	}

	if len(km.Runes) == 0 {
		return m, nil
	}

	isPaste := len(km.Runes) > 1
	raw := string(km.Runes)

	if isNumeric {
		if isPaste {
			sanitized := stripAtFirstNonDigit(raw)
			if sanitized != raw {
				m.detail.editMsg = "pasted content sanitized"
			}
			m.detail.editBuf += sanitized
		} else {
			// Single rune: silently drop non-digits.
			if r := km.Runes[0]; r >= '0' && r <= '9' {
				m.detail.editBuf += string(r)
			}
		}
		m = clampNumericEdit(m)
	} else {
		m.detail.editBuf += sanitizePlainText(raw)
	}
	return m, nil
}

// handleModelSuggKey handles keyboard input while the model suggestion list
// has focus (modelSuggFocus == true).
func (m Model) handleModelSuggKey(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	sugs := workflowmodel.ModelSuggestions
	switch km.Type {
	case tea.KeyEsc:
		m.detail.modelSuggFocus = false
	case tea.KeyDown:
		if m.detail.modelSuggIdx < len(sugs)-1 {
			m.detail.modelSuggIdx++
		}
	case tea.KeyUp:
		if m.detail.modelSuggIdx > 0 {
			m.detail.modelSuggIdx--
		}
	case tea.KeyEnter:
		rows := buildOutlineRows(m.doc, m.outline.collapsed)
		stepIdx := cursorStepIdx(rows, m.outline.cursor)
		if stepIdx >= 0 && stepIdx < len(m.doc.Steps) && m.detail.modelSuggIdx < len(sugs) {
			m.doc.Steps[stepIdx].Model = sugs[m.detail.modelSuggIdx]
			m.dirty = true
		}
		m.detail.modelSuggFocus = false
	}
	return m, nil
}

// commitChoiceSelection writes the selected choice value into the doc step.
func commitChoiceSelection(m Model) Model {
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	stepIdx := cursorStepIdx(rows, m.outline.cursor)
	if stepIdx < 0 || stepIdx >= len(m.doc.Steps) {
		return m
	}
	fields := buildDetailFields(m.doc.Steps[stepIdx])
	if m.detail.cursor >= len(fields) || m.detail.choiceIdx >= len(m.detail.choiceOptions) {
		return m
	}
	chosen := m.detail.choiceOptions[m.detail.choiceIdx]
	step := m.doc.Steps[stepIdx]
	switch fields[m.detail.cursor].label {
	case "Kind":
		step.Kind = workflowmodel.StepKind(chosen)
		step.IsClaudeSet = step.Kind == workflowmodel.StepKindClaude
	case "CaptureMode":
		step.CaptureMode = chosen
	case "OnTimeout":
		step.OnTimeout = chosen
	case "ResumePrevious":
		step.ResumePrevious = chosen == "true"
	case "BreakLoopIfEmpty":
		step.BreakLoopIfEmpty = chosen == "true"
	}
	m.doc.Steps[stepIdx] = step
	m.dirty = true
	return m
}

// commitDetailEdit writes editBuf into the doc step field at detail.cursor.
func commitDetailEdit(m Model) Model {
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	stepIdx := cursorStepIdx(rows, m.outline.cursor)
	if stepIdx < 0 || stepIdx >= len(m.doc.Steps) {
		return m
	}
	fields := buildDetailFields(m.doc.Steps[stepIdx])
	if m.detail.cursor >= len(fields) {
		return m
	}
	f := fields[m.detail.cursor]
	step := m.doc.Steps[stepIdx]
	val := m.detail.editBuf
	switch f.label {
	case "Name":
		step.Name = val
	case "Model":
		step.Model = val
	case "PromptFile":
		step.PromptFile = val
	case "Command":
		if val == "" {
			step.Command = nil
		} else {
			step.Command = strings.Fields(val)
		}
	case "CaptureAs":
		step.CaptureAs = val
	case "SkipIfCaptureEmpty":
		step.SkipIfCaptureEmpty = val
	case "TimeoutSeconds":
		n, _ := strconv.Atoi(val)
		step.TimeoutSeconds = n
	}
	if strings.HasPrefix(f.label, "containerEnv[") {
		idx := envFieldIndex(f.label)
		if idx >= 0 && idx < len(step.Env) {
			parts := strings.SplitN(val, "=", 2)
			if len(parts) == 2 {
				step.Env[idx].Key = parts[0]
				step.Env[idx].Value = parts[1]
			}
		}
	}
	if strings.HasPrefix(f.label, "env[") {
		idx := envFieldIndex(f.label)
		if idx >= 0 && idx < len(step.Env) {
			step.Env[idx].Key = val
		}
	}
	m.doc.Steps[stepIdx] = step
	m.dirty = true
	return m
}

// clampNumericEdit clamps editBuf to the [numMin, numMax] range of the current field.
func clampNumericEdit(m Model) Model {
	if m.detail.editBuf == "" {
		return m
	}
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	stepIdx := cursorStepIdx(rows, m.outline.cursor)
	if stepIdx < 0 || stepIdx >= len(m.doc.Steps) {
		return m
	}
	fields := buildDetailFields(m.doc.Steps[stepIdx])
	if m.detail.cursor >= len(fields) {
		return m
	}
	f := fields[m.detail.cursor]
	n, err := strconv.Atoi(m.detail.editBuf)
	if err != nil {
		return m
	}
	if n < f.numMin {
		n = f.numMin
	}
	if n > f.numMax {
		n = f.numMax
	}
	m.detail.editBuf = strconv.Itoa(n)
	return m
}

// stripAtFirstNonDigit returns s truncated at the first non-digit rune.
func stripAtFirstNonDigit(s string) string {
	for i, r := range s {
		if r < '0' || r > '9' {
			return s[:i]
		}
	}
	return s
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

// handleValidateComplete processes the result of the async validation command.
// Step 3–5 of the three-stage save state machine (D-13).
func (m Model) handleValidateComplete(msg validateCompleteMsg) (tea.Model, tea.Cmd) {
	m.validateInProgress = false
	for _, item := range msg.items {
		if item.isFatal {
			// Fatal findings block save.
			m.prevFocus = m.focus
			m.findingsPanel = buildFindingsPanel(msg.items, m.doc.Steps, m.findingsPanel)
			m.dialog = dialogState{kind: DialogFindingsPanel}
			return m, nil
		}
	}
	if len(msg.items) > 0 {
		// Check if all non-fatal findings are already acknowledged (GAP-027/028).
		allAcked := true
		for _, item := range msg.items {
			if !m.findingsPanel.ackSet[item.text] {
				allAcked = false
				break
			}
		}
		if !allAcked {
			// Warn/info-only findings: show acknowledgment dialog.
			m.prevFocus = m.focus
			m.findingsPanel = buildFindingsPanel(msg.items, m.doc.Steps, m.findingsPanel)
			m.dialog = dialogState{kind: DialogAcknowledgeFindings}
			return m, nil
		}
		// All acknowledged: rebuild panel to preserve ackSet, then proceed.
		m.findingsPanel = buildFindingsPanel(msg.items, m.doc.Steps, m.findingsPanel)
	}
	// Zero findings or all acknowledged: proceed directly to save.
	m.saveInProgress = true
	return m, m.makeSaveCmd()
}

func (m Model) handleSaveComplete(msg saveCompleteMsg) (tea.Model, tea.Cmd) {
	return m.handleSaveResult(msg)
}

// handleSaveResult is the shared save-completion handler.
func (m Model) handleSaveResult(msg saveCompleteMsg) (tea.Model, tea.Cmd) {
	m.saveInProgress = false
	if msg.result.Kind != workflowio.SaveErrorNone {
		m.prevFocus = m.focus
		m.dialog = dialogState{kind: DialogError, payload: saveErrorMsg(msg.result)}
		m.pendingQuit = false
		return m, nil
	}
	// Success.
	m.dirty = false
	m.saveSnapshot = msg.result.Snapshot
	if msg.result.Snapshot != nil {
		m.diskDoc = m.doc
	}
	m.saveBanner = "Saved at " + time.Now().Format("15:04:05")
	if m.pendingQuit {
		m.pendingQuit = false
		// D-PR2-10: re-route to QuitConfirm so user explicitly confirms exit.
		// dirty=false now, so handleGlobalKey will open DialogQuitConfirm.
		return m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyCtrlQ})
	}
	return m, nil
}

func (m Model) handleOpenFileResult(msg openFileResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.prevFocus = m.focus
		if len(msg.rawBytes) > 0 {
			// Parse error: show recovery dialog with raw bytes.
			m.dialog = dialogState{kind: DialogRecovery, payload: string(msg.rawBytes)}
		} else {
			// Other error (permission, not found, etc.): show error dialog.
			m.dialog = dialogState{kind: DialogError, payload: msg.err.Error()}
		}
		m.loaded = false
		return m, nil
	}
	m.doc = msg.doc
	m.diskDoc = msg.diskDoc
	if msg.workflowDir != "" {
		m.workflowDir = msg.workflowDir
	}
	if msg.companions != nil {
		m.companions = msg.companions
	}
	m.loaded = true
	m.dirty = false
	m.outline.cursor = 0
	m.saveSnapshot = nil // F-98: reset snapshot on session transition
	m.dialog = dialogState{}

	// Set banner state from load-pipeline signals (D-23, GAP-035).
	m.banners = bannerState{
		isSymlink:          msg.isSymlink,
		symlinkTarget:      msg.symlinkTarget,
		isExternalWorkflow: msg.isExternal,
		isReadOnly:         msg.isReadOnly,
		isSharedInstall:    msg.isSharedInstall,
	}

	// Log load-time security signals (F-PR2-9).
	if msg.isSymlink {
		m.logEvent(fmtSymlinkDetected(msg.symlinkTarget))
	}
	if msg.isExternal {
		m.logEvent(fmtExternalWorkflowDetected(msg.workflowDir))
	}
	if msg.isReadOnly {
		m.logEvent(fmtReadOnlyDetected(msg.workflowDir))
	}
	if msg.isSharedInstall {
		m.logEvent("shared_install_detected")
	}

	return m, nil
}

// runValidate runs validation synchronously (used by the UnsavedChanges dialog
// path). The async Ctrl+S path uses makeValidateCmd instead.
func (m Model) runValidate() (bool, []findingResult) {
	if m.validateFn != nil {
		items := m.validateFn(m.doc, m.workflowDir, m.companions)
		for _, item := range items {
			if item.isFatal {
				return true, items
			}
		}
		return false, items
	}
	errs := workflowvalidate.Validate(m.doc, m.workflowDir, m.companions)
	items := make([]findingResult, len(errs))
	for i, e := range errs {
		items[i] = findingResult{
			text:     e.Error(),
			isFatal:  e.IsFatal(),
			stepName: e.StepName,
		}
	}
	for _, item := range items {
		if item.isFatal {
			return true, items
		}
	}
	return false, items
}

// makeValidateCmd returns a tea.Cmd that runs validation asynchronously.
// It deep-copies doc and snapshots companions so the goroutine has its own data.
func (m Model) makeValidateCmd() tea.Cmd {
	docCopy := deepCopyDoc(m.doc)
	companionsCopy := snapshotCompanions(m)
	workflowDir := m.workflowDir
	vfn := m.validateFn
	return func() tea.Msg {
		if vfn != nil {
			return validateCompleteMsg{items: vfn(docCopy, workflowDir, companionsCopy)}
		}
		errs := workflowvalidate.Validate(docCopy, workflowDir, companionsCopy)
		items := make([]findingResult, len(errs))
		for i, e := range errs {
			items[i] = findingResult{
				text:     e.Error(),
				isFatal:  e.IsFatal(),
				stepName: e.StepName,
			}
		}
		return validateCompleteMsg{items: items}
	}
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

// makeLoadCmd returns a tea.Cmd that loads the workflow from the current
// workflowDir. On parse error, rawBytes is set (routes to DialogRecovery);
// on other errors, only err is set (routes to DialogError).
func (m Model) makeLoadCmd() tea.Cmd {
	workflowDir := m.workflowDir
	return func() tea.Msg {
		result, err := workflowio.Load(workflowDir)
		if err != nil {
			return openFileResultMsg{err: err}
		}
		if result.RecoveryView != nil {
			return openFileResultMsg{
				err:      fmt.Errorf("workflowedit: parse error in config.json"),
				rawBytes: result.RecoveryView,
			}
		}
		return openFileResultMsg{
			doc:         result.Doc,
			diskDoc:     result.Doc,
			companions:  result.Companions,
			workflowDir: workflowDir,
		}
	}
}

// safeToDelete checks whether path is safe to delete:
//   - path (after EvalSymlinks) is strictly inside workflowDir (containment)
//   - path is a regular file per os.Lstat (not a symlink, FIFO, socket, or dir)
func safeToDelete(workflowDir, path string) bool {
	realDir, err := filepath.EvalSymlinks(workflowDir)
	if err != nil {
		return false
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	if !strings.HasPrefix(realPath, realDir+string(filepath.Separator)) {
		return false
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode().IsRegular()
}

// saveErrorMsg returns a human-readable error message for a failed save result.
func saveErrorMsg(result workflowio.SaveResult) string {
	switch result.Kind {
	case workflowio.SaveErrorPermission:
		return "Permission denied saving workflow. Check file permissions and try again."
	case workflowio.SaveErrorDiskFull:
		return "Disk full. Free up space and try again."
	case workflowio.SaveErrorEXDEV:
		return "Cannot save across devices. The workflow directory must be on the same filesystem."
	case workflowio.SaveErrorSymlinkEscape:
		return "Save rejected: symlink would escape the workflow directory."
	case workflowio.SaveErrorTargetNotRegularFile:
		return "Save rejected: target is not a regular file (FIFO, socket, or device)."
	case workflowio.SaveErrorParse:
		if result.Err != nil {
			return "Marshal error serializing workflow: " + result.Err.Error()
		}
		return "Marshal error serializing workflow."
	case workflowio.SaveErrorConflictDetected:
		return "File conflict: the workflow was modified externally. Reload or overwrite."
	default:
		if result.Err != nil {
			return "Save error: " + result.Err.Error()
		}
		return "Save error."
	}
}

// deepCopyDoc returns a deep copy of doc so the caller can mutate it safely
// without affecting the original. Used by makeValidateCmd.
func deepCopyDoc(doc workflowmodel.WorkflowDoc) workflowmodel.WorkflowDoc {
	cp := workflowmodel.WorkflowDoc{
		DefaultModel: doc.DefaultModel,
	}
	if doc.StatusLine != nil {
		sl := *doc.StatusLine
		cp.StatusLine = &sl
	}
	if len(doc.Steps) > 0 {
		cp.Steps = make([]workflowmodel.Step, len(doc.Steps))
		for i, s := range doc.Steps {
			cs := s
			if len(s.Command) > 0 {
				cs.Command = make([]string, len(s.Command))
				copy(cs.Command, s.Command)
			}
			if len(s.Env) > 0 {
				cs.Env = make([]workflowmodel.EnvEntry, len(s.Env))
				copy(cs.Env, s.Env)
			}
			cp.Steps[i] = cs
		}
	}
	return cp
}

// snapshotCompanions returns a deep copy of the companions map.
func snapshotCompanions(m Model) map[string][]byte {
	if len(m.companions) == 0 {
		return nil
	}
	cp := make(map[string][]byte, len(m.companions))
	for k, v := range m.companions {
		bv := make([]byte, len(v))
		copy(bv, v)
		cp[k] = bv
	}
	return cp
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
