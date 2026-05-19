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

// immediateDismiss returns a fast DismissalConfig. With the FakeClient's
// default WorkspaceList (nil), the observer treats the created workspace as
// absent and fires dismissal at once so RunPhase1 returns promptly.
func immediateDismiss() cmuxctl.DismissalConfig {
	return cmuxctl.DismissalConfig{PollInterval: time.Millisecond, PollTimeout: 20 * time.Millisecond}
}

func runPhase1(t *testing.T, fake *cmuxctl.FakeClient, projectDir string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := cmuxctl.RunPhase1(ctx, fake, projectDir, &out, immediateDismiss())
	return out.String(), err
}

func TestSanitizeBasename(t *testing.T) {
	cases := map[string]string{
		"my-repo":       "my-repo",
		"My_Repo.v2":    "My_Repo.v2",
		"weird/\\ name": "weird-name",
		"--leading--":   "leading",
		"":              "repo",
		"///":           "repo",
	}
	for in, want := range cases {
		if got := cmuxctl.SanitizeBasename(in); got != want {
			t.Errorf("SanitizeBasename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRunPhase1_WorkspaceCreateOpts(t *testing.T) {
	fake := &cmuxctl.FakeClient{}
	if _, err := runPhase1(t, fake, "/work/my-repo"); err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}
	if len(fake.CreateCalls) != 1 {
		t.Fatalf("want 1 WorkspaceCreate, got %d", len(fake.CreateCalls))
	}
	c := fake.CreateCalls[0]
	if !strings.HasPrefix(c.Title, "pr9k-my-repo-") {
		t.Errorf("title = %q, want pr9k-my-repo-<ts>", c.Title)
	}
	if c.WorkingDirectory != "/work/my-repo" {
		t.Errorf("working_directory = %q", c.WorkingDirectory)
	}
	if !strings.Contains(c.InitialCommand, "cmux-pane --role=log") {
		t.Errorf("first surface must run the log pane: %q", c.InitialCommand)
	}
	if !strings.Contains(c.InitialCommand, "PR9K_CMUX_SOCKET=") || !strings.Contains(c.InitialCommand, "PR9K_PROJECT_DIR=") {
		t.Errorf("pane env must be embedded in initial_command: %q", c.InitialCommand)
	}
}

func TestRunPhase1_SplitsHeaderAndFooter(t *testing.T) {
	fake := &cmuxctl.FakeClient{}
	if _, err := runPhase1(t, fake, "/work/repo"); err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}
	if len(fake.SplitCalls) != 2 {
		t.Fatalf("want 2 surface.split (header, footer), got %d", len(fake.SplitCalls))
	}
	roles := map[string]bool{}
	for _, s := range fake.SplitCalls {
		if s.Direction == "" {
			t.Error("surface.split must always send a direction (cmux requires it)")
		}
		if s.Workspace.Empty() {
			t.Error("split must target the created workspace handle")
		}
		switch {
		case strings.Contains(s.InitialCommand, "--role=header"):
			roles["header"] = true
		case strings.Contains(s.InitialCommand, "--role=footer"):
			roles["footer"] = true
		}
	}
	if !roles["header"] || !roles["footer"] {
		t.Errorf("missing header/footer split: %v", roles)
	}
}

func TestRunPhase1_ConfirmationPrinted_SanitizedOnly(t *testing.T) {
	fake := &cmuxctl.FakeClient{}
	out, err := runPhase1(t, fake, "/work/weird name!!")
	if err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}
	if !strings.Contains(out, "pr9k workspace: pr9k-weird-name-") {
		t.Errorf("confirmation not printed with sanitized label: %q", out)
	}
	if strings.Contains(out, "weird name!!") {
		t.Errorf("D-23 violation: raw basename leaked to output: %q", out)
	}
}

func TestRunPhase1_CapturesAndRestoresPriorWorkspace(t *testing.T) {
	prior := cmuxctl.Workspace{ID: "prior-uuid", Ref: "workspace:9"}
	fake := &cmuxctl.FakeClient{
		WorkspaceCurrentFunc: func(_ context.Context) (cmuxctl.Workspace, error) {
			return prior, nil
		},
	}
	if _, err := runPhase1(t, fake, "/work/repo"); err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}
	if len(fake.SelectCalls) != 1 || fake.SelectCalls[0].ID != "prior-uuid" {
		t.Errorf("prior workspace not restored on teardown: %+v", fake.SelectCalls)
	}
}

func TestRunPhase1_NoPriorWorkspace_NoSelect(t *testing.T) {
	fake := &cmuxctl.FakeClient{
		WorkspaceCurrentFunc: func(_ context.Context) (cmuxctl.Workspace, error) {
			return cmuxctl.Workspace{}, errors.New("no workspace selected")
		},
	}
	if _, err := runPhase1(t, fake, "/work/repo"); err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}
	if len(fake.SelectCalls) != 0 {
		t.Errorf("must not restore focus when there was no prior workspace: %+v", fake.SelectCalls)
	}
}

func TestRunPhase1_TeardownClosesWorkspaceExactlyOnce(t *testing.T) {
	fake := &cmuxctl.FakeClient{}
	if _, err := runPhase1(t, fake, "/work/repo"); err != nil {
		t.Fatalf("RunPhase1: %v", err)
	}
	if len(fake.CloseCalls) != 1 {
		t.Errorf("WorkspaceClose called %d times, want exactly 1", len(fake.CloseCalls))
	}
}

func TestRunPhase1_WorkspaceCreateError_Aborts(t *testing.T) {
	fake := &cmuxctl.FakeClient{
		WorkspaceCreateFunc: func(_ context.Context, _ cmuxctl.WorkspaceCreateOpts) (cmuxctl.Workspace, error) {
			return cmuxctl.Workspace{}, errors.New("boom")
		},
	}
	_, err := runPhase1(t, fake, "/work/repo")
	if err == nil || !strings.Contains(err.Error(), "workspace create") {
		t.Fatalf("want workspace-create error, got %v", err)
	}
	if len(fake.SplitCalls) != 0 {
		t.Error("must not split panes when workspace.create failed")
	}
}

func TestRunPhase1_SplitError_TeardownStillRuns(t *testing.T) {
	fake := &cmuxctl.FakeClient{
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (cmuxctl.Surface, error) {
			return cmuxctl.Surface{}, errors.New("split failed")
		},
	}
	_, err := runPhase1(t, fake, "/work/repo")
	if err == nil || !strings.Contains(err.Error(), "split") {
		t.Fatalf("want split error, got %v", err)
	}
	if len(fake.CloseCalls) != 1 {
		t.Errorf("teardown must close the workspace after a split failure: %+v", fake.CloseCalls)
	}
}

func TestRunPhase1_FatalDismissal_ReturnsError(t *testing.T) {
	fake := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(ctx context.Context) ([]cmuxctl.WorkspaceInfo, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := cmuxctl.RunPhase1(ctx, fake, "/work/repo", &out,
		cmuxctl.DismissalConfig{PollInterval: time.Millisecond, PollTimeout: 5 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "dismissal poll fatal") {
		t.Fatalf("want fatal dismissal error, got %v", err)
	}
}

func TestRunPhase1_FailedClose_OrphanDiagnostic(t *testing.T) {
	var stderr bytes.Buffer
	fake := &cmuxctl.FakeClient{
		WorkspaceCloseFunc: func(_ context.Context, _ cmuxctl.Workspace) error {
			return errors.New("close refused")
		},
	}
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := cmuxctl.RunPhase1(ctx, fake, "/work/repo", &out, cmuxctl.DismissalConfig{
		PollInterval: time.Millisecond, PollTimeout: 20 * time.Millisecond, Stderr: &stderr,
	})
	if err == nil || !strings.Contains(err.Error(), "workspace close failed") {
		t.Fatalf("want workspace-close-failed error, got %v", err)
	}
	if !strings.Contains(stderr.String(), "orphan workspace") {
		t.Errorf("missing orphan diagnostic on stderr: %q", stderr.String())
	}
}

func TestRunPhase1_ContextCancel_ReturnsCtxErr(t *testing.T) {
	fake := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(_ context.Context) ([]cmuxctl.WorkspaceInfo, error) {
			return wsInfo("fake-ws"), nil // never dismissed by observation
		},
		SurfaceListFunc: func(_ context.Context, _ cmuxctl.Workspace) ([]cmuxctl.SurfaceInfo, error) {
			return surfList(3), nil
		},
	}
	var out bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- cmuxctl.RunPhase1(ctx, fake, "/work/repo", &out,
			cmuxctl.DismissalConfig{PollInterval: 10 * time.Millisecond, PollTimeout: 50 * time.Millisecond})
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunPhase1 did not return after context cancel")
	}
	if len(fake.CloseCalls) != 1 {
		t.Errorf("teardown must still close the workspace on ctx cancel: %+v", fake.CloseCalls)
	}
}

func TestRunPhase1_DistinctWorkspaceLabelsAcrossRuns(t *testing.T) {
	seen := map[string]bool{}
	for range 3 {
		fake := &cmuxctl.FakeClient{}
		if _, err := runPhase1(t, fake, "/work/repo"); err != nil {
			t.Fatalf("RunPhase1: %v", err)
		}
		title := fake.CreateCalls[0].Title
		if seen[title] {
			t.Errorf("duplicate workspace label across runs: %q", title)
		}
		seen[title] = true
		time.Sleep(time.Millisecond)
	}
}
