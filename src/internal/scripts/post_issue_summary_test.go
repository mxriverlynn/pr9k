package scripts_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot resolves the repo root relative to this source file
// (src/internal/scripts/ → three levels up).
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
	return filepath.Join(repoRoot(t), "workflow", "scripts", "post_issue_summary")
}

// fakeSentinelGh creates a fake `gh` that touches a sentinel file when invoked.
func fakeSentinelGh(t *testing.T) (ghDir, sentinel string) {
	t.Helper()
	dir := t.TempDir()
	sentinel = filepath.Join(dir, "gh.called")
	script := fmt.Sprintf("#!/usr/bin/env bash\ntouch %q\n", sentinel)
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	return dir, sentinel
}

// recordingGh creates a fake `gh` that appends each argv entry (one per line) to a record file.
func recordingGh(t *testing.T) (ghDir, record string) {
	t.Helper()
	dir := t.TempDir()
	record = filepath.Join(dir, "gh.argv")
	script := fmt.Sprintf("#!/usr/bin/env bash\nprintf '%%s\\n' \"$@\" >> %q\n", record)
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write recording gh: %v", err)
	}
	return dir, record
}

// runScript runs the post_issue_summary script via bash with the given args,
// working directory, and PATH prefix. Returns stderr and exit code.
func runScript(t *testing.T, args []string, workDir, pathPrefix string) (stderr string, exitCode int) {
	t.Helper()
	cmdArgs := append([]string{scriptPath(t)}, args...)
	cmd := exec.Command("bash", cmdArgs...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "PATH="+pathPrefix+":"+os.Getenv("PATH"))
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	err := cmd.Run()
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

// TP-002: missing progress.txt → exit 0, gh not invoked.
func TestPostIssueSummary_MissingProgressFile(t *testing.T) {
	workDir := t.TempDir()
	ghDir, sentinel := fakeSentinelGh(t)

	_, exitCode := runScript(t, []string{"999"}, workDir, ghDir)

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
	if err := os.WriteFile(filepath.Join(workDir, "progress.txt"), []byte{}, 0o644); err != nil {
		t.Fatalf("write progress.txt: %v", err)
	}
	ghDir, sentinel := fakeSentinelGh(t)

	_, exitCode := runScript(t, []string{"999"}, workDir, ghDir)

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
	if err := os.WriteFile(filepath.Join(workDir, "progress.txt"), []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatalf("write progress.txt: %v", err)
	}
	ghDir, record := recordingGh(t)

	_, exitCode := runScript(t, []string{"42"}, workDir, ghDir)

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

	stderr, exitCode := runScript(t, nil, workDir, ghDir)

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

// TP-001: non-empty .pr9k/iteration.jsonl is preferred over progress.txt.
// The body must be built from the JSONL; progress.txt content must not appear.
func TestPostIssueSummary_JSONLPreference(t *testing.T) {
	workDir := t.TempDir()
	cacheDir := filepath.Join(workDir, ".pr9k")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir .pr9k: %v", err)
	}
	jsonl := `{"step_name":"feature-work","status":"done","schema_version":1,"iteration_num":1,"duration_s":1.5}` + "\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "iteration.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write iteration.jsonl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "progress.txt"), []byte("OLD CONTENT"), 0o644); err != nil {
		t.Fatalf("write progress.txt: %v", err)
	}

	ghDir, record := recordingGh(t)
	_, exitCode := runScript(t, []string{"42"}, workDir, ghDir)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("read gh.argv: %v", err)
	}
	wantBody := "### Ralph iteration summary\n\n- feature-work [done]"
	want := "issue\ncomment\n42\n--body\n" + wantBody + "\n"
	if got := string(data); got != want {
		t.Fatalf("gh argv:\ngot:  %q\nwant: %q", got, want)
	}
	if strings.Contains(string(data), "OLD CONTENT") {
		t.Error("progress.txt content must not appear when iteration.jsonl is present")
	}
}

// TP-002: jq formats each JSONL line into "- <name> [<status>]" with an optional
// " — <notes>" suffix. Empty or absent notes must produce no suffix.
func TestPostIssueSummary_JSONLNotesFormatting(t *testing.T) {
	workDir := t.TempDir()
	cacheDir := filepath.Join(workDir, ".pr9k")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir .pr9k: %v", err)
	}
	lines := strings.Join([]string{
		`{"step_name":"a","status":"done"}`,
		`{"step_name":"b","status":"failed","notes":"timed out"}`,
		`{"step_name":"c","status":"done","notes":""}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "iteration.jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatalf("write iteration.jsonl: %v", err)
	}

	ghDir, record := recordingGh(t)
	_, exitCode := runScript(t, []string{"42"}, workDir, ghDir)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("read gh.argv: %v", err)
	}
	body := string(data)
	for _, wantLine := range []string{"- a [done]", "- b [failed] — timed out", "- c [done]"} {
		if !strings.Contains(body, wantLine) {
			t.Errorf("gh body does not contain %q; got %q", wantLine, body)
		}
	}
	// Empty notes must not produce a suffix.
	if strings.Contains(body, "- c [done] —") {
		t.Errorf("empty notes must produce no suffix; got %q", body)
	}
}

// TP-003: When iteration.jsonl is absent or empty, the script falls back to
// progress.txt for backward compatibility.
func TestPostIssueSummary_ProgressFileFallback(t *testing.T) {
	t.Run("no_cache_dir", func(t *testing.T) {
		workDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(workDir, "progress.txt"), []byte("progress content\n"), 0o644); err != nil {
			t.Fatalf("write progress.txt: %v", err)
		}

		ghDir, record := recordingGh(t)
		_, exitCode := runScript(t, []string{"42"}, workDir, ghDir)
		if exitCode != 0 {
			t.Fatalf("exit code = %d, want 0", exitCode)
		}

		data, err := os.ReadFile(record)
		if err != nil {
			t.Fatalf("read gh.argv: %v", err)
		}
		wantBody := "### Ralph iteration summary\n\nprogress content"
		want := "issue\ncomment\n42\n--body\n" + wantBody + "\n"
		if got := string(data); got != want {
			t.Fatalf("gh argv:\ngot:  %q\nwant: %q", got, want)
		}
	})

	t.Run("empty_jsonl", func(t *testing.T) {
		workDir := t.TempDir()
		cacheDir := filepath.Join(workDir, ".pr9k")
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			t.Fatalf("mkdir .pr9k: %v", err)
		}
		// Empty JSONL file — must not satisfy the -s test.
		if err := os.WriteFile(filepath.Join(cacheDir, "iteration.jsonl"), []byte{}, 0o644); err != nil {
			t.Fatalf("write empty iteration.jsonl: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workDir, "progress.txt"), []byte("progress content\n"), 0o644); err != nil {
			t.Fatalf("write progress.txt: %v", err)
		}

		ghDir, record := recordingGh(t)
		_, exitCode := runScript(t, []string{"42"}, workDir, ghDir)
		if exitCode != 0 {
			t.Fatalf("exit code = %d, want 0", exitCode)
		}

		data, err := os.ReadFile(record)
		if err != nil {
			t.Fatalf("read gh.argv: %v", err)
		}
		wantBody := "### Ralph iteration summary\n\nprogress content"
		want := "issue\ncomment\n42\n--body\n" + wantBody + "\n"
		if got := string(data); got != want {
			t.Fatalf("gh argv:\ngot:  %q\nwant: %q", got, want)
		}
	})
}

// TP-008: Empty .pr9k/iteration.jsonl with no progress.txt → exit 0,
// gh not invoked. Catches regressions if the -s guard is accidentally inverted.
func TestPostIssueSummary_EmptyJSONLNoProgress(t *testing.T) {
	workDir := t.TempDir()
	cacheDir := filepath.Join(workDir, ".pr9k")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir .pr9k: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "iteration.jsonl"), []byte{}, 0o644); err != nil {
		t.Fatalf("write empty iteration.jsonl: %v", err)
	}

	ghDir, sentinel := fakeSentinelGh(t)
	_, exitCode := runScript(t, []string{"999"}, workDir, ghDir)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("gh was invoked but should not have been")
	}
}
