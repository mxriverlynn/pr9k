// Package workflowmodel defines the mutable in-memory representation of a
// workflow bundle that the TUI editor reads and writes. It has no dependencies
// on other pr9k internal packages.
package workflowmodel

import "encoding/json"

// StepKind identifies whether a step runs Claude or a shell command.
type StepKind string

const (
	StepKindClaude StepKind = "claude"
	StepKindShell  StepKind = "shell"
)

// EnvEntry represents one entry from the env or containerEnv section.
// When IsLiteral is false the entry is a passthrough name from the env array;
// Value is empty. When IsLiteral is true the entry is an explicit key-value
// pair from containerEnv.
type EnvEntry struct {
	Key       string
	Value     string
	IsLiteral bool
}

// StatusLineBlock holds the optional statusLine configuration block.
type StatusLineBlock struct {
	Type                   string
	Command                string
	RefreshIntervalSeconds int
}

// Step is one workflow step. IsClaudeSet distinguishes the three states:
//   - new/untyped step: Kind == "", IsClaudeSet == false
//   - shell step:       Kind == StepKindShell, IsClaudeSet == false
//   - claude step:      Kind == StepKindClaude, IsClaudeSet == true
type Step struct {
	Name               string
	Kind               StepKind
	IsClaudeSet        bool
	Model              string
	PromptFile         string
	Command            []string
	Env                []EnvEntry
	CaptureAs          string
	CaptureMode        string
	BreakLoopIfEmpty   bool
	SkipIfCaptureEmpty string
	TimeoutSeconds     int
	OnTimeout          string
	ResumePrevious     bool
}

// WorkflowDoc is the mutable in-memory representation of a config.json bundle.
// UnknownFields captures any JSON keys not mapped to typed fields; they are
// recorded on load and discarded on save.
type WorkflowDoc struct {
	DefaultModel  string
	StatusLine    *StatusLineBlock
	Steps         []Step
	UnknownFields map[string]json.RawMessage
}
