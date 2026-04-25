package workflowedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// fieldCursorFor returns the detail pane cursor index for a field with the
// given label in the field list built from step. Panics if not found.
func fieldCursorFor(step workflowmodel.Step, label string) int {
	for i, f := range buildDetailFields(step) {
		if f.label == label {
			return i
		}
	}
	panic("field not found: " + label)
}

// detailModelOnStep returns a model with focus on detail, outline cursor on
// the step row (row 3 for a single iteration step), and step pre-loaded.
func detailModelOnStep(steps ...workflowmodel.Step) Model {
	m := newLoadedModel(steps...)
	m.focus = focusDetail
	m.outline.cursor = 3 // step at flat row 3
	return m
}

// ──────────────────────────────────────────────
// Plain text field sanitization
// ──────────────────────────────────────────────

// TestDetailPane_PlainTextField_StripsNewlinesOnInput verifies that newlines
// are stripped from paste input on a plain text field.
func TestDetailPane_PlainTextField_StripsNewlinesOnInput(t *testing.T) {
	step := workflowmodel.Step{Name: "", Kind: workflowmodel.StepKindShell, Command: []string{"echo"}}
	m := detailModelOnStep(step)
	m.detail.cursor = fieldCursorFor(step, "Name")

	m = applyKey(m, keyEnter()) // enter edit mode
	if !m.detail.editing {
		t.Fatal("Enter should start editing on plain text field")
	}

	// Multi-rune paste with newline
	m = applyMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello\nworld")})
	if strings.Contains(m.detail.editBuf, "\n") {
		t.Errorf("editBuf should not contain newlines, got %q", m.detail.editBuf)
	}
	if m.detail.editBuf != "helloworld" {
		t.Errorf("want editBuf=helloworld, got %q", m.detail.editBuf)
	}
}

// TestDetailPane_PlainTextField_StripsANSIEscapesOnInput verifies that ANSI
// escape sequences are stripped from input on a plain text field.
func TestDetailPane_PlainTextField_StripsANSIEscapesOnInput(t *testing.T) {
	step := workflowmodel.Step{Name: "", Kind: workflowmodel.StepKindShell, Command: []string{"echo"}}
	m := detailModelOnStep(step)
	m.detail.cursor = fieldCursorFor(step, "Name")

	m = applyKey(m, keyEnter())
	if !m.detail.editing {
		t.Fatal("Enter should start editing on plain text field")
	}

	// ESC [ 3 1 m r e d ESC [ 0 m — multi-rune triggers paste path
	m = applyMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("\x1b[31mred\x1b[0m")})
	if strings.Contains(m.detail.editBuf, "\x1b") {
		t.Errorf("editBuf should not contain ANSI escapes, got %q", m.detail.editBuf)
	}
	if !strings.Contains(m.detail.editBuf, "red") {
		t.Errorf("want 'red' in editBuf after ANSI strip, got %q", m.detail.editBuf)
	}
}

// ──────────────────────────────────────────────
// Choice-list dropdown
// ──────────────────────────────────────────────

// TestDetailPane_ChoiceList_OpenWithEnter_NavigateWithArrows_ConfirmWithEnter
// verifies Enter opens the dropdown, arrows navigate, and Enter confirms.
func TestDetailPane_ChoiceList_OpenWithEnter_NavigateWithArrows_ConfirmWithEnter(t *testing.T) {
	step := sampleStep("s")
	m := detailModelOnStep(step)
	m.detail.cursor = fieldCursorFor(step, "CaptureMode")

	// Enter opens the dropdown.
	m = applyKey(m, keyEnter())
	if !m.detail.dropdownOpen {
		t.Fatal("Enter on CaptureMode should open choice dropdown")
	}
	if len(m.detail.choiceOptions) == 0 {
		t.Fatal("choiceOptions should be set when dropdown is open")
	}

	// Arrow Down navigates.
	m = applyKey(m, keyDown())
	if m.detail.choiceIdx != 1 {
		t.Errorf("want choiceIdx=1 after Down, got %d", m.detail.choiceIdx)
	}

	// Enter confirms selection.
	m = applyKey(m, keyEnter())
	if m.detail.dropdownOpen {
		t.Error("dropdown should close after Enter")
	}
	if m.doc.Steps[0].CaptureMode != "fullStdout" {
		t.Errorf("want CaptureMode=fullStdout, got %q", m.doc.Steps[0].CaptureMode)
	}
	if !m.dirty {
		t.Error("model should be dirty after choice selection")
	}
}

// TestDetailPane_ChoiceList_TypedCharJumpsToOption verifies that typing the
// first letter of an option jumps to it while the dropdown is open.
func TestDetailPane_ChoiceList_TypedCharJumpsToOption(t *testing.T) {
	step := sampleStep("s")
	m := detailModelOnStep(step)
	m.detail.cursor = fieldCursorFor(step, "CaptureMode")

	// Open the dropdown (choiceIdx starts at 0 = lastLine).
	m = applyKey(m, keyEnter())
	if !m.detail.dropdownOpen {
		t.Fatal("Enter should open dropdown")
	}

	// Type 'f' — should jump to fullStdout (index 1).
	m = applyKey(m, keyRune('f'))
	if m.detail.choiceIdx != 1 {
		t.Errorf("want choiceIdx=1 (fullStdout) after typing 'f', got %d", m.detail.choiceIdx)
	}
}

// ──────────────────────────────────────────────
// Numeric fields
// ──────────────────────────────────────────────

// TestDetailPane_NumericField_NonDigitsTypedSilentlyIgnored verifies that
// a single non-digit keystroke is silently dropped (no message).
func TestDetailPane_NumericField_NonDigitsTypedSilentlyIgnored(t *testing.T) {
	step := sampleStep("s")
	m := detailModelOnStep(step)
	m.detail.cursor = fieldCursorFor(step, "TimeoutSeconds")

	m = applyKey(m, keyEnter())
	if !m.detail.editing {
		t.Fatal("Enter should open numeric editing for TimeoutSeconds")
	}

	// Single non-digit — silently ignored.
	m = applyMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.detail.editBuf != "" {
		t.Errorf("non-digit should be silently ignored, editBuf=%q", m.detail.editBuf)
	}
	if m.detail.editMsg != "" {
		t.Errorf("no message should appear for typed non-digit, got %q", m.detail.editMsg)
	}
}

// TestDetailPane_NumericField_PastedContent_StripsAndShowsMessage verifies
// that a paste containing non-digits is truncated at the first non-digit and
// a "pasted content sanitized" message is set.
func TestDetailPane_NumericField_PastedContent_StripsAndShowsMessage(t *testing.T) {
	step := sampleStep("s")
	m := detailModelOnStep(step)
	m.detail.cursor = fieldCursorFor(step, "TimeoutSeconds")

	m = applyKey(m, keyEnter())

	// Multi-rune paste heuristic: "1a2b3" → strip at first non-digit → "1".
	m = applyMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1a2b3")})
	if m.detail.editBuf != "1" {
		t.Errorf("paste should strip at first non-digit, got %q", m.detail.editBuf)
	}
	if m.detail.editMsg != "pasted content sanitized" {
		t.Errorf("want 'pasted content sanitized' message, got %q", m.detail.editMsg)
	}
}

// TestDetailPane_NumericField_RangeClampOnInput verifies that a value exceeding
// the field max is clamped to the maximum.
func TestDetailPane_NumericField_RangeClampOnInput(t *testing.T) {
	step := sampleStep("s")
	m := detailModelOnStep(step)
	m.detail.cursor = fieldCursorFor(step, "TimeoutSeconds") // max=3600

	m = applyKey(m, keyEnter())

	// Type "99999" digit by digit (each single rune is not a paste).
	for _, r := range "99999" {
		m = applyMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.detail.editBuf != "3600" {
		t.Errorf("want editBuf=3600 after clamping, got %q", m.detail.editBuf)
	}
}

// ──────────────────────────────────────────────
// Secret masking — key-pattern gate
// ──────────────────────────────────────────────

// TestDetailPane_SecretMask_KeyPatternMatched_ValueMasked verifies that a
// containerEnv entry whose key contains a secret suffix is rendered masked.
func TestDetailPane_SecretMask_KeyPatternMatched_ValueMasked(t *testing.T) {
	step := workflowmodel.Step{
		Name:    "s",
		Kind:    workflowmodel.StepKindShell,
		Command: []string{"echo"},
		Env:     []workflowmodel.EnvEntry{{Key: "API_TOKEN", Value: "tok_secret", IsLiteral: true}},
	}
	m := detailModelOnStep(step)
	m.detail.cursor = fieldCursorFor(step, "containerEnv[0]")

	view := m.View()
	if strings.Contains(view, "tok_secret") {
		t.Error("secret value should be masked when key matches _TOKEN pattern")
	}
	if !strings.Contains(view, GlyphMasked) {
		t.Errorf("view should contain mask glyph for _TOKEN key, got %q", view)
	}
}

// TestDetailPane_SecretMask_KeyPatternUnmatched_ValueVisible verifies that a
// containerEnv entry whose key does not match any secret suffix is not masked.
func TestDetailPane_SecretMask_KeyPatternUnmatched_ValueVisible(t *testing.T) {
	step := workflowmodel.Step{
		Name:    "s",
		Kind:    workflowmodel.StepKindShell,
		Command: []string{"echo"},
		Env:     []workflowmodel.EnvEntry{{Key: "NODE_ENV", Value: "production", IsLiteral: true}},
	}
	m := detailModelOnStep(step)
	m.detail.cursor = fieldCursorFor(step, "containerEnv[0]")

	view := m.View()
	if !strings.Contains(view, "production") {
		t.Errorf("non-secret value should be visible, view: %q", view)
	}
	if strings.Contains(view, GlyphMasked) {
		t.Error("NODE_ENV should not be masked")
	}
}

// TestDetailPane_SecretMask_RevealOnR_RemaskOnFocusLeave verifies that 'r'
// reveals a secret-masked field and Tab (focus-leave) re-masks it.
func TestDetailPane_SecretMask_RevealOnR_RemaskOnFocusLeave(t *testing.T) {
	step := workflowmodel.Step{
		Name:    "s",
		Kind:    workflowmodel.StepKindShell,
		Command: []string{"echo"},
		Env:     []workflowmodel.EnvEntry{{Key: "MY_TOKEN", Value: "secret123", IsLiteral: true}},
	}
	m := detailModelOnStep(step)
	cursor := fieldCursorFor(step, "containerEnv[0]")
	m.detail.cursor = cursor

	// Initially masked.
	view := m.View()
	if strings.Contains(view, "secret123") {
		t.Error("value should be masked initially")
	}
	if !strings.Contains(view, GlyphMasked) {
		t.Errorf("masked field should show %q initially", GlyphMasked)
	}

	// Press 'r' to reveal.
	m = applyKey(m, keyRune('r'))
	if m.detail.revealedField != cursor {
		t.Errorf("want revealedField=%d after r, got %d", cursor, m.detail.revealedField)
	}

	// Tab to outline re-masks (focus-leave).
	m = applyKey(m, keyTab())
	if m.focus != focusOutline {
		t.Fatalf("want focusOutline after Tab, got %d", m.focus)
	}
	if m.detail.revealedField != -1 {
		t.Errorf("want revealedField=-1 after focus-leave, got %d", m.detail.revealedField)
	}
}

// ──────────────────────────────────────────────
// Model suggestion field
// ──────────────────────────────────────────────

// TestDetailPane_ModelField_SuggestionsAppearOnFocus verifies that the model
// suggestion list appears in the view when the Model field is focused.
func TestDetailPane_ModelField_SuggestionsAppearOnFocus(t *testing.T) {
	step := workflowmodel.Step{
		Name:        "s",
		Kind:        workflowmodel.StepKindClaude,
		IsClaudeSet: true,
		PromptFile:  "prompts/s.md",
		Model:       "my-current-model",
	}
	m := detailModelOnStep(step)
	m.detail.cursor = fieldCursorFor(step, "Model")

	view := m.View()
	found := false
	for _, sug := range workflowmodel.ModelSuggestions {
		if strings.Contains(view, sug) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("suggestions should appear when Model field is focused, view: %q", view)
	}
}

// TestDetailPane_ModelField_FreeTextValueOutsideSuggestionList_Accepted verifies
// that a custom model name (not in the suggestion list) is accepted and written
// to the doc when editing is committed.
func TestDetailPane_ModelField_FreeTextValueOutsideSuggestionList_Accepted(t *testing.T) {
	step := workflowmodel.Step{
		Name:        "s",
		Kind:        workflowmodel.StepKindClaude,
		IsClaudeSet: true,
		PromptFile:  "prompts/s.md",
		// Model is empty so editBuf starts empty.
	}
	m := detailModelOnStep(step)
	m.detail.cursor = fieldCursorFor(step, "Model")

	// Enter opens text editing mode.
	m = applyKey(m, keyEnter())
	if !m.detail.editing {
		t.Fatal("Enter should start editing on Model field")
	}

	// Type a custom model name (multi-rune = paste heuristic, but plain text).
	m = applyMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("my-custom-model")})

	// Confirm with Enter.
	m = applyKey(m, keyEnter())
	if m.detail.editing {
		t.Error("editing should end after Enter confirmation")
	}
	if m.doc.Steps[0].Model != "my-custom-model" {
		t.Errorf("want Model=my-custom-model, got %q", m.doc.Steps[0].Model)
	}
	if !m.dirty {
		t.Error("model should be dirty after editing")
	}
}

// ──────────────────────────────────────────────
// Path picker PickerKind context
// ──────────────────────────────────────────────

// TestPathPicker_NewContext_PreFillIsWorkflowDir verifies that a PickerKindNew
// picker is pre-filled with the workflow directory (directory, not config.json).
func TestPathPicker_NewContext_PreFillIsWorkflowDir(t *testing.T) {
	m := newTestModel()
	picker := newPathPickerForNew(m.projectDir)
	m.dialog = dialogState{kind: DialogPathPicker, payload: picker}

	p := m.dialog.payload.(pathPickerModel)
	if strings.HasSuffix(p.input, "config.json") {
		t.Errorf("New context pre-fill should be workflow dir (not config.json), got %q", p.input)
	}
	if !strings.Contains(p.input, ".pr9k") {
		t.Errorf("New context pre-fill should be under .pr9k, got %q", p.input)
	}
	if p.kind != PickerKindNew {
		t.Errorf("want PickerKindNew, got %d", p.kind)
	}
}

// TestPathPicker_NewContext_ButtonLabelIsCreate verifies that a PickerKindNew
// picker shows "Create" rather than "Open" in the dialog render.
func TestPathPicker_NewContext_ButtonLabelIsCreate(t *testing.T) {
	m := newTestModel()
	picker := newPathPickerForNew(m.projectDir)
	m.dialog = dialogState{kind: DialogPathPicker, payload: picker}

	view := m.View()
	if !strings.Contains(view, "Create") {
		t.Errorf("New context picker should show 'Create', view: %q", view)
	}
}

// TestPathPicker_OpenContext_PreFillIsConfigJson verifies that the Ctrl+O path
// picker is pre-filled with the default config.json path.
func TestPathPicker_OpenContext_PreFillIsConfigJson(t *testing.T) {
	m := newTestModel()
	m = applyKey(m, keyCtrlO())

	picker, ok := m.dialog.payload.(pathPickerModel)
	if !ok {
		t.Fatal("expected pathPickerModel payload after Ctrl+O")
	}
	if !strings.HasSuffix(picker.input, "config.json") {
		t.Errorf("Open picker pre-fill should end with config.json, got %q", picker.input)
	}
	if picker.kind != PickerKindOpen {
		t.Errorf("Ctrl+O should use PickerKindOpen, got %d", picker.kind)
	}
}

// TestPathPicker_NewContext_ExistingConfigWarns verifies that the New-context
// picker warns when the target directory already contains a config.json.
func TestPathPicker_NewContext_ExistingConfigWarns(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	m := newTestModel()
	picker := pathPickerModel{
		kind:  PickerKindNew,
		input: dir + "/",
	}
	m.dialog = dialogState{kind: DialogPathPicker, payload: picker}

	view := m.View()
	if !strings.Contains(view, "overwrite") && !strings.Contains(view, "already") {
		t.Errorf("New picker should warn about existing config.json, view: %q", view)
	}
}
