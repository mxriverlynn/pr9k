package claudestream

import (
	"strings"
	"unicode"
)

// Slug converts a step name to a kebab-case identifier suitable for use in
// filenames (D14). Rules:
//   - Lowercased.
//   - Runs of non-alphanumeric characters (including spaces, punctuation, and
//     unicode non-letters/non-digits) are replaced by a single "-".
//   - Leading and trailing "-" are trimmed.
//
// Examples:
//
//	"Feature work"    → "feature-work"
//	"Fix review items"→ "fix-review-items"
//	"Close issue"     → "close-issue"
func Slug(name string) string {
	var sb strings.Builder
	inSep := true // treat start as a separator so leading dashes are suppressed

	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(unicode.ToLower(r))
			inSep = false
		} else {
			if !inSep {
				sb.WriteByte('-')
				inSep = true
			}
		}
	}

	s := sb.String()
	return strings.TrimRight(s, "-")
}
