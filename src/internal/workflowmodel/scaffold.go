package workflowmodel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Empty returns a minimal WorkflowDoc with a single placeholder iteration step.
// The prompt file referenced by the placeholder does not exist on disk at
// scaffold creation time; it is created on first save or first editor open.
func Empty() WorkflowDoc {
	return WorkflowDoc{
		Steps: []Step{
			{
				Name:        "step-1",
				Phase:       StepPhaseIteration,
				Kind:        StepKindClaude,
				IsClaudeSet: true,
				Model:       DefaultScaffoldModel,
				PromptFile:  "step-1.md",
			},
		},
	}
}

// CopyFromDefault reads config.json from bundlePath and returns a WorkflowDoc
// populated from it. All three phases (initialize, iteration, finalize) are
// flattened into the Steps field in order: initialize → iteration → finalize.
// The caller supplies the bundle path; it is not cached between calls.
func CopyFromDefault(bundlePath string) (WorkflowDoc, error) {
	configPath := filepath.Join(bundlePath, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return WorkflowDoc{}, fmt.Errorf("workflowmodel: read default bundle %s: %w", configPath, err)
	}
	return parseConfig(data)
}

// rawStep is the JSON shape of a single step in config.json.
type rawStep struct {
	Name               string   `json:"name"`
	Model              string   `json:"model,omitempty"`
	PromptFile         string   `json:"promptFile,omitempty"`
	IsClaude           *bool    `json:"isClaude"`
	Command            []string `json:"command,omitempty"`
	CaptureAs          string   `json:"captureAs,omitempty"`
	CaptureMode        string   `json:"captureMode,omitempty"`
	BreakLoopIfEmpty   bool     `json:"breakLoopIfEmpty,omitempty"`
	SkipIfCaptureEmpty string   `json:"skipIfCaptureEmpty,omitempty"`
	TimeoutSeconds     int      `json:"timeoutSeconds,omitempty"`
	OnTimeout          string   `json:"onTimeout,omitempty"`
	ResumePrevious     bool     `json:"resumePrevious,omitempty"`
	Effort             string   `json:"effort,omitempty"`
}

// rawStatusLine is the JSON shape of the statusLine block in config.json.
type rawStatusLine struct {
	Type                   string `json:"type,omitempty"`
	Command                string `json:"command"`
	RefreshIntervalSeconds *int   `json:"refreshIntervalSeconds,omitempty"`
}

// rawDefaults is the JSON shape of the top-level "defaults" block.
type rawDefaults struct {
	Effort string `json:"effort,omitempty"`
	Model  string `json:"model,omitempty"`
}

// rawConfig is the JSON shape of config.json.
type rawConfig struct {
	Initialize   []rawStep         `json:"initialize"`
	Iteration    []rawStep         `json:"iteration"`
	Finalize     []rawStep         `json:"finalize"`
	StatusLine   *rawStatusLine    `json:"statusLine,omitempty"`
	Defaults     *rawDefaults      `json:"defaults,omitempty"`
	Env          []string          `json:"env,omitempty"`
	ContainerEnv map[string]string `json:"containerEnv,omitempty"`
}

// ParseConfig parses config.json bytes into a WorkflowDoc. Exported so
// workflowio can parse raw bytes it has already read (for recovery-view support).
func ParseConfig(data []byte) (WorkflowDoc, error) { return parseConfig(data) }

func parseConfig(data []byte) (WorkflowDoc, error) {
	var rc rawConfig
	if err := json.Unmarshal(data, &rc); err != nil {
		return WorkflowDoc{}, fmt.Errorf("workflowmodel: parse config.json: %w", err)
	}

	var steps []Step
	for _, rs := range rc.Initialize {
		steps = append(steps, convertStep(rs, StepPhaseInitialize))
	}
	for _, rs := range rc.Iteration {
		steps = append(steps, convertStep(rs, StepPhaseIteration))
	}
	for _, rs := range rc.Finalize {
		steps = append(steps, convertStep(rs, StepPhaseFinalize))
	}

	doc := WorkflowDoc{Steps: steps, Env: rc.Env, ContainerEnv: rc.ContainerEnv}
	if rc.StatusLine != nil {
		sl := &StatusLineBlock{
			Type:    rc.StatusLine.Type,
			Command: rc.StatusLine.Command,
		}
		if rc.StatusLine.RefreshIntervalSeconds != nil {
			sl.RefreshIntervalSeconds = *rc.StatusLine.RefreshIntervalSeconds
		}
		doc.StatusLine = sl
	}
	if rc.Defaults != nil {
		doc.Defaults = &DefaultsBlock{
			Effort: rc.Defaults.Effort,
			Model:  rc.Defaults.Model,
		}
	}
	return doc, nil
}

func convertStep(rs rawStep, phase StepPhase) Step {
	s := Step{
		Name:               rs.Name,
		Phase:              phase,
		Model:              rs.Model,
		PromptFile:         rs.PromptFile,
		CaptureAs:          rs.CaptureAs,
		CaptureMode:        rs.CaptureMode,
		BreakLoopIfEmpty:   rs.BreakLoopIfEmpty,
		SkipIfCaptureEmpty: rs.SkipIfCaptureEmpty,
		TimeoutSeconds:     rs.TimeoutSeconds,
		OnTimeout:          rs.OnTimeout,
		ResumePrevious:     rs.ResumePrevious,
		Effort:             rs.Effort,
	}
	if len(rs.Command) > 0 {
		cmd := make([]string, len(rs.Command))
		copy(cmd, rs.Command)
		s.Command = cmd
	}
	if rs.IsClaude != nil {
		if *rs.IsClaude {
			s.Kind = StepKindClaude
			s.IsClaudeSet = true
		} else {
			s.Kind = StepKindShell
			s.IsClaudeSet = false
		}
	}
	return s
}
