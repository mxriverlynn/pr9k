package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// varPattern matches {{VAR}} placeholders where VAR is an uppercase identifier.
var varPattern = regexp.MustCompile(`\{\{([A-Z_][A-Z0-9_]*)\}\}`)

const (
	phasePreLoop  = "pre-loop"
	phaseLoop     = "loop"
	phasePostLoop = "post-loop"
)

// varDecl records where an outputVariable was declared.
type varDecl struct {
	phase    string
	stepName string
	stepIdx  int
}

// ValidateVariables performs startup variable validation for a WorkflowConfig.
// It runs after structural validation and before any step executes. All errors
// are collected and returned as a single combined error.
func ValidateVariables(cfg *WorkflowConfig, projectDir string) error {
	// Build declaration map: variable name → where it was declared.
	decls := buildDeclMap(cfg)

	var errs []string

	phases := []struct {
		name  string
		steps []Step
	}{
		{phasePreLoop, cfg.PreLoop},
		{phaseLoop, cfg.Loop},
		{phasePostLoop, cfg.PostLoop},
	}

	for _, phase := range phases {
		for i, s := range phase.steps {
			// Collect variables reachable at this step.
			reachable := reachableAt(decls, phase.name, i)

			// Check shadowing: loop outputVariable must not duplicate a pre-loop outputVariable.
			if phase.name == phaseLoop && s.OutputVariable != "" {
				if d, ok := decls[s.OutputVariable]; ok && d.phase == phasePreLoop {
					errs = append(errs, fmt.Sprintf("step %q: outputVariable %q shadows pre-loop variable", s.Name, s.OutputVariable))
				}
			}

			if s.IsClaudeStep() {
				promptPath := filepath.Join(projectDir, "prompts", s.PromptFile)
				data, err := os.ReadFile(promptPath)
				if err != nil {
					errs = append(errs, fmt.Sprintf("step %q: could not read prompt file %s: %v", s.Name, promptPath, err))
					continue
				}
				promptText := string(data)
				promptVars := scanVars(promptText)

				// Build sets for bidirectional check.
				injectSet := make(map[string]bool, len(s.InjectVars))
				for _, v := range s.InjectVars {
					injectSet[v] = true
				}

				// injectVariables entry not found in prompt.
				for _, v := range s.InjectVars {
					if !promptVars[v] {
						errs = append(errs, fmt.Sprintf("step %q: injectVariables entry %q not found as {{%s}} in prompt file", s.Name, v, v))
					}
				}

				// {{VAR}} in prompt not listed in injectVariables.
				for v := range promptVars {
					if !injectSet[v] {
						errs = append(errs, fmt.Sprintf("step %q: {{%s}} in prompt file not listed in injectVariables", s.Name, v))
					}
				}

				// All injectVariables reference reachable variables.
				for _, v := range s.InjectVars {
					stepErrs := checkVarReachable(s.Name, v, decls, reachable, phase.name)
					errs = append(errs, stepErrs...)
				}
			}

			if s.IsCommandStep() {
				// All {{VAR}} in commands reference reachable variables.
				for _, arg := range s.Command {
					for _, v := range scanVarList(arg) {
						stepErrs := checkVarReachable(s.Name, v, decls, reachable, phase.name)
						errs = append(errs, stepErrs...)
					}
				}
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("steps: %s", strings.Join(errs, "; "))
	}
	return nil
}

// buildDeclMap returns a map from outputVariable name to where it was declared.
func buildDeclMap(cfg *WorkflowConfig) map[string]varDecl {
	decls := make(map[string]varDecl)
	phases := []struct {
		name  string
		steps []Step
	}{
		{phasePreLoop, cfg.PreLoop},
		{phaseLoop, cfg.Loop},
		{phasePostLoop, cfg.PostLoop},
	}
	for _, phase := range phases {
		for i, s := range phase.steps {
			if s.OutputVariable != "" {
				// First declaration wins for shadowing detection.
				if _, exists := decls[s.OutputVariable]; !exists {
					decls[s.OutputVariable] = varDecl{phase: phase.name, stepName: s.Name, stepIdx: i}
				} else if phase.name != phaseLoop {
					// Only loop shadowing of pre-loop is checked as an error;
					// for the decl map we keep the first occurrence.
				}
			}
		}
	}
	return decls
}

// reachableAt returns the set of variable names reachable at stepIdx within phaseName.
// Scoping rules:
//   - pre-loop vars are available in pre-loop (steps before declaring step), loop, post-loop
//   - loop vars are available in loop (steps before declaring step in the same iteration)
//   - post-loop vars are available in post-loop (steps before declaring step)
func reachableAt(decls map[string]varDecl, phaseName string, stepIdx int) map[string]bool {
	reachable := make(map[string]bool)
	for name, d := range decls {
		switch d.phase {
		case phasePreLoop:
			switch phaseName {
			case phasePreLoop:
				// Available only after the declaring step.
				if d.stepIdx < stepIdx {
					reachable[name] = true
				}
			case phaseLoop, phasePostLoop:
				reachable[name] = true
			}
		case phaseLoop:
			if phaseName == phaseLoop && d.stepIdx < stepIdx {
				reachable[name] = true
			}
			// Not available in post-loop.
		case phasePostLoop:
			if phaseName == phasePostLoop && d.stepIdx < stepIdx {
				reachable[name] = true
			}
		}
	}
	return reachable
}

// checkVarReachable validates that varName is reachable at the current step.
// It returns error strings for any violations found.
func checkVarReachable(stepName, varName string, decls map[string]varDecl, reachable map[string]bool, currentPhase string) []string {
	d, declared := decls[varName]
	if !declared {
		return []string{fmt.Sprintf("step %q: {{%s}} references undefined variable", stepName, varName)}
	}

	// Post-loop cannot reference loop-scoped variables.
	if currentPhase == phasePostLoop && d.phase == phaseLoop {
		return []string{fmt.Sprintf("step %q: references loop-scoped variable %q from post-loop", stepName, varName)}
	}

	// Forward reference within the same phase.
	if !reachable[varName] && d.phase == currentPhase {
		return []string{fmt.Sprintf("step %q: references variable %q declared by later step %q", stepName, varName, d.stepName)}
	}

	if !reachable[varName] {
		return []string{fmt.Sprintf("step %q: {{%s}} references undefined variable", stepName, varName)}
	}

	return nil
}

// scanVars returns the set of {{VAR}} names found in text.
func scanVars(text string) map[string]bool {
	matches := varPattern.FindAllStringSubmatch(text, -1)
	result := make(map[string]bool, len(matches))
	for _, m := range matches {
		result[m[1]] = true
	}
	return result
}

// scanVarList returns a slice of {{VAR}} names found in text (may contain duplicates).
func scanVarList(text string) []string {
	matches := varPattern.FindAllStringSubmatch(text, -1)
	result := make([]string, 0, len(matches))
	for _, m := range matches {
		result = append(result, m[1])
	}
	return result
}
