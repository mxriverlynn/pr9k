package preflight

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveProfileDir_WithCLAUDE_CONFIG_DIR(t *testing.T) {
	want := "/custom/claude/dir"
	t.Setenv("CLAUDE_CONFIG_DIR", want)

	got := ResolveProfileDir()
	if got != want {
		t.Errorf("ResolveProfileDir() = %q, want %q", got, want)
	}
}

func TestResolveProfileDir_FallsBackToHomeClaud(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home := os.Getenv("HOME")

	got := ResolveProfileDir()
	want := filepath.Join(home, ".claude")
	if got != want {
		t.Errorf("ResolveProfileDir() = %q, want %q", got, want)
	}
}

func TestCheckProfileDir_NonexistentPath(t *testing.T) {
	err := CheckProfileDir("/tmp/ralph-preflight-nonexistent-xyzzy-12345")
	if err == nil {
		t.Fatal("expected error for nonexistent path, got nil")
	}
	if !strings.Contains(err.Error(), "claude profile directory not found") {
		t.Errorf("error %q does not contain expected text", err.Error())
	}
}

func TestCheckProfileDir_FilePath(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "profile-file-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	err = CheckProfileDir(f.Name())
	if err == nil {
		t.Fatal("expected error for file path, got nil")
	}
	if !strings.Contains(err.Error(), "claude profile path is not a directory") {
		t.Errorf("error %q does not contain expected text", err.Error())
	}
}

func TestCheckProfileDir_ValidDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := CheckProfileDir(dir); err != nil {
		t.Errorf("expected nil error for valid dir, got %v", err)
	}
}

// TP-003: CheckProfileDir wraps non-ErrNotExist stat errors with package context.
func TestCheckProfileDir_StatPermissionError_WrappedWithContext(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("requires non-root: root bypasses permission checks")
	}

	parent := t.TempDir()
	subdir := filepath.Join(parent, "profile")
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(parent, 0700)
	})

	err := CheckProfileDir(subdir)

	if err == nil {
		t.Fatal("expected error for permission-denied stat, got nil")
	}
	if !strings.Contains(err.Error(), "preflight:") {
		t.Errorf("expected preflight prefix in error, got: %q", err.Error())
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("expected permission error in chain, got: %v", err)
	}
}

// TP-004: ResolveProfileDir converts a relative CLAUDE_CONFIG_DIR to an absolute path.
func TestResolveProfileDir_RelativePath_BecomeAbsolute(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "relative/claude")

	got := ResolveProfileDir()

	if !filepath.IsAbs(got) {
		t.Errorf("expected absolute path, got %q", got)
	}
	if !strings.HasSuffix(got, "relative/claude") {
		t.Errorf("expected path ending with %q, got %q", "relative/claude", got)
	}
}

// SUGG-002: ResolveProfileDir falls back to cwd/.claude when both CLAUDE_CONFIG_DIR and HOME are empty.
func TestResolveProfileDir_BothEnvVarsEmpty_FallsBackToCwdClaud(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("HOME", "")

	got := ResolveProfileDir()

	if !filepath.IsAbs(got) {
		t.Errorf("expected absolute path, got %q", got)
	}
	if !strings.HasSuffix(got, ".claude") {
		t.Errorf("expected path ending with .claude, got %q", got)
	}
}

// SUGG-004: ResolveProfileDir trims trailing whitespace from CLAUDE_CONFIG_DIR.
func TestResolveProfileDir_TrailingWhitespace_Trimmed(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/custom/claude/dir  ")

	got := ResolveProfileDir()

	if got != "/custom/claude/dir" {
		t.Errorf("ResolveProfileDir() = %q, want %q", got, "/custom/claude/dir")
	}
}

// SUGG-003: ResolveProfileDir trims leading and trailing whitespace from
// CLAUDE_CONFIG_DIR (strings.TrimSpace, not just TrimRight).
func TestResolveProfileDir_LeadingAndTrailingWhitespace_Trimmed(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "  /custom/claude/dir  ")

	got := ResolveProfileDir()

	if got != "/custom/claude/dir" {
		t.Errorf("ResolveProfileDir() = %q, want %q", got, "/custom/claude/dir")
	}
}

func TestCheckCredentials_NoCredentialsFile(t *testing.T) {
	dir := t.TempDir()
	w, err := CheckCredentials(dir)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if w != "" {
		t.Errorf("expected empty warning, got %q", w)
	}
}

func TestCheckCredentials_ZeroByteCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	w, err := CheckCredentials(dir)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if !strings.Contains(w, "will likely fail authentication") {
		t.Errorf("warning %q does not contain expected text", w)
	}
}

// SUGG-003: CheckCredentials propagates non-ErrNotExist stat errors directly.
func TestCheckCredentials_StatPermissionError_PropagatedWrapped(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("requires non-root: root bypasses permission checks")
	}

	parent := t.TempDir()
	credPath := filepath.Join(parent, ".credentials.json")
	if err := os.WriteFile(credPath, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(parent, 0700)
	})

	_, err := CheckCredentials(parent)

	if err == nil {
		t.Fatal("expected error for permission-denied stat, got nil")
	}
	if !strings.Contains(err.Error(), "preflight:") {
		t.Errorf("expected preflight prefix in error, got: %q", err.Error())
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("expected permission error in chain, got: %v", err)
	}
}

func TestCheckCredentials_NonEmptyCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(path, []byte(`{"token":"abc"}`), 0600); err != nil {
		t.Fatal(err)
	}

	w, err := CheckCredentials(dir)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if w != "" {
		t.Errorf("expected empty warning for non-empty credentials, got %q", w)
	}
}
