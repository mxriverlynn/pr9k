// Package cmuxctl provides the cmux JSON-RPC 2.0 client interface and
// implementations used by pr9k's cmux mode.
package cmuxctl

import "context"

// Identity is returned by SystemIdentify.
type Identity struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// SplitOpts controls how a new pane is created relative to an existing pane.
type SplitOpts struct {
	// PaneID is the parent pane to split from; empty means the current pane.
	PaneID string `json:"pane_id,omitempty"`
}

// PaneInfo describes one pane returned by SurfaceList.
type PaneInfo struct {
	ID     string `json:"id"`
	Exited bool   `json:"exited"`
}

// CmuxClient is the RPC surface for pr9k's cmux Phase 1 workspace lifecycle.
// All methods accept a context for cancellation and deadline propagation.
type CmuxClient interface {
	SystemIdentify(ctx context.Context) (Identity, error)
	WorkspaceCurrent(ctx context.Context) (string, error)
	WorkspaceList(ctx context.Context) ([]string, error)
	WorkspaceCreate(ctx context.Context, name string) error
	WorkspaceClose(ctx context.Context, name string) error
	WorkspaceSelect(ctx context.Context, name string) error
	SurfaceSplit(ctx context.Context, opts SplitOpts) (string, error)
	SurfaceSpawn(ctx context.Context, paneID string, argv []string) error
	SurfaceHide(ctx context.Context, paneID string) error
	SurfaceList(ctx context.Context, workspaceName string) ([]PaneInfo, error)
}
