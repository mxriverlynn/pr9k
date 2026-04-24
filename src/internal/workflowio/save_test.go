package workflowio_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// fakeSaveFS records WriteAtomic call order and can inject per-basename errors.
type fakeSaveFS struct {
	writeOrder []string
	writeErr   map[string]error // key: filepath.Base(path)
	statResult os.FileInfo
	statErr    error
}

func (f *fakeSaveFS) WriteAtomic(path string, data []byte, mode os.FileMode) error {
	base := filepath.Base(path)
	if f.writeErr != nil {
		if err, ok := f.writeErr[base]; ok {
			return err
		}
	}
	f.writeOrder = append(f.writeOrder, base)
	return nil
}

func (f *fakeSaveFS) Stat(path string) (os.FileInfo, error) {
	return f.statResult, f.statErr
}

type fakeFileInfo struct {
	modTime time.Time
	size    int64
}

func (fi *fakeFileInfo) Name() string       { return "config.json" }
func (fi *fakeFileInfo) Size() int64        { return fi.size }
func (fi *fakeFileInfo) Mode() os.FileMode  { return 0o600 }
func (fi *fakeFileInfo) ModTime() time.Time { return fi.modTime }
func (fi *fakeFileInfo) IsDir() bool        { return false }
func (fi *fakeFileInfo) Sys() any           { return nil }

// dirtyDoc returns a WorkflowDoc with a single step — distinct from an empty doc.
func dirtyDoc() workflowmodel.WorkflowDoc {
	return workflowmodel.WorkflowDoc{
		Steps: []workflowmodel.Step{
			{Name: "s1", Kind: workflowmodel.StepKindClaude, IsClaudeSet: true},
		},
	}
}

func TestSave_CompanionFilesWrittenBeforeConfig(t *testing.T) {
	t.Parallel()
	diskDoc := workflowmodel.WorkflowDoc{}
	memDoc := dirtyDoc()
	companions := map[string][]byte{"prompts/step-1.md": []byte("# prompt")}

	fs := &fakeSaveFS{
		statResult: &fakeFileInfo{modTime: time.Now(), size: 100},
	}

	result := workflowio.Save(fs, "/fake/dir", diskDoc, memDoc, companions)
	if result.Kind != workflowio.SaveErrorNone {
		t.Fatalf("Save: unexpected error kind %v: %v", result.Kind, result.Err)
	}

	configIdx := -1
	companionIdx := -1
	for i, name := range fs.writeOrder {
		if name == "config.json" {
			configIdx = i
		}
		if name == "step-1.md" {
			companionIdx = i
		}
	}
	if configIdx == -1 {
		t.Fatal("config.json was never written")
	}
	if companionIdx == -1 {
		t.Fatal("companion file step-1.md was never written")
	}
	if companionIdx > configIdx {
		t.Errorf("companion at write pos %d, config.json at %d: companion must be written first",
			companionIdx, configIdx)
	}
}

func TestSave_NoOp_FileNotRewritten(t *testing.T) {
	t.Parallel()
	doc := dirtyDoc()
	fs := &fakeSaveFS{}

	result := workflowio.Save(fs, "/fake/dir", doc, doc, nil)
	if result.Kind != workflowio.SaveErrorNone {
		t.Fatalf("Save: unexpected error kind %v", result.Kind)
	}
	if len(fs.writeOrder) != 0 {
		t.Errorf("Save wrote %d file(s) when doc is not dirty, want 0", len(fs.writeOrder))
	}
}

func TestSave_CompanionWriteFailure_RollsBack(t *testing.T) {
	t.Parallel()
	diskDoc := workflowmodel.WorkflowDoc{}
	memDoc := dirtyDoc()
	companions := map[string][]byte{"prompts/step-1.md": []byte("# prompt")}

	writeErr := errors.New("permission denied")
	fs := &fakeSaveFS{
		writeErr: map[string]error{"step-1.md": writeErr},
	}

	result := workflowio.Save(fs, "/fake/dir", diskDoc, memDoc, companions)
	if result.Kind == workflowio.SaveErrorNone {
		t.Fatal("Save: expected error, got SaveErrorNone")
	}
	for _, name := range fs.writeOrder {
		if name == "config.json" {
			t.Error("config.json was written after companion write failure")
		}
	}
}

func TestSave_ConfigRenameFailure_ReturnsTypedError(t *testing.T) {
	t.Parallel()
	diskDoc := workflowmodel.WorkflowDoc{}
	memDoc := dirtyDoc()

	fs := &fakeSaveFS{
		writeErr: map[string]error{"config.json": errors.New("write failed")},
	}

	result := workflowio.Save(fs, "/fake/dir", diskDoc, memDoc, nil)
	if result.Kind == workflowio.SaveErrorNone {
		t.Fatal("Save: expected non-None error kind")
	}
	if result.Err == nil {
		t.Error("Save: expected non-nil Err")
	}
}

func TestSave_EXDEV_ClassifiedAsSaveErrorEXDEV(t *testing.T) {
	t.Parallel()
	diskDoc := workflowmodel.WorkflowDoc{}
	memDoc := dirtyDoc()

	exdevErr := &os.PathError{Op: "rename", Path: "/tmp/x", Err: syscall.EXDEV}
	fs := &fakeSaveFS{
		writeErr: map[string]error{"config.json": exdevErr},
	}

	result := workflowio.Save(fs, "/fake/dir", diskDoc, memDoc, nil)
	if result.Kind != workflowio.SaveErrorEXDEV {
		t.Errorf("Save: expected SaveErrorEXDEV, got %v (err: %v)", result.Kind, result.Err)
	}
}

func TestSave_ErrPermission_ClassifiedAsSaveErrorPermission(t *testing.T) {
	t.Parallel()
	diskDoc := workflowmodel.WorkflowDoc{}
	memDoc := dirtyDoc()

	permErr := &os.PathError{Op: "open", Path: "/fake/dir/config.json", Err: syscall.EACCES}
	fs := &fakeSaveFS{
		writeErr: map[string]error{"config.json": permErr},
	}

	result := workflowio.Save(fs, "/fake/dir", diskDoc, memDoc, nil)
	if result.Kind != workflowio.SaveErrorPermission {
		t.Errorf("Save: expected SaveErrorPermission, got %v (err: %v)", result.Kind, result.Err)
	}
}

func TestMarshalDoc_IsClaudeOmittedWhenNotSet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	diskDoc := workflowmodel.WorkflowDoc{}

	// Step with IsClaudeSet == false: isClaude key must be absent.
	memDocShell := workflowmodel.WorkflowDoc{
		Steps: []workflowmodel.Step{
			{Name: "shell-step", Kind: workflowmodel.StepKindShell},
		},
	}
	result := workflowio.Save(workflowio.RealSaveFS(), dir, diskDoc, memDocShell, nil)
	if result.Kind != workflowio.SaveErrorNone {
		t.Fatalf("Save (no flag): %v", result.Err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Contains(data, []byte(`"isClaude"`)) {
		t.Errorf("isClaude key present in JSON when IsClaudeSet == false:\n%s", data)
	}

	// Step with IsClaudeSet == true, Kind == Claude: isClaude must be true.
	memDocClaude := workflowmodel.WorkflowDoc{
		Steps: []workflowmodel.Step{
			{Name: "claude-step", Kind: workflowmodel.StepKindClaude, IsClaudeSet: true},
		},
	}
	result2 := workflowio.Save(workflowio.RealSaveFS(), dir, diskDoc, memDocClaude, nil)
	if result2.Kind != workflowio.SaveErrorNone {
		t.Fatalf("Save (claude): %v", result2.Err)
	}
	data2, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Contains(data2, []byte(`"isClaude": true`)) {
		t.Errorf("isClaude: true not found in JSON when IsClaudeSet == true:\n%s", data2)
	}
}

func TestSave_ReturnsSnapshotOnSuccess(t *testing.T) {
	t.Parallel()
	diskDoc := workflowmodel.WorkflowDoc{}
	memDoc := dirtyDoc()

	wantMod := time.Now().Truncate(time.Second)
	fs := &fakeSaveFS{
		statResult: &fakeFileInfo{modTime: wantMod, size: 42},
	}

	result := workflowio.Save(fs, "/fake/dir", diskDoc, memDoc, nil)
	if result.Kind != workflowio.SaveErrorNone {
		t.Fatalf("Save: unexpected error: %v", result.Err)
	}
	if result.Snapshot == nil {
		t.Fatal("Save: Snapshot is nil on success")
	}
	if !result.Snapshot.ModTime.Equal(wantMod) {
		t.Errorf("Snapshot.ModTime = %v, want %v", result.Snapshot.ModTime, wantMod)
	}
	if result.Snapshot.Size != 42 {
		t.Errorf("Snapshot.Size = %d, want 42", result.Snapshot.Size)
	}
}

func TestSave_PreservesPhaseBoundaries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Write a config.json with 2 initialize, 1 iteration, 3 finalize steps.
	raw := `{
  "initialize": [
    {"name": "init-1", "isClaude": true},
    {"name": "init-2", "isClaude": true}
  ],
  "iteration": [
    {"name": "iter-1", "isClaude": true}
  ],
  "finalize": [
    {"name": "final-1", "isClaude": true},
    {"name": "final-2", "isClaude": true},
    {"name": "final-3", "isClaude": true}
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Load, then save back with an empty diskDoc to force a write.
	result, err := workflowio.Load(dir)
	if err != nil || result.RecoveryView != nil {
		t.Fatalf("Load: err=%v recoveryView=%v", err, result.RecoveryView != nil)
	}

	saveResult := workflowio.Save(workflowio.RealSaveFS(), dir, workflowmodel.WorkflowDoc{}, result.Doc, nil)
	if saveResult.Kind != workflowio.SaveErrorNone {
		t.Fatalf("Save: %v", saveResult.Err)
	}

	// Read saved JSON and verify each phase array length.
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var cfg struct {
		Initialize []json.RawMessage `json:"initialize"`
		Iteration  []json.RawMessage `json:"iteration"`
		Finalize   []json.RawMessage `json:"finalize"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got := len(cfg.Initialize); got != 2 {
		t.Errorf("initialize phase len = %d, want 2", got)
	}
	if got := len(cfg.Iteration); got != 1 {
		t.Errorf("iteration phase len = %d, want 1", got)
	}
	if got := len(cfg.Finalize); got != 3 {
		t.Errorf("finalize phase len = %d, want 3", got)
	}
}
