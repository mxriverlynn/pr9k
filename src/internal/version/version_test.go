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
