package ui

import "github.com/mxriverlynn/pr9k/ralph-tui/internal/statusline"

// Compile-time assertion: *statusline.Runner must satisfy StatusReader.
// Placed in a non-test file so every build (not just the test binary) detects
// interface drift if Runner's method set changes.
var _ StatusReader = (*statusline.Runner)(nil)
