package workflowmodel_test

import (
	"encoding/json"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

func TestIsDirty_IdenticalDocs_False(t *testing.T) {
	doc := workflowmodel.WorkflowDoc{
		DefaultModel: "claude-sonnet-4-6",
		Steps: []workflowmodel.Step{
			{Name: "step-1", Kind: workflowmodel.StepKindClaude, IsClaudeSet: true},
		},
	}
	if workflowmodel.IsDirty(doc, doc) {
		t.Error("IsDirty: identical docs should not be dirty")
	}
}

func TestIsDirty_StepAdded_True(t *testing.T) {
	disk := workflowmodel.WorkflowDoc{}
	mem := workflowmodel.WorkflowDoc{
		Steps: []workflowmodel.Step{
			{Name: "step-1"},
		},
	}
	if !workflowmodel.IsDirty(disk, mem) {
		t.Error("IsDirty: adding a step should mark dirty")
	}
}

func TestIsDirty_StepRemoved_True(t *testing.T) {
	disk := workflowmodel.WorkflowDoc{
		Steps: []workflowmodel.Step{
			{Name: "step-1"},
		},
	}
	mem := workflowmodel.WorkflowDoc{}
	if !workflowmodel.IsDirty(disk, mem) {
		t.Error("IsDirty: removing a step should mark dirty")
	}
}

func TestIsDirty_StepFieldChanged_True(t *testing.T) {
	tests := []struct {
		name string
		disk workflowmodel.Step
		mem  workflowmodel.Step
	}{
		{
			name: "Name",
			disk: workflowmodel.Step{Name: "step-1"},
			mem:  workflowmodel.Step{Name: "step-renamed"},
		},
		{
			name: "Kind",
			disk: workflowmodel.Step{Name: "s", Kind: workflowmodel.StepKindClaude, IsClaudeSet: true},
			mem:  workflowmodel.Step{Name: "s", Kind: workflowmodel.StepKindShell, IsClaudeSet: false},
		},
		{
			name: "Model",
			disk: workflowmodel.Step{Name: "s", Kind: workflowmodel.StepKindClaude, IsClaudeSet: true, Model: "opus"},
			mem:  workflowmodel.Step{Name: "s", Kind: workflowmodel.StepKindClaude, IsClaudeSet: true, Model: "sonnet"},
		},
		{
			name: "PromptFile",
			disk: workflowmodel.Step{Name: "s", PromptFile: "a.md"},
			mem:  workflowmodel.Step{Name: "s", PromptFile: "b.md"},
		},
		{
			name: "Command",
			disk: workflowmodel.Step{Name: "s", Command: []string{"echo", "hi"}},
			mem:  workflowmodel.Step{Name: "s", Command: []string{"echo", "bye"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			disk := workflowmodel.WorkflowDoc{Steps: []workflowmodel.Step{tc.disk}}
			mem := workflowmodel.WorkflowDoc{Steps: []workflowmodel.Step{tc.mem}}
			if !workflowmodel.IsDirty(disk, mem) {
				t.Errorf("IsDirty: changing Step.%s should mark dirty", tc.name)
			}
		})
	}
}

func TestIsDirty_UnknownFieldsIgnored(t *testing.T) {
	disk := workflowmodel.WorkflowDoc{
		UnknownFields: map[string]json.RawMessage{
			"foo": json.RawMessage(`"bar"`),
		},
	}
	mem := workflowmodel.WorkflowDoc{
		UnknownFields: map[string]json.RawMessage{
			"baz": json.RawMessage(`"qux"`),
		},
	}
	if workflowmodel.IsDirty(disk, mem) {
		t.Error("IsDirty: differences in UnknownFields should not mark dirty")
	}
}
