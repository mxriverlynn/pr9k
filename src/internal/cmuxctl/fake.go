package cmuxctl

import (
	"context"
	"sync"
)

// FakeClient is a test double for the cmux v2 CmuxClient. Every method is
// scriptable via its Func field; unset Funcs return sensible defaults so a
// zero FakeClient drives RunPhase1 to completion. All mutable state is mu-guarded.
//
// HangNext/HangRelease let tests simulate a hanging RPC: send to HangNext
// before the call, and the call blocks until HangRelease (or ctx cancel).
type FakeClient struct {
	mu sync.Mutex

	SystemIdentifyFunc   func(ctx context.Context) (Identity, error)
	WorkspaceCurrentFunc func(ctx context.Context) (Workspace, error)
	WorkspaceListFunc    func(ctx context.Context) ([]WorkspaceInfo, error)
	WorkspaceCreateFunc  func(ctx context.Context, opts WorkspaceCreateOpts) (Workspace, error)
	WorkspaceCloseFunc   func(ctx context.Context, ws Workspace) error
	WorkspaceSelectFunc  func(ctx context.Context, ws Workspace) error
	SurfaceSplitFunc     func(ctx context.Context, opts SplitOpts) (Surface, error)
	SurfaceListFunc      func(ctx context.Context, ws Workspace) ([]SurfaceInfo, error)

	// Recorders — appended under mu; read after all goroutines have joined.
	CreateCalls []WorkspaceCreateOpts
	CloseCalls  []Workspace
	SelectCalls []Workspace
	SplitCalls  []SplitOpts

	HangNext    chan struct{}
	HangRelease chan struct{}
}

var _ CmuxClient = (*FakeClient)(nil)

// SetHangChannels sets HangNext and HangRelease under f.mu.
func (f *FakeClient) SetHangChannels(next, release chan struct{}) {
	f.mu.Lock()
	f.HangNext = next
	f.HangRelease = release
	f.mu.Unlock()
}

func (f *FakeClient) maybehang(ctx context.Context) error {
	f.mu.Lock()
	hangNext := f.HangNext
	hangRelease := f.HangRelease
	f.mu.Unlock()

	if hangNext == nil {
		return nil
	}
	select {
	case <-hangNext:
	default:
		return nil
	}
	select {
	case <-hangRelease:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *FakeClient) SystemIdentify(ctx context.Context) (Identity, error) {
	if err := f.maybehang(ctx); err != nil {
		return Identity{}, err
	}
	f.mu.Lock()
	fn := f.SystemIdentifyFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	return Identity{SocketPath: "/fake/cmux.sock"}, nil
}

func (f *FakeClient) WorkspaceCurrent(ctx context.Context) (Workspace, error) {
	if err := f.maybehang(ctx); err != nil {
		return Workspace{}, err
	}
	f.mu.Lock()
	fn := f.WorkspaceCurrentFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	return Workspace{}, nil
}

func (f *FakeClient) WorkspaceList(ctx context.Context) ([]WorkspaceInfo, error) {
	if err := f.maybehang(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	fn := f.WorkspaceListFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	return nil, nil
}

func (f *FakeClient) WorkspaceCreate(ctx context.Context, opts WorkspaceCreateOpts) (Workspace, error) {
	if err := f.maybehang(ctx); err != nil {
		return Workspace{}, err
	}
	f.mu.Lock()
	f.CreateCalls = append(f.CreateCalls, opts)
	fn := f.WorkspaceCreateFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, opts)
	}
	return Workspace{ID: "fake-ws", Ref: "workspace:fake"}, nil
}

func (f *FakeClient) WorkspaceClose(ctx context.Context, ws Workspace) error {
	if err := f.maybehang(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	f.CloseCalls = append(f.CloseCalls, ws)
	fn := f.WorkspaceCloseFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, ws)
	}
	return nil
}

func (f *FakeClient) WorkspaceSelect(ctx context.Context, ws Workspace) error {
	if err := f.maybehang(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	f.SelectCalls = append(f.SelectCalls, ws)
	fn := f.WorkspaceSelectFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, ws)
	}
	return nil
}

func (f *FakeClient) SurfaceSplit(ctx context.Context, opts SplitOpts) (Surface, error) {
	if err := f.maybehang(ctx); err != nil {
		return Surface{}, err
	}
	f.mu.Lock()
	f.SplitCalls = append(f.SplitCalls, opts)
	fn := f.SurfaceSplitFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, opts)
	}
	return Surface{SurfaceID: "fake-surface", SurfaceRef: "surface:fake"}, nil
}

func (f *FakeClient) SurfaceList(ctx context.Context, ws Workspace) ([]SurfaceInfo, error) {
	if err := f.maybehang(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	fn := f.SurfaceListFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, ws)
	}
	return nil, nil
}
