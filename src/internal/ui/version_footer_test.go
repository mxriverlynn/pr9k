package ui

import (
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/version"
)

// TP-109-02: Version appears in TUI footer.
// Creates a Model whose version label carries version.Version, renders View(),
// and asserts the version string is present in the output.
func TestView_FooterContainsCurrentVersion(t *testing.T) {
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step-one"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	m := NewModel(header, kh, "pr9k v"+version.Version)
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	out := stripANSI(m.View())

	if !strings.Contains(out, version.Version) {
		t.Errorf("View() output does not contain version %q;\ngot:\n%s", version.Version, out)
	}
}
