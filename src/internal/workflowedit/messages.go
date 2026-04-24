package workflowedit

import (
	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// validateCompleteMsg is dispatched when the async validation command finishes.
type validateCompleteMsg struct {
	items []findingResult
}

// saveCompleteMsg is dispatched when the async save command finishes.
type saveCompleteMsg struct {
	result workflowio.SaveResult
}

// openFileResultMsg is dispatched after an open-file attempt, carrying either
// the loaded document or an error + raw bytes for the recovery view.
type openFileResultMsg struct {
	doc         workflowmodel.WorkflowDoc
	diskDoc     workflowmodel.WorkflowDoc
	workflowDir string
	err         error
	rawBytes    []byte
}
