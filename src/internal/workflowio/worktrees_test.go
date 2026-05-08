package workflowio_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// captureSaveFS is a SaveFS that records the bytes written to config.json.
type captureSaveFS struct {
	written map[string][]byte
}

func (c *captureSaveFS) WriteAtomic(path string, data []byte, mode os.FileMode) error {
	if c.written == nil {
		c.written = make(map[string][]byte)
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	c.written[path] = cp
	return nil
}

func (c *captureSaveFS) Stat(_ string) (os.FileInfo, error) {
	return &captureFileInfo{}, nil
}

type captureFileInfo struct{}

func (captureFileInfo) Name() string       { return "config.json" }
func (captureFileInfo) Size() int64        { return 100 }
func (captureFileInfo) Mode() os.FileMode  { return 0o600 }
func (captureFileInfo) ModTime() time.Time { return time.Now() }
func (captureFileInfo) IsDir() bool        { return false }
func (captureFileInfo) Sys() any           { return nil }

// saveAndCapture calls workflowio.Save and returns the bytes written to
// config.json. It fails the test if Save reports an error.
func saveAndCapture(t *testing.T, _ *fakeSaveFS, diskDoc, memDoc workflowmodel.WorkflowDoc) []byte {
	t.Helper()
	fs := &captureSaveFS{}
	result := workflowio.Save(fs, "/fake/dir", diskDoc, memDoc, nil)
	if result.Kind != workflowio.SaveErrorNone {
		t.Fatalf("Save: unexpected error kind %v: %v", result.Kind, result.Err)
	}
	data, ok := fs.written["/fake/dir/config.json"]
	if !ok {
		t.Fatal("config.json was never written")
	}
	return data
}

// TestMarshal_Worktrees_RoundTrip verifies that a WorkflowDoc with a
// WorktreesBlock marshals to JSON containing the worktrees field and that
// the output parses back to the same values.
func TestMarshal_Worktrees_RoundTrip(t *testing.T) {
	t.Parallel()
	doc := workflowmodel.WorkflowDoc{
		Steps: []workflowmodel.Step{
			{Name: "s1", Kind: workflowmodel.StepKindShell, IsClaudeSet: false, Command: []string{"echo", "ok"}},
		},
		Worktrees: &workflowmodel.WorktreesBlock{Enabled: true, AutoCleanup: true},
	}
	diskDoc := workflowmodel.WorkflowDoc{}
	result := saveAndCapture(t, nil, diskDoc, doc)

	var out struct {
		Worktrees *struct {
			Enabled     bool `json:"enabled"`
			AutoCleanup bool `json:"autoCleanup"`
		} `json:"worktrees"`
	}
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if out.Worktrees == nil {
		t.Fatal("expected worktrees field in JSON output, got nil")
	}
	if !out.Worktrees.Enabled {
		t.Error("expected worktrees.enabled == true")
	}
	if !out.Worktrees.AutoCleanup {
		t.Error("expected worktrees.autoCleanup == true")
	}
}

// TestMarshal_Worktrees_Absent verifies that when Worktrees is nil in the
// WorkflowDoc, no worktrees key appears in the JSON output.
func TestMarshal_Worktrees_Absent(t *testing.T) {
	t.Parallel()
	doc := workflowmodel.WorkflowDoc{
		Steps: []workflowmodel.Step{
			{Name: "s1", Kind: workflowmodel.StepKindShell, IsClaudeSet: false, Command: []string{"echo", "ok"}},
		},
	}
	diskDoc := workflowmodel.WorkflowDoc{}
	result := saveAndCapture(t, nil, diskDoc, doc)

	var out map[string]json.RawMessage
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, ok := out["worktrees"]; ok {
		t.Error("expected no worktrees key in JSON when Worktrees is nil")
	}
}
