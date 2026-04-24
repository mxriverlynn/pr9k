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
// 0.7.2: PATCH bump for the workflow-builder feature (issue #163 / WU-12).
func TestVersion_PinnedAt072(t *testing.T) {
	if Version != "0.7.2" {
		t.Errorf("version must remain 0.7.2 for the workflow-builder release; got %q — update this test intentionally when bumping", Version)
	}
}
