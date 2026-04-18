package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/version"
)

// repoRoot returns the workspace root directory, resolved from this test file's
// absolute path so the test works correctly regardless of the working directory.
func docTestRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	// file is .../src/cmd/pr9k/doc_integrity_test.go
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

	want := "pr9k v" + version.Version
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
// Pins both the prerequisites prose ("pr9k 0.6.1 or later") and the
// field-value table row (`"0.6.1"`).
func TestDocIntegrity_ConfiguringStatusLine_VersionMentionsCurrent(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/how-to/configuring-a-status-line.md")

	assertContains(t, content, "pr9k "+version.Version,
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

	want := "pr9k v" + version.Version
	assertContains(t, content, want, "docs/features/tui-display.md ASCII diagram version label")
}

// legacyName is the old binary name assembled at runtime so this test file
// does not itself contain the legacy string literal that the guard tests for.
var legacyName = "ralph" + "-tui"

// TestCIWorkflow_NoStaleRalphTuiReferences asserts that .github/workflows/ci.yml
// has been updated to reference the renamed src/ directory and contains no
// stale path references to the old tool name.
func TestCIWorkflow_NoStaleRalphTuiReferences(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, ".github/workflows/ci.yml")

	if strings.Contains(content, legacyName) {
		t.Errorf(".github/workflows/ci.yml still contains %q — update working-directory and cache-dependency-path to use src/", legacyName)
	}
	if !strings.Contains(content, "working-directory: src") {
		t.Error(".github/workflows/ci.yml does not contain \"working-directory: src\"")
	}
}

// TP-002: Pin that the Makefile copies src/config.json into bin/.pr9k/workflow/ and
// does not reference the legacy filename in any cp target.
func TestMakefile_CopiesConfigJSONToBin(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "Makefile")

	assertContains(t, content, "cp src/config.json bin/.pr9k/workflow/", "Makefile cp target")
	assertNotContains(t, content, legacyConfigName, "Makefile legacy config filename")
}

// TP-001: All four bundle-layout lines under bin/.pr9k/workflow/ are present
// and the three legacy top-level positions are absent.
func TestMakefile_BundleLayoutIsUnderPr9kWorkflow(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "Makefile")

	assertContains(t, content, "mkdir -p bin/.pr9k/workflow", "Makefile mkdir target")
	assertContains(t, content, "cp -r prompts bin/.pr9k/workflow/prompts", "Makefile prompts cp target")
	assertContains(t, content, "cp -r scripts bin/.pr9k/workflow/scripts", "Makefile scripts cp target")
	assertContains(t, content, "cp ralph-art.txt bin/.pr9k/workflow/", "Makefile ralph-art.txt cp target")

	assertNotContains(t, content, "cp -r prompts bin/prompts", "Makefile legacy prompts position")
	assertNotContains(t, content, "cp -r scripts bin/scripts", "Makefile legacy scripts position")
	// Legacy: "cp ralph-art.txt bin/" followed by newline (not "bin/.pr9k/…")
	assertNotContains(t, content, "cp ralph-art.txt bin/\n", "Makefile legacy ralph-art.txt position")
}

// TP-003: The Makefile copies scripts/ into bin/.pr9k/workflow/scripts/, which
// is where config.json command[0] prefixes ("scripts/X") will resolve against
// workflowDir at runtime.
func TestBundleLayout_MakefileWiresScriptsToWhereResolveCommandLooksForThem(t *testing.T) {
	root := docTestRepoRoot(t)
	makefile := readFile(t, root, "Makefile")
	configJSON := readFile(t, root, "src/config.json")

	// The Makefile must copy scripts/ into the bundle.
	assertContains(t, makefile, "cp -r scripts bin/.pr9k/workflow/scripts", "Makefile scripts bundle copy")

	// config.json commands that start with "scripts/" must reference source files
	// that actually exist under the repo-level scripts/ directory.
	lines := strings.Split(configJSON, "\n")
	for _, line := range lines {
		if idx := strings.Index(line, `"scripts/`); idx != -1 {
			// Extract the scripts/X portion up to the closing quote.
			rest := line[idx+1:]
			end := strings.IndexByte(rest, '"')
			if end == -1 {
				continue
			}
			scriptRef := rest[:end] // e.g. "scripts/get_next_issue"
			scriptPath := filepath.Join(root, scriptRef)
			assertFileExists(t, scriptPath)
		}
	}
}

// TestMakefile_BuildsBinaryNamedPr9k asserts that the Makefile builds the pr9k
// binary and references the renamed src/ directory.
func TestMakefile_BuildsBinaryNamedPr9k(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "Makefile")

	if strings.Contains(content, "./cmd/"+legacyName) {
		t.Errorf("Makefile still references ./cmd/%s — update to ./cmd/pr9k", legacyName)
	}
	if strings.Contains(content, "bin/"+legacyName) {
		t.Errorf("Makefile still references bin/%s — update to bin/pr9k", legacyName)
	}
	if !strings.Contains(content, "./cmd/pr9k") {
		t.Error("Makefile does not reference ./cmd/pr9k as the build target")
	}
	if !strings.Contains(content, "../bin/pr9k") {
		t.Error("Makefile does not reference ../bin/pr9k as the output binary")
	}
}

// TP-001: .gitignore contains the .pr9k/ entry (line-anchored to reject partial matches).
func TestGitignore_IgnoresPr9kDir(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, ".gitignore")
	assertContains(t, content, "\n.pr9k/\n", ".gitignore .pr9k/ entry")
}

// TP-002: .gitignore preserves the legacy logs/ entry.
func TestGitignore_PreservesLogsEntry(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, ".gitignore")
	assertContains(t, content, "\nlogs/\n", ".gitignore logs/ entry")
}

// TP-003: .gitignore preserves the .ralph-cache/ entry.
func TestGitignore_PreservesRalphCacheEntry(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, ".gitignore")
	assertContains(t, content, "\n.ralph-cache/\n", ".gitignore .ralph-cache/ entry")
}

// TP-004: git actually ignores .pr9k/anything (behavioral pin via git check-ignore).
func TestGitignore_Pr9kDirIsActuallyIgnoredByGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on $PATH")
	}
	root := docTestRepoRoot(t)
	cmd := exec.Command("git", "check-ignore", "-q", ".pr9k/test-file")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			t.Error(".pr9k/test-file is NOT ignored by git — check .gitignore for missing or negated .pr9k/ pattern")
		} else {
			t.Fatalf("git check-ignore failed unexpectedly: %v", err)
		}
	}
}

// forEachLiveDocFile calls fn(relative-path, content) for every Markdown file in the
// live doc surface: docs/ (excluding docs/plans/ and docs/adr/) plus README.md and CLAUDE.md.
// docs/plans/ and docs/adr/ are frozen historical records and are never visited.
func forEachLiveDocFile(t *testing.T, root string, fn func(rel, content string)) {
	t.Helper()
	for _, f := range []string{"README.md", "CLAUDE.md"} {
		fn(f, readFile(t, root, f))
	}
	docsDir := filepath.Join(root, "docs")
	err := filepath.WalkDir(docsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "plans" || name == "adr" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".md" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		fn(rel, string(data))
		return nil
	})
	if err != nil {
		t.Fatalf("forEachLiveDocFile WalkDir: %v", err)
	}
}

// TP-001 (HIGH): No legacy tool name in live docs outside allowed exemptions.
// Exemptions: the screenshot filename (workflow-identity artifact) and the
// frozen historical plan link are stripped before the check.
func TestDocSweep_NoLegacyToolNameInLiveDocs(t *testing.T) {
	root := docTestRepoRoot(t)
	exempt := []string{
		legacyName + "-screenshot.png",
		"docs/plans/" + legacyName + ".md",
		// ADR index entry in CLAUDE.md documents the rename; the arrow context is intentional.
		legacyName + "` → `pr9k`",
	}
	var offenders []string
	forEachLiveDocFile(t, root, func(rel, content string) {
		s := content
		for _, e := range exempt {
			s = strings.ReplaceAll(s, e, "")
		}
		if strings.Contains(s, legacyName) {
			offenders = append(offenders, rel)
		}
	})
	if len(offenders) > 0 {
		t.Errorf("TP-001: live docs contain %q outside allowed exemptions:\n  %s",
			legacyName, strings.Join(offenders, "\n  "))
	}
}

// TP-002 (HIGH): No legacy config filename in live docs.
func TestDocSweep_NoLegacyConfigNameInLiveDocs(t *testing.T) {
	root := docTestRepoRoot(t)
	// The ADR index entry in CLAUDE.md documents the rename; the arrow context is intentional.
	configExempt := legacyConfigName + "` → `config.json`"
	var offenders []string
	forEachLiveDocFile(t, root, func(rel, content string) {
		s := strings.ReplaceAll(content, configExempt, "")
		if strings.Contains(s, legacyConfigName) {
			offenders = append(offenders, rel)
		}
	})
	if len(offenders) > 0 {
		t.Errorf("TP-002: live docs contain %q — update to config.json:\n  %s",
			legacyConfigName, strings.Join(offenders, "\n  "))
	}
}

// TP-003 (HIGH): No legacy log paths in live docs.
// Checks that <projectDir>/logs/ and logs/ralph- (without .pr9k/ prefix) are absent.
func TestDocSweep_NoLegacyLogPathsInLiveDocs(t *testing.T) {
	root := docTestRepoRoot(t)
	var offenders []string
	forEachLiveDocFile(t, root, func(rel, content string) {
		for _, pat := range []string{"<projectDir>/logs/", "<project-dir>/logs/"} {
			if strings.Contains(content, pat) {
				offenders = append(offenders, rel+": contains "+pat)
			}
		}
		// logs/ralph- is only valid when preceded by .pr9k/
		stripped := strings.ReplaceAll(content, ".pr9k/logs/ralph-", "")
		if strings.Contains(stripped, "logs/ralph-") {
			offenders = append(offenders, rel+": contains logs/ralph- without .pr9k/ prefix")
		}
	})
	if len(offenders) > 0 {
		t.Errorf("TP-003: live docs contain legacy log paths:\n  %s", strings.Join(offenders, "\n  "))
	}
}

// TP-004 (HIGH): No legacy iteration log path in live docs.
func TestDocSweep_NoLegacyIterationJsonlInLiveDocs(t *testing.T) {
	root := docTestRepoRoot(t)
	var offenders []string
	forEachLiveDocFile(t, root, func(rel, content string) {
		if strings.Contains(content, legacyIterationPath) {
			offenders = append(offenders, rel)
		}
	})
	if len(offenders) > 0 {
		t.Errorf("TP-004: live docs contain %q — update to .pr9k/iteration.jsonl:\n  %s",
			legacyIterationPath, strings.Join(offenders, "\n  "))
	}
}

// TP-005 (HIGH): building-custom-workflows.md has the Per-Repo Workflow Override section
// with both resolution candidates documented.
func TestDocSweep_BuildingCustomWorkflows_PerRepoOverrideSection(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/how-to/building-custom-workflows.md")
	assertContains(t, content, "## Per-Repo Workflow Override",
		"building-custom-workflows.md: Per-Repo Workflow Override section header")
	assertContains(t, content, "<projectDir>/.pr9k/workflow/",
		"building-custom-workflows.md: in-repo override candidate path")
	assertContains(t, content, "<executableDir>/.pr9k/workflow/",
		"building-custom-workflows.md: shipped bundle candidate path")
}

// TP-006 (MED): README.md quickstart references the current binary name and not the legacy one.
func TestDocSweep_README_QuickstartReferencesSrc(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "README.md")
	assertContains(t, content, "bin/pr9k", "README.md: bin/pr9k binary reference")
	assertNotContains(t, content, "bin/"+legacyName, "README.md: stale legacy binary reference")
}

// TP-007 (MED): CLAUDE.md has no stale legacy cd commands and references src/.
func TestDocSweep_CLAUDEmd_NoStaleLegacyCdCommands(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "CLAUDE.md")
	if strings.Contains(content, "cd "+legacyName) {
		t.Errorf("CLAUDE.md: contains \"cd %s\" — update to \"cd src\"", legacyName)
	}
	assertContains(t, content, "cd src", "CLAUDE.md: cd src invocation")
	assertNotContains(t, content, "bin/"+legacyName, "CLAUDE.md: stale legacy binary reference")
}

// TP-008 (MED): The frozen historical plan link in README.md and CLAUDE.md is annotated
// with a "historical" or "original" qualifier so readers know the filename is intentionally frozen.
func TestDocSweep_FrozenPlanLinkAnnotated(t *testing.T) {
	root := docTestRepoRoot(t)
	plan := "docs/plans/" + legacyName + ".md"
	for _, f := range []string{"README.md", "CLAUDE.md"} {
		content := readFile(t, root, f)
		if !strings.Contains(content, plan) {
			t.Errorf("%s: does not reference %s", f, plan)
			continue
		}
		idx := strings.Index(content, plan)
		start := idx - 200
		if start < 0 {
			start = 0
		}
		end := idx + len(plan) + 200
		if end > len(content) {
			end = len(content)
		}
		lower := strings.ToLower(content[start:end])
		if !strings.Contains(lower, "historical") && !strings.Contains(lower, "original") {
			t.Errorf("%s: link to %s lacks a historical/original annotation in surrounding text", f, plan)
		}
	}
}

// TP-009 (MED): The screenshot image file still exists (exempted from rename per non-goals).
func TestDocSweep_ScreenshotFileExists(t *testing.T) {
	root := docTestRepoRoot(t)
	assertFileExists(t, filepath.Join(root, "images", legacyName+"-screenshot.png"))
}

// TP-010 (MED): caching-build-artifacts.md .gitignore block lists both .pr9k/ and .ralph-cache/.
func TestDocSweep_CachingBuildArtifacts_DualDirGitignoreBlock(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/how-to/caching-build-artifacts.md")
	assertContains(t, content, "# pr9k runtime state",
		"caching-build-artifacts.md: runtime state comment")
	assertContains(t, content, ".pr9k/",
		"caching-build-artifacts.md: .pr9k/ gitignore entry")
	assertContains(t, content, ".ralph-cache/",
		"caching-build-artifacts.md: .ralph-cache/ gitignore entry")
}

// TP-011 (MED): passing-environment-variables.md mentions both .pr9k/ and .ralph-cache/.
func TestDocSweep_PassingEnvVars_MentionsBothDirs(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/how-to/passing-environment-variables.md")
	assertContains(t, content, ".pr9k/",
		"passing-environment-variables.md: .pr9k/ directory reference")
	assertContains(t, content, ".ralph-cache/",
		"passing-environment-variables.md: .ralph-cache/ directory reference")
}

// TP-012 (MED): workflow-organization/design.md Status line is "Implemented" and
// lists all issues #135–#141.
func TestDocSweep_WorkflowOrgDesign_StatusImplemented(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/plans/workflow-organization/design.md")
	assertContains(t, content, "Implemented",
		"workflow-organization/design.md: Status is Implemented")
	for _, issue := range []string{"#135", "#136", "#137", "#138", "#139", "#140", "#141"} {
		assertContains(t, content, issue,
			"workflow-organization/design.md: Status lists "+issue)
	}
}

// TP-013 (LOW): docs/architecture.md has no logs/ralph- without .pr9k/ prefix.
// Focused follow-up to TP-003 for the known violation found during planning.
func TestDocSweep_Architecture_LogPathHasPr9kPrefix(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/architecture.md")
	stripped := strings.ReplaceAll(content, ".pr9k/logs/ralph-", "")
	if strings.Contains(stripped, "logs/ralph-") {
		t.Error("docs/architecture.md: contains logs/ralph- without .pr9k/ prefix")
	}
}

// TP-014 (LOW): No legacy cd commands in live docs (regression guard).
func TestDocSweep_NoStaleLegacyCdCommandsInDocs(t *testing.T) {
	root := docTestRepoRoot(t)
	cdLegacy := "cd " + legacyName
	var offenders []string
	forEachLiveDocFile(t, root, func(rel, content string) {
		if strings.Contains(content, cdLegacy) {
			offenders = append(offenders, rel)
		}
	})
	if len(offenders) > 0 {
		t.Errorf("TP-014: live docs contain %q — update to \"cd src\":\n  %s",
			cdLegacy, strings.Join(offenders, "\n  "))
	}
}

// TP-015 (LOW): workflow-organization/design.md non-goals document the screenshot filename exemption.
func TestDocSweep_WorkflowOrgDesign_ScreenshotExemptionDocumented(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/plans/workflow-organization/design.md")
	assertContains(t, content, legacyName+"-screenshot.png",
		"workflow-organization/design.md: screenshot filename exemption in non-goals")
}

// TP-001 (HIGH): the renamed ADR file exists at the exact path referenced in CLAUDE.md.
func TestDocIntegrity_ADR_Pr9kRenameFileExists(t *testing.T) {
	root := docTestRepoRoot(t)
	assertFileExists(t, filepath.Join(root, "docs", "adr", "20260418175134-pr9k-rename-and-pr9k-layout.md"))
}

// TP-002 (HIGH): CLAUDE.md's ## ADRs section lists the new ADR with required tokens.
func TestDocIntegrity_CLAUDEmd_IndexesNewPr9kRenameADR(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "CLAUDE.md")

	adrsIdx := strings.Index(content, "## ADRs")
	if adrsIdx == -1 {
		t.Fatal("CLAUDE.md: could not find '## ADRs' section")
	}
	nextSection := strings.Index(content[adrsIdx+len("## ADRs"):], "\n## ")
	var adrsSection string
	if nextSection == -1 {
		adrsSection = content[adrsIdx:]
	} else {
		adrsSection = content[adrsIdx : adrsIdx+len("## ADRs")+nextSection]
	}
	assertContains(t, adrsSection, "20260418175134-pr9k-rename-and-pr9k-layout.md", "CLAUDE.md ## ADRs section")
	assertContains(t, adrsSection, "Apply when", "CLAUDE.md ## ADRs section")
}

// TP-003 (HIGH): the ADR body contains identifying tokens for all four sub-decisions.
func TestDocIntegrity_ADR_CoversAllFourDecisions(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/adr/20260418175134-pr9k-rename-and-pr9k-layout.md")

	// Decision 1: binary rename
	assertContains(t, content, legacyName, "ADR decision 1: legacy name")
	assertContains(t, content, "pr9k", "ADR decision 1: new name")
	// Decision 2: .pr9k/ layout
	assertContains(t, content, ".pr9k/", "ADR decision 2: .pr9k/ layout")
	// Decision 3: two-candidate resolveWorkflowDir
	assertContains(t, content, "resolveWorkflowDir", "ADR decision 3: resolveWorkflowDir")
	assertContains(t, content, "two-candidate", "ADR decision 3: two-candidate rule")
	// Decision 4: config.json rename
	assertContains(t, content, legacyConfigName, "ADR decision 4: legacy config name")
	assertContains(t, content, "config.json", "ADR decision 4: new config name")
}

// TP-004 (HIGH): ADR Supersedes section names all three prior ADRs and each file exists.
func TestDocIntegrity_ADR_SupersedesSectionReferencesPriorADRs(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/adr/20260418175134-pr9k-rename-and-pr9k-layout.md")

	supersedesIdx := strings.Index(content, "## Supersedes")
	if supersedesIdx == -1 {
		t.Fatal("ADR: could not find '## Supersedes' section")
	}
	applyWhenIdx := strings.Index(content[supersedesIdx:], "## Apply when")
	var supersedesSection string
	if applyWhenIdx == -1 {
		supersedesSection = content[supersedesIdx:]
	} else {
		supersedesSection = content[supersedesIdx : supersedesIdx+applyWhenIdx]
	}

	priorADRs := []string{
		"20260413162428-workflow-project-dir-split.md",
		"20260413160000-require-docker-sandbox.md",
		"20260410170952-narrow-reading-principle.md",
	}
	for _, adr := range priorADRs {
		assertContains(t, supersedesSection, adr, "ADR Supersedes section")
		assertFileExists(t, filepath.Join(root, "docs", "adr", adr))
	}
}

// TP-005 (HIGH): ADR contains an ## Apply when section header.
func TestDocIntegrity_ADR_HasApplyWhenSection(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/adr/20260418175134-pr9k-rename-and-pr9k-layout.md")
	assertContains(t, content, "## Apply when", "ADR Apply when section")
}

// TP-006 (MED): ADR Non-goals section exists and lists the .ralph-cache/ and ralph- exemptions.
func TestDocIntegrity_ADR_NonGoalsListPresent(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/adr/20260418175134-pr9k-rename-and-pr9k-layout.md")
	assertContains(t, content, "## Non-goals", "ADR Non-goals section header")
	assertContains(t, content, ".ralph-cache/", "ADR Non-goals: .ralph-cache/ exemption")
	assertContains(t, content, "ralph-", "ADR Non-goals: ralph- prefix exemption")
}

// TP-007 (MED): CLAUDE.md index entry for the new ADR names all four decisions.
func TestDocIntegrity_CLAUDEmd_IndexEntryCoversAllDecisions(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "CLAUDE.md")

	lineIdx := strings.Index(content, "20260418175134-pr9k-rename-and-pr9k-layout.md")
	if lineIdx == -1 {
		t.Fatal("CLAUDE.md: could not find line referencing 20260418175134-pr9k-rename-and-pr9k-layout.md")
	}
	lineEnd := strings.IndexByte(content[lineIdx:], '\n')
	var line string
	if lineEnd == -1 {
		line = content[lineIdx:]
	} else {
		line = content[lineIdx : lineIdx+lineEnd]
	}

	assertContains(t, line, legacyName, "CLAUDE.md ADR index entry")
	assertContains(t, line, "pr9k", "CLAUDE.md ADR index entry")
	assertContains(t, line, ".pr9k/", "CLAUDE.md ADR index entry")
	assertContains(t, line, "resolveWorkflowDir", "CLAUDE.md ADR index entry")
	assertContains(t, line, legacyConfigName, "CLAUDE.md ADR index entry")
	assertContains(t, line, "config.json", "CLAUDE.md ADR index entry")
}

// TP-008 (MED): commit 32b668f only added files under docs/adr/ — none were modified or deleted.
// Note: this test is keyed to a specific commit SHA. If the commit is rewritten (e.g. squash
// merge), this test must be updated or deleted.
func TestDocIntegrity_ADR_ExistingADRsUnmodifiedByPr9kRenameCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on $PATH")
	}
	root := docTestRepoRoot(t)
	cmd := exec.Command("git", "show", "--name-status", "32b668f")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("commit 32b668f unreachable: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "docs/adr/") {
			continue
		}
		if !strings.HasPrefix(line, "A") {
			t.Errorf("expected commit 32b668f to only ADD docs/adr/ files, got: %s", line)
		}
	}
}

// TP-009 (LOW): ADR filename matches the 14-digit timestamp convention.
func TestDocIntegrity_ADR_Pr9kRenameFilenameConvention(t *testing.T) {
	root := docTestRepoRoot(t)
	adrDir := filepath.Join(root, "docs", "adr")
	entries, err := os.ReadDir(adrDir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", adrDir, err)
	}
	found := false
	for _, e := range entries {
		if e.Name() == "20260418175134-pr9k-rename-and-pr9k-layout.md" {
			found = true
		}
	}
	if !found {
		t.Error("docs/adr/: no file named 20260418175134-pr9k-rename-and-pr9k-layout.md found")
	}
}

// TP-010 (LOW): ADR is marked Status: accepted.
func TestDocIntegrity_ADR_StatusAccepted(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "docs/adr/20260418175134-pr9k-rename-and-pr9k-layout.md")
	assertContains(t, content, "Status:** accepted", "ADR status line")
}

// TP-005: git actually ignores logs/ and .ralph-cache/ (behavioral pin via git check-ignore).
func TestGitignore_LegacyDirsAreActuallyIgnoredByGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on $PATH")
	}
	root := docTestRepoRoot(t)
	for _, path := range []string{"logs/anything", ".ralph-cache/anything"} {
		cmd := exec.Command("git", "check-ignore", "-q", path)
		cmd.Dir = root
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				t.Errorf("%q is NOT ignored by git — check .gitignore for missing or negated pattern", path)
			} else {
				t.Fatalf("git check-ignore failed unexpectedly for %q: %v", path, err)
			}
		}
	}
}
