package workflowedit

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mxriverlynn/pr9k/src/internal/uichrome"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// renderBordered returns the outline pane as a self-contained bordered block
// implementing D18–D25 and D49. Height is o.height-2 to fit within
// renderEditView's content budget (which prepends session header + blank line).
//
// Row anatomy for a step:
//
//	focus(2) gripper(2) space(1) kind(3) space(1) name…  scroll(1)
//
// Reverse-video is applied only to the reorder-mode active step (D25).
func (o outlinePanel) renderBordered(doc workflowmodel.WorkflowDoc, cursor int, reorderMode bool) string {
	// Subtract 2 rows consumed by renderEditView's "header\n" prefix.
	paneH := o.height - 2
	if o.width <= 0 || paneH < 3 {
		return o.renderFlat(doc, cursor, reorderMode)
	}

	// innerW: visible chars between the left │ and the scroll-indicator column.
	innerW := o.width - 2
	contentH := paneH - 2 // top border + bottom border = 2

	rows := buildOutlineRows(doc, o.collapsed)
	offset := computeScrollOffset(cursor, len(rows), contentH)

	gray := lipgloss.NewStyle().Foreground(uichrome.LightGray)
	vbar := gray.Render("│")

	var sb strings.Builder

	// Top border — total width = o.width.
	sb.WriteString(uichrome.RenderTopBorder("Outline", o.width))
	sb.WriteString("\n")

	// Content rows.
	for i := 0; i < contentH; i++ {
		rowIdx := offset + i
		var content string
		if rowIdx < len(rows) {
			row := rows[rowIdx]
			content = buildOutlineRowContent(row, doc, rowIdx, cursor, reorderMode, innerW, o.collapsed)
			if reorderMode && rowIdx == cursor && row.kind == rowKindStep {
				// Reverse-video applies only to the reorder-active step (D25).
				content = lipgloss.NewStyle().Reverse(true).Render(content)
			}
		}
		// Pad to innerW (CJK-safe via lipgloss.Width).
		w := lipgloss.Width(content)
		switch {
		case w < innerW:
			content += strings.Repeat(" ", innerW-w)
		case w > innerW:
			content = lipgloss.NewStyle().MaxWidth(innerW).Render(content)
		}
		scrollChar := scrollGlyphAt(i, contentH, offset, len(rows))
		sb.WriteString(vbar + content + gray.Render(scrollChar) + "\n")
	}

	// Bottom border — BottomBorder takes innerW (chars between corners).
	sb.WriteString(uichrome.BottomBorder(innerW))

	return sb.String()
}

// computeScrollOffset centers the cursor within the contentH-row viewport.
// Returns the index of the first row to display.
func computeScrollOffset(cursor, total, contentH int) int {
	if total <= contentH {
		return 0
	}
	offset := cursor - contentH/2
	if offset < 0 {
		offset = 0
	}
	if offset > total-contentH {
		offset = total - contentH
	}
	return offset
}

// scrollGlyphAt returns the scroll indicator character for viewport row i.
// Returns "│" when no indicator is needed.
func scrollGlyphAt(i, contentH, offset, total int) string {
	if total <= contentH {
		return "│"
	}
	canScrollUp := offset > 0
	canScrollDown := offset+contentH < total

	if i == 0 && canScrollUp {
		return GlyphScrollUp
	}
	if i == contentH-1 && canScrollDown {
		return GlyphScrollDown
	}

	// Proportional thumb position.
	thumbPos := 0
	if total > contentH {
		thumbPos = offset * contentH / (total - contentH)
		if thumbPos >= contentH {
			thumbPos = contentH - 1
		}
	}
	if i == thumbPos {
		return GlyphScrollThumb
	}
	return "│"
}

// buildOutlineRowContent returns the visible content for one outline row
// (no borders, no scroll indicator), truncated to innerW via lipgloss.Width (D49).
func buildOutlineRowContent(row outlineRow, doc workflowmodel.WorkflowDoc, rowIdx, cursor int, reorderMode bool, innerW int, collapsed map[sectionKey]bool) string {
	focused := rowIdx == cursor
	switch row.kind {
	case rowKindStep:
		return buildStepRowContent(row, doc, focused, innerW)
	case rowKindSectionHeader:
		return buildHeaderRowContent(row, doc, focused, collapsed, innerW)
	case rowKindAddRow:
		return buildAddRowContent(row, focused, innerW)
	case rowKindEnvItem, rowKindContainerEnvItem:
		return buildEnvItemRowContent(row, focused, innerW)
	}
	return ""
}

func buildStepRowContent(row outlineRow, doc workflowmodel.WorkflowDoc, focused bool, innerW int) string {
	focusPrefix := "  "
	if focused {
		focusPrefix = "> "
	}
	name := doc.Steps[row.stepIdx].Name
	if name == "" {
		name = HintNoName
	}
	kind := kindGlyphFor(doc.Steps[row.stepIdx].Kind)
	// Fixed prefix: focus(2) + gripper(2) + " "(1) + kind(3) + " "(1) = 9 chars.
	fixed := focusPrefix + GlyphGripper + " " + kind + " "
	fixedW := lipgloss.Width(fixed)
	nameW := innerW - fixedW
	if nameW < 1 {
		nameW = 1
	}
	return fixed + lipgloss.NewStyle().MaxWidth(nameW).Render(name)
}

func buildHeaderRowContent(row outlineRow, doc workflowmodel.WorkflowDoc, focused bool, collapsed map[sectionKey]bool, innerW int) string {
	focusPrefix := "  "
	if focused {
		focusPrefix = "> "
	}
	chevron := GlyphChevronExpanded
	if collapsed[row.section] {
		chevron = GlyphChevronCollapsed
	}
	label := sectionLabel(row.section, doc)
	// Fixed prefix: focus(2) + chevron(1) + " "(1) = 4 chars.
	fixed := focusPrefix + chevron + " "
	fixedW := lipgloss.Width(fixed)
	labelW := innerW - fixedW
	if labelW < 1 {
		labelW = 1
	}
	return fixed + lipgloss.NewStyle().MaxWidth(labelW).Render(label)
}

func buildAddRowContent(row outlineRow, focused bool, innerW int) string {
	focusPrefix := "  "
	if focused {
		focusPrefix = "> "
	}
	label := addRowLabel(row.section)
	// Fixed: focus(2) + GlyphAddItem(1) + " "(1) = 4 chars.
	fixed := focusPrefix + GlyphAddItem + " "
	fixedW := lipgloss.Width(fixed)
	labelW := innerW - fixedW
	if labelW < 1 {
		labelW = 1
	}
	return fixed + lipgloss.NewStyle().MaxWidth(labelW).Render(label)
}

func buildEnvItemRowContent(row outlineRow, focused bool, innerW int) string {
	focusPrefix := "  "
	if focused {
		focusPrefix = "> "
	}
	// Indent(2) + focus(2) = 4 chars fixed.
	fixed := "  " + focusPrefix
	fixedW := lipgloss.Width(fixed)
	labelW := innerW - fixedW
	if labelW < 1 {
		labelW = 1
	}
	return fixed + lipgloss.NewStyle().MaxWidth(labelW).Render(row.label)
}

// kindGlyphFor maps a step kind to its outline display glyph (D21).
func kindGlyphFor(kind workflowmodel.StepKind) string {
	switch kind {
	case workflowmodel.StepKindShell:
		return GlyphKindShell
	case workflowmodel.StepKindClaude:
		return GlyphKindClaude
	}
	return GlyphKindUnset
}

// renderFlat is the legacy flat render used when o.width <= 0 (no WindowSizeMsg yet).
// It preserves backward-compatibility for tests that do not send a WindowSizeMsg.
func (o outlinePanel) renderFlat(doc workflowmodel.WorkflowDoc, cursor int, reorderMode bool) string {
	rows := buildOutlineRows(doc, o.collapsed)
	if len(rows) == 0 {
		return "(no steps)\n"
	}
	var sb strings.Builder
	for i, row := range rows {
		focused := i == cursor
		switch row.kind {
		case rowKindSectionHeader:
			chevron := GlyphChevronExpanded
			if o.collapsed[row.section] {
				chevron = GlyphChevronCollapsed
			}
			label := sectionLabel(row.section, doc)
			prefix := "  "
			if focused {
				prefix = "> "
			}
			sb.WriteString(prefix + chevron + " " + label + "\n")

		case rowKindStep:
			name := doc.Steps[row.stepIdx].Name
			if name == "" {
				name = HintNoName
			}
			var prefix string
			switch {
			case reorderMode && focused:
				prefix = GlyphGripper + " "
			case focused:
				prefix = "> "
			default:
				prefix = "  "
			}
			sb.WriteString("  " + prefix + name + "\n")

		case rowKindEnvItem, rowKindContainerEnvItem:
			prefix := "  "
			if focused {
				prefix = "> "
			}
			sb.WriteString("    " + prefix + row.label + "\n")

		case rowKindAddRow:
			prefix := "  "
			if focused {
				prefix = "> "
			}
			addLabel := addRowLabel(row.section)
			sb.WriteString("  " + prefix + GlyphAddItem + " " + addLabel + "\n")
		}
	}
	return sb.String()
}
