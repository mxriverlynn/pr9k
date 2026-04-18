package claudestream_test

import (
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/claudestream"
)

func TestSlug(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Feature work", "feature-work"},
		{"Fix review items", "fix-review-items"},
		{"Close issue", "close-issue"},
		{"Update docs", "update-docs"},
		// Punctuation and mixed separators.
		{"Get next issue!", "get-next-issue"},
		{"  leading spaces  ", "leading-spaces"},
		{"trailing-dash-", "trailing-dash"},
		{"-leading-dash", "leading-dash"},
		// Numbers preserved.
		{"Step 2 of 3", "step-2-of-3"},
		// All separators collapse to one dash.
		{"a   b---c", "a-b-c"},
		// Unicode letters preserved and lowercased.
		{"Über cool", "über-cool"},
		// Single word.
		{"init", "init"},
		// Already kebab-case.
		{"already-kebab", "already-kebab"},
		// Empty string.
		{"", ""},
		// Only non-alphanumeric.
		{"!!!", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := claudestream.Slug(tc.name)
			if got != tc.want {
				t.Errorf("Slug(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
