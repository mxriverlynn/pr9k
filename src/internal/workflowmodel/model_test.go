package workflowmodel_test

import (
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// TestStep_IsClaudeSet_DistinguishesNewFromShell verifies that the three
// step states (new/untyped, shell, claude) are unambiguously representable.
func TestStep_IsClaudeSet_DistinguishesNewFromShell(t *testing.T) {
	// New step: no kind chosen yet — both Kind and IsClaudeSet are zero values.
	newStep := workflowmodel.Step{
		Name:        "new-step",
		Kind:        "",
		IsClaudeSet: false,
	}

	// Shell step: explicitly set as shell.
	shellStep := workflowmodel.Step{
		Name:        "shell-step",
		Kind:        workflowmodel.StepKindShell,
		IsClaudeSet: false,
	}

	// Claude step: explicitly set as claude.
	claudeStep := workflowmodel.Step{
		Name:        "claude-step",
		Kind:        workflowmodel.StepKindClaude,
		IsClaudeSet: true,
	}

	// New step has empty Kind and false IsClaudeSet.
	if newStep.Kind != "" {
		t.Errorf("new step Kind = %q, want empty", newStep.Kind)
	}
	if newStep.IsClaudeSet {
		t.Error("new step IsClaudeSet = true, want false")
	}

	// Shell step has StepKindShell and false IsClaudeSet.
	if shellStep.Kind != workflowmodel.StepKindShell {
		t.Errorf("shell step Kind = %q, want %q", shellStep.Kind, workflowmodel.StepKindShell)
	}
	if shellStep.IsClaudeSet {
		t.Error("shell step IsClaudeSet = true, want false")
	}

	// Claude step has StepKindClaude and true IsClaudeSet.
	if claudeStep.Kind != workflowmodel.StepKindClaude {
		t.Errorf("claude step Kind = %q, want %q", claudeStep.Kind, workflowmodel.StepKindClaude)
	}
	if !claudeStep.IsClaudeSet {
		t.Error("claude step IsClaudeSet = false, want true")
	}

	// New and shell are distinguishable even though both have IsClaudeSet=false.
	if newStep.Kind == shellStep.Kind {
		t.Error("new step Kind == shell step Kind; they must be distinguishable")
	}
}
