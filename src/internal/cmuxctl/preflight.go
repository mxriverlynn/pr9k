package cmuxctl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mxriverlynn/pr9k/src/internal/ansi"
)

// CmuxProber abstracts cmux binary presence for unit testing.
type CmuxProber interface {
	CmuxBinaryAvailable() bool
}

// RealCmuxProber is the production implementation of CmuxProber.
type RealCmuxProber struct{}

// CmuxBinaryAvailable reports whether the cmux binary is on PATH.
func (RealCmuxProber) CmuxBinaryAvailable() bool {
	_, err := exec.LookPath("cmux")
	return err == nil
}

// Preflight runs the five distinguishable cmux failure-condition checks and
// validates CMUX_SOCKET_PATH per D-15. Returns a non-empty slice on any
// failure; returns nil on success. Checks are run sequentially and the first
// blocking condition short-circuits the remaining checks.
func Preflight(ctx context.Context, prober CmuxProber, client CmuxClient) []error {
	// Condition 1: binary presence.
	if !prober.CmuxBinaryAvailable() {
		return []error{errors.New("cmuxctl: cmux is not installed; see the cmux setup how-to")}
	}

	// D-15: resolve and validate the socket path before net.Dial.
	socketPath, err := resolveSocketPath()
	if err != nil {
		return []error{err}
	}

	// Conditions 2, 3, 4: classify the dial error.
	// net.DialUnix is used instead of net.Dial to avoid the address-resolution
	// path (LookupPort) that is unnecessary for Unix sockets.
	conn, dialErr := net.DialUnix("unix", nil, &net.UnixAddr{Name: socketPath, Net: "unix"})
	if dialErr != nil {
		return []error{classifyDialError(dialErr, socketPath)}
	}
	_ = conn.Close()

	// Condition 5: capability check via system.identify.
	id, err := client.SystemIdentify(ctx)
	if err != nil {
		// TODO(#225): replace ansi.StripAll with ansi.StripForTerminalOutput once #225 lands.
		safe := string(ansi.StripAll([]byte(err.Error())))
		return []error{fmt.Errorf("cmuxctl: cmux version is incompatible with pr9k cmux mode: %s", safe)}
	}
	if id.Name != "cmux" {
		// TODO(OI-1): verify the expected identity name against the pinned cmux version.
		safe := string(ansi.StripAll([]byte(id.Name)))
		return []error{fmt.Errorf("cmuxctl: cmux version is incompatible with pr9k cmux mode: system.identify returned name=%q", safe)}
	}

	return nil
}

// resolveSocketPath resolves the cmux socket path via cmux's discovery
// contract (see resolveCmuxSocketPath) and performs the D-15 validation steps
// (EvalSymlinks, socket-type, world-writable parent directory). Returns the
// resolved canonical path or a package-prefixed error. The "not found" error
// names both the chosen path and every location considered, so the operator
// can see exactly where pr9k looked.
func resolveSocketPath() (string, error) {
	raw, tried := resolveCmuxSocketPath(realSocketDeps())
	notFound := func() error {
		return fmt.Errorf(
			"cmuxctl: cmux socket not found at %s (looked in: %s); start cmux, then launch pr9k from inside a cmux pane, or set CMUX_SOCKET_PATH",
			raw, strings.Join(tried, ", "))
	}

	// EvalSymlinks canonicalises the path and detects non-existent targets.
	resolved, err := filepath.EvalSymlinks(raw)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", notFound()
		}
		return "", fmt.Errorf("cmuxctl: cmux socket %s: %w", raw, err)
	}

	// Require a Unix socket type.
	info, err := os.Stat(resolved)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", notFound()
		}
		return "", fmt.Errorf("cmuxctl: cmux socket %s stat: %w", resolved, err)
	}
	if info.Mode().Type() != fs.ModeSocket {
		return "", fmt.Errorf("cmuxctl: %s is not a Unix socket", resolved)
	}

	// Reject world-writable parent directory (SEC-003 mitigation).
	parent := filepath.Dir(resolved)
	parentInfo, err := os.Stat(parent)
	if err != nil {
		return "", fmt.Errorf("cmuxctl: cmux socket parent directory %s: %w", parent, err)
	}
	if parentInfo.Mode().Perm()&0o002 != 0 {
		return "", fmt.Errorf("cmuxctl: cmux socket parent directory %s is world-writable (possible socket-redirection attack)", parent)
	}

	return resolved, nil
}

// classifyDialError maps low-level connection errors to the five distinguishable
// cmux failure conditions.
func classifyDialError(err error, socketPath string) error {
	var sysErr *os.SyscallError
	if errors.As(err, &sysErr) {
		errno := sysErr.Err
		if errors.Is(errno, syscall.EACCES) {
			return fmt.Errorf("cmuxctl: cmux mode must be launched from inside a cmux session (socket: %s)", socketPath)
		}
		if errors.Is(errno, syscall.ECONNREFUSED) {
			return errors.New("cmuxctl: cmux socket is disabled in cmux configuration; re-enable it and try again")
		}
	}
	// Fallback: treat as "not running" (covers ENOENT in the dial path and other transient errors).
	return errors.New("cmuxctl: cmux is installed but not running; start cmux and try again")
}
