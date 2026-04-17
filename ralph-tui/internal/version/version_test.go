package version

import (
	"errors"
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
