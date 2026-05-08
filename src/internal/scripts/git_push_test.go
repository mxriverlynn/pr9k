package scripts_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitPushScriptPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "workflow", "scripts", "git_push")
}

// fakeGit creates a fake `git` binary in a temp dir that records its argv and
// exits with the given exitCode.
func fakeGit(t *testing.T, exitCode int) (binDir string, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "git.argv")
	script := fmt.Sprintf("#!/usr/bin/env bash\nprintf '%%s\\n' \"$@\" >> %q\nexit %d\n", argsFile, exitCode)
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	return dir, argsFile
}

// runGitPush runs the git_push script with a custom PATH prepended.
func runGitPush(t *testing.T, workDir, extraPath string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command("bash", gitPushScriptPath(t))
	cmd.Dir = workDir
	if extraPath != "" {
		cmd.Env = append(os.Environ(), "PATH="+extraPath+":"+os.Getenv("PATH"))
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stdout = strings.TrimRight(outBuf.String(), "\n")
	stderr = errBuf.String()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return
}

// TestGitPush_PropagatesNonZeroExit verifies that a failing git push causes the
// script to exit non-zero, rather than silently succeeding.
func TestGitPush_PropagatesNonZeroExit(t *testing.T) {
	binDir, _ := fakeGit(t, 1)
	workDir := t.TempDir()

	_, _, exitCode := runGitPush(t, workDir, binDir)
	if exitCode == 0 {
		t.Fatal("expected non-zero exit code when git push fails, got 0")
	}
}

// TestGitPush_UsesSetUpstreamOriginBranch verifies the script calls
// git push --set-upstream origin <branch> so fresh branches succeed without
// manual intervention.
func TestGitPush_UsesSetUpstreamOriginBranch(t *testing.T) {
	binDir, argsFile := fakeGit(t, 0)
	workDir := t.TempDir()

	runGitPush(t, workDir, binDir)

	argsRaw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read git argv file: %v", err)
	}
	args := strings.Split(strings.TrimRight(string(argsRaw), "\n"), "\n")

	// The push invocation must contain --set-upstream and origin.
	wantArgs := []string{"push", "--set-upstream", "origin"}
	for _, want := range wantArgs {
		found := false
		for _, got := range args {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("git push args %v: missing expected argument %q", args, want)
		}
	}
}
