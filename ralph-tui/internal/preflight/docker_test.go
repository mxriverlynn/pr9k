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

// TP-001: CheckDocker propagates a non-ExitError from SandboxImagePresent directly.
func TestCheckDocker_ImageNonExitError_PropagatedRaw(t *testing.T) {
	p := fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    false,
		imageErr:        errors.New("network timeout"),
	}
	errs := CheckDocker(p)

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Error() != "network timeout" {
		t.Errorf("expected raw error %q, got %q", "network timeout", errs[0].Error())
	}
}

// spyProber wraps fakeProber and records which methods were called.
type spyProber struct {
	fakeProber
	daemonCalled bool
	imageCalled  bool
}

func (s *spyProber) DockerDaemonReachable() error {
	s.daemonCalled = true
	return s.fakeProber.DockerDaemonReachable()
}

func (s *spyProber) SandboxImagePresent() (bool, error) {
	s.imageCalled = true
	return s.fakeProber.SandboxImagePresent()
}

// TP-005: CheckDocker short-circuits when binary is missing — daemon and image not probed.
func TestCheckDocker_BinaryMissing_ShortCircuits(t *testing.T) {
	spy := &spyProber{fakeProber: fakeProber{binaryAvailable: false}}
	errs := CheckDocker(spy)

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "not installed") {
		t.Errorf("unexpected error text: %q", errs[0].Error())
	}
	if spy.daemonCalled {
		t.Error("DockerDaemonReachable should not be called when binary is missing")
	}
	if spy.imageCalled {
		t.Error("SandboxImagePresent should not be called when binary is missing")
	}
}
