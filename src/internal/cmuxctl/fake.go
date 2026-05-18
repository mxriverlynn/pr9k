package cmuxctl

import (
	"context"
	"sync"
)

// SpawnCall records the arguments passed to SurfaceSpawn.
type SpawnCall struct {
	PaneID string
	Argv   []string
	Env    map[string]string
}

// FakeClient is a test double for CmuxClient. Every method is scriptable via
// its corresponding Func field. All mutable state is protected by mu.
//
// The HangNext and HangRelease channels let tests simulate a hanging RPC call:
// send a value to HangNext before the call, and the call will block until a
// value is sent to HangRelease (or the context is cancelled).
type FakeClient struct {
	mu sync.Mutex

	// Scripted responses. If nil, the method returns its zero value and nil error.
	SystemIdentifyFunc   func(ctx context.Context) (Identity, error)
	WorkspaceCurrentFunc func(ctx context.Context) (string, error)
	WorkspaceListFunc    func(ctx context.Context) ([]string, error)
	WorkspaceCreateFunc  func(ctx context.Context, name string) error
	WorkspaceCloseFunc   func(ctx context.Context, name string) error
	WorkspaceSelectFunc  func(ctx context.Context, name string) error
	SurfaceSplitFunc     func(ctx context.Context, opts SplitOpts) (string, error)
	SurfaceSpawnFunc     func(ctx context.Context, paneID string, argv []string, env map[string]string) error
	SurfaceHideFunc      func(ctx context.Context, paneID string) error
	SurfaceListFunc      func(ctx context.Context, workspaceName string) ([]PaneInfo, error)

	// Recorders — appended under mu; read after all goroutines have joined.
	CreateCalls []string
	CloseCalls  []string
	SelectCalls []string
	SpawnCalls  []SpawnCall
	HideCalls   []string
	SplitCalls  []SplitOpts

	// Hang inject seams. Both channels must be initialized by the caller before use.
	HangNext    chan struct{} // send to this to make the next call block
	HangRelease chan struct{} // send to this to release the blocked call
}

// Compile-time interface assertion — must stay in the production file.
var _ CmuxClient = (*FakeClient)(nil)

// SetHangChannels sets HangNext and HangRelease under f.mu so callers can
// update them safely while methods may be running concurrently on other
// goroutines.
func (f *FakeClient) SetHangChannels(next, release chan struct{}) {
	f.mu.Lock()
	f.HangNext = next
	f.HangRelease = release
	f.mu.Unlock()
}

// maybehang blocks the current call if HangNext has a pending value.
// It waits on HangRelease or ctx cancellation. Snapshots HangNext and
// HangRelease under f.mu (snapshot-then-unlock) so concurrent writes via
// SetHangChannels do not race with the channel reads below.
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
	return Identity{}, nil
}

func (f *FakeClient) WorkspaceCurrent(ctx context.Context) (string, error) {
	if err := f.maybehang(ctx); err != nil {
		return "", err
	}
	f.mu.Lock()
	fn := f.WorkspaceCurrentFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	return "", nil
}

func (f *FakeClient) WorkspaceList(ctx context.Context) ([]string, error) {
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

func (f *FakeClient) WorkspaceCreate(ctx context.Context, name string) error {
	if err := f.maybehang(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	f.CreateCalls = append(f.CreateCalls, name)
	fn := f.WorkspaceCreateFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, name)
	}
	return nil
}

func (f *FakeClient) WorkspaceClose(ctx context.Context, name string) error {
	if err := f.maybehang(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	f.CloseCalls = append(f.CloseCalls, name)
	fn := f.WorkspaceCloseFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, name)
	}
	return nil
}

func (f *FakeClient) WorkspaceSelect(ctx context.Context, name string) error {
	if err := f.maybehang(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	f.SelectCalls = append(f.SelectCalls, name)
	fn := f.WorkspaceSelectFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, name)
	}
	return nil
}

func (f *FakeClient) SurfaceSplit(ctx context.Context, opts SplitOpts) (string, error) {
	if err := f.maybehang(ctx); err != nil {
		return "", err
	}
	f.mu.Lock()
	f.SplitCalls = append(f.SplitCalls, opts)
	fn := f.SurfaceSplitFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, opts)
	}
	return "", nil
}

func (f *FakeClient) SurfaceSpawn(ctx context.Context, paneID string, argv []string, env map[string]string) error {
	if err := f.maybehang(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	f.SpawnCalls = append(f.SpawnCalls, SpawnCall{PaneID: paneID, Argv: argv, Env: env})
	fn := f.SurfaceSpawnFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, paneID, argv, env)
	}
	return nil
}

func (f *FakeClient) SurfaceHide(ctx context.Context, paneID string) error {
	if err := f.maybehang(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	f.HideCalls = append(f.HideCalls, paneID)
	fn := f.SurfaceHideFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, paneID)
	}
	return nil
}

func (f *FakeClient) SurfaceList(ctx context.Context, workspaceName string) ([]PaneInfo, error) {
	if err := f.maybehang(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	fn := f.SurfaceListFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, workspaceName)
	}
	return nil, nil
}
