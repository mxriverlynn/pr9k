package cmuxctl

import (
	"errors"
	"strings"
	"testing"
)

// fakeFS models a tiny in-memory filesystem for socket resolution tests:
// existing paths and the contents of readable marker files.
type fakeFS struct {
	exists map[string]bool
	files  map[string]string
}

func (f fakeFS) pathExists(p string) bool { return f.exists[p] }

func (f fakeFS) readFile(p string) ([]byte, error) {
	if v, ok := f.files[p]; ok {
		return []byte(v), nil
	}
	return nil, errors.New("not found")
}

// deps builds a socketDeps from explicit env values, a user-config-dir result,
// and a fakeFS.
func deps(env map[string]string, cfgDir string, cfgErr error, fs fakeFS) socketDeps {
	return socketDeps{
		getenv: func(k string) string { return env[k] },
		userConfigDir: func() (string, error) {
			if cfgErr != nil {
				return "", cfgErr
			}
			return cfgDir, nil
		},
		pathExists: fs.pathExists,
		readFile:   fs.readFile,
	}
}

func TestResolveCmuxSocketPath_CanonicalEnvWinsVerbatim(t *testing.T) {
	d := deps(
		map[string]string{
			socketEnvCanonical: "  /custom/cmux.sock  ",
			socketEnvAlias:     "/alias/cmux.sock",
		},
		"/Users/x/Library/Application Support", nil,
		fakeFS{exists: map[string]bool{"/custom/cmux.sock": true}},
	)
	got, tried := resolveCmuxSocketPath(d)
	if got != "/custom/cmux.sock" {
		t.Fatalf("chosen = %q, want trimmed canonical env value", got)
	}
	if len(tried) != 1 || !strings.Contains(tried[0], socketEnvCanonical) {
		t.Fatalf("tried = %v, want single canonical-env entry", tried)
	}
}

func TestResolveCmuxSocketPath_AliasUsedWhenCanonicalEmpty(t *testing.T) {
	d := deps(
		map[string]string{
			socketEnvCanonical: "   ",
			socketEnvAlias:     "/alias/cmux.sock",
		},
		"/home/x/.config", nil, fakeFS{},
	)
	got, _ := resolveCmuxSocketPath(d)
	if got != "/alias/cmux.sock" {
		t.Fatalf("chosen = %q, want alias env value", got)
	}
}

func TestResolveCmuxSocketPath_MarkerFileResolvesLiveSocket(t *testing.T) {
	cfg := "/Users/x/Library/Application Support"
	marker := cfg + "/cmux/last-socket-path"
	live := "/Users/x/Library/Application Support/cmux/cmux.sock"
	d := deps(
		map[string]string{},
		cfg, nil,
		fakeFS{
			exists: map[string]bool{live: true},
			files:  map[string]string{marker: live + "\n"},
		},
	)
	got, _ := resolveCmuxSocketPath(d)
	if got != live {
		t.Fatalf("chosen = %q, want marker-resolved live socket %q", got, live)
	}
}

func TestResolveCmuxSocketPath_AppSupportMarkerPreferredOverTmpMirror(t *testing.T) {
	cfg := "/Users/x/Library/Application Support"
	appMarker := cfg + "/cmux/last-socket-path"
	appSock := "/sock/app.sock"
	tmpSock := "/sock/tmp.sock"
	d := deps(
		map[string]string{},
		cfg, nil,
		fakeFS{
			exists: map[string]bool{appSock: true, tmpSock: true},
			files: map[string]string{
				appMarker:     appSock,
				tmpMarkerPath: tmpSock,
			},
		},
	)
	got, _ := resolveCmuxSocketPath(d)
	if got != appSock {
		t.Fatalf("chosen = %q, want app-support marker target %q", got, appSock)
	}
}

func TestResolveCmuxSocketPath_StaleMarkerFallsThroughToStableDefault(t *testing.T) {
	cfg := "/Users/x/Library/Application Support"
	marker := cfg + "/cmux/last-socket-path"
	stable := cfg + "/cmux/cmux.sock"
	d := deps(
		map[string]string{},
		cfg, nil,
		fakeFS{
			// Marker points at a socket that no longer exists; stable default does.
			exists: map[string]bool{stable: true},
			files:  map[string]string{marker: "/gone/old.sock"},
		},
	)
	got, _ := resolveCmuxSocketPath(d)
	if got != stable {
		t.Fatalf("chosen = %q, want stable default %q", got, stable)
	}
}

func TestResolveCmuxSocketPath_MacOSStableDefaultShape(t *testing.T) {
	cfg := "/Users/x/Library/Application Support"
	d := deps(map[string]string{}, cfg, nil, fakeFS{})
	got, tried := resolveCmuxSocketPath(d)
	want := cfg + "/cmux/cmux.sock"
	if got != want {
		t.Fatalf("chosen = %q, want macOS stable default %q", got, want)
	}
	if !strings.Contains(got, "Library/Application Support/cmux/cmux.sock") {
		t.Fatalf("macOS path shape unexpected: %q", got)
	}
	// Nothing existed, so every tier should appear in the diagnostics.
	joined := strings.Join(tried, " | ")
	for _, frag := range []string{"last-socket-path", tmpMarkerPath, want, legacySocketPath} {
		if !strings.Contains(joined, frag) {
			t.Errorf("tried %v missing %q", tried, frag)
		}
	}
}

func TestResolveCmuxSocketPath_LinuxStableDefaultShape(t *testing.T) {
	cfg := "/home/x/.config"
	d := deps(map[string]string{}, cfg, nil, fakeFS{})
	got, _ := resolveCmuxSocketPath(d)
	want := "/home/x/.config/cmux/cmux.sock"
	if got != want {
		t.Fatalf("chosen = %q, want Linux stable default %q", got, want)
	}
}

func TestResolveCmuxSocketPath_LegacyUsedWhenItExistsAndStableDoesNot(t *testing.T) {
	cfg := "/home/x/.config"
	d := deps(
		map[string]string{},
		cfg, nil,
		fakeFS{exists: map[string]bool{legacySocketPath: true}},
	)
	got, _ := resolveCmuxSocketPath(d)
	if got != legacySocketPath {
		t.Fatalf("chosen = %q, want legacy %q", got, legacySocketPath)
	}
}

func TestResolveCmuxSocketPath_UserConfigDirErrorFallsBackToLegacy(t *testing.T) {
	d := deps(map[string]string{}, "", errors.New("no home"), fakeFS{})
	got, tried := resolveCmuxSocketPath(d)
	if got != legacySocketPath {
		t.Fatalf("chosen = %q, want legacy %q on config-dir error", got, legacySocketPath)
	}
	if len(tried) == 0 || !strings.Contains(strings.Join(tried, " "), legacySocketPath) {
		t.Fatalf("tried = %v, want legacy entry", tried)
	}
}

func TestFirstLine(t *testing.T) {
	cases := map[string]string{
		"/a/b.sock":            "/a/b.sock",
		"/a/b.sock\n":          "/a/b.sock",
		"/a/b.sock\nextra\n":   "/a/b.sock",
		"\n/leading":           "",
		"/only-cr\r":           "/only-cr\r", // CR is not a line terminator here
	}
	for in, want := range cases {
		if got := firstLine(in); got != want {
			t.Errorf("firstLine(%q) = %q, want %q", in, got, want)
		}
	}
}
