package workflowio_test

import (
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
	companions := map[string][]byte{"step-1.md": []byte("# prompt")}

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
