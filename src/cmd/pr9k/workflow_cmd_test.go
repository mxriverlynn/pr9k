package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// WG-1: newWorkflowCmd() returns a command with name "workflow".
func TestNewWorkflowCmd_Registration(t *testing.T) {
	cmd := newWorkflowCmd()
	if cmd.Name() != "workflow" {
		t.Errorf("command name = %q, want %q", cmd.Name(), "workflow")
	}
}

// WG-2: --iterations / -n must NOT be present on the workflow command.
func TestNewWorkflowCmd_NoIterationsFlag(t *testing.T) {
	cmd := newWorkflowCmd()
	if cmd.Flags().Lookup("iterations") != nil {
		t.Error("workflow command must not expose --iterations flag")
	}
	if cmd.Flags().ShorthandLookup("n") != nil {
		t.Error("workflow command must not expose -n shorthand")
	}
}

// WG-3: --workflow-dir must be present on the workflow command.
func TestNewWorkflowCmd_HasWorkflowDirFlag(t *testing.T) {
	cmd := newWorkflowCmd()
	if cmd.Flags().Lookup("workflow-dir") == nil {
		t.Error("workflow command missing --workflow-dir flag")
	}
}

// WG-4: --project-dir must be present on the workflow command.
func TestNewWorkflowCmd_HasProjectDirFlag(t *testing.T) {
	cmd := newWorkflowCmd()
	if cmd.Flags().Lookup("project-dir") == nil {
		t.Error("workflow command missing --project-dir flag")
	}
}

// WG-5: flag set is exactly {workflow-dir, project-dir, help}.
func TestNewWorkflowCmd_NoUnexpectedFlags(t *testing.T) {
	cmd := newWorkflowCmd()
	// InitDefaultHelpFlag adds the help flag lazily; call it to ensure
	// the flag set is fully populated before we inspect it.
	cmd.InitDefaultHelpFlag()
	allowed := map[string]bool{
		"workflow-dir": true,
		"project-dir":  true,
		"help":         true,
	}

	var flagNames []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		flagNames = append(flagNames, f.Name)
	})

	if len(flagNames) != len(allowed) {
		t.Errorf("expected exactly %d flags %v, got %d: %v",
			len(allowed), allowedKeys(allowed), len(flagNames), flagNames)
	}
	for _, name := range flagNames {
		if !allowed[name] {
			t.Errorf("unexpected flag %q on workflow command", name)
		}
	}
}

func allowedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// T-1: runWorkflowBuilder exits cleanly given a valid project directory.
func TestRunWorkflowBuilder_ExitsCleanly(t *testing.T) {
	dir := t.TempDir()
	cmd := newWorkflowCmd()
	cmd.SetContext(context.Background())
	if err := runWorkflowBuilder(cmd, "", dir); err != nil {
		t.Errorf("runWorkflowBuilder returned unexpected error: %v", err)
	}
}

// T-2: runWorkflowBuilder propagates a logger error with "workflow:" prefix.
func TestRunWorkflowBuilder_PropagatesLoggerError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission checks do not apply when running as root")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o444); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	cmd := newWorkflowCmd()
	cmd.SetContext(context.Background())
	err := runWorkflowBuilder(cmd, "", filepath.Join(parent, "sub"))
	if err == nil {
		t.Fatal("expected error from runWorkflowBuilder, got nil")
	}
	if !strings.Contains(err.Error(), "workflow:") {
		t.Errorf("error missing 'workflow:' prefix: %v", err)
	}
}
