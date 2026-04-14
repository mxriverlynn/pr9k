package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/preflight"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/sandbox"
)

// sandboxLoginDeps holds injected dependencies so unit tests can drive every
// branch without shelling out to a real docker daemon or a real TTY.
type sandboxLoginDeps struct {
	prober            preflight.Prober
	dockerInteractive dockerInteractiveFunc
	dockerRun         dockerRunFunc
	uid               int
	gid               int
	profileDir        string
	stdin             io.Reader
	stdout            io.Writer
	stderr            io.Writer
}

// newSandboxLoginCmd returns the production `sandbox login` cobra command
// wired with real docker dependencies and the resolved profile dir.
func newSandboxLoginCmd() *cobra.Command {
	uid, gid := sandbox.HostUIDGID()
	return newSandboxLoginCmdWith(&sandboxLoginDeps{
		prober:            preflight.RealProber{},
		dockerInteractive: realDockerInteractive,
		dockerRun:         realDockerRun,
		uid:               uid,
		gid:               gid,
		profileDir:        preflight.ResolveProfileDir(),
		stdin:             os.Stdin,
		stdout:            os.Stdout,
		stderr:            os.Stderr,
	})
}

// newSandboxLoginCmdWith builds the cobra command using the provided deps.
// Separated from newSandboxLoginCmd so tests can inject fakes.
func newSandboxLoginCmdWith(deps *sandboxLoginDeps) *cobra.Command {
	return &cobra.Command{
		Use:           "login",
		Short:         "Launch an interactive sandbox to authenticate the Claude profile",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSandboxLogin(deps)
		},
	}
}

func runSandboxLogin(deps *sandboxLoginDeps) error {
	// Step 1: Docker reachability check.
	if !deps.prober.DockerBinaryAvailable() {
		_, _ = fmt.Fprintln(deps.stderr, "Docker is not installed. Install Docker and try again.")
		return errSilentExit
	}
	if err := deps.prober.DockerDaemonReachable(); err != nil {
		_, _ = fmt.Fprintln(deps.stderr, "Docker is installed but the daemon isn't running. Start Docker and try again.")
		return errSilentExit
	}

	// Step 2: Image presence; auto-pull with a verbose note if missing.
	present, err := deps.prober.SandboxImagePresent()
	if err != nil {
		_, _ = fmt.Fprintf(deps.stderr, "Failed to check sandbox image: %v\n", err)
		return errSilentExit
	}
	if !present {
		_, _ = fmt.Fprintln(deps.stdout, "Sandbox image not found; pulling it first — run 'ralph-tui sandbox create' next time to separate this step.")
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

	// Step 3: Ensure the profile directory exists so docker can bind-mount it.
	if err := os.MkdirAll(deps.profileDir, 0o700); err != nil {
		_, _ = fmt.Fprintf(deps.stderr, "Failed to prepare profile directory %s: %v\n", deps.profileDir, err)
		return errSilentExit
	}

	// Step 4: Interactive run. Claude's REPL output drives the session;
	// non-zero exit propagates as a silent exit (user has already seen any
	// error output from inside the container).
	args := sandbox.BuildLoginArgs(deps.profileDir, deps.uid, deps.gid)
	exitCode, runErr := deps.dockerInteractive(args, deps.stdin, deps.stdout, deps.stderr)
	if runErr != nil {
		_, _ = fmt.Fprintf(deps.stderr, "Sandbox login failed: %v\n", runErr)
		return errSilentExit
	}
	if exitCode != 0 {
		return errSilentExit
	}
	return nil
}
