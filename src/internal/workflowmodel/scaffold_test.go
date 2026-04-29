package workflowmodel_test

import (
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

func TestEmpty_ProducesMinimalValidShape(t *testing.T) {
	doc := workflowmodel.Empty()
	if len(doc.Steps) != 1 {
		t.Fatalf("Empty: expected exactly 1 step, got %d", len(doc.Steps))
	}
	step := doc.Steps[0]
	if step.Name != "step-1" {
		t.Errorf("Empty: step.Name = %q, want %q", step.Name, "step-1")
	}
	if !step.IsClaudeSet {
		t.Error("Empty: step.IsClaudeSet should be true for placeholder step")
	}
	if step.Kind != workflowmodel.StepKindClaude {
		t.Errorf("Empty: step.Kind = %q, want %q", step.Kind, workflowmodel.StepKindClaude)
	}
	if step.PromptFile != "step-1.md" {
		t.Errorf("Empty: step.PromptFile = %q, want %q", step.PromptFile, "step-1.md")
	}
}

func TestEmpty_StepModelIsDefaultScaffoldModel(t *testing.T) {
	doc := workflowmodel.Empty()
	step := doc.Steps[0]
	if step.Model != workflowmodel.DefaultScaffoldModel {
		t.Errorf("Empty: step.Model = %q, want DefaultScaffoldModel %q",
			step.Model, workflowmodel.DefaultScaffoldModel)
	}
}

func TestCopyFromDefault_NonExistentBundlePath_ReturnsError(t *testing.T) {
	_, err := workflowmodel.CopyFromDefault("/does/not/exist")
	if err == nil {
		t.Error("CopyFromDefault: expected error for non-existent path, got nil")
	}
}
