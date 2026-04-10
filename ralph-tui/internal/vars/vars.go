// Package vars owns runtime variable state for the ralph-tui workflow.
// It maintains two scoped tables — persistent and iteration — plus a set of
// built-in variables seeded and updated by the orchestrator.
package vars

import (
	"fmt"
	"strconv"
)

// Phase identifies which workflow phase is active, controlling variable visibility.
type Phase int

const (
	// Initialize is the startup phase; captureAs bindings go into the persistent table.
	Initialize Phase = iota
	// Iteration is the repeating loop phase; captureAs bindings go into the iteration
	// table and are cleared at the start of each iteration via ResetIteration.
	Iteration
	// Finalize is the teardown phase; only the persistent table is visible.
	Finalize
)

// reservedNames is the set of built-in variable names that captureAs bindings
// must not overwrite. The VarTable enforces this as a defense-in-depth check;
// the validator (#40) is the primary enforcement point.
var reservedNames = map[string]bool{
	"PROJECT_DIR": true,
	"MAX_ITER":    true,
	"ITER":        true,
	"STEP_NUM":    true,
	"STEP_COUNT":  true,
	"STEP_NAME":   true,
}

// VarTable holds runtime variable state for a single ralph-tui run.
//
// The persistent table is written by initialize-phase captureAs bindings and
// by the orchestrator for built-in variables. It is visible in all phases and
// is never cleared.
//
// The iteration table is written by iteration-phase captureAs bindings and is
// cleared at the start of each iteration via ResetIteration. It is not visible
// during the finalize phase.
//
// Resolution order during an iteration step: iteration table → persistent table.
// During finalize (or initialize): persistent table only.
type VarTable struct {
	persistent map[string]string
	iteration  map[string]string
	phase      Phase
}

// New creates a VarTable seeded with the built-in variables derived from
// the CLI flags. projectDir is the resolved project directory; maxIter is the
// value of --iterations (0 means unbounded).
func New(projectDir string, maxIter int) *VarTable {
	vt := &VarTable{
		persistent: make(map[string]string),
		iteration:  make(map[string]string),
		phase:      Initialize,
	}
	vt.persistent["PROJECT_DIR"] = projectDir
	vt.persistent["MAX_ITER"] = strconv.Itoa(maxIter)
	return vt
}

// SetPhase updates the active phase, which controls the resolution order used
// by Get. Call this as the orchestrator transitions between workflow phases.
func (vt *VarTable) SetPhase(phase Phase) {
	vt.phase = phase
}

// Get looks up a variable using the current phase's resolution order.
// During Iteration: checks the iteration table first, then the persistent table.
// During Initialize or Finalize: checks only the persistent table.
// Returns the value and true if found; empty string and false if not.
func (vt *VarTable) Get(name string) (string, bool) {
	return vt.GetInPhase(vt.phase, name)
}

// GetInPhase looks up a variable using the resolution order for the given phase.
// During Iteration: checks the iteration table first, then the persistent table.
// During Initialize or Finalize: checks only the persistent table.
func (vt *VarTable) GetInPhase(phase Phase, name string) (string, bool) {
	if phase == Iteration {
		if v, ok := vt.iteration[name]; ok {
			return v, true
		}
	}
	v, ok := vt.persistent[name]
	return v, ok
}

// Bind records a captureAs variable binding for the given phase.
// For Initialize, the value is stored in the persistent table.
// For Iteration, the value is stored in the iteration table.
// For Finalize, captureAs bindings are not valid and Bind panics.
// Bind panics if name is a reserved built-in name.
func (vt *VarTable) Bind(phase Phase, name, value string) {
	if reservedNames[name] {
		panic(fmt.Sprintf("vars: attempt to bind reserved variable %q via captureAs", name))
	}
	switch phase {
	case Initialize:
		vt.persistent[name] = value
	case Iteration:
		vt.iteration[name] = value
	case Finalize:
		panic(fmt.Sprintf("vars: captureAs binding %q is not valid in the finalize phase", name))
	}
}

// ResetIteration clears the iteration-scoped variable table. Call this at the
// start of each new iteration before any iteration steps run.
func (vt *VarTable) ResetIteration() {
	vt.iteration = make(map[string]string)
}

// SetIteration rebinds the ITER built-in to the given iteration number n.
// This must only be called during the Iteration phase.
func (vt *VarTable) SetIteration(n int) {
	vt.persistent["ITER"] = strconv.Itoa(n)
}

// SetStep rebinds the STEP_NUM, STEP_COUNT, and STEP_NAME built-ins.
// Call this just before each step runs.
func (vt *VarTable) SetStep(num, count int, name string) {
	vt.persistent["STEP_NUM"] = strconv.Itoa(num)
	vt.persistent["STEP_COUNT"] = strconv.Itoa(count)
	vt.persistent["STEP_NAME"] = name
}
