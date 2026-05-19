package main

import (
	"os"
	"path/filepath"
	"testing"
)

// shortSockPath returns a unique Unix-socket path under a short /tmp directory.
//
// macOS caps a Unix socket address (sun_path) at 104 bytes. The
// /var/folders/<...>/T/<TestName>/NNN paths t.TempDir() returns exceed that
// once "/test.sock" is appended, so net.Listen("unix", ...) fails with
// "bind: invalid argument" (EINVAL) on the primary development platform. /tmp
// is present and short on both macOS and Linux; the 0700 mode keeps the
// directory non-world-writable.
func shortSockPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "p9k")
	if err != nil {
		t.Fatalf("shortSockPath: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "test.sock")
}
