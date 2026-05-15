package cmuxctl_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
)

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
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/myrepo", &buf)
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
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/myrepo", &buf)
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
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/myrepo", &buf)
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
	if err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/myrepo", &buf); err != nil {
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
	_ = cmuxctl.RunPhase1(context.Background(), fake, projectDir, &buf)

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
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/repo", &buf)
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
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/repo", &buf)
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
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/repo", &buf)
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
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/myrepo", &buf)
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
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/repo", &buf)
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
	err := cmuxctl.RunPhase1(context.Background(), fake, "/tmp/repo", &buf)
	if err != nil {
		t.Fatalf("RunPhase1 with empty current workspace: %v", err)
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
