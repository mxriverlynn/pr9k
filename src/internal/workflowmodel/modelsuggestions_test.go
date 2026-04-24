package workflowmodel_test

import (
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

func TestModelSuggestions_DefaultScaffoldModelIsFirstEntry(t *testing.T) {
	if len(workflowmodel.ModelSuggestions) == 0 {
		t.Fatal("ModelSuggestions is empty")
	}
	if workflowmodel.ModelSuggestions[0] != workflowmodel.DefaultScaffoldModel {
		t.Errorf("ModelSuggestions[0] = %q, want DefaultScaffoldModel %q",
			workflowmodel.ModelSuggestions[0], workflowmodel.DefaultScaffoldModel)
	}
}
