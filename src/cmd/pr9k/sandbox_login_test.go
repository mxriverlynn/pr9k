package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeInteractive holds a sequence of canned responses for dockerInteractiveFunc
// calls. Stdin is recorded for inspection but never read (tests assert argv,
// not stdin payload).
type fakeInteractive struct {
	calls     [][]string
	responses []fakeRunResponse
}

func (f *fakeInteractive) run(args []string, _ io.Reader, stdout, stderr io.Writer) (int, error) {
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

// newLoginTestDeps builds a sandboxLoginDeps with captured stdout/stderr
// writers and the provided prober/fakeRun/fakeInteractive and profileDir.
func newLoginTestDeps(prober *fakeProber, fr *fakeRun, fi *fakeInteractive, profileDir string, outBuf, errBuf *bytes.Buffer) *sandboxLoginDeps {
	return &sandboxLoginDeps{
		prober:            prober,
		dockerInteractive: fi.run,
		dockerRun:         fr.run,
		uid:               501,
		gid:               20,
		profileDir:        profileDir,
		stdin:             strings.NewReader(""),
		stdout:            outBuf,
		stderr:            errBuf,
	}
}

// runLoginCmd builds a command from deps and executes it. Returns the
// error from cobra's Execute (which equals RunE's return value due to SilenceErrors).
func runLoginCmd(deps *sandboxLoginDeps) error {
	cmd := newSandboxLoginCmdWith(deps)
	cmd.SetArgs([]string{})
	return cmd.Execute()
}

func TestSandboxLogin_DockerBinaryMissing(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{binaryAvailable: false}
	fr := &fakeRun{}
	fi := &fakeInteractive{}
	deps := newLoginTestDeps(prober, fr, fi, t.TempDir(), &outBuf, &errBuf)

	err := runLoginCmd(deps)

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

func TestSandboxLogin_DaemonUnreachable(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		daemonErr:       errors.New("connection refused"),
	}
	fr := &fakeRun{}
	fi := &fakeInteractive{}
	deps := newLoginTestDeps(prober, fr, fi, t.TempDir(), &outBuf, &errBuf)

	err := runLoginCmd(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "Docker is installed but the daemon isn't running. Start Docker and try again.") {
		t.Errorf("want daemon-unreachable message, got stderr: %q", errBuf.String())
	}
}

func TestSandboxLogin_ImagePresentErr(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresentErr: errors.New("inspect failed"),
	}
	fr := &fakeRun{}
	fi := &fakeInteractive{}
	deps := newLoginTestDeps(prober, fr, fi, t.TempDir(), &outBuf, &errBuf)

	err := runLoginCmd(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "Failed to check sandbox image: inspect failed") {
		t.Errorf("want image-check-error message in stderr, got %q", errBuf.String())
	}
	if len(fr.calls)+len(fi.calls) != 0 {
		t.Errorf("no docker calls expected, got run=%d interactive=%d", len(fr.calls), len(fi.calls))
	}
}

func TestSandboxLogin_ImagePresent_RunsInteractive(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    true,
	}
	fr := &fakeRun{}
	fi := &fakeInteractive{responses: []fakeRunResponse{{exitCode: 0}}}
	deps := newLoginTestDeps(prober, fr, fi, t.TempDir(), &outBuf, &errBuf)

	err := runLoginCmd(deps)

	if err != nil {
		t.Errorf("want nil, got %v", err)
	}
	if len(fr.calls) != 0 {
		t.Errorf("no pull expected when image present; got run calls %v", fr.calls)
	}
	if len(fi.calls) != 1 {
		t.Fatalf("want exactly 1 interactive call, got %d: %v", len(fi.calls), fi.calls)
	}
	if strings.Contains(outBuf.String(), "Sandbox image not found") {
		t.Errorf("verbose pull note should NOT appear when image present, got stdout: %q", outBuf.String())
	}
}

func TestSandboxLogin_ImageAbsent_PullThenInteractive(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    false,
	}
	fr := &fakeRun{responses: []fakeRunResponse{{exitCode: 0}}}
	fi := &fakeInteractive{responses: []fakeRunResponse{{exitCode: 0}}}
	deps := newLoginTestDeps(prober, fr, fi, t.TempDir(), &outBuf, &errBuf)

	err := runLoginCmd(deps)

	if err != nil {
		t.Errorf("want nil, got %v", err)
	}
	if !strings.Contains(outBuf.String(), "Sandbox image not found") {
		t.Errorf("want \"Sandbox image not found\" in stdout, got %q", outBuf.String())
	}
	if !strings.Contains(outBuf.String(), "'pr9k sandbox create'") {
		t.Errorf("want \"'pr9k sandbox create'\" in stdout, got %q", outBuf.String())
	}
	if len(fr.calls) != 1 || len(fr.calls[0]) < 2 || fr.calls[0][1] != "pull" {
		t.Errorf("want exactly one docker pull call, got %v", fr.calls)
	}
	if len(fi.calls) != 1 {
		t.Fatalf("want exactly 1 interactive call after pull, got %d: %v", len(fi.calls), fi.calls)
	}
}

func TestSandboxLogin_PullFails(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    false,
	}
	fr := &fakeRun{responses: []fakeRunResponse{
		{exitCode: 1, stderr: "Error: manifest unknown"},
	}}
	fi := &fakeInteractive{}
	deps := newLoginTestDeps(prober, fr, fi, t.TempDir(), &outBuf, &errBuf)

	err := runLoginCmd(deps)

	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "Failed to pull sandbox image.") {
		t.Errorf("want pull-failure message in stderr, got %q", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "Error: manifest unknown") {
		t.Errorf("want captured pull stderr forwarded, got %q", errBuf.String())
	}
	if len(fi.calls) != 0 {
		t.Errorf("interactive must not run after pull failure; got %d calls", len(fi.calls))
	}
}

func TestSandboxLogin_ProfileDirAutoCreated(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    true,
	}
	fr := &fakeRun{}
	fi := &fakeInteractive{responses: []fakeRunResponse{{exitCode: 0}}}

	// profileDir is under a temp dir but does not exist yet.
	profileDir := filepath.Join(t.TempDir(), "nested", "claude")
	deps := newLoginTestDeps(prober, fr, fi, profileDir, &outBuf, &errBuf)

	err := runLoginCmd(deps)
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

func TestSandboxLogin_ProfileDirIsFile(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    true,
	}

	// Create a file at the profile path so MkdirAll fails.
	parent := t.TempDir()
	profilePath := filepath.Join(parent, "claude-as-a-file")
	if err := os.WriteFile(profilePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	fr := &fakeRun{}
	fi := &fakeInteractive{}
	deps := newLoginTestDeps(prober, fr, fi, profilePath, &outBuf, &errBuf)

	err := runLoginCmd(deps)
	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit, got %v", err)
	}
	if !strings.Contains(errBuf.String(), "Failed to prepare profile directory") {
		t.Errorf("want profile-dir-prep error in stderr, got %q", errBuf.String())
	}
	if len(fi.calls) != 0 {
		t.Errorf("interactive must not run when profile dir prep fails; got %d calls", len(fi.calls))
	}
}

func TestSandboxLogin_NonZeroInteractiveExit(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    true,
	}
	fr := &fakeRun{}
	fi := &fakeInteractive{responses: []fakeRunResponse{{exitCode: 1}}}
	deps := newLoginTestDeps(prober, fr, fi, t.TempDir(), &outBuf, &errBuf)

	err := runLoginCmd(deps)
	if !errors.Is(err, errSilentExit) {
		t.Errorf("want errSilentExit on non-zero interactive exit, got %v", err)
	}
}

func TestSandboxLogin_ArgsIncludeBindMountAndInteractive(t *testing.T) {
	t.Parallel()
	var outBuf, errBuf bytes.Buffer
	prober := &fakeProber{
		binaryAvailable: true,
		imagePresent:    true,
	}
	fr := &fakeRun{}
	fi := &fakeInteractive{responses: []fakeRunResponse{{exitCode: 0}}}
	profileDir := t.TempDir()
	deps := newLoginTestDeps(prober, fr, fi, profileDir, &outBuf, &errBuf)

	err := runLoginCmd(deps)
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if len(fi.calls) != 1 {
		t.Fatalf("want 1 interactive call, got %d", len(fi.calls))
	}
	argv := fi.calls[0]
	joined := strings.Join(argv, " ")

	// Must contain: -it, profile bind-mount, CLAUDE_CONFIG_DIR env, uid mapping.
	wants := []string{
		"-it",
		"--init",
		"--rm",
		"-u 501:20",
		"type=bind,source=" + profileDir + ",target=/home/agent/.claude",
		"CLAUDE_CONFIG_DIR=/home/agent/.claude",
	}
	for _, want := range wants {
		if !strings.Contains(joined, want) {
			t.Errorf("argv missing %q; got %v", want, argv)
		}
	}

	// Must NOT contain: project-dir mount, -p, --permission-mode, -w, --cidfile.
	notWants := []string{
		"--cidfile",
		"-w",
		"--permission-mode",
		"-p",
	}
	for _, notWant := range notWants {
		for _, a := range argv {
			if a == notWant {
				t.Errorf("argv must NOT contain %q; got %v", notWant, argv)
			}
		}
	}

	// argv must end with "claude" (no -p prompt trailer).
	if argv[len(argv)-1] != "claude" {
		t.Errorf("argv must end with 'claude', got tail %q", argv[len(argv)-3:])
	}
}
