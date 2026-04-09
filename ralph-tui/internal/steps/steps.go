package steps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Step defines a single step in the ralph workflow.
type Step struct {
	Name           string   `json:"name"`
	Model          string   `json:"model,omitempty"`
	PromptFile     string   `json:"promptFile,omitempty"`
	PermissionMode string   `json:"permissionMode,omitempty"`
	InjectVars     []string `json:"injectVariables,omitempty"`
	// Command holds the argv for non-claude steps. Arguments may contain
	// template placeholders (e.g. "{{ISSUE_ID}}") that callers must substitute
	// before execution; the steps package does no expansion itself.
	Command         []string `json:"command,omitempty"`
	OutputVariable  string   `json:"outputVariable,omitempty"`
	ExitLoopIfEmpty bool     `json:"exitLoopIfEmpty,omitempty"`
}

// IsClaudeStep reports whether this step is a Claude prompt step.
func (s Step) IsClaudeStep() bool { return s.PromptFile != "" }

// IsCommandStep reports whether this step is a shell command step.
func (s Step) IsCommandStep() bool { return len(s.Command) > 0 }

// DefaultModel returns the step's model, or "sonnet" if unset.
func (s Step) DefaultModel() string {
	if s.Model != "" {
		return s.Model
	}
	return "sonnet"
}

// DefaultPermissionMode returns the step's permission mode, or "acceptEdits" if unset.
func (s Step) DefaultPermissionMode() string {
	if s.PermissionMode != "" {
		return s.PermissionMode
	}
	return "acceptEdits"
}

// WorkflowConfig holds the three-phase step configuration.
type WorkflowConfig struct {
	PreLoop  []Step `json:"pre-loop"`
	Loop     []Step `json:"loop"`
	PostLoop []Step `json:"post-loop"`
}

// LoadWorkflowConfig reads a three-phase workflow config from stepsFile (resolved
// relative to projectDir), validates its structure, and returns the config.
func LoadWorkflowConfig(projectDir, stepsFile string) (*WorkflowConfig, error) {
	path := filepath.Join(projectDir, stepsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("steps: could not read %s: %w", path, err)
	}

	var cfg WorkflowConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("steps: malformed JSON in %s: %w", path, err)
	}

	if err := validateStructure(&cfg); err != nil {
		return nil, err
	}

	if err := ValidateVariables(&cfg, projectDir); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validateStructure checks all steps across all three phases and returns a
// combined error listing every violation found.
func validateStructure(cfg *WorkflowConfig) error {
	phases := []struct {
		name   string
		steps  []Step
		isLoop bool
	}{
		{"pre-loop", cfg.PreLoop, false},
		{"loop", cfg.Loop, true},
		{"post-loop", cfg.PostLoop, false},
	}

	var errs []string
	for _, phase := range phases {
		for _, s := range phase.steps {
			// Rule 1: both promptFile and command
			if s.PromptFile != "" && len(s.Command) > 0 {
				errs = append(errs, fmt.Sprintf("step %q: has both promptFile and command", s.Name))
			}
			// Rule 2: neither promptFile nor command
			if s.PromptFile == "" && len(s.Command) == 0 {
				errs = append(errs, fmt.Sprintf("step %q: must have promptFile or command", s.Name))
			}
			// Rule 3: model without promptFile
			if s.Model != "" && s.PromptFile == "" {
				errs = append(errs, fmt.Sprintf("step %q: model requires promptFile", s.Name))
			}
			// Rule 4: injectVariables without promptFile
			if len(s.InjectVars) > 0 && s.PromptFile == "" {
				errs = append(errs, fmt.Sprintf("step %q: injectVariables requires promptFile", s.Name))
			}
			// Rule 5: permissionMode without promptFile
			if s.PermissionMode != "" && s.PromptFile == "" {
				errs = append(errs, fmt.Sprintf("step %q: permissionMode requires promptFile", s.Name))
			}
			// Rule 6: exitLoopIfEmpty without outputVariable
			if s.ExitLoopIfEmpty && s.OutputVariable == "" {
				errs = append(errs, fmt.Sprintf("step %q: exitLoopIfEmpty requires outputVariable", s.Name))
			}
			// Rule 7: exitLoopIfEmpty outside loop phase
			if s.ExitLoopIfEmpty && !phase.isLoop {
				errs = append(errs, fmt.Sprintf("step %q: exitLoopIfEmpty only valid in loop phase", s.Name))
			}
			// Rule 8: command array present but empty
			if s.Command != nil && len(s.Command) == 0 {
				errs = append(errs, fmt.Sprintf("step %q: command array must not be empty", s.Name))
			}
			// Rule 9: outputVariable on a Claude step
			if s.OutputVariable != "" && s.PromptFile != "" {
				errs = append(errs, fmt.Sprintf("step %q: outputVariable requires command, not promptFile", s.Name))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("steps: %s", strings.Join(errs, "; "))
	}
	return nil
}

// LoadSteps loads the iteration step definitions from configs/ralph-steps.json,
// resolved relative to projectDir.
func LoadSteps(projectDir string) ([]Step, error) {
	return loadStepsFile(filepath.Join(projectDir, "configs", "ralph-steps.json"))
}

// LoadFinalizeSteps loads the finalization step definitions from
// configs/ralph-finalize-steps.json, resolved relative to projectDir.
func LoadFinalizeSteps(projectDir string) ([]Step, error) {
	return loadStepsFile(filepath.Join(projectDir, "configs", "ralph-finalize-steps.json"))
}

// BuildReplacer creates a strings.Replacer that maps "{{KEY}}" to the
// corresponding value for each entry in vars. Substitution is single-pass, so
// a variable value that itself contains "{{OTHER}}" is never re-expanded
// (template injection safe).
func BuildReplacer(vars map[string]string) *strings.Replacer {
	pairs := make([]string, 0, len(vars)*2)
	for k, v := range vars {
		pairs = append(pairs, "{{"+k+"}}", v)
	}
	return strings.NewReplacer(pairs...)
}

// BuildPrompt reads the prompt file for the given step, applies single-pass
// "{{KEY}}" substitution from vars, and returns the result.
// Unrecognized "{{...}}" patterns are left as literal text.
// Substitution is single-pass, so a variable value containing "{{OTHER}}" is
// never re-expanded (template injection safe).
func BuildPrompt(projectDir string, step Step, vars map[string]string) (string, error) {
	if step.PromptFile == "" {
		return "", fmt.Errorf("steps: PromptFile must not be empty")
	}
	promptPath := filepath.Join(projectDir, "prompts", step.PromptFile)
	data, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("steps: could not read prompt %s: %w", promptPath, err)
	}

	return BuildReplacer(vars).Replace(string(data)), nil
}

func loadStepsFile(path string) ([]Step, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("steps: could not read %s: %w", path, err)
	}

	var steps []Step
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil, fmt.Errorf("steps: malformed JSON in %s: %w", path, err)
	}

	return steps, nil
}
