package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newShellTestDeps builds a sandboxShellDeps with captured stdout/stderr
// writers and the provided prober/fakeRun/fakeInteractive, projectDir, and
// profileDir.
func newShellTestDeps(prober *fakeProber, fr *fakeRun, fi *fakeInteractive, projectDir, profileDir string, outBuf, errBuf *bytes.Buffer) *sandboxShellDeps {
	return &sandboxShellDeps{
		prober:            prober,
		dockerInteractive: fi.run,
		dockerRun:         fr.run,
		uid:               501,
		gid:               20,
		projectDir:        projectDir,
		profileDir:        profileDir,
		stdin:             strings.NewReader(""),
		stdout:            outBuf,
		stderr:            errBuf,
	}
}

func TestSandboxShell_DockerBinaryMissing(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{binaryAvailable: false}
	fr := &fakeRun{}
	fi := &fakeInteractive{}
	deps := newShellTestDeps(prober, fr, fi, t.TempDir(), t.TempDir(), &outBuf, &errBuf)

	err := runSandboxShell(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "Docker is not installed. Install Docker and try again.") {
		t.Errorf("want docker-not-installed message, got stderr: %q", errBuf.String())
	}
	if len(fr.calls)+len(fi.calls) != 0 {
		t.Errorf("no docker calls expected, got run=%d interactive=%d", len(fr.calls), len(fi.calls))
	}
}

func TestSandboxShell_DaemonUnreachable(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		daemonErr:       errors.New("connection refused"),
	}
	fr := &fakeRun{}
	fi := &fakeInteractive{}
	deps := newShellTestDeps(prober, fr, fi, t.TempDir(), t.TempDir(), &outBuf, &errBuf)

	err := runSandboxShell(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "Docker is installed but the daemon isn't running. Start Docker and try again.") {
		t.Errorf("want daemon-unreachable message, got stderr: %q", errBuf.String())
	}
}

func TestSandboxShell_ImagePresentErr(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresentErr: errors.New("inspect failed"),
	}
	fr := &fakeRun{}
	fi := &fakeInteractive{}
	deps := newShellTestDeps(prober, fr, fi, t.TempDir(), t.TempDir(), &outBuf, &errBuf)

	err := runSandboxShell(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "Failed to check sandbox image: inspect failed") {
		t.Errorf("want image-check-error message in stderr, got %q", errBuf.String())
	}
}

func TestSandboxShell_ImagePresent_RunsBash(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{binaryAvailable: true, imagePresent: true}
	fr := &fakeRun{}
	fi := &fakeInteractive{responses: []fakeRunResponse{{exitCode: 0}}}
	deps := newShellTestDeps(prober, fr, fi, t.TempDir(), t.TempDir(), &outBuf, &errBuf)

	err := runSandboxShell(deps)
	if err != nil {
		t.Errorf("want nil, got %v", err)
	}
	if len(fr.calls) != 0 {
		t.Errorf("no pull expected when image present; got run calls %v", fr.calls)
	}
	if len(fi.calls) != 1 {
		t.Fatalf("want exactly 1 interactive call, got %d: %v", len(fi.calls), fi.calls)
	}
}

func TestSandboxShell_ImageAbsent_PullThenBash(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{binaryAvailable: true, imagePresent: false}
	fr := &fakeRun{responses: []fakeRunResponse{{exitCode: 0}}}
	fi := &fakeInteractive{responses: []fakeRunResponse{{exitCode: 0}}}
	deps := newShellTestDeps(prober, fr, fi, t.TempDir(), t.TempDir(), &outBuf, &errBuf)

	err := runSandboxShell(deps)
	if err != nil {
		t.Errorf("want nil, got %v", err)
	}
	if !strings.Contains(outBuf.String(), "Sandbox image not found") {
		t.Errorf("want \"Sandbox image not found\" in stdout, got %q", outBuf.String())
	}
	if len(fr.calls) != 1 || len(fr.calls[0]) < 2 || fr.calls[0][1] != "pull" {
		t.Errorf("want exactly one docker pull call, got %v", fr.calls)
	}
	if len(fi.calls) != 1 {
		t.Fatalf("want exactly 1 interactive call after pull, got %d: %v", len(fi.calls), fi.calls)
	}
}

func TestSandboxShell_PullFails(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{binaryAvailable: true, imagePresent: false}
	fr := &fakeRun{responses: []fakeRunResponse{
		{exitCode: 1, stderr: "Error: manifest unknown"},
	}}
	fi := &fakeInteractive{}
	deps := newShellTestDeps(prober, fr, fi, t.TempDir(), t.TempDir(), &outBuf, &errBuf)

	err := runSandboxShell(deps)
	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "Failed to pull sandbox image.") {
		t.Errorf("want pull-failure message in stderr, got %q", errBuf.String())
	}
	if len(fi.calls) != 0 {
		t.Errorf("interactive must not run after pull failure; got %d calls", len(fi.calls))
	}
}

func TestSandboxShell_ProfileDirAutoCreated(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{binaryAvailable: true, imagePresent: true}
	fr := &fakeRun{}
	fi := &fakeInteractive{responses: []fakeRunResponse{{exitCode: 0}}}

	profileDir := filepath.Join(t.TempDir(), "nested", "claude")
	deps := newShellTestDeps(prober, fr, fi, t.TempDir(), profileDir, &outBuf, &errBuf)

	err := runSandboxShell(deps)
	if err != nil {
		t.Fatalf("want nil, got %v (stderr=%q)", err, errBuf.String())
	}

	info, statErr := os.Stat(profileDir)
	if statErr != nil {
		t.Fatalf("profile dir not created: %v", statErr)
	}
	if !info.IsDir() {
		t.Errorf("profile path is not a directory: mode=%v", info.Mode())
	}
}

func TestSandboxShell_NonZeroInteractiveExit(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{binaryAvailable: true, imagePresent: true}
	fr := &fakeRun{}
	fi := &fakeInteractive{responses: []fakeRunResponse{{exitCode: 1}}}
	deps := newShellTestDeps(prober, fr, fi, t.TempDir(), t.TempDir(), &outBuf, &errBuf)

	err := runSandboxShell(deps)
	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit on non-zero interactive exit, got %v", err)
	}
}

func TestSandboxShell_ArgsIncludeBindMountsAndBash(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{binaryAvailable: true, imagePresent: true}
	fr := &fakeRun{}
	fi := &fakeInteractive{responses: []fakeRunResponse{{exitCode: 0}}}
	projectDir := t.TempDir()
	profileDir := t.TempDir()
	deps := newShellTestDeps(prober, fr, fi, projectDir, profileDir, &outBuf, &errBuf)

	err := runSandboxShell(deps)
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if len(fi.calls) != 1 {
		t.Fatalf("want 1 interactive call, got %d", len(fi.calls))
	}
	argv := fi.calls[0]
	joined := strings.Join(argv, " ")

	wants := []string{
		"-it",
		"--rm",
		"--init",
		"-u 501:20",
		"type=bind,source=" + projectDir + ",target=/home/agent/workspace",
		"type=bind,source=" + profileDir + ",target=/home/agent/.claude",
		"CLAUDE_CONFIG_DIR=/home/agent/.claude",
	}
	for _, want := range wants {
		if !strings.Contains(joined, want) {
			t.Errorf("argv missing %q; got %v", want, argv)
		}
	}

	if argv[len(argv)-1] != "bash" {
		t.Errorf("argv must end with 'bash', got tail %q", argv[len(argv)-3:])
	}
}
