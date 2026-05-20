package cmuxctl

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// cmuxDebug enables verbose diagnostics on stderr (the operator's persistent
// terminal, not a vanishing cmux pane) when PR9K_CMUX_DEBUG is non-empty.
// This is a temporary R3 investigation aid for the "workspace flashes then
// disappears / handshake: context canceled" failure.
var cmuxDebug = os.Getenv("PR9K_CMUX_DEBUG") != ""

var dbgMu sync.Mutex

// dbg prints a timestamped diagnostic line to stderr when PR9K_CMUX_DEBUG is
// set; otherwise it is a no-op.
func dbg(format string, a ...any) {
	if !cmuxDebug {
		return
	}
	dbgMu.Lock()
	defer dbgMu.Unlock()
	_, _ = fmt.Fprintf(os.Stderr, "[pr9k-cmux-debug] "+format+"\n", a...)
}

// workspaceIDs renders a workspace.list result compactly for diagnostics.
func workspaceIDs(ws []WorkspaceInfo) string {
	parts := make([]string, 0, len(ws))
	for _, w := range ws {
		parts = append(parts, w.ID+"|"+w.Ref)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
