//go:build !windows

package workflowedit

import (
	"strings"
	"testing"
)

// TestSharedInstall_TriggersBanner_WhenDifferentUID verifies that the model
// shows a shared-install warning banner when the workflow bundle is owned by
// a different user than the current process.
func TestSharedInstall_TriggersBanner_WhenDifferentUID(t *testing.T) {
	m := newTestModel()
	m = m.WithSharedInstallWarning("Workflow bundle is installed under a different user; saves may be permission-denied.")

	view := m.View()
	if !strings.Contains(view, "different user") {
		t.Error("expected shared-install warning in View() when different UID detected")
	}
}

// TestSharedInstall_NoBanner_WhenSameUID verifies that no warning is shown
// when the workflow bundle is owned by the current user.
func TestSharedInstall_NoBanner_WhenSameUID(t *testing.T) {
	m := newTestModel()
	// WithSharedInstallWarning not called — same-UID case.

	view := m.View()
	if strings.Contains(view, "different user") {
		t.Errorf("unexpected shared-install warning in View() when same UID: %q", view)
	}
}

// TestSharedInstallWarning_RendersBeforeMainContent pins the D-43 ordering
// constraint: the shared-install banner must appear before the shortcut footer.
func TestSharedInstallWarning_RendersBeforeMainContent(t *testing.T) {
	const bannerText = "different user"
	m := newTestModel()
	m = m.WithSharedInstallWarning("Workflow bundle is installed under a different user; saves may be permission-denied.")

	view := m.View()
	bannerIdx := strings.Index(view, bannerText)
	ctrlIdx := strings.Index(view, "Ctrl+")
	if bannerIdx < 0 {
		t.Fatal("banner text not found in View()")
	}
	if ctrlIdx < 0 {
		t.Fatal("shortcut line (Ctrl+) not found in View()")
	}
	if bannerIdx >= ctrlIdx {
		t.Errorf("shared-install banner (pos %d) must appear before shortcut line (pos %d)", bannerIdx, ctrlIdx)
	}
}
