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
	if !strings.Contains(errs[0].Error(), "sandbox create") {
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

// TP-001: CheckDocker wraps a non-ExitError from SandboxImagePresent with package context.
func TestCheckDocker_ImageNonExitError_WrappedWithContext(t *testing.T) {
	underlying := errors.New("network timeout")
	p := fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    false,
		imageErr:        underlying,
	}
	errs := CheckDocker(p)

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "preflight:") {
		t.Errorf("expected preflight prefix in error, got %q", errs[0].Error())
	}
	if !errors.Is(errs[0], underlying) {
		t.Errorf("expected wrapped underlying error, got %q", errs[0].Error())
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

// WARN-004: CheckDocker short-circuits when daemon is unreachable — image not probed.
func TestCheckDocker_DaemonUnreachable_ShortCircuits(t *testing.T) {
	spy := &spyProber{fakeProber: fakeProber{
		binaryAvailable: true,
		daemonErr:       errors.New("connection refused"),
	}}
	errs := CheckDocker(spy)

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "daemon isn't running") {
		t.Errorf("unexpected error text: %q", errs[0].Error())
	}
	if spy.imageCalled {
		t.Error("SandboxImagePresent should not be called when daemon is unreachable")
	}
}
