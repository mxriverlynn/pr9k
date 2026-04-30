package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// docTestRepoRoot returns the workspace root directory, resolved from this test file's
// absolute path so the test works correctly regardless of the working directory.
func docTestRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	// file is .../src/cmd/pr9k/build_config_test.go
	// three levels up reaches the workspace root
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// readFile reads a file relative to the repo root.
func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("readFile %q: %v", rel, err)
	}
	return string(data)
}

func assertContains(t *testing.T, s, substr, context string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("%s: expected to contain %q", context, substr)
	}
}

func assertNotContains(t *testing.T, s, substr, context string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("%s: expected NOT to contain %q", context, substr)
	}
}

// legacyName is the old binary name assembled at runtime so this test file
// does not itself contain the legacy string literal that the guard tests for.
var legacyName = "ralph" + "-tui"

// TestCIWorkflow_NoStaleRalphTuiReferences asserts that .github/workflows/ci.yml
// has been updated to reference the renamed src/ directory and contains no
// stale path references to the old tool name.
func TestCIWorkflow_NoStaleRalphTuiReferences(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, ".github/workflows/ci.yml")

	if strings.Contains(content, legacyName) {
		t.Errorf(".github/workflows/ci.yml still contains %q — update working-directory and cache-dependency-path to use src/", legacyName)
	}
	if !strings.Contains(content, "working-directory: src") {
		t.Error(".github/workflows/ci.yml does not contain \"working-directory: src\"")
	}
}

// TestMakefile_CopiesConfigJSONToBin pins that the Makefile copies workflow/config.json
// into bin/.pr9k/workflow/ and does not reference the legacy filename in any cp target.
func TestMakefile_CopiesConfigJSONToBin(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "Makefile")

	assertContains(t, content, "cp workflow/config.json bin/.pr9k/workflow/", "Makefile cp target")
	assertNotContains(t, content, legacyConfigName, "Makefile legacy config filename")
}

// TestMakefile_BundleLayoutIsUnderPr9kWorkflow asserts all four bundle-layout lines
// under bin/.pr9k/workflow/ are present and the three legacy top-level positions are absent.
func TestMakefile_BundleLayoutIsUnderPr9kWorkflow(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "Makefile")

	assertContains(t, content, "mkdir -p bin/.pr9k/workflow", "Makefile mkdir target")
	assertContains(t, content, "cp -r workflow/prompts bin/.pr9k/workflow/prompts", "Makefile prompts cp target")
	assertContains(t, content, "cp -r workflow/scripts bin/.pr9k/workflow/scripts", "Makefile scripts cp target")
	assertContains(t, content, "cp ralph-art.txt bin/.pr9k/workflow/", "Makefile ralph-art.txt cp target")

	assertNotContains(t, content, "cp -r prompts bin/prompts", "Makefile legacy prompts position")
	assertNotContains(t, content, "cp -r scripts bin/scripts", "Makefile legacy scripts position")
	// Legacy: "cp ralph-art.txt bin/" followed by newline (not "bin/.pr9k/…")
	assertNotContains(t, content, "cp ralph-art.txt bin/\n", "Makefile legacy ralph-art.txt position")
}

// TestMakefile_BuildsBinaryNamedPr9k asserts that the Makefile builds the pr9k
// binary and references the renamed src/ directory.
func TestMakefile_BuildsBinaryNamedPr9k(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, "Makefile")

	if strings.Contains(content, "./cmd/"+legacyName) {
		t.Errorf("Makefile still references ./cmd/%s — update to ./cmd/pr9k", legacyName)
	}
	if strings.Contains(content, "bin/"+legacyName) {
		t.Errorf("Makefile still references bin/%s — update to bin/pr9k", legacyName)
	}
	if !strings.Contains(content, "./cmd/pr9k") {
		t.Error("Makefile does not reference ./cmd/pr9k as the build target")
	}
	if !strings.Contains(content, "../bin/pr9k") {
		t.Error("Makefile does not reference ../bin/pr9k as the output binary")
	}
}

// TestGitignore_IgnoresPr9kDir asserts .gitignore ignores the runtime-output paths
// under .pr9k/ (logs and iteration log) while leaving .pr9k/workflow/ tracked so
// per-repo workflow overrides can be committed. Line-anchored to reject partial matches.
func TestGitignore_IgnoresPr9kDir(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, ".gitignore")
	assertContains(t, content, "\n.pr9k/logs/\n", ".gitignore .pr9k/logs/ entry")
	assertContains(t, content, "\n.pr9k/iteration.jsonl\n", ".gitignore .pr9k/iteration.jsonl entry")
}

// TestGitignore_PreservesLogsEntry asserts .gitignore preserves the legacy logs/ entry.
func TestGitignore_PreservesLogsEntry(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, ".gitignore")
	assertContains(t, content, "\nlogs/\n", ".gitignore logs/ entry")
}

// TestGitignore_PreservesRalphCacheEntry asserts .gitignore preserves the .ralph-cache/ entry.
func TestGitignore_PreservesRalphCacheEntry(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, ".gitignore")
	assertContains(t, content, "\n.ralph-cache/\n", ".gitignore .ralph-cache/ entry")
}

// TestGitignore_Pr9kDirIsActuallyIgnoredByGit asserts git actually ignores
// runtime-output paths under .pr9k/ but tracks .pr9k/workflow/ (behavioral pin via
// git check-ignore).
func TestGitignore_Pr9kDirIsActuallyIgnoredByGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on $PATH")
	}
	root := docTestRepoRoot(t)

	ignored := []string{".pr9k/logs/ralph-123.log", ".pr9k/iteration.jsonl"}
	for _, path := range ignored {
		cmd := exec.Command("git", "check-ignore", "-q", path)
		cmd.Dir = root
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				t.Errorf("%s is NOT ignored by git — check .gitignore for missing entry", path)
			} else {
				t.Fatalf("git check-ignore %s failed unexpectedly: %v", path, err)
			}
		}
	}

	tracked := ".pr9k/workflow/config.json"
	cmd := exec.Command("git", "check-ignore", "-q", tracked)
	cmd.Dir = root
	err := cmd.Run()
	if err == nil {
		t.Errorf("%s IS ignored by git — .pr9k/workflow/ must remain trackable for per-repo overrides", tracked)
	} else if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("git check-ignore %s failed unexpectedly: %v", tracked, err)
	}
}

// TestGitignore_WorkflowIsTracked_BundleIsIgnored pins that bin/ and .pr9k/ are ignored
// and that the top-level workflow/ source directory is NOT ignored.
func TestGitignore_WorkflowIsTracked_BundleIsIgnored(t *testing.T) {
	root := docTestRepoRoot(t)
	content := readFile(t, root, ".gitignore")

	hasBinEntry := strings.Contains(content, "\nbin/\n") || strings.Contains(content, "\nbin\n")
	if !hasBinEntry {
		t.Error(".gitignore: expected a line matching bin/ or bin")
	}
	assertContains(t, content, ".pr9k/", ".gitignore .pr9k/ entry")

	if strings.Contains(content, "\nworkflow/\n") || strings.Contains(content, "\nworkflow\n") {
		t.Error(".gitignore: workflow/ must NOT be ignored — it is a tracked source directory")
	}
}

// TestGitignore_LegacyDirsAreActuallyIgnoredByGit asserts git actually ignores logs/
// and .ralph-cache/ (behavioral pin via git check-ignore).
func TestGitignore_LegacyDirsAreActuallyIgnoredByGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on $PATH")
	}
	root := docTestRepoRoot(t)
	for _, path := range []string{"logs/anything", ".ralph-cache/anything"} {
		cmd := exec.Command("git", "check-ignore", "-q", path)
		cmd.Dir = root
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				t.Errorf("%q is NOT ignored by git — check .gitignore for missing or negated pattern", path)
			} else {
				t.Fatalf("git check-ignore failed unexpectedly for %q: %v", path, err)
			}
		}
	}
}
