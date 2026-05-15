package cmuxctl_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
)

// fastDismissal returns a DismissalConfig with a 1ms poll interval so existing
// RunPhase1 tests complete quickly. WorkspaceList returns nil by default, so
// the observer fires dismissal on the first poll (workspace not found).
func fastDismissal() cmuxctl.DismissalConfig {
	return cmuxctl.DismissalConfig{PollInterval: time.Millisecond}
}

// ---- D11 sanitization tests --------------------------------------------------

var sanitizeTests = []struct {
	name  string
	input string
	want  string
}{
	{name: "plain alphanumeric", input: "myrepo", want: "myrepo"},
	{name: "spaces replaced", input: "my repo", want: "my-repo"},
	{name: "slashes replaced", input: "org/repo", want: "org-repo"},
	{name: "dots kept", input: "my.repo", want: "my.repo"},
	{name: "underscores kept", input: "my_repo", want: "my_repo"},
	{name: "hyphens kept", input: "my-repo", want: "my-repo"},
	{name: "control chars replaced", input: "my\x00repo", want: "my-repo"},
	{name: "unicode replaced", input: "répo", want: "r-po"},
	{name: "leading hyphen trimmed", input: "-repo", want: "repo"},
	{name: "trailing hyphen trimmed", input: "repo-", want: "repo"},
	{name: "leading and trailing hyphens trimmed", input: "-repo-", want: "repo"},
	{name: "hyphen runs collapsed", input: "my--repo", want: "my-repo"},
	{name: "all hyphens collapse to empty fallback", input: "---", want: "repo"},
	{name: "empty string fallback", input: "", want: "repo"},
	{name: "root path basename fallback", input: "/", want: "repo"},
	{name: "mixed invalid chars", input: "!@#$%", want: "repo"},
}

func TestSanitizeBasename(t *testing.T) {
	for _, tc := range sanitizeTests {
		t.Run(tc.name, func(t *testing.T) {
			got := cmuxctl.SanitizeBasename(tc.input)
			if got != tc.want {
				t.Errorf("SanitizeBasename(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---- workspace-name composition tests ----------------------------------------

func TestRunPhase1_WorkspaceNameShape(t *testing.T) {
	var createNames []string
	fake := &cmuxctl.FakeClient{
		WorkspaceCreateFunc: func(_ context.Context, name string) error {
			createNames = append(createNames, name)
			return nil
		},
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/myrepo", &buf, fastDismissal())
	if err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}

	if len(createNames) == 0 {
		t.Fatal("WorkspaceCreate was never called")
	}
	name := createNames[0]
	if !strings.HasPrefix(name, "pr9k-myrepo-") {
		t.Errorf("workspace name %q does not start with pr9k-myrepo-", name)
	}
}

// ---- collision retry tests ---------------------------------------------------

func TestRunPhase1_CollisionRetrySucceeds(t *testing.T) {
	callCount := 0
	fake := &cmuxctl.FakeClient{
		WorkspaceCreateFunc: func(_ context.Context, _ string) error {
			callCount++
			if callCount == 1 {
				return cmuxctl.ErrWorkspaceExists
			}
			return nil
		},
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/myrepo", &buf, fastDismissal())
	if err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}
	if callCount != 2 {
		t.Errorf("WorkspaceCreate called %d times, want 2", callCount)
	}
}

func TestRunPhase1_CollisionRetryFailsOnSecondCollision(t *testing.T) {
	fake := &cmuxctl.FakeClient{
		WorkspaceCreateFunc: func(_ context.Context, _ string) error {
			return cmuxctl.ErrWorkspaceExists
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/myrepo", &buf, fastDismissal())
	if err == nil {
		t.Fatal("expected error on second collision, got nil")
	}
}

func TestRunPhase1_CollisionRetryDifferentTimestamps(t *testing.T) {
	var names []string
	callCount := 0
	fake := &cmuxctl.FakeClient{
		WorkspaceCreateFunc: func(_ context.Context, name string) error {
			names = append(names, name)
			callCount++
			if callCount == 1 {
				return cmuxctl.ErrWorkspaceExists
			}
			return nil
		},
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
	}

	var buf bytes.Buffer
	if err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/myrepo", &buf, fastDismissal()); err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 WorkspaceCreate calls, got %d", len(names))
	}
	if names[0] == names[1] {
		t.Errorf("collision retry used same workspace name: %q", names[0])
	}
}

// ---- D-23: pre-sanitized basename never in operator output -------------------

func TestRunPhase1_PreSanitizedBasenameNotInOutput(t *testing.T) {
	// projectDir with a basename that differs after sanitization
	projectDir := "/tmp/my repo!unsafe"

	fake := &cmuxctl.FakeClient{
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
	}

	var buf bytes.Buffer
	_ = cmuxctl.RunPhase1(context.Background(), fake, projectDir, &buf, fastDismissal())

	output := buf.String()
	// "my repo!unsafe" is the pre-sanitized form and must not appear
	if strings.Contains(output, "my repo!unsafe") {
		t.Errorf("pre-sanitized basename %q appeared in operator output: %q", "my repo!unsafe", output)
	}
}

// ---- spawn order tests -------------------------------------------------------

// callEvent records the type and pane/opts for sequencing assertions.
type callEvent struct {
	kind   string // "spawn", "hide", "split"
	paneID string
}

func TestRunPhase1_SpawnOrder(t *testing.T) {
	var events []callEvent
	splitCount := 0

	paneIDs := []string{"orch-pane", "header-pane", "log-pane", "footer-pane"}

	fake := &cmuxctl.FakeClient{
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			id := paneIDs[splitCount]
			splitCount++
			events = append(events, callEvent{kind: "split", paneID: id})
			return id, nil
		},
		SurfaceSpawnFunc: func(_ context.Context, paneID string, _ []string) error {
			events = append(events, callEvent{kind: "spawn", paneID: paneID})
			return nil
		},
		SurfaceHideFunc: func(_ context.Context, paneID string) error {
			events = append(events, callEvent{kind: "hide", paneID: paneID})
			return nil
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/repo", &buf, fastDismissal())
	if err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}

	// Find first spawn (orchestrator), then hide, then remaining panes
	firstSpawnIdx := -1
	firstHideIdx := -1
	for i, e := range events {
		if e.kind == "spawn" && firstSpawnIdx == -1 {
			firstSpawnIdx = i
		}
		if e.kind == "hide" && firstHideIdx == -1 {
			firstHideIdx = i
		}
	}

	if firstSpawnIdx == -1 {
		t.Fatal("no SurfaceSpawn calls recorded")
	}
	if firstHideIdx == -1 {
		t.Fatal("no SurfaceHide calls recorded")
	}

	// Orchestrator spawn must come before hide
	if firstSpawnIdx >= firstHideIdx {
		t.Errorf("orchestrator spawn (idx %d) did not precede hide (idx %d); events: %+v", firstSpawnIdx, firstHideIdx, events)
	}

	// All three visible pane spawns must come after the hide
	visibleSpawnCount := 0
	for i, e := range events {
		if e.kind == "spawn" && i > firstHideIdx {
			visibleSpawnCount++
		}
	}
	if visibleSpawnCount < 3 {
		t.Errorf("expected >= 3 visible pane spawns after hide, got %d; events: %+v", visibleSpawnCount, events)
	}
}

// ---- shell one-liner tests ---------------------------------------------------

func TestRunPhase1_OrchestratorSpawnArgv(t *testing.T) {
	var spawns []cmuxctl.SpawnCall
	fake := &cmuxctl.FakeClient{
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "some-pane", nil
		},
		SurfaceSpawnFunc: func(_ context.Context, paneID string, argv []string) error {
			spawns = append(spawns, cmuxctl.SpawnCall{PaneID: paneID, Argv: argv})
			return nil
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/repo", &buf, fastDismissal())
	if err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}

	if len(spawns) == 0 {
		t.Fatal("no SurfaceSpawn calls recorded")
	}

	// Orchestrator is the first spawn
	orch := spawns[0]
	if len(orch.Argv) < 3 || orch.Argv[0] != "sh" || orch.Argv[1] != "-c" {
		t.Errorf("orchestrator argv = %v, want [sh -c ...]", orch.Argv)
	}
	if !strings.Contains(orch.Argv[2], "tail -f /dev/null") {
		t.Errorf("orchestrator one-liner %q does not contain 'tail -f /dev/null'", orch.Argv[2])
	}
	// Orchestrator must NOT contain "sleep infinity"
	for _, arg := range orch.Argv {
		if strings.Contains(arg, "sleep infinity") {
			t.Errorf("orchestrator argv contains 'sleep infinity' (not POSIX-portable): %v", orch.Argv)
		}
	}
}

func TestRunPhase1_VisiblePaneSpawnArgvContainsTailFDev(t *testing.T) {
	var spawns []cmuxctl.SpawnCall
	fake := &cmuxctl.FakeClient{
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "some-pane", nil
		},
		SurfaceSpawnFunc: func(_ context.Context, paneID string, argv []string) error {
			spawns = append(spawns, cmuxctl.SpawnCall{PaneID: paneID, Argv: argv})
			return nil
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/repo", &buf, fastDismissal())
	if err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}

	// Skip first spawn (orchestrator); remaining visible pane spawns
	for i, sp := range spawns[1:] {
		if len(sp.Argv) < 3 || sp.Argv[0] != "sh" || sp.Argv[1] != "-c" {
			t.Errorf("visible pane %d argv = %v, want [sh -c ...]", i, sp.Argv)
		}
		if !strings.Contains(sp.Argv[2], "tail -f /dev/null") {
			t.Errorf("visible pane %d one-liner %q does not contain 'tail -f /dev/null'", i, sp.Argv[2])
		}
	}
}

// ---- workspace-name confirmation printing -----------------------------------

func TestRunPhase1_ConfirmationPrintedAfterCreate(t *testing.T) {
	createDone := false
	var printedBefore string

	fake := &cmuxctl.FakeClient{
		WorkspaceCreateFunc: func(_ context.Context, name string) error {
			createDone = true
			return nil
		},
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			if !createDone {
				printedBefore = "split before create"
			}
			return "some-pane", nil
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/myrepo", &buf, fastDismissal())
	if err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}

	if printedBefore != "" {
		t.Error("split happened before WorkspaceCreate")
	}

	output := buf.String()
	if !strings.Contains(output, "pr9k-myrepo-") {
		t.Errorf("workspace name confirmation not printed; output: %q", output)
	}
}

// ---- WorkspaceCurrent captured per spec D10 ---------------------------------

func TestRunPhase1_CapturesPriorWorkspace(t *testing.T) {
	currentCalled := false
	fake := &cmuxctl.FakeClient{
		WorkspaceCurrentFunc: func(_ context.Context) (string, error) {
			currentCalled = true
			return "prior-workspace", nil
		},
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "some-pane", nil
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/repo", &buf, fastDismissal())
	if err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}
	if !currentCalled {
		t.Error("WorkspaceCurrent was not called")
	}
}

func TestRunPhase1_EmptyCurrentWorkspaceIsOK(t *testing.T) {
	fake := &cmuxctl.FakeClient{
		WorkspaceCurrentFunc: func(_ context.Context) (string, error) {
			return "", nil // no prior workspace
		},
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "some-pane", nil
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/repo", &buf, fastDismissal())
	if err != nil {
		t.Fatalf("RunPhase1 with empty current workspace: %v", err)
	}
}

// ---- WorkspaceCurrent error degrades gracefully (spec D10) — TP-221-002 -----

// TestRunPhase1_WorkspaceCurrentErrorDegraces verifies that a WorkspaceCurrent
// RPC error is swallowed per spec D10 ("record no prior workspace on error")
// and RunPhase1 continues to create the workspace and spawn panes.
func TestRunPhase1_WorkspaceCurrentErrorDegraces(t *testing.T) {
	fake := &cmuxctl.FakeClient{
		WorkspaceCurrentFunc: func(_ context.Context) (string, error) {
			return "", errors.New("rpc down")
		},
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/myrepo", &buf, fastDismissal())
	if err != nil {
		t.Fatalf("RunPhase1 must return nil when WorkspaceCurrent errors; got: %v", err)
	}
	if len(fake.CreateCalls) == 0 {
		t.Error("WorkspaceCreate was not called after WorkspaceCurrent error")
	}
	if len(fake.SpawnCalls) == 0 {
		t.Error("SurfaceSpawn was not called after WorkspaceCurrent error")
	}
}

// ---- TP-222-009: RunPhase1 returns fatal error on N=3 consecutive timeouts ---

// TestRunPhase1_FatalDismissal_ReturnsError verifies that when the dismissal
// observer escalates to Fatal=true (D7: three consecutive poll timeouts),
// RunPhase1 returns a non-nil error whose message contains "dismissal poll fatal"
// and the sanitized workspace name. A non-nil return here is the contract that
// lets #224 teardown choose a non-zero exit code.
func TestRunPhase1_FatalDismissal_ReturnsError(t *testing.T) {
	t.Parallel()

	fake := &cmuxctl.FakeClient{
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "some-pane", nil
		},
		WorkspaceListFunc: func(ctx context.Context) ([]string, error) {
			// Block until context is cancelled — simulates a hung RPC so every
			// call times out, driving the N=3 consecutive-timeout escalation.
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	cfg := cmuxctl.DismissalConfig{
		PollInterval: time.Millisecond,
		PollTimeout:  5 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(ctx, fake, "/tmp/myrepo", &buf, cfg)
	if err == nil {
		t.Fatal("RunPhase1 returned nil; expected a non-nil fatal-dismissal error")
	}
	if !strings.Contains(err.Error(), "dismissal poll fatal") {
		t.Errorf("error message %q does not contain %q", err.Error(), "dismissal poll fatal")
	}
	if !strings.Contains(err.Error(), "myrepo") {
		t.Errorf("error message %q does not contain sanitized workspace name %q", err.Error(), "myrepo")
	}
}

// ---- ErrWorkspaceExists sentinel --------------------------------------------

func TestErrWorkspaceExists_IsDistinct(t *testing.T) {
	if cmuxctl.ErrWorkspaceExists == nil {
		t.Fatal("ErrWorkspaceExists is nil")
	}
	other := errors.New("some other error")
	if errors.Is(other, cmuxctl.ErrWorkspaceExists) {
		t.Error("unrelated error matches ErrWorkspaceExists")
	}
}

// ---- TP-224: Teardown sequence tests ----------------------------------------

// TP-224-001: Successful WorkspaceClose → exit code 0; WorkspaceSelect called
// with the prior workspace name.
func TestTeardown_SuccessfulClose_ExitZero(t *testing.T) {
	t.Parallel()

	fake := &cmuxctl.FakeClient{
		WorkspaceCurrentFunc: func(_ context.Context) (string, error) {
			return "prior-ws", nil
		},
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/repo", &buf, fastDismissal())
	if err != nil {
		t.Fatalf("RunPhase1 returned non-nil: %v", err)
	}
	if len(fake.CloseCalls) != 1 {
		t.Errorf("WorkspaceClose called %d times, want 1", len(fake.CloseCalls))
	}
	if len(fake.SelectCalls) != 1 || fake.SelectCalls[0] != "prior-ws" {
		t.Errorf("WorkspaceSelect calls = %v, want [prior-ws]", fake.SelectCalls)
	}
}

// TP-224-002: Failed WorkspaceClose → orphan diagnostic on stderr;
// focus-restore still attempted; exit code non-zero.
func TestTeardown_FailedClose_OrphanDiagnosticAndNonZeroExit(t *testing.T) {
	t.Parallel()

	var stderrBuf bytes.Buffer
	fake := &cmuxctl.FakeClient{
		WorkspaceCurrentFunc: func(_ context.Context) (string, error) {
			return "prior-ws", nil
		},
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
		WorkspaceCloseFunc: func(_ context.Context, _ string) error {
			return errors.New("rpc error: close failed")
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/myrepo", &buf,
		cmuxctl.DismissalConfig{
			PollInterval: time.Millisecond,
			Stderr:       &stderrBuf,
		})
	if err == nil {
		t.Fatal("RunPhase1 returned nil; expected non-nil on WorkspaceClose failure")
	}

	diag := stderrBuf.String()
	if !strings.Contains(diag, "orphan workspace") {
		t.Errorf("orphan diagnostic not found in stderr: %q", diag)
	}
	if !strings.Contains(diag, "myrepo") {
		t.Errorf("workspace name not found in orphan diagnostic: %q", diag)
	}
	// focus-restore still attempted even on close failure
	if len(fake.SelectCalls) != 1 {
		t.Errorf("WorkspaceSelect called %d times, want 1", len(fake.SelectCalls))
	}
}

// TP-224-003: Empty prior workspace → WorkspaceSelect never called.
func TestTeardown_EmptyPriorWorkspace_NoSelectCall(t *testing.T) {
	t.Parallel()

	fake := &cmuxctl.FakeClient{
		WorkspaceCurrentFunc: func(_ context.Context) (string, error) {
			return "", nil // no prior workspace
		},
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/repo", &buf, fastDismissal())
	if err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}
	if len(fake.SelectCalls) != 0 {
		t.Errorf("WorkspaceSelect called %d times, want 0; calls: %v", len(fake.SelectCalls), fake.SelectCalls)
	}
}

// TP-224-004: Stale prior workspace (WorkspaceSelect returns error) →
// error is silently ignored; no extra output on the run writer.
func TestTeardown_StalePriorWorkspace_SelectErrorIgnored(t *testing.T) {
	t.Parallel()

	var stderrBuf bytes.Buffer
	fake := &cmuxctl.FakeClient{
		WorkspaceCurrentFunc: func(_ context.Context) (string, error) {
			return "stale-ws", nil
		},
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
		WorkspaceSelectFunc: func(_ context.Context, _ string) error {
			return errors.New("workspace not found")
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/repo", &buf,
		cmuxctl.DismissalConfig{
			PollInterval: time.Millisecond,
			Stderr:       &stderrBuf,
		})
	if err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}
	// stderr must be silent (no extra output for select failure)
	if diag := stderrBuf.String(); diag != "" {
		t.Errorf("unexpected stderr output on stale prior workspace: %q", diag)
	}
}

// TP-224-005: sync.Once — if teardown is triggered via two concurrent paths,
// WorkspaceClose is called exactly once.
func TestTeardown_SyncOnce_CloseCalledExactlyOnce(t *testing.T) {
	t.Parallel()

	fake := &cmuxctl.FakeClient{
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
		// WorkspaceList returns empty immediately → dismissal fires at first poll.
	}

	// Use a pre-cancelled context so both the ctx.Done() path and the dismissal
	// path (WorkspaceList returns nil → dismissal) may fire concurrently.
	ctx, cancel := context.WithCancel(context.Background())

	var buf bytes.Buffer
	// Start RunPhase1 in a goroutine, cancel context just after entry.
	done := make(chan error, 1)
	go func() {
		done <- cmuxctl.RunPhase1(ctx, fake, "/tmp/repo", &buf, cmuxctl.DismissalConfig{
			PollInterval: time.Millisecond,
		})
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunPhase1 did not return within 5s")
	}

	if len(fake.CloseCalls) > 1 {
		t.Errorf("WorkspaceClose called %d times (want ≤1); sync.Once should prevent double execution", len(fake.CloseCalls))
	}
}

// TP-224-006: Dismissal-observer goroutine joins cleanly before RunPhase1 returns.
// If Wait() is not called, the race detector would catch the missing join.
func TestTeardown_ObserverJoinsBeforeReturn(t *testing.T) {
	t.Parallel()

	fake := &cmuxctl.FakeClient{
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var buf bytes.Buffer
	// Normal dismissal path: WorkspaceList returns nil → dismissal fires → RunPhase1 returns.
	err := cmuxctl.RunPhase1(ctx, fake, "/tmp/repo", &buf, fastDismissal())
	if err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}
	// If we reach here without a race-detector warning, the goroutine joined.
}

// TP-224-007: Exit-code matrix — context cancellation (signal-driven) → non-zero.
func TestTeardown_ContextCancel_NonZeroExit(t *testing.T) {
	t.Parallel()

	// Block WorkspaceList forever so dismissal never fires; we cancel ctx instead.
	ready := make(chan struct{})
	fake := &cmuxctl.FakeClient{
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
		WorkspaceListFunc: func(ctx context.Context) ([]string, error) {
			select {
			case <-ready:
				return []string{"pr9k-repo-ts"}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
		SurfaceListFunc: func(_ context.Context, _ string) ([]cmuxctl.PaneInfo, error) {
			return nil, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- cmuxctl.RunPhase1(ctx, fake, "/tmp/repo", &buf, cmuxctl.DismissalConfig{
			PollInterval: time.Millisecond,
		})
	}()

	cancel()
	close(ready)

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected non-nil error on context cancellation (signal-driven), got nil")
		}
		// Gap-2: WorkspaceClose must have run on a fresh context even though the
		// parent was cancelled — this pins the context.Background() closeCtx decision.
		if len(fake.CloseCalls) != 1 {
			t.Errorf("WorkspaceClose called %d times after ctx cancel, want 1 (fresh closeCtx must survive parent cancellation)", len(fake.CloseCalls))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunPhase1 did not return within 5s after context cancel")
	}
}

// TP-224-009: Fatal dismissal (N=3 consecutive poll timeouts) still runs
// teardown — WorkspaceClose is called exactly once on the fatal path.
// This pins the deferred runTeardown contract: the fatal branch must not
// return before the deferred fires.
func TestTeardown_FatalDismissal_TeardownRuns(t *testing.T) {
	t.Parallel()

	fake := &cmuxctl.FakeClient{
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
		WorkspaceListFunc: func(ctx context.Context) ([]string, error) {
			// Block until context is cancelled — simulates a hung RPC so every
			// call times out, driving the N=3 consecutive-timeout escalation.
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	cfg := cmuxctl.DismissalConfig{
		PollInterval: time.Millisecond,
		PollTimeout:  5 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(ctx, fake, "/tmp/myrepo", &buf, cfg)
	if err == nil {
		t.Fatal("RunPhase1 returned nil; expected non-nil fatal-dismissal error")
	}
	// Teardown must have run on the fatal path.
	if len(fake.CloseCalls) != 1 {
		t.Errorf("WorkspaceClose called %d times on fatal path, want 1 (teardown must run via defer)", len(fake.CloseCalls))
	}
}

// TP-224-008: Partial-setup failure (SurfaceSpawn fails after WorkspaceCreate) →
// teardown still runs (WorkspaceClose called) and RunPhase1 returns non-nil.
func TestTeardown_PartialSetup_TeardownRuns(t *testing.T) {
	t.Parallel()

	fake := &cmuxctl.FakeClient{
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
		SurfaceSpawnFunc: func(_ context.Context, _ string, _ []string) error {
			return errors.New("spawn failed")
		},
	}

	var buf bytes.Buffer
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/repo", &buf, fastDismissal())
	if err == nil {
		t.Fatal("expected non-nil error on spawn failure")
	}
	if len(fake.CloseCalls) != 1 {
		t.Errorf("WorkspaceClose called %d times, want 1 (teardown should run on partial-setup failure)", len(fake.CloseCalls))
	}
}
