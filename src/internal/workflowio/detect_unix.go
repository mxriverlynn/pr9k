//go:build !windows

package workflowio

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// DetectSharedInstall reports whether the bundle directory is owned by a
// different UID than the current process. This indicates a system-wide install
// (e.g. /usr/local/share) that should not be edited in-place (D-43).
func DetectSharedInstall(workflowDir string) (bool, error) {
	resolved, err := filepath.EvalSymlinks(workflowDir)
	if err != nil {
		return false, fmt.Errorf("workflowio: DetectSharedInstall eval symlinks: %w", err)
	}
	var st syscall.Stat_t
	if err := syscall.Stat(resolved, &st); err != nil {
		return false, fmt.Errorf("workflowio: DetectSharedInstall stat %s: %w", resolved, err)
	}
	return uint32(st.Uid) != uint32(os.Getuid()), nil
}
