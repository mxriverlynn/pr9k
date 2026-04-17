package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/version"
)

// repoRoot returns the workspace root directory, resolved from this test file's
// absolute path so the test works correctly regardless of the working directory.
func docTestRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	// file is .../ralph-tui/cmd/ralph-tui/doc_integrity_test.go
	// three levels up reaches the workspace root
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// readFile is a test helper that reads a file relative to the repo root and
// returns its content as a string, failing the test if the file cannot be read.
func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("readFile %q: %v", rel, err)
	}
	return string(data)
}

// assertFileExists fails the test if the file at the given path does not exist.
func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist: %s (%v)", path, err)
	}
}

// assertContains fails the test if s does not contain substr.
func assertContains(t *testing.T, s, substr, context string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("%s: expected to contain %q", context, substr)
	}
}

// assertNotContains fails the test if s contains substr.
func assertNotContains(t *testing.T, s, substr, context string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("%s: expected NOT to contain %q", context, substr)
	}
}

// TP-109-04: ADR file exists at documented path.
func TestDocIntegrity_ADRFileExists(t *testing.T) {
	root := docTestRepoRoot(t)
	assertFileExists(t, filepath.Join(root, "docs", "adr", "20260416-clipboard-and-selection.md"))
}

// TP-109-05: How-to file exists at documented path.
func TestDocIntegrity_HowToFileExists(t *testing.T) {
	root := docTestRepoRoot(t)
	assertFileExists(t, filepath.Join(root, "docs", "how-to", "copying-log-text.md"))
}

// TP-109-06: tui-display.md CLAUDE.md entry contains "ModeSelect" and "clipboard copy"
// and does NOT contain the stale "future copy/select wiring" placeholder text.
func TestDocIntegrity_CLAUDEmd_TuiDisplayEntryUpdated(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "CLAUDE.md")

	assertContains(t, content, "ModeSelect", "CLAUDE.md tui-display.md entry")
	assertContains(t, content, "clipboard copy", "CLAUDE.md tui-display.md entry")
	assertNotContains(t, content, "future copy/select wiring", "CLAUDE.md tui-display.md entry")
}

// TP-109-07: ADR references correct clipboard dependency (github.com/atotto/clipboard v0.1.4).
func TestDocIntegrity_ADR_ClipboardDependency(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/adr/20260416-clipboard-and-selection.md")

	assertContains(t, content, "github.com/atotto/clipboard", "ADR clipboard dependency")
	assertContains(t, content, "v0.1.4", "ADR clipboard version")
}

// TP-109-08: ADR references correct x/term dependency (golang.org/x/term v0.42.0).
func TestDocIntegrity_ADR_TermDependency(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/adr/20260416-clipboard-and-selection.md")

	assertContains(t, content, "golang.org/x/term", "ADR x/term dependency")
	assertContains(t, content, "v0.42.0", "ADR x/term version")
}

// TP-109-09: ADR decision (d) references OSC 52 and stderr.
func TestDocIntegrity_ADR_OSC52FallbackDocumented(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/adr/20260416-clipboard-and-selection.md")

	assertContains(t, content, "OSC 52", "ADR OSC 52 decision")
	assertContains(t, content, "stderr", "ADR OSC 52 stderr reference")
}

// TP-109-10: ADR pairs-with link is valid — the referenced Docker sandbox ADR exists.
func TestDocIntegrity_ADR_PairsWithLinkValid(t *testing.T) {
	root := docTestRepoRoot(t)
	assertFileExists(t, filepath.Join(root, "docs", "adr", "20260413160000-require-docker-sandbox.md"))
}

// TP-109-11: How-to documents three copy paths (Path 1, Path 2, Path 3).
func TestDocIntegrity_HowTo_ThreeCopyPathsPresent(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/how-to/copying-log-text.md")

	for _, path := range []string{"Path 1", "Path 2", "Path 3"} {
		assertContains(t, content, path, "copying-log-text.md")
	}
}

// TP-109-12: How-to keyboard reference table present with key bindings.
func TestDocIntegrity_HowTo_KeyboardReferencePresent(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/how-to/copying-log-text.md")

	assertContains(t, content, "Keyboard reference for Select mode", "copying-log-text.md")
	for _, key := range []string{"`h`", "`l`", "`j`", "`k`", "`y`", "`Esc`"} {
		assertContains(t, content, key, "copying-log-text.md keyboard reference")
	}
}

// TP-109-13: How-to clipboard delivery section present with all three delivery mechanisms.
func TestDocIntegrity_HowTo_ClipboardDeliveryPresent(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/how-to/copying-log-text.md")

	for _, want := range []string{"Clipboard delivery", "OSC 52", "xclip", "xsel"} {
		assertContains(t, content, want, "copying-log-text.md clipboard delivery section")
	}
}

// TP-109-14: How-to Related documentation links are valid.
// The file references reading-the-tui.md, keyboard-input.md, and tui-display.md,
// and all three referenced files exist on disk.
func TestDocIntegrity_HowTo_RelatedLinksValid(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/how-to/copying-log-text.md")

	relatedLinks := []struct {
		ref  string
		path string
	}{
		{"reading-the-tui.md", "docs/how-to/reading-the-tui.md"},
		{"keyboard-input.md", "docs/features/keyboard-input.md"},
		{"tui-display.md", "docs/features/tui-display.md"},
	}
	for _, l := range relatedLinks {
		assertContains(t, content, l.ref, "copying-log-text.md Related documentation")
		assertFileExists(t, filepath.Join(root, l.path))
	}
}

// TP-109-15: reading-the-tui.md links to copying-log-text.md.
func TestDocIntegrity_ReadingTheTUI_LinksToHowTo(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/how-to/reading-the-tui.md")

	assertContains(t, content, "copying-log-text.md", "reading-the-tui.md cross-link")
}

// TP-109-16: reading-the-tui.md Related section includes copy how-to.
func TestDocIntegrity_ReadingTheTUI_RelatedSectionIncludesCopyHowTo(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/how-to/reading-the-tui.md")

	// Locate the Related documentation section and verify "Copying Log Text" appears in it.
	relatedIdx := strings.Index(content, "## Related documentation")
	if relatedIdx == -1 {
		t.Fatal("reading-the-tui.md: could not find '## Related documentation' section")
	}
	relatedSection := content[relatedIdx:]
	assertContains(t, relatedSection, "Copying Log Text", "reading-the-tui.md Related documentation section")
}

// TP-109-17: architecture.md ModeSelect block includes mouse entry points.
func TestDocIntegrity_Architecture_ModeSelectIncludesMouse(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/architecture.md")

	// The ModeSelect section must document mouse entry via left-click and left-drag.
	assertContains(t, content, "left-click", "architecture.md ModeSelect section")
	assertContains(t, content, "left-drag", "architecture.md ModeSelect section")
}

// TP-109-18: architecture.md keyboard-input summary includes mouse events.
func TestDocIntegrity_Architecture_KeyboardSummaryIncludesMouseEvents(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/architecture.md")

	assertContains(t, content, "mouse events", "architecture.md keyboard-input summary paragraph")
}

// TP-109-19: tui-display.md documents SelectCommittedShortcuts.
func TestDocIntegrity_TuiDisplay_DocumentsSelectCommittedShortcuts(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/features/tui-display.md")

	assertContains(t, content, "SelectCommittedShortcuts", "tui-display.md")
}

// TP-109-20: keyboard-input.md documents mouse gestures in ModeSelect.
func TestDocIntegrity_KeyboardInput_DocumentsMouseGestures(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/features/keyboard-input.md")

	for _, want := range []string{"Left-drag", "Shift-click", "MouseActionMotion"} {
		assertContains(t, content, want, "keyboard-input.md ModeSelect mouse gestures")
	}
}

// TP-109-21: keyboard-input.md constants table includes SelectCommittedShortcuts
// with the documented value.
func TestDocIntegrity_KeyboardInput_SelectCommittedShortcutsValue(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/features/keyboard-input.md")

	assertContains(t, content, "SelectCommittedShortcuts", "keyboard-input.md constants table")
	assertContains(t, content, "y copy  esc cancel  drag for new selection",
		"keyboard-input.md SelectCommittedShortcuts value")
}

// TP-109-24: versioning.md prose states the current release matches version.Version.
// Guards against the M2 class of bug: bumping the constant but forgetting to update
// the versioning standard's prose.
func TestDocIntegrity_VersioningMd_CurrentReleaseMatchesVersion(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/coding-standards/versioning.md")

	want := "The current release is `" + version.Version + "`"
	assertContains(t, content, want, "versioning.md current release prose")
}

// TP-109-25: reading-the-tui.md ASCII diagram shows the current version label.
// Guards against the S2 class of bug: bumping the version but forgetting the
// illustrative diagram in the how-to guide.
func TestDocIntegrity_ReadingTheTUI_AsciiDiagramVersionCurrent(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/how-to/reading-the-tui.md")

	want := "ralph-tui v" + version.Version
	assertContains(t, content, want, "reading-the-tui.md ASCII diagram version label")
}

// TP-002: docs/code-packages/statusline.md payload example contains current version.
// Guards against the payload schema reference showing a stale version after a bump.
func TestDocIntegrity_StatuslineMd_PayloadVersionMatchesCurrent(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/code-packages/statusline.md")

	want := `"version": "` + version.Version + `"`
	assertContains(t, content, want, "docs/code-packages/statusline.md payload example version")
}

// TP-003a: docs/features/status-line.md payload example contains current version.
func TestDocIntegrity_StatusLineMd_PayloadVersionMatchesCurrent(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/features/status-line.md")

	want := `"version": "` + version.Version + `"`
	assertContains(t, content, want, "docs/features/status-line.md payload example version")
}

// TP-003b: docs/how-to/configuring-a-status-line.md version mentions are current.
// Pins both the prerequisites prose ("ralph-tui 0.6.0 or later") and the
// field-value table row (`"0.6.0"`).
func TestDocIntegrity_ConfiguringStatusLine_VersionMentionsCurrent(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/how-to/configuring-a-status-line.md")

	assertContains(t, content, "ralph-tui "+version.Version,
		"docs/how-to/configuring-a-status-line.md prerequisites prose")
	assertContains(t, content, `"`+version.Version+`"`,
		"docs/how-to/configuring-a-status-line.md field-value table")
}

// TP-004: docs/features/tui-display.md ASCII diagram shows the current version label.
// Parallel to TestDocIntegrity_ReadingTheTUI_AsciiDiagramVersionCurrent for the
// adjacent feature doc touched in the same commit.
func TestDocIntegrity_TUIDisplay_AsciiDiagramVersionCurrent(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/features/tui-display.md")

	want := "ralph-tui v" + version.Version
	assertContains(t, content, want, "docs/features/tui-display.md ASCII diagram version label")
}
