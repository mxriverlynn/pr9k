package workflowmodel_test

import (
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// TestParseConfig_Worktrees_RoundTrip verifies that a config with the
// worktrees block parses correctly into the WorkflowDoc.
func TestParseConfig_Worktrees_RoundTrip(t *testing.T) {
	t.Parallel()
	data := []byte(`{
		"initialize": [],
		"iteration": [{"name":"s1","isClaude":false,"command":["echo","ok"]}],
		"finalize": [],
		"worktrees": {"enabled": true, "autoCleanup": true}
	}`)
	doc, err := workflowmodel.ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if doc.Worktrees == nil {
		t.Fatal("expected Worktrees block to be set, got nil")
	}
	if !doc.Worktrees.Enabled {
		t.Error("expected Worktrees.Enabled == true")
	}
	if !doc.Worktrees.AutoCleanup {
		t.Error("expected Worktrees.AutoCleanup == true")
	}
}

// TestParseConfig_Worktrees_Absent verifies that a config without the
// worktrees block parses with Worktrees == nil (back-compat).
func TestParseConfig_Worktrees_Absent(t *testing.T) {
	t.Parallel()
	data := []byte(`{
		"initialize": [],
		"iteration": [{"name":"s1","isClaude":false,"command":["echo","ok"]}],
		"finalize": []
	}`)
	doc, err := workflowmodel.ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if doc.Worktrees != nil {
		t.Errorf("expected Worktrees to be nil when block absent, got %+v", doc.Worktrees)
	}
}

// TestParseConfig_Worktrees_DefaultsBothFalse verifies that an explicit
// {"enabled":false,"autoCleanup":false} block parses correctly.
func TestParseConfig_Worktrees_DefaultsBothFalse(t *testing.T) {
	t.Parallel()
	data := []byte(`{
		"initialize": [],
		"iteration": [{"name":"s1","isClaude":false,"command":["echo","ok"]}],
		"finalize": [],
		"worktrees": {"enabled": false, "autoCleanup": false}
	}`)
	doc, err := workflowmodel.ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if doc.Worktrees == nil {
		t.Fatal("expected Worktrees block to be set when explicitly present, got nil")
	}
	if doc.Worktrees.Enabled {
		t.Error("expected Worktrees.Enabled == false")
	}
	if doc.Worktrees.AutoCleanup {
		t.Error("expected Worktrees.AutoCleanup == false")
	}
}
