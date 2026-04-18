package validator_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/sandbox"
	"github.com/mxriverlynn/pr9k/src/internal/steps"
	"github.com/mxriverlynn/pr9k/src/internal/validator"
)

func getRalphTUIDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "workflow")
}

// assembleWorkflowDir builds a temp directory that mirrors the workflow bundle
// layout (config.json + prompts/ + scripts/) from source-tree locations:
//   - config.json  lives at workflow/config.json
//   - prompts/          lives at workflow/prompts/
//   - scripts/          lives at workflow/scripts/
func assembleWorkflowDir(t *testing.T) string {
	t.Helper()
	ralphTUIDir := getRalphTUIDir(t)

	dir := t.TempDir()

	data, err := os.ReadFile(filepath.Join(ralphTUIDir, "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		t.Fatalf("write config.json: %v", err)
	}

	// TP-010: preflight — verify bundle components exist before symlinking so failures
	// are immediately actionable rather than producing an opaque "no such file" from Symlink.
	for _, rel := range []string{"config.json", "prompts", "scripts"} {
		if _, err := os.Stat(filepath.Join(ralphTUIDir, rel)); err != nil {
			t.Fatalf("workflow bundle incomplete: %s missing — run from repo root (%v)", rel, err)
		}
	}

	for _, sub := range []string{"prompts", "scripts"} {
		abs, err := filepath.Abs(filepath.Join(ralphTUIDir, sub))
		if err != nil {
			t.Fatalf("abs path for %s: %v", sub, err)
		}
		if err := os.Symlink(abs, filepath.Join(dir, sub)); err != nil {
			t.Fatalf("symlink %s: %v", sub, err)
		}
	}

	return dir
}

// TP-001: production config.json passes validation with zero fatal errors,
// and the containerEnv block contains no collision notices (TP-003).
func TestValidate_ProductionStepsJSON(t *testing.T) {
	workflowDir := assembleWorkflowDir(t)
	errs := validator.Validate(workflowDir)
	if n := validator.FatalErrorCount(errs); n != 0 {
		t.Fatalf("production config.json has %d fatal validation error(s): %v", n, errs)
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

// TestLoadSteps_ReviewVerdictAdjacency pins that "Check review verdict" appears
// immediately between "Code review" and "Fix review items". A future insertion
// between these three steps would silently invalidate the capture dependency.
func TestLoadSteps_ReviewVerdictAdjacency(t *testing.T) {
	sf, err := steps.LoadSteps(getRalphTUIDir(t))
	if err != nil {
		t.Fatalf("LoadSteps: %v", err)
	}

	crIdx, crvIdx, friIdx := -1, -1, -1
	for i, step := range sf.Iteration {
		switch step.Name {
		case "Code review":
			crIdx = i
		case "Check review verdict":
			crvIdx = i
		case "Fix review items":
			friIdx = i
		}
	}
	if crIdx < 0 {
		t.Fatal(`no iteration step named "Code review"`)
	}
	if crvIdx < 0 {
		t.Fatal(`no iteration step named "Check review verdict"`)
	}
	if friIdx < 0 {
		t.Fatal(`no iteration step named "Fix review items"`)
	}

	if crIdx >= crvIdx || crvIdx >= friIdx {
		t.Errorf("want Code review (%d) < Check review verdict (%d) < Fix review items (%d)", crIdx, crvIdx, friIdx)
	}
	if crvIdx != crIdx+1 || friIdx != crvIdx+1 {
		t.Errorf("steps must be consecutive: Code review (%d), Check review verdict (%d), Fix review items (%d)", crIdx, crvIdx, friIdx)
	}
}

// TestCodeReviewPrompt_ContainsSentinel verifies the code-review-changes.md
// prompt instructs the model to emit the NOTHING-TO-FIX sentinel when no
// issues are found — a misspelling or removal would silently disable the skip.
func TestCodeReviewPrompt_ContainsSentinel(t *testing.T) {
	ralphTUIDir := getRalphTUIDir(t)
	data, err := os.ReadFile(filepath.Join(ralphTUIDir, "prompts", "code-review-changes.md"))
	if err != nil {
		t.Fatalf("read code-review-changes.md: %v", err)
	}
	if !strings.Contains(string(data), "NOTHING-TO-FIX") {
		t.Error("code-review-changes.md does not contain the NOTHING-TO-FIX sentinel")
	}
}

// TestLoadSteps_TestWritingStep_TimeoutSeconds pins that the "Test writing" step
// in the shipped config.json has timeoutSeconds: 900. This guards against
// accidental removal of the conservative cap that prevents runaway test-writing
// runs from blocking the iteration loop indefinitely.
func TestLoadSteps_TestWritingStep_TimeoutSeconds(t *testing.T) {
	ralphTUIDir := getRalphTUIDir(t)
	sf, err := steps.LoadSteps(ralphTUIDir)
	if err != nil {
		t.Fatalf("LoadSteps: %v", err)
	}
	var found bool
	for _, s := range sf.Iteration {
		if s.Name == "Test writing" {
			found = true
			if s.TimeoutSeconds != 900 {
				t.Errorf("Test writing TimeoutSeconds: want 900, got %d", s.TimeoutSeconds)
			}
		}
	}
	if !found {
		t.Error("no iteration step named \"Test writing\" found")
	}
}

// TP-018: Exclusivity pin — only the "Test writing" step has a positive
// TimeoutSeconds across all three phases. Any deliberate addition of a timeout
// to another step must update this test to signal the intentional policy change.
func TestLoadSteps_OnlyTestWritingHasTimeout(t *testing.T) {
	ralphTUIDir := getRalphTUIDir(t)
	sf, err := steps.LoadSteps(ralphTUIDir)
	if err != nil {
		t.Fatalf("LoadSteps: %v", err)
	}

	var timedOut []string
	for _, s := range sf.Initialize {
		if s.TimeoutSeconds > 0 {
			timedOut = append(timedOut, s.Name)
		}
	}
	for _, s := range sf.Iteration {
		if s.TimeoutSeconds > 0 {
			timedOut = append(timedOut, s.Name)
		}
	}
	for _, s := range sf.Finalize {
		if s.TimeoutSeconds > 0 {
			timedOut = append(timedOut, s.Name)
		}
	}

	want := []string{"Test writing"}
	if len(timedOut) != len(want) {
		t.Errorf("steps with TimeoutSeconds > 0: got %v, want %v", timedOut, want)
		return
	}
	for i := range want {
		if timedOut[i] != want[i] {
			t.Errorf("steps with TimeoutSeconds > 0: got %v, want %v", timedOut, want)
			return
		}
	}
}
