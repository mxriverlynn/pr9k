package cmuxctl_test

import (
	"os"
	"testing"
)

// socketTempDir returns a short-path directory suitable for binding Unix
// domain sockets.
//
// macOS limits a Unix socket address (sun_path) to 104 bytes. The per-test
// directories returned by t.TempDir() live under /var/folders/<...long...>/T/
// <TestName>/NNN, which routinely exceeds 104 bytes once "/cmux.sock" is
// appended — so net.Listen("unix", ...) fails with "bind: invalid argument"
// (EINVAL) on the primary development platform. This helper creates a short
// 0700 directory directly under /tmp (present and short on both macOS and
// Linux) so socket binds stay well under the limit. The 0700 mode keeps the
// socket's parent non-world-writable, so the cmuxctl D-15 check is unaffected;
// tests that exercise the world-writable path chmod the directory themselves.
func socketTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "p9k")
	if err != nil {
		t.Fatalf("socketTempDir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
