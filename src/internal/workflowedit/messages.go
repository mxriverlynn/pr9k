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
// rawBytes is non-nil only for parse errors (recovery view); other errors
// leave rawBytes nil, routing to DialogError instead of DialogRecovery.
type openFileResultMsg struct {
	doc         workflowmodel.WorkflowDoc
	diskDoc     workflowmodel.WorkflowDoc
	companions  map[string][]byte
	workflowDir string
	err         error
	rawBytes    []byte
}

// quitMsg is dispatched to signal a clean shutdown request. It is pre-dispatched
// in Update (tier-0) to prevent dialogs from swallowing a programmatic quit.
type quitMsg struct{}
