package validator_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/validator"
)

func getRalphTUIDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "workflow")
}

// assembleWorkflowDir builds a temp directory that mirrors the workflow bundle
// layout (config.json + prompts/ + scripts/) from source-tree locations:
//   - config.json  lives at workflow/config.json
//   - prompts/          lives at workflow/prompts/
//   - scripts/          lives at workflow/scripts/
func assembleWorkflowDir(t *testing.T) string {
	t.Helper()
	ralphTUIDir := getRalphTUIDir(t)

	dir := t.TempDir()

	data, err := os.ReadFile(filepath.Join(ralphTUIDir, "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		t.Fatalf("write config.json: %v", err)
	}

	for _, rel := range []string{"config.json", "prompts", "scripts"} {
		if _, err := os.Stat(filepath.Join(ralphTUIDir, rel)); err != nil {
			t.Fatalf("workflow bundle incomplete: %s missing — run from repo root (%v)", rel, err)
		}
	}

	for _, sub := range []string{"prompts", "scripts"} {
		abs, err := filepath.Abs(filepath.Join(ralphTUIDir, sub))
		if err != nil {
			t.Fatalf("abs path for %s: %v", sub, err)
		}
		if err := os.Symlink(abs, filepath.Join(dir, sub)); err != nil {
			t.Fatalf("symlink %s: %v", sub, err)
		}
	}

	return dir
}

// TestValidate_ProductionStepsJSON is the single test guarding the bundled
// workflow/config.json: it loads the production bundle and asserts that the
// validator returns zero fatal findings.
func TestValidate_ProductionStepsJSON(t *testing.T) {
	workflowDir := assembleWorkflowDir(t)
	errs := validator.Validate(workflowDir)
	if n := validator.FatalErrorCount(errs); n != 0 {
		t.Fatalf("production config.json has %d fatal validation error(s): %v", n, errs)
	}
}
