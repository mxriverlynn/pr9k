package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/claudestream"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
)

// --- evaluateResumeGates unit tests ---

// TestEvaluateResumeGates_G1_EmptySessionID verifies gate G1: when the
// previous step produced no session ID, resume is blocked.
func TestEvaluateResumeGates_G1_EmptySessionID(t *testing.T) {
	t.Parallel()
	prev := claudestream.StepStats{SessionID: "", InputTokens: 100}
	sid, gate := evaluateResumeGates(prev, ui.StepDone, func(string) bool { return false })
	if sid != "" {
		t.Errorf("G1: want empty session ID, got %q", sid)
	}
	if !strings.HasPrefix(gate, "G1:") {
		t.Errorf("G1: want gate label starting with 'G1:', got %q", gate)
	}
}

// TestEvaluateResumeGates_G2_PrevStepFailed verifies gate G2: when the
// previous step ended as StepFailed, resume is blocked.
func TestEvaluateResumeGates_G2_PrevStepFailed(t *testing.T) {
	t.Parallel()
	prev := claudestream.StepStats{SessionID: "sid-abc", InputTokens: 100}
	sid, gate := evaluateResumeGates(prev, ui.StepFailed, func(string) bool { return false })
	if sid != "" {
		t.Errorf("G2: want empty session ID, got %q", sid)
	}
	if !strings.HasPrefix(gate, "G2:") {
		t.Errorf("G2: want gate label starting with 'G2:', got %q", gate)
	}
}

// TestEvaluateResumeGates_G2_PrevStepSkipped verifies gate G2 for the skipped
// state: skipped is also not StepDone and must block resume.
func TestEvaluateResumeGates_G2_PrevStepSkipped(t *testing.T) {
	t.Parallel()
	prev := claudestream.StepStats{SessionID: "sid-abc", InputTokens: 100}
	sid, gate := evaluateResumeGates(prev, ui.StepSkipped, func(string) bool { return false })
	if sid != "" {
		t.Errorf("G2/Skipped: want empty session ID, got %q", sid)
	}
	if !strings.HasPrefix(gate, "G2:") {
		t.Errorf("G2/Skipped: want gate label starting with 'G2:', got %q", gate)
	}
}

// TestEvaluateResumeGates_G4_TokensAtLimit verifies gate G4: when the previous
// step's input token count equals the 200 000 threshold, resume is blocked.
func TestEvaluateResumeGates_G4_TokensAtLimit(t *testing.T) {
	t.Parallel()
	prev := claudestream.StepStats{SessionID: "sid-abc", InputTokens: 200_000}
	sid, gate := evaluateResumeGates(prev, ui.StepDone, func(string) bool { return false })
	if sid != "" {
		t.Errorf("G4 at limit: want empty session ID, got %q", sid)
	}
	if !strings.HasPrefix(gate, "G4:") {
		t.Errorf("G4 at limit: want gate label starting with 'G4:', got %q", gate)
	}
}

// TestEvaluateResumeGates_G4_TokensBelowLimit verifies that a token count just
// below the 200 000 threshold does not trigger G4.
func TestEvaluateResumeGates_G4_TokensBelowLimit(t *testing.T) {
	t.Parallel()
	prev := claudestream.StepStats{SessionID: "sid-abc", InputTokens: 199_999}
	sid, gate := evaluateResumeGates(prev, ui.StepDone, func(string) bool { return false })
	if gate != "" {
		t.Errorf("G4 below limit: want empty gate, got %q", gate)
	}
	if sid != "sid-abc" {
		t.Errorf("G4 below limit: want sid %q, got %q", "sid-abc", sid)
	}
}

// TestEvaluateResumeGates_G5_Blacklisted verifies gate G5: when the previous
// step's session ID appears in the timeout blacklist, resume is blocked.
func TestEvaluateResumeGates_G5_Blacklisted(t *testing.T) {
	t.Parallel()
	prev := claudestream.StepStats{SessionID: "timed-out-sid", InputTokens: 100}
	sid, gate := evaluateResumeGates(prev, ui.StepDone, func(id string) bool { return id == "timed-out-sid" })
	if sid != "" {
		t.Errorf("G5: want empty session ID, got %q", sid)
	}
	if !strings.HasPrefix(gate, "G5:") {
		t.Errorf("G5: want gate label starting with 'G5:', got %q", gate)
	}
}

// TestEvaluateResumeGates_AllPass verifies that when all five gates pass, the
// previous step's session ID is returned and the gate label is empty.
func TestEvaluateResumeGates_AllPass(t *testing.T) {
	t.Parallel()
	prev := claudestream.StepStats{SessionID: "good-sid", InputTokens: 10_000}
	sid, gate := evaluateResumeGates(prev, ui.StepDone, func(string) bool { return false })
	if sid != "good-sid" {
		t.Errorf("AllPass: want sid %q, got %q", "good-sid", sid)
	}
	if gate != "" {
		t.Errorf("AllPass: want empty gate label, got %q", gate)
	}
}

// --- Integration tests: Run() passes --resume to sandboxed command ---

// makeResumePromptFile writes "test prompt" to workflowDir/prompts/<filename>.
func makeResumePromptFile(t *testing.T, workflowDir, filename string) {
	t.Helper()
	dir := filepath.Join(workflowDir, "prompts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("makeResumePromptFile: mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte("test prompt"), 0o644); err != nil {
		t.Fatalf("makeResumePromptFile: write: %v", err)
	}
}

// resumeClaudeStep returns a Step with IsClaude=true pointing to promptFile.
// resumePrevious controls whether the step requests session resumption.
func resumeClaudeStep(name, promptFile string, resumePrevious bool) steps.Step {
	return steps.Step{
		Name:           name,
		IsClaude:       true,
		PromptFile:     promptFile,
		ResumePrevious: resumePrevious,
	}
}

// TestRun_ResumePrevious_AllGatesPass_InjectsResumeFlag verifies that when all
// five resume gates pass, the second initialize step's docker command contains
// "--resume <sid>".
func TestRun_ResumePrevious_AllGatesPass_InjectsResumeFlag(t *testing.T) {
	workflowDir := t.TempDir()
	makeResumePromptFile(t, workflowDir, "step1.txt")
	makeResumePromptFile(t, workflowDir, "step2.txt")

	const prevSID = "resume-me-sid"
	executor := &fakeExecutor{
		projectDir: makeCacheDir(t),
		lastStatsReturn: []claudestream.StepStats{
			{SessionID: prevSID, InputTokens: 5_000},
			{SessionID: "step2-sid", InputTokens: 200},
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: workflowDir,
		Iterations:  1,
		InitializeSteps: []steps.Step{
			resumeClaudeStep("init-step-1", "step1.txt", false),
			resumeClaudeStep("init-step-2", "step2.txt", true),
		},
	}

	Run(executor, header, kh, cfg)

	if len(executor.runSandboxedStepCalls) < 2 {
		t.Fatalf("want at least 2 RunSandboxedStep calls, got %d", len(executor.runSandboxedStepCalls))
	}
	cmd := executor.runSandboxedStepCalls[1].command
	found := false
	for i, arg := range cmd {
		if arg == "--resume" && i+1 < len(cmd) && cmd[i+1] == prevSID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("second step command does not contain '--resume %s': %v", prevSID, cmd)
	}
}

// TestRun_ResumePrevious_G1Blocked_FreshSession verifies that when G1 blocks
// (previous step has no session ID), the second step runs without --resume and
// a log entry containing "G1:" is written.
func TestRun_ResumePrevious_G1Blocked_FreshSession(t *testing.T) {
	workflowDir := t.TempDir()
	makeResumePromptFile(t, workflowDir, "step1.txt")
	makeResumePromptFile(t, workflowDir, "step2.txt")

	executor := &fakeExecutor{
		projectDir: makeCacheDir(t),
		lastStatsReturn: []claudestream.StepStats{
			{SessionID: "", InputTokens: 5_000}, // G1 blocks: no session ID
			{},
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: workflowDir,
		Iterations:  1,
		InitializeSteps: []steps.Step{
			resumeClaudeStep("init-step-1", "step1.txt", false),
			resumeClaudeStep("init-step-2", "step2.txt", true),
		},
	}

	Run(executor, header, kh, cfg)

	if len(executor.runSandboxedStepCalls) < 2 {
		t.Fatalf("want at least 2 RunSandboxedStep calls, got %d", len(executor.runSandboxedStepCalls))
	}
	cmd := executor.runSandboxedStepCalls[1].command
	for _, arg := range cmd {
		if arg == "--resume" {
			t.Errorf("second step command must not contain '--resume' when G1 blocks: %v", cmd)
		}
	}
	var gateLogged bool
	for _, line := range executor.logLines {
		if strings.Contains(line, "G1:") {
			gateLogged = true
			break
		}
	}
	if !gateLogged {
		t.Errorf("expected log entry mentioning G1 gate, got lines: %v", executor.logLines)
	}
}

// TestRun_ResumePrevious_G4Blocked_FreshSession verifies that when G4 blocks
// (input tokens >= 200 000), the second step runs without --resume.
func TestRun_ResumePrevious_G4Blocked_FreshSession(t *testing.T) {
	workflowDir := t.TempDir()
	makeResumePromptFile(t, workflowDir, "step1.txt")
	makeResumePromptFile(t, workflowDir, "step2.txt")

	executor := &fakeExecutor{
		projectDir: makeCacheDir(t),
		lastStatsReturn: []claudestream.StepStats{
			{SessionID: "fat-context-sid", InputTokens: 200_000}, // G4 blocks
			{},
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: workflowDir,
		Iterations:  1,
		InitializeSteps: []steps.Step{
			resumeClaudeStep("init-step-1", "step1.txt", false),
			resumeClaudeStep("init-step-2", "step2.txt", true),
		},
	}

	Run(executor, header, kh, cfg)

	if len(executor.runSandboxedStepCalls) < 2 {
		t.Fatalf("want at least 2 RunSandboxedStep calls, got %d", len(executor.runSandboxedStepCalls))
	}
	cmd := executor.runSandboxedStepCalls[1].command
	for _, arg := range cmd {
		if arg == "--resume" {
			t.Errorf("second step command must not contain '--resume' when G4 blocks: %v", cmd)
		}
	}
	var gateLogged bool
	for _, line := range executor.logLines {
		if strings.Contains(line, "G4:") {
			gateLogged = true
			break
		}
	}
	if !gateLogged {
		t.Errorf("expected log entry mentioning G4 gate, got lines: %v", executor.logLines)
	}
}

// TestRun_ResumePrevious_G5Blocked_FreshSession verifies that when G5 blocks
// (session ID is in the timeout blacklist), the second step runs without --resume.
func TestRun_ResumePrevious_G5Blocked_FreshSession(t *testing.T) {
	workflowDir := t.TempDir()
	makeResumePromptFile(t, workflowDir, "step1.txt")
	makeResumePromptFile(t, workflowDir, "step2.txt")

	const blacklistedSID = "timed-out-session"
	executor := &fakeExecutor{
		projectDir: makeCacheDir(t),
		lastStatsReturn: []claudestream.StepStats{
			{SessionID: blacklistedSID, InputTokens: 1_000},
			{},
		},
		sessionBlacklist: map[string]bool{blacklistedSID: true},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: workflowDir,
		Iterations:  1,
		InitializeSteps: []steps.Step{
			resumeClaudeStep("init-step-1", "step1.txt", false),
			resumeClaudeStep("init-step-2", "step2.txt", true),
		},
	}

	Run(executor, header, kh, cfg)

	if len(executor.runSandboxedStepCalls) < 2 {
		t.Fatalf("want at least 2 RunSandboxedStep calls, got %d", len(executor.runSandboxedStepCalls))
	}
	cmd := executor.runSandboxedStepCalls[1].command
	for _, arg := range cmd {
		if arg == "--resume" {
			t.Errorf("second step command must not contain '--resume' when G5 blocks: %v", cmd)
		}
	}
	var gateLogged bool
	for _, line := range executor.logLines {
		if strings.Contains(line, "G5:") {
			gateLogged = true
			break
		}
	}
	if !gateLogged {
		t.Errorf("expected log entry mentioning G5 gate, got lines: %v", executor.logLines)
	}
}

// TestRun_ResumePrevious_FalseDoesNotInjectResume verifies that a step with
// ResumePrevious=false never receives --resume even when the previous step has
// a valid session ID.
func TestRun_ResumePrevious_FalseDoesNotInjectResume(t *testing.T) {
	workflowDir := t.TempDir()
	makeResumePromptFile(t, workflowDir, "step1.txt")
	makeResumePromptFile(t, workflowDir, "step2.txt")

	executor := &fakeExecutor{
		projectDir: makeCacheDir(t),
		lastStatsReturn: []claudestream.StepStats{
			{SessionID: "valid-sid", InputTokens: 1_000},
			{},
		},
	}
	header := &fakeRunHeader{}
	kh := newTestKeyHandler()

	cfg := RunConfig{
		WorkflowDir: workflowDir,
		Iterations:  1,
		InitializeSteps: []steps.Step{
			resumeClaudeStep("init-step-1", "step1.txt", false),
			resumeClaudeStep("init-step-2", "step2.txt", false), // ResumePrevious=false
		},
	}

	Run(executor, header, kh, cfg)

	if len(executor.runSandboxedStepCalls) < 2 {
		t.Fatalf("want at least 2 RunSandboxedStep calls, got %d", len(executor.runSandboxedStepCalls))
	}
	cmd := executor.runSandboxedStepCalls[1].command
	for _, arg := range cmd {
		if arg == "--resume" {
			t.Errorf("step with ResumePrevious=false must not contain '--resume': %v", cmd)
		}
	}
}
