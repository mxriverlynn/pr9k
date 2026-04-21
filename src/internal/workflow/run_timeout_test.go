package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/steps"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
	"github.com/mxriverlynn/pr9k/src/internal/vars"
)

// TP-003: Initialize-phase rec.Notes is set to "timed out after Ns" when the
// executor reports WasTimedOut() and the step has TimeoutSeconds > 0.
func TestRun_Timeout_InitializePhaseNotesWiring(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{
		projectDir:    projectDir,
		runStepErrors: []error{fmt.Errorf("killed")},
		wasTimedOut:   true,
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		InitializeSteps: []steps.Step{
			{Name: "init-step", IsClaude: false, Command: []string{"true"}, TimeoutSeconds: 60},
		},
	}

	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)
	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()
	time.Sleep(30 * time.Millisecond)
	actions <- ui.ActionContinue
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete")
	}

	recs := readIterationLog(t, projectDir)
	var found bool
	for _, rec := range recs {
		if rec.StepName == "init-step" {
			found = true
			if rec.Status != "failed" {
				t.Errorf("Status: want %q, got %q", "failed", rec.Status)
			}
			if rec.Notes != "timed out after 60s" {
				t.Errorf("Notes: want %q, got %q", "timed out after 60s", rec.Notes)
			}
			if rec.IterationNum != 0 {
				t.Errorf("IterationNum: want 0, got %d", rec.IterationNum)
			}
		}
	}
	if !found {
		t.Error("no record found for init-step in iteration log")
	}
}

// TP-004: Finalize-phase rec.Notes is set to "timed out after Ns" when the
// executor reports WasTimedOut() and the step has TimeoutSeconds > 0.
func TestRun_Timeout_FinalizePhaseNotesWiring(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{
		projectDir:    projectDir,
		runStepErrors: []error{fmt.Errorf("killed")},
		wasTimedOut:   true,
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		// Empty Steps to avoid ordering runStepErrors against iteration steps.
		FinalizeSteps: []steps.Step{
			{Name: "finalize-step", IsClaude: false, Command: []string{"true"}, TimeoutSeconds: 120},
		},
	}

	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)
	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()
	time.Sleep(30 * time.Millisecond)
	actions <- ui.ActionContinue
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete")
	}

	recs := readIterationLog(t, projectDir)
	var found bool
	for _, rec := range recs {
		if rec.StepName == "finalize-step" {
			found = true
			if rec.Status != "failed" {
				t.Errorf("Status: want %q, got %q", "failed", rec.Status)
			}
			if rec.Notes != "timed out after 120s" {
				t.Errorf("Notes: want %q, got %q", "timed out after 120s", rec.Notes)
			}
			if rec.IterationNum != 0 {
				t.Errorf("IterationNum: want 0, got %d", rec.IterationNum)
			}
		}
	}
	if !found {
		t.Error("no record found for finalize-step in iteration log")
	}
}

// SUGG-007a: Claude-branch sibling of TP-003. The initialize-phase notes wiring
// uses the RunSandboxedStep dispatcher path (IsClaude:true) which goes through
// a distinct code path from non-claude steps (run.go:129-148).
func TestRun_Timeout_InitializePhaseNotesWiring_Claude(t *testing.T) {
	workflowDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workflowDir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "prompts", "init.md"), []byte("do work"), 0644); err != nil {
		t.Fatal(err)
	}

	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{
		projectDir:             projectDir,
		runSandboxedStepErrors: []error{fmt.Errorf("killed")},
		wasTimedOut:            true,
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir: workflowDir,
		Iterations:  1,
		InitializeSteps: []steps.Step{
			{Name: "init-claude", IsClaude: true, Model: "sonnet", PromptFile: "init.md", TimeoutSeconds: 60},
		},
	}

	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)
	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()
	time.Sleep(30 * time.Millisecond)
	actions <- ui.ActionContinue
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete")
	}

	recs := readIterationLog(t, projectDir)
	var found bool
	for _, rec := range recs {
		if rec.StepName == "init-claude" {
			found = true
			if rec.Notes != "timed out after 60s" {
				t.Errorf("Notes: want %q, got %q", "timed out after 60s", rec.Notes)
			}
		}
	}
	if !found {
		t.Error("no record found for init-claude in iteration log")
	}
}

// SUGG-007b: Claude-branch sibling of TP-004. The finalize-phase notes wiring
// uses the RunSandboxedStep dispatcher path (IsClaude:true).
func TestRun_Timeout_FinalizePhaseNotesWiring_Claude(t *testing.T) {
	workflowDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workflowDir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "prompts", "final.md"), []byte("do finalize"), 0644); err != nil {
		t.Fatal(err)
	}

	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{
		projectDir:             projectDir,
		runSandboxedStepErrors: []error{fmt.Errorf("killed")},
		wasTimedOut:            true,
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir: workflowDir,
		Iterations:  1,
		FinalizeSteps: []steps.Step{
			{Name: "finalize-claude", IsClaude: true, Model: "sonnet", PromptFile: "final.md", TimeoutSeconds: 120},
		},
	}

	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)
	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()
	time.Sleep(30 * time.Millisecond)
	actions <- ui.ActionContinue
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete")
	}

	recs := readIterationLog(t, projectDir)
	var found bool
	for _, rec := range recs {
		if rec.StepName == "finalize-claude" {
			found = true
			if rec.Notes != "timed out after 120s" {
				t.Errorf("Notes: want %q, got %q", "timed out after 120s", rec.Notes)
			}
		}
	}
	if !found {
		t.Error("no record found for finalize-claude in iteration log")
	}
}

// TP-005: stepDispatcher propagates TimeoutSeconds from the ResolvedStep to
// RunSandboxedStep's SandboxOptions for claude steps.
func TestRun_Timeout_StepDispatcherPropagatesTimeoutToSandboxed(t *testing.T) {
	workflowDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workflowDir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "prompts", "p.md"), []byte("do work"), 0644); err != nil {
		t.Fatal(err)
	}

	executor := &fakeExecutor{projectDir: t.TempDir()}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: workflowDir,
		Iterations:  1,
		Steps: []steps.Step{
			{Name: "c", IsClaude: true, Model: "sonnet", PromptFile: "p.md", TimeoutSeconds: 42},
		},
	}

	Run(executor, header, kh, cfg)

	if len(executor.runSandboxedStepCalls) != 1 {
		t.Fatalf("expected 1 RunSandboxedStep call, got %d", len(executor.runSandboxedStepCalls))
	}
	got := executor.runSandboxedStepCalls[0].opts.TimeoutSeconds
	if got != 42 {
		t.Errorf("SandboxOptions.TimeoutSeconds: want 42, got %d", got)
	}
}

// TP-006: stepDispatcher propagates TimeoutSeconds from the ResolvedStep to
// RunStepFull's timeoutSeconds argument for non-claude steps.
func TestRun_Timeout_StepDispatcherPropagatesTimeoutToRunStepFull(t *testing.T) {
	executor := &fakeExecutor{}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps: []steps.Step{
			{Name: "s", IsClaude: false, Command: []string{"true"}, TimeoutSeconds: 7},
		},
	}

	Run(executor, header, kh, cfg)

	if len(executor.runStepFullTimeouts) < 1 {
		t.Fatalf("expected at least 1 RunStepFull call, got %d", len(executor.runStepFullTimeouts))
	}
	got := executor.runStepFullTimeouts[0]
	if got != 7 {
		t.Errorf("RunStepFull timeoutSeconds: want 7, got %d", got)
	}
}

// TP-009: The `&&` guard (executor.WasTimedOut() && s.TimeoutSeconds > 0)
// prevents the timeout note from being set on a step that failed for unrelated
// reasons (not a timeout, no TimeoutSeconds configured).
func TestRun_Timeout_NoteNotSetWhenNotTimedOut(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{
		projectDir:    projectDir,
		runStepErrors: []error{fmt.Errorf("exit 1")},
		wasTimedOut:   false,
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps: []steps.Step{
			// No TimeoutSeconds on this step.
			{Name: "fail-step", IsClaude: false, Command: []string{"false"}},
		},
	}

	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)
	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()
	time.Sleep(30 * time.Millisecond)
	actions <- ui.ActionContinue
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete")
	}

	recs := readIterationLog(t, projectDir)
	var found bool
	for _, rec := range recs {
		if rec.StepName == "fail-step" {
			found = true
			if rec.Notes != "" {
				t.Errorf("Notes: want empty string, got %q — timeout note must not leak into non-timeout failures", rec.Notes)
			}
			if rec.Status != "failed" {
				t.Errorf("Status: want %q, got %q", "failed", rec.Status)
			}
		}
	}
	if !found {
		t.Error("no record found for fail-step in iteration log")
	}
}

// TP-012 (MED): buildStep non-claude branch copies s.TimeoutSeconds into the
// returned ResolvedStep.
func TestBuildStep_NonClaude_CopiesTimeoutSeconds(t *testing.T) {
	workflowDir := t.TempDir()
	vt := vars.New(workflowDir, t.TempDir(), 1)
	executor := &fakeExecutor{projectDir: t.TempDir()}

	resolved, err := buildStep(
		workflowDir,
		steps.Step{IsClaude: false, Command: []string{"true"}, TimeoutSeconds: 300},
		vt, vars.Iteration, nil, nil, executor, "",
	)
	if err != nil {
		t.Fatalf("buildStep: %v", err)
	}
	if resolved.TimeoutSeconds != 300 {
		t.Errorf("ResolvedStep.TimeoutSeconds: want 300, got %d", resolved.TimeoutSeconds)
	}
}

// TP-013 (MED): buildStep claude branch copies s.TimeoutSeconds into the
// returned ResolvedStep.
func TestBuildStep_Claude_CopiesTimeoutSeconds(t *testing.T) {
	workflowDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workflowDir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "prompts", "p.md"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	vt := vars.New(workflowDir, t.TempDir(), 1)
	executor := &fakeExecutor{projectDir: t.TempDir()}

	resolved, err := buildStep(
		workflowDir,
		steps.Step{Name: "c", IsClaude: true, Model: "sonnet", PromptFile: "p.md", TimeoutSeconds: 300},
		vt, vars.Iteration, nil, nil, executor, "",
	)
	if err != nil {
		t.Fatalf("buildStep: %v", err)
	}
	if resolved.TimeoutSeconds != 300 {
		t.Errorf("ResolvedStep.TimeoutSeconds: want 300, got %d", resolved.TimeoutSeconds)
	}
}

// TOT-B1: buildStep non-claude branch copies s.OnTimeout into the ResolvedStep.
func TestBuildStep_NonClaude_CopiesOnTimeout(t *testing.T) {
	workflowDir := t.TempDir()
	vt := vars.New(workflowDir, t.TempDir(), 1)
	executor := &fakeExecutor{projectDir: t.TempDir()}

	resolved, err := buildStep(
		workflowDir,
		steps.Step{IsClaude: false, Command: []string{"true"}, TimeoutSeconds: 300, OnTimeout: "continue"},
		vt, vars.Iteration, nil, nil, executor, "",
	)
	if err != nil {
		t.Fatalf("buildStep: %v", err)
	}
	if resolved.OnTimeout != "continue" {
		t.Errorf("ResolvedStep.OnTimeout: want %q, got %q", "continue", resolved.OnTimeout)
	}
}

// TOT-B2: buildStep claude branch copies s.OnTimeout into the ResolvedStep.
func TestBuildStep_Claude_CopiesOnTimeout(t *testing.T) {
	workflowDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workflowDir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "prompts", "p.md"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	vt := vars.New(workflowDir, t.TempDir(), 1)
	executor := &fakeExecutor{projectDir: t.TempDir()}

	resolved, err := buildStep(
		workflowDir,
		steps.Step{Name: "c", IsClaude: true, Model: "sonnet", PromptFile: "p.md", TimeoutSeconds: 300, OnTimeout: "continue"},
		vt, vars.Iteration, nil, nil, executor, "",
	)
	if err != nil {
		t.Fatalf("buildStep: %v", err)
	}
	if resolved.OnTimeout != "continue" {
		t.Errorf("ResolvedStep.OnTimeout: want %q, got %q", "continue", resolved.OnTimeout)
	}
}

// TOT-X1: Cross-step integration — step A is a non-claude step with
// TimeoutSeconds and OnTimeout=continue whose subprocess the fakeExecutor
// "times out" (timer fires mid-step via onRunStepFull hook + error return);
// step B is a non-claude step that succeeds. Iteration log must contain:
//   - step A: exactly one record, status="failed", notes="timed out after Ns"
//   - step B: exactly one record, status="done", no spurious WARN-001 "timed out after 0s" record
//
// This covers V4/V13 from the plan review: without ClearTimeoutFlag, the
// residual WasTimedOut=true would fire stepDispatcher's onTimeoutRetry at the
// start of step B and emit a bogus "failed" record for step B.
func TestRun_OnTimeoutContinue_DoesNotLeakTimeoutFlagToNextStep(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{
		projectDir: projectDir,
		// Step A errors; step B succeeds.
		runStepErrors: []error{fmt.Errorf("killed by timer"), nil},
	}
	// Simulate the real Runner: wasTimedOut starts false and is set true by the
	// timer goroutine during step A's execution. The hook fires at the start of
	// each RunStepFull call; for idx==0 (step A) we set wasTimedOut=true so the
	// post-return check observes a timeout. For idx==1 (step B) we leave it
	// alone — if ClearTimeoutFlag was called after step A's soft-fail, it is
	// already false.
	executor.onRunStepFull = func(f *fakeExecutor, callIdx int) {
		if callIdx == 0 {
			f.wasTimedOut = true
		}
	}

	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps: []steps.Step{
			{Name: "step-a", IsClaude: false, Command: []string{"true"}, TimeoutSeconds: 60, OnTimeout: "continue"},
			{Name: "step-b", IsClaude: false, Command: []string{"true"}},
		},
	}

	kh := newTestKeyHandler()
	Run(executor, header, kh, cfg)

	recs := readIterationLog(t, projectDir)

	// Count records per step.
	recsByStep := map[string][]IterationRecord{}
	for _, r := range recs {
		recsByStep[r.StepName] = append(recsByStep[r.StepName], r)
	}

	// step-a: exactly one record, status="failed", notes="timed out after 60s".
	a := recsByStep["step-a"]
	if len(a) != 1 {
		t.Fatalf("step-a: want 1 record, got %d: %+v", len(a), a)
	}
	if a[0].Status != "failed" {
		t.Errorf("step-a Status: want %q, got %q", "failed", a[0].Status)
	}
	if a[0].Notes != "timed out after 60s" {
		t.Errorf("step-a Notes: want %q, got %q", "timed out after 60s", a[0].Notes)
	}

	// step-b: exactly one record, status="done", no timeout note.
	b := recsByStep["step-b"]
	if len(b) != 1 {
		t.Fatalf("step-b: want 1 record (no spurious WARN-001 leak), got %d: %+v", len(b), b)
	}
	if b[0].Status != "done" {
		t.Errorf("step-b Status: want %q, got %q — WasTimedOut flag leaked across steps", "done", b[0].Status)
	}
	if b[0].Notes != "" {
		t.Errorf("step-b Notes: want empty, got %q — stale timeout note leaked to next step", b[0].Notes)
	}

	// ClearTimeoutFlag was called at least once by the orchestrate-layer soft-fail branch.
	if executor.clearTimeoutFlagCalls < 1 {
		t.Errorf("expected ClearTimeoutFlag to be called on the soft-fail branch, got %d calls", executor.clearTimeoutFlagCalls)
	}
}
