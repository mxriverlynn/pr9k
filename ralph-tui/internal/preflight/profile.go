package preflight

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ResolveProfileDir returns $CLAUDE_CONFIG_DIR if set and non-empty,
// else $HOME/.claude. The returned path is absolute (filepath.Abs applied)
// but symlinks are not resolved — profile dir realpath is not material
// for the stat check.
func ResolveProfileDir() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return dir
		}
		return abs
	}
	home := os.Getenv("HOME")
	abs, err := filepath.Abs(filepath.Join(home, ".claude"))
	if err != nil {
		return filepath.Join(home, ".claude")
	}
	return abs
}

// CheckProfileDir stats path and returns an error if it does not exist
// or is not a directory.
func CheckProfileDir(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("claude profile directory not found: %s. Set CLAUDE_CONFIG_DIR or create ~/.claude", path)
		}
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("claude profile path is not a directory: %s. Point CLAUDE_CONFIG_DIR at a directory", path)
	}
	return nil
}

// CheckCredentials performs a best-effort check for a zero-byte
// .credentials.json inside profileDir. A missing file is not a warning
// (fresh profile is valid). A zero-byte file returns a warning string.
// Any other stat error (besides ErrNotExist) is returned as an error.
func CheckCredentials(profileDir string) (warning string, _ error) {
	path := filepath.Join(profileDir, ".credentials.json")
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	if fi.Size() == 0 {
		return fmt.Sprintf(
			"Warning: %s is empty. Claude will likely fail authentication. Re-authenticate with 'claude login' inside the sandbox.",
			path,
		), nil
	}
	return "", nil
}
