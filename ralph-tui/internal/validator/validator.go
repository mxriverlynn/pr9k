// Package validator implements D13 config validation for ralph-steps.json.
// It covers all eight validation categories from the UX corrections design plan
// and returns a collected slice of structured errors — one per problem found.
// Validation runs in a single pass so all errors are visible before exit 1.
package validator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/vars"
)

// Error represents a single config validation failure.
// Category, Phase, and StepName identify where the problem was found;
// Problem is a human-readable description of what is wrong.
type Error struct {
	Category string
	Phase    string
	StepName string
	Problem  string
}

// Error implements the error interface. Step-level errors include the step name;
// file-level errors (no step name) omit it.
func (e Error) Error() string {
	if e.StepName == "" {
		return fmt.Sprintf("config error: %s: %s: %s", e.Category, e.Phase, e.Problem)
	}
	return fmt.Sprintf("config error: %s: %s step %q: %s", e.Category, e.Phase, e.StepName, e.Problem)
}

// vStep is the strict per-step struct used during validation.
// IsClaude uses *bool to distinguish absent (nil → error) from explicit false.
// CaptureAs uses *string to distinguish absent (nil → not set) from explicit
// empty string (pointer to "" → error).
type vStep struct {
	Name             string   `json:"name"`
	Model            string   `json:"model,omitempty"`
	PromptFile       string   `json:"promptFile,omitempty"`
	IsClaude         *bool    `json:"isClaude"`
	Command          []string `json:"command,omitempty"`
	CaptureAs        *string  `json:"captureAs,omitempty"`
	BreakLoopIfEmpty bool     `json:"breakLoopIfEmpty,omitempty"`
}

// vFile is the strict top-level struct.
// Each phase field uses *[]vStep so that a missing key (nil) is distinguished
// from an explicitly empty array (non-nil, len 0).
type vFile struct {
	Initialize *[]vStep `json:"initialize"`
	Iteration  *[]vStep `json:"iteration"`
	Finalize   *[]vStep `json:"finalize"`
}

// reservedBuiltins is the set of built-in variable names that captureAs bindings
// must not shadow.
var reservedBuiltins = map[string]bool{
	"WORKFLOW_DIR": true,
	"PROJECT_DIR":  true,
	"MAX_ITER":     true,
	"ITER":         true,
	"STEP_NUM":     true,
	"STEP_COUNT":   true,
	"STEP_NAME":    true,
}

// Validate loads ralph-steps.json from workflowDir and validates all D13
// categories. It returns all errors found; an empty slice means valid.
// Validation collects every error before returning — it does not stop at the
// first failure.
func Validate(workflowDir string) []Error {
	var errs []Error

	// Category 1 — file presence.
	path := filepath.Join(workflowDir, "ralph-steps.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return []Error{cfgErr("file", "config", "", fmt.Sprintf("could not read %s: %v", path, err))}
	}

	// Category 1 — parseability and no unknown fields (V6 Option A).
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var vf vFile
	if err := dec.Decode(&vf); err != nil {
		return []Error{cfgErr("parse", "config", "", fmt.Sprintf("malformed JSON in %s: %v", path, err))}
	}

	// Category 1 — required top-level array keys.
	if vf.Initialize == nil {
		errs = append(errs, cfgErr("file", "config", "", `missing required top-level array "initialize"`))
	}
	if vf.Iteration == nil {
		errs = append(errs, cfgErr("file", "config", "", `missing required top-level array "iteration"`))
	}
	if vf.Finalize == nil {
		errs = append(errs, cfgErr("file", "config", "", `missing required top-level array "finalize"`))
	}

	// Category 3 — iteration must have at least 1 step.
	if vf.Iteration != nil && len(*vf.Iteration) < 1 {
		errs = append(errs, cfgErr("phase-size", "iteration", "", "iteration array must have at least 1 step"))
	}

	// Without all three phases we cannot walk variable scopes.
	if vf.Initialize == nil || vf.Iteration == nil || vf.Finalize == nil {
		return errs
	}

	// Build the initialize-phase scope: WORKFLOW_DIR, PROJECT_DIR, MAX_ITER,
	// STEP_NUM, STEP_COUNT, STEP_NAME.  ITER is deliberately excluded — it is a
	// validation error if any initialize or finalize step references it.
	initScope := map[string]bool{
		"WORKFLOW_DIR": true,
		"PROJECT_DIR":  true,
		"MAX_ITER":     true,
		"STEP_NUM":     true,
		"STEP_COUNT":   true,
		"STEP_NAME":    true,
	}

	// Validate initialize; collect captureAs names for the persistent scope.
	initCaptures := validatePhase(workflowDir, vars.Initialize, "initialize", *vf.Initialize, initScope, &errs)

	// Persistent scope = initialize seeds + all captureAs from initialize.
	persistentScope := copyScope(initScope)
	for _, name := range initCaptures {
		persistentScope[name] = true
	}

	// Iteration scope = persistent + ITER.
	iterScope := copyScope(persistentScope)
	iterScope["ITER"] = true

	validatePhase(workflowDir, vars.Iteration, "iteration", *vf.Iteration, iterScope, &errs)

	// Finalize scope = persistent only (no ITER, no iteration captures).
	validatePhase(workflowDir, vars.Finalize, "finalize", *vf.Finalize, persistentScope, &errs)

	return errs
}

// validatePhase validates all steps in one phase and returns the captureAs names
// introduced by that phase (for persistent scope building).
func validatePhase(
	workflowDir string,
	phase vars.Phase,
	phaseName string,
	steps []vStep,
	initialScope map[string]bool,
	errs *[]Error,
) []string {
	seenNames := make(map[string]bool)
	seenCaptureAs := make(map[string]bool)
	scope := copyScope(initialScope)
	var captures []string

	for i, step := range steps {
		stepName := step.Name
		if stepName == "" {
			stepName = fmt.Sprintf("<unnamed step %d>", i)
		}

		at := func(category, problem string) Error {
			return cfgErr(category, phaseName, stepName, problem)
		}

		// Schema 2 — name must be non-empty.
		if step.Name == "" {
			*errs = append(*errs, at("schema", "name must not be empty"))
		}

		// Schema 6.1 — no duplicate names within the phase.
		if step.Name != "" {
			if seenNames[step.Name] {
				*errs = append(*errs, at("schema", fmt.Sprintf("duplicate step name %q in phase", step.Name)))
			}
			seenNames[step.Name] = true
		}

		// Schema 2 — isClaude is required; missing is an error (V6 Option A).
		if step.IsClaude == nil {
			*errs = append(*errs, at("schema", "isClaude is required"))
		}

		isClaude := step.IsClaude != nil && *step.IsClaude

		// Schema 2 — exactly one of {command, promptFile} must match isClaude.
		if step.IsClaude != nil {
			if isClaude {
				if step.PromptFile == "" {
					*errs = append(*errs, at("schema", "claude step must have a non-empty promptFile"))
				}
				if step.Model == "" {
					*errs = append(*errs, at("schema", "claude step must have a non-empty model"))
				}
				if len(step.Command) > 0 {
					*errs = append(*errs, at("schema", "claude step must not have command"))
				}
			} else {
				if len(step.Command) == 0 {
					*errs = append(*errs, at("schema", "non-claude step must have a non-empty command array"))
				}
				if step.PromptFile != "" {
					*errs = append(*errs, at("schema", "non-claude step must not have promptFile"))
				}
			}
		}

		// Schema 2 — captureAs: if set, must be non-empty and not shadow reserved names.
		// Schema 6.2 — no duplicate captureAs within the phase.
		if step.CaptureAs != nil {
			ca := *step.CaptureAs
			if ca == "" {
				*errs = append(*errs, at("schema", "captureAs must not be empty when set"))
			} else {
				if reservedBuiltins[ca] {
					*errs = append(*errs, at("schema", fmt.Sprintf("captureAs %q shadows reserved built-in variable", ca)))
				}
				if seenCaptureAs[ca] {
					*errs = append(*errs, at("schema", fmt.Sprintf("duplicate captureAs %q in phase", ca)))
				}
				seenCaptureAs[ca] = true
			}
		}

		// Schema 2 — breakLoopIfEmpty requires captureAs AND iteration phase.
		if step.BreakLoopIfEmpty {
			if step.CaptureAs == nil || *step.CaptureAs == "" {
				*errs = append(*errs, at("schema", "breakLoopIfEmpty requires captureAs to be set"))
			}
			if phase != vars.Iteration {
				*errs = append(*errs, at("schema", "breakLoopIfEmpty is only valid in the iteration phase"))
			}
		}

		// Category 4 — referenced files must exist.
		if step.IsClaude != nil {
			if isClaude && step.PromptFile != "" {
				promptPath := filepath.Join(workflowDir, "prompts", step.PromptFile)
				if _, err := os.Stat(promptPath); err != nil {
					*errs = append(*errs, at("file", fmt.Sprintf("prompt file %q not found", step.PromptFile)))
				}
			}
			if !isClaude && len(step.Command) > 0 {
				if msg := validateCommandPath(workflowDir, step.Command[0]); msg != "" {
					*errs = append(*errs, at("file", msg))
				}
			}
		}

		// Category 5 — variable references must be in scope.
		if step.IsClaude != nil {
			refs := extractStepRefs(workflowDir, step, isClaude)
			for _, ref := range refs {
				if !scope[ref] {
					*errs = append(*errs, at("variable", fmt.Sprintf("unresolved variable reference {{%s}}", ref)))
				}
			}
		}

		// Extend scope with this step's captureAs for subsequent steps.
		// Add to scope even if invalid (to reduce cascading errors), but only
		// track non-reserved first-time names in captures.
		if step.CaptureAs != nil && *step.CaptureAs != "" {
			ca := *step.CaptureAs
			if !scope[ca] {
				scope[ca] = true
				if !reservedBuiltins[ca] {
					captures = append(captures, ca)
				}
			}
		}
	}

	return captures
}

// validateCommandPath checks that cmd (command[0]) is resolvable.
// A path containing "/" is treated as relative (resolved under workflowDir) or
// absolute.  A bare name is looked up via exec.LookPath.
func validateCommandPath(workflowDir, cmd string) string {
	// Uses "/" as path separator; assumes Unix. Revise if Windows support is added.
	if strings.Contains(cmd, "/") {
		var resolved string
		if filepath.IsAbs(cmd) {
			resolved = cmd
		} else {
			resolved = filepath.Join(workflowDir, cmd)
		}
		if _, err := os.Stat(resolved); err != nil {
			return fmt.Sprintf("command %q not found at %s", cmd, resolved)
		}
		return ""
	}
	if _, err := exec.LookPath(cmd); err != nil {
		return fmt.Sprintf("command %q not found in PATH", cmd)
	}
	return ""
}

// extractStepRefs returns the variable names referenced by {{VAR}} tokens in
// the step's prompt file (for claude steps) or command arguments (for non-claude
// steps).  If the prompt file cannot be read, nil is returned — a missing file
// is already reported by category 4.
func extractStepRefs(workflowDir string, step vStep, isClaude bool) []string {
	if isClaude {
		if step.PromptFile == "" {
			return nil
		}
		data, err := os.ReadFile(filepath.Join(workflowDir, "prompts", step.PromptFile))
		if err != nil {
			return nil
		}
		return vars.ExtractReferences(string(data))
	}
	var refs []string
	for _, arg := range step.Command {
		refs = append(refs, vars.ExtractReferences(arg)...)
	}
	return refs
}

// cfgErr constructs a validation Error.
func cfgErr(category, phase, stepName, problem string) Error {
	return Error{Category: category, Phase: phase, StepName: stepName, Problem: problem}
}

// copyScope returns a shallow copy of a scope map.
func copyScope(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	maps.Copy(dst, src)
	return dst
}
