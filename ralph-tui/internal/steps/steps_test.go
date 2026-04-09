package steps_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
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

func TestLoadSteps_Count(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}
	if len(got) != 8 {
		t.Errorf("expected 8 steps, got %d", len(got))
	}
}

func TestLoadFinalizeSteps_Count(t *testing.T) {
	got, err := steps.LoadFinalizeSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadFinalizeSteps returned error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 finalization steps, got %d", len(got))
	}
}

func TestLoadSteps_Order(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}

	wantNames := []string{
		"Feature work",
		"Test planning",
		"Test writing",
		"Code review",
		"Review fixes",
		"Close issue",
		"Update docs",
		"Git push",
	}
	for i, want := range wantNames {
		if got[i].Name != want {
			t.Errorf("step[%d]: expected name %q, got %q", i, want, got[i].Name)
		}
	}
}

func TestLoadFinalizeSteps_Order(t *testing.T) {
	got, err := steps.LoadFinalizeSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadFinalizeSteps returned error: %v", err)
	}

	wantNames := []string{"Deferred work", "Lessons learned", "Final git push"}
	for i, want := range wantNames {
		if got[i].Name != want {
			t.Errorf("step[%d]: expected name %q, got %q", i, want, got[i].Name)
		}
	}
}

func TestLoadSteps_ClaudeFieldsPopulated(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}

	// "Feature work" is a claude step
	s := got[0]
	if !s.IsClaudeStep() {
		t.Error("Feature work: expected IsClaudeStep()=true")
	}
	if s.Model == "" {
		t.Error("Feature work: expected non-empty Model")
	}
	if s.PromptFile == "" {
		t.Error("Feature work: expected non-empty PromptFile")
	}
}

func TestLoadSteps_NonClaudeFieldsPopulated(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}

	// "Git push" is a non-claude step
	s := got[7]
	if s.IsClaudeStep() {
		t.Error("Git push: expected IsClaudeStep()=false")
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

func TestLoadSteps_MissingOptionalFieldsNoError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "configs"), 0755); err != nil {
		t.Fatal(err)
	}
	json := `[{"name":"Only Name","command":["echo"]}]`
	if err := os.WriteFile(filepath.Join(dir, "configs", "ralph-steps.json"), []byte(json), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 step, got %d", len(got))
	}
	s := got[0]
	if s.Model != "" || s.PromptFile != "" {
		t.Error("optional fields should be zero values when absent from JSON")
	}
}

func TestLoadSteps_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "configs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "configs", "ralph-steps.json"), []byte(`not valid json`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := steps.LoadSteps(dir)
	if err == nil {
		t.Fatal("expected an error for malformed JSON, got nil")
	}
	// Error should include the file path
	wantPath := filepath.Join(dir, "configs", "ralph-steps.json")
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

func TestLoadFinalizeSteps_FileNotFound(t *testing.T) {
	_, err := steps.LoadFinalizeSteps("/nonexistent/path")
	if err == nil {
		t.Fatal("expected an error for missing file, got nil")
	}
}

// T2 — LoadFinalizeSteps malformed JSON
func TestLoadFinalizeSteps_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "configs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "configs", "ralph-finalize-steps.json"), []byte(`not valid json`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := steps.LoadFinalizeSteps(dir)
	if err == nil {
		t.Fatal("expected an error for malformed JSON, got nil")
	}
	wantPath := filepath.Join(dir, "configs", "ralph-finalize-steps.json")
	if !strings.Contains(err.Error(), wantPath) {
		t.Errorf("error %q should contain file path %q", err.Error(), wantPath)
	}
}

// T3 — Finalization step field validation
func TestLoadFinalizeSteps_ClaudeFieldsPopulated(t *testing.T) {
	got, err := steps.LoadFinalizeSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadFinalizeSteps returned error: %v", err)
	}

	// "Deferred work" is a finalization claude step
	s := got[0]
	if !s.IsClaudeStep() {
		t.Error("Deferred work: expected IsClaudeStep()=true")
	}
	if s.Model == "" {
		t.Error("Deferred work: expected non-empty Model")
	}
	if s.PromptFile == "" {
		t.Error("Deferred work: expected non-empty PromptFile")
	}
}

func TestLoadFinalizeSteps_NonClaudeFieldsPopulated(t *testing.T) {
	got, err := steps.LoadFinalizeSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadFinalizeSteps returned error: %v", err)
	}

	// "Final git push" is a non-claude step
	s := got[2]
	if s.IsClaudeStep() {
		t.Error("Final git push: expected IsClaudeStep()=false")
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

// T4 — Empty steps array
func TestLoadSteps_EmptyArray(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "configs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "configs", "ralph-steps.json"), []byte(`[]`), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := steps.LoadSteps(dir)
	if err != nil {
		t.Fatalf("expected no error for empty array, got: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 steps, got %d", len(got))
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

	result, err := steps.BuildPrompt(dir, step)
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

	_, err := steps.BuildPrompt(dir, step)
	if err == nil {
		t.Fatal("expected error for missing prompt file, got nil")
	}
}

func TestBuildPrompt_RealNewlines(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "nl.txt", "line one\nline two\n")
	step := steps.Step{PromptFile: "nl.txt"}

	result, err := steps.BuildPrompt(dir, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "\n") {
		t.Error("expected real newlines in result, found none")
	}
	// Ensure no literal backslash-n sequences
	if strings.Contains(result, `\n`) {
		t.Error("result contains literal \\n instead of real newlines")
	}
}

func TestBuildPrompt_EmptyFile(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "empty.txt", "")
	step := steps.Step{PromptFile: "empty.txt"}

	result, err := steps.BuildPrompt(dir, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "" {
		t.Errorf("got %q, want empty string", result)
	}
}

func TestBuildPrompt_NoTrailingNewline(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "notail.txt", "no trailing newline")
	step := steps.Step{PromptFile: "notail.txt"}

	result, err := steps.BuildPrompt(dir, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "no trailing newline"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

// T1 — BuildPrompt error message includes file path and wraps OS error
func TestBuildPrompt_ErrorIncludesPathAndWrapsOSError(t *testing.T) {
	dir := t.TempDir()
	// No prompts/ subdirectory — file will not exist
	step := steps.Step{PromptFile: "missing.txt"}

	_, err := steps.BuildPrompt(dir, step)
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

// T2 — BuildPrompt preserves multiline prompt file content
func TestBuildPrompt_MultilineContent(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "multi.txt", "line one\nline two\nline three\n")
	step := steps.Step{PromptFile: "multi.txt"}

	result, err := steps.BuildPrompt(dir, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "line one\nline two\nline three\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

// T3 — BuildPrompt with empty PromptFile field returns an error
func TestBuildPrompt_EmptyPromptFile(t *testing.T) {
	dir := t.TempDir()
	promptsDir := filepath.Join(dir, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	step := steps.Step{PromptFile: ""}

	_, err := steps.BuildPrompt(dir, step)
	if err == nil {
		t.Fatal("expected error when PromptFile is empty, got nil")
	}
}

// T5 — Command field values
func TestLoadSteps_CommandValues(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}

	// "Git push" command should be ["git", "push"]
	gitPush := got[7]
	if len(gitPush.Command) != 2 || gitPush.Command[0] != "git" || gitPush.Command[1] != "push" {
		t.Errorf("Git push: expected command [git push], got %v", gitPush.Command)
	}

	// "Close issue" command should contain "close_gh_issue"
	closeIssue := got[5]
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

// --- LoadWorkflowConfig tests ---

func writeWorkflowConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadWorkflowConfig_ValidThreePhase(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{
		"pre-loop":  [{"name":"Pre step",  "command":["echo","pre"]}],
		"loop":      [{"name":"Loop step", "promptFile":"work.md","model":"opus"}],
		"post-loop": [{"name":"Post step", "command":["echo","post"]}]
	}`)

	cfg, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.PreLoop) != 1 || cfg.PreLoop[0].Name != "Pre step" {
		t.Errorf("unexpected PreLoop: %+v", cfg.PreLoop)
	}
	if len(cfg.Loop) != 1 || cfg.Loop[0].Name != "Loop step" || cfg.Loop[0].Model != "opus" {
		t.Errorf("unexpected Loop: %+v", cfg.Loop)
	}
	if len(cfg.PostLoop) != 1 || cfg.PostLoop[0].Name != "Post step" {
		t.Errorf("unexpected PostLoop: %+v", cfg.PostLoop)
	}
}

func TestLoadWorkflowConfig_ClaudeStepDefaults(t *testing.T) {
	s := steps.Step{Name: "x", PromptFile: "work.md"}
	if s.DefaultModel() != "sonnet" {
		t.Errorf("DefaultModel: got %q, want %q", s.DefaultModel(), "sonnet")
	}
	if s.DefaultPermissionMode() != "acceptEdits" {
		t.Errorf("DefaultPermissionMode: got %q, want %q", s.DefaultPermissionMode(), "acceptEdits")
	}
}

func TestLoadWorkflowConfig_IsClaudeIsCommand(t *testing.T) {
	claude := steps.Step{Name: "c", PromptFile: "work.md"}
	cmd := steps.Step{Name: "d", Command: []string{"echo"}}
	if !claude.IsClaudeStep() {
		t.Error("expected IsClaudeStep()=true for step with PromptFile")
	}
	if claude.IsCommandStep() {
		t.Error("expected IsCommandStep()=false for step with PromptFile only")
	}
	if cmd.IsClaudeStep() {
		t.Error("expected IsClaudeStep()=false for command step")
	}
	if !cmd.IsCommandStep() {
		t.Error("expected IsCommandStep()=true for command step")
	}
}

func TestLoadWorkflowConfig_ErrorBothPromptFileAndCommand(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[], "loop":[{"name":"bad","promptFile":"x.md","command":["echo"]}], "post-loop":[]
	}`)
	_, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `step "bad": has both promptFile and command`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadWorkflowConfig_ErrorNeitherPromptFileNorCommand(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[], "loop":[{"name":"empty"}], "post-loop":[]
	}`)
	_, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `step "empty": must have promptFile or command`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadWorkflowConfig_ErrorModelWithoutPromptFile(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[], "loop":[{"name":"m","command":["echo"],"model":"opus"}], "post-loop":[]
	}`)
	_, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `step "m": model requires promptFile`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadWorkflowConfig_ErrorInjectVarsWithoutPromptFile(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[], "loop":[{"name":"iv","command":["echo"],"injectVariables":["FOO"]}], "post-loop":[]
	}`)
	_, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `step "iv": injectVariables requires promptFile`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadWorkflowConfig_ErrorPermissionModeWithoutPromptFile(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[], "loop":[{"name":"pm","command":["echo"],"permissionMode":"bypassPermissions"}], "post-loop":[]
	}`)
	_, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `step "pm": permissionMode requires promptFile`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadWorkflowConfig_ErrorExitLoopIfEmptyWithoutOutputVariable(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[], "loop":[{"name":"el","command":["echo"],"exitLoopIfEmpty":true}], "post-loop":[]
	}`)
	_, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `step "el": exitLoopIfEmpty requires outputVariable`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadWorkflowConfig_ErrorExitLoopIfEmptyOutsideLoop(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[{"name":"pre-el","command":["echo"],"exitLoopIfEmpty":true,"outputVariable":"X"}],
		"loop":[], "post-loop":[]
	}`)
	_, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `step "pre-el": exitLoopIfEmpty only valid in loop phase`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadWorkflowConfig_ErrorEmptyCommandArray(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[], "loop":[{"name":"ec","command":[]}], "post-loop":[]
	}`)
	_, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `step "ec": command array must not be empty`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadWorkflowConfig_ErrorOutputVariableOnClaudeStep(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[], "loop":[{"name":"ov","promptFile":"work.md","outputVariable":"X"}], "post-loop":[]
	}`)
	_, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `step "ov": outputVariable requires command, not promptFile`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadWorkflowConfig_MultipleErrorsCollected(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[
			{"name":"both","promptFile":"x.md","command":["echo"]},
			{"name":"neither"}
		],
		"loop":[], "post-loop":[]
	}`)
	_, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `"both"`) {
		t.Errorf("expected error about 'both' step, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"neither"`) {
		t.Errorf("expected error about 'neither' step, got: %v", err)
	}
}

func TestLoadWorkflowConfig_MissingFile(t *testing.T) {
	_, err := steps.LoadWorkflowConfig("/nonexistent", "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadWorkflowConfig_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(`not json`), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestLoadWorkflowConfig_EmptyPhasesValid(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{"pre-loop":[],"loop":[],"post-loop":[]}`)
	cfg, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err != nil {
		t.Fatalf("expected no error for empty phases, got: %v", err)
	}
	if len(cfg.PreLoop) != 0 || len(cfg.Loop) != 0 || len(cfg.PostLoop) != 0 {
		t.Error("expected all phases to be empty")
	}
}

// Gap 1: DefaultModel/DefaultPermissionMode with explicit values
func TestLoadWorkflowConfig_ClaudeStepDefaultsExplicitValues(t *testing.T) {
	s := steps.Step{Name: "x", PromptFile: "work.md", Model: "opus", PermissionMode: "bypassPermissions"}
	if s.DefaultModel() != "opus" {
		t.Errorf("DefaultModel: got %q, want %q", s.DefaultModel(), "opus")
	}
	if s.DefaultPermissionMode() != "bypassPermissions" {
		t.Errorf("DefaultPermissionMode: got %q, want %q", s.DefaultPermissionMode(), "bypassPermissions")
	}
}

// Gap 2: Cross-phase error collection
func TestLoadWorkflowConfig_MultipleErrorsAcrossPhases(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[{"name":"bad-pre"}],
		"loop":[{"name":"bad-loop"}],
		"post-loop":[]
	}`)
	_, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `"bad-pre"`) {
		t.Errorf("expected error about 'bad-pre' step, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"bad-loop"`) {
		t.Errorf("expected error about 'bad-loop' step, got: %v", err)
	}
}

// Gap 3: exitLoopIfEmpty valid in loop phase
func TestLoadWorkflowConfig_ExitLoopIfEmptyValidInLoopPhase(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[],
		"loop":[{"name":"capture","command":["echo"],"exitLoopIfEmpty":true,"outputVariable":"ISSUE"}],
		"post-loop":[]
	}`)
	_, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err != nil {
		t.Errorf("expected no error for valid exitLoopIfEmpty in loop phase, got: %v", err)
	}
}

// Gap 5: Error messages include file path
func TestLoadWorkflowConfig_MissingFileErrorIncludesPath(t *testing.T) {
	_, err := steps.LoadWorkflowConfig("/nonexistent", "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	wantPath := filepath.Join("/nonexistent", "ralph-steps.json")
	if !strings.Contains(err.Error(), wantPath) {
		t.Errorf("error %q should contain file path %q", err.Error(), wantPath)
	}
}

func TestLoadWorkflowConfig_MalformedJSONErrorIncludesPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(`not json`), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	wantPath := filepath.Join(dir, "ralph-steps.json")
	if !strings.Contains(err.Error(), wantPath) {
		t.Errorf("error %q should contain file path %q", err.Error(), wantPath)
	}
}

// Gap 6: InjectVars JSON deserialization
func TestLoadWorkflowConfig_InjectVarsDeserialized(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[],
		"loop":[{"name":"inject","promptFile":"work.md","injectVariables":["ISSUE","SHA"]}],
		"post-loop":[]
	}`)
	cfg, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := cfg.Loop[0]
	if len(s.InjectVars) != 2 {
		t.Fatalf("expected 2 InjectVars, got %d: %v", len(s.InjectVars), s.InjectVars)
	}
	if s.InjectVars[0] != "ISSUE" || s.InjectVars[1] != "SHA" {
		t.Errorf("expected InjectVars=[ISSUE SHA], got %v", s.InjectVars)
	}
}
