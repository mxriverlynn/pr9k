package main

import (
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/version"
)

// TestVersion_WorkflowBuilderBumped verifies that the version has been
// incremented for the workflow-builder feature (WU-12 / issue #163).
// Version 0.7.1 was the pre-feature baseline; this test fails until the
// patch bump is committed.
func TestVersion_WorkflowBuilderBumped(t *testing.T) {
	if version.Version == "0.7.1" {
		t.Errorf("version %q: workflow-builder feature requires a patch bump; expected 0.7.2 or higher",
			version.Version)
	}
}
