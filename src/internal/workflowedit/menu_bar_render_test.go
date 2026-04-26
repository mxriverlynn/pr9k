package workflowedit

import (
	"strings"
	"testing"
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
