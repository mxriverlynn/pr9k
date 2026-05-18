// Package interactionchannel provides the Phase 2 IPC surface between the
// orchestrator pane and the three display panes. Transport is a Unix domain
// socket with length-prefixed JSON framing.
package interactionchannel

// Message is the interface implemented by every wire message type.
// The WireType discriminator is embedded as a "type" JSON field in every
// serialized message for forward-compatibility.
type Message interface {
	// WireType returns the string written to the "type" JSON field.
	WireType() string
	// sentinel prevents external packages from implementing this interface.
	sealed()
}

// IntentType names the intent a pane forwards to the orchestrator.
type IntentType string

const (
	IntentRetry    IntentType = "retry"
	IntentContinue IntentType = "continue"
	IntentQuit     IntentType = "quit"
	IntentSkip     IntentType = "skip"
	IntentNext     IntentType = "next"
)

// --- pane → orchestrator ---

// Ready is sent by a display pane immediately after connecting. Semantic
// handling (readiness tracking, handshake completion) is deferred to U2.
type Ready struct {
	Role string `json:"role"`
}

func (Ready) WireType() string { return "ready" }
func (Ready) sealed()          {}

// Intent carries a user-initiated action from the footer pane to the
// orchestrator. Semantic forwarding to the workflow loop is deferred to U5.
type Intent struct {
	Kind IntentType `json:"kind"`
}

func (Intent) WireType() string { return "intent" }
func (Intent) sealed()          {}

// DoneAck is sent by a display pane after it receives WorkspaceDone and
// has transitioned to its final-state render. Semantic handling is deferred
// to U9.
type DoneAck struct {
	Role string `json:"role"`
}

func (DoneAck) WireType() string { return "done_ack" }
func (DoneAck) sealed()          {}

// --- orchestrator → pane ---

// StateHeader carries all data the header pane needs to render one frame.
// StepStates values are the integer representations of ui.StepState. The
// interactionchannel package uses raw ints to avoid a cyclic import with
// internal/ui.
type StateHeader struct {
	IterationLine string   `json:"iteration_line"`
	StepNames     []string `json:"step_names"`
	StepStates    []int    `json:"step_states"`
}

func (StateHeader) WireType() string { return "state_header" }
func (StateHeader) sealed()          {}

// StateLog carries a batch of log lines to the log pane. Each element of
// Lines is a pre-rendered (ANSI-bearing) byte slice.
type StateLog struct {
	Lines [][]byte `json:"lines"`
}

func (StateLog) WireType() string { return "state_log" }
func (StateLog) sealed()          {}

// StateFooter carries the keyboard mode and shortcut bar text to the footer
// pane. Mode is the integer representation of ui.Mode.
type StateFooter struct {
	Mode         int    `json:"mode"`
	ShortcutLine string `json:"shortcut_line"`
}

func (StateFooter) WireType() string { return "state_footer" }
func (StateFooter) sealed()          {}

// WorkspaceDone signals that the orchestrator has finished the workflow run.
// The ExitCode mirrors the process exit code the orchestrator will use.
// Semantic handling (DoneAck tracking, socket teardown) is deferred to U9.
type WorkspaceDone struct {
	ExitCode int `json:"exit_code"`
}

func (WorkspaceDone) WireType() string { return "workspace_done" }
func (WorkspaceDone) sealed()          {}

// Compile-time assertions that every concrete type satisfies Message.
var (
	_ Message = Ready{}
	_ Message = Intent{}
	_ Message = DoneAck{}
	_ Message = StateHeader{}
	_ Message = StateLog{}
	_ Message = StateFooter{}
	_ Message = WorkspaceDone{}
)
