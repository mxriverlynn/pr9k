package validator_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/validator"
)

// assembleWorkflowDir builds a temp directory that mirrors the workflow bundle
// layout (ralph-steps.json + prompts/ + scripts/) from source-tree locations:
//   - ralph-steps.json  lives at ralph-tui/ralph-steps.json
//   - prompts/          lives at the repo root
//   - scripts/          lives at the repo root
func assembleWorkflowDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// test file: ralph-tui/internal/validator/production_steps_test.go
	// ralph-tui dir: two levels up; repo root: three levels up
	ralphTUIDir := filepath.Join(filepath.Dir(filename), "..", "..")
	repoRoot := filepath.Join(filepath.Dir(filename), "..", "..", "..")

	dir := t.TempDir()

	data, err := os.ReadFile(filepath.Join(ralphTUIDir, "ralph-steps.json"))
	if err != nil {
		t.Fatalf("read ralph-steps.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), data, 0644); err != nil {
		t.Fatalf("write ralph-steps.json: %v", err)
	}

	for _, sub := range []string{"prompts", "scripts"} {
		abs, err := filepath.Abs(filepath.Join(repoRoot, sub))
		if err != nil {
			t.Fatalf("abs path for %s: %v", sub, err)
		}
		if err := os.Symlink(abs, filepath.Join(dir, sub)); err != nil {
			t.Fatalf("symlink %s: %v", sub, err)
		}
	}

	return dir
}

// TP-001: production ralph-steps.json passes validation with zero fatal errors.
func TestValidate_ProductionStepsJSON(t *testing.T) {
	workflowDir := assembleWorkflowDir(t)
	errs := validator.Validate(workflowDir)
	if n := validator.FatalErrorCount(errs); n != 0 {
		t.Fatalf("production ralph-steps.json has %d fatal validation error(s): %v", n, errs)
	}
}

// TP-001 (cont.): iteration phase contains "Summarize to issue" wired to the correct script.
func TestLoadSteps_IterationContainsSummarizeToIssue(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	ralphTUIDir := filepath.Join(filepath.Dir(filename), "..", "..")

	sf, err := steps.LoadSteps(ralphTUIDir)
	if err != nil {
		t.Fatalf("LoadSteps: %v", err)
	}
	for _, step := range sf.Iteration {
		if step.Name == "Summarize to issue" {
			if len(step.Command) == 0 || step.Command[0] != "scripts/post_issue_summary" {
				t.Fatalf("step %q: Command[0] = %q, want %q", step.Name, step.Command[0], "scripts/post_issue_summary")
			}
			return
		}
	}
	t.Fatal(`iteration phase has no step named "Summarize to issue"`)
}
