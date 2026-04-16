package version

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// repoRoot returns the workspace root directory, resolved from this test file's
// absolute path so the test works correctly regardless of the working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	// file is .../ralph-tui/internal/version/version_test.go
	// three levels up reaches the workspace root
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// TP-109-03: Version follows semver format ^\d+\.\d+\.\d+$.
var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func TestVersion_FollowsSemver(t *testing.T) {
	if !semverRe.MatchString(Version) {
		t.Errorf("version.Version %q does not match semver pattern %s", Version, semverRe)
	}
}

// TP-109-22: Previous version was 0.4.1.
// Checks git history to confirm the version bump came from the correct base.
func TestVersion_PreviousWas0_4_1(t *testing.T) {
	cmd := exec.Command("git", "show", "e41bd74^:ralph-tui/internal/version/version.go")
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("git show failed (git unavailable or commit not reachable): %v", err)
	}
	const want = "0.4.1"
	if !strings.Contains(string(out), want) {
		t.Errorf("expected previous version.go to contain %q; got:\n%s", want, out)
	}
}

// TP-109-23: Minor version bumped (not patch) — major=0, minor=5, patch=0.
// Confirms the v key binding and ModeSelect extension warranted a minor bump
// per docs/coding-standards/versioning.md for 0.y.z projects.
func TestVersion_MajorMinorPatch(t *testing.T) {
	parts := strings.SplitN(Version, ".", 3)
	if len(parts) != 3 {
		t.Fatalf("version %q does not have three dot-separated parts", Version)
	}
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"major", parts[0], "0"},
		{"minor", parts[1], "5"},
		{"patch", parts[2], "0"},
	}
	for _, c := range checks {
		if _, err := strconv.Atoi(c.got); err != nil {
			t.Errorf("%s part %q is not a valid integer", c.name, c.got)
		}
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}
