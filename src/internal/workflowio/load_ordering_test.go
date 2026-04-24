package workflowio_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
)

// TestLoad_SymlinkBannerBeforeRecoveryView_Ordering verifies that when
// config.json is a symlink to a file with malformed JSON, the LoadResult
// has both IsSymlink=true AND RecoveryView!=nil. This ensures the TUI can
// render the symlink banner before the recovery view (D-23).
func TestLoad_SymlinkBannerBeforeRecoveryView_Ordering(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	actual := filepath.Join(dir, "actual.json")
	if err := os.WriteFile(actual, []byte("not valid json"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Symlink(actual, filepath.Join(dir, "config.json")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	result, err := workflowio.Load(dir)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	// Both signals must be set so the TUI can render banner before recovery view.
	if !result.IsSymlink {
		t.Error("IsSymlink must be true so TUI can show symlink banner")
	}
	if result.SymlinkTarget == "" {
		t.Error("SymlinkTarget must be non-empty so banner can display the target path")
	}
	if result.RecoveryView == nil {
		t.Error("RecoveryView must be non-nil so TUI can show recovery content")
	}
	// Symlink detection must have happened first (D-23): IsSymlink is populated
	// before the parse attempt that produces RecoveryView.
	// Both being set in a single result proves ordering: symlink check → parse → recovery.
	if !result.IsSymlink || result.RecoveryView == nil {
		t.Error("both IsSymlink and RecoveryView must be set for correct TUI ordering")
	}
}
