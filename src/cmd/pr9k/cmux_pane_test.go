package main

import (
	"io"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

// suppressCobra silences cobra's default error output for a command under test,
// so test output stays clean when we deliberately invoke invalid arguments.
func suppressCobra(cmd interface {
	SetOut(io.Writer)
	SetErr(io.Writer)
}) {
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
}

// ---- ActionSkip existence test -----------------------------------------------

// TestActionSkip_IsDistinctStepAction verifies that ui.ActionSkip exists as a
// StepAction value and is distinct from the other three values.
func TestActionSkip_IsDistinctStepAction(t *testing.T) {
	if ui.ActionSkip == ui.ActionRetry {
		t.Error("ActionSkip must differ from ActionRetry")
	}
	if ui.ActionSkip == ui.ActionContinue {
		t.Error("ActionSkip must differ from ActionContinue")
	}
	if ui.ActionSkip == ui.ActionQuit {
		t.Error("ActionSkip must differ from ActionQuit")
	}
}

// ---- cmux-pane subcommand tests ----------------------------------------------

// TestCmuxPaneCmd_ValidRoles verifies that each of the four valid role values
// parses successfully and dispatches without error when PR9K_CMUX_SOCKET and
// PR9K_PROJECT_DIR are set.
func TestCmuxPaneCmd_ValidRoles(t *testing.T) {
	for _, role := range []string{"orchestrator", "header", "log", "footer"} {
		t.Run(role, func(t *testing.T) {
			t.Setenv("PR9K_CMUX_SOCKET", "/tmp/test.sock")
			t.Setenv("PR9K_PROJECT_DIR", t.TempDir())
			cmd := newCmuxPaneCmd()
			suppressCobra(cmd)
			cmd.SetArgs([]string{"--role=" + role})
			if err := cmd.Execute(); err != nil {
				t.Errorf("--role=%s: unexpected error: %v", role, err)
			}
		})
	}
}

// TestCmuxPaneCmd_InvalidRole verifies that an unrecognized role value returns
// a clear error containing the invalid value.
func TestCmuxPaneCmd_InvalidRole(t *testing.T) {
	t.Setenv("PR9K_CMUX_SOCKET", "/tmp/test.sock")
	cmd := newCmuxPaneCmd()
	suppressCobra(cmd)
	cmd.SetArgs([]string{"--role=bogus"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --role=bogus, got nil")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error %q does not mention the invalid role value %q", err.Error(), "bogus")
	}
}

// TestCmuxPaneCmd_MissingRole verifies that omitting --role entirely returns an
// error (cobra required-flag enforcement).
func TestCmuxPaneCmd_MissingRole(t *testing.T) {
	t.Setenv("PR9K_CMUX_SOCKET", "/tmp/test.sock")
	cmd := newCmuxPaneCmd()
	suppressCobra(cmd)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when --role is omitted, got nil")
	}
}

// TestCmuxPaneCmd_MissingSocket verifies that a valid --role with no
// PR9K_CMUX_SOCKET environment variable returns a clear error.
func TestCmuxPaneCmd_MissingSocket(t *testing.T) {
	t.Setenv("PR9K_CMUX_SOCKET", "")
	cmd := newCmuxPaneCmd()
	suppressCobra(cmd)
	cmd.SetArgs([]string{"--role=orchestrator"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when PR9K_CMUX_SOCKET is unset, got nil")
	}
	if !strings.Contains(err.Error(), "PR9K_CMUX_SOCKET") {
		t.Errorf("error %q does not mention PR9K_CMUX_SOCKET", err.Error())
	}
}

// TestCmuxPaneCmd_SocketPathPassedToRunner verifies that PR9K_CMUX_SOCKET is
// read and passed to the dispatched runner. We do this by checking that the
// command succeeds when a socket path is provided (the runner stub accepts it).
func TestCmuxPaneCmd_SocketPathPassedToRunner(t *testing.T) {
	const socketPath = "/tmp/pr9k-cmux-test.sock"
	t.Setenv("PR9K_CMUX_SOCKET", socketPath)
	t.Setenv("PR9K_PROJECT_DIR", t.TempDir())
	cmd := newCmuxPaneCmd()
	suppressCobra(cmd)
	cmd.SetArgs([]string{"--role=footer"})
	if err := cmd.Execute(); err != nil {
		t.Errorf("unexpected error with socket path %q: %v", socketPath, err)
	}
}
