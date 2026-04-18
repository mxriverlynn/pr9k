package steps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	Command   []string `json:"command,omitempty"`
	CaptureAs string   `json:"captureAs,omitempty"`
	// CaptureMode controls how stdout is bound to the captureAs variable.
	// "" and "lastLine" both use the existing last-non-empty-line behavior.
	// "fullStdout" joins all stdout lines with "\n", capped at 32 KiB.
	CaptureMode      string `json:"captureMode,omitempty"`
	BreakLoopIfEmpty bool   `json:"breakLoopIfEmpty,omitempty"`
	// SkipIfCaptureEmpty names an iteration-phase variable (bound by an earlier
	// iteration step via captureAs) whose value is checked before this step runs.
	// If the value is empty and the capturing step completed successfully (StepDone),
	// this step is skipped without error and recorded as "skipped" in the iteration
	// log. Load-time constraints: must be non-empty when set, must reference a
	// captureAs bound earlier in the same iteration phase (initialize-phase captures
	// are not allowed), and is only valid in the iteration phase.
	SkipIfCaptureEmpty string `json:"skipIfCaptureEmpty,omitempty"`
	// TimeoutSeconds, when positive, limits the wall-clock time for this step.
	// On expiry the step is terminated (SIGTERM then SIGKILL after 10 s) via the
	// cidfile-driven Terminator path so Docker containers are cleaned up correctly.
	// Zero means no timeout (the default).
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
}

// StatusLineConfig holds the optional status-line configuration from ralph-steps.json.
// Populated by LoadSteps; not yet consumed by the TUI (wiring is a follow-up).
type StatusLineConfig struct {
	Type                   string `json:"type,omitempty"`
	Command                string `json:"command"`
	RefreshIntervalSeconds *int   `json:"refreshIntervalSeconds,omitempty"`
}

// StepFile holds the three groups of steps loaded from ralph-steps.json.
type StepFile struct {
	Env          []string          `json:"env,omitempty"`
	ContainerEnv map[string]string `json:"containerEnv,omitempty"`
	Initialize   []Step            `json:"initialize"`
	Iteration    []Step            `json:"iteration"`
	Finalize     []Step            `json:"finalize"`
	StatusLine   *StatusLineConfig `json:"statusLine,omitempty"`
}

// LoadSteps loads the step definitions from ralph-steps.json,
// resolved relative to workflowDir.
func LoadSteps(workflowDir string) (StepFile, error) {
	path := filepath.Join(workflowDir, "ralph-steps.json")
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
func BuildPrompt(workflowDir string, step Step, vt *vars.VarTable, phase vars.Phase) (string, error) {
	if step.PromptFile == "" {
		return "", fmt.Errorf("steps: PromptFile must not be empty")
	}
	promptPath := filepath.Join(workflowDir, "prompts", step.PromptFile)
	absPath, absErr := filepath.Abs(promptPath)
	absPrompts, absPromptsErr := filepath.Abs(filepath.Join(workflowDir, "prompts"))
	if absErr != nil || absPromptsErr != nil || !strings.HasPrefix(absPath, absPrompts+string(filepath.Separator)) {
		return "", fmt.Errorf("steps: prompt path escapes prompts directory: %s", step.PromptFile)
	}
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
