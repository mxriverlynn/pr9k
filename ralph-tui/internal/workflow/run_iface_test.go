package workflow

import "github.com/mxriverlynn/pr9k/ralph-tui/internal/statusline"

// TP-001: compile-time assertion that *statusline.Runner satisfies StatusRunner.
// If Runner's method set drifts from the interface, this file fails to compile.
var _ StatusRunner = (*statusline.Runner)(nil)
