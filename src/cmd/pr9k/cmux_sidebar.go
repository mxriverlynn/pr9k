package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
	"github.com/mxriverlynn/pr9k/src/internal/logger"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
	"github.com/mxriverlynn/pr9k/src/internal/workflow"
)

// sidebarStatusKey is the cmux workspace status key pr9k uses for step names (D-8).
const sidebarStatusKey = "pr9k.step"

// errorModeSuffix is appended to the step name when entering error mode (D-9).
// U+2014 em-dash — same precedent as ui.IterationIssueSep.
const errorModeSuffix = " — awaiting input"

// cmuxSidebar is the sidebar adapter that mirrors workflow progress into the
// cmux workspace status and progress fields. All exported methods are safe for
// concurrent use via snapshot-then-unlock (docs/coding-standards/concurrency.md).
//
// When constructed with an empty Workspace (ID=="" && Ref==""), the sidebar logs
// a warning and sets disabled=true; all subsequent method calls are no-ops.
type cmuxSidebar struct {
	client         cmuxctl.CmuxClient
	ws             cmuxctl.Workspace
	log            *logger.Logger
	mu             sync.Mutex
	lastStepName   string
	progressPushed bool
	disabled       bool
}

// newCmuxSidebar constructs a cmuxSidebar. If ws is empty, it logs a warning
// and marks the sidebar as disabled so all calls become no-ops.
func newCmuxSidebar(client cmuxctl.CmuxClient, ws cmuxctl.Workspace, log *logger.Logger) *cmuxSidebar {
	s := &cmuxSidebar{client: client, ws: ws, log: log}
	if ws.Empty() {
		_ = log.Log("cmuxSidebar", "warning: constructed with zero Workspace; sidebar updates disabled")
		s.disabled = true
	}
	return s
}

// pushStatus calls WorkspaceSetStatus with sidebarStatusKey and the given value.
func (s *cmuxSidebar) pushStatus(ctx context.Context, value string) error {
	if s == nil {
		return nil
	}
	return s.client.WorkspaceSetStatus(ctx, s.ws, sidebarStatusKey, value)
}

// PushStep records the current step name and pushes it to the cmux sidebar.
// context.DeadlineExceeded propagates as fatal; any other error is logged and
// swallowed so sidebar failures never abort the workflow.
func (s *cmuxSidebar) PushStep(ctx context.Context, name string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	disabled := s.disabled
	s.lastStepName = name
	s.mu.Unlock()

	if disabled {
		return nil
	}

	if err := s.pushStatus(ctx, name); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		_ = s.log.Log("cmuxSidebar", fmt.Sprintf("PushStep error: %v", err))
		return nil
	}
	return nil
}

// PushProgress pushes a bounded progress fraction to the cmux workspace.
// When maxIter <= 0 the call is a no-op (unbounded iteration mode).
func (s *cmuxSidebar) PushProgress(ctx context.Context, iter, maxIter int) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	disabled := s.disabled
	s.mu.Unlock()

	if disabled || maxIter <= 0 {
		return nil
	}

	fraction := float64(iter) / float64(maxIter)
	label := fmt.Sprintf("%d / %d", iter, maxIter)

	if err := s.client.WorkspaceSetProgress(ctx, s.ws, fraction, label); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		_ = s.log.Log("cmuxSidebar", fmt.Sprintf("PushProgress error: %v", err))
		return nil
	}

	s.mu.Lock()
	s.progressPushed = true
	s.mu.Unlock()
	return nil
}

// EnterErrorMode appends errorModeSuffix to the last step name and pushes the
// combined string to the cmux workspace status.
func (s *cmuxSidebar) EnterErrorMode(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	disabled := s.disabled
	value := s.lastStepName + errorModeSuffix
	s.mu.Unlock()

	if disabled {
		return nil
	}

	if err := s.pushStatus(ctx, value); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		_ = s.log.Log("cmuxSidebar", fmt.Sprintf("EnterErrorMode error: %v", err))
		return nil
	}
	return nil
}

// ClearProgress clears the cmux workspace progress indicator.
func (s *cmuxSidebar) ClearProgress(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	disabled := s.disabled
	s.mu.Unlock()

	if disabled {
		return nil
	}

	if err := s.client.WorkspaceClearProgress(ctx, s.ws); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		_ = s.log.Log("cmuxSidebar", fmt.Sprintf("ClearProgress error: %v", err))
		return nil
	}
	return nil
}

// ClearAll clears the cmux workspace status key and, when progress was
// previously pushed, clears the progress indicator too.
func (s *cmuxSidebar) ClearAll(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	disabled := s.disabled
	pushed := s.progressPushed
	s.mu.Unlock()

	if disabled {
		return nil
	}

	if err := s.client.WorkspaceClearStatus(ctx, s.ws, sidebarStatusKey); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		_ = s.log.Log("cmuxSidebar", fmt.Sprintf("ClearAll ClearStatus error: %v", err))
		return nil
	}

	if pushed {
		if err := s.client.WorkspaceClearProgress(ctx, s.ws); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			_ = s.log.Log("cmuxSidebar", fmt.Sprintf("ClearAll ClearProgress error: %v", err))
			return nil
		}
	}
	return nil
}

// sidebarAwareHeader wraps cmuxHeader to additionally mirror workflow progress
// into the cmux sidebar on every header mutation. It satisfies workflow.RunHeader.
type sidebarAwareHeader struct {
	inner         *cmuxHeader
	sidebar       *cmuxSidebar
	ctx           context.Context
	finalizeBegun bool
}

// Compile-time assertion: *sidebarAwareHeader satisfies workflow.RunHeader.
var _ workflow.RunHeader = (*sidebarAwareHeader)(nil)

func (h *sidebarAwareHeader) SetPhaseSteps(names []string) {
	h.inner.SetPhaseSteps(names)
}

func (h *sidebarAwareHeader) SetStepState(idx int, state ui.StepState) {
	h.inner.SetStepState(idx, state)
	if state == ui.StepActive {
		_ = h.sidebar.PushStep(h.ctx, h.inner.nameAt(idx))
	}
}

func (h *sidebarAwareHeader) RenderInitializeLine(stepNum, stepCount int, stepName string) {
	h.inner.RenderInitializeLine(stepNum, stepCount, stepName)
	_ = h.sidebar.PushStep(h.ctx, stepName)
}

func (h *sidebarAwareHeader) RenderIterationLine(iter, maxIter int, issueID string) {
	h.inner.RenderIterationLine(iter, maxIter, issueID)
	_ = h.sidebar.PushProgress(h.ctx, iter, maxIter)
}

// RenderFinalizeLine delegates to the inner header. On the first call, clears
// the progress indicator (D-13). Every call pushes the current step name to
// the sidebar.
func (h *sidebarAwareHeader) RenderFinalizeLine(stepNum, stepCount int, stepName string) {
	h.inner.RenderFinalizeLine(stepNum, stepCount, stepName)
	if !h.finalizeBegun {
		h.finalizeBegun = true
		_ = h.sidebar.ClearProgress(h.ctx)
	}
	_ = h.sidebar.PushStep(h.ctx, stepName)
}
