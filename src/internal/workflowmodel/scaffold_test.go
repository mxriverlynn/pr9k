package workflowmodel_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// defaultBundlePath returns the workflow/ directory relative to this test file.
// Uses runtime.Caller so it is independent of the working directory when tests are run.
func defaultBundlePath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "workflow")
}

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

func TestCopyFromDefault_ReadsDefaultBundle(t *testing.T) {
	bundlePath := defaultBundlePath(t)
	doc, err := workflowmodel.CopyFromDefault(bundlePath)
	if err != nil {
		t.Fatalf("CopyFromDefault: unexpected error: %v", err)
	}
	if len(doc.Steps) == 0 {
		t.Error("CopyFromDefault: expected non-empty Steps from default bundle")
	}
}

func TestCopyFromDefault_InputImmutability(t *testing.T) {
	bundlePath := defaultBundlePath(t)

	doc1, err := workflowmodel.CopyFromDefault(bundlePath)
	if err != nil {
		t.Fatalf("CopyFromDefault first call: %v", err)
	}
	originalLen := len(doc1.Steps)

	// Mutate the first result.
	doc1.Steps = append(doc1.Steps, workflowmodel.Step{Name: "extra-injected"})

	// Second call must return an independent result unaffected by the mutation.
	doc2, err := workflowmodel.CopyFromDefault(bundlePath)
	if err != nil {
		t.Fatalf("CopyFromDefault second call: %v", err)
	}
	if len(doc2.Steps) != originalLen {
		t.Errorf("CopyFromDefault: second call returned %d steps, want %d (input mutation leaked)",
			len(doc2.Steps), originalLen)
	}
}
