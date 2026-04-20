// Package vars owns runtime variable state for the pr9k workflow.
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
	"WORKFLOW_DIR": true,
	"PROJECT_DIR":  true,
	"MAX_ITER":     true,
	"ITER":         true,
	"STEP_NUM":     true,
	"STEP_COUNT":   true,
	"STEP_NAME":    true,
}

// VarTable holds runtime variable state for a single pr9k run.
//
// The persistent table is written by initialize-phase captureAs bindings and
// by the orchestrator for built-in variables. It is visible in all phases and
// is never cleared.
//
// The iteration table is written by iteration-phase captureAs bindings and is
// cleared at the start of each iteration via ResetIteration. It is not visible
// during the finalize phase.
//
// The finalize table is written by finalize-phase captureAs bindings. Finalize
// runs once, so its entries are never cleared. It is visible only during the
// finalize phase.
//
// Resolution order during an iteration step: iteration table → persistent table.
// During finalize: finalize table → persistent table.
// During initialize: persistent table only.
type VarTable struct {
	persistent map[string]string
	iteration  map[string]string
	finalize   map[string]string
	phase      Phase
}

// New creates a VarTable seeded with the built-in variables derived from
// the CLI flags. workflowDir is the resolved workflow bundle directory (install
// dir); projectDir is the resolved target repository directory; maxIter is the
// value of --iterations (0 means unbounded).
func New(workflowDir, projectDir string, maxIter int) *VarTable {
	vt := &VarTable{
		persistent: make(map[string]string),
		iteration:  make(map[string]string),
		finalize:   make(map[string]string),
		phase:      Initialize,
	}
	vt.persistent["WORKFLOW_DIR"] = workflowDir
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
// During Finalize: checks the finalize table first, then the persistent table.
// During Initialize: checks only the persistent table.
func (vt *VarTable) GetInPhase(phase Phase, name string) (string, bool) {
	switch phase {
	case Iteration:
		if v, ok := vt.iteration[name]; ok {
			return v, true
		}
	case Finalize:
		if v, ok := vt.finalize[name]; ok {
			return v, true
		}
	}
	v, ok := vt.persistent[name]
	return v, ok
}

// Bind records a captureAs variable binding for the given phase.
// For Initialize, the value is stored in the persistent table.
// For Iteration, the value is stored in the iteration table.
// For Finalize, the value is stored in the finalize table.
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
		vt.finalize[name] = value
	}
}

// ResetIteration clears the iteration-scoped variable table. Call this at the
// start of each new iteration before any iteration steps run.
func (vt *VarTable) ResetIteration() {
	vt.iteration = make(map[string]string)
}

// SetIteration rebinds the ITER built-in to the given iteration number n.
// The orchestrator is expected to call this at the start of each iteration;
// the method itself is phase-agnostic and safe to call in any phase.
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

// AllCaptures returns a defensive copy of all non-built-in variables visible
// in the given phase. During Iteration, both the iteration and persistent
// tables contribute (iteration entries shadow persistent ones). During
// Finalize, both the finalize and persistent tables contribute (finalize
// entries shadow persistent ones). During Initialize, only the persistent
// table is included. Reserved built-in names are excluded from the result.
func (vt *VarTable) AllCaptures(phase Phase) map[string]string {
	result := make(map[string]string)
	for k, v := range vt.persistent {
		if !reservedNames[k] {
			result[k] = v
		}
	}
	switch phase {
	case Iteration:
		for k, v := range vt.iteration {
			if !reservedNames[k] {
				result[k] = v
			}
		}
	case Finalize:
		for k, v := range vt.finalize {
			if !reservedNames[k] {
				result[k] = v
			}
		}
	}
	return result
}
