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
