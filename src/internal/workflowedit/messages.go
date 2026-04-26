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
// The banner-signal fields (isSymlink, isExternal, isReadOnly, isSharedInstall)
// are forwarded from workflowio detection at load time (D-23, GAP-035).
type openFileResultMsg struct {
	doc         workflowmodel.WorkflowDoc
	diskDoc     workflowmodel.WorkflowDoc
	companions  map[string][]byte
	workflowDir string
	err         error
	rawBytes    []byte
	// Load-pipeline banner signals.
	isSymlink       bool
	symlinkTarget   string
	isExternal      bool
	isReadOnly      bool
	isSharedInstall bool
}

// quitMsg is dispatched to signal a clean shutdown request. It is pre-dispatched
// in Update (tier-0) to prevent dialogs from swallowing a programmatic quit.
type quitMsg struct{}

// clearSaveBannerMsg is dispatched via tea.Tick after a successful save to clear
// the transient save banner. The gen field guards against stale ticks: the
// handler clears m.saveBanner only when msg.gen == m.bannerGen (D-7).
type clearSaveBannerMsg struct {
	gen int
}

// clearBoundaryFlashMsg is dispatched via tea.Tick(150ms) after a phase-boundary
// decline to clear the cursor-row inversion. The seq field guards against stale
// ticks: the handler clears m.boundaryFlash only when msg.seq == m.boundaryFlash (D-12).
type clearBoundaryFlashMsg struct {
	seq uint64
}
