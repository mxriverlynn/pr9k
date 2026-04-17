// Package statusline provides the stdin payload builder and ANSI sanitizer
// for the status-line script contract.
package statusline

// State is a copy-by-value snapshot of workflow state at the moment a
// status-line refresh is triggered. Safe to pass across goroutines; treat
// as immutable after construction.
type State struct {
	SessionID     string
	Version       string
	Phase         string
	Iteration     int
	MaxIterations int
	StepNum       int
	StepCount     int
	StepName      string
	WorkflowDir   string
	ProjectDir    string
	// Captures is a defensive copy of the VarTable snapshot visible in the
	// current phase.
	Captures map[string]string
}
