package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mxriverlynn/pr9k/src/internal/steps"
)

// IterationRecord is one JSONL line written to .ralph-cache/iteration.jsonl
// after each workflow step completes. SchemaVersion starts at 1; bump on
// incompatible field changes.
type IterationRecord struct {
	SchemaVersion int     `json:"schema_version"`
	IssueID       string  `json:"issue_id"`
	IterationNum  int     `json:"iteration_num"`
	StepName      string  `json:"step_name"`
	Model         string  `json:"model,omitempty"`
	Status        string  `json:"status"` // "done" | "skipped" | "failed" | "unknown"
	DurationS     float64 `json:"duration_s"`
	InputTokens   int     `json:"input_tokens,omitempty"`
	OutputTokens  int     `json:"output_tokens,omitempty"`
	SessionID     string  `json:"session_id,omitempty"`
	Notes         string  `json:"notes,omitempty"`
}

// newIterationRecord builds an IterationRecord with the common fields populated.
// Callers set phase-specific fields (DurationS, InputTokens, OutputTokens,
// SessionID, Notes) after construction.
func newIterationRecord(issueID string, iterNum int, s steps.Step, status string) IterationRecord {
	return IterationRecord{
		SchemaVersion: 1,
		IssueID:       issueID,
		IterationNum:  iterNum,
		StepName:      s.Name,
		Model:         s.Model,
		Status:        status,
	}
}

// AppendIterationRecord appends one JSON line to
// <projectDir>/.ralph-cache/iteration.jsonl. Safe for concurrent callers:
// O_APPEND writes smaller than PIPE_BUF are atomic on POSIX. The caller is
// responsible for ensuring .ralph-cache/ exists (preflight.Run guarantees this).
func AppendIterationRecord(projectDir string, rec IterationRecord) (err error) {
	path := filepath.Join(projectDir, ".ralph-cache", "iteration.jsonl")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("workflow: iteration log: open %s: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("workflow: iteration log: close %s: %w", path, cerr)
		}
	}()
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("workflow: iteration log: marshal: %w", err)
	}
	data = append(data, '\n')
	if _, err = f.Write(data); err != nil {
		return fmt.Errorf("workflow: iteration log: write %s: %w", path, err)
	}
	return nil
}
