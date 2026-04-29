package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mxriverlynn/pr9k/src/internal/preflight"
	"github.com/mxriverlynn/pr9k/src/internal/sandbox"
)

// sandboxShellDeps holds injected dependencies so unit tests can drive every
// branch without shelling out to a real docker daemon or a real TTY.
type sandboxShellDeps struct {
	prober            preflight.Prober
	dockerInteractive dockerInteractiveFunc
	dockerRun         dockerRunFunc
	uid               int
	gid               int
	projectDir        string
	profileDir        string
	stdin             io.Reader
	stdout            io.Writer
	stderr            io.Writer
}

// newSandboxShellCmd returns the production `sandbox shell` cobra command
// wired with real docker dependencies and the resolved profile dir. The
// project dir is captured at command construction from the current working
// directory.
func newSandboxShellCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "shell",
		Short:         "Open an interactive bash shell inside the sandbox container",
		Long:          "Launches a sandbox container with the current project directory and Claude profile mounted, dropping you into an interactive bash shell. The container is removed when the shell exits.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("sandbox shell: resolve current directory: %w", err)
			}
			uid, gid := sandbox.HostUIDGID()
			return runSandboxShell(&sandboxShellDeps{
				prober:            preflight.RealProber{},
				dockerInteractive: realDockerInteractive,
				dockerRun:         realDockerRun,
				uid:               uid,
				gid:               gid,
				projectDir:        projectDir,
				profileDir:        preflight.ResolveProfileDir(),
				stdin:             os.Stdin,
				stdout:            os.Stdout,
				stderr:            os.Stderr,
			})
		},
	}
}

// runSandboxShell ensures Docker is reachable, pulls the sandbox image if
// missing, prepares the profile directory, and launches an interactive bash
// shell inside the sandbox container. The container is removed when the
// shell exits (`docker run --rm`).
func runSandboxShell(deps *sandboxShellDeps) error {
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
		_, _ = fmt.Fprintln(deps.stdout, "Sandbox image not found; pulling it first — run 'pr9k sandbox create' next time to separate this step.")
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

	// Step 4: Interactive bash session. The container exits when the user
	// types `exit` or hits Ctrl-D; `--rm` removes it from the local store.
	args := sandbox.BuildShellArgs(deps.projectDir, deps.profileDir, deps.uid, deps.gid)
	exitCode, runErr := deps.dockerInteractive(args, deps.stdin, deps.stdout, deps.stderr)
	if runErr != nil {
		_, _ = fmt.Fprintf(deps.stderr, "Sandbox shell failed: %v\n", runErr)
		return errSilentExit
	}
	if exitCode != 0 {
		return errSilentExit
	}
	return nil
}
