package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPath_ReturnsAbsolutePathInTempDir(t *testing.T) {
	path, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}

	tmpDir := os.TempDir()
	if !strings.HasPrefix(path, tmpDir) {
		t.Errorf("expected path under %q, got %q", tmpDir, path)
	}
}

func TestPath_MatchesRalphCidPattern(t *testing.T) {
	path, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	base := filepath.Base(path)
	if !strings.HasPrefix(base, "ralph-") || !strings.HasSuffix(base, ".cid") {
		t.Errorf("expected basename to match ralph-*.cid, got %q", base)
	}
}

func TestPath_FileDoesNotExistAfterReturn(t *testing.T) {
	path, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected path to not exist after Path() returns, but os.Stat returned: %v", err)
	}
}

func TestPath_ConsecutiveCallsReturnDistinctPaths(t *testing.T) {
	path1, err := Path()
	if err != nil {
		t.Fatalf("first Path() error: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path1) })

	path2, err := Path()
	if err != nil {
		t.Fatalf("second Path() error: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path2) })

	if path1 == path2 {
		t.Errorf("expected distinct paths, both returned %q", path1)
	}
}

func TestCleanup_RemovesExistingFile(t *testing.T) {
	f, err := os.CreateTemp("", "ralph-test-*.cid")
	if err != nil {
		t.Fatalf("CreateTemp error: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	path := f.Name()

	if err := Cleanup(path); err != nil {
		t.Fatalf("Cleanup error: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file to be removed, got stat result: %v", err)
	}
}

// TestCleanup_NonENOENTErrorPropagates verifies that Cleanup returns a non-nil
// error when os.Remove fails for a reason other than ENOENT (TP-007).
// A non-empty directory cannot be removed by os.Remove, triggering that path.
func TestCleanup_NonENOENTErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	// Create a file inside dir so dir is non-empty and os.Remove(dir) will fail.
	inner := filepath.Join(dir, "inner.txt")
	if err := os.WriteFile(inner, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := Cleanup(dir)
	if err == nil {
		t.Error("expected non-nil error when removing non-empty directory, got nil")
	}
}

func TestCleanup_ToleratesENOENT(t *testing.T) {
	// Generate a path that does not exist.
	path, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	// path already does not exist (Path removes it).

	if err := Cleanup(path); err != nil {
		t.Errorf("Cleanup on missing file should return nil, got %v", err)
	}
}
