package workflowio_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
)

func TestLoad_ParseErrorReturnsRecoveryView(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("not valid json at all"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	result, _ := workflowio.Load(dir)
	if result.RecoveryView == nil {
		t.Fatal("Load: expected non-nil RecoveryView for parse error")
	}
	if !bytes.Contains(result.RecoveryView, []byte("not valid json")) {
		t.Error("Load: RecoveryView does not contain the raw content")
	}
}

func TestLoad_RecoveryView_OSC8Stripped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Embed an OSC 8 hyperlink sequence into the raw content.
	osc8 := "\x1b]8;;http://example.com\x1b\\\x1b]8;;\x1b\\"
	content := "not-json " + osc8 + " tail"
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	result, _ := workflowio.Load(dir)
	if result.RecoveryView == nil {
		t.Fatal("Load: expected RecoveryView for malformed JSON")
	}
	for _, b := range result.RecoveryView {
		if b == 0x1b {
			t.Error("Load: RecoveryView contains ESC byte; ANSI stripping failed")
			break
		}
	}
}

func TestLoad_SymlinkDetectedBeforeParse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	actual := filepath.Join(dir, "actual.json")
	if err := os.WriteFile(actual, []byte("{{{not json"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Symlink(actual, filepath.Join(dir, "config.json")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	result, _ := workflowio.Load(dir)
	if !result.IsSymlink {
		t.Error("Load: expected IsSymlink=true when config.json is a symlink")
	}
	if result.RecoveryView == nil {
		t.Error("Load: expected RecoveryView for malformed JSON even when symlink detected")
	}
}

func TestLoad_FIFOTarget_Rejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "config.json")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}

	_, err := workflowio.Load(dir)
	if err == nil {
		t.Fatal("Load: expected error for FIFO target, got nil")
	}
	if !errors.Is(err, workflowio.ErrNotRegularFile) {
		t.Errorf("Load: want errors.Is(err, ErrNotRegularFile), got %v", err)
	}
}

func TestLoad_RegularFileTarget_Accepted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	config := `{"iteration":[{"name":"step-1","isClaude":true,"promptFile":"step-1.md"}]}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	result, err := workflowio.Load(dir)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if result.RecoveryView != nil {
		t.Error("Load: RecoveryView should be nil on successful parse")
	}
	if len(result.Doc.Steps) == 0 {
		t.Error("Load: expected non-empty Steps after successful parse")
	}
}
