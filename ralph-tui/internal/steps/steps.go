package steps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/vars"
)

// Step defines a single step in the ralph workflow.
type Step struct {
	Name       string `json:"name"`
	Model      string `json:"model,omitempty"`
	PromptFile string `json:"promptFile,omitempty"`
	IsClaude   bool   `json:"isClaude"`
	// Command holds the argv for non-claude steps. Arguments may contain
	// template placeholders (e.g. "{{ISSUE_ID}}") that callers must substitute
	// before execution; the steps package does no expansion itself.
	Command          []string `json:"command,omitempty"`
	CaptureAs        string   `json:"captureAs,omitempty"`
	BreakLoopIfEmpty bool     `json:"breakLoopIfEmpty,omitempty"`
}

// StepFile holds the three groups of steps loaded from ralph-steps.json.
type StepFile struct {
	Initialize []Step `json:"initialize"`
	Iteration  []Step `json:"iteration"`
	Finalize   []Step `json:"finalize"`
}

// LoadSteps loads the step definitions from ralph-steps.json,
// resolved relative to projectDir.
func LoadSteps(projectDir string) (StepFile, error) {
	path := filepath.Join(projectDir, "ralph-steps.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return StepFile{}, fmt.Errorf("steps: could not read %s: %w", path, err)
	}

	var sf StepFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return StepFile{}, fmt.Errorf("steps: malformed JSON in %s: %w", path, err)
	}

	return sf, nil
}

// BuildPrompt reads the prompt file for the given step, substitutes {{VAR}}
// tokens using vt and phase, and returns the result.
func BuildPrompt(projectDir string, step Step, vt *vars.VarTable, phase vars.Phase) (string, error) {
	if step.PromptFile == "" {
		return "", fmt.Errorf("steps: PromptFile must not be empty")
	}
	promptPath := filepath.Join(projectDir, "prompts", step.PromptFile)
	data, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("steps: could not read prompt %s: %w", promptPath, err)
	}

	content, err := vars.Substitute(string(data), vt, phase)
	if err != nil {
		return "", fmt.Errorf("steps: substitution failed in prompt %s: %w", promptPath, err)
	}
	return content, nil
}
