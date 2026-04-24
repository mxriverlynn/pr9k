package main

import (
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
