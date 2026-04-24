package workflowio_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
)

func TestDetectCrashTempFiles_ActivePID_ClassifiedAsActive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pid := os.Getpid()
	ns := time.Now().UnixNano()
	tempName := fmt.Sprintf("config.json.%d-%d.tmp", pid, ns)
	if err := os.WriteFile(filepath.Join(dir, tempName), []byte("data"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"iteration":[]}`), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	files, err := workflowio.DetectCrashTempFiles(dir)
	if err != nil {
		t.Fatalf("DetectCrashTempFiles: %v", err)
	}

	found := false
	for _, f := range files {
		if f.PID == pid {
			found = true
			if f.Classification != workflowio.CrashTempActive {
				t.Errorf("temp file with active PID classified as %v, want CrashTempActive", f.Classification)
			}
		}
	}
	if !found {
		t.Errorf("DetectCrashTempFiles: temp file with current PID %d not found in results", pid)
	}
}

func TestDetectCrashTempFiles_DeadPID_ClassifiedAsCrashEra(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Use a PID that far exceeds kernel max (Linux max_pid <= 4194304).
	deadPID := 999999999
	ns := time.Now().UnixNano()
	tempName := fmt.Sprintf("config.json.%d-%d.tmp", deadPID, ns)
	if err := os.WriteFile(filepath.Join(dir, tempName), []byte("data"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"iteration":[]}`), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	files, err := workflowio.DetectCrashTempFiles(dir)
	if err != nil {
		t.Fatalf("DetectCrashTempFiles: %v", err)
	}

	found := false
	for _, f := range files {
		if f.PID == deadPID {
			found = true
			if f.Classification != workflowio.CrashTempCrash {
				t.Errorf("temp file with dead PID classified as %v, want CrashTempCrash", f.Classification)
			}
		}
	}
	if !found {
		t.Errorf("DetectCrashTempFiles: temp file with dead PID %d not found", deadPID)
	}
}

func TestDetectCrashTempFiles_GlobPatternMatchesAtomicWriteTempFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Name exactly as atomicwrite does: <basename>.<pid>-<nanoseconds>.tmp
	pid := os.Getpid()
	ns := time.Now().UnixNano()
	tempName := fmt.Sprintf("config.json.%d-%d.tmp", pid, ns)
	if err := os.WriteFile(filepath.Join(dir, tempName), []byte("data"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"iteration":[]}`), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	files, err := workflowio.DetectCrashTempFiles(dir)
	if err != nil {
		t.Fatalf("DetectCrashTempFiles: %v", err)
	}

	found := false
	for _, f := range files {
		if filepath.Base(f.Path) == tempName {
			found = true
		}
	}
	if !found {
		t.Errorf("DetectCrashTempFiles: atomicwrite-pattern temp file %q not found in results", tempName)
	}
}

func TestDetectCrashTempFiles_CompanionTempFile_Detected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// config.json references a companion so companionBasenames returns "step-1.md".
	configJSON := `{"iteration":[{"name":"step","isClaude":true,"promptFile":"prompts/step-1.md"}]}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(configJSON), 0o600); err != nil {
		t.Fatalf("setup config.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0o700); err != nil {
		t.Fatalf("setup prompts dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "step-1.md"), []byte("# prompt"), 0o600); err != nil {
		t.Fatalf("setup companion: %v", err)
	}

	// Write a crash-temp file matching the companion basename pattern.
	pid := os.Getpid()
	ns := time.Now().UnixNano()
	tempName := fmt.Sprintf("step-1.md.%d-%d.tmp", pid, ns)
	if err := os.WriteFile(filepath.Join(dir, tempName), []byte("tmp"), 0o600); err != nil {
		t.Fatalf("setup companion temp file: %v", err)
	}

	files, err := workflowio.DetectCrashTempFiles(dir)
	if err != nil {
		t.Fatalf("DetectCrashTempFiles: %v", err)
	}

	found := false
	for _, f := range files {
		if filepath.Base(f.Path) == tempName {
			found = true
		}
	}
	if !found {
		t.Errorf("DetectCrashTempFiles: companion temp file %q not found in results %v", tempName, files)
	}
}

func TestDetectCrashTempFiles_NonWorkflowTempFiles_NotMatched(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// These files don't match the config.json.<pid>-<ns>.tmp pattern.
	for _, name := range []string{"something-else.tmp", "unrelated.backup.tmp", "random.tmp"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"iteration":[]}`), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	files, err := workflowio.DetectCrashTempFiles(dir)
	if err != nil {
		t.Fatalf("DetectCrashTempFiles: %v", err)
	}

	for _, f := range files {
		base := filepath.Base(f.Path)
		switch base {
		case "something-else.tmp", "unrelated.backup.tmp", "random.tmp":
			t.Errorf("DetectCrashTempFiles: non-workflow temp file %q should not be matched", base)
		}
	}
}
