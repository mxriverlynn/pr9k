package version

import (
	"regexp"
	"testing"
)

// TP-109-03: Version follows semver format ^\d+\.\d+\.\d+$.
var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func TestVersion_FollowsSemver(t *testing.T) {
	if !semverRe.MatchString(Version) {
		t.Errorf("version.Version %q does not match semver pattern %s", Version, semverRe)
	}
}

// Pins the current release. Update this test intentionally when bumping.
// 0.7.1: PATCH bump for the onTimeout policy schema addition.
func TestVersion_PinnedAt071(t *testing.T) {
	if Version != "0.7.1" {
		t.Errorf("version must remain 0.7.1 for the onTimeout release; got %q — update this test intentionally when bumping", Version)
	}
}
