package steps_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/vars"
)

// projectRoot returns the path two levels up from this test file's directory
// (internal/steps/ → ralph-tui/). Uses runtime.Caller so it is independent
// of the working directory when tests are run.
func projectRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

func TestLoadSteps_IterationCount(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}
	if len(got.Iteration) != 10 {
		t.Errorf("expected 10 iteration steps, got %d", len(got.Iteration))
	}
}

func TestLoadSteps_InitializeCount(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}
	if len(got.Initialize) != 2 {
		t.Errorf("expected 2 initialize steps, got %d", len(got.Initialize))
	}
}

func TestLoadSteps_InitializeOrder(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}

	wantNames := []string{"Splash", "Get GitHub user"}
	for i, want := range wantNames {
		if got.Initialize[i].Name != want {
			t.Errorf("step[%d]: expected name %q, got %q", i, want, got.Initialize[i].Name)
		}
	}
}

func TestLoadSteps_FinalizeCount(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}
	if len(got.Finalize) != 3 {
		t.Errorf("expected 3 finalization steps, got %d", len(got.Finalize))
	}
}

func TestLoadSteps_IterationOrder(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}

	wantNames := []string{
		"Get next issue",
		"Get starting SHA",
		"Feature work",
		"Test planning",
		"Test writing",
		"Code review",
		"Fix review items",
		"Close issue",
		"Update docs",
		"Git push",
	}
	for i, want := range wantNames {
		if got.Iteration[i].Name != want {
			t.Errorf("step[%d]: expected name %q, got %q", i, want, got.Iteration[i].Name)
		}
	}
}

func TestLoadSteps_FinalizeOrder(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}

	wantNames := []string{"Deferred work", "Lessons learned", "Final git push"}
	for i, want := range wantNames {
		if got.Finalize[i].Name != want {
			t.Errorf("step[%d]: expected name %q, got %q", i, want, got.Finalize[i].Name)
		}
	}
}

func TestLoadSteps_IterationClaudeFieldsPopulated(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}

	// "Feature work" is a claude step (index 2; preceded by two non-claude data-gathering steps)
	s := got.Iteration[2]
	if !s.IsClaude {
		t.Error("Feature work: expected IsClaude=true")
	}
	if s.Model == "" {
		t.Error("Feature work: expected non-empty Model")
	}
	if s.PromptFile == "" {
		t.Error("Feature work: expected non-empty PromptFile")
	}
}

func TestLoadSteps_IterationNonClaudeFieldsPopulated(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}

	// "Git push" is a non-claude step (index 9; two new steps prepended)
	s := got.Iteration[9]
	if s.IsClaude {
		t.Error("Git push: expected IsClaude=false")
	}
	if len(s.Command) == 0 {
		t.Error("Git push: expected non-empty Command")
	}
	if s.Model != "" {
		t.Errorf("Git push: expected empty Model, got %q", s.Model)
	}
	if s.PromptFile != "" {
		t.Errorf("Git push: expected empty PromptFile, got %q", s.PromptFile)
	}
}

func TestLoadSteps_FinalizeClaudeFieldsPopulated(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}

	// "Deferred work" is a finalization claude step
	s := got.Finalize[0]
	if !s.IsClaude {
		t.Error("Deferred work: expected IsClaude=true")
	}
	if s.Model == "" {
		t.Error("Deferred work: expected non-empty Model")
	}
	if s.PromptFile == "" {
		t.Error("Deferred work: expected non-empty PromptFile")
	}
}

func TestLoadSteps_FinalizeNonClaudeFieldsPopulated(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}

	// "Final git push" is a non-claude step
	s := got.Finalize[2]
	if s.IsClaude {
		t.Error("Final git push: expected IsClaude=false")
	}
	if len(s.Command) == 0 {
		t.Error("Final git push: expected non-empty Command")
	}
	if s.Model != "" {
		t.Errorf("Final git push: expected empty Model, got %q", s.Model)
	}
	if s.PromptFile != "" {
		t.Errorf("Final git push: expected empty PromptFile, got %q", s.PromptFile)
	}
}

func TestLoadSteps_MissingOptionalFieldsNoError(t *testing.T) {
	dir := t.TempDir()
	json := `{"iteration":[{"name":"Only Name","isClaude":false}],"finalize":[]}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(json), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(got.Iteration) != 1 {
		t.Fatalf("expected 1 iteration step, got %d", len(got.Iteration))
	}
	s := got.Iteration[0]
	if s.Model != "" || s.PromptFile != "" || len(s.Command) != 0 {
		t.Error("optional fields should be zero values when absent from JSON")
	}
}

func TestLoadSteps_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(`not valid json`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := steps.LoadSteps(dir)
	if err == nil {
		t.Fatal("expected an error for malformed JSON, got nil")
	}
	// Error should include the file path
	wantPath := filepath.Join(dir, "ralph-steps.json")
	if !strings.Contains(err.Error(), wantPath) {
		t.Errorf("error %q should contain file path %q", err.Error(), wantPath)
	}
}

func TestLoadSteps_FileNotFound(t *testing.T) {
	_, err := steps.LoadSteps("/nonexistent/path")
	if err == nil {
		t.Fatal("expected an error for missing file, got nil")
	}
}

func TestLoadSteps_InitializeDefaultsEmpty(t *testing.T) {
	dir := t.TempDir()
	json := `{"iteration":[],"finalize":[]}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(json), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(got.Initialize) != 0 {
		t.Errorf("expected Initialize to be empty when absent from JSON, got %d steps", len(got.Initialize))
	}
}

func TestLoadSteps_InitializeDeserializes(t *testing.T) {
	dir := t.TempDir()
	json := `{"initialize":[{"name":"Setup","isClaude":false,"command":["echo","ready"]}],"iteration":[],"finalize":[]}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(json), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(got.Initialize) != 1 {
		t.Fatalf("expected 1 initialize step, got %d", len(got.Initialize))
	}
	if got.Initialize[0].Name != "Setup" {
		t.Errorf("expected initialize step name %q, got %q", "Setup", got.Initialize[0].Name)
	}
}

func TestStep_CaptureAsDefault(t *testing.T) {
	dir := t.TempDir()
	json := `{"iteration":[{"name":"Only Name","isClaude":false}],"finalize":[]}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(json), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got.Iteration[0].CaptureAs != "" {
		t.Errorf("expected CaptureAs to be empty by default, got %q", got.Iteration[0].CaptureAs)
	}
}

func TestStep_CaptureAsDeserializes(t *testing.T) {
	dir := t.TempDir()
	json := `{"iteration":[{"name":"Get Issue","isClaude":false,"captureAs":"ISSUE_ID"}],"finalize":[]}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(json), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got.Iteration[0].CaptureAs != "ISSUE_ID" {
		t.Errorf("expected CaptureAs %q, got %q", "ISSUE_ID", got.Iteration[0].CaptureAs)
	}
}

func TestStep_BreakLoopIfEmptyDefault(t *testing.T) {
	dir := t.TempDir()
	json := `{"iteration":[{"name":"Only Name","isClaude":false}],"finalize":[]}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(json), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got.Iteration[0].BreakLoopIfEmpty {
		t.Error("expected BreakLoopIfEmpty to be false by default")
	}
}

func TestStep_BreakLoopIfEmptyDeserializes(t *testing.T) {
	dir := t.TempDir()
	json := `{"iteration":[{"name":"Get Issue","isClaude":false,"captureAs":"ISSUE_ID","breakLoopIfEmpty":true}],"finalize":[]}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(json), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !got.Iteration[0].BreakLoopIfEmpty {
		t.Error("expected BreakLoopIfEmpty to be true when set in JSON")
	}
}

func TestLoadSteps_EmptyArrays(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(`{"iteration":[],"finalize":[]}`), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error for empty arrays, got: %v", err)
	}
	if len(got.Iteration) != 0 {
		t.Errorf("expected 0 iteration steps, got %d", len(got.Iteration))
	}
	if len(got.Finalize) != 0 {
		t.Errorf("expected 0 finalize steps, got %d", len(got.Finalize))
	}
}

func TestLoadSteps_CommandValues(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}

	// "Git push" command should be ["git", "push"] (index 9; two new steps prepended)
	gitPush := got.Iteration[9]
	if len(gitPush.Command) != 2 || gitPush.Command[0] != "git" || gitPush.Command[1] != "push" {
		t.Errorf("Git push: expected command [git push], got %v", gitPush.Command)
	}

	// "Close issue" command should contain "close_gh_issue" (index 7; two new steps prepended)
	closeIssue := got.Iteration[7]
	found := false
	for _, part := range closeIssue.Command {
		if strings.Contains(part, "close_gh_issue") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Close issue: expected command to contain 'close_gh_issue', got %v", closeIssue.Command)
	}
}

// TP-001: StepFile.Env deserialization — populated array
func TestLoadSteps_EnvPopulatedArray(t *testing.T) {
	dir := t.TempDir()
	json := `{"env":["GITHUB_TOKEN","AWS_KEY"],"iteration":[{"name":"Work","isClaude":false,"command":["echo"]}],"finalize":[]}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(json), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	want := []string{"GITHUB_TOKEN", "AWS_KEY"}
	if len(got.Env) != len(want) {
		t.Fatalf("expected Env length %d, got %d: %v", len(want), len(got.Env), got.Env)
	}
	for i, w := range want {
		if got.Env[i] != w {
			t.Errorf("Env[%d]: expected %q, got %q", i, w, got.Env[i])
		}
	}
}

// TP-002: StepFile.Env deserialization — absent key defaults to nil/empty
func TestLoadSteps_EnvAbsentKeyIsEmpty(t *testing.T) {
	dir := t.TempDir()
	json := `{"iteration":[{"name":"Work","isClaude":false,"command":["echo"]}],"finalize":[]}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(json), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(got.Env) != 0 {
		t.Errorf("expected Env to be nil or empty when absent from JSON, got %v", got.Env)
	}
}

// TP-009: StepFile.Env deserialization — empty array is non-nil with length 0
func TestLoadSteps_EnvEmptyArrayIsNonNil(t *testing.T) {
	dir := t.TempDir()
	json := `{"env":[],"iteration":[{"name":"Work","isClaude":false,"command":["echo"]}],"finalize":[]}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(json), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got.Env == nil {
		t.Error("expected Env to be non-nil for explicit empty array, got nil")
	}
	if len(got.Env) != 0 {
		t.Errorf("expected Env length 0, got %d: %v", len(got.Env), got.Env)
	}
}

// BuildPrompt tests

func makeTempProjectWithPrompt(t *testing.T, filename, content string) string {
	t.Helper()
	dir := t.TempDir()
	promptsDir := filepath.Join(dir, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, filename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBuildPrompt_ReturnsFileContent(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "feature.txt", "do the thing\n")
	step := steps.Step{PromptFile: "feature.txt"}
	vt := vars.New(dir, dir, 0)

	result, err := steps.BuildPrompt(dir, step, vt, vars.Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "do the thing\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestBuildPrompt_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	step := steps.Step{PromptFile: "missing.txt"}
	vt := vars.New(dir, dir, 0)

	_, err := steps.BuildPrompt(dir, step, vt, vars.Iteration)
	if err == nil {
		t.Fatal("expected error for missing prompt file, got nil")
	}
}

func TestBuildPrompt_ErrorIncludesPathAndWrapsOSError(t *testing.T) {
	dir := t.TempDir()
	// No prompts/ subdirectory — file will not exist
	step := steps.Step{PromptFile: "missing.txt"}
	vt := vars.New(dir, dir, 0)

	_, err := steps.BuildPrompt(dir, step, vt, vars.Iteration)
	if err == nil {
		t.Fatal("expected error for missing prompt file, got nil")
	}

	wantPath := filepath.Join(dir, "prompts", "missing.txt")
	if !strings.Contains(err.Error(), wantPath) {
		t.Errorf("error %q should contain file path %q", err.Error(), wantPath)
	}

	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected errors.Is(err, os.ErrNotExist) to be true, got false; err=%v", err)
	}
}

func TestBuildPrompt_EmptyPromptFile(t *testing.T) {
	dir := t.TempDir()
	promptsDir := filepath.Join(dir, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	step := steps.Step{PromptFile: ""}
	vt := vars.New(dir, dir, 0)

	_, err := steps.BuildPrompt(dir, step, vt, vars.Iteration)
	if err == nil {
		t.Fatal("expected error when PromptFile is empty, got nil")
	}
}

func TestBuildPrompt_SubstitutesVarsInContent(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "feature.txt", "implement issue {{ISSUE_ID}}\n")
	step := steps.Step{PromptFile: "feature.txt"}
	vt := vars.New(dir, dir, 0)
	vt.SetPhase(vars.Iteration)
	vt.Bind(vars.Iteration, "ISSUE_ID", "42")

	result, err := steps.BuildPrompt(dir, step, vt, vars.Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "implement issue 42\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestBuildPrompt_UnresolvedVarBecomesEmpty(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "feature.txt", "value: {{UNKNOWN}}\n")
	step := steps.Step{PromptFile: "feature.txt"}
	vt := vars.New(dir, dir, 0)

	result, err := steps.BuildPrompt(dir, step, vt, vars.Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "value: \n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

// ----------------------------------------------------------------------------
// T1: LoadSteps populates StatusLine fields from JSON
// ----------------------------------------------------------------------------

func TestLoadSteps_StatusLineDeserializes(t *testing.T) {
	dir := t.TempDir()
	jsonContent := `{"iteration":[],"finalize":[],"statusLine":{"type":"command","command":"scripts/status.sh","refreshIntervalSeconds":5}}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(jsonContent), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got.StatusLine == nil {
		t.Fatal("expected StatusLine to be non-nil")
	}
	if got.StatusLine.Type != "command" {
		t.Errorf("expected Type %q, got %q", "command", got.StatusLine.Type)
	}
	if got.StatusLine.Command != "scripts/status.sh" {
		t.Errorf("expected Command %q, got %q", "scripts/status.sh", got.StatusLine.Command)
	}
	if got.StatusLine.RefreshIntervalSeconds == nil {
		t.Fatal("expected RefreshIntervalSeconds to be non-nil")
	}
	if *got.StatusLine.RefreshIntervalSeconds != 5 {
		t.Errorf("expected RefreshIntervalSeconds 5, got %d", *got.StatusLine.RefreshIntervalSeconds)
	}
}

// ----------------------------------------------------------------------------
// T2: LoadSteps leaves StatusLine nil when the key is absent
// ----------------------------------------------------------------------------

func TestLoadSteps_StatusLineAbsentIsNil(t *testing.T) {
	dir := t.TempDir()
	jsonContent := `{"iteration":[{"name":"Work","isClaude":false,"command":["echo"]}],"finalize":[]}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(jsonContent), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got.StatusLine != nil {
		t.Errorf("expected StatusLine to be nil when absent from JSON, got %+v", got.StatusLine)
	}
}

// ----------------------------------------------------------------------------
// T3: LoadSteps preserves RefreshIntervalSeconds pointer semantics for zero vs absent
// ----------------------------------------------------------------------------

func TestLoadSteps_StatusLineRefreshIntervalZeroIsNonNilPointer(t *testing.T) {
	dir := t.TempDir()
	jsonContent := `{"iteration":[],"finalize":[],"statusLine":{"command":"echo","refreshIntervalSeconds":0}}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(jsonContent), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got.StatusLine == nil {
		t.Fatal("expected StatusLine to be non-nil")
	}
	if got.StatusLine.RefreshIntervalSeconds == nil {
		t.Fatal("expected RefreshIntervalSeconds to be non-nil pointer when set to 0")
	}
	if *got.StatusLine.RefreshIntervalSeconds != 0 {
		t.Errorf("expected RefreshIntervalSeconds 0, got %d", *got.StatusLine.RefreshIntervalSeconds)
	}
}

func TestLoadSteps_StatusLineRefreshIntervalAbsentIsNilPointer(t *testing.T) {
	dir := t.TempDir()
	jsonContent := `{"iteration":[],"finalize":[],"statusLine":{"command":"echo"}}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(jsonContent), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got.StatusLine == nil {
		t.Fatal("expected StatusLine to be non-nil")
	}
	if got.StatusLine.RefreshIntervalSeconds != nil {
		t.Errorf("expected RefreshIntervalSeconds to be nil when absent, got %d", *got.StatusLine.RefreshIntervalSeconds)
	}
}

// --- TP-009: ContainerEnv JSON deserialization ---

// TestLoadSteps_ContainerEnvPopulated verifies that a populated containerEnv map
// is correctly deserialized from ralph-steps.json.
func TestLoadSteps_ContainerEnvPopulated(t *testing.T) {
	dir := t.TempDir()
	content := `{"containerEnv":{"K":"V"},"iteration":[{"name":"Work","isClaude":false,"command":["echo"]}],"finalize":[]}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.ContainerEnv) != 1 {
		t.Fatalf("expected ContainerEnv length 1, got %d: %v", len(got.ContainerEnv), got.ContainerEnv)
	}
	if got.ContainerEnv["K"] != "V" {
		t.Errorf(`ContainerEnv["K"] = %q, want "V"`, got.ContainerEnv["K"])
	}
}

// TestLoadSteps_ContainerEnvAbsentIsNil verifies that an absent containerEnv key
// deserializes to a nil/empty map.
func TestLoadSteps_ContainerEnvAbsentIsNil(t *testing.T) {
	dir := t.TempDir()
	content := `{"iteration":[{"name":"Work","isClaude":false,"command":["echo"]}],"finalize":[]}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.ContainerEnv) != 0 {
		t.Errorf("expected ContainerEnv nil/empty when absent from JSON, got %v", got.ContainerEnv)
	}
}

// TestLoadSteps_ContainerEnvEmptyMap verifies that an explicit empty containerEnv
// object deserializes to an empty map without error.
func TestLoadSteps_ContainerEnvEmptyMap(t *testing.T) {
	dir := t.TempDir()
	content := `{"containerEnv":{},"iteration":[{"name":"Work","isClaude":false,"command":["echo"]}],"finalize":[]}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.ContainerEnv) != 0 {
		t.Errorf("expected ContainerEnv empty for explicit {}, got %v", got.ContainerEnv)
	}
}
