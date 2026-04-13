package preflight

import (
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

// TP-003: CheckProfileDir returns the raw stat error for non-ErrNotExist failures.
func TestCheckProfileDir_StatPermissionError_PropagatedRaw(t *testing.T) {
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
	if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected raw stat error, not a custom message, got: %q", err.Error())
	}
	if !os.IsPermission(err) {
		t.Errorf("expected permission error, got: %v", err)
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
