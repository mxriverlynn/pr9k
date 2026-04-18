package main

import (
	"errors"
	"io"
	"os/exec"
	"regexp"

	"github.com/spf13/cobra"
)

// errSilentExit signals that the subcommand printed its own error to stderr and
// main should exit 1 without printing anything further.
var errSilentExit = errors.New("silent exit")

// dockerRunFunc runs a docker command, directing stdout and stderr to the
// provided writers. Returns the process exit code (0 on success) and any
// exec-level error (distinct from a non-zero exit code).
type dockerRunFunc func(args []string, stdout, stderr io.Writer) (exitCode int, err error)

// dockerInteractiveFunc runs a docker command with stdin attached (for
// interactive `docker run -it ...` usage). Returns the process exit code
// (0 on success) and any exec-level error (distinct from a non-zero exit code).
type dockerInteractiveFunc func(args []string, stdin io.Reader, stdout, stderr io.Writer) (exitCode int, err error)

// realDockerRun is the production implementation of dockerRunFunc.
func realDockerRun(args []string, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 {
		return -1, errors.New("realDockerRun: args must not be empty")
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

// realDockerInteractive is the production implementation of dockerInteractiveFunc.
// Unlike realDockerRun it attaches stdin too, so `docker run -it ...` can
// drive an interactive container from the user's TTY.
func realDockerInteractive(args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 {
		return -1, errors.New("realDockerInteractive: args must not be empty")
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

// ansiEscapeRe matches ANSI/VT terminal escape sequences: CSI sequences
// (\x1b[...m), OSC sequences (\x1b]...\x07), and Fe sequences (\x1b[@-_]).
// Used to strip terminal injection from untrusted subprocess output before
// reflecting it to the user's terminal (SEC-001).
var ansiEscapeRe = regexp.MustCompile(`\x1b(?:[@-Z\\-_]|\[[0-9;]*[ -/]*[@-~]|\][^\x07]*\x07)`)

// stripANSI removes ANSI/VT escape sequences from s so that untrusted
// subprocess output cannot inject terminal control codes when printed.
func stripANSI(s string) string {
	return ansiEscapeRe.ReplaceAllString(s, "")
}

// newSandboxCmd returns the parent `sandbox` cobra command, with `create` and
// `login` children attached. The parent has no RunE — running bare
// `pr9k sandbox` prints help.
func newSandboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "sandbox",
		Short:         "Manage the Claude Code sandbox image and authentication",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.AddCommand(newSandboxCreateCmd(), newSandboxLoginCmd())
	return cmd
}
