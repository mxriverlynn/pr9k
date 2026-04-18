package statusline

import "encoding/json"

type stepJSON struct {
	Num   int    `json:"num"`
	Count int    `json:"count"`
	Name  string `json:"name"`
}

type payloadJSON struct {
	SessionID     string            `json:"sessionId"`
	Version       string            `json:"version"`
	Phase         string            `json:"phase"`
	Iteration     int               `json:"iteration"`
	MaxIterations int               `json:"maxIterations"`
	Step          stepJSON          `json:"step"`
	Mode          string            `json:"mode"`
	WorkflowDir   string            `json:"workflowDir"`
	ProjectDir    string            `json:"projectDir"`
	Captures      map[string]string `json:"captures"`
}

// BuildPayload marshals s and mode into the JSON payload written to a
// status-line script's stdin. All fields are always present; captures is
// always an object (never null).
func BuildPayload(s State, mode string) ([]byte, error) {
	captures := s.Captures
	if captures == nil {
		captures = map[string]string{}
	}

	p := payloadJSON{
		SessionID:     s.SessionID,
		Version:       s.Version,
		Phase:         s.Phase,
		Iteration:     s.Iteration,
		MaxIterations: s.MaxIterations,
		Step: stepJSON{
			Num:   s.StepNum,
			Count: s.StepCount,
			Name:  s.StepName,
		},
		Mode:        mode,
		WorkflowDir: s.WorkflowDir,
		ProjectDir:  s.ProjectDir,
		Captures:    captures,
	}
	return json.Marshal(p)
}
