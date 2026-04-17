package main

import (
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/cli"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/statusline"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
)

// --- TP-001: modeString ---

func TestModeString_AllModes(t *testing.T) {
	cases := []struct {
		mode ui.Mode
		want string
	}{
		{ui.ModeNormal, "normal"},
		{ui.ModeError, "error"},
		{ui.ModeQuitConfirm, "quitconfirm"},
		{ui.ModeNextConfirm, "nextconfirm"},
		{ui.ModeDone, "done"},
		{ui.ModeSelect, "select"},
		{ui.ModeQuitting, "quitting"},
		{ui.ModeHelp, "help"},
		{ui.Mode(99), "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			got := modeString(tc.mode)
			if got != tc.want {
				t.Errorf("modeString(%v) = %q, want %q", tc.mode, got, tc.want)
			}
		})
	}
}

// --- TP-002: newStatusLineSender ---

func TestNewStatusLineSender_PayloadDiscarded(t *testing.T) {
	var received []tea.Msg
	send := func(msg tea.Msg) {
		received = append(received, msg)
	}
	sender := newStatusLineSender(send)

	sender(nil)
	sender("some string")
	sender(42)
	sender(struct{ X int }{X: 99})

	if len(received) != 4 {
		t.Fatalf("expected 4 sends, got %d", len(received))
	}
	for i, msg := range received {
		if _, ok := msg.(ui.StatusLineUpdatedMsg); !ok {
			t.Errorf("send %d: expected StatusLineUpdatedMsg, got %T", i, msg)
		}
	}
}

func TestNewStatusLineSender_AlwaysStatusLineUpdatedMsg(t *testing.T) {
	var count int
	send := func(_ tea.Msg) { count++ }
	sender := newStatusLineSender(send)

	sender(nil)

	if count != 1 {
		t.Errorf("expected exactly 1 call to send, got %d", count)
	}
}

// --- TP-003: newModeGetter freshness ---

func TestNewModeGetter_ReflectsCurrentMode(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	getter := newModeGetter(kh)

	first := getter()
	if first != "normal" {
		t.Errorf("initial mode = %q, want %q", first, "normal")
	}

	kh.SetMode(ui.ModeSelect)
	second := getter()
	if second == first {
		t.Errorf("mode getter returned same value after SetMode; both = %q", first)
	}
	if second != "select" {
		t.Errorf("after SetMode(ModeSelect) got %q, want %q", second, "select")
	}
}

func TestNewModeGetter_DoesNotSnapshotAtCreation(t *testing.T) {
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)
	kh.SetMode(ui.ModeError)

	getter := newModeGetter(kh)
	kh.SetMode(ui.ModeDone)

	got := getter()
	if got != "done" {
		t.Errorf("getter returned %q after post-creation SetMode; want %q", got, "done")
	}
}

// --- TP-004: buildStatusLineConfig ---

func TestBuildStatusLineConfig_Nil(t *testing.T) {
	got := buildStatusLineConfig(nil)
	if got != nil {
		t.Errorf("expected nil for nil input, got %+v", got)
	}
}

func TestBuildStatusLineConfig_Populated(t *testing.T) {
	interval := 5
	slc := &steps.StatusLineConfig{
		Command:                "./x.sh",
		RefreshIntervalSeconds: &interval,
	}
	got := buildStatusLineConfig(slc)
	if got == nil {
		t.Fatal("expected non-nil config for populated input")
	}
	if got.Command != "./x.sh" {
		t.Errorf("Command = %q, want %q", got.Command, "./x.sh")
	}
	if got.RefreshIntervalSeconds != slc.RefreshIntervalSeconds {
		t.Error("RefreshIntervalSeconds pointer was not preserved (pointer identity lost)")
	}
}

func TestBuildStatusLineConfig_NilInterval(t *testing.T) {
	slc := &steps.StatusLineConfig{Command: "./y.sh", RefreshIntervalSeconds: nil}
	got := buildStatusLineConfig(slc)
	if got == nil {
		t.Fatal("expected non-nil config")
	}
	if got.RefreshIntervalSeconds != nil {
		t.Errorf("expected nil interval, got %v", got.RefreshIntervalSeconds)
	}
}

// --- TP-005: runWithShutdown ordering ---

type fakeTeaProgram struct {
	onRun func()
}

func (f *fakeTeaProgram) Run() (tea.Model, error) {
	if f.onRun != nil {
		f.onRun()
	}
	return nil, nil
}

type fakeShutdowner struct {
	onShutdown func()
}

func (f *fakeShutdowner) Shutdown() {
	if f.onShutdown != nil {
		f.onShutdown()
	}
}

func TestRunWithShutdown_OrderIsRunThenShutdown(t *testing.T) {
	var mu sync.Mutex
	var events []string
	record := func(e string) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	workflowDone := make(chan struct{})
	close(workflowDone)

	fakeProg := &fakeTeaProgram{onRun: func() { record("Run") }}
	fakeRunner := &fakeShutdowner{onShutdown: func() { record("Shutdown") }}

	_ = runWithShutdown(fakeProg, fakeRunner, workflowDone)

	mu.Lock()
	snap := make([]string, len(events))
	copy(snap, events)
	mu.Unlock()

	if len(snap) != 2 {
		t.Fatalf("expected 2 events [Run, Shutdown], got %v", snap)
	}
	if snap[0] != "Run" {
		t.Errorf("first event = %q, want %q", snap[0], "Run")
	}
	if snap[1] != "Shutdown" {
		t.Errorf("second event = %q, want %q", snap[1], "Shutdown")
	}
}

func TestRunWithShutdown_WaitsForWorkflowDone(t *testing.T) {
	workflowDone := make(chan struct{})
	released := make(chan struct{})

	fakeProg := &fakeTeaProgram{}
	fakeRunner := &fakeShutdowner{}

	go func() {
		close(workflowDone)
		close(released)
	}()

	_ = runWithShutdown(fakeProg, fakeRunner, workflowDone)

	select {
	case <-released:
	default:
		t.Error("runWithShutdown returned before workflowDone was closed")
	}
}

func TestRunWithShutdown_ShutdownBeforeWorkflowDone(t *testing.T) {
	var mu sync.Mutex
	var events []string
	record := func(e string) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	workflowDone := make(chan struct{})
	fakeProg := &fakeTeaProgram{onRun: func() { record("Run") }}
	fakeRunner := &fakeShutdowner{onShutdown: func() {
		record("Shutdown")
		go func() {
			record("WorkflowDone")
			close(workflowDone)
		}()
	}}

	_ = runWithShutdown(fakeProg, fakeRunner, workflowDone)

	mu.Lock()
	snap := make([]string, len(events))
	copy(snap, events)
	mu.Unlock()

	shutdownIdx := -1
	workflowIdx := -1
	for i, e := range snap {
		switch e {
		case "Shutdown":
			shutdownIdx = i
		case "WorkflowDone":
			workflowIdx = i
		}
	}
	if shutdownIdx < 0 {
		t.Fatal("Shutdown event not recorded")
	}
	if workflowIdx < 0 {
		t.Fatal("WorkflowDone event not recorded")
	}
	if shutdownIdx >= workflowIdx {
		t.Errorf("Shutdown (%d) must precede WorkflowDone (%d); events = %v", shutdownIdx, workflowIdx, snap)
	}
}

// --- TP-006: buildRunConfig Runner identity ---

func TestBuildRunConfig_RunnerFieldIdentity(t *testing.T) {
	cfg := &cli.Config{
		WorkflowDir: "/wf",
		Iterations:  3,
	}
	stepFile := steps.StepFile{}
	statusRunner := statusline.NewNoOp()

	got := buildRunConfig(cfg, stepFile, statusRunner, 80, "stamp-123")

	if got.Runner != statusRunner {
		t.Error("RunConfig.Runner is not identity-equal to the statusRunner passed to buildRunConfig")
	}
}

func TestBuildRunConfig_FieldMapping(t *testing.T) {
	cfg := &cli.Config{
		WorkflowDir: "/workflow",
		Iterations:  5,
	}
	stepFile := steps.StepFile{
		Env:        []string{"FOO=bar"},
		Initialize: []steps.Step{{Name: "init"}},
		Iteration:  []steps.Step{{Name: "iter"}},
		Finalize:   []steps.Step{{Name: "fin"}},
	}
	runner := statusline.NewNoOp()

	got := buildRunConfig(cfg, stepFile, runner, 120, "run-stamp")

	if got.WorkflowDir != "/workflow" {
		t.Errorf("WorkflowDir = %q, want %q", got.WorkflowDir, "/workflow")
	}
	if got.Iterations != 5 {
		t.Errorf("Iterations = %d, want 5", got.Iterations)
	}
	if len(got.Env) != 1 || got.Env[0] != "FOO=bar" {
		t.Errorf("Env = %v, want [FOO=bar]", got.Env)
	}
	if len(got.InitializeSteps) != 1 || got.InitializeSteps[0].Name != "init" {
		t.Errorf("InitializeSteps mismatch: %v", got.InitializeSteps)
	}
	if len(got.Steps) != 1 || got.Steps[0].Name != "iter" {
		t.Errorf("Steps mismatch: %v", got.Steps)
	}
	if len(got.FinalizeSteps) != 1 || got.FinalizeSteps[0].Name != "fin" {
		t.Errorf("FinalizeSteps mismatch: %v", got.FinalizeSteps)
	}
	if got.LogWidth != 120 {
		t.Errorf("LogWidth = %d, want 120", got.LogWidth)
	}
	if got.RunStamp != "run-stamp" {
		t.Errorf("RunStamp = %q, want %q", got.RunStamp, "run-stamp")
	}
}
