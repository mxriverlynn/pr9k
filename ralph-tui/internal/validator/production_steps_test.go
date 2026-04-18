package validator_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/sandbox"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/validator"
)

func getRalphTUIDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

// assembleWorkflowDir builds a temp directory that mirrors the workflow bundle
// layout (ralph-steps.json + prompts/ + scripts/) from source-tree locations:
//   - ralph-steps.json  lives at ralph-tui/ralph-steps.json
//   - prompts/          lives at the repo root
//   - scripts/          lives at the repo root
func assembleWorkflowDir(t *testing.T) string {
	t.Helper()
	ralphTUIDir := getRalphTUIDir(t)
	repoRoot := filepath.Join(ralphTUIDir, "..")

	dir := t.TempDir()

	data, err := os.ReadFile(filepath.Join(ralphTUIDir, "ralph-steps.json"))
	if err != nil {
		t.Fatalf("read ralph-steps.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), data, 0o644); err != nil {
		t.Fatalf("write ralph-steps.json: %v", err)
	}

	for _, sub := range []string{"prompts", "scripts"} {
		abs, err := filepath.Abs(filepath.Join(repoRoot, sub))
		if err != nil {
			t.Fatalf("abs path for %s: %v", sub, err)
		}
		if err := os.Symlink(abs, filepath.Join(dir, sub)); err != nil {
			t.Fatalf("symlink %s: %v", sub, err)
		}
	}

	return dir
}

// TP-001: production ralph-steps.json passes validation with zero fatal errors,
// and the containerEnv block contains no collision notices (TP-003).
func TestValidate_ProductionStepsJSON(t *testing.T) {
	workflowDir := assembleWorkflowDir(t)
	errs := validator.Validate(workflowDir)
	if n := validator.FatalErrorCount(errs); n != 0 {
		t.Fatalf("production ralph-steps.json has %d fatal validation error(s): %v", n, errs)
	}
	// TP-003: none of the returned entries should be a containerEnv collision
	// notice (Severity==info, Category=="containerEnv"). Such a notice means one
	// of the Go cache keys also appears in the env allowlist — Docker's last-write
	// rule would silently discard the host value in that case.
	for _, e := range errs {
		if e.Severity == validator.SeverityInfo && e.Category == "containerEnv" {
			t.Errorf("containerEnv collision notice: %v", e)
		}
	}
}

// TP-001: all four Go cache keys are present in the production containerEnv block.
func TestLoadSteps_ProductionStepsJSON_ContainerEnvKeys(t *testing.T) {
	sf, err := steps.LoadSteps(getRalphTUIDir(t))
	if err != nil {
		t.Fatalf("LoadSteps: %v", err)
	}

	required := []string{"GOPATH", "GOCACHE", "GOMODCACHE", "XDG_CACHE_HOME"}
	for _, key := range required {
		if _, ok := sf.ContainerEnv[key]; !ok {
			t.Errorf("containerEnv missing key %q", key)
		}
	}
}

// TP-002: every production containerEnv value is clean and resolves under the
// container bind-mount target (/home/agent/workspace/).
func TestProductionStepsJSON_ContainerEnvValuesUnderBindMount(t *testing.T) {
	sf, err := steps.LoadSteps(getRalphTUIDir(t))
	if err != nil {
		t.Fatalf("LoadSteps: %v", err)
	}

	t.Run("clean_stable", func(t *testing.T) {
		for key, val := range sf.ContainerEnv {
			if filepath.Clean(val) != val {
				t.Errorf("containerEnv[%q] = %q: filepath.Clean changes it to %q (dot-segments or trailing slash)", key, val, filepath.Clean(val))
			}
		}
	})

	prefix := sandbox.ContainerRepoPath + "/"
	t.Run("under_bind_mount", func(t *testing.T) {
		for key, val := range sf.ContainerEnv {
			if !strings.HasPrefix(val, prefix) {
				t.Errorf("containerEnv[%q] = %q: want prefix %q (must resolve under the container bind-mount)", key, val, prefix)
			}
		}
	})
}

// TP-003: ordering invariant — "Get starting SHA" before "Feature work", "Get post-feature diff" after.
// Also asserts STARTING_SHA is bound by exactly one step and that PRE_REVIEW_DIFF's command
// references {{STARTING_SHA}}..HEAD, pinning the dependency direction.
func TestProductionStepsJSON_StartingShaDiffInvariant(t *testing.T) {
	sf, err := steps.LoadSteps(getRalphTUIDir(t))
	if err != nil {
		t.Fatalf("LoadSteps: %v", err)
	}

	startSHAIdx, featureWorkIdx, postDiffIdx := -1, -1, -1
	for i, step := range sf.Iteration {
		switch step.Name {
		case "Get starting SHA":
			startSHAIdx = i
		case "Feature work":
			featureWorkIdx = i
		case "Get post-feature diff":
			postDiffIdx = i
		}
	}
	if startSHAIdx < 0 {
		t.Fatal(`no iteration step named "Get starting SHA"`)
	}
	if featureWorkIdx < 0 {
		t.Fatal(`no iteration step named "Feature work"`)
	}
	if postDiffIdx < 0 {
		t.Fatal(`no iteration step named "Get post-feature diff"`)
	}

	if startSHAIdx >= featureWorkIdx {
		t.Errorf("Get starting SHA (idx %d) must appear before Feature work (idx %d)", startSHAIdx, featureWorkIdx)
	}
	if postDiffIdx <= featureWorkIdx {
		t.Errorf("Get post-feature diff (idx %d) must appear after Feature work (idx %d)", postDiffIdx, featureWorkIdx)
	}

	var captureCount int
	for _, step := range sf.Iteration {
		if step.CaptureAs == "STARTING_SHA" {
			captureCount++
		}
	}
	if captureCount != 1 {
		t.Errorf("STARTING_SHA is bound by %d steps, want exactly 1", captureCount)
	}

	postDiffStep := sf.Iteration[postDiffIdx]
	found := false
	for _, arg := range postDiffStep.Command {
		if strings.Contains(arg, "{{STARTING_SHA}}..HEAD") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Get post-feature diff command %v does not contain {{STARTING_SHA}}..HEAD", postDiffStep.Command)
	}
}

// TP-007: shape assertions for the three new capture steps added in issue #127.
func TestLoadSteps_IterationCaptureSteps_Shape(t *testing.T) {
	sf, err := steps.LoadSteps(getRalphTUIDir(t))
	if err != nil {
		t.Fatalf("LoadSteps: %v", err)
	}

	cases := []struct {
		name          string
		wantCmd0      string
		wantCapture   string
		wantMode      string
		wantNonClaude bool
	}{
		{"Get issue body", "gh", "ISSUE_BODY", "fullStdout", true},
		{"Get project card", "scripts/project_card", "PROJECT_CARD", "fullStdout", true},
		{"Get post-feature diff", "git", "PRE_REVIEW_DIFF", "fullStdout", true},
		{"Check review verdict", "scripts/review_verdict", "REVIEW_HAS_FIXES", "lastLine", true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var found *steps.Step
			for i := range sf.Iteration {
				if sf.Iteration[i].Name == tc.name {
					found = &sf.Iteration[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("no iteration step named %q", tc.name)
			}
			if tc.wantNonClaude && found.IsClaude {
				t.Errorf("IsClaude = true, want false")
			}
			if len(found.Command) == 0 || found.Command[0] != tc.wantCmd0 {
				t.Errorf("Command[0] = %q, want %q", found.Command[0], tc.wantCmd0)
			}
			if found.CaptureAs != tc.wantCapture {
				t.Errorf("CaptureAs = %q, want %q", found.CaptureAs, tc.wantCapture)
			}
			if found.CaptureMode != tc.wantMode {
				t.Errorf("CaptureMode = %q, want %q", found.CaptureMode, tc.wantMode)
			}
		})
	}

	// "Get post-feature diff" must also include --stat in its command.
	for _, step := range sf.Iteration {
		if step.Name != "Get post-feature diff" {
			continue
		}
		found := false
		for _, arg := range step.Command {
			if arg == "--stat" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Get post-feature diff command %v does not include --stat", step.Command)
		}
		break
	}
}

// TP-001 (cont.): iteration phase contains "Summarize to issue" wired to the correct script.
func TestLoadSteps_IterationContainsSummarizeToIssue(t *testing.T) {
	sf, err := steps.LoadSteps(getRalphTUIDir(t))
	if err != nil {
		t.Fatalf("LoadSteps: %v", err)
	}
	for _, step := range sf.Iteration {
		if step.Name == "Summarize to issue" {
			if len(step.Command) == 0 || step.Command[0] != "scripts/post_issue_summary" {
				t.Fatalf("step %q: Command[0] = %q, want %q", step.Name, step.Command[0], "scripts/post_issue_summary")
			}
			return
		}
	}
	t.Fatal(`iteration phase has no step named "Summarize to issue"`)
}

// TestLoadSteps_FixReviewItems_SkipIfCaptureEmpty pins that "Fix review items"
// has skipIfCaptureEmpty: "REVIEW_HAS_FIXES", wiring it to "Check review verdict".
func TestLoadSteps_FixReviewItems_SkipIfCaptureEmpty(t *testing.T) {
	sf, err := steps.LoadSteps(getRalphTUIDir(t))
	if err != nil {
		t.Fatalf("LoadSteps: %v", err)
	}
	for _, step := range sf.Iteration {
		if step.Name == "Fix review items" {
			if step.SkipIfCaptureEmpty != "REVIEW_HAS_FIXES" {
				t.Errorf("SkipIfCaptureEmpty = %q, want %q", step.SkipIfCaptureEmpty, "REVIEW_HAS_FIXES")
			}
			return
		}
	}
	t.Fatal(`iteration phase has no step named "Fix review items"`)
}
