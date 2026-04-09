package steps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	Command     []string `json:"command,omitempty"`
	PrependVars bool     `json:"prependVars,omitempty"`
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

// BuildPrompt reads the prompt file for the given step and returns its content.
// When step.PrependVars is true, it prepends ISSUENUMBER and STARTINGSHA lines.
func BuildPrompt(projectDir string, step Step, issueID string, startingSHA string) (string, error) {
	if step.PromptFile == "" {
		return "", fmt.Errorf("steps: PromptFile must not be empty")
	}
	promptPath := filepath.Join(projectDir, "prompts", step.PromptFile)
	data, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("steps: could not read prompt %s: %w", promptPath, err)
	}

	content := string(data)
	if step.PrependVars {
		content = "ISSUENUMBER=" + issueID + "\nSTARTINGSHA=" + startingSHA + "\n" + content
	}
	return content, nil
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
