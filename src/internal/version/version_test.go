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

// TP-005 (issue #145): version must remain at 0.7.0 for this issue.
// Update this test intentionally when bumping.
func TestVersion_PinnedAt070(t *testing.T) {
	if Version != "0.7.0" {
		t.Errorf("version must remain 0.7.0 for issue #145; got %q — update this test intentionally when bumping", Version)
	}
}
