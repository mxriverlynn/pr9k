package workflowedit

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mxriverlynn/pr9k/src/internal/uichrome"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// detailLabelColW is the fixed character width reserved for the label column (D50).
// Labels longer than this are truncated via lipgloss.Width.
const detailLabelColW = 16

// renderBordered renders the detail pane as a self-contained bordered block
// implementing D26–D33, D47, D50, D51. Called by render() when dimensions are set.
//
// Row anatomy (D29/D30/D31):
//
//	focus(2) + label(detailLabelColW, D50 truncated) + ": " + "[ " + value + " ]" + optional ▾
//
// The pane renders to exactly (d.height-2) lines so that after joining with the outline
// pane and being wrapped by renderContentPanel, everything stays within the chrome budget.
//
// Width budget: after joining outline (ow) + detail (d.width), WrapLine in
// renderContentPanel truncates to innerW = m.width-2, which cuts the last 2 chars.
// To keep the scroll indicator visible:
//   - Top/bottom borders are (d.width-2) chars wide.
//   - Each content row is: vbar(1) + content(d.width-4) + scrollChar(1) = d.width-2 chars.
func (d detailPane) renderBordered(step workflowmodel.Step) string {
	// Subtract 2 rows consumed by renderEditView's "header\n" prefix, matching
	// the outline pane's paneH = o.height-2 convention.
	paneH := d.height - 2
	if d.width <= 0 || paneH < 3 {
		return ""
	}

	// rowContentW: visible content chars between the left │ and the scroll-indicator.
	// d.width-4 ensures the scroll indicator lands within WrapLine's visible range.
	rowContentW := d.width - 4
	if rowContentW < 2 {
		rowContentW = 2
	}
	contentH := paneH - 2 // top border + bottom border

	fields := buildDetailFields(step)
	focusIdx := d.cursor
	if focusIdx < 0 {
		focusIdx = 0
	}
	if focusIdx >= len(fields) {
		focusIdx = len(fields) - 1
	}

	// Phase 1: count base rows (1 per field, +1 for multiLine action row) to
	// estimate each field's Y position for the D51 flip decision.
	baseRowOf := make([]int, len(fields))
	baseRow := 0
	for i, f := range fields {
		baseRowOf[i] = baseRow
		baseRow++
		if f.kind == fieldKindMultiLine {
			baseRow++ // action row always shown for multiLine fields
		}
	}
	totalBaseRows := baseRow

	// Estimate scroll offset and focused field Y for D51 flip decision.
	focusBaseRow := 0
	if focusIdx < len(baseRowOf) {
		focusBaseRow = baseRowOf[focusIdx]
	}
	estOffset := computeScrollOffset(focusBaseRow, totalBaseRows, contentH)
	focusedYEst := focusBaseRow - estOffset

	// D51: if open dropdown would extend past pane bottom, flip above.
	flipDropdownAbove := false
	if d.dropdownOpen && focusIdx < len(fields) && fields[focusIdx].kind == fieldKindChoice {
		numOpts := len(d.choiceOptions)
		if focusedYEst+1+numOpts > contentH {
			flipDropdownAbove = true
		}
	}

	// Phase 2: build the flat list of display rows.
	var allRows []string
	focusRowStart := 0

	for i, f := range fields {
		focused := i == focusIdx
		fieldRows := d.buildFieldRows(step, f, i, focused, rowContentW, flipDropdownAbove)

		if focused {
			if flipDropdownAbove && d.dropdownOpen && f.kind == fieldKindChoice {
				// Options come first; field row follows.
				focusRowStart = len(allRows) + len(d.choiceOptions)
			} else {
				focusRowStart = len(allRows)
			}
		}
		allRows = append(allRows, fieldRows...)
	}

	// Final scroll offset centered on the focused field row.
	offset := computeScrollOffset(focusRowStart, len(allRows), contentH)

	gray := lipgloss.NewStyle().Foreground(uichrome.LightGray)
	vbar := gray.Render("│")

	var sb strings.Builder
	// Top border: d.width-2 chars wide (fits within WrapLine's visible range).
	sb.WriteString(uichrome.RenderTopBorder("Detail", d.width-2))
	sb.WriteString("\n")

	for i := 0; i < contentH; i++ {
		rowIdx := offset + i
		var content string
		if rowIdx < len(allRows) {
			content = allRows[rowIdx]
		}
		// Pad/truncate to rowContentW (CJK-safe via lipgloss.Width, D50).
		w := lipgloss.Width(content)
		switch {
		case w < rowContentW:
			content += strings.Repeat(" ", rowContentW-w)
		case w > rowContentW:
			content = lipgloss.NewStyle().MaxWidth(rowContentW).Render(content)
		}
		scrollChar := scrollGlyphAt(i, contentH, offset, len(allRows))
		sb.WriteString(vbar + content + gray.Render(scrollChar) + "\n")
	}

	// Bottom border: BottomBorder(rowContentW) = rowContentW+2 = d.width-2 chars wide.
	sb.WriteString(uichrome.BottomBorder(rowContentW))
	return sb.String()
}

// buildFieldRows returns the display rows for one field. Most fields produce
// one row; multiLine fields produce two (field + action row, D32); choice fields
// with an open dropdown produce 1+N rows (D51 flip-aware).
//
// rowContentW is the visible content width (between vbar and scrollChar).
func (d detailPane) buildFieldRows(step workflowmodel.Step, f detailField, fieldIdx int, focused bool, rowContentW int, flipDropdownAbove bool) []string {
	val := fieldValue(step, f)
	if focused && d.editing {
		val = d.editBuf
	}

	prefix := "  "
	if focused {
		prefix = "> "
	}

	// Label column with D50 truncation via lipgloss.Width.
	label := truncateDetailLabel(f.label, detailLabelColW)
	labelW := lipgloss.Width(label)
	// Pad to fixed column width for alignment.
	if labelW < detailLabelColW {
		label += strings.Repeat(" ", detailLabelColW-labelW)
	}

	// Available width for the bracketed control region.
	// Row = prefix(2) + label(detailLabelColW) + ": "(2) + control = rowContentW.
	controlW := rowContentW - 2 - detailLabelColW - 2
	if controlW < 5 {
		controlW = 5
	}

	makeFieldRow := func(displayVal, indicator string) string {
		// "[ " + val + optional " indicator" + " ]" → minimum "[ x ]" = 5 chars.
		valW := controlW - 4 // "[ " + " ]"
		if indicator != "" {
			valW -= lipgloss.Width(indicator) + 1 // " " + indicator
		}
		if valW < 0 {
			valW = 0
		}
		if lipgloss.Width(displayVal) > valW {
			displayVal = lipgloss.NewStyle().MaxWidth(valW).Render(displayVal)
		}
		ctrl := "[ " + displayVal
		if indicator != "" {
			ctrl += " " + indicator
		}
		ctrl += " ]"
		return prefix + label + ": " + ctrl
	}

	switch f.kind {
	case fieldKindChoice:
		fieldRow := makeFieldRow(val, "▾")
		if focused && d.dropdownOpen {
			optRows := d.buildDropdownOptionRows(rowContentW)
			if flipDropdownAbove {
				return append(optRows, fieldRow)
			}
			return append([]string{fieldRow}, optRows...)
		}
		return []string{fieldRow}

	case fieldKindSecretMask:
		displayVal := GlyphMasked
		if d.revealedField == fieldIdx {
			displayVal = val
		}
		return []string{makeFieldRow(displayVal, "")}

	case fieldKindMultiLine:
		fieldRow := makeFieldRow(val, "")
		actionRow := "  ↩ Ctrl+E to edit" // D32
		return []string{fieldRow, actionRow}

	case fieldKindModelSuggest:
		fieldRow := makeFieldRow(val, "")
		// Show suggestions whenever this field is focused (matches renderFlat behaviour).
		if focused {
			return append([]string{fieldRow}, d.buildModelSuggRows()...)
		}
		return []string{fieldRow}

	default: // fieldKindText, fieldKindNumeric
		return []string{makeFieldRow(val, "")}
	}
}

// buildDropdownOptionRows returns the rendered rows for an open choice dropdown.
// The highlighted option (d.choiceIdx) uses reverse-video (D25).
// rowContentW is passed in to ensure the padded strings use the correct width.
func (d detailPane) buildDropdownOptionRows(rowContentW int) []string {
	rows := make([]string, len(d.choiceOptions))
	for j, opt := range d.choiceOptions {
		pfx := "  "
		if j == d.choiceIdx {
			pfx = "> "
		}
		row := pfx + opt
		// Pad to rowContentW for consistent width before applying reverse-video.
		w := lipgloss.Width(row)
		switch {
		case w < rowContentW:
			row += strings.Repeat(" ", rowContentW-w)
		case w > rowContentW:
			row = lipgloss.NewStyle().MaxWidth(rowContentW).Render(row)
		}
		if j == d.choiceIdx {
			// Reverse-video only on the highlighted item (D25).
			rows[j] = lipgloss.NewStyle().Reverse(true).Render(row)
		} else {
			rows[j] = row
		}
	}
	return rows
}

// buildModelSuggRows returns the rendered suggestion rows for the modelSuggest field.
func (d detailPane) buildModelSuggRows() []string {
	sugs := workflowmodel.ModelSuggestions
	rows := make([]string, len(sugs))
	for j, sug := range sugs {
		pfx := "    "
		if d.modelSuggFocus && j == d.modelSuggIdx {
			pfx = "  > "
		}
		rows[j] = pfx + sug
	}
	return rows
}

// truncateDetailLabel truncates label to maxW chars using lipgloss.Width (D50).
func truncateDetailLabel(label string, maxW int) string {
	if lipgloss.Width(label) <= maxW {
		return label
	}
	return lipgloss.NewStyle().MaxWidth(maxW).Render(label)
}
