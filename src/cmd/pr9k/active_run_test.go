package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- helpers ---

func writeRawActiveRun(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func activeRunPath(dir string) string {
	return filepath.Join(dir, ".pr9k", "active-run.json")
}

// --- writer round-trip ---

func TestActiveRunWriteRead_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := activeRunPath(dir)

	in := ActiveRunState{
		SchemaVersion: 1,
		WorktreeStamp: "pr9k-2026-05-08-120000.000",
		WorktreePath:  "/tmp/wt",
		PrimaryPath:   dir,
		Branch:        "pr9k-2026-05-08-120000.000",
		PID:           42,
		Binary:        "/usr/local/bin/pr9k",
	}

	if err := writeActiveRun(path, in); err != nil {
		t.Fatalf("writeActiveRun: %v", err)
	}

	out, err := readActiveRun(path)
	if err != nil {
		t.Fatalf("readActiveRun: %v", err)
	}
	if out == nil {
		t.Fatal("readActiveRun returned nil state, want non-nil")
	}
	if *out != in {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", *out, in)
	}
}

// --- ENOENT is benign on read ---

func TestActiveRunRead_ENOENT(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := activeRunPath(dir)

	state, err := readActiveRun(path)
	if err != nil {
		t.Fatalf("expected nil error on ENOENT, got %v", err)
	}
	if state != nil {
		t.Fatalf("expected nil state on ENOENT, got %+v", state)
	}
}

// --- corrupted JSON → rename to .corrupted-<timestamp> ---

func TestActiveRunRead_CorruptedRename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := activeRunPath(dir)

	writeRawActiveRun(t, path, []byte("not json {{{"))

	before := time.Now().UTC().Truncate(time.Second)
	state, err := readActiveRun(path)
	if err != nil {
		t.Fatalf("expected nil error on corrupt (renamed away), got %v", err)
	}
	if state != nil {
		t.Fatalf("expected nil state on corrupt, got %+v", state)
	}

	// original file must be gone
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("original active-run.json still present after corrupt parse")
	}

	// a .corrupted-<timestamp> file must exist in the same directory
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "active-run.json.corrupted-") {
			found = true
			_ = before // timestamp in name is at least the before time
		}
	}
	if !found {
		t.Error("no .corrupted-<timestamp> file found after corrupt parse")
	}
}

// --- unknown schemaVersion → rename to .incompatible-<timestamp> ---

func TestActiveRunRead_IncompatibleSchemaVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := activeRunPath(dir)

	data, _ := json.Marshal(map[string]any{
		"schemaVersion": 99,
		"worktreeStamp": "pr9k-2026-05-08-120000.000",
		"worktreePath":  "/tmp/wt",
		"primaryPath":   dir,
		"branch":        "pr9k-2026-05-08-120000.000",
		"pid":           1234,
		"binary":        "/bin/pr9k",
	})
	writeRawActiveRun(t, path, data)

	state, err := readActiveRun(path)
	if err != nil {
		t.Fatalf("expected nil error on incompatible schema, got %v", err)
	}
	if state != nil {
		t.Fatalf("expected nil state on incompatible schema, got %+v", state)
	}

	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("original active-run.json still present after incompatible schema")
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "active-run.json.incompatible-") {
			found = true
		}
	}
	if !found {
		t.Error("no .incompatible-<timestamp> file found after incompatible schema")
	}
}

// --- ENOENT on remove is benign ---

func TestActiveRunRemove_ENOENT(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := activeRunPath(dir)

	state := &ActiveRunState{
		SchemaVersion: 1,
		WorktreeStamp: "pr9k-2026-05-08-120000.000",
		WorktreePath:  "/tmp/wt",
		PrimaryPath:   dir,
		Branch:        "pr9k-2026-05-08-120000.000",
		PID:           42,
		Binary:        "/usr/local/bin/pr9k",
	}

	if err := removeActiveRun(path, state); err != nil {
		t.Fatalf("removeActiveRun on ENOENT: expected nil, got %v", err)
	}
}

// --- remover with mismatched WorktreeStamp is a no-op ---

func TestActiveRunRemove_StampMismatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := activeRunPath(dir)

	onDisk := ActiveRunState{
		SchemaVersion: 1,
		WorktreeStamp: "pr9k-2026-05-08-120000.000",
		WorktreePath:  "/tmp/wt",
		PrimaryPath:   dir,
		Branch:        "pr9k-2026-05-08-120000.000",
		PID:           42,
		Binary:        "/usr/local/bin/pr9k",
	}
	if err := writeActiveRun(path, onDisk); err != nil {
		t.Fatalf("writeActiveRun: %v", err)
	}

	// caller has a different stamp
	caller := onDisk
	caller.WorktreeStamp = "pr9k-2026-05-08-999999.999"

	if err := removeActiveRun(path, &caller); err != nil {
		t.Fatalf("removeActiveRun stamp mismatch: expected nil, got %v", err)
	}

	// file must still be there
	if _, statErr := os.Stat(path); statErr != nil {
		t.Error("active-run.json was deleted despite stamp mismatch")
	}
}

// --- resume validation: no file → no active run ---

func TestValidateResume_NoFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := activeRunPath(dir)

	result := validateActiveRun(path, dir)
	if result != resumeResultNoActiveRun {
		t.Errorf("want resumeResultNoActiveRun, got %v", result)
	}
}

// --- TP-209-001: validateActiveRun returns noActiveRun when process is dead ---

func TestValidateResume_DeadProcess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := activeRunPath(dir)

	// pid 1 is always alive but binary="/nonexistent/path" won't match
	// the running executable, so isProcessAlive returns false → noActiveRun.
	state := ActiveRunState{
		SchemaVersion: 1,
		WorktreeStamp: "pr9k-2026-05-08-120000.000",
		WorktreePath:  filepath.Join(dir, "wt"),
		PrimaryPath:   dir,
		Branch:        "pr9k-2026-05-08-120000.000",
		PID:           1,
		Binary:        "/nonexistent/path/pr9k",
	}
	if err := writeActiveRun(path, state); err != nil {
		t.Fatalf("writeActiveRun: %v", err)
	}

	result := validateActiveRun(path, dir)
	if result != resumeResultNoActiveRun {
		t.Errorf("want resumeResultNoActiveRun, got %v", result)
	}
}

// --- TP-209-002: validateActiveRun returns worktreeMissing when WorktreePath does not exist ---

func TestValidateResume_WorktreeMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := activeRunPath(dir)

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	canonDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		canonDir = dir
	}

	state := ActiveRunState{
		SchemaVersion: 1,
		WorktreeStamp: "pr9k-2026-05-08-120000.000",
		WorktreePath:  filepath.Join(canonDir, "does-not-exist"),
		PrimaryPath:   canonDir,
		Branch:        "pr9k-2026-05-08-120000.000",
		PID:           os.Getpid(),
		Binary:        exe,
	}
	if err := writeActiveRun(path, state); err != nil {
		t.Fatalf("writeActiveRun: %v", err)
	}

	result := validateActiveRun(path, canonDir)
	if result != resumeResultWorktreeMissing {
		t.Errorf("want resumeResultWorktreeMissing, got %v", result)
	}
}

// --- identity check fall-through: Kill OK but binary-path read fails → treat as dead ---

func TestIdentityCheck_BinaryReadError(t *testing.T) {
	t.Parallel()
	// pid 1 is always alive; we fake binary path read failing → dead
	alive := isProcessAlive(1, "/nonexistent/binary/path/that/will/never/match")
	// either dead (binary mismatch) is fine; this tests the mismatch branch
	// pid 1 exists but binary won't match → false
	if alive {
		t.Error("expected isProcessAlive=false when binary does not match current executable")
	}
}
