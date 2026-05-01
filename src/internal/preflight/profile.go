package preflight

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveProfileDir returns $CLAUDE_CONFIG_DIR if set and non-empty,
// else $HOME/.claude. The returned path is absolute (filepath.Abs applied)
// but symlinks are not resolved — profile dir realpath is not material
// for the stat check. Trailing whitespace in CLAUDE_CONFIG_DIR is trimmed
// to guard against .env file parsers that include it.
func ResolveProfileDir() string {
	if dir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); dir != "" {
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
			return fmt.Errorf("preflight: claude profile directory not found: %s. Set CLAUDE_CONFIG_DIR or create ~/.claude", path)
		}
		return fmt.Errorf("preflight: stat profile dir %s: %w", path, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("preflight: claude profile path is not a directory: %s. Point CLAUDE_CONFIG_DIR at a directory", path)
	}
	return nil
}
