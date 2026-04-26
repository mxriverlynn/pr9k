package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/src/internal/version"
	"github.com/mxriverlynn/pr9k/src/internal/workflowedit"
	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
	"github.com/mxriverlynn/pr9k/src/internal/workflowvalidate"
)

// countingFS implements workflowio.SaveFS for integration tests, recording
// how many WriteAtomic calls were made and returning a synthetic FileInfo
// from Stat so the save pipeline can capture a non-nil SaveSnapshot.
type countingFS struct {
	writeAtomicCalls int
	modTime          time.Time
}

func (f *countingFS) WriteAtomic(_ string, _ []byte, _ os.FileMode) error {
	f.writeAtomicCalls++
	return nil
}

func (f *countingFS) Stat(_ string) (os.FileInfo, error) {
	return countingFSFileInfo{modTime: f.modTime}, nil
}

type countingFSFileInfo struct{ modTime time.Time }

func (i countingFSFileInfo) Name() string       { return "config.json" }
func (i countingFSFileInfo) Size() int64        { return 512 }
func (i countingFSFileInfo) Mode() os.FileMode  { return 0o600 }
func (i countingFSFileInfo) ModTime() time.Time { return i.modTime }
func (i countingFSFileInfo) IsDir() bool        { return false }
func (i countingFSFileInfo) Sys() any           { return nil }

// noopEditorRunner implements workflowedit.EditorRunner, dropping all
// invocations so the test never spawns a real editor process.
type noopEditorRunner struct{}

func (noopEditorRunner) Run(_ string, _ workflowedit.ExecCallback) tea.Cmd { return nil }

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

// TestBundleBuilderSmoke_DefaultBundleDrivesModelEndToEnd drives
// workflowedit.Model through the full load → Ctrl+S → save pipeline against
// the bundled default workflow (F-PR2-25, GAP-044).
func TestBundleBuilderSmoke_DefaultBundleDrivesModelEndToEnd(t *testing.T) {
	root := docTestRepoRoot(t)
	srcBundle := filepath.Join(root, "workflow")

	tmpDir := t.TempDir()
	bundleMirrorDir(t, srcBundle, tmpDir)

	result, err := workflowio.Load(tmpDir)
	if err != nil {
		t.Fatalf("workflowio.Load: %v", err)
	}

	// Step 1: construct model with a counting fake FS and no-op editor.
	fs := &countingFS{modTime: time.Now()}
	m := workflowedit.New(fs, noopEditorRunner{}, "/testproject", tmpDir).WithNoValidation()

	// Step 2: inject bundle. diskDoc is empty so the model is dirty and
	// Ctrl+S triggers a real config.json write. companions is nil so
	// writeAtomicCalls stays at exactly 1 (config.json only).
	loadMsg := workflowedit.LoadResultMsg(result.Doc, workflowmodel.WorkflowDoc{}, nil, tmpDir)
	next, _ := m.Update(loadMsg)
	m = next.(workflowedit.Model)

	// Step 3: send Ctrl+S to start the validate → save pipeline.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(workflowedit.Model)
	if cmd == nil {
		t.Fatal("Ctrl+S returned nil cmd; expected validate command")
	}

	// Steps 4–5: drain the async command chain (validate → save) until
	// the model reaches quiescence.
	for cmd != nil {
		msg := cmd()
		next, cmd = m.Update(msg)
		m = next.(workflowedit.Model)
	}

	// Step 6: assert save pipeline completed correctly.
	if fs.writeAtomicCalls != 1 {
		t.Errorf("WriteAtomic call count = %d; want 1", fs.writeAtomicCalls)
	}
	if m.IsDirty() {
		t.Error("model.IsDirty() is true after save; want false")
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
