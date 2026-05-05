package steps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mxriverlynn/pr9k/src/internal/vars"
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
	// OnTimeout selects the per-step policy applied when TimeoutSeconds fires.
	// Accepted values:
	//   ""         same as "fail" — current behavior: enter error mode and block
	//              on user input (c/r/q) for the failed step.
	//   "fail"     explicit form of the default.
	//   "continue" soft-fail: log the timeout, advance to the next step without
	//              prompting, and render the step with the StepTimedOutContinuing
	//              glyph ("[!]"). Intended for unattended runs where the work was
	//              already partially completed before the timer fired.
	// Only meaningful when TimeoutSeconds > 0.
	OnTimeout string `json:"onTimeout,omitempty"`
	// ResumePrevious, when true, requests that this claude step resume the
	// previous claude step's session via --resume <session_id>. Five runtime
	// gates (G1–G5) must all pass; any failure logs the blocking gate and falls
	// through to a fresh session instead of aborting. Only valid on claude steps.
	// The default workflow ships with this field unset on all steps — engine
	// support is present but feature-flagged-off pending Phase C A/B validation.
	ResumePrevious bool `json:"resumePrevious,omitempty"`
	// Effort, when set on a claude step, is forwarded to the claude CLI as
	// "--effort <value>". Valid values: "low", "medium", "high", "xhigh", "max".
	// Empty means the flag is not passed at all (the CLI's own default applies).
	// Only valid on claude steps.
	Effort string `json:"effort,omitempty"`
}

// ValidEffortValues lists the accepted values for Step.Effort. Exported so
// the validator can share the same source of truth.
var ValidEffortValues = []string{"low", "medium", "high", "xhigh", "max"}

// IsValidEffort reports whether v is a valid Step.Effort value. Empty is valid
// (means: do not pass --effort to the CLI).
func IsValidEffort(v string) bool {
	if v == "" {
		return true
	}
	return slices.Contains(ValidEffortValues, v)
}

// StatusLineConfig holds the optional status-line configuration from config.json.
// Populated by LoadSteps; not yet consumed by the TUI (wiring is a follow-up).
type StatusLineConfig struct {
	Type                   string `json:"type,omitempty"`
	Command                string `json:"command"`
	RefreshIntervalSeconds *int   `json:"refreshIntervalSeconds,omitempty"`
}

// Defaults holds the optional top-level "defaults" block. Each field is applied
// to claude steps that do not set the corresponding step-level value.
type Defaults struct {
	// Effort is the default value forwarded to the claude CLI as "--effort <v>"
	// for claude steps that do not set their own Effort. Valid values match
	// ValidEffortValues. Empty means no default; the CLI's own default applies
	// unless a step sets its own Effort.
	Effort string `json:"effort,omitempty"`
	// Model is the default Claude model name applied to claude steps that do not
	// set their own Model. The value is passed through verbatim to "claude
	// --model". Empty means no default; in that case every claude step must set
	// its own Model.
	Model string `json:"model,omitempty"`
}

// StepFile holds the three groups of steps loaded from config.json.
type StepFile struct {
	Env          []string          `json:"env,omitempty"`
	ContainerEnv map[string]string `json:"containerEnv,omitempty"`
	Defaults     *Defaults         `json:"defaults,omitempty"`
	Initialize   []Step            `json:"initialize"`
	Iteration    []Step            `json:"iteration"`
	Finalize     []Step            `json:"finalize"`
	StatusLine   *StatusLineConfig `json:"statusLine,omitempty"`
}

// EffectiveEffort returns the effort to use for s, applying the StepFile's
// defaults when the step does not set its own Effort. Returns "" when neither
// is set (i.e., do not pass --effort to the CLI).
func (sf StepFile) EffectiveEffort(s Step) string {
	if s.Effort != "" {
		return s.Effort
	}
	if sf.Defaults != nil {
		return sf.Defaults.Effort
	}
	return ""
}

// EffectiveModel returns the model to use for s, applying the StepFile's
// defaults when the step does not set its own Model. Returns "" when neither
// is set; the validator rejects that combination for claude steps before
// runtime ever sees an unset effective model.
func (sf StepFile) EffectiveModel(s Step) string {
	if s.Model != "" {
		return s.Model
	}
	if sf.Defaults != nil {
		return sf.Defaults.Model
	}
	return ""
}

// LoadSteps loads the step definitions from config.json,
// resolved relative to workflowDir.
func LoadSteps(workflowDir string) (StepFile, error) {
	path := filepath.Join(workflowDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return StepFile{}, fmt.Errorf("steps: could not read %s: %w", path, err)
	}

	var sf StepFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return StepFile{}, fmt.Errorf("steps: malformed JSON in %s: %w", path, err)
	}

	// SUGG-003: Reject negative TimeoutSeconds at load time so that a broken
	// validator call chain cannot silently treat a negative value as "no timeout"
	// (the runtime guard `if timeoutSeconds > 0` would pass through 0 but reject
	// negatives anyway — this makes the failure explicit and early).
	for _, group := range [][]Step{sf.Initialize, sf.Iteration, sf.Finalize} {
		for _, s := range group {
			if s.TimeoutSeconds < 0 {
				return StepFile{}, fmt.Errorf("steps: step %q: timeoutSeconds must not be negative", s.Name)
			}
			if !IsValidEffort(s.Effort) {
				return StepFile{}, fmt.Errorf("steps: step %q: effort %q is not valid (use one of %v)", s.Name, s.Effort, ValidEffortValues)
			}
		}
	}
	if sf.Defaults != nil && !IsValidEffort(sf.Defaults.Effort) {
		return StepFile{}, fmt.Errorf("steps: defaults.effort %q is not valid (use one of %v)", sf.Defaults.Effort, ValidEffortValues)
	}

	// Apply top-level defaults: claude steps that do not set their own value
	// inherit from the defaults block. Non-claude steps are left alone — the
	// fields are meaningless for them and the validator will reject any
	// explicit value on them.
	if sf.Defaults != nil {
		if sf.Defaults.Effort != "" {
			applyDefaultEffort(sf.Initialize, sf.Defaults.Effort)
			applyDefaultEffort(sf.Iteration, sf.Defaults.Effort)
			applyDefaultEffort(sf.Finalize, sf.Defaults.Effort)
		}
		if sf.Defaults.Model != "" {
			applyDefaultModel(sf.Initialize, sf.Defaults.Model)
			applyDefaultModel(sf.Iteration, sf.Defaults.Model)
			applyDefaultModel(sf.Finalize, sf.Defaults.Model)
		}
	}

	return sf, nil
}

// applyDefaultEffort fills in Effort on claude steps that did not set it.
func applyDefaultEffort(group []Step, defaultEffort string) {
	for i := range group {
		if group[i].IsClaude && group[i].Effort == "" {
			group[i].Effort = defaultEffort
		}
	}
}

// applyDefaultModel fills in Model on claude steps that did not set it.
func applyDefaultModel(group []Step, defaultModel string) {
	for i := range group {
		if group[i].IsClaude && group[i].Model == "" {
			group[i].Model = defaultModel
		}
	}
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
