// Package workflowvalidate is a thin bridge that the future workflow-builder
// TUI will call into instead of importing internal/validator directly. It
// converts workflowmodel.WorkflowDoc to the shape validator.ValidateDoc
// expects and delegates (D-4).
package workflowvalidate

import (
	"github.com/mxriverlynn/pr9k/src/internal/validator"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// Validate runs all D13 validation categories against doc and returns any
// errors found. workflowDir is the workflow bundle directory (the directory
// containing config.json, prompts/, and scripts/). companions is an optional
// map of in-memory file bytes keyed by path relative to workflowDir (e.g.,
// "prompts/step-1.md"); when a key is present its bytes are used instead of
// reading from disk.
func Validate(doc workflowmodel.WorkflowDoc, workflowDir string, companions map[string][]byte) []validator.Error {
	return validator.ValidateDoc(doc, workflowDir, companions)
}
