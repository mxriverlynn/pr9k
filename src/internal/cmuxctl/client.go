// Package cmuxctl provides the cmux v2 socket client and the workspace
// lifecycle pr9k's cmux mode uses.
//
// Wire protocol (verified against cmux 0.64.7, commit 4d04459dd): newline-
// delimited JSON. Request {"id","method","params"}; success
// {"id","ok":true,"result":{…}}; error {"id","ok":false,"error":{"code":
// "<string>","message":"…"}}. Before the first request, a cmux in its default
// cmuxOnly mode writes a bare "ERROR: …\n" line to non-descendant clients and
// closes — handled as a typed plaintext error, not JSON.
//
// cmux identifies workspaces/surfaces by UUID plus a short ref ("workspace:2",
// "surface:4"); there is no unique workspace "name". pr9k therefore carries
// opaque handles, never names.
package cmuxctl

import "context"

// Identity is the cmux v2 system.identify result. cmux v2 returns no product
// name/version (verified at 2f96c15c2); a successful identify carrying a
// non-empty SocketPath is the capability proof that the peer is cmux v2.
type Identity struct {
	SocketPath string `json:"socket_path"`
}

// Workspace is an opaque cmux v2 workspace handle. ID is the UUID; Ref is the
// short form ("workspace:N"). Either is accepted as input by cmux.
type Workspace struct {
	ID  string `json:"workspace_id"`
	Ref string `json:"workspace_ref"`
}

// Empty reports whether the handle carries no usable identifier.
func (w Workspace) Empty() bool { return w.ID == "" && w.Ref == "" }

// Surface is an opaque cmux v2 surface handle (one terminal pane's content),
// returned by surface.split.
type Surface struct {
	SurfaceID  string `json:"surface_id"`
	SurfaceRef string `json:"surface_ref"`
	PaneID     string `json:"pane_id"`
	PaneRef    string `json:"pane_ref"`
}

// SplitDirection is the required cmux v2 surface.split direction.
type SplitDirection string

// cmux v2 surface.split accepts exactly these directions.
const (
	SplitLeft  SplitDirection = "left"
	SplitRight SplitDirection = "right"
	SplitUp    SplitDirection = "up"
	SplitDown  SplitDirection = "down"
)

// WorkspaceCreateOpts are the cmux v2 workspace.create params pr9k uses. The
// workspace's first terminal surface runs InitialCommand in WorkingDirectory
// with InitialEnv applied.
type WorkspaceCreateOpts struct {
	Title            string            `json:"title,omitempty"`
	WorkingDirectory string            `json:"working_directory,omitempty"`
	InitialCommand   string            `json:"initial_command,omitempty"`
	InitialEnv       map[string]string `json:"initial_env,omitempty"`
}

// SplitOpts are the cmux v2 surface.split params pr9k uses. Direction is
// required by cmux. The new surface runs InitialCommand; surface.split has no
// initial_env param, so env must be embedded in InitialCommand by the caller.
type SplitOpts struct {
	Workspace        Workspace      `json:"-"`
	SurfaceID        string         `json:"surface_id,omitempty"`
	Direction        SplitDirection `json:"direction"`
	WorkingDirectory string         `json:"working_directory,omitempty"`
	InitialCommand   string         `json:"initial_command,omitempty"`
}

// WorkspaceInfo is one entry of workspace.list (dismissal observation).
//
// NOTE: cmux's workspace.list summary objects use "id"/"ref" — NOT the
// "workspace_id"/"workspace_ref" keys used by workspace.create /
// workspace.current top-level handle echoes (verified at cmux 2f96c15c2,
// v2WorkspaceSummaryPayload). Getting these tags wrong makes every entry
// empty, so the dismissal observer never matches the pr9k workspace and
// false-fires "workspace removed" on its first poll, cancelling the
// readiness handshake.
type WorkspaceInfo struct {
	ID  string `json:"id"`
	Ref string `json:"ref"`
}

// SurfaceInfo is one entry of surface.list (dismissal observation). cmux
// reports a surface's liveness; a closed/exited surface flips Exited.
type SurfaceInfo struct {
	SurfaceID string `json:"surface_id"`
	Exited    bool   `json:"exited"`
}

// CmuxClient is the cmux v2 RPC surface pr9k's cmux mode uses. All methods
// accept a context for cancellation and deadline propagation.
//
// Note vs. the pre-rework interface: cmux v2 has no surface.spawn or
// surface.hide; workspace operations key on a Workspace handle, not a name;
// surface creation and the command it runs are a single call.
type CmuxClient interface {
	SystemIdentify(ctx context.Context) (Identity, error)
	WorkspaceCurrent(ctx context.Context) (Workspace, error)
	WorkspaceList(ctx context.Context) ([]WorkspaceInfo, error)
	WorkspaceCreate(ctx context.Context, opts WorkspaceCreateOpts) (Workspace, error)
	WorkspaceClose(ctx context.Context, ws Workspace) error
	WorkspaceSelect(ctx context.Context, ws Workspace) error
	SurfaceSplit(ctx context.Context, opts SplitOpts) (Surface, error)
	SurfaceList(ctx context.Context, ws Workspace) ([]SurfaceInfo, error)
	WorkspaceSetStatus(ctx context.Context, ws Workspace, key, value string) error
	WorkspaceClearStatus(ctx context.Context, ws Workspace, key string) error
	WorkspaceSetProgress(ctx context.Context, ws Workspace, fraction float64, label string) error
	WorkspaceClearProgress(ctx context.Context, ws Workspace) error
}
