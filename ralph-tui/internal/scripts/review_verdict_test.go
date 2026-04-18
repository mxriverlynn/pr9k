package scripts_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func reviewVerdictPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "scripts", "review_verdict")
}

// runVerdict runs the review_verdict script via bash in workDir and returns
// stdout, stderr, and exit code.
func runVerdict(t *testing.T, workDir string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command("bash", reviewVerdictPath(t))
	cmd.Dir = workDir
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

// writeReview writes content to code-review.md in workDir.
func writeReview(t *testing.T, workDir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workDir, "code-review.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write code-review.md: %v", err)
	}
}

// TestReviewVerdict_ExactSentinel: "NOTHING-TO-FIX\n" → empty stdout (skip signal).
func TestReviewVerdict_ExactSentinel(t *testing.T) {
	workDir := t.TempDir()
	writeReview(t, workDir, "NOTHING-TO-FIX\n")

	stdout, _, exitCode := runVerdict(t, workDir)
	if exitCode != 0 {
		t.Fatalf("exit code: want 0, got %d", exitCode)
	}
	if stdout != "" {
		t.Errorf("stdout: want empty, got %q", stdout)
	}
}

// TestReviewVerdict_WithTrailingWhitespace: trailing spaces should still match.
func TestReviewVerdict_WithTrailingWhitespace(t *testing.T) {
	workDir := t.TempDir()
	writeReview(t, workDir, "NOTHING-TO-FIX   \n")

	stdout, _, exitCode := runVerdict(t, workDir)
	if exitCode != 0 {
		t.Fatalf("exit code: want 0, got %d", exitCode)
	}
	if stdout != "" {
		t.Errorf("stdout: want empty (trailing whitespace stripped), got %q", stdout)
	}
}

// TestReviewVerdict_WithLeadingHeading: "# Review\n\nNOTHING-TO-FIX" → empty stdout.
func TestReviewVerdict_WithLeadingHeading(t *testing.T) {
	workDir := t.TempDir()
	writeReview(t, workDir, "# Review\n\nNOTHING-TO-FIX\n")

	stdout, _, exitCode := runVerdict(t, workDir)
	if exitCode != 0 {
		t.Fatalf("exit code: want 0, got %d", exitCode)
	}
	if stdout != "" {
		t.Errorf("stdout: want empty (heading stripped), got %q", stdout)
	}
}

// TestReviewVerdict_SentinelInProse: sentinel quoted inside prose → "yes".
func TestReviewVerdict_SentinelInProse(t *testing.T) {
	workDir := t.TempDir()
	writeReview(t, workDir, "# Review\n\nSome issues found. `NOTHING-TO-FIX` is not the verdict here.\n")

	stdout, _, exitCode := runVerdict(t, workDir)
	if exitCode != 0 {
		t.Fatalf("exit code: want 0, got %d", exitCode)
	}
	if stdout != "yes" {
		t.Errorf("stdout: want %q, got %q", "yes", stdout)
	}
}

// TestReviewVerdict_EmptyFile: empty code-review.md → "yes" (fail-safe).
func TestReviewVerdict_EmptyFile(t *testing.T) {
	workDir := t.TempDir()
	writeReview(t, workDir, "")

	stdout, _, exitCode := runVerdict(t, workDir)
	if exitCode != 0 {
		t.Fatalf("exit code: want 0, got %d", exitCode)
	}
	if stdout != "yes" {
		t.Errorf("stdout: want %q (fail-safe), got %q", "yes", stdout)
	}
}

// TestReviewVerdict_WhitespaceOnly: whitespace-only file → "yes" (fail-safe).
func TestReviewVerdict_WhitespaceOnly(t *testing.T) {
	workDir := t.TempDir()
	writeReview(t, workDir, "   \n\n   \n")

	stdout, _, exitCode := runVerdict(t, workDir)
	if exitCode != 0 {
		t.Fatalf("exit code: want 0, got %d", exitCode)
	}
	if stdout != "yes" {
		t.Errorf("stdout: want %q (fail-safe for whitespace-only), got %q", "yes", stdout)
	}
}

// TestReviewVerdict_MissingFile: no code-review.md → "yes" (fail-safe).
func TestReviewVerdict_MissingFile(t *testing.T) {
	workDir := t.TempDir()
	// no file written

	stdout, _, exitCode := runVerdict(t, workDir)
	if exitCode != 0 {
		t.Fatalf("exit code: want 0, got %d", exitCode)
	}
	if stdout != "yes" {
		t.Errorf("stdout: want %q (fail-safe for missing file), got %q", "yes", stdout)
	}
}

// TestReviewVerdict_NormalReviewBody: real review content → "yes".
func TestReviewVerdict_NormalReviewBody(t *testing.T) {
	workDir := t.TempDir()
	writeReview(t, workDir, "# Code Review\n\n## Issues\n\n- WARN-001: foo is unguarded\n- SUGG-001: rename bar\n")

	stdout, _, exitCode := runVerdict(t, workDir)
	if exitCode != 0 {
		t.Fatalf("exit code: want 0, got %d", exitCode)
	}
	if stdout != "yes" {
		t.Errorf("stdout: want %q, got %q", "yes", stdout)
	}
}
