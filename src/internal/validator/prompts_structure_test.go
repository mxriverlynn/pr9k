package validator_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// todoWriteHint is the exact hint line added to every prompt by issue #125.
// TP-004 and TP-005: `strings.Count` against this constant enforces
// byte-identical hint text in every prompt file. If anyone edits the hint in
// one prompt without updating the constant (or vice versa), that file's count
// becomes 0 and TP-001 fails.
// This selector must match the Anthropic tool name exactly — see issue #125.
const todoWriteHint = `You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".`

func promptsDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// test file: src/internal/validator/prompts_structure_test.go
	// repo root: three levels up; prompts live under workflow/
	repoRoot := filepath.Join(filepath.Dir(filename), "..", "..", "..")
	abs, err := filepath.Abs(filepath.Join(repoRoot, "workflow", "prompts"))
	if err != nil {
		t.Fatalf("abs path for prompts: %v", err)
	}
	return abs
}

type promptFile struct {
	Name string
	Body string
}

// loadPromptFiles reads prompt files (those whose first line starts with "@")
// from the prompts directory, sorted by name for deterministic test output.
// Only "@"-prefixed files are included so that future READMEs or draft files
// in that directory do not cause spurious test failures.
//
// Note: this file lives under internal/validator for proximity to
// production_steps_test.go, which also asserts properties of workflow files
// from the repo root — even though it does not import the validator package.
func loadPromptFiles(t *testing.T) []promptFile {
	t.Helper()
	dir := promptsDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read prompts dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var files []promptFile
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		body := string(data)
		if !strings.HasPrefix(body, "@") {
			continue
		}
		files = append(files, promptFile{Name: name, Body: body})
	}
	if len(files) == 0 {
		t.Fatal("no prompt files found")
	}
	return files
}

// TP-001 / TP-006: hint appears exactly once in every prompt file.
func TestPrompts_HintPresentExactlyOnce(t *testing.T) {
	for _, pf := range loadPromptFiles(t) {
		pf := pf
		t.Run(pf.Name, func(t *testing.T) {
			n := strings.Count(pf.Body, todoWriteHint)
			if n != 1 {
				t.Errorf("TodoWrite hint count = %d, want 1 (0 = missing, 2+ = duplicated)", n)
			}
		})
	}
}

// TP-002: hint sits immediately after the first line (@… include).
func TestPrompts_HintPositionAfterInclude(t *testing.T) {
	for _, pf := range loadPromptFiles(t) {
		pf := pf
		t.Run(pf.Name, func(t *testing.T) {
			lines := strings.Split(pf.Body, "\n")
			if len(lines) < 2 {
				t.Errorf("too few lines (%d), expected at least 2", len(lines))
				return
			}
			if !strings.HasPrefix(lines[0], "@") {
				t.Errorf("line 0 = %q, want @… include", lines[0])
			}
			if lines[1] != todoWriteHint {
				t.Errorf("line 1 = %q, want TodoWrite hint", lines[1])
			}
		})
	}
}

var numberedStepRe = regexp.MustCompile(`^(\d+)\. `)

// TP-003: numbered steps start at 1 and are consecutive (no gaps, no renumbering).
func TestPrompts_NumberedStepsConsecutive(t *testing.T) {
	for _, pf := range loadPromptFiles(t) {
		pf := pf
		t.Run(pf.Name, func(t *testing.T) {
			var nums []int
			for _, line := range strings.Split(pf.Body, "\n") {
				if m := numberedStepRe.FindStringSubmatch(line); m != nil {
					n, err := strconv.Atoi(m[1])
					if err != nil {
						t.Fatalf("parse step number %q: %v", m[1], err)
					}
					nums = append(nums, n)
				}
			}
			if len(nums) == 0 {
				t.Errorf("no numbered steps found")
				return
			}
			if nums[0] != 1 {
				t.Errorf("first step = %d, want 1", nums[0])
			}
			for i := 1; i < len(nums); i++ {
				if nums[i] != nums[i-1]+1 {
					t.Errorf("step sequence broken at index %d: %d → %d", i, nums[i-1], nums[i])
				}
			}
		})
	}
}

// TP-005: all six iteration-phase prompts reference {{ISSUE_BODY}} and {{PROJECT_CARD}};
// four of them also reference {{PRE_REVIEW_DIFF}}; feature-work.md and test-planning.md
// must NOT reference {{PRE_REVIEW_DIFF}} (those run before the feature commit exists).
func TestPrompts_IterationPhase_ReferencesContextVars(t *testing.T) {
	dir := promptsDir(t)

	// Prompts that should contain both ISSUE_BODY and PROJECT_CARD (but NOT PRE_REVIEW_DIFF).
	preFeaturePrompts := []string{"feature-work.md", "test-planning.md"}
	// Prompts that should contain ISSUE_BODY, PROJECT_CARD, and PRE_REVIEW_DIFF.
	postFeaturePrompts := []string{"test-writing.md", "code-review-changes.md", "code-review-fixes.md", "update-docs.md"}

	readPrompt := func(t *testing.T, name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(data)
	}

	for _, name := range preFeaturePrompts {
		name := name
		t.Run(name, func(t *testing.T) {
			body := readPrompt(t, name)
			if !strings.Contains(body, "{{ISSUE_BODY}}") {
				t.Errorf("missing {{ISSUE_BODY}}")
			}
			if !strings.Contains(body, "{{PROJECT_CARD}}") {
				t.Errorf("missing {{PROJECT_CARD}}")
			}
			if strings.Contains(body, "{{PRE_REVIEW_DIFF}}") {
				t.Errorf("must not reference {{PRE_REVIEW_DIFF}} (runs before feature commit exists)")
			}
		})
	}

	for _, name := range postFeaturePrompts {
		name := name
		t.Run(name, func(t *testing.T) {
			body := readPrompt(t, name)
			if !strings.Contains(body, "{{ISSUE_BODY}}") {
				t.Errorf("missing {{ISSUE_BODY}}")
			}
			if !strings.Contains(body, "{{PROJECT_CARD}}") {
				t.Errorf("missing {{PROJECT_CARD}}")
			}
			if !strings.Contains(body, "{{PRE_REVIEW_DIFF}}") {
				t.Errorf("missing {{PRE_REVIEW_DIFF}}")
			}
		})
	}
}

// TP-007: every prompt file ends with a newline.
func TestPrompts_TrailingNewline(t *testing.T) {
	for _, pf := range loadPromptFiles(t) {
		pf := pf
		t.Run(pf.Name, func(t *testing.T) {
			if !strings.HasSuffix(pf.Body, "\n") {
				t.Errorf("missing trailing newline")
			}
		})
	}
}
