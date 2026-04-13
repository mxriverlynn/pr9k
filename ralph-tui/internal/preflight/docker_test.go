package preflight

import (
	"errors"
	"strings"
	"testing"
)

// fakeProber is a test double for Prober.
type fakeProber struct {
	binaryAvailable bool
	daemonErr       error
	imagePresent    bool
	imageErr        error
}

func (f fakeProber) DockerBinaryAvailable() bool        { return f.binaryAvailable }
func (f fakeProber) DockerDaemonReachable() error       { return f.daemonErr }
func (f fakeProber) SandboxImagePresent() (bool, error) { return f.imagePresent, f.imageErr }

func TestCheckDocker_BinaryMissing(t *testing.T) {
	p := fakeProber{binaryAvailable: false}
	errs := CheckDocker(p)

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "docker is not installed") {
		t.Errorf("unexpected error text: %q", errs[0].Error())
	}
}

func TestCheckDocker_DaemonUnreachable(t *testing.T) {
	p := fakeProber{
		binaryAvailable: true,
		daemonErr:       errors.New("connection refused"),
	}
	errs := CheckDocker(p)

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "daemon isn't running") {
		t.Errorf("unexpected error text: %q", errs[0].Error())
	}
}

func TestCheckDocker_ImageMissing(t *testing.T) {
	p := fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    false,
	}
	errs := CheckDocker(p)

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "create-sandbox") {
		t.Errorf("unexpected error text: %q", errs[0].Error())
	}
}

func TestCheckDocker_AllGreen(t *testing.T) {
	p := fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    true,
	}
	errs := CheckDocker(p)

	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}
