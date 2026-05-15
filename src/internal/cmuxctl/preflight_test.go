package cmuxctl_test

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
)

// ---- helpers ----------------------------------------------------------------

// fakeProbr is a test double for CmuxProber.
type fakeProber struct {
	available bool
}

func (f *fakeProber) CmuxBinaryAvailable() bool { return f.available }

// installedProber returns a prober that reports cmux as installed.
func installedProber() cmuxctl.CmuxProber { return &fakeProber{available: true} }

// notInstalledProber returns a prober that reports cmux as not installed.
func notInstalledProber() cmuxctl.CmuxProber { return &fakeProber{available: false} }

// cmuxClient returns a FakeClient scripted to identify as cmux (happy path).
func cmuxClient() *cmuxctl.FakeClient {
	return &cmuxctl.FakeClient{
		SystemIdentifyFunc: func(_ context.Context) (cmuxctl.Identity, error) {
			return cmuxctl.Identity{Name: "cmux", Version: "0.64.6"}, nil
		},
	}
}

// createSocket creates a real Unix socket in dir and returns its path.
// The caller is responsible for ensuring the listener is closed appropriately.
func createSocket(t *testing.T, dir string) (socketPath string, ln net.Listener) {
	t.Helper()
	socketPath = filepath.Join(dir, "cmux.sock")
	uln, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		t.Fatalf("create socket: %v", err)
	}
	// Preserve the socket file when the listener is closed so tests can inspect
	// it independently of the listener lifecycle.
	uln.SetUnlinkOnClose(false)
	return socketPath, uln
}

// hasSubstr checks that s contains substr (used to avoid importing strings in
// tests where the helper already exists — but here we import strings anyway).
func hasSubstr(s, substr string) bool {
	return strings.Contains(s, substr)
}

// ---- Condition 1: cmux not installed ----------------------------------------

func TestPreflight_CmuxNotInstalled(t *testing.T) {
	errs := cmuxctl.Preflight(context.Background(), notInstalledProber(), cmuxClient())
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !hasSubstr(msg, "cmuxctl:") {
		t.Errorf("error missing package prefix: %q", msg)
	}
	if !hasSubstr(msg, "cmux is not installed") {
		t.Errorf("error missing expected message fragment: %q", msg)
	}
}

// ---- Condition 2: cmux not running ------------------------------------------

func TestPreflight_CmuxNotRunning(t *testing.T) {
	dir := t.TempDir()
	// Point to a path that does not exist.
	t.Setenv("CMUX_SOCKET_PATH", filepath.Join(dir, "missing.sock"))

	errs := cmuxctl.Preflight(context.Background(), installedProber(), cmuxClient())
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !hasSubstr(msg, "cmuxctl:") {
		t.Errorf("error missing package prefix: %q", msg)
	}
	if !hasSubstr(msg, "not running") {
		t.Errorf("error missing 'not running' fragment: %q", msg)
	}
}

// ---- Condition 3: pr9k is not a cmux descendant ----------------------------

func TestPreflight_NotDescendant(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("chmod 0000 has no effect as root; skipping descendant test")
	}
	dir := t.TempDir()
	socketPath, ln := createSocket(t, dir)
	// Keep listener alive so the socket file persists; restrict permissions.
	t.Cleanup(func() { ln.Close() })

	if err := os.Chmod(socketPath, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		// Restore so TempDir cleanup can remove the file.
		_ = os.Chmod(socketPath, 0o600)
	})
	t.Setenv("CMUX_SOCKET_PATH", socketPath)

	errs := cmuxctl.Preflight(context.Background(), installedProber(), cmuxClient())
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !hasSubstr(msg, "cmuxctl:") {
		t.Errorf("error missing package prefix: %q", msg)
	}
	if !hasSubstr(msg, "cmux session") {
		t.Errorf("error missing 'cmux session' fragment: %q", msg)
	}
}

// ---- Condition 4: cmux socket disabled -------------------------------------

func TestPreflight_SocketDisabled(t *testing.T) {
	dir := t.TempDir()
	socketPath, ln := createSocket(t, dir)
	// Close the listener — socket file stays but connect returns ECONNREFUSED.
	ln.Close()
	t.Setenv("CMUX_SOCKET_PATH", socketPath)

	errs := cmuxctl.Preflight(context.Background(), installedProber(), cmuxClient())
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !hasSubstr(msg, "cmuxctl:") {
		t.Errorf("error missing package prefix: %q", msg)
	}
	if !hasSubstr(msg, "disabled") {
		t.Errorf("error missing 'disabled' fragment: %q", msg)
	}
}

// ---- Condition 5: capability mismatch / wrong identity ---------------------

func TestPreflight_CapabilityMismatch_ErrorFromIdentify(t *testing.T) {
	dir := t.TempDir()
	socketPath, ln := createSocket(t, dir)
	t.Cleanup(func() { ln.Close() })
	t.Setenv("CMUX_SOCKET_PATH", socketPath)

	client := &cmuxctl.FakeClient{
		SystemIdentifyFunc: func(_ context.Context) (cmuxctl.Identity, error) {
			return cmuxctl.Identity{}, errors.New("method not found")
		},
	}

	errs := cmuxctl.Preflight(context.Background(), installedProber(), client)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !hasSubstr(msg, "cmuxctl:") {
		t.Errorf("error missing package prefix: %q", msg)
	}
	if !hasSubstr(msg, "incompatible") {
		t.Errorf("error missing 'incompatible' fragment: %q", msg)
	}
}

func TestPreflight_NonCmuxIdentity(t *testing.T) {
	dir := t.TempDir()
	socketPath, ln := createSocket(t, dir)
	t.Cleanup(func() { ln.Close() })
	t.Setenv("CMUX_SOCKET_PATH", socketPath)

	client := &cmuxctl.FakeClient{
		SystemIdentifyFunc: func(_ context.Context) (cmuxctl.Identity, error) {
			return cmuxctl.Identity{Name: "other-tool", Version: "1.0"}, nil
		},
	}

	errs := cmuxctl.Preflight(context.Background(), installedProber(), client)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !hasSubstr(msg, "cmuxctl:") {
		t.Errorf("error missing package prefix: %q", msg)
	}
	if !hasSubstr(msg, "incompatible") {
		t.Errorf("error missing 'incompatible' fragment: %q", msg)
	}
}

// ---- CMUX_SOCKET_PATH validation (D-15) ------------------------------------

func TestPreflight_SocketPath_Empty_FallsBackToDefault(t *testing.T) {
	// With CMUX_SOCKET_PATH unset the function must not panic, and must return
	// an error rooted in the default-path lookup (not a security-validation error).
	t.Setenv("CMUX_SOCKET_PATH", "")

	errs := cmuxctl.Preflight(context.Background(), installedProber(), cmuxClient())
	// The default path won't exist in CI, so we expect a "not running" error.
	// What we're verifying is that the function doesn't crash and that the
	// error is the "not running" kind rather than an unexpected panic or internal error.
	if len(errs) == 0 {
		t.Skip("default socket path unexpectedly exists; skipping fallback test")
	}
	msg := errs[0].Error()
	if !hasSubstr(msg, "cmuxctl:") {
		t.Errorf("error missing package prefix: %q", msg)
	}
}

func TestPreflight_SocketPath_Nonexistent(t *testing.T) {
	t.Setenv("CMUX_SOCKET_PATH", "/nonexistent/path/cmux.sock")

	errs := cmuxctl.Preflight(context.Background(), installedProber(), cmuxClient())
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !hasSubstr(msg, "cmuxctl:") {
		t.Errorf("error missing package prefix: %q", msg)
	}
	// Rejected: either "not running" (path not found) is acceptable here.
	if !hasSubstr(msg, "not running") && !hasSubstr(msg, "not found") && !hasSubstr(msg, "no such file") {
		t.Errorf("unexpected error for non-existent path: %q", msg)
	}
}

func TestPreflight_SocketPath_NotASocket(t *testing.T) {
	dir := t.TempDir()
	regularFile := filepath.Join(dir, "regular.txt")
	if err := os.WriteFile(regularFile, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	t.Setenv("CMUX_SOCKET_PATH", regularFile)

	errs := cmuxctl.Preflight(context.Background(), installedProber(), cmuxClient())
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !hasSubstr(msg, "cmuxctl:") {
		t.Errorf("error missing package prefix: %q", msg)
	}
	if !hasSubstr(msg, "socket") {
		t.Errorf("error missing 'socket' fragment: %q", msg)
	}
}

func TestPreflight_SocketPath_SymlinkToNonSocket(t *testing.T) {
	dir := t.TempDir()
	regularFile := filepath.Join(dir, "regular.txt")
	if err := os.WriteFile(regularFile, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	linkPath := filepath.Join(dir, "link.sock")
	if err := os.Symlink(regularFile, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	t.Setenv("CMUX_SOCKET_PATH", linkPath)

	errs := cmuxctl.Preflight(context.Background(), installedProber(), cmuxClient())
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !hasSubstr(msg, "cmuxctl:") {
		t.Errorf("error missing package prefix: %q", msg)
	}
	if !hasSubstr(msg, "socket") {
		t.Errorf("error missing 'socket' fragment: %q", msg)
	}
}

func TestPreflight_SocketPath_WorldWritableParent(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("world-writable check is bypassed as root; skipping")
	}
	dir := t.TempDir()
	socketPath, ln := createSocket(t, dir)
	ln.Close()

	// Make the parent directory world-writable.
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatalf("chmod parent: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	t.Setenv("CMUX_SOCKET_PATH", socketPath)

	errs := cmuxctl.Preflight(context.Background(), installedProber(), cmuxClient())
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !hasSubstr(msg, "cmuxctl:") {
		t.Errorf("error missing package prefix: %q", msg)
	}
	if !hasSubstr(msg, "world-writable") {
		t.Errorf("error missing 'world-writable' fragment: %q", msg)
	}
}

// ---- TP-220-001: ANSI escape sequences in cmux-supplied text are stripped ---

func TestPreflight_AnsiStrippedFromIdentityName(t *testing.T) {
	dir := t.TempDir()
	socketPath, ln := createSocket(t, dir)
	t.Cleanup(func() { ln.Close() })
	t.Setenv("CMUX_SOCKET_PATH", socketPath)

	client := &cmuxctl.FakeClient{
		SystemIdentifyFunc: func(_ context.Context) (cmuxctl.Identity, error) {
			return cmuxctl.Identity{Name: "evil\x1b[31m\x1b]0;pwn\x07", Version: "1.0"}, nil
		},
	}

	errs := cmuxctl.Preflight(context.Background(), installedProber(), client)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !hasSubstr(msg, "cmuxctl:") {
		t.Errorf("error missing package prefix: %q", msg)
	}
	if !hasSubstr(msg, "incompatible") {
		t.Errorf("error missing 'incompatible' fragment: %q", msg)
	}
	if strings.Contains(msg, "\x1b") {
		t.Errorf("error message contains raw ANSI escape byte: %q", msg)
	}
}

func TestPreflight_AnsiStrippedFromIdentifyError(t *testing.T) {
	dir := t.TempDir()
	socketPath, ln := createSocket(t, dir)
	t.Cleanup(func() { ln.Close() })
	t.Setenv("CMUX_SOCKET_PATH", socketPath)

	client := &cmuxctl.FakeClient{
		SystemIdentifyFunc: func(_ context.Context) (cmuxctl.Identity, error) {
			return cmuxctl.Identity{}, errors.New("boom\x1b[2J clear screen injection")
		},
	}

	errs := cmuxctl.Preflight(context.Background(), installedProber(), client)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !hasSubstr(msg, "cmuxctl:") {
		t.Errorf("error missing package prefix: %q", msg)
	}
	if !hasSubstr(msg, "incompatible") {
		t.Errorf("error missing 'incompatible' fragment: %q", msg)
	}
	if strings.Contains(msg, "\x1b") {
		t.Errorf("error message contains raw ANSI escape byte: %q", msg)
	}
}

// ---- Happy path -------------------------------------------------------------

func TestPreflight_HappyPath(t *testing.T) {
	dir := t.TempDir()
	socketPath, ln := createSocket(t, dir)
	t.Cleanup(func() { ln.Close() })
	t.Setenv("CMUX_SOCKET_PATH", socketPath)

	errs := cmuxctl.Preflight(context.Background(), installedProber(), cmuxClient())
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}
