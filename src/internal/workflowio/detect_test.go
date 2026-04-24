package workflowio_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
)

func TestDetectSymlink_NonSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"iteration":[]}`), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	isSymlink, _, err := workflowio.DetectSymlink(dir)
	if err != nil {
		t.Fatalf("DetectSymlink: %v", err)
	}
	if isSymlink {
		t.Error("DetectSymlink: expected false for regular file, got true")
	}
}

func TestDetectSymlink_IsSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	actual := filepath.Join(dir, "actual.json")
	if err := os.WriteFile(actual, []byte(`{"iteration":[]}`), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Symlink(actual, filepath.Join(dir, "config.json")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	isSymlink, target, err := workflowio.DetectSymlink(dir)
	if err != nil {
		t.Fatalf("DetectSymlink: %v", err)
	}
	if !isSymlink {
		t.Error("DetectSymlink: expected true for symlink, got false")
	}
	if target == "" {
		t.Error("DetectSymlink: expected non-empty target for symlink")
	}
}

func TestDetectReadOnly_WritableDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	readOnly, err := workflowio.DetectReadOnly(dir)
	if err != nil {
		t.Fatalf("DetectReadOnly: %v", err)
	}
	if readOnly {
		t.Error("DetectReadOnly: expected false for writable temp dir")
	}
}

func TestDetectExternalWorkflow_Inside(t *testing.T) {
	t.Parallel()
	projectDir := t.TempDir()
	workflowDir := filepath.Join(projectDir, ".pr9k", "workflow")

	if workflowio.DetectExternalWorkflow(workflowDir, projectDir) {
		t.Error("DetectExternalWorkflow: expected false when workflow is inside project")
	}
}

func TestDetectExternalWorkflow_Outside(t *testing.T) {
	t.Parallel()
	projectDir := t.TempDir()
	workflowDir := t.TempDir() // different temp dir = outside project

	if !workflowio.DetectExternalWorkflow(workflowDir, projectDir) {
		t.Error("DetectExternalWorkflow: expected true when workflow is outside project")
	}
}

func TestDetectSharedInstall_OwnedByCurrentUser_ReturnsFalse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	shared, err := workflowio.DetectSharedInstall(dir)
	if err != nil {
		t.Fatalf("DetectSharedInstall: %v", err)
	}
	if shared {
		t.Error("DetectSharedInstall: expected false for dir owned by current user")
	}
}

func TestCreateEmptyCompanion_MissingPromptsDir_CreatesItContained(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := workflowio.CreateEmptyCompanion(dir, "prompts/step-1.md"); err != nil {
		t.Fatalf("CreateEmptyCompanion: %v", err)
	}

	target := filepath.Join(dir, "prompts", "step-1.md")
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat created file: %v", err)
	}
	if !fi.Mode().IsRegular() {
		t.Error("CreateEmptyCompanion: created path is not a regular file")
	}
}

func TestCreateEmptyCompanion_EscapeAttempt_Rejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	err := workflowio.CreateEmptyCompanion(dir, "../../../etc/evil")
	if err == nil {
		t.Fatal("CreateEmptyCompanion: expected error for path escape, got nil")
	}
}

func TestCreateEmptyCompanion_FIFOAtTarget_Rejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	promptsDir := filepath.Join(dir, "prompts")
	if err := os.MkdirAll(promptsDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	fifoPath := filepath.Join(promptsDir, "step-1.md")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}

	err := workflowio.CreateEmptyCompanion(dir, "prompts/step-1.md")
	if err == nil {
		t.Fatal("CreateEmptyCompanion: expected error for FIFO at target, got nil")
	}
}
