package preflight

import (
	"errors"
	"os/exec"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/sandbox"
)

// Prober abstracts docker binary and daemon checks so unit tests can
// drive every failure mode without shelling out.
type Prober interface {
	DockerBinaryAvailable() bool
	DockerDaemonReachable() error
	SandboxImagePresent() (bool, error)
}

// RealProber is the production implementation of Prober.
type RealProber struct{}

// DockerBinaryAvailable reports whether the docker binary is on PATH.
func (RealProber) DockerBinaryAvailable() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

// DockerDaemonReachable runs `docker version` and returns nil on exit 0.
func (RealProber) DockerDaemonReachable() error {
	return exec.Command("docker", "version").Run()
}

// SandboxImagePresent runs `docker image inspect <ImageTag>` and returns
// true if the image is present locally.
func (RealProber) SandboxImagePresent() (bool, error) {
	err := exec.Command("docker", "image", "inspect", sandbox.ImageTag).Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CheckDocker runs the three-step docker check and collects all errors
// before returning. Returns a nil or empty slice on success.
func CheckDocker(p Prober) []error {
	if !p.DockerBinaryAvailable() {
		return []error{errors.New("docker is not installed. Install Docker and try again")}
	}

	if err := p.DockerDaemonReachable(); err != nil {
		return []error{errors.New("docker daemon isn't running. Start Docker and try again")}
	}

	present, err := p.SandboxImagePresent()
	if err != nil {
		return []error{err}
	}
	if !present {
		return []error{errors.New("claude sandbox image is missing. Run: ralph-tui create-sandbox")}
	}

	return nil
}
