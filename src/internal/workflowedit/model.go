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
	"github.com/charmbracelet/lipgloss"
	"github.com/mxriverlynn/pr9k/src/internal/uichrome"
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

func (b bannerState) allBannerTexts() []string {
	var out []string
	if b.isReadOnly {
		out = append(out, "[ro]")
	}
	if b.isExternalWorkflow {
		out = append(out, "[ext]")
	}
	if b.isSymlink {
		target := b.symlinkTarget
		if target != "" {
			out = append(out, "[sym → "+target+"]")
		} else {
			out = append(out, "[sym]")
		}
	}
	if b.isSharedInstall {
		out = append(out, "[shared]")
	}
	if b.hasUnknownField {
		out = append(out, "[?fields]")
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
	// bannerGen is incremented on each save success so stale clearSaveBannerMsg
	// ticks from prior saves are ignored (D-7).
	bannerGen int

	banners bannerState // active warning banners set from load-pipeline signals

	// boundaryFlash is non-zero when the outline should invert the cursor row
	// to signal a phase-boundary decline. A clearBoundaryFlashMsg with the same
	// seq value resets it to 0 (D-12).
	boundaryFlash uint64

	logW io.Writer // session-event log destination; nil disables logging

	// nowFn returns the current time; defaults to time.Now. Replaced in tests
	// to avoid time.Sleep (D-8).
	nowFn func() time.Time

	// validateFn overrides the real validator when non-nil; used by tests.
	validateFn func(workflowmodel.WorkflowDoc, string, map[string][]byte) []findingResult

	// lastValidateOK tracks the outcome of the most recent validation run.
	// nil = no validation has run yet; &true = last run had no fatal findings;
	// &false = last run had fatal findings.
	lastValidateOK *bool
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
		nowFn:       time.Now,
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

// resetSecretMask resets the detail pane's revealedField to -1 (masked) so that
// sensitive values are not visible after any dialog opens (D-13).
func resetSecretMask(m Model) Model {
	m.detail.revealedField = -1
	return m
}

// openDialog sets the active dialog, resetting the detail pane's revealed secret
// mask so sensitive values are never visible across a dialog boundary (D-13).
func openDialog(m Model, ds dialogState) Model {
	m = resetSecretMask(m)
	m.logEvent(fmtDialogOpen(ds.kind))
	m.dialog = ds
	return m
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

func fmtResize(w, h int) string {
	return fmt.Sprintf("resize width=%d height=%d", w, h)
}

func fmtDialogOpen(kind DialogKind) string {
	return "dialog_open kind=" + dialogKindName(kind)
}

func fmtDialogClose(kind DialogKind) string {
	return "dialog_close kind=" + dialogKindName(kind)
}

func fmtFocusChanged(focus focusTarget) string {
	return "focus_changed focus=" + focusTargetName(focus)
}

func fmtValidateComplete(ok bool) string {
	if ok {
		return "validate_complete ok=true"
	}
	return "validate_complete ok=false"
}

// dialogKindName returns a short snake_case name for a DialogKind value.
func dialogKindName(k DialogKind) string {
	switch k {
	case DialogPathPicker:
		return "path_picker"
	case DialogNewChoice:
		return "new_choice"
	case DialogUnsavedChanges:
		return "unsaved_changes"
	case DialogQuitConfirm:
		return "quit_confirm"
	case DialogExternalEditorOpening:
		return "external_editor_opening"
	case DialogFindingsPanel:
		return "findings_panel"
	case DialogError:
		return "error"
	case DialogCrashTempNotice:
		return "crash_temp_notice"
	case DialogFirstSaveConfirm:
		return "first_save_confirm"
	case DialogRemoveConfirm:
		return "remove_confirm"
	case DialogFileConflict:
		return "file_conflict"
	case DialogSaveInProgress:
		return "save_in_progress"
	case DialogRecovery:
		return "recovery"
	case DialogAcknowledgeFindings:
		return "acknowledge_findings"
	case DialogCopyBrokenRef:
		return "copy_broken_ref"
	default:
		return fmt.Sprintf("unknown_%d", int(k))
	}
}

// focusTargetName returns a short name for a focusTarget value.
func focusTargetName(f focusTarget) string {
	switch f {
	case focusOutline:
		return "outline"
	case focusDetail:
		return "detail"
	case focusMenu:
		return "menu"
	default:
		return "unknown"
	}
}

// Init satisfies tea.Model. No startup commands are needed.
func (m Model) Init() tea.Cmd { return nil }

// Update satisfies tea.Model. The routing order (D-9 + D-PR2-19 + D-14) is:
//  0. typed-message pre-dispatch — validateCompleteMsg | saveCompleteMsg | quitMsg |
//     clearBoundaryFlashMsg
//     0b. WindowSizeMsg — always handled first to keep dimensions current, then routing
//     continues to the active tier so dialogs/help remain in control.
//  1. helpOpen  → updateHelpModal
//  2. dialog    → updateDialog
//  3. globalKey → handleGlobalKey
//  4. default   → updateEditView
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Tier-0 pre-dispatch: these typed messages bypass dialog/help state so
	// that save completions, load results, and programmatic quit are never
	// swallowed by an active dialog (D-PR2-19).
	switch msg.(type) {
	case validateCompleteMsg, saveCompleteMsg, openFileResultMsg, quitMsg, clearBoundaryFlashMsg, clearSaveBannerMsg:
		return m.updateAsyncCompletion(msg)
	}

	// Tier-0 WindowSizeMsg (D-14): always update dimensions first so the chrome
	// frame stays correct even while a dialog or help modal is active, then
	// fall through to the active tier which typically ignores non-key messages.
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		next, _ := m.handleWindowSize(wsm)
		m = next.(Model)
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

// updateAsyncCompletion handles the pre-dispatched message types that bypass
// dialog/help state (tier-0, D-PR2-19).
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
	case clearBoundaryFlashMsg:
		if msg.seq == m.boundaryFlash {
			m.boundaryFlash = 0
		}
		return m, nil
	case clearSaveBannerMsg:
		if msg.gen == m.bannerGen {
			m.saveBanner = ""
			m.logEvent("save_banner_cleared")
		}
		return m, nil
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

// --- rendering helpers ---

func (m Model) renderDialog() string {
	body := dialogBodyFor(m.dialog.kind, m.dialog.payload)
	return renderDialogShell(body, m.width, m.height)
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
	columns := lipgloss.JoinHorizontal(lipgloss.Top, outlineStr, detailStr)
	return header + "\n" + columns
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
			m.logEvent(fmtDialogClose(m.dialog.kind))
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
		// show a not-yet-implemented error (full copy wiring deferred to PR-3).
		_, err := workflowmodel.CopyFromDefault(m.workflowDir)
		if err != nil {
			m = openDialog(m, dialogState{kind: DialogCopyBrokenRef})
			return m, nil
		}
		m = openDialog(m, dialogState{
			kind:    DialogError,
			payload: "Copy from default not yet implemented — use Empty or open an existing workflow",
		})
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
		m.logEvent(fmtDialogClose(m.dialog.kind))
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
			m = openDialog(m, dialogState{kind: DialogFindingsPanel})
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
			m = openDialog(m, dialogState{kind: DialogError, payload: "Cannot delete: path is outside workflow directory or is not a regular file."})
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
		projectDir := m.projectDir
		m = openDialog(m, dialogState{kind: DialogExternalEditorOpening})
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
			// Populate all five banner-signal fields (D-15); errors treated as false.
			isReadOnly, _ := workflowio.DetectReadOnly(workflowDir)
			isExternal := workflowio.DetectExternalWorkflow(workflowDir, projectDir)
			isSharedInstall, _ := workflowio.DetectSharedInstall(workflowDir)
			return openFileResultMsg{
				doc:             result.Doc,
				diskDoc:         result.Doc,
				companions:      result.Companions,
				workflowDir:     workflowDir,
				isSymlink:       result.IsSymlink,
				symlinkTarget:   result.SymlinkTarget,
				isReadOnly:      isReadOnly,
				isExternal:      isExternal,
				isSharedInstall: isSharedInstall,
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
		// Copy anyway: full copy wiring deferred to PR-3; show explicit message
		// so the user knows the action did not complete silently.
		m = openDialog(m, dialogState{
			kind:    DialogError,
			payload: "Copy from default not yet implemented — use Empty or open an existing workflow",
		})
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
		m = openDialog(m, dialogState{kind: DialogNewChoice})
	case tea.KeyCtrlO:
		m.prevFocus = m.focus
		defaultPath := filepath.Join(m.projectDir, ".pr9k", "workflow", "config.json")
		m = openDialog(m, dialogState{kind: DialogPathPicker, payload: newPathPicker(defaultPath)})
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
				m = openDialog(m, dialogState{kind: DialogFileConflict})
				return m, nil
			}
		}
		m.forceSave = false // clear after use
		m.validateInProgress = true
		m.logEvent("validate_started")
		return m, m.makeValidateCmd()
	case tea.KeyCtrlQ:
		if m.saveInProgress || m.validateInProgress {
			m.prevFocus = m.focus
			m = openDialog(m, dialogState{kind: DialogSaveInProgress})
			m.pendingQuit = true
			return m, nil
		}
		m.prevFocus = m.focus
		if m.dirty {
			m = openDialog(m, dialogState{kind: DialogUnsavedChanges})
		} else {
			m = openDialog(m, dialogState{kind: DialogQuitConfirm})
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
		var cmd tea.Cmd
		switch km.Type {
		case tea.KeyUp:
			m, cmd = doMoveStepUp(m)
			if cmd == nil {
				m.dirty = true
			}
		case tea.KeyDown:
			m, cmd = doMoveStepDown(m)
			if cmd == nil {
				m.dirty = true
			}
		}
		return m, cmd
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
			m.logEvent(fmtFocusChanged(focusDetail))
		}
	case tea.KeyDelete:
		stepIdx := cursorStepIdx(rows, m.outline.cursor)
		if stepIdx >= 0 {
			name := m.doc.Steps[stepIdx].Name
			m.prevFocus = m.focus
			m = openDialog(m, dialogState{kind: DialogRemoveConfirm, payload: name})
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
		m.doc.ContainerEnv["NEW_KEY"] = ""
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
	var cmd tea.Cmd
	switch km.Type {
	case tea.KeyUp:
		m, cmd = doMoveStepUp(m)
	case tea.KeyDown:
		m, cmd = doMoveStepDown(m)
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
	return m, cmd
}

// boundaryDeclineCmd returns a tea.Cmd that fires clearBoundaryFlashMsg after
// 150ms. seq must equal m.boundaryFlash at the time of the decline so stale
// ticks from earlier declines are ignored (D-12).
func boundaryDeclineCmd(seq uint64) tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(_ time.Time) tea.Msg {
		return clearBoundaryFlashMsg{seq: seq}
	})
}

// doMoveStepUp swaps the step at cursor with the previous step in doc.Steps.
// Returns (Model, nil) on success or (Model, clearBoundaryFlashMsg cmd) when the
// swap would cross a phase boundary (D-12).
func doMoveStepUp(m Model) (Model, tea.Cmd) {
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	i := cursorStepIdx(rows, m.outline.cursor)
	if i <= 0 {
		return m, nil
	}
	// Phase-boundary guard (D-12): decline the swap if phases differ.
	if m.doc.Steps[i].Phase != m.doc.Steps[i-1].Phase {
		m.boundaryFlash++
		m.logEvent("phase_boundary_decline")
		return m, boundaryDeclineCmd(m.boundaryFlash)
	}
	steps := make([]workflowmodel.Step, len(m.doc.Steps))
	copy(steps, m.doc.Steps)
	steps[i], steps[i-1] = steps[i-1], steps[i]
	m.doc.Steps = steps
	// Move cursor to the swapped step's new position.
	m.outline.cursor = findStepCursorByIdx(m, i-1)
	return m, nil
}

// doMoveStepDown swaps the step at cursor with the next step in doc.Steps.
// Returns (Model, nil) on success or (Model, clearBoundaryFlashMsg cmd) when the
// swap would cross a phase boundary (D-12).
func doMoveStepDown(m Model) (Model, tea.Cmd) {
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	i := cursorStepIdx(rows, m.outline.cursor)
	if i < 0 || i >= len(m.doc.Steps)-1 {
		return m, nil
	}
	// Phase-boundary guard (D-12): decline the swap if phases differ.
	if m.doc.Steps[i].Phase != m.doc.Steps[i+1].Phase {
		m.boundaryFlash++
		m.logEvent("phase_boundary_decline")
		return m, boundaryDeclineCmd(m.boundaryFlash)
	}
	steps := make([]workflowmodel.Step, len(m.doc.Steps))
	copy(steps, m.doc.Steps)
	steps[i], steps[i+1] = steps[i+1], steps[i]
	m.doc.Steps = steps
	m.outline.cursor = findStepCursorByIdx(m, i+1)
	return m, nil
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
		if m.detail.revealedField >= 0 {
			m.logEvent("secret_remasked")
		}
		m.focus = focusOutline
		m.detail.revealedField = -1 // re-mask on focus-leave (D-47)
		m.detail.modelSuggFocus = false
		m.logEvent(fmtFocusChanged(focusOutline))

	case tea.KeyCtrlE:
		// Ctrl+E on a multiline field opens the companion file in the external
		// editor so the user can edit multi-line content (D-16).
		if stepIdx >= 0 && stepIdx < len(m.doc.Steps) {
			fields := buildDetailFields(m.doc.Steps[stepIdx])
			if m.detail.cursor < len(fields) && fields[m.detail.cursor].kind == fieldKindMultiLine {
				step := m.doc.Steps[stepIdx]
				filePath := multiLineFilePath(m.workflowDir, step, fields[m.detail.cursor])
				if filePath != "" {
					m = openDialog(m, dialogState{kind: DialogExternalEditorOpening})
					reloadCmd := m.makeLoadCmd()
					return m, m.editor.Run(filePath, func(_ error) tea.Msg {
						return reloadCmd()
					})
				}
			}
		}

	case tea.KeyDown:
		if stepIdx >= 0 && stepIdx < len(m.doc.Steps) {
			fields := buildDetailFields(m.doc.Steps[stepIdx])
			if m.detail.cursor < len(fields) && fields[m.detail.cursor].kind == fieldKindModelSuggest {
				// First Down from model field moves focus into suggestion list.
				m.detail.modelSuggFocus = true
				m.detail.modelSuggIdx = 0
				return m, nil
			}
			if m.detail.cursor < len(fields)-1 {
				m.detail.cursor++
			}
		} else {
			m.detail.cursor++
		}

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
					m.logEvent("secret_remasked")
				} else {
					m.detail.revealedField = m.detail.cursor
					m.logEvent("secret_revealed")
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
		var ok bool
		m, ok = commitDetailEdit(m)
		if ok {
			m.detail.editing = false
			m.detail.editMsg = ""
		}
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
// Returns (updated model, true) on success, or (model with editMsg set, false)
// when the commit is rejected (e.g. Command round-trip would corrupt quoted args).
func commitDetailEdit(m Model) (Model, bool) {
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	stepIdx := cursorStepIdx(rows, m.outline.cursor)
	if stepIdx < 0 || stepIdx >= len(m.doc.Steps) {
		return m, true
	}
	fields := buildDetailFields(m.doc.Steps[stepIdx])
	if m.detail.cursor >= len(fields) {
		return m, true
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
		// Reject the commit if any original argv element contains whitespace:
		// strings.Join→Fields is lossy for such elements and would silently corrupt
		// the command (e.g. ["bash","-c","echo hello"] → ["bash","-c","echo","hello"]).
		for _, arg := range step.Command {
			if strings.ContainsAny(arg, " \t\n\r") {
				m.detail.editMsg = "Command has quoted args — edit in external editor (Ctrl+E)"
				return m, false
			}
		}
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
			} else {
				m.detail.editMsg = "Expected key=value format"
				return m, false
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
	return m, true
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
	m.logEvent(fmtResize(msg.Width, msg.Height))
	if msg.Width < uichrome.MinTerminalWidth || msg.Height < uichrome.MinTerminalHeight {
		m.logEvent("terminal_too_small")
	}
	ow := outlineWidth(m.width)
	dw := m.width - ow
	if dw < 1 {
		dw = 1
	}
	panelH := m.height - ChromeRows // D-20: subtract full chrome budget
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
	ok := true
	for _, item := range msg.items {
		if item.isFatal {
			ok = false
			break
		}
	}
	m.lastValidateOK = &ok
	m.logEvent(fmtValidateComplete(ok))
	for _, item := range msg.items {
		if item.isFatal {
			// Fatal findings block save.
			m.prevFocus = m.focus
			m.findingsPanel = buildFindingsPanel(msg.items, m.doc.Steps, m.findingsPanel)
			m = openDialog(m, dialogState{kind: DialogFindingsPanel})
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
			m = openDialog(m, dialogState{kind: DialogAcknowledgeFindings})
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
		m = openDialog(m, dialogState{kind: DialogError, payload: saveErrorMsg(msg.result)})
		m.pendingQuit = false
		return m, nil
	}
	// Success.
	m.dirty = false
	m.saveSnapshot = msg.result.Snapshot
	if msg.result.Snapshot != nil {
		m.diskDoc = m.doc
	}
	m.saveBanner = "Saved at " + m.nowFn().Format("15:04:05")
	m.logEvent("save_banner_set")
	m.bannerGen++
	clearGen := m.bannerGen
	bannerCmd := tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return clearSaveBannerMsg{gen: clearGen}
	})
	if m.pendingQuit {
		m.pendingQuit = false
		// D-PR2-10: re-route to QuitConfirm so user explicitly confirms exit.
		// dirty=false now, so handleGlobalKey will open DialogQuitConfirm.
		nextM, nextCmd := m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyCtrlQ})
		return nextM, tea.Batch(bannerCmd, nextCmd)
	}
	return m, bannerCmd
}

func (m Model) handleOpenFileResult(msg openFileResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.prevFocus = m.focus
		if len(msg.rawBytes) > 0 {
			// Parse error: show recovery dialog with raw bytes.
			m = openDialog(m, dialogState{kind: DialogRecovery, payload: string(msg.rawBytes)})
		} else {
			// Other error (permission, not found, etc.): show error dialog.
			m = openDialog(m, dialogState{kind: DialogError, payload: msg.err.Error()})
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
// All five banner-signal fields are populated from workflowio detect functions (D-15).
func (m Model) makeLoadCmd() tea.Cmd {
	workflowDir := m.workflowDir
	projectDir := m.projectDir
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
		// Populate all five banner-signal fields (D-15); errors are treated as false.
		isReadOnly, _ := workflowio.DetectReadOnly(workflowDir)
		isExternal := workflowio.DetectExternalWorkflow(workflowDir, projectDir)
		isSharedInstall, _ := workflowio.DetectSharedInstall(workflowDir)
		return openFileResultMsg{
			doc:             result.Doc,
			diskDoc:         result.Doc,
			companions:      result.Companions,
			workflowDir:     workflowDir,
			isSymlink:       result.IsSymlink,
			symlinkTarget:   result.SymlinkTarget,
			isReadOnly:      isReadOnly,
			isExternal:      isExternal,
			isSharedInstall: isSharedInstall,
		}
	}
}

// multiLineFilePath returns the companion file path for a fieldKindMultiLine field,
// or "" when no file path can be derived (e.g. the field value is empty). The
// returned path is absolute under workflowDir.
func multiLineFilePath(workflowDir string, step workflowmodel.Step, f detailField) string {
	switch f.label {
	case "PromptFile":
		if step.PromptFile == "" {
			return ""
		}
		return filepath.Join(workflowDir, step.PromptFile)
	case "Command":
		if len(step.Command) > 0 && step.Command[0] != "" {
			return filepath.Join(workflowDir, step.Command[0])
		}
		return ""
	}
	return ""
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
	if len(doc.Env) > 0 {
		cp.Env = make([]string, len(doc.Env))
		copy(cp.Env, doc.Env)
	}
	if len(doc.ContainerEnv) > 0 {
		cp.ContainerEnv = make(map[string]string, len(doc.ContainerEnv))
		for k, v := range doc.ContainerEnv {
			cp.ContainerEnv[k] = v
		}
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
