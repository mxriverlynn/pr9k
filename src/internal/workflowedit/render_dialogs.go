package workflowedit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mxriverlynn/pr9k/src/internal/uichrome"
)

// dialogBody holds the per-kind content for a dialog overlay (D-3, D-36, D-37).
type dialogBody struct {
	title  string   // rendered in the top border
	rows   []string // body content lines
	footer string   // D-37 button row; uses [ Label ] bracket grammar
	width  int      // hint; actual width clamped by renderDialogShell
}

// dialogBodyFor dispatches to the per-kind builder for the active dialog (D-3).
// The switch covers all 15 non-None DialogKind values.
func dialogBodyFor(kind DialogKind, payload any) dialogBody {
	switch kind {
	case DialogQuitConfirm:
		return buildQuitConfirmBody()
	case DialogFirstSaveConfirm:
		return buildFirstSaveConfirmBody()
	case DialogPathPicker:
		return buildPathPickerBody(payload)
	case DialogNewChoice:
		return buildNewChoiceBody()
	case DialogRecovery:
		return buildRecoveryBody(payload)
	case DialogAcknowledgeFindings:
		return buildAcknowledgeFindingsBody()
	case DialogFileConflict:
		return buildFileConflictBody()
	case DialogFindingsPanel:
		return buildFindingsPanelBody()
	case DialogSaveInProgress:
		return buildSaveInProgressBody()
	case DialogExternalEditorOpening:
		return buildExternalEditorOpeningBody()
	case DialogCopyBrokenRef:
		return buildCopyBrokenRefBody()
	case DialogRemoveConfirm:
		return buildRemoveConfirmBody(payload)
	case DialogUnsavedChanges:
		return buildUnsavedChangesBody()
	case DialogError:
		return buildErrorBody(payload)
	case DialogCrashTempNotice:
		return buildCrashTempNoticeBody(payload)
	}
	return dialogBody{}
}

// renderDialogShell renders the D-36 bordered dialog overlay.
// body.width is a hint; actual inner width is clamped to
// [DialogMinWidth, min(w-4, DialogMaxWidth)]. When the terminal is
// uninitialized (w <= 4), DialogMaxWidth is used as the fallback.
func renderDialogShell(body dialogBody, w, h int) string {
	innerW := dialogInnerWidth(body.width, w)

	lines := make([]string, 0, len(body.rows)+3)
	lines = append(lines, uichrome.RenderTopBorder(body.title, innerW+2))
	for _, row := range body.rows {
		lines = append(lines, dialogRow(row, innerW))
	}
	lines = append(lines, dialogRow(body.footer, innerW))
	lines = append(lines, uichrome.BottomBorder(innerW))

	return strings.Join(lines, "\n")
}

// dialogInnerWidth returns the clamped inner width for a dialog body.
// When the terminal width is <= 4 (uninitialized or too small), DialogMaxWidth
// is used. The result is always >= DialogMinWidth.
func dialogInnerWidth(desired, termW int) int {
	maxW := uichrome.DialogMaxWidth
	if termW > 4 {
		if termW-4 < maxW {
			maxW = termW - 4
		}
	}
	if maxW < uichrome.DialogMinWidth {
		maxW = uichrome.DialogMinWidth
	}
	if desired <= 0 {
		return maxW
	}
	if desired < uichrome.DialogMinWidth {
		return uichrome.DialogMinWidth
	}
	if desired > maxW {
		return maxW
	}
	return desired
}

// dialogRow renders one body line inside a dialog box: "│ content...padded │".
// Total visible width = innerW + 2 (content area + two border chars).
func dialogRow(content string, innerW int) string {
	if innerW < 4 {
		return "││"
	}
	contentW := innerW - 2 // 1-char left margin + 1-char right margin
	truncated := lipgloss.NewStyle().MaxWidth(contentW).Render(content)
	pad := contentW - lipgloss.Width(truncated)
	if pad < 0 {
		pad = 0
	}
	return "│ " + truncated + strings.Repeat(" ", pad) + " │"
}

// --- Per-kind body builders ---

func buildQuitConfirmBody() dialogBody {
	return dialogBody{
		title:  "Quit",
		rows:   []string{"Quit the workflow builder?"},
		footer: "[ y  Yes ]  [ Esc  No ]",
	}
}

func buildFirstSaveConfirmBody() dialogBody {
	return dialogBody{
		title:  "First Save",
		rows:   []string{"Save to external or symlinked workflow?"},
		footer: "[ y  yes ]  [ n  no ]",
	}
}

func buildPathPickerBody(payload any) dialogBody {
	picker, ok := payload.(pathPickerModel)
	if !ok {
		return dialogBody{
			title:  "Open Workflow",
			rows:   []string{"Path: "},
			footer: "[ Enter  Open ]  [ Esc  cancel ]",
		}
	}
	if picker.kind == PickerKindNew {
		rows := []string{"Path: " + picker.input}
		targetConfig := filepath.Join(strings.TrimSuffix(picker.input, "/"), "config.json")
		if _, err := os.Stat(targetConfig); err == nil {
			rows = append(rows, "Path already contains a config.json — will overwrite")
		}
		return dialogBody{
			title:  "New Workflow",
			rows:   rows,
			footer: "[ Enter  Create ]  [ Esc  cancel ]",
		}
	}
	return dialogBody{
		title:  "Open Workflow",
		rows:   []string{"Path: " + picker.input},
		footer: "[ Enter  Open ]  [ Esc  cancel ]",
	}
}

func buildNewChoiceBody() dialogBody {
	return dialogBody{
		title:  "New Workflow",
		rows:   []string{"Choose how to create the workflow:"},
		footer: "[ c  Copy ]  [ e  Empty ]  [ Esc  Cancel ]",
	}
}

func buildRecoveryBody(payload any) dialogBody {
	raw, _ := payload.(string)
	rows := []string{"The workflow config is malformed."}
	if raw != "" {
		rows = append(rows, raw)
	}
	return dialogBody{
		title:  "Recovery",
		rows:   rows,
		footer: "[ o  open editor ]  [ r  reload ]  [ d  discard ]  [ c  cancel ]",
	}
}

func buildAcknowledgeFindingsBody() dialogBody {
	return dialogBody{
		title:  "Validation Warnings",
		rows:   []string{"Validation warnings found."},
		footer: "[ Enter  acknowledge ]  [ Esc  cancel ]",
	}
}

func buildFileConflictBody() dialogBody {
	return dialogBody{
		title:  "File Conflict",
		rows:   []string{"The workflow file was changed on disk."},
		footer: "[ o  overwrite ]  [ r  reload ]  [ c  cancel ]",
	}
}

func buildFindingsPanelBody() dialogBody {
	return dialogBody{
		title:  "Findings",
		rows:   []string{"Validation Findings:"},
		footer: "[ Enter  acknowledge ]  [ Esc  close ]",
	}
}

func buildSaveInProgressBody() dialogBody {
	return dialogBody{
		title:  "Save in Progress",
		rows:   []string{"Save in progress — please wait."},
		footer: "[ Please wait… ]",
	}
}

func buildExternalEditorOpeningBody() dialogBody {
	return dialogBody{
		title:  "External Editor",
		rows:   []string{"Opening external editor — please wait."},
		footer: "[ Ctrl+C  cancel ]",
	}
}

func buildCopyBrokenRefBody() dialogBody {
	return dialogBody{
		title:  "Broken Reference",
		rows:   []string{"Default model reference is broken."},
		footer: "[ y  copy anyway ]  [ c  cancel ]",
	}
}

func buildRemoveConfirmBody(payload any) dialogBody {
	name, _ := payload.(string)
	return dialogBody{
		title:  "Delete Step",
		rows:   []string{fmt.Sprintf("Delete step %q?", name)},
		footer: "[ Del  Delete ]  [ Esc  Cancel ]",
	}
}

func buildUnsavedChangesBody() dialogBody {
	return dialogBody{
		title:  "Unsaved Changes",
		rows:   []string{"You have unsaved changes."},
		footer: "[ Ctrl+S  Save ]  [ Esc  Cancel ]  [ d  Discard ]",
	}
}

func buildErrorBody(payload any) dialogBody {
	msg, _ := payload.(string)
	return dialogBody{
		title:  "Error",
		rows:   []string{msg},
		footer: "[ Esc  close ]",
	}
}

func buildCrashTempNoticeBody(payload any) dialogBody {
	path, _ := payload.(string)
	return dialogBody{
		title:  "Crash Recovery",
		rows:   []string{"Crash temp file detected:", path},
		footer: "[ d  discard ]  [ l  leave ]",
	}
}
