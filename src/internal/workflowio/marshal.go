package workflowio

import (
	"encoding/json"

	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// marshalDoc serialises doc to config.json-compatible JSON. All steps are
// written under the "iteration" phase; phases are flat in WorkflowDoc
// (WU-5 will reintroduce phase-aware save/load).
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
	}
	type outStatusLine struct {
		Type                   string `json:"type,omitempty"`
		Command                string `json:"command"`
		RefreshIntervalSeconds int    `json:"refreshIntervalSeconds,omitempty"`
	}
	type outConfig struct {
		Initialize []outStep      `json:"initialize"`
		Iteration  []outStep      `json:"iteration"`
		Finalize   []outStep      `json:"finalize"`
		StatusLine *outStatusLine `json:"statusLine,omitempty"`
	}

	steps := make([]outStep, len(doc.Steps))
	for i, s := range doc.Steps {
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
		steps[i] = os
	}

	cfg := outConfig{
		Initialize: []outStep{},
		Iteration:  steps,
		Finalize:   []outStep{},
	}
	if doc.StatusLine != nil {
		cfg.StatusLine = &outStatusLine{
			Type:                   doc.StatusLine.Type,
			Command:                doc.StatusLine.Command,
			RefreshIntervalSeconds: doc.StatusLine.RefreshIntervalSeconds,
		}
	}

	return json.MarshalIndent(cfg, "", "  ")
}
