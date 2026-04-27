package workflowedit

import (
	"strings"

	"github.com/mxriverlynn/pr9k/src/internal/uichrome"
)

// View satisfies tea.Model. It assembles the 9-element chrome frame (D-1, D-5):
//  1. Top border
//  2. Menu bar (placeholder; full render in WU-6)
//  3. Session header row 1 — title/path (placeholder; full render in WU-5)
//  4. Session header row 2 — banner slot / saveBanner (D-7, D-14)
//  5. Separator
//  6. Content panel (panelH = m.height - ChromeRows rows; placeholders until WU-7/WU-8)
//  7. Separator
//  8. Shortcut/footer (placeholder; Validating guard in WU-12)
//  9. Bottom border
//
// Overlay splice for dialogs, help, and findings panel is a placeholder for WU-9/WU-10.
// When dimensions are uninitialized (width<=0 or height<=0), the legacy flat layout is
// returned so tests that do not send a WindowSizeMsg continue to pass unchanged.
func (m Model) View() string {
	// Uninitialized dimensions: preserve legacy flat layout for zero-size models.
	if m.width <= 0 || m.height <= 0 {
		return m.viewFallback()
	}
	// D48 minimum-size guard (D-19).
	if m.width < uichrome.MinTerminalWidth || m.height < uichrome.MinTerminalHeight {
		return "Terminal too small — resize to at least 60×16"
	}

	innerW := m.width - 2 // two │ border characters
	panelH := m.height - ChromeRows

	parts := make([]string, 0, 9)

	// 1. Top border.
	parts = append(parts, uichrome.RenderTopBorder("pr9k workflow builder", m.width))

	// 2. Menu bar — D4 label with mnemonic accent; open state uses reverse video.
	parts = append(parts, uichrome.WrapLine(m.renderMenuBar(), innerW))

	// 3. Session header row 1 — title/path.
	parts = append(parts, uichrome.WrapLine(m.renderSessionHeader(), innerW))

	// 4. Session header row 2 — banner slot (D-7, D-14).
	parts = append(parts, uichrome.WrapLine(m.saveBanner, innerW))

	// 5. Separator.
	parts = append(parts, uichrome.HRuleLine(innerW))

	// 6. Content panel — exactly panelH rows (placeholder renders until WU-7/WU-8).
	parts = append(parts, m.renderContentPanel(panelH, innerW))

	// 7. Separator.
	parts = append(parts, uichrome.HRuleLine(innerW))

	// 8. Shortcut/footer.
	parts = append(parts, uichrome.WrapLine(m.ShortcutLine(), innerW))

	// 9. Bottom border.
	parts = append(parts, uichrome.BottomBorder(innerW))

	frame := strings.Join(parts, "\n")

	// D11: overlay the dropdown below the menu bar (row 2, col 1) when File menu open.
	if m.menu.open {
		frame = uichrome.Overlay(frame, m.renderMenuDropdown(), 2, 1)
	}

	return frame
}

// renderContentPanel renders the content area as exactly panelH wrapped lines.
// For WU-4 this is a placeholder that re-uses the existing inline renders;
// WU-7 and WU-8 replace those renders with the full bordered pane layouts.
func (m Model) renderContentPanel(panelH, innerW int) string {
	var raw string
	if m.helpOpen {
		raw = m.renderHelpModal()
	} else if m.dialog.kind != DialogNone {
		raw = m.renderDialog()
	} else if !m.loaded {
		raw = m.renderEmptyEditor()
	} else {
		raw = m.renderEditView()
	}

	rawLines := strings.Split(raw, "\n")
	out := make([]string, panelH)
	for i := range out {
		line := ""
		if i < len(rawLines) {
			line = rawLines[i]
		}
		out[i] = uichrome.WrapLine(line, innerW)
	}
	return strings.Join(out, "\n")
}

// viewFallback returns the legacy flat View() layout used when m.width == 0
// or m.height == 0 (model not yet sized by a WindowSizeMsg). This preserves
// backward-compatibility for tests that do not send a WindowSizeMsg.
func (m Model) viewFallback() string {
	var sb strings.Builder
	sb.WriteString(m.renderMenuBar())
	sb.WriteString("\n")
	if m.menu.open {
		sb.WriteString(m.renderMenuDropdown())
		sb.WriteString("\n")
	}
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
