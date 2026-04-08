package workflow

import (
	"path/filepath"
	"strings"
)

// ResolveCommand replaces template variables in command and resolves relative
// script paths against projectDir.
//
// For each element:
//   - All occurrences of "{{ISSUE_ID}}" are replaced with issueID.
//   - The first element (the executable) is resolved relative to projectDir if
//     it is a relative path containing a path separator (i.e. not a bare
//     command like "git").
func ResolveCommand(projectDir string, command []string, issueID string) []string {
	if len(command) == 0 {
		return command
	}

	result := make([]string, len(command))
	for i, arg := range command {
		result[i] = strings.ReplaceAll(arg, "{{ISSUE_ID}}", issueID)
	}

	// Resolve the executable if it looks like a relative script path.
	exe := result[0]
	if !filepath.IsAbs(exe) && strings.ContainsRune(exe, '/') {
		result[0] = filepath.Join(projectDir, exe)
	}

	return result
}
