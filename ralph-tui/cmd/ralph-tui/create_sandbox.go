package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/preflight"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/sandbox"
)

// errSilentExit signals that the subcommand printed its own error to stderr and
// main should exit 1 without printing anything further.
var errSilentExit = errors.New("silent exit")

// dockerRunFunc runs a docker command, directing stdout and stderr to the
// provided writers. Returns the process exit code (0 on success) and any
// exec-level error (distinct from a non-zero exit code).
type dockerRunFunc func(args []string, stdout, stderr io.Writer) (exitCode int, err error)

// createSandboxDeps holds injected dependencies so unit tests can drive every
// branch without shelling out to a real docker daemon.
type createSandboxDeps struct {
	prober    preflight.Prober
	dockerRun dockerRunFunc
	uid       int
	gid       int
	stdout    io.Writer
	stderr    io.Writer
}

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

// newCreateSandboxCmd returns the production create-sandbox cobra command wired
// with real docker dependencies.
func newCreateSandboxCmd() *cobra.Command {
	uid, gid := sandbox.HostUIDGID()
	return newCreateSandboxCmdWith(&createSandboxDeps{
		prober:    preflight.RealProber{},
		dockerRun: realDockerRun,
		uid:       uid,
		gid:       gid,
		stdout:    os.Stdout,
		stderr:    os.Stderr,
	})
}

// newCreateSandboxCmdWith builds the cobra command using the provided deps.
// Separated from newCreateSandboxCmd so tests can inject fakes.
func newCreateSandboxCmdWith(deps *createSandboxDeps) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:           "create-sandbox",
		Short:         "Pull the sandbox image and verify it can run under the current user",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateSandbox(deps, force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "re-pull the sandbox image even if it is already present")
	return cmd
}

// semverRe matches a semver-shaped pattern (e.g. "2.1.101") anywhere in a string.
var semverRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

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

func runCreateSandbox(deps *createSandboxDeps, force bool) error {
	// Step 1: Docker reachability check.
	_, _ = fmt.Fprint(deps.stdout, "Checking Docker... ")
	if !deps.prober.DockerBinaryAvailable() {
		_, _ = fmt.Fprintln(deps.stdout)
		_, _ = fmt.Fprintln(deps.stderr, "Docker is not installed. Install Docker and try again.")
		return errSilentExit
	}
	if err := deps.prober.DockerDaemonReachable(); err != nil {
		_, _ = fmt.Fprintln(deps.stdout)
		_, _ = fmt.Fprintln(deps.stderr, "Docker is installed but the daemon isn't running. Start Docker and try again.")
		return errSilentExit
	}
	_, _ = fmt.Fprintln(deps.stdout, "✓")

	// Step 2: Image presence / pull.
	present, err := deps.prober.SandboxImagePresent()
	if err != nil {
		_, _ = fmt.Fprintf(deps.stderr, "Failed to check sandbox image: %v\n", err)
		return errSilentExit
	}

	if present && !force {
		_, _ = fmt.Fprintf(deps.stdout, "Image %s already present; skipping pull (use --force to re-pull).\n", sandbox.ImageTag)
	} else {
		// Pull — stream stdout directly; capture stderr for failure reporting.
		var pullStderr bytes.Buffer
		exitCode, runErr := deps.dockerRun([]string{"docker", "pull", sandbox.ImageTag}, deps.stdout, &pullStderr)
		if runErr != nil {
			_, _ = fmt.Fprintf(deps.stderr, "Failed to pull sandbox image: %v\n", runErr)
			return errSilentExit
		}
		if exitCode != 0 {
			_, _ = fmt.Fprintln(deps.stderr, "Failed to pull sandbox image.")
			if s := pullStderr.String(); s != "" {
				_, _ = fmt.Fprint(deps.stderr, s)
			}
			return errSilentExit
		}
	}

	// Step 3: Smoke test.
	var smokeStdout, smokeStderr bytes.Buffer
	smokeArgs := []string{
		"docker", "run", "--rm",
		"-u", fmt.Sprintf("%d:%d", deps.uid, deps.gid),
		sandbox.ImageTag,
		"claude", "--version",
	}
	exitCode, runErr := deps.dockerRun(smokeArgs, &smokeStdout, &smokeStderr)
	if runErr != nil {
		_, _ = fmt.Fprintf(deps.stderr, "Sandbox smoke test failed: %v\n", runErr)
		return errSilentExit
	}
	if exitCode != 0 {
		_, _ = fmt.Fprintf(deps.stderr, "Sandbox smoke test failed — container exited with status %d.\n", exitCode)
		if s := smokeStderr.String(); s != "" {
			_, _ = fmt.Fprint(deps.stderr, s)
		}
		return errSilentExit
	}

	// Accept version output from stdout first, then stderr.
	// stripANSI prevents terminal injection from a malicious or compromised image.
	output := stripANSI(strings.TrimSpace(smokeStdout.String()))
	if output == "" {
		output = stripANSI(strings.TrimSpace(smokeStderr.String()))
	}
	if output == "" {
		_, _ = fmt.Fprintln(deps.stderr, "Sandbox smoke test failed — image ran but produced no version output. Image may be corrupted or a locally-tagged stub. Re-pull with --force.")
		return errSilentExit
	}

	if !semverRe.MatchString(output) {
		_, _ = fmt.Fprintf(deps.stdout, "Sandbox smoke test warning — unexpected version output: %s. The image may not be claude-code (e.g., a tag squat or local stub). Re-pull with --force or verify the image digest before proceeding.\n", output)
	} else {
		_, _ = fmt.Fprintf(deps.stdout, "Sandbox verified: %s under UID %d:%d.\n", output, deps.uid, deps.gid)
	}

	// Step 4: Done.
	_, _ = fmt.Fprintln(deps.stdout, "Sandbox ready.")
	return nil
}
