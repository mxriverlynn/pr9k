package steps_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
)

// makePromptDir creates a temp directory with prompt files and returns its path.
func makePromptDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	promptsDir := filepath.Join(dir, "prompts")
	if err := os.Mkdir(promptsDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(promptsDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// T1 — Valid config with pre-loop var used in loop command.
func TestValidateVariables_ValidConfig(t *testing.T) {
	dir := makePromptDir(t, nil)
	cfg := &steps.WorkflowConfig{
		PreLoop: []steps.Step{
			{Name: "GetUser", Command: []string{"get_gh_user"}, OutputVariable: "GH_USERNAME"},
		},
		Loop: []steps.Step{
			{Name: "Work", Command: []string{"do-work", "{{GH_USERNAME}}"}},
		},
	}
	if err := steps.ValidateVariables(cfg, dir); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// T2 — No-shadowing violation: loop outputVariable duplicates pre-loop outputVariable.
func TestValidateVariables_ShadowingViolation(t *testing.T) {
	dir := makePromptDir(t, nil)
	cfg := &steps.WorkflowConfig{
		PreLoop: []steps.Step{
			{Name: "SetFoo", Command: []string{"set-foo"}, OutputVariable: "FOO"},
		},
		Loop: []steps.Step{
			{Name: "LoopSetFoo", Command: []string{"loop-set-foo"}, OutputVariable: "FOO"},
		},
	}
	err := steps.ValidateVariables(cfg, dir)
	if err == nil {
		t.Fatal("expected shadowing error, got nil")
	}
	if !strings.Contains(err.Error(), "shadows pre-loop variable") {
		t.Errorf("expected shadowing error, got: %v", err)
	}
}

// T3 — injectVariables entry not found in prompt file.
func TestValidateVariables_InjectVarNotInPrompt(t *testing.T) {
	dir := makePromptDir(t, map[string]string{
		"work.txt": "do some work with no placeholders",
	})
	cfg := &steps.WorkflowConfig{
		Loop: []steps.Step{
			{Name: "Work", PromptFile: "work.txt", InjectVars: []string{"X"}},
		},
	}
	err := steps.ValidateVariables(cfg, dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `injectVariables entry "X" not found as {{X}} in prompt file`) {
		t.Errorf("unexpected error: %v", err)
	}
}

// T4 — {{VAR}} in prompt not listed in injectVariables.
func TestValidateVariables_PromptVarNotInInjectVars(t *testing.T) {
	dir := makePromptDir(t, map[string]string{
		"work.txt": "Use {{X}} to do work",
	})
	cfg := &steps.WorkflowConfig{
		Loop: []steps.Step{
			{Name: "Work", PromptFile: "work.txt", InjectVars: []string{}},
		},
	}
	err := steps.ValidateVariables(cfg, dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "{{X}} in prompt file not listed in injectVariables") {
		t.Errorf("unexpected error: %v", err)
	}
}

// T5 — Command {{VAR}} references undefined variable.
func TestValidateVariables_CommandRefUndefinedVar(t *testing.T) {
	dir := makePromptDir(t, nil)
	cfg := &steps.WorkflowConfig{
		Loop: []steps.Step{
			{Name: "Work", Command: []string{"do-work", "{{NOPE}}"}},
		},
	}
	err := steps.ValidateVariables(cfg, dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "{{NOPE}} references undefined variable") {
		t.Errorf("unexpected error: %v", err)
	}
}

// T6 — Post-loop references loop-scoped variable.
func TestValidateVariables_PostLoopRefLoopVar(t *testing.T) {
	dir := makePromptDir(t, map[string]string{
		"finalize.txt": "Finalize issue {{ISSUE_NUMBER}}",
	})
	cfg := &steps.WorkflowConfig{
		Loop: []steps.Step{
			{Name: "GetIssue", Command: []string{"get-issue"}, OutputVariable: "ISSUE_NUMBER"},
		},
		PostLoop: []steps.Step{
			{Name: "Finalize", PromptFile: "finalize.txt", InjectVars: []string{"ISSUE_NUMBER"}},
		},
	}
	err := steps.ValidateVariables(cfg, dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `references loop-scoped variable "ISSUE_NUMBER" from post-loop`) {
		t.Errorf("unexpected error: %v", err)
	}
}

// T7 — Forward reference within a phase: step 2 in loop uses var defined by step 3.
func TestValidateVariables_ForwardReferenceWithinPhase(t *testing.T) {
	dir := makePromptDir(t, nil)
	cfg := &steps.WorkflowConfig{
		Loop: []steps.Step{
			{Name: "UseA", Command: []string{"use", "{{A}}"}},
			{Name: "DefineA", Command: []string{"define-a"}, OutputVariable: "A"},
		},
	}
	err := steps.ValidateVariables(cfg, dir)
	if err == nil {
		t.Fatal("expected forward reference error, got nil")
	}
	if !strings.Contains(err.Error(), `references variable "A" declared by later step`) {
		t.Errorf("unexpected error: %v", err)
	}
}

// T8 — Pre-loop variable is available in loop command.
func TestValidateVariables_PreLoopVarAvailableInLoop(t *testing.T) {
	dir := makePromptDir(t, nil)
	cfg := &steps.WorkflowConfig{
		PreLoop: []steps.Step{
			{Name: "SetX", Command: []string{"set-x"}, OutputVariable: "X"},
		},
		Loop: []steps.Step{
			{Name: "UseX", Command: []string{"use", "{{X}}"}},
		},
	}
	if err := steps.ValidateVariables(cfg, dir); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// T9 — Pre-loop variable is available in post-loop.
func TestValidateVariables_PreLoopVarAvailableInPostLoop(t *testing.T) {
	dir := makePromptDir(t, map[string]string{
		"post.txt": "Hello {{X}}",
	})
	cfg := &steps.WorkflowConfig{
		PreLoop: []steps.Step{
			{Name: "SetX", Command: []string{"set-x"}, OutputVariable: "X"},
		},
		PostLoop: []steps.Step{
			{Name: "UseX", PromptFile: "post.txt", InjectVars: []string{"X"}},
		},
	}
	if err := steps.ValidateVariables(cfg, dir); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// T10 — Loop variable available to a later loop step.
func TestValidateVariables_LoopVarAvailableToLaterLoopStep(t *testing.T) {
	dir := makePromptDir(t, nil)
	cfg := &steps.WorkflowConfig{
		Loop: []steps.Step{
			{Name: "DefA", Command: []string{"def-a"}, OutputVariable: "A"},
			{Name: "Intermediate", Command: []string{"intermediate"}},
			{Name: "UseA", Command: []string{"use", "{{A}}"}},
		},
	}
	if err := steps.ValidateVariables(cfg, dir); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// T11 — Multiple errors are all collected.
func TestValidateVariables_MultipleErrorsCollected(t *testing.T) {
	dir := makePromptDir(t, map[string]string{
		"work.txt": "Use {{MISSING_INJECT}}",
	})
	cfg := &steps.WorkflowConfig{
		PreLoop: []steps.Step{
			{Name: "SetFoo", Command: []string{"set-foo"}, OutputVariable: "FOO"},
		},
		Loop: []steps.Step{
			// Error 1: shadows pre-loop FOO
			{Name: "ShadowFoo", Command: []string{"shadow"}, OutputVariable: "FOO"},
			// Error 2: {{NOPE}} is undefined
			{Name: "BadCmd", Command: []string{"cmd", "{{NOPE}}"}},
			// Error 3: prompt has {{MISSING_INJECT}} but injectVariables is empty
			{Name: "BadPrompt", PromptFile: "work.txt", InjectVars: []string{}},
		},
	}
	err := steps.ValidateVariables(cfg, dir)
	if err == nil {
		t.Fatal("expected multiple errors, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "shadows pre-loop variable") {
		t.Errorf("missing shadowing error in: %v", msg)
	}
	if !strings.Contains(msg, "{{NOPE}} references undefined variable") {
		t.Errorf("missing undefined variable error in: %v", msg)
	}
	if !strings.Contains(msg, "{{MISSING_INJECT}} in prompt file not listed in injectVariables") {
		t.Errorf("missing inject-var error in: %v", msg)
	}
}
