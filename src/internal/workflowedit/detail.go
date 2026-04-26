package workflowedit

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/mxriverlynn/pr9k/src/internal/ansi"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// detailFieldKind classifies the editing behaviour of each detail pane field.
type detailFieldKind int

const (
	fieldKindText         detailFieldKind = iota // single-line plain text
	fieldKindChoice                              // constrained choice list (▾)
	fieldKindNumeric                             // integer with min/max clamping
	fieldKindModelSuggest                        // free text + suggestion overlay
	fieldKindSecretMask                          // masked unless key-pattern matched
)

// detailField describes one editable row in the detail pane.
type detailField struct {
	label   string
	kind    detailFieldKind
	choices []string // fieldKindChoice only
	numMin  int      // fieldKindNumeric only
	numMax  int      // fieldKindNumeric only
}

// detailPane is the right-hand field-editing pane.
type detailPane struct {
	vp            viewport.Model
	cursor        int
	revealedField int // index of the currently revealed secret field; -1 if none
	dropdownOpen  bool
	width         int
	height        int
	scrolls       int // incremented per scroll event; aids test assertions

	// Text / numeric editing (active when editing == true).
	editing bool
	editBuf string // current contents of the edit buffer
	editMsg string // transient status message (e.g. "pasted content sanitized")

	// Choice-list state (active when dropdownOpen == true).
	choiceOptions []string
	choiceIdx     int

	// Model-suggestion state.
	modelSuggIdx   int
	modelSuggFocus bool // true when keyboard focus is inside the suggestion list
}

func newDetailPane(width, height int) detailPane {
	return detailPane{
		vp:            viewport.New(width, height),
		revealedField: -1,
		width:         width,
		height:        height,
	}
}

// ShortcutLine returns the shortcut hints for the current detail pane state.
func (d detailPane) ShortcutLine() string {
	if d.dropdownOpen {
		return "↑/↓  navigate  ·  Enter  confirm  ·  Esc  cancel"
	}
	if d.editing {
		return "type to edit  ·  Enter  confirm  ·  Esc  cancel"
	}
	if d.modelSuggFocus {
		return "↑/↓  navigate  ·  Enter  pick  ·  Esc  collapse"
	}
	return "Tab  outline  ·  ↑/↓  navigate  ·  Enter  edit  ·  r  reveal/mask"
}

// isSensitiveKey returns true when key contains a standard secret-suffix
// pattern. The check is case-insensitive substring matching.
func isSensitiveKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, sfx := range []string{
		"_TOKEN", "_SECRET", "_KEY", "_PASSWORD",
		"_PASSPHRASE", "_CREDENTIAL", "_APIKEY",
	} {
		if strings.Contains(upper, sfx) {
			return true
		}
	}
	return false
}

// buildDetailFields returns the ordered list of editable fields for step.
func buildDetailFields(step workflowmodel.Step) []detailField {
	fields := []detailField{
		{label: "Name", kind: fieldKindText},
		{label: "Kind", kind: fieldKindChoice, choices: []string{"claude", "shell"}},
	}
	if step.Kind == workflowmodel.StepKindClaude {
		fields = append(fields,
			detailField{label: "Model", kind: fieldKindModelSuggest},
			detailField{label: "PromptFile", kind: fieldKindText},
		)
	} else {
		fields = append(fields,
			detailField{label: "Command", kind: fieldKindText},
		)
	}
	fields = append(fields,
		detailField{label: "CaptureAs", kind: fieldKindText},
		detailField{label: "CaptureMode", kind: fieldKindChoice, choices: []string{"lastLine", "fullStdout"}},
		detailField{label: "TimeoutSeconds", kind: fieldKindNumeric, numMin: 0, numMax: 3600},
		detailField{label: "OnTimeout", kind: fieldKindChoice, choices: []string{"continue", "fail"}},
	)
	if step.Kind == workflowmodel.StepKindClaude {
		fields = append(fields,
			detailField{label: "ResumePrevious", kind: fieldKindChoice, choices: []string{"false", "true"}},
		)
	}
	fields = append(fields,
		detailField{label: "BreakLoopIfEmpty", kind: fieldKindChoice, choices: []string{"false", "true"}},
		detailField{label: "SkipIfCaptureEmpty", kind: fieldKindText},
	)
	for i, env := range step.Env {
		if env.IsLiteral {
			kind := fieldKindText
			if isSensitiveKey(env.Key) {
				kind = fieldKindSecretMask
			}
			fields = append(fields, detailField{label: fmt.Sprintf("containerEnv[%d]", i), kind: kind})
		} else {
			fields = append(fields, detailField{label: fmt.Sprintf("env[%d]", i), kind: fieldKindText})
		}
	}
	return fields
}

// fieldValue returns the string representation of f's value for step.
func fieldValue(step workflowmodel.Step, f detailField) string {
	switch f.label {
	case "Name":
		return step.Name
	case "Kind":
		return string(step.Kind)
	case "Model":
		return step.Model
	case "PromptFile":
		return step.PromptFile
	case "Command":
		return strings.Join(step.Command, " ")
	case "CaptureAs":
		return step.CaptureAs
	case "CaptureMode":
		return step.CaptureMode
	case "TimeoutSeconds":
		if step.TimeoutSeconds == 0 {
			return ""
		}
		return strconv.Itoa(step.TimeoutSeconds)
	case "OnTimeout":
		return step.OnTimeout
	case "ResumePrevious":
		if step.ResumePrevious {
			return "true"
		}
		return "false"
	case "BreakLoopIfEmpty":
		if step.BreakLoopIfEmpty {
			return "true"
		}
		return "false"
	case "SkipIfCaptureEmpty":
		return step.SkipIfCaptureEmpty
	}
	if strings.HasPrefix(f.label, "containerEnv[") {
		idx := envFieldIndex(f.label)
		if idx >= 0 && idx < len(step.Env) {
			return step.Env[idx].Key + "=" + step.Env[idx].Value
		}
	}
	if strings.HasPrefix(f.label, "env[") {
		idx := envFieldIndex(f.label)
		if idx >= 0 && idx < len(step.Env) {
			return step.Env[idx].Key
		}
	}
	return ""
}

// envFieldIndex parses the integer from labels like "containerEnv[2]".
func envFieldIndex(label string) int {
	start := strings.IndexByte(label, '[')
	end := strings.IndexByte(label, ']')
	if start < 0 || end <= start {
		return -1
	}
	n, err := strconv.Atoi(label[start+1 : end])
	if err != nil {
		return -1
	}
	return n
}

// sanitizePlainText strips ANSI escape sequences and newlines from s.
func sanitizePlainText(s string) string {
	s = string(ansi.StripAll([]byte(s)))
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// render builds the visible detail string for the given step.
func (d detailPane) render(step workflowmodel.Step) string {
	fields := buildDetailFields(step)
	var sb strings.Builder

	for i, f := range fields {
		prefix := "  "
		if i == d.cursor {
			prefix = "> "
		}
		val := fieldValue(step, f)

		switch {
		case i == d.cursor && d.editing:
			line := fmt.Sprintf("%s%s: [%s]", prefix, f.label, d.editBuf)
			if d.editMsg != "" {
				line += "  " + d.editMsg
			}
			sb.WriteString(line)

		case i == d.cursor && d.dropdownOpen:
			fmt.Fprintf(&sb, "%s%s: ▾\n", prefix, f.label)
			for j, opt := range d.choiceOptions {
				optPrefix := "    "
				if j == d.choiceIdx {
					optPrefix = "  > "
				}
				fmt.Fprintf(&sb, "%s%s", optPrefix, opt)
				if j < len(d.choiceOptions)-1 {
					sb.WriteString("\n")
				}
			}

		case f.kind == fieldKindModelSuggest && i == d.cursor:
			fmt.Fprintf(&sb, "%s%s: %s", prefix, f.label, val)
			sugs := workflowmodel.ModelSuggestions
			if len(sugs) > 0 {
				sb.WriteString("\n")
				for j, sug := range sugs {
					sugPrefix := "    "
					if d.modelSuggFocus && j == d.modelSuggIdx {
						sugPrefix = "  > "
					}
					sb.WriteString(sugPrefix + sug)
					if j < len(sugs)-1 {
						sb.WriteString("\n")
					}
				}
			}

		case f.kind == fieldKindSecretMask:
			if d.revealedField == i {
				fmt.Fprintf(&sb, "%s%s: %s", prefix, f.label, val)
			} else {
				fmt.Fprintf(&sb, "%s%s: %s [press r to reveal]", prefix, f.label, GlyphMasked)
			}

		default:
			fmt.Fprintf(&sb, "%s%s: %s", prefix, f.label, val)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
