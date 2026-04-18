package workflow_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func workflowTestRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	// file is .../src/internal/workflow/resolve_config_commands_test.go
	// three levels up reaches the workspace root
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// TP-004 (issue #145): every "command" entry in workflow/config.json whose first
// element starts with "scripts/" resolves to an executable file under workflowDir.
// This pins the invariant that command paths are relative to the bundle root.
func TestWorkflowConfigCommands_ScriptsAreExecutable(t *testing.T) {
	root := workflowTestRepoRoot(t)
	workflowDir := filepath.Join(root, "workflow")

	configData, err := os.ReadFile(filepath.Join(workflowDir, "config.json"))
	if err != nil {
		t.Fatalf("read workflow/config.json: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(configData, &raw); err != nil {
		t.Fatalf("parse workflow/config.json: %v", err)
	}

	// Collect all steps from initialize, iteration, and finalize arrays.
	var allSteps []map[string]json.RawMessage
	for _, key := range []string{"initialize", "iteration", "finalize"} {
		arr, ok := raw[key]
		if !ok {
			continue
		}
		var steps []map[string]json.RawMessage
		if err := json.Unmarshal(arr, &steps); err != nil {
			t.Fatalf("parse %s array: %v", key, err)
		}
		allSteps = append(allSteps, steps...)
	}

	for _, step := range allSteps {
		cmdRaw, ok := step["command"]
		if !ok {
			continue
		}
		var args []string
		if err := json.Unmarshal(cmdRaw, &args); err != nil || len(args) == 0 {
			continue
		}
		if len(args[0]) < 8 || args[0][:8] != "scripts/" {
			continue
		}
		scriptPath := filepath.Join(workflowDir, args[0])
		info, err := os.Stat(scriptPath)
		if err != nil {
			t.Errorf("command %q: file not found at %s: %v", args[0], scriptPath, err)
			continue
		}
		if info.Mode()&0o111 == 0 {
			t.Errorf("command %q: file at %s is not executable (mode %o)", args[0], scriptPath, info.Mode())
		}
	}
}
