package cmuxctl

import (
	"os"
	"path/filepath"
	"strings"
)

// cmux socket discovery constants.
//
// pr9k mirrors cmux's *stable-variant* socket-discovery contract so that
// `pr9k --cmux` finds the same socket cmux's own CLI would, on every platform,
// without the operator having to set CMUX_SOCKET_PATH by hand. The contract is
// taken from cmux's CMUXSocketPathDomain (SocketPathMarkerFiles /
// CLISocketPathResolver): an env override, then a marker file whose contents
// are the live socket path, then the stable default socket, then the legacy
// default. Nightly/staging/dev cmux variants use different marker names; they
// are intentionally out of scope and remain reachable via CMUX_SOCKET_PATH.
const (
	// socketEnvCanonical is cmux's canonical socket-path override.
	socketEnvCanonical = "CMUX_SOCKET_PATH"
	// socketEnvAlias is cmux's deprecated compatibility alias for the override.
	socketEnvAlias = "CMUX_SOCKET"
	// cmuxConfigSubdir is the cmux-owned subdirectory inside the user config dir.
	cmuxConfigSubdir = "cmux"
	// stableSocketFileName is cmux's stable socket basename.
	stableSocketFileName = "cmux.sock"
	// markerFileName is cmux's stable "last socket path" marker basename.
	markerFileName = "last-socket-path"
	// tmpMarkerPath is cmux's /tmp mirror of the stable marker file.
	tmpMarkerPath = "/tmp/cmux-last-socket-path"
	// legacySocketPath is cmux's documented legacy default socket path. On
	// macOS this is rejected by the D-15 world-writable-parent check (/tmp is
	// 0777); it is retained only for contract fidelity and Linux setups where
	// an operator has explicitly placed the socket there.
	legacySocketPath = "/tmp/cmux.sock"
)

// socketDeps holds the OS interactions used by socket resolution. Production
// code uses realSocketDeps(); tests inject fakes for hermetic, cross-platform
// coverage.
type socketDeps struct {
	getenv        func(string) string
	userConfigDir func() (string, error)
	pathExists    func(string) bool
	readFile      func(string) ([]byte, error)
}

// realSocketDeps returns socketDeps wired to the real operating system.
func realSocketDeps() socketDeps {
	return socketDeps{
		getenv:        os.Getenv,
		userConfigDir: os.UserConfigDir,
		pathExists: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
		readFile: os.ReadFile,
	}
}

// resolveCmuxSocketPath returns the cmux socket path pr9k should use, plus the
// ordered, human-readable list of locations it considered (for diagnostics in
// the "socket not found" error). It never fails: when nothing resolves it
// returns the cmux-correct stable default so the error message names the path
// the operator should expect cmux to create.
//
// Resolution order (cmux stable-variant contract):
//  1. $CMUX_SOCKET_PATH (canonical override) — used verbatim.
//  2. $CMUX_SOCKET (deprecated alias) — used verbatim.
//  3. Marker file contents: <userConfigDir>/cmux/last-socket-path, then the
//     /tmp/cmux-last-socket-path mirror. The first marker that exists and
//     points at an existing socket wins.
//  4. Stable default: <userConfigDir>/cmux/cmux.sock. os.UserConfigDir() is
//     ~/Library/Application Support on macOS and ~/.config (or
//     $XDG_CONFIG_HOME) on Linux — exactly cmux's stable socket directory.
//  5. Legacy default: /tmp/cmux.sock.
func resolveCmuxSocketPath(d socketDeps) (chosen string, tried []string) {
	if v := strings.TrimSpace(d.getenv(socketEnvCanonical)); v != "" {
		return v, []string{socketEnvCanonical + "=" + v}
	}
	if v := strings.TrimSpace(d.getenv(socketEnvAlias)); v != "" {
		return v, []string{socketEnvAlias + "=" + v}
	}

	// Stable cmux directory: <userConfigDir>/cmux. May be empty if the OS
	// cannot report a user config dir (rare); the legacy default still applies.
	var stableDir string
	if cfg, err := d.userConfigDir(); err == nil {
		if cfg = strings.TrimSpace(cfg); cfg != "" {
			stableDir = filepath.Join(cfg, cmuxConfigSubdir)
		}
	}

	// Marker files: contents are the live socket path. Prefer the
	// user-config-dir marker over the /tmp mirror.
	markers := make([]string, 0, 2)
	if stableDir != "" {
		markers = append(markers, filepath.Join(stableDir, markerFileName))
	}
	markers = append(markers, tmpMarkerPath)
	for _, m := range markers {
		tried = append(tried, "marker "+m)
		data, err := d.readFile(m)
		if err != nil {
			continue
		}
		p := strings.TrimSpace(firstLine(string(data)))
		if p != "" && d.pathExists(p) {
			return p, tried
		}
	}

	if stableDir != "" {
		stable := filepath.Join(stableDir, stableSocketFileName)
		tried = append(tried, stable)
		if d.pathExists(stable) {
			return stable, tried
		}
		tried = append(tried, legacySocketPath)
		if d.pathExists(legacySocketPath) {
			return legacySocketPath, tried
		}
		// Nothing exists — name the cmux-correct stable default so the error
		// message points the operator at the right place.
		return stable, tried
	}

	tried = append(tried, legacySocketPath)
	return legacySocketPath, tried
}

// firstLine returns s up to (not including) the first newline. Marker files
// hold a single path but may have a trailing newline or stray extra lines.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
