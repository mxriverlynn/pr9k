package configs_test

import (
	"encoding/json"
	"os"
	"testing"
)

// step mirrors the structure used in the JSON config files.
type step struct {
	Name        string   `json:"name"`
	IsClaude    bool     `json:"isClaude"`
	Model       string   `json:"model"`
	PromptFile  string   `json:"promptFile"`
	PrependVars bool     `json:"prependVars"`
	Command     []string `json:"command"`
}

func loadSteps(t *testing.T, filename string) []step {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read %s: %v", filename, err)
	}
	var steps []step
	if err := json.Unmarshal(data, &steps); err != nil {
		t.Fatalf("could not parse %s as JSON: %v", filename, err)
	}
	return steps
}

func TestRalphStepsJSON_ValidAndCount(t *testing.T) {
	steps := loadSteps(t, "ralph-steps.json")
	if len(steps) != 8 {
		t.Errorf("expected 8 steps in ralph-steps.json, got %d", len(steps))
	}
	for i, s := range steps {
		if s.Name == "" {
			t.Errorf("step %d: missing name field", i)
		}
		if s.IsClaude {
			if s.Model == "" {
				t.Errorf("step %d (%q): isClaude=true but missing model", i, s.Name)
			}
			if s.PromptFile == "" {
				t.Errorf("step %d (%q): isClaude=true but missing promptFile", i, s.Name)
			}
			if !s.PrependVars {
				t.Errorf("step %d (%q): isClaude=true but prependVars is false; iteration steps require ISSUENUMBER and STARTINGSHA", i, s.Name)
			}
		} else {
			if len(s.Command) == 0 {
				t.Errorf("step %d (%q): isClaude=false but missing command", i, s.Name)
			}
		}
	}
}

func TestRalphFinalizeStepsJSON_ValidAndCount(t *testing.T) {
	steps := loadSteps(t, "ralph-finalize-steps.json")
	if len(steps) != 3 {
		t.Errorf("expected 3 steps in ralph-finalize-steps.json, got %d", len(steps))
	}
	for i, s := range steps {
		if s.Name == "" {
			t.Errorf("step %d: missing name field", i)
		}
		if s.IsClaude {
			if s.Model == "" {
				t.Errorf("step %d (%q): isClaude=true but missing model", i, s.Name)
			}
			if s.PromptFile == "" {
				t.Errorf("step %d (%q): isClaude=true but missing promptFile", i, s.Name)
			}
			if s.PrependVars {
				t.Errorf("step %d (%q): isClaude=true but prependVars is true; finalization steps must not prepend issue/SHA vars", i, s.Name)
			}
		} else {
			if len(s.Command) == 0 {
				t.Errorf("step %d (%q): isClaude=false but missing command", i, s.Name)
			}
		}
	}
}
