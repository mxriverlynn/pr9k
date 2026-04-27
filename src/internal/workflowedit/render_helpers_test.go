package workflowedit

import (
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/ansi"
)

// stripView returns the ANSI-stripped output of m.View() for use in substring
// assertions (D-6: ANSI-stripped substring + structural assertion strategy).
func stripView(m Model) string {
	return string(ansi.StripAll([]byte(m.View())))
}

// stripStr strips ANSI escape sequences from any string, for ShortcutLine and
// other render outputs that may carry styling in later commits.
func stripStr(s string) string {
	return string(ansi.StripAll([]byte(s)))
}

// assertModalFits validates that an active dialog/modal content fits within the
// model's frame dimensions (dialog/modal height-fits-frame validation).
func assertModalFits(t *testing.T, m Model) {
	t.Helper()
	view := stripView(m)
	if view == "" {
		t.Error("assertModalFits: modal view must not be empty")
		return
	}
	if m.height > 0 {
		lines := strings.Count(view, "\n") + 1
		if lines > m.height {
			t.Errorf("assertModalFits: modal has %d lines but frame height is %d", lines, m.height)
		}
	}
}
