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

// CheckCredentials performs a best-effort check that the sandboxed claude
// will have credentials to authenticate with. A missing or zero-byte
// .credentials.json returns a warning string; non-ErrNotExist stat errors
// are returned as errors. When ANTHROPIC_API_KEY is set on the host, the
// sandbox authenticates via the BuiltinEnvAllowlist passthrough and the
// credentials file is not required — the file check is skipped entirely.
func CheckCredentials(profileDir string) (warning string, _ error) {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "", nil
	}
	path := filepath.Join(profileDir, ".credentials.json")
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Sprintf(
				"Warning: %s does not exist. The sandboxed claude has no credentials to authenticate with. Run 'pr9k sandbox --interactive' to authenticate, or set ANTHROPIC_API_KEY in the host environment.",
				path,
			), nil
		}
		return "", fmt.Errorf("preflight: stat credentials %s: %w", path, err)
	}
	if fi.Size() == 0 {
		return fmt.Sprintf(
			"Warning: %s is empty. Claude will likely fail authentication. Re-authenticate by running 'pr9k sandbox --interactive' and using '/login' inside the sandbox.",
			path,
		), nil
	}
	return "", nil
}
