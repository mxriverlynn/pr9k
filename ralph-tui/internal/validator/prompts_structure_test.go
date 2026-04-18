package validator_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// todoWriteHint is the exact hint line added to every prompt by issue #125.
// TP-004 and TP-005: TP-001's strings.Count against this constant enforces
// byte-identity across all files. If TodoWrite is ever renamed in the tool
// registry, TP-001 is the test that fails — that's the intended behavior.
// This selector must match the Anthropic tool name exactly — see issue #125.
const todoWriteHint = `You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".`

func promptsDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// test file: ralph-tui/internal/validator/prompts_structure_test.go
	// repo root: three levels up
	repoRoot := filepath.Join(filepath.Dir(filename), "..", "..", "..")
	abs, err := filepath.Abs(filepath.Join(repoRoot, "prompts"))
	if err != nil {
		t.Fatalf("abs path for prompts: %v", err)
	}
	return abs
}

func loadPromptFiles(t *testing.T) map[string]string {
	t.Helper()
	dir := promptsDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read prompts dir: %v", err)
	}
	files := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		files[e.Name()] = string(data)
	}
	if len(files) == 0 {
		t.Fatal("no prompt files found")
	}
	return files
}

// TP-001 / TP-006: hint appears exactly once in every prompt file.
func TestPrompts_HintPresentExactlyOnce(t *testing.T) {
	for name, body := range loadPromptFiles(t) {
		n := strings.Count(body, todoWriteHint)
		if n != 1 {
			t.Errorf("%s: TodoWrite hint count = %d, want 1 (0 = missing, 2+ = duplicated)", name, n)
		}
	}
}

// TP-002: hint sits immediately after the first line (@… include).
func TestPrompts_HintPositionAfterInclude(t *testing.T) {
	for name, body := range loadPromptFiles(t) {
		lines := strings.Split(body, "\n")
		if len(lines) < 2 {
			t.Errorf("%s: too few lines (%d), expected at least 2", name, len(lines))
			continue
		}
		if !strings.HasPrefix(lines[0], "@") {
			t.Errorf("%s: line 0 = %q, want @… include", name, lines[0])
		}
		if lines[1] != todoWriteHint {
			t.Errorf("%s: line 1 = %q, want TodoWrite hint", name, lines[1])
		}
	}
}

var numberedStepRe = regexp.MustCompile(`^(\d+)\. `)

// TP-003: numbered steps start at 1 and are consecutive (no gaps, no renumbering).
func TestPrompts_NumberedStepsConsecutive(t *testing.T) {
	for name, body := range loadPromptFiles(t) {
		var nums []int
		for _, line := range strings.Split(body, "\n") {
			if m := numberedStepRe.FindStringSubmatch(line); m != nil {
				n := 0
				for _, ch := range m[1] {
					n = n*10 + int(ch-'0')
				}
				nums = append(nums, n)
			}
		}
		if len(nums) == 0 {
			t.Errorf("%s: no numbered steps found", name)
			continue
		}
		if nums[0] != 1 {
			t.Errorf("%s: first step = %d, want 1", name, nums[0])
		}
		for i := 1; i < len(nums); i++ {
			if nums[i] != nums[i-1]+1 {
				t.Errorf("%s: step sequence broken at index %d: %d → %d", name, i, nums[i-1], nums[i])
			}
		}
	}
}

// TP-007: every prompt file ends with a newline.
func TestPrompts_TrailingNewline(t *testing.T) {
	for name, body := range loadPromptFiles(t) {
		if !strings.HasSuffix(body, "\n") {
			t.Errorf("%s: missing trailing newline", name)
		}
	}
}
