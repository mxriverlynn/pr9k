package scripts_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot resolves the repo root relative to this source file
// (ralph-tui/internal/scripts/ → three levels up).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..", "..")
}

func scriptPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "scripts", "post_issue_summary")
}

// fakeSentinelGh creates a fake `gh` that touches a sentinel file when invoked.
func fakeSentinelGh(t *testing.T) (ghDir, sentinel string) {
	t.Helper()
	dir := t.TempDir()
	sentinel = filepath.Join(dir, "gh.called")
	script := "#!/usr/bin/env bash\ntouch " + sentinel + "\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	return dir, sentinel
}

// recordingGh creates a fake `gh` that appends each argv entry (one per line) to a record file.
func recordingGh(t *testing.T) (ghDir, record string) {
	t.Helper()
	dir := t.TempDir()
	record = filepath.Join(dir, "gh.argv")
	script := "#!/usr/bin/env bash\nprintf '%s\\n' \"$@\" >> " + record + "\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0755); err != nil {
		t.Fatalf("write recording gh: %v", err)
	}
	return dir, record
}

// runScript runs the post_issue_summary script via bash with the given args,
// working directory, and PATH prefix. Returns stdout, stderr, and exit code.
func runScript(t *testing.T, args []string, workDir, pathPrefix string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmdArgs := append([]string{scriptPath(t)}, args...)
	cmd := exec.Command("bash", cmdArgs...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "PATH="+pathPrefix+":"+os.Getenv("PATH"))
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return
}

// TP-002: missing progress.txt → exit 0, gh not invoked.
func TestPostIssueSummary_MissingProgressFile(t *testing.T) {
	workDir := t.TempDir()
	ghDir, sentinel := fakeSentinelGh(t)

	_, _, exitCode := runScript(t, []string{"999"}, workDir, ghDir)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("gh was invoked but should not have been")
	}
}

// TP-003: empty progress.txt → exit 0, gh not invoked.
func TestPostIssueSummary_EmptyProgressFile(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "progress.txt"), []byte{}, 0644); err != nil {
		t.Fatalf("write progress.txt: %v", err)
	}
	ghDir, sentinel := fakeSentinelGh(t)

	_, _, exitCode := runScript(t, []string{"999"}, workDir, ghDir)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("gh was invoked but should not have been")
	}
}

// TP-004: populated progress.txt → gh issue comment with exact heading + body, no trailing newline.
func TestPostIssueSummary_PopulatedProgressFile(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "progress.txt"), []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatalf("write progress.txt: %v", err)
	}
	ghDir, record := recordingGh(t)

	_, _, exitCode := runScript(t, []string{"42"}, workDir, ghDir)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("read gh.argv: %v", err)
	}
	// printf '%s\n' appends a newline after each arg, including the body arg.
	wantBody := "### Ralph iteration summary\n\nhello\nworld"
	want := "issue\ncomment\n42\n--body\n" + wantBody + "\n"
	if got := string(data); got != want {
		t.Fatalf("gh argv:\ngot:  %q\nwant: %q", got, want)
	}
}

// TP-005: missing issue-id argument → non-zero exit, stderr contains usage string, gh not invoked.
func TestPostIssueSummary_MissingArg(t *testing.T) {
	workDir := t.TempDir()
	ghDir, sentinel := fakeSentinelGh(t)

	_, stderr, exitCode := runScript(t, nil, workDir, ghDir)

	if exitCode == 0 {
		t.Fatal("exit code = 0, want non-zero")
	}
	if !strings.Contains(stderr, "Usage: post_issue_summary") {
		t.Fatalf("stderr %q does not contain %q", stderr, "Usage: post_issue_summary")
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("gh was invoked but should not have been")
	}
}
