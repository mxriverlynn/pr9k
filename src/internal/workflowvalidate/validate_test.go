package workflowvalidate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/validator"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
	"github.com/mxriverlynn/pr9k/src/internal/workflowvalidate"
)

// tempProject creates a temp dir with prompts/ and scripts/ subdirectories.
func tempProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestValidate_BridgesToValidatorValidateDoc verifies end-to-end that the
// bridge delegates to validator.ValidateDoc, returning the same errors.
func TestValidate_BridgesToValidatorValidateDoc(t *testing.T) {
	dir := tempProject(t)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{
		"initialize": [],
		"iteration": [{"name":"s","isClaude":true,"model":"sonnet","promptFile":"p.md"}],
		"finalize": []
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Companion provides the prompt in memory.
	companions := map[string][]byte{"prompts/p.md": []byte("do work")}
	doc := workflowmodel.WorkflowDoc{}

	// Via bridge.
	bridgeErrs := workflowvalidate.Validate(doc, dir, companions)
	// Via validator directly.
	directErrs := validator.ValidateDoc(doc, dir, companions)

	if len(bridgeErrs) != len(directErrs) {
		t.Fatalf("bridge returned %d errors, direct returned %d; want equal\nbridge: %v\ndirect: %v",
			len(bridgeErrs), len(directErrs), bridgeErrs, directErrs)
	}
	for i := range directErrs {
		if bridgeErrs[i].Error() != directErrs[i].Error() {
			t.Errorf("error[%d] mismatch:\n  bridge: %s\n  direct: %s",
				i, bridgeErrs[i].Error(), directErrs[i].Error())
		}
	}
}

// TestValidate_PreservesErrorOrdering verifies that the bridge preserves the
// same error ordering as validator.ValidateDoc.
func TestValidate_PreservesErrorOrdering(t *testing.T) {
	dir := tempProject(t)
	// Config with multiple errors: missing isClaude, missing name, missing model.
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{
		"initialize": [],
		"iteration": [
			{"name":"","isClaude":true,"model":"","promptFile":""},
			{"name":"s2","isClaude":false,"command":[]}
		],
		"finalize": []
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := workflowmodel.WorkflowDoc{}

	bridgeErrs := workflowvalidate.Validate(doc, dir, nil)
	directErrs := validator.ValidateDoc(doc, dir, nil)

	if len(bridgeErrs) != len(directErrs) {
		t.Fatalf("bridge returned %d errors, direct returned %d", len(bridgeErrs), len(directErrs))
	}
	for i := range directErrs {
		if bridgeErrs[i].Error() != directErrs[i].Error() {
			t.Errorf("error ordering differs at [%d]:\n  bridge: %s\n  direct: %s",
				i, bridgeErrs[i].Error(), directErrs[i].Error())
		}
	}
}
