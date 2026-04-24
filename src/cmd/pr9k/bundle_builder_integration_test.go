package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/version"
	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
	"github.com/mxriverlynn/pr9k/src/internal/workflowvalidate"
)

// TestVersion_WorkflowBuilderBumped verifies that the version has been
// incremented for the workflow-builder feature (WU-12 / issue #163).
// Version 0.7.1 was the pre-feature baseline; this test fails until the
// patch bump is committed.
func TestVersion_WorkflowBuilderBumped(t *testing.T) {
	if version.Version == "0.7.1" {
		t.Errorf("version %q: workflow-builder feature requires a patch bump; expected 0.7.2 or higher",
			version.Version)
	}
}

// TestBundleBuilderSmoke_DefaultBundleLoadsAndValidates verifies that the
// bundled default workflow (workflow/config.json) can be loaded and produces
// no fatal validation findings. This exercises the builder's primary startup
// path against a mirrored copy of the source-tree bundle (F-115).
func TestBundleBuilderSmoke_DefaultBundleLoadsAndValidates(t *testing.T) {
	root := docTestRepoRoot(t)
	srcBundle := filepath.Join(root, "workflow")

	tmpDir := t.TempDir()
	bundleMirrorDir(t, srcBundle, tmpDir)

	result, err := workflowio.Load(tmpDir)
	if err != nil {
		t.Fatalf("workflowio.Load: %v", err)
	}
	if result.RecoveryView != nil {
		t.Fatalf("Load returned a parse-failure recovery view:\n%s", string(result.RecoveryView))
	}

	findings := workflowvalidate.Validate(result.Doc, tmpDir, result.Companions)
	var fatalCount int
	for _, f := range findings {
		if f.IsFatal() {
			fatalCount++
			t.Logf("  fatal: %s", f.Error())
		}
	}
	if fatalCount > 0 {
		t.Errorf("ValidateDoc returned %d fatal finding(s) for the default bundle (see logged lines above)", fatalCount)
	}
}

// TestBundleBuilderSmoke_DefaultBundleHasNonEmptyDoc verifies that the default
// bundle produces a doc with at least one step. A doc with zero steps would make
// validation vacuous — zero findings are trivially produced by an empty step list.
// The exact count is not pinned; non-zero is the stable contract (T-1).
func TestBundleBuilderSmoke_DefaultBundleHasNonEmptyDoc(t *testing.T) {
	root := docTestRepoRoot(t)
	srcBundle := filepath.Join(root, "workflow")

	tmpDir := t.TempDir()
	bundleMirrorDir(t, srcBundle, tmpDir)

	result, err := workflowio.Load(tmpDir)
	if err != nil {
		t.Fatalf("workflowio.Load: %v", err)
	}
	if len(result.Doc.Steps) == 0 {
		t.Error("default bundle loaded with zero steps; expected a non-empty step list")
	}
}

// TestBundleBuilderSmoke_DefaultBundleCompanionsLoaded verifies that the default
// bundle populates result.Companions with at least one entry. If companion loading
// were silently broken, Companions would be empty and the validator would fall back
// to disk reads — the smoke test would pass while in-memory companion tracking broke
// (T-3).
func TestBundleBuilderSmoke_DefaultBundleCompanionsLoaded(t *testing.T) {
	root := docTestRepoRoot(t)
	srcBundle := filepath.Join(root, "workflow")

	tmpDir := t.TempDir()
	bundleMirrorDir(t, srcBundle, tmpDir)

	result, err := workflowio.Load(tmpDir)
	if err != nil {
		t.Fatalf("workflowio.Load: %v", err)
	}
	if len(result.Companions) == 0 {
		t.Error("default bundle loaded with empty Companions map; expected at least one companion file")
	}
}

// TestBundleBuilderSmoke_NoOpSave_DoesNotWrite verifies that a freshly-loaded
// bundle doc is not dirty against itself — meaning a save immediately after
// opening the default bundle is a no-op and no files would be written.
func TestBundleBuilderSmoke_NoOpSave_DoesNotWrite(t *testing.T) {
	root := docTestRepoRoot(t)
	srcBundle := filepath.Join(root, "workflow")

	tmpDir := t.TempDir()
	bundleMirrorDir(t, srcBundle, tmpDir)

	result, err := workflowio.Load(tmpDir)
	if err != nil {
		t.Fatalf("workflowio.Load: %v", err)
	}
	if workflowmodel.IsDirty(result.Doc, result.Doc) {
		t.Error("IsDirty(doc, doc) is true for a freshly-loaded bundle; expected false (save would be no-op)")
	}
}

// bundleMirrorDir recursively copies src into dst, preserving file modes
// (executable bits for scripts). dst must already exist (e.g. from t.TempDir()).
func bundleMirrorDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode()&0o777)
	})
	if err != nil {
		t.Fatalf("bundleMirrorDir %s → %s: %v", src, dst, err)
	}
}
