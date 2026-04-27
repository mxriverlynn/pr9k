package workflowedit

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestMenuBar_ClosedState_ShowsFileLabel verifies the menu bar shows "File" when closed.
func TestMenuBar_ClosedState_ShowsFileLabel(t *testing.T) {
	m := newTestModel()
	view := stripView(m)
	if !strings.Contains(view, "File") {
		t.Errorf("view should contain menu label, got %q", view)
	}
}

// TestMenuBar_OpenState_ShowsMenuItems verifies the File dropdown contents.
func TestMenuBar_OpenState_ShowsMenuItems(t *testing.T) {
	m := newTestModel()
	m.menu.open = true
	view := stripView(m)
	for _, item := range []string{"New", "Open", "Save", "Quit"} {
		if !strings.Contains(view, item) {
			t.Errorf("open menu should contain %q, got %q", item, view)
		}
	}
}

// TestMenuBar_F10_TogglesMenu verifies F10 opens and closes the menu.
func TestMenuBar_F10_TogglesMenu(t *testing.T) {
	m := newTestModel()
	open := applyKey(m, keyF10())
	if !open.menu.open {
		t.Error("menu should open on first F10")
	}
	closed := applyKey(open, keyF10())
	if closed.menu.open {
		t.Error("menu should close on second F10")
	}
}

// TestMenuBar_FileLabelRendered verifies the menu bar row (row 1) shows only "File"
// and not the dropdown items inline — items belong in the overlay, not the menu bar row.
func TestMenuBar_FileLabelRendered(t *testing.T) {
	m := newTestModel()
	m = applyMsg(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.menu.open = true
	lines := strings.Split(stripStr(m.View()), "\n")
	menuBarLine := lines[1]
	if !strings.Contains(menuBarLine, "File") {
		t.Errorf("menu bar row should contain 'File', got: %q", menuBarLine)
	}
	// Dropdown items must NOT appear inline in the menu bar row (D11: overlay below).
	if strings.Contains(menuBarLine, "New") {
		t.Errorf("menu items should not be inline in menu bar row (expected overlay), got: %q", menuBarLine)
	}
}

// TestMenuBar_DropdownOpenOverlay verifies the open dropdown uses D11 bordered chrome
// (╭ at row 2, right below the menu bar) and contains all four menu items.
func TestMenuBar_DropdownOpenOverlay(t *testing.T) {
	m := newTestModel()
	m = applyMsg(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.menu.open = true
	viewLines := strings.Split(m.View(), "\n")
	// Row 2 is the session-header row; when the dropdown is overlaid it should contain ╭
	if !strings.Contains(viewLines[2], "╭") {
		t.Errorf("row 2 should contain dropdown top border (╭), got: %q", stripStr(viewLines[2]))
	}
	stripped := stripStr(m.View())
	for _, item := range []string{"New", "Open", "Save", "Quit"} {
		if !strings.Contains(stripped, item) {
			t.Errorf("open menu dropdown should contain %q", item)
		}
	}
}

// TestMenuBar_GreyedSaveWhenReadOnly verifies Save shows Ctrl+S when enabled but omits
// it when the workflow is read-only (D12: greyed items omit shortcut label).
func TestMenuBar_GreyedSaveWhenReadOnly(t *testing.T) {
	// Use a loaded model so Save is enabled (D12: greyed only when not loaded or read-only).
	m := newLoadedModelWithWidth(80, 24)
	m.menu.open = true
	// Enabled case: Save shortcut should be visible
	normal := stripStr(m.View())
	if !strings.Contains(normal, "Ctrl+S") {
		t.Error("enabled Save should show Ctrl+S shortcut in dropdown")
	}
	// Read-only case: Save still visible but shortcut omitted
	m.banners.isReadOnly = true
	greyed := stripStr(m.View())
	if !strings.Contains(greyed, "Save") {
		t.Error("greyed Save should still be visible (D12: greyed items stay visible)")
	}
	if strings.Contains(greyed, "Ctrl+S") {
		t.Error("greyed Save should not show Ctrl+S shortcut (D12: greyed items omit shortcut)")
	}
}

// TestMenuBar_DropdownBelowMenuBar verifies the dropdown overlay starts at row 2
// (below the menu bar row 1), not inline in the menu bar row itself.
func TestMenuBar_DropdownBelowMenuBar(t *testing.T) {
	m := newTestModel()
	m = applyMsg(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.menu.open = true
	lines := strings.Split(m.View(), "\n")
	menuBarLine := lines[1]
	dropdownStartLine := lines[2]
	if strings.Contains(menuBarLine, "╭") {
		t.Errorf("menu bar row should not contain dropdown chrome, got: %q", stripStr(menuBarLine))
	}
	if !strings.Contains(dropdownStartLine, "╭") {
		t.Errorf("dropdown should start at row 2 (below menu bar), got: %q", stripStr(dropdownStartLine))
	}
}
