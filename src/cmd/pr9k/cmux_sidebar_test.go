package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

func mustNewTestSidebar(t *testing.T, fc *cmuxctl.FakeClient, ws cmuxctl.Workspace) *cmuxSidebar {
	t.Helper()
	log := mustNewTestLogger(t)
	return newCmuxSidebar(fc, ws, log)
}

func validWorkspace() cmuxctl.Workspace {
	return cmuxctl.Workspace{ID: "fake-ws", Ref: "workspace:1"}
}

// TestCmuxSidebar_PushStep verifies that PushStep calls WorkspaceSetStatus once
// with sidebarStatusKey and the step name as the value.
func TestCmuxSidebar_PushStep(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	s := mustNewTestSidebar(t, fc, validWorkspace())

	if err := s.PushStep(context.Background(), "Feature work"); err != nil {
		t.Fatalf("PushStep error: %v", err)
	}

	if len(fc.SetStatusCalls) != 1 {
		t.Fatalf("expected 1 SetStatusCall, got %d", len(fc.SetStatusCalls))
	}
	call := fc.SetStatusCalls[0]
	if call.Key != sidebarStatusKey {
		t.Errorf("key = %q, want %q", call.Key, sidebarStatusKey)
	}
	if call.Value != "Feature work" {
		t.Errorf("value = %q, want %q", call.Value, "Feature work")
	}
}

// TestCmuxSidebar_PushProgress_Bounded verifies that PushProgress with maxIter > 0
// calls WorkspaceSetProgress with the correct fraction and label.
func TestCmuxSidebar_PushProgress_Bounded(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	s := mustNewTestSidebar(t, fc, validWorkspace())

	if err := s.PushProgress(context.Background(), 1, 3); err != nil {
		t.Fatalf("PushProgress error: %v", err)
	}

	if len(fc.SetProgressCalls) != 1 {
		t.Fatalf("expected 1 SetProgressCall, got %d", len(fc.SetProgressCalls))
	}
	call := fc.SetProgressCalls[0]
	wantFraction := 1.0 / 3.0
	if call.Fraction != wantFraction {
		t.Errorf("fraction = %v, want %v", call.Fraction, wantFraction)
	}
	if call.Label != "1 / 3" {
		t.Errorf("label = %q, want %q", call.Label, "1 / 3")
	}
}

// TestCmuxSidebar_PushProgress_Unbounded verifies that PushProgress with maxIter == 0
// is a no-op and produces zero SetProgressCalls.
func TestCmuxSidebar_PushProgress_Unbounded(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	s := mustNewTestSidebar(t, fc, validWorkspace())

	if err := s.PushProgress(context.Background(), 1, 0); err != nil {
		t.Fatalf("PushProgress error: %v", err)
	}

	if len(fc.SetProgressCalls) != 0 {
		t.Errorf("expected 0 SetProgressCalls for maxIter=0, got %d", len(fc.SetProgressCalls))
	}
}

// TestCmuxSidebar_EnterErrorMode verifies that after PushStep("Feature work"),
// EnterErrorMode sets status to "Feature work — awaiting input".
func TestCmuxSidebar_EnterErrorMode(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	s := mustNewTestSidebar(t, fc, validWorkspace())

	if err := s.PushStep(context.Background(), "Feature work"); err != nil {
		t.Fatalf("PushStep error: %v", err)
	}
	fc.SetStatusCalls = nil // reset recorder

	if err := s.EnterErrorMode(context.Background()); err != nil {
		t.Fatalf("EnterErrorMode error: %v", err)
	}

	if len(fc.SetStatusCalls) != 1 {
		t.Fatalf("expected 1 SetStatusCall after EnterErrorMode, got %d", len(fc.SetStatusCalls))
	}
	want := "Feature work" + errorModeSuffix
	if fc.SetStatusCalls[0].Value != want {
		t.Errorf("value = %q, want %q", fc.SetStatusCalls[0].Value, want)
	}
}

// TestCmuxSidebar_ClearAll_ClearsProgressOnlyIfPushed verifies that ClearAll
// calls ClearStatus always, and ClearProgress only when PushProgress was called.
func TestCmuxSidebar_ClearAll_ClearsProgressOnlyIfPushed(t *testing.T) {
	t.Run("with prior PushProgress", func(t *testing.T) {
		fc := &cmuxctl.FakeClient{}
		s := mustNewTestSidebar(t, fc, validWorkspace())
		_ = s.PushProgress(context.Background(), 1, 3)

		if err := s.ClearAll(context.Background()); err != nil {
			t.Fatalf("ClearAll error: %v", err)
		}
		if len(fc.ClearStatusCalls) != 1 {
			t.Errorf("expected 1 ClearStatusCall, got %d", len(fc.ClearStatusCalls))
		}
		if len(fc.ClearProgressCalls) != 1 {
			t.Errorf("expected 1 ClearProgressCall, got %d", len(fc.ClearProgressCalls))
		}
	})

	t.Run("without prior PushProgress", func(t *testing.T) {
		fc := &cmuxctl.FakeClient{}
		s := mustNewTestSidebar(t, fc, validWorkspace())

		if err := s.ClearAll(context.Background()); err != nil {
			t.Fatalf("ClearAll error: %v", err)
		}
		if len(fc.ClearStatusCalls) != 1 {
			t.Errorf("expected 1 ClearStatusCall, got %d", len(fc.ClearStatusCalls))
		}
		if len(fc.ClearProgressCalls) != 0 {
			t.Errorf("expected 0 ClearProgressCalls without prior PushProgress, got %d", len(fc.ClearProgressCalls))
		}
	})
}

// TestCmuxSidebar_NonTimeoutErrorIsSwallowed verifies that a non-deadline error
// returned by the client is logged and swallowed (PushStep returns nil).
func TestCmuxSidebar_NonTimeoutErrorIsSwallowed(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	fc.WorkspaceSetStatusFunc = func(_ context.Context, _ cmuxctl.Workspace, _, _ string) error {
		return errors.New("connection refused")
	}
	s := mustNewTestSidebar(t, fc, validWorkspace())

	err := s.PushStep(context.Background(), "step")
	if err != nil {
		t.Errorf("PushStep should return nil for non-deadline errors, got %v", err)
	}
}

// TestCmuxSidebar_TypedTimeoutErrorIsFatal verifies that *cmuxctl.TimeoutError
// (the queue-level timeout from RealClient) propagates from PushStep as fatal.
func TestCmuxSidebar_TypedTimeoutErrorIsFatal(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	fc.WorkspaceSetStatusFunc = func(_ context.Context, _ cmuxctl.Workspace, _, _ string) error {
		return &cmuxctl.TimeoutError{Method: "workspace.set_status", Duration: 8 * time.Second}
	}
	s := mustNewTestSidebar(t, fc, validWorkspace())

	err := s.PushStep(context.Background(), "step")
	var te *cmuxctl.TimeoutError
	if !errors.As(err, &te) {
		t.Errorf("PushStep should propagate *cmuxctl.TimeoutError, got %v (%T)", err, err)
	}
}

// TestCmuxSidebar_ContextDeadlineExceededIsSwallowed verifies that a plain
// context.DeadlineExceeded (not wrapped in *TimeoutError) is treated as a
// non-fatal sidebar error and swallowed.
func TestCmuxSidebar_ContextDeadlineExceededIsSwallowed(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	fc.WorkspaceSetStatusFunc = func(_ context.Context, _ cmuxctl.Workspace, _, _ string) error {
		return context.DeadlineExceeded
	}
	s := mustNewTestSidebar(t, fc, validWorkspace())

	err := s.PushStep(context.Background(), "step")
	if err != nil {
		t.Errorf("PushStep should swallow non-TypedTimeout errors, got %v", err)
	}
}

// TestCmuxSidebar_DisabledOnZeroWorkspace verifies that constructing a cmuxSidebar
// with a zero Workspace logs a warning and all methods become no-ops.
func TestCmuxSidebar_DisabledOnZeroWorkspace(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	log := mustNewTestLogger(t)

	// Capture log output by writing to a buffer via the log Writer.
	// We rely on the disabled flag making all methods no-ops.
	s := newCmuxSidebar(fc, cmuxctl.Workspace{}, log)

	if !s.disabled {
		t.Error("expected disabled=true for zero Workspace")
	}

	// All methods must be no-ops.
	_ = s.PushStep(context.Background(), "x")
	_ = s.PushProgress(context.Background(), 1, 3)
	_ = s.EnterErrorMode(context.Background())
	_ = s.ClearProgress(context.Background())
	_ = s.ClearAll(context.Background())

	if len(fc.SetStatusCalls) != 0 || len(fc.SetProgressCalls) != 0 ||
		len(fc.ClearStatusCalls) != 0 || len(fc.ClearProgressCalls) != 0 {
		t.Error("expected zero RPC calls on disabled sidebar")
	}
}

// TestSidebarAwareHeader_CallSequence drives initialize → iteration × 3 → first
// finalize → second finalize and asserts the ordered recorder-slice sequence on
// the FakeClient.
func TestSidebarAwareHeader_CallSequence(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	log := mustNewTestLogger(t)
	ws := validWorkspace()
	sidebar := newCmuxSidebar(fc, ws, log)

	ch := interactionchannel.NewFakeInteractionChannel()
	inner := newCmuxHeader(ch)
	inner.SetPhaseSteps([]string{"init-step"})

	h := &sidebarAwareHeader{
		inner:   inner,
		sidebar: sidebar,
		ctx:     context.Background(),
	}

	// Initialize phase.
	h.RenderInitializeLine(1, 1, "init-step")

	// Iteration phase × 3.
	h.RenderIterationLine(1, 3, "")
	h.RenderIterationLine(2, 3, "")
	h.RenderIterationLine(3, 3, "")

	// Finalize phase — first call clears progress, second does not.
	h.RenderFinalizeLine(1, 2, "finalize-step-1")
	h.RenderFinalizeLine(2, 2, "finalize-step-2")

	// Check SetStatusCalls: initialize push + 2 finalize pushes = 3 total
	wantStatus := []string{"init-step", "finalize-step-1", "finalize-step-2"}
	if len(fc.SetStatusCalls) != len(wantStatus) {
		t.Fatalf("SetStatusCalls: got %d, want %d: %+v", len(fc.SetStatusCalls), len(wantStatus), fc.SetStatusCalls)
	}
	for i, call := range fc.SetStatusCalls {
		if call.Value != wantStatus[i] {
			t.Errorf("SetStatusCalls[%d].Value = %q, want %q", i, call.Value, wantStatus[i])
		}
	}

	// Check SetProgressCalls: 3 iteration calls.
	if len(fc.SetProgressCalls) != 3 {
		t.Errorf("SetProgressCalls: got %d, want 3", len(fc.SetProgressCalls))
	}

	// Check ClearProgressCalls: exactly one (from first RenderFinalizeLine).
	if len(fc.ClearProgressCalls) != 1 {
		t.Errorf("ClearProgressCalls: got %d, want 1", len(fc.ClearProgressCalls))
	}
}

// TestNameAt_OutOfBounds verifies that nameAt returns "" for negative and
// out-of-bounds indices without panicking.
func TestNameAt_OutOfBounds(t *testing.T) {
	ch := interactionchannel.NewFakeInteractionChannel()
	h := newCmuxHeader(ch)
	h.SetPhaseSteps([]string{"a", "b", "c"})

	if got := h.nameAt(-1); got != "" {
		t.Errorf("nameAt(-1) = %q, want %q", got, "")
	}
	if got := h.nameAt(3); got != "" {
		t.Errorf("nameAt(3) = %q, want %q", got, "")
	}
	if got := h.nameAt(1); !strings.Contains(got, "b") {
		t.Errorf("nameAt(1) = %q, want %q", got, "b")
	}
}

// TestSidebarAwareHeader_SetStepState_ActivePushesStep verifies that SetStepState
// with StepActive calls sidebar.PushStep with the name at that index.
func TestSidebarAwareHeader_SetStepState_ActivePushesStep(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	log := mustNewTestLogger(t)
	ws := validWorkspace()
	sidebar := newCmuxSidebar(fc, ws, log)

	ch := interactionchannel.NewFakeInteractionChannel()
	inner := newCmuxHeader(ch)
	inner.SetPhaseSteps([]string{"step-a", "step-b"})

	h := &sidebarAwareHeader{
		inner:   inner,
		sidebar: sidebar,
		ctx:     context.Background(),
	}

	h.SetStepState(1, ui.StepActive)

	if len(fc.SetStatusCalls) != 1 {
		t.Fatalf("expected 1 SetStatusCall, got %d", len(fc.SetStatusCalls))
	}
	if fc.SetStatusCalls[0].Value != "step-b" {
		t.Errorf("SetStatusCalls[0].Value = %q, want %q", fc.SetStatusCalls[0].Value, "step-b")
	}
}

// TestCmuxSidebar_NilReceiverSafe verifies that all *cmuxSidebar methods are safe
// to call on a nil receiver (W-5 nil-guard contract).
func TestCmuxSidebar_NilReceiverSafe(t *testing.T) {
	var s *cmuxSidebar
	ctx := context.Background()
	if err := s.PushStep(ctx, "x"); err != nil {
		t.Errorf("PushStep: %v", err)
	}
	if err := s.PushProgress(ctx, 1, 3); err != nil {
		t.Errorf("PushProgress: %v", err)
	}
	if err := s.EnterErrorMode(ctx); err != nil {
		t.Errorf("EnterErrorMode: %v", err)
	}
	if err := s.ClearProgress(ctx); err != nil {
		t.Errorf("ClearProgress: %v", err)
	}
	if err := s.ClearAll(ctx); err != nil {
		t.Errorf("ClearAll: %v", err)
	}
}

// TestSidebarAwareHeader_SetStepState_NonActiveSilent verifies that SetStepState
// with a state other than StepActive does not call the sidebar.
func TestSidebarAwareHeader_SetStepState_NonActiveSilent(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	log := mustNewTestLogger(t)
	ws := validWorkspace()
	sidebar := newCmuxSidebar(fc, ws, log)

	ch := interactionchannel.NewFakeInteractionChannel()
	inner := newCmuxHeader(ch)
	inner.SetPhaseSteps([]string{"step-a"})

	h := &sidebarAwareHeader{
		inner:   inner,
		sidebar: sidebar,
		ctx:     context.Background(),
	}

	h.SetStepState(0, ui.StepDone)

	if len(fc.SetStatusCalls) != 0 {
		t.Errorf("expected 0 SetStatusCalls for StepDone, got %d", len(fc.SetStatusCalls))
	}
}
