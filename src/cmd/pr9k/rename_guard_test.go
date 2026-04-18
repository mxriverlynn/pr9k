package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// legacyToolName is the old binary/directory name, assembled at runtime to
// avoid the guard test itself appearing as a match in its own scan.
var legacyToolName = "ralph" + "-tui"

// skipBinary reports whether a file should be skipped because it is binary or
// hidden (and thus not a candidate for legacy name references).
func skipBinary(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch filepath.Ext(name) {
	case ".png", ".jpg", ".jpeg", ".gif", ".ico", ".woff", ".woff2", ".ttf", ".eot",
		".pdf", ".zip", ".tar", ".gz", ".sum":
		return true
	}
	return false
}

// skipDir reports whether a directory should be skipped entirely.
func skipDir(name string) bool {
	switch name {
	case ".git", ".ralph-cache", "bin", "vendor":
		return true
	}
	return skipBinary(name)
}

// skipFile reports whether a specific filename should be excluded from the scan.
// Workflow tracking files are excluded because they may contain historical references
// written before the rename and are never committed.
func skipFile(name string) bool {
	switch name {
	case "progress.txt", "deferred.txt", "test-plan.md", "code-review.md":
		return true
	}
	return false
}

// checkNoLegacyNameInTree walks root and fails the test if any non-excluded file
// contains the legacy tool name. All offending paths are collected before failing.
func checkNoLegacyNameInTree(t *testing.T, root string) {
	t.Helper()
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if skipBinary(d.Name()) || skipFile(d.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(data), legacyToolName) {
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir %s: %v", root, err)
	}
	if len(offenders) > 0 {
		t.Errorf("files in %s still contain %q (acceptance criterion: grep returns zero matches):\n  %s",
			root, legacyToolName, strings.Join(offenders, "\n  "))
	}
}

// TestNoLegacyRalphTuiReferences_Src asserts that no source file under src/
// contains the legacy tool name. This is an explicit acceptance criterion for
// the workflow-organization rename issue.
func TestNoLegacyRalphTuiReferences_Src(t *testing.T) {
	root := docTestRepoRoot(t)
	checkNoLegacyNameInTree(t, filepath.Join(root, "src"))
}

// TestNoLegacyRalphTuiReferences_Scripts asserts that no file under workflow/scripts/
// contains the legacy tool name.
func TestNoLegacyRalphTuiReferences_Scripts(t *testing.T) {
	root := docTestRepoRoot(t)
	checkNoLegacyNameInTree(t, filepath.Join(root, "workflow", "scripts"))
}

// legacyConfigName is the old config filename, assembled at runtime to avoid the
// guard matching its own source.
var legacyConfigName = "ralph-steps" + ".json"

// checkNoLegacyConfigInTree walks root and fails the test if any non-excluded file
// contains the legacy config filename.
func checkNoLegacyConfigInTree(t *testing.T, root string) {
	t.Helper()
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if skipBinary(d.Name()) || skipFile(d.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(data), legacyConfigName) {
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir %s: %v", root, err)
	}
	if len(offenders) > 0 {
		t.Errorf("files in %s still contain %q (acceptance criterion: grep returns zero matches):\n  %s",
			root, legacyConfigName, strings.Join(offenders, "\n  "))
	}
}

// TP-001: Regression guard — no file under src/ contains the legacy config filename.
func TestNoLegacyRalphStepsJSONReferences_Src(t *testing.T) {
	root := docTestRepoRoot(t)
	checkNoLegacyConfigInTree(t, filepath.Join(root, "src"))
}

// TP-001: Regression guard — no file under workflow/scripts/ contains the legacy config filename.
func TestNoLegacyRalphStepsJSONReferences_Scripts(t *testing.T) {
	root := docTestRepoRoot(t)
	checkNoLegacyConfigInTree(t, filepath.Join(root, "workflow", "scripts"))
}

// TP-001: Regression guard — the repo-root Makefile does not reference the legacy config filename.
func TestNoLegacyRalphStepsJSONReferences_Makefile(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "Makefile")
	if strings.Contains(content, legacyConfigName) {
		t.Errorf("Makefile contains %q — update to config.json", legacyConfigName)
	}
}

// legacyIterationPath is the old iteration log path, assembled at runtime to
// avoid the guard test itself appearing as a match in its own scan.
var legacyIterationPath = ".ralph" + "-cache/iteration.jsonl"

// checkNoLegacyIterationPathInTree walks root and fails if any non-excluded file
// contains the legacy iteration log path. A broad .ralph-cache walker is not
// viable (too many intentional preserves), but the exact combined path is narrow
// enough: after TP-001 it appears in no committed file.
func checkNoLegacyIterationPathInTree(t *testing.T, root string) {
	t.Helper()
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if skipBinary(d.Name()) || skipFile(d.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(data), legacyIterationPath) {
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir %s: %v", root, err)
	}
	if len(offenders) > 0 {
		t.Errorf("files in %s still contain %q — update to .pr9k/iteration.jsonl:\n  %s",
			root, legacyIterationPath, strings.Join(offenders, "\n  "))
	}
}

// TP-007: Regression guard — no file under src/ contains the legacy iteration log path.
func TestNoLegacyIterationJsonlPath_Src(t *testing.T) {
	root := docTestRepoRoot(t)
	checkNoLegacyIterationPathInTree(t, filepath.Join(root, "src"))
}

// TP-007: Regression guard — no file under workflow/scripts/ contains the legacy iteration log path.
func TestNoLegacyIterationJsonlPath_Scripts(t *testing.T) {
	root := docTestRepoRoot(t)
	checkNoLegacyIterationPathInTree(t, filepath.Join(root, "workflow", "scripts"))
}

// TP-007: Regression guard — the repo-root Makefile does not contain the legacy iteration log path.
func TestNoLegacyIterationJsonlPath_Makefile(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "Makefile")
	if strings.Contains(content, legacyIterationPath) {
		t.Errorf("Makefile contains %q — update to .pr9k/iteration.jsonl", legacyIterationPath)
	}
}

// TP-002 (issue #145): Extend rename-guard coverage to workflow/prompts/ and workflow/config.json.

// TestNoLegacyRalphTuiReferences_WorkflowPrompts asserts no file under workflow/prompts/
// contains the legacy tool name.
func TestNoLegacyRalphTuiReferences_WorkflowPrompts(t *testing.T) {
	root := docTestRepoRoot(t)
	checkNoLegacyNameInTree(t, filepath.Join(root, "workflow", "prompts"))
}

// TestNoLegacyRalphStepsJSONReferences_WorkflowPrompts asserts no file under workflow/prompts/
// contains the legacy config filename.
func TestNoLegacyRalphStepsJSONReferences_WorkflowPrompts(t *testing.T) {
	root := docTestRepoRoot(t)
	checkNoLegacyConfigInTree(t, filepath.Join(root, "workflow", "prompts"))
}

// TestNoLegacyIterationJsonlPath_WorkflowPrompts asserts no file under workflow/prompts/
// contains the legacy iteration log path.
func TestNoLegacyIterationJsonlPath_WorkflowPrompts(t *testing.T) {
	root := docTestRepoRoot(t)
	checkNoLegacyIterationPathInTree(t, filepath.Join(root, "workflow", "prompts"))
}

// TestNoLegacyRalphTuiReferences_WorkflowConfigJSON asserts workflow/config.json
// does not contain the legacy tool name.
func TestNoLegacyRalphTuiReferences_WorkflowConfigJSON(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "workflow/config.json")
	if strings.Contains(content, legacyToolName) {
		t.Errorf("workflow/config.json contains %q — update to pr9k conventions", legacyToolName)
	}
}

// TestNoLegacyRalphStepsJSONReferences_WorkflowConfigJSON asserts workflow/config.json
// does not contain the legacy config filename.
func TestNoLegacyRalphStepsJSONReferences_WorkflowConfigJSON(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "workflow/config.json")
	if strings.Contains(content, legacyConfigName) {
		t.Errorf("workflow/config.json contains %q — update to config.json conventions", legacyConfigName)
	}
}

// TestNoLegacyIterationJsonlPath_WorkflowConfigJSON asserts workflow/config.json
// does not contain the legacy iteration log path.
func TestNoLegacyIterationJsonlPath_WorkflowConfigJSON(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "workflow/config.json")
	if strings.Contains(content, legacyIterationPath) {
		t.Errorf("workflow/config.json contains %q — update to .pr9k/iteration.jsonl", legacyIterationPath)
	}
}
