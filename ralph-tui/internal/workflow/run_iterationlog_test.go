package workflow

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/claudestream"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
)

// TP-004: InputTokens, OutputTokens, and SessionID are populated from
// capturedStats after a claude step completes. LastStats() is called exactly once
// (single-flight contract, D21).
func TestRun_IterationLog_CapturedStats(t *testing.T) {
	workflowDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workflowDir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "prompts", "step.txt"), []byte("do work"), 0644); err != nil {
		t.Fatal(err)
	}

	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{
		projectDir: projectDir,
		lastStatsReturn: []claudestream.StepStats{
			{InputTokens: 1234, OutputTokens: 567, SessionID: "abc-123"},
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: workflowDir,
		Iterations:  1,
		Steps: []steps.Step{
			{Name: "feature-work", IsClaude: true, Model: "sonnet", PromptFile: "step.txt"},
		},
	}

	Run(executor, header, kh, cfg)

	recs := readIterationLog(t, projectDir)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.InputTokens != 1234 {
		t.Errorf("InputTokens: want 1234, got %d", r.InputTokens)
	}
	if r.OutputTokens != 567 {
		t.Errorf("OutputTokens: want 567, got %d", r.OutputTokens)
	}
	if r.SessionID != "abc-123" {
		t.Errorf("SessionID: want %q, got %q", "abc-123", r.SessionID)
	}
	if executor.lastStatsCalls != 1 {
		t.Errorf("LastStats calls: want 1 (single-flight), got %d", executor.lastStatsCalls)
	}
}

// TP-005: Model field is copied from the step definition into the IterationRecord
// for all phases. Empty model is omitted from JSON (omitempty contract).
func TestRun_IterationLog_ModelField(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{projectDir: projectDir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps: []steps.Step{
			{Name: "opus-step", IsClaude: false, Command: []string{"echo", "x"}, Model: "opus"},
			{Name: "no-model-step", IsClaude: false, Command: []string{"echo", "x"}},
		},
	}

	Run(executor, header, kh, cfg)

	recs := readIterationLog(t, projectDir)
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	if recs[0].Model != "opus" {
		t.Errorf("record 0 Model: want %q, got %q", "opus", recs[0].Model)
	}
	if recs[1].Model != "" {
		t.Errorf("record 1 Model: want %q, got %q", "", recs[1].Model)
	}
	// Verify omitempty: re-marshal the second record and confirm "model" is absent.
	data, err := json.Marshal(recs[1])
	if err != nil {
		t.Fatalf("marshal record 1: %v", err)
	}
	if strings.Contains(string(data), `"model"`) {
		t.Errorf("expected omitempty to elide empty model field; got %s", data)
	}
}

// TP-006: IssueID in the iteration record reflects the VarTable state at the time
// the record is appended, not after the captureAs bind. For a step that captures
// ISSUE_ID, the record for that step has IssueID==""; the next step sees "42".
func TestRun_IterationLog_IssueIDOrderingAfterCaptureAs(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{
		projectDir:      projectDir,
		runStepCaptures: []string{"42", ""},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps: []steps.Step{
			captureStep("get_issue", "ISSUE_ID"),
			nonClaudeSteps("feature-work")[0],
		},
	}

	Run(executor, header, kh, cfg)

	recs := readIterationLog(t, projectDir)
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	if recs[0].IssueID != "" {
		t.Errorf("get_issue record IssueID: want empty (bind after append), got %q", recs[0].IssueID)
	}
	if recs[1].IssueID != "42" {
		t.Errorf("feature-work record IssueID: want %q, got %q", "42", recs[1].IssueID)
	}
}

// TP-007: AppendIterationRecord write failures are non-fatal. Both iteration
// steps must complete even when the log write returns an error, and a warning
// must appear in the executor's log output.
func TestRun_IterationLog_WriteFailureNonFatal(t *testing.T) {
	// projectDir has no .ralph-cache/ — AppendIterationRecord returns an error.
	projectDir := t.TempDir()
	executor := &fakeExecutor{projectDir: projectDir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       nonClaudeSteps("step-one", "step-two"),
	}

	Run(executor, header, kh, cfg)

	if len(executor.runStepCalls) != 2 {
		t.Errorf("both steps must execute despite log write error; got %d calls: %v",
			len(executor.runStepCalls), executor.runStepCalls)
	}
	warnFound := false
	for _, line := range executor.logLines {
		if strings.HasPrefix(line, "warning: workflow: iteration log: ") {
			warnFound = true
			break
		}
	}
	if !warnFound {
		t.Errorf("expected at least one log line with prefix %q; got %v",
			"warning: workflow: iteration log: ", executor.logLines)
	}
}

// TP-009: Initialize phase appends a record with IterationNum==0 and the
// correct status when the step fails.
func TestRun_IterationLog_InitializePhase(t *testing.T) {
	projectDir := makeCacheDir(t)
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		projectDir:    projectDir,
		runStepErrors: []error{errors.New("boom")},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir:     t.TempDir(),
		Iterations:      1,
		InitializeSteps: nonClaudeSteps("init-step"),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()

	// Allow the goroutine to reach the blocking point in Orchestrate.
	time.Sleep(30 * time.Millisecond)
	actions <- ui.ActionContinue

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete after ActionContinue")
	}

	recs := readIterationLog(t, projectDir)
	var found bool
	for _, r := range recs {
		if r.StepName == "init-step" {
			found = true
			if r.Status != "failed" {
				t.Errorf("init-step Status: want %q, got %q", "failed", r.Status)
			}
			if r.IterationNum != 0 {
				t.Errorf("init-step IterationNum: want 0, got %d", r.IterationNum)
			}
		}
	}
	if !found {
		t.Error("no record found for init-step in iteration log")
	}
}

// TP-010: Finalize phase appends a record with IterationNum==0 and status "done".
func TestRun_IterationLog_FinalizePhase(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{projectDir: projectDir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    1,
		FinalizeSteps: nonClaudeSteps("git-push"),
	}

	Run(executor, header, kh, cfg)

	recs := readIterationLog(t, projectDir)
	if len(recs) != 1 {
		t.Fatalf("want 1 record (finalize only), got %d", len(recs))
	}
	r := recs[0]
	if r.StepName != "git-push" {
		t.Errorf("StepName: want %q, got %q", "git-push", r.StepName)
	}
	if r.Status != "done" {
		t.Errorf("Status: want %q, got %q", "done", r.Status)
	}
	if r.IterationNum != 0 {
		t.Errorf("IterationNum: want 0, got %d", r.IterationNum)
	}
}

// TP-011: IterationNum sequence across all three phases is [0, 1, 2, 0]
// (initialize=0, iteration1=1, iteration2=2, finalize=0).
func TestRun_IterationLog_IterationNumSequence(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{projectDir: projectDir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:     t.TempDir(),
		Iterations:      2,
		InitializeSteps: nonClaudeSteps("init"),
		Steps:           nonClaudeSteps("iter-step"),
		FinalizeSteps:   nonClaudeSteps("finalize"),
	}

	Run(executor, header, kh, cfg)

	recs := readIterationLog(t, projectDir)
	if len(recs) != 4 {
		t.Fatalf("want 4 records (1 init + 2 iter + 1 finalize), got %d", len(recs))
	}
	wantNums := []int{0, 1, 2, 0}
	for i, r := range recs {
		if r.IterationNum != wantNums[i] {
			t.Errorf("record %d IterationNum: want %d, got %d", i, wantNums[i], r.IterationNum)
		}
	}
}

// TP-012: DurationS is non-negative for every record.
func TestRun_IterationLog_DurationS(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{projectDir: projectDir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       nonClaudeSteps("timed-step"),
	}

	Run(executor, header, kh, cfg)

	recs := readIterationLog(t, projectDir)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	if recs[0].DurationS < 0 {
		t.Errorf("DurationS: want >= 0, got %f", recs[0].DurationS)
	}
}

// TP-013: stepDispatcher captures stats once per RunSandboxedStep call and never
// calls LastStats more than once per RunStep invocation (D21 single-flight contract).
func TestStepDispatcher_CapturedStats(t *testing.T) {
	exec := &fakeExecutor{
		lastStatsReturn: []claudestream.StepStats{
			{InputTokens: 100},
			{InputTokens: 200},
		},
	}
	current := ui.ResolvedStep{
		Name:     "claude-step",
		IsClaude: true,
		Command:  []string{"docker", "run", "image"},
	}
	d := &stepDispatcher{exec: exec, current: current}

	if err := d.RunStep("claude-step", current.Command); err != nil {
		t.Fatalf("first RunStep: %v", err)
	}
	if d.capturedStats.InputTokens != 100 {
		t.Errorf("after first call: capturedStats.InputTokens want 100, got %d", d.capturedStats.InputTokens)
	}

	if err := d.RunStep("claude-step", current.Command); err != nil {
		t.Fatalf("second RunStep: %v", err)
	}
	if d.capturedStats.InputTokens != 200 {
		t.Errorf("after second call: capturedStats.InputTokens want 200, got %d", d.capturedStats.InputTokens)
	}

	if exec.lastStatsCalls != 2 {
		t.Errorf("LastStats calls: want 2 (one per RunStep), got %d", exec.lastStatsCalls)
	}
}

// TP-014: stepStatus maps all StepState values to the correct string.
// StepPending returns "unknown" (SetStepState was never called — step never ran).
// The default branch catches any unlisted future states and maps them to "done".
func TestStepStatus(t *testing.T) {
	tests := []struct {
		state ui.StepState
		want  string
	}{
		{ui.StepDone, "done"},
		{ui.StepFailed, "failed"},
		{ui.StepSkipped, "skipped"},
		{ui.StepPending, "unknown"},
		{ui.StepActive, "done"},
	}
	for _, tc := range tests {
		if got := stepStatus(tc.state); got != tc.want {
			t.Errorf("stepStatus(%v) = %q, want %q", tc.state, got, tc.want)
		}
	}
}

// TP-015: stateTracker.SetStepState records the last state and ignores the idx
// parameter (intentional — only the final state matters for IterationRecord).
func TestStateTracker_SetStepState(t *testing.T) {
	s := &stateTracker{}

	s.SetStepState(0, ui.StepActive)
	s.SetStepState(0, ui.StepDone)
	if s.lastState != ui.StepDone {
		t.Errorf("lastState after idx=0: want StepDone, got %v", s.lastState)
	}

	// Non-zero idx must also be accepted (idx is intentionally ignored).
	s.SetStepState(99, ui.StepFailed)
	if s.lastState != ui.StepFailed {
		t.Errorf("lastState after idx=99: want StepFailed, got %v", s.lastState)
	}
}

// badStep creates a non-claude step with an invalid captureMode, which causes
// buildStep to return an error (simulating a prep failure).
func badStep(name string) steps.Step {
	return steps.Step{
		Name:        name,
		IsClaude:    false,
		Command:     []string{"echo", name},
		CaptureMode: "badMode",
	}
}

// TP-017: buildStep failure in the initialize phase appends a failed record
// with Notes containing the error message (SUGG-005).
func TestRun_IterationLog_InitializePrepFailure(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{projectDir: projectDir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:     t.TempDir(),
		Iterations:      1,
		InitializeSteps: []steps.Step{badStep("init-bad")},
	}

	Run(executor, header, kh, cfg)

	recs := readIterationLog(t, projectDir)
	if len(recs) != 1 {
		t.Fatalf("want 1 record for failed init step, got %d", len(recs))
	}
	r := recs[0]
	if r.StepName != "init-bad" {
		t.Errorf("StepName: want %q, got %q", "init-bad", r.StepName)
	}
	if r.Status != "failed" {
		t.Errorf("Status: want %q, got %q", "failed", r.Status)
	}
	if r.Notes == "" {
		t.Error("Notes must contain the prep error message")
	}
	if r.IterationNum != 0 {
		t.Errorf("IterationNum: want 0, got %d", r.IterationNum)
	}
}

// TP-018: buildStep failure in the iteration phase appends a failed record
// with Notes containing the error message (SUGG-005).
func TestRun_IterationLog_IterationPrepFailure(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{projectDir: projectDir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps:       []steps.Step{badStep("iter-bad")},
	}

	Run(executor, header, kh, cfg)

	recs := readIterationLog(t, projectDir)
	if len(recs) != 1 {
		t.Fatalf("want 1 record for failed iteration step, got %d", len(recs))
	}
	r := recs[0]
	if r.StepName != "iter-bad" {
		t.Errorf("StepName: want %q, got %q", "iter-bad", r.StepName)
	}
	if r.Status != "failed" {
		t.Errorf("Status: want %q, got %q", "failed", r.Status)
	}
	if r.Notes == "" {
		t.Error("Notes must contain the prep error message")
	}
	if r.IterationNum != 1 {
		t.Errorf("IterationNum: want 1, got %d", r.IterationNum)
	}
}

// TP-019: buildStep failure in the finalize phase appends a failed record
// with Notes containing the error message (SUGG-005).
func TestRun_IterationLog_FinalizePrepFailure(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{projectDir: projectDir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:   t.TempDir(),
		Iterations:    1,
		FinalizeSteps: []steps.Step{badStep("finalize-bad")},
	}

	Run(executor, header, kh, cfg)

	recs := readIterationLog(t, projectDir)
	if len(recs) != 1 {
		t.Fatalf("want 1 record for failed finalize step, got %d", len(recs))
	}
	r := recs[0]
	if r.StepName != "finalize-bad" {
		t.Errorf("StepName: want %q, got %q", "finalize-bad", r.StepName)
	}
	if r.Status != "failed" {
		t.Errorf("Status: want %q, got %q", "failed", r.Status)
	}
	if r.Notes == "" {
		t.Error("Notes must contain the prep error message")
	}
	if r.IterationNum != 0 {
		t.Errorf("IterationNum: want 0, got %d", r.IterationNum)
	}
}

// TestRun_SkipIfCaptureEmpty_SourceDoneEmptyCapture: when the source step
// completes (StepDone) with empty capture, the target step is skipped and
// the iteration log records status "skipped".
func TestRun_SkipIfCaptureEmpty_SourceDoneEmptyCapture(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{
		projectDir: projectDir,
		// step 0 (verdict) returns empty capture; step 1 (fix) should be skipped
		runStepCaptures: []string{""},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps: []steps.Step{
			{Name: "verdict", IsClaude: false, Command: []string{"echo"}, CaptureAs: "VERDICT"},
			{Name: "fix", IsClaude: false, Command: []string{"echo", "fix"}, SkipIfCaptureEmpty: "VERDICT"},
		},
	}

	Run(executor, header, kh, cfg)

	recs := readIterationLog(t, projectDir)
	// 2 records: verdict (done) + fix (skipped)
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d: %v", len(recs), recs)
	}
	if recs[0].Status != "done" {
		t.Errorf("verdict record Status: want %q, got %q", "done", recs[0].Status)
	}
	if recs[0].StepName != "verdict" {
		t.Errorf("verdict record StepName: want %q, got %q", "verdict", recs[0].StepName)
	}
	if recs[1].Status != "skipped" {
		t.Errorf("fix record Status: want %q, got %q", "skipped", recs[1].Status)
	}
	if recs[1].StepName != "fix" {
		t.Errorf("fix record StepName: want %q, got %q", "fix", recs[1].StepName)
	}
	// The skip step should NOT have been executed.
	if len(executor.runStepCalls) != 1 {
		t.Errorf("runStepCalls: want 1 (only verdict), got %d", len(executor.runStepCalls))
	}
	// Header should reflect skipped state for step index 1.
	skippedCalls := 0
	for _, sc := range header.stepStateCalls {
		if sc.idx == 1 && sc.state == ui.StepSkipped {
			skippedCalls++
		}
	}
	if skippedCalls == 0 {
		t.Error("SetStepState(1, StepSkipped) was never called")
	}
}

// TestRun_SkipIfCaptureEmpty_SourceDoneNonEmptyCapture: when the source step
// completes (StepDone) with non-empty capture, the target step runs normally.
func TestRun_SkipIfCaptureEmpty_SourceDoneNonEmptyCapture(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{
		projectDir:      projectDir,
		runStepCaptures: []string{"yes"}, // non-empty verdict → fix must run
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps: []steps.Step{
			{Name: "verdict", IsClaude: false, Command: []string{"echo", "yes"}, CaptureAs: "VERDICT"},
			{Name: "fix", IsClaude: false, Command: []string{"echo", "fix"}, SkipIfCaptureEmpty: "VERDICT"},
		},
	}

	Run(executor, header, kh, cfg)

	// Both steps must have been called.
	if len(executor.runStepCalls) != 2 {
		t.Errorf("runStepCalls: want 2, got %d", len(executor.runStepCalls))
	}
	recs := readIterationLog(t, projectDir)
	if len(recs) != 2 {
		t.Fatalf("want 2 iteration records, got %d", len(recs))
	}
	if recs[1].Status != "done" {
		t.Errorf("fix record Status: want %q, got %q", "done", recs[1].Status)
	}
}

// TestRun_SkipIfCaptureEmpty_SourceFailedEmptyCapture: when the source step
// fails (StepFailed), the target step does NOT skip even if the capture is empty.
// The error from the source step already propagated to ModeError; the fix step
// runs normally so a crashing verdict script cannot silently suppress the fix.
func TestRun_SkipIfCaptureEmpty_SourceFailedEmptyCapture(t *testing.T) {
	projectDir := makeCacheDir(t)
	actions := make(chan ui.StepAction, 10)
	kh := ui.NewKeyHandler(func() {}, actions)

	executor := &fakeExecutor{
		projectDir: projectDir,
		// step 0 (verdict) returns an error → StepFailed; capture will be empty
		runStepErrors:   []error{errors.New("permission denied")},
		runStepCaptures: []string{""},
	}
	header := &fakeRunHeader{}

	cfg := RunConfig{
		WorkflowDir: t.TempDir(),
		Iterations:  1,
		Steps: []steps.Step{
			{Name: "verdict", IsClaude: false, Command: []string{"scripts/verdict"}, CaptureAs: "VERDICT"},
			{Name: "fix", IsClaude: false, Command: []string{"echo", "fix"}, SkipIfCaptureEmpty: "VERDICT"},
		},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(executor, header, kh, cfg)
	}()

	// Allow the goroutine to reach the blocking point in Orchestrate (ModeError).
	time.Sleep(30 * time.Millisecond)
	actions <- ui.ActionContinue

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete after ActionContinue")
	}

	// Both steps must have been dispatched (fix was not silently skipped).
	if len(executor.runStepCalls) != 2 {
		t.Errorf("runStepCalls: want 2 (verdict + fix), got %d", len(executor.runStepCalls))
	}
	if len(executor.runStepCalls) >= 2 && executor.runStepCalls[1].name != "fix" {
		t.Errorf("second call name: want %q, got %q", "fix", executor.runStepCalls[1].name)
	}
}

// TP-016: SchemaVersion == 1 in records from all three phases (initialize,
// iteration, finalize). Bumping the literal requires updating this test.
func TestRun_IterationLog_SchemaVersionAllPhases(t *testing.T) {
	projectDir := makeCacheDir(t)
	executor := &fakeExecutor{projectDir: projectDir}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir:     t.TempDir(),
		Iterations:      1,
		InitializeSteps: nonClaudeSteps("init"),
		Steps:           nonClaudeSteps("iter-step"),
		FinalizeSteps:   nonClaudeSteps("finalize"),
	}

	Run(executor, header, kh, cfg)

	recs := readIterationLog(t, projectDir)
	if len(recs) != 3 {
		t.Fatalf("want 3 records (1 init + 1 iter + 1 finalize), got %d", len(recs))
	}
	for i, r := range recs {
		if r.SchemaVersion != 1 {
			t.Errorf("record %d SchemaVersion: want 1, got %d", i, r.SchemaVersion)
		}
	}
}
