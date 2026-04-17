package ui

import (
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/statusline"
)

// TestStatusReader_InterfaceAssertion is a compile-time check that
// *statusline.Runner satisfies the StatusReader interface documented in
// model.go. If Runner's method set drifts away from the interface (e.g., a
// method is renamed or removed), this file will fail to compile.
var _ StatusReader = (*statusline.Runner)(nil)
