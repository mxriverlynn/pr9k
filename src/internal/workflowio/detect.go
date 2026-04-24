package workflowio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DetectSymlink reports whether workflowDir/config.json is a symlink.
// If it is, target is the link's destination as returned by os.Readlink.
func DetectSymlink(workflowDir string) (isSymlink bool, target string, err error) {
	configPath := filepath.Join(workflowDir, "config.json")
	fi, err := os.Lstat(configPath)
	if err != nil {
		return false, "", fmt.Errorf("workflowio: DetectSymlink stat %s: %w", configPath, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return false, "", nil
	}
	dest, err := os.Readlink(configPath)
	if err != nil {
		return false, "", fmt.Errorf("workflowio: DetectSymlink readlink %s: %w", configPath, err)
	}
	return true, dest, nil
}

// DetectReadOnly reports whether the bundle directory is read-only for the
// current process by attempting to create and immediately remove a probe file.
func DetectReadOnly(workflowDir string) (bool, error) {
	probe := filepath.Join(workflowDir, ".pr9k-write-probe")
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return true, nil
		}
		return false, fmt.Errorf("workflowio: DetectReadOnly probe %s: %w", probe, err)
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return false, nil
}

// DetectExternalWorkflow reports whether workflowDir is outside projectDir.
func DetectExternalWorkflow(workflowDir, projectDir string) bool {
	clean := func(p string) string { return filepath.Clean(p) + string(os.PathSeparator) }
	return !strings.HasPrefix(clean(workflowDir), clean(projectDir))
}

// CreateEmptyCompanion creates an empty file at workflowDir/promptFile.
// It applies EvalSymlinks with ENOENT walkback, verifies containment within
// workflowDir, MkdirAll for missing intermediate directories, and rejects
// any existing non-regular file at the target (e.g. a FIFO). The file is
// created with O_CREATE|O_EXCL to prevent overwriting an existing companion.
func CreateEmptyCompanion(workflowDir, promptFile string) error {
	resolvedWorkflowDir, err := filepath.EvalSymlinks(workflowDir)
	if err != nil {
		return fmt.Errorf("workflowio: CreateEmptyCompanion eval workflowDir: %w", err)
	}

	targetPath := filepath.Join(workflowDir, promptFile)
	resolvedTarget, err := resolveWithWalkback(targetPath)
	if err != nil {
		return fmt.Errorf("workflowio: CreateEmptyCompanion resolve target: %w", err)
	}

	// Containment check: resolved parent must be under resolvedWorkflowDir.
	resolvedParent := filepath.Dir(resolvedTarget)
	rel, err := filepath.Rel(resolvedWorkflowDir, resolvedParent)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("workflowio: CreateEmptyCompanion: path escapes workflow dir: %s", promptFile)
	}

	// Create missing intermediate directories.
	if err := os.MkdirAll(resolvedParent, 0o700); err != nil {
		return fmt.Errorf("workflowio: CreateEmptyCompanion mkdir %s: %w", resolvedParent, err)
	}

	// Reject non-regular file already at target.
	if fi, err := os.Lstat(resolvedTarget); err == nil {
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("%w: companion target %s", ErrNotRegularFile, resolvedTarget)
		}
	}

	f, err := os.OpenFile(resolvedTarget, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("workflowio: CreateEmptyCompanion create %s: %w", resolvedTarget, err)
	}
	return f.Close()
}

// resolveWithWalkback resolves path via EvalSymlinks; on ENOENT walks toward
// the root until an existing ancestor is found, then appends the missing suffix.
func resolveWithWalkback(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("workflowio: eval symlinks %s: %w", path, err)
	}

	remaining := filepath.Base(path)
	dir := filepath.Dir(path)
	for {
		resolved, err = filepath.EvalSymlinks(dir)
		if err == nil {
			return filepath.Join(resolved, remaining), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("workflowio: eval symlinks %s: %w", dir, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("workflowio: no existing ancestor for %s", path)
		}
		remaining = filepath.Join(filepath.Base(dir), remaining)
		dir = parent
	}
}
