package workflowio

import (
	"encoding/json"

	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// marshalDoc serialises doc to config.json-compatible JSON. Steps are bucketed
// into initialize/iteration/finalize by their Phase field.
func marshalDoc(doc workflowmodel.WorkflowDoc) ([]byte, error) {
	type outStep struct {
		Name               string   `json:"name"`
		IsClaude           *bool    `json:"isClaude,omitempty"`
		Model              string   `json:"model,omitempty"`
		PromptFile         string   `json:"promptFile,omitempty"`
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
	type outStatusLine struct {
		Type                   string `json:"type,omitempty"`
		Command                string `json:"command"`
		RefreshIntervalSeconds int    `json:"refreshIntervalSeconds,omitempty"`
	}
	type outDefaults struct {
		Effort string `json:"effort,omitempty"`
	}
	type outConfig struct {
		Initialize   []outStep         `json:"initialize"`
		Iteration    []outStep         `json:"iteration"`
		Finalize     []outStep         `json:"finalize"`
		StatusLine   *outStatusLine    `json:"statusLine,omitempty"`
		Defaults     *outDefaults      `json:"defaults,omitempty"`
		Env          []string          `json:"env,omitempty"`
		ContainerEnv map[string]string `json:"containerEnv,omitempty"`
	}

	var initSteps, iterSteps, finalSteps []outStep
	for _, s := range doc.Steps {
		os := outStep{
			Name:               s.Name,
			Model:              s.Model,
			PromptFile:         s.PromptFile,
			CaptureAs:          s.CaptureAs,
			CaptureMode:        s.CaptureMode,
			BreakLoopIfEmpty:   s.BreakLoopIfEmpty,
			SkipIfCaptureEmpty: s.SkipIfCaptureEmpty,
			TimeoutSeconds:     s.TimeoutSeconds,
			OnTimeout:          s.OnTimeout,
			ResumePrevious:     s.ResumePrevious,
			Effort:             s.Effort,
		}
		if len(s.Command) > 0 {
			cmd := make([]string, len(s.Command))
			copy(cmd, s.Command)
			os.Command = cmd
		}
		if s.IsClaudeSet {
			b := s.Kind == workflowmodel.StepKindClaude
			os.IsClaude = &b
		}
		switch s.Phase {
		case workflowmodel.StepPhaseInitialize:
			initSteps = append(initSteps, os)
		case workflowmodel.StepPhaseFinalize:
			finalSteps = append(finalSteps, os)
		default:
			iterSteps = append(iterSteps, os)
		}
	}

	if initSteps == nil {
		initSteps = []outStep{}
	}
	if iterSteps == nil {
		iterSteps = []outStep{}
	}
	if finalSteps == nil {
		finalSteps = []outStep{}
	}

	cfg := outConfig{
		Initialize:   initSteps,
		Iteration:    iterSteps,
		Finalize:     finalSteps,
		Env:          doc.Env,
		ContainerEnv: doc.ContainerEnv,
	}
	if doc.StatusLine != nil {
		cfg.StatusLine = &outStatusLine{
			Type:                   doc.StatusLine.Type,
			Command:                doc.StatusLine.Command,
			RefreshIntervalSeconds: doc.StatusLine.RefreshIntervalSeconds,
		}
	}
	if doc.Defaults != nil {
		cfg.Defaults = &outDefaults{Effort: doc.Defaults.Effort}
	}

	return json.MarshalIndent(cfg, "", "  ")
}
