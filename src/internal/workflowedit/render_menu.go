package workflowedit

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mxriverlynn/pr9k/src/internal/uichrome"
)

// renderMenuBar renders the D4 menu bar row content.
// Closed state: "F" (white mnemonic accent) + "ile" (LightGray).
// Open state: "File" in reverse video (signalling the menu is active).
func (m Model) renderMenuBar() string {
	if m.menu.open {
		return lipgloss.NewStyle().Reverse(true).Render("File")
	}
	f := lipgloss.NewStyle().Foreground(uichrome.White).Render("F")
	ile := lipgloss.NewStyle().Foreground(uichrome.LightGray).Render("ile")
	return f + ile
}

// renderMenuDropdown renders the D11 bordered dropdown for the File menu.
// Row format: │ Label   Shortcut │  (item left-aligned, shortcut right-aligned).
// Save is rendered in Dim without shortcut when browse-only or no workflow
// loaded (D12: greyed items omit shortcut label).
func (m Model) renderMenuDropdown() string {
	saveGreyed := !m.loaded || m.banners.isReadOnly

	type item struct {
		label, shortcut string
	}
	items := []item{
		{"New", "Ctrl+N"},
		{"Open", "Ctrl+O"},
		{"Save", "Ctrl+S"},
		{"Quit", "Ctrl+Q"},
	}

	// Compute inner width: longest (label + 2 spaces + shortcut) among enabled items.
	maxContent := 0
	for _, it := range items {
		w := len(it.label) + 2 + len(it.shortcut)
		if w > maxContent {
			maxContent = w
		}
	}
	// innerW excludes the two border │ characters; includes one space of padding each side.
	innerW := maxContent + 2 // +2 for " " padding left and right
	// Border line: exactly innerW "─" characters between ╭ and ╮.
	hline := strings.Repeat("─", innerW)
	topBorder := "╭" + hline + "╮"
	bottomBorder := "╰" + hline + "╯"

	rows := make([]string, 0, len(items)+2)
	rows = append(rows, topBorder)

	for _, it := range items {
		var row string
		if it.label == "Save" && saveGreyed {
			// D12: greyed — label in Dim, shortcut omitted, padded to full width.
			label := lipgloss.NewStyle().Foreground(uichrome.Dim).Render(it.label)
			padW := innerW - 2 - len(it.label) // visual width == len (ASCII labels)
			if padW < 0 {
				padW = 0
			}
			row = "│ " + label + strings.Repeat(" ", padW) + " │"
		} else {
			label := lipgloss.NewStyle().Foreground(uichrome.White).Render(it.label)
			shortcut := lipgloss.NewStyle().Foreground(uichrome.LightGray).Render(it.shortcut)
			// Gap between label and shortcut to right-align shortcut.
			gapW := innerW - 2 - len(it.label) - len(it.shortcut)
			if gapW < 1 {
				gapW = 1
			}
			row = "│ " + label + strings.Repeat(" ", gapW) + shortcut + " │"
		}
		rows = append(rows, row)
	}

	rows = append(rows, bottomBorder)
	return strings.Join(rows, "\n")
}
