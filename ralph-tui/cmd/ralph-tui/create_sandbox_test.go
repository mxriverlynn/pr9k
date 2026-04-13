package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// fakeProber implements preflight.Prober for unit tests.
type fakeProber struct {
	binaryAvailable bool
	daemonErr       error
	imagePresent    bool
	imagePresentErr error
}

func (f *fakeProber) DockerBinaryAvailable() bool        { return f.binaryAvailable }
func (f *fakeProber) DockerDaemonReachable() error       { return f.daemonErr }
func (f *fakeProber) SandboxImagePresent() (bool, error) { return f.imagePresent, f.imagePresentErr }

// fakeRun holds a sequence of canned responses for dockerRunFunc calls.
type fakeRun struct {
	calls     [][]string
	responses []fakeRunResponse
}

type fakeRunResponse struct {
	exitCode int
	stdout   string
	stderr   string
	err      error
}

func (f *fakeRun) run(args []string, stdout, stderr io.Writer) (int, error) {
	idx := len(f.calls)
	f.calls = append(f.calls, args)
	if idx >= len(f.responses) {
		return 0, nil
	}
	resp := f.responses[idx]
	if resp.err != nil {
		return -1, resp.err
	}
	if resp.stdout != "" {
		_, _ = io.WriteString(stdout, resp.stdout)
	}
	if resp.stderr != "" {
		_, _ = io.WriteString(stderr, resp.stderr)
	}
	return resp.exitCode, nil
}

// newTestDeps builds a createSandboxDeps with captured stdout/stderr writers
// and the provided prober and fakeRun, using uid=501 gid=20.
func newTestDeps(prober *fakeProber, fr *fakeRun, outBuf, errBuf *bytes.Buffer) *createSandboxDeps {
	return &createSandboxDeps{
		prober:    prober,
		dockerRun: fr.run,
		uid:       501,
		gid:       20,
		stdout:    outBuf,
		stderr:    errBuf,
	}
}

// runCmd builds a command from deps, sets args, and executes it. Returns the
// error from cobra's Execute (which equals RunE's return value due to SilenceErrors).
func runCmd(deps *createSandboxDeps, args ...string) error {
	cmd := newCreateSandboxCmdWith(deps)
	cmd.SetArgs(args)
	return cmd.Execute()
}

// TestCreateSandbox_DockerBinaryMissing verifies the exact error message and
// errSilentExit when the docker binary is not on PATH.
func TestCreateSandbox_DockerBinaryMissing(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{binaryAvailable: false}
	fr := &fakeRun{}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "Docker is not installed. Install Docker and try again.") {
		t.Errorf("want docker-not-installed message, got stderr: %q", errBuf.String())
	}
	if len(fr.calls) != 0 {
		t.Errorf("no docker exec calls expected, got %d", len(fr.calls))
	}
}

// TestCreateSandbox_DaemonUnreachable verifies the exact error message and
// errSilentExit when the daemon is not running.
func TestCreateSandbox_DaemonUnreachable(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		daemonErr:       errors.New("connection refused"),
	}
	fr := &fakeRun{}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "Docker is installed but the daemon isn't running. Start Docker and try again.") {
		t.Errorf("want daemon-unreachable message, got stderr: %q", errBuf.String())
	}
}

// TestCreateSandbox_ImagePresent_NoForce verifies that pull is skipped when
// the image is already present and --force is not passed, and that a green
// smoke test produces "Sandbox ready."
func TestCreateSandbox_ImagePresent_NoForce(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    true,
	}
	// Only one docker call: the smoke test.
	fr := &fakeRun{responses: []fakeRunResponse{
		{exitCode: 0, stdout: "claude 2.1.101"},
	}}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps)

	if err != nil {
		t.Errorf("want nil, got %v", err)
	}
	// Pull must NOT have run (no "docker pull" call).
	for _, call := range fr.calls {
		if len(call) > 1 && call[1] == "pull" {
			t.Errorf("docker pull must not run when image present and --force not set; calls=%v", fr.calls)
		}
	}
	if !strings.Contains(outBuf.String(), "Sandbox ready.") {
		t.Errorf("want 'Sandbox ready.' in stdout, got %q", outBuf.String())
	}
}

// TestCreateSandbox_ImagePresent_Force verifies that pull runs when --force is
// set even though the image is already present, and that smoke test also runs.
func TestCreateSandbox_ImagePresent_Force(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    true,
	}
	// Two calls: pull, then smoke test.
	fr := &fakeRun{responses: []fakeRunResponse{
		{exitCode: 0},                           // pull succeeds
		{exitCode: 0, stdout: "claude 2.1.101"}, // smoke test succeeds
	}}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps, "--force")

	if err != nil {
		t.Errorf("want nil, got %v", err)
	}
	// Pull must have run.
	pullFound := false
	for _, call := range fr.calls {
		if len(call) > 1 && call[1] == "pull" {
			pullFound = true
		}
	}
	if !pullFound {
		t.Errorf("expected docker pull call when --force set; calls=%v", fr.calls)
	}
	if !strings.Contains(outBuf.String(), "Sandbox ready.") {
		t.Errorf("want 'Sandbox ready.' in stdout, got %q", outBuf.String())
	}
}

// TestCreateSandbox_PullFails_SmokeNotRun verifies that a non-zero pull exit
// emits the failure message and returns errSilentExit without running smoke test.
func TestCreateSandbox_PullFails_SmokeNotRun(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    false,
	}
	fr := &fakeRun{responses: []fakeRunResponse{
		{exitCode: 1, stderr: "Error: manifest unknown"},
	}}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "Failed to pull sandbox image.") {
		t.Errorf("want pull-failure message in stderr, got %q", errBuf.String())
	}
	// Smoke test must not have run.
	for _, call := range fr.calls {
		if len(call) > 1 && call[1] == "run" {
			t.Errorf("smoke test must not run after pull failure; calls=%v", fr.calls)
		}
	}
}

// TestCreateSandbox_SmokeTest_NonZeroExit verifies the exact failure message
// (including captured stderr) when the smoke test container exits non-zero.
func TestCreateSandbox_SmokeTest_NonZeroExit(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    false,
	}
	fr := &fakeRun{responses: []fakeRunResponse{
		{exitCode: 0},                        // pull succeeds
		{exitCode: 2, stderr: "exec failed"}, // smoke test fails
	}}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	gotStderr := errBuf.String()
	if !strings.Contains(gotStderr, "Sandbox smoke test failed — container exited with status 2.") {
		t.Errorf("want exit-status-2 failure message, got stderr: %q", gotStderr)
	}
	if !strings.Contains(gotStderr, "exec failed") {
		t.Errorf("want captured stderr in output, got: %q", gotStderr)
	}
}

// TestCreateSandbox_SmokeTest_EmptyOutput verifies the "no version output"
// failure when the container exits 0 but writes nothing to stdout or stderr.
func TestCreateSandbox_SmokeTest_EmptyOutput(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    false,
	}
	fr := &fakeRun{responses: []fakeRunResponse{
		{exitCode: 0}, // pull succeeds
		{exitCode: 0}, // smoke test: exit 0, no output
	}}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "image ran but produced no version output") {
		t.Errorf("want no-version-output message, got stderr: %q", errBuf.String())
	}
}

// TestCreateSandbox_SmokeTest_UnexpectedOutput verifies the warning (not failure)
// path when exit 0 output does not match a semver pattern.
func TestCreateSandbox_SmokeTest_UnexpectedOutput(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    false,
	}
	fr := &fakeRun{responses: []fakeRunResponse{
		{exitCode: 0},                        // pull succeeds
		{exitCode: 0, stdout: "hello world"}, // smoke test: non-semver output
	}}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps)

	// Warning path must still return nil (success exit).
	if err != nil {
		t.Errorf("want nil (success), got %v", err)
	}
	gotStdout := outBuf.String()
	if !strings.Contains(gotStdout, "Sandbox smoke test warning — unexpected version output: hello world.") {
		t.Errorf("want warning message in stdout, got %q", gotStdout)
	}
	if !strings.Contains(gotStdout, "Sandbox ready.") {
		t.Errorf("want 'Sandbox ready.' after warning, got %q", gotStdout)
	}
}

// TestCreateSandbox_SmokeTest_Success verifies the "Sandbox verified:" success
// line with injected uid/gid values.
func TestCreateSandbox_SmokeTest_Success(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    false,
	}
	fr := &fakeRun{responses: []fakeRunResponse{
		{exitCode: 0},                           // pull succeeds
		{exitCode: 0, stdout: "claude 2.1.101"}, // smoke test: valid version
	}}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps)

	if err != nil {
		t.Errorf("want nil, got %v", err)
	}
	if !strings.Contains(outBuf.String(), "Sandbox verified: claude 2.1.101 under UID 501:20.") {
		t.Errorf("want verified message with uid/gid, got stdout: %q", outBuf.String())
	}
	if !strings.Contains(outBuf.String(), "Sandbox ready.") {
		t.Errorf("want 'Sandbox ready.', got stdout: %q", outBuf.String())
	}
}

// TestCreateSandbox_SmokeTest_VersionFromStderr verifies that a version string
// emitted on stderr (not stdout) is still accepted.
func TestCreateSandbox_SmokeTest_VersionFromStderr(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    true,
	}
	// Only smoke test call (image present, no force).
	fr := &fakeRun{responses: []fakeRunResponse{
		{exitCode: 0, stderr: "claude 2.1.101"}, // version on stderr
	}}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps)

	if err != nil {
		t.Errorf("want nil, got %v", err)
	}
	if !strings.Contains(outBuf.String(), "Sandbox verified: claude 2.1.101 under UID 501:20.") {
		t.Errorf("want verified message from stderr output, got stdout: %q", outBuf.String())
	}
}

// TestCreateSandbox_ImagePresentErr verifies that when SandboxImagePresent returns
// an error, the command prints the error to stderr and returns errSilentExit
// without making any docker exec calls.
func TestCreateSandbox_ImagePresentErr(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresentErr: errors.New("inspect failed"),
	}
	fr := &fakeRun{}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "Failed to check sandbox image: inspect failed") {
		t.Errorf("want image-check-error message in stderr, got %q", errBuf.String())
	}
	if len(fr.calls) != 0 {
		t.Errorf("no docker exec calls expected, got %d: %v", len(fr.calls), fr.calls)
	}
}

// TestCreateSandbox_PullExecError verifies that when dockerRun returns an
// exec-level error during pull (distinct from a non-zero exit code), the command
// prints the error to stderr, returns errSilentExit, and does not run the smoke test.
func TestCreateSandbox_PullExecError(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    false,
	}
	fr := &fakeRun{responses: []fakeRunResponse{
		{err: errors.New("exec: not found")},
	}}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "Failed to pull sandbox image: exec: not found") {
		t.Errorf("want pull-exec-error message in stderr, got %q", errBuf.String())
	}
	// Smoke test must not have run.
	for _, call := range fr.calls {
		if len(call) > 1 && call[1] == "run" {
			t.Errorf("smoke test must not run after pull exec error; calls=%v", fr.calls)
		}
	}
}

// TestCreateSandbox_SmokeExecError verifies that when dockerRun returns an
// exec-level error during the smoke test, the command prints the error to stderr
// and returns errSilentExit.
func TestCreateSandbox_SmokeExecError(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    true,
	}
	fr := &fakeRun{responses: []fakeRunResponse{
		{err: errors.New("container runtime error")},
	}}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "Sandbox smoke test failed: container runtime error") {
		t.Errorf("want smoke-exec-error message in stderr, got %q", errBuf.String())
	}
}

// TestCreateSandbox_PullFails_StderrForwarded verifies that when docker pull
// exits non-zero with stderr output, the captured stderr is forwarded to the
// user after the "Failed to pull" message.
func TestCreateSandbox_PullFails_StderrForwarded(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    false,
	}
	fr := &fakeRun{responses: []fakeRunResponse{
		{exitCode: 1, stderr: "Error: manifest unknown"},
	}}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	got := errBuf.String()
	if !strings.Contains(got, "Failed to pull sandbox image.") {
		t.Errorf("want pull-failure message in stderr, got %q", got)
	}
	if !strings.Contains(got, "Error: manifest unknown") {
		t.Errorf("want captured pull stderr forwarded to user, got %q", got)
	}
}

// TestCreateSandbox_PullFails_EmptyStderr verifies that when docker pull exits
// non-zero but produces no stderr, only "Failed to pull sandbox image." appears
// (no extra blank line or empty content appended).
func TestCreateSandbox_PullFails_EmptyStderr(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    false,
	}
	fr := &fakeRun{responses: []fakeRunResponse{
		{exitCode: 1},
	}}
	deps := newTestDeps(prober, fr, &outBuf, &errBuf)

	err := runCmd(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if errBuf.String() != "Failed to pull sandbox image.\n" {
		t.Errorf("want exactly %q in stderr, got %q", "Failed to pull sandbox image.\n", errBuf.String())
	}
}

// TestCreateSandbox_SmokeTest_ArgsIncludeUID verifies that the smoke test docker
// run argv includes -u <uid>:<gid> using the injected uid/gid values.
func TestCreateSandbox_SmokeTest_ArgsIncludeUID(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    true,
	}
	fr := &fakeRun{responses: []fakeRunResponse{
		{exitCode: 0, stdout: "claude 2.1.101"},
	}}
	deps := &createSandboxDeps{
		prober:    prober,
		dockerRun: fr.run,
		uid:       1000,
		gid:       1001,
		stdout:    &outBuf,
		stderr:    &errBuf,
	}

	err := runCmd(deps)

	if err != nil {
		t.Errorf("want nil, got %v", err)
	}
	// Find the docker run call and verify -u flag.
	found := false
	for _, call := range fr.calls {
		if len(call) > 1 && call[1] == "run" {
			for i, a := range call {
				if a == "-u" && i+1 < len(call) && call[i+1] == "1000:1001" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("want -u 1000:1001 in smoke test argv, calls=%v", fr.calls)
	}
}
