package steps_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
)

// projectRoot returns the path three levels up from this test file's directory
// (internal/steps/ → ralph-tui/ → repo root). Uses runtime.Caller so it is
// independent of the working directory when tests are run. The repo root is the
// canonical project directory: it holds ralph-steps.json and the prompts/ folder,
// matching the runtime layout where the ralph-tui binary lives in the repo root.
func projectRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..", "..")
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

	result, err := steps.BuildPrompt(dir, step, nil)
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

	_, err := steps.BuildPrompt(dir, step, nil)
	if err == nil {
		t.Fatal("expected error for missing prompt file, got nil")
	}
}

func TestBuildPrompt_RealNewlines(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "nl.txt", "line one\nline two\n")
	step := steps.Step{PromptFile: "nl.txt"}

	result, err := steps.BuildPrompt(dir, step, nil)
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

	result, err := steps.BuildPrompt(dir, step, nil)
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

	result, err := steps.BuildPrompt(dir, step, nil)
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

	_, err := steps.BuildPrompt(dir, step, nil)
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

	result, err := steps.BuildPrompt(dir, step, nil)
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

	_, err := steps.BuildPrompt(dir, step, nil)
	if err == nil {
		t.Fatal("expected error when PromptFile is empty, got nil")
	}
}

// New tests per issue #22

// Test: substitute single variable in prompt
func TestBuildPrompt_SubstituteSingleVariable(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "issue.txt", "Issue: {{ISSUE_NUMBER}}\n")
	step := steps.Step{PromptFile: "issue.txt"}

	result, err := steps.BuildPrompt(dir, step, map[string]string{"ISSUE_NUMBER": "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "Issue: 42\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

// Test: substitute multiple variables in prompt
func TestBuildPrompt_SubstituteMultipleVariables(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "multi-var.txt", "Issue: {{ISSUE_NUMBER}} SHA: {{STARTING_SHA}}\n")
	step := steps.Step{PromptFile: "multi-var.txt"}

	result, err := steps.BuildPrompt(dir, step, map[string]string{
		"ISSUE_NUMBER": "42",
		"STARTING_SHA": "abc123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "Issue: 42 SHA: abc123\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

// Test: no variables in prompt — content unchanged
func TestBuildPrompt_NoVariables_Passthrough(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "plain.txt", "no templates here\n")
	step := steps.Step{PromptFile: "plain.txt"}

	result, err := steps.BuildPrompt(dir, step, map[string]string{"X": "y"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "no templates here\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

// Test: single-pass prevents injection — var value containing "{{OTHER}}" must NOT expand
func TestBuildPrompt_SinglePassPreventsInjection(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "inject.txt", "{{CAPTURED}}")
	step := steps.Step{PromptFile: "inject.txt"}

	result, err := steps.BuildPrompt(dir, step, map[string]string{
		"CAPTURED": "{{OTHER}}",
		"OTHER":    "injected",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Single-pass: "{{CAPTURED}}" becomes "{{OTHER}}" literally; not re-expanded.
	want := "{{OTHER}}"
	if result != want {
		t.Errorf("got %q, want %q (injection should be prevented)", result, want)
	}
}

// Test: unrecognized variable left as literal text
func TestBuildPrompt_UnrecognizedVarLeftAsLiteral(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "unknown.txt", "{{UNKNOWN_VAR}}")
	step := steps.Step{PromptFile: "unknown.txt"}

	result, err := steps.BuildPrompt(dir, step, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "{{UNKNOWN_VAR}}"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
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
	dir := makeTempProjectWithPrompt(t, "work.md", "do the work")
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

// T4 — BuildPrompt with nil vars when prompt contains template placeholders
func TestBuildPrompt_NilVarsWithTemplatePlaceholder(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "tmpl.txt", "do {{SOMETHING}}")
	step := steps.Step{PromptFile: "tmpl.txt"}

	result, err := steps.BuildPrompt(dir, step, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "do {{SOMETHING}}"
	if result != want {
		t.Errorf("got %q, want %q (placeholder should be left as literal with nil vars)", result, want)
	}
}

// T16 — LoadWorkflowConfig surfaces variable validation errors.
func TestLoadWorkflowConfig_VariableValidationError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[],
		"loop":[{"name":"BadCmd","command":["use","{{UNDEFINED}}"]}],
		"post-loop":[]
	}`)
	cfg, err := steps.LoadWorkflowConfig(dir, "ralph-steps.json")
	if err == nil {
		t.Fatal("expected variable validation error, got nil")
	}
	if cfg != nil {
		t.Errorf("expected nil config on error, got %+v", cfg)
	}
	if !strings.Contains(err.Error(), "references undefined variable") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Gap 6: InjectVars JSON deserialization
func TestLoadWorkflowConfig_InjectVarsDeserialized(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "work.md", "Issue: {{ISSUE}} SHA: {{SHA}}")
	writeWorkflowConfig(t, dir, `{
		"pre-loop":[
			{"name":"get-issue","command":["get-issue"],"outputVariable":"ISSUE"},
			{"name":"get-sha","command":["get-sha"],"outputVariable":"SHA"}
		],
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

// --- Production config tests ---

// TestProductionConfig_LoadsAndValidates verifies that the actual ralph-steps.json
// loads successfully, passes structural and variable validation, and has the
// expected step counts per phase.
func TestProductionConfig_LoadsAndValidates(t *testing.T) {
	root := projectRoot(t)
	cfg, err := steps.LoadWorkflowConfig(root, "ralph-steps.json")
	if err != nil {
		t.Fatalf("LoadWorkflowConfig returned error: %v", err)
	}

	if len(cfg.PreLoop) != 1 {
		t.Errorf("pre-loop: expected 1 step, got %d", len(cfg.PreLoop))
	}
	if len(cfg.Loop) != 10 {
		t.Errorf("loop: expected 10 steps, got %d", len(cfg.Loop))
	}
	if len(cfg.PostLoop) != 3 {
		t.Errorf("post-loop: expected 3 steps, got %d", len(cfg.PostLoop))
	}
}

// TestProductionConfig_StepNames verifies the step names in all three phases.
func TestProductionConfig_StepNames(t *testing.T) {
	root := projectRoot(t)
	cfg, err := steps.LoadWorkflowConfig(root, "ralph-steps.json")
	if err != nil {
		t.Fatalf("LoadWorkflowConfig returned error: %v", err)
	}

	wantPreLoop := []string{"Get GitHub username"}
	for i, want := range wantPreLoop {
		if i >= len(cfg.PreLoop) || cfg.PreLoop[i].Name != want {
			t.Errorf("pre-loop[%d]: expected %q, got %q", i, want, cfg.PreLoop[i].Name)
		}
	}

	wantLoop := []string{
		"Get next issue",
		"Get starting SHA",
		"Feature work",
		"Test planning",
		"Test writing",
		"Code review",
		"Review fixes",
		"Close issue",
		"Update docs",
		"Git push",
	}
	for i, want := range wantLoop {
		if i >= len(cfg.Loop) || cfg.Loop[i].Name != want {
			t.Errorf("loop[%d]: expected %q, got %q", i, want, cfg.Loop[i].Name)
		}
	}

	wantPostLoop := []string{"Deferred work", "Lessons learned", "Final git push"}
	for i, want := range wantPostLoop {
		if i >= len(cfg.PostLoop) || cfg.PostLoop[i].Name != want {
			t.Errorf("post-loop[%d]: expected %q, got %q", i, want, cfg.PostLoop[i].Name)
		}
	}
}

// TestPromptFiles_UseNewVariableFormat verifies that each prompt file that
// declares injectVariables contains matching {{VAR}} patterns.
func TestPromptFiles_UseNewVariableFormat(t *testing.T) {
	root := projectRoot(t)
	promptsDir := filepath.Join(root, "prompts")

	tests := []struct {
		file string
		vars []string
	}{
		{"feature-work.md", []string{"ISSUE_NUMBER"}},
		{"test-planning.md", []string{"STARTING_SHA", "ISSUE_NUMBER"}},
		{"test-writing.md", []string{"ISSUE_NUMBER"}},
		{"code-review-changes.md", []string{"STARTING_SHA", "ISSUE_NUMBER"}},
		{"code-review-fixes.md", []string{"ISSUE_NUMBER"}},
		{"update-docs.md", []string{"ISSUE_NUMBER", "STARTING_SHA"}},
	}

	for _, tc := range tests {
		data, err := os.ReadFile(filepath.Join(promptsDir, tc.file))
		if err != nil {
			t.Errorf("%s: could not read file: %v", tc.file, err)
			continue
		}
		content := string(data)
		for _, v := range tc.vars {
			pattern := "{{" + v + "}}"
			if !strings.Contains(content, pattern) {
				t.Errorf("%s: expected to contain %s", tc.file, pattern)
			}
		}
	}
}

// TestPromptFiles_NoOldVariableFormat verifies that no prompt file contains
// the old ISSUENUMBER= or STARTINGSHA= prepend format.
func TestPromptFiles_NoOldVariableFormat(t *testing.T) {
	root := projectRoot(t)
	promptsDir := filepath.Join(root, "prompts")

	entries, err := os.ReadDir(promptsDir)
	if err != nil {
		t.Fatalf("could not read prompts directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(promptsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: could not read: %v", entry.Name(), err)
			continue
		}
		content := string(data)
		if strings.Contains(content, "ISSUENUMBER=") {
			t.Errorf("%s: contains old prepend format 'ISSUENUMBER='", entry.Name())
		}
		if strings.Contains(content, "STARTINGSHA=") {
			t.Errorf("%s: contains old prepend format 'STARTINGSHA='", entry.Name())
		}
	}
}

// TestOldConfigFiles_Deleted verifies the old configs/ directory and its files
// no longer exist.
func TestOldConfigFiles_Deleted(t *testing.T) {
	root := projectRoot(t)
	oldFiles := []string{
		filepath.Join(root, "ralph-tui", "configs", "ralph-steps.json"),
		filepath.Join(root, "ralph-tui", "configs", "ralph-finalize-steps.json"),
		filepath.Join(root, "ralph-tui", "configs"),
	}
	for _, path := range oldFiles {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("expected %s to be deleted, but it exists", path)
		}
	}
}

// T44 — Production config: Claude steps have non-empty model and promptFile;
// command steps have non-empty command and no model or promptFile.
func TestProductionConfig_ClaudeAndCommandFieldsPopulated(t *testing.T) {
	root := projectRoot(t)
	cfg, err := steps.LoadWorkflowConfig(root, "ralph-steps.json")
	if err != nil {
		t.Fatalf("LoadWorkflowConfig returned error: %v", err)
	}

	allSteps := append(append(cfg.PreLoop, cfg.Loop...), cfg.PostLoop...)
	for _, s := range allSteps {
		if s.IsClaudeStep() {
			if s.PromptFile == "" {
				t.Errorf("Claude step %q: promptFile is empty", s.Name)
			}
			if s.Model == "" {
				t.Errorf("Claude step %q: model is empty", s.Name)
			}
		}
		if s.IsCommandStep() {
			if len(s.Command) == 0 {
				t.Errorf("command step %q: command is empty", s.Name)
			}
			if s.Model != "" {
				t.Errorf("command step %q: should not have model %q", s.Name, s.Model)
			}
			if s.PromptFile != "" {
				t.Errorf("command step %q: should not have promptFile %q", s.Name, s.PromptFile)
			}
		}
	}
}

// T45 — Production config: variable flow is correct across phases.
func TestProductionConfig_VariableScopingAndFlow(t *testing.T) {
	root := projectRoot(t)
	cfg, err := steps.LoadWorkflowConfig(root, "ralph-steps.json")
	if err != nil {
		t.Fatalf("LoadWorkflowConfig returned error: %v", err)
	}

	// GH_USERNAME produced by pre-loop
	if cfg.PreLoop[0].OutputVariable != "GH_USERNAME" {
		t.Errorf("pre-loop[0] outputVariable: got %q, want %q", cfg.PreLoop[0].OutputVariable, "GH_USERNAME")
	}

	// Get next issue consumes GH_USERNAME and produces ISSUE_NUMBER
	getNextIssue := cfg.Loop[0]
	if getNextIssue.OutputVariable != "ISSUE_NUMBER" {
		t.Errorf("loop[0] outputVariable: got %q, want %q", getNextIssue.OutputVariable, "ISSUE_NUMBER")
	}
	if !slices.Contains(getNextIssue.Command, "{{GH_USERNAME}}") {
		t.Errorf("loop[0] command %v: expected {{GH_USERNAME}} arg", getNextIssue.Command)
	}

	// Get starting SHA produces STARTING_SHA
	getStartingSHA := cfg.Loop[1]
	if getStartingSHA.OutputVariable != "STARTING_SHA" {
		t.Errorf("loop[1] outputVariable: got %q, want %q", getStartingSHA.OutputVariable, "STARTING_SHA")
	}

	// Downstream Claude steps inject ISSUE_NUMBER and/or STARTING_SHA
	issueNumberUsers := []string{"Feature work", "Test planning", "Test writing", "Code review", "Review fixes", "Update docs"}
	startingSHAUsers := []string{"Test planning", "Code review", "Update docs"}

	byName := make(map[string]steps.Step)
	for _, s := range cfg.Loop {
		byName[s.Name] = s
	}

	for _, name := range issueNumberUsers {
		s, ok := byName[name]
		if !ok {
			t.Errorf("step %q not found in loop", name)
			continue
		}
		if !slices.Contains(s.InjectVars, "ISSUE_NUMBER") {
			t.Errorf("step %q: expected ISSUE_NUMBER in injectVariables, got %v", name, s.InjectVars)
		}
	}

	for _, name := range startingSHAUsers {
		s, ok := byName[name]
		if !ok {
			t.Errorf("step %q not found in loop", name)
			continue
		}
		if !slices.Contains(s.InjectVars, "STARTING_SHA") {
			t.Errorf("step %q: expected STARTING_SHA in injectVariables, got %v", name, s.InjectVars)
		}
	}
}

// T46 — Production config: exitLoopIfEmpty wired only on Get next issue.
func TestProductionConfig_ExitLoopIfEmptyWiring(t *testing.T) {
	root := projectRoot(t)
	cfg, err := steps.LoadWorkflowConfig(root, "ralph-steps.json")
	if err != nil {
		t.Fatalf("LoadWorkflowConfig returned error: %v", err)
	}

	getNextIssue := cfg.Loop[0]
	if !getNextIssue.ExitLoopIfEmpty {
		t.Errorf("loop[0] %q: expected exitLoopIfEmpty=true", getNextIssue.Name)
	}
	if getNextIssue.OutputVariable != "ISSUE_NUMBER" {
		t.Errorf("loop[0] %q: expected outputVariable=ISSUE_NUMBER, got %q", getNextIssue.Name, getNextIssue.OutputVariable)
	}

	for _, s := range cfg.Loop[1:] {
		if s.ExitLoopIfEmpty {
			t.Errorf("step %q: unexpected exitLoopIfEmpty=true", s.Name)
		}
	}
	for _, s := range cfg.PreLoop {
		if s.ExitLoopIfEmpty {
			t.Errorf("pre-loop step %q: unexpected exitLoopIfEmpty=true", s.Name)
		}
	}
	for _, s := range cfg.PostLoop {
		if s.ExitLoopIfEmpty {
			t.Errorf("post-loop step %q: unexpected exitLoopIfEmpty=true", s.Name)
		}
	}
}

// T47 — Production config: command steps reference correct executables and args.
func TestProductionConfig_CommandStepsUseCorrectScripts(t *testing.T) {
	root := projectRoot(t)
	cfg, err := steps.LoadWorkflowConfig(root, "ralph-steps.json")
	if err != nil {
		t.Fatalf("LoadWorkflowConfig returned error: %v", err)
	}

	tests := []struct {
		phase string
		step  steps.Step
		want  []string
	}{
		{"pre-loop", cfg.PreLoop[0], []string{"scripts/get_gh_user"}},
		{"loop[0]", cfg.Loop[0], []string{"scripts/get_next_issue", "{{GH_USERNAME}}"}},
		{"loop[7]", cfg.Loop[7], []string{"scripts/close_gh_issue", "{{ISSUE_NUMBER}}"}},
		{"loop[9]", cfg.Loop[9], []string{"git", "push"}},
		{"post-loop[2]", cfg.PostLoop[2], []string{"git", "push"}},
	}

	for _, tc := range tests {
		if len(tc.step.Command) != len(tc.want) {
			t.Errorf("%s %q: command %v, want %v", tc.phase, tc.step.Name, tc.step.Command, tc.want)
			continue
		}
		for i, arg := range tc.want {
			if tc.step.Command[i] != arg {
				t.Errorf("%s %q: command[%d]=%q, want %q", tc.phase, tc.step.Name, i, tc.step.Command[i], arg)
			}
		}
	}
}

// T48 — Production config: every Claude step's promptFile exists on disk.
func TestProductionConfig_PromptFilesExist(t *testing.T) {
	root := projectRoot(t)
	cfg, err := steps.LoadWorkflowConfig(root, "ralph-steps.json")
	if err != nil {
		t.Fatalf("LoadWorkflowConfig returned error: %v", err)
	}

	allSteps := append(append(cfg.PreLoop, cfg.Loop...), cfg.PostLoop...)
	for _, s := range allSteps {
		if !s.IsClaudeStep() {
			continue
		}
		path := filepath.Join(root, "prompts", s.PromptFile)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("step %q: promptFile %q not found at %s", s.Name, s.PromptFile, path)
		}
	}
}

// T49 — Production config: specific steps use expected models.
func TestProductionConfig_ModelAssignments(t *testing.T) {
	root := projectRoot(t)
	cfg, err := steps.LoadWorkflowConfig(root, "ralph-steps.json")
	if err != nil {
		t.Fatalf("LoadWorkflowConfig returned error: %v", err)
	}

	byName := make(map[string]steps.Step)
	allSteps := append(append(cfg.PreLoop, cfg.Loop...), cfg.PostLoop...)
	for _, s := range allSteps {
		byName[s.Name] = s
	}

	wantModels := map[string]string{
		"Feature work":   "sonnet",
		"Test planning":  "opus",
		"Test writing":   "sonnet",
		"Code review":    "opus",
		"Review fixes":   "sonnet",
		"Update docs":    "sonnet",
		"Deferred work":  "sonnet",
		"Lessons learned": "sonnet",
	}

	for name, wantModel := range wantModels {
		s, ok := byName[name]
		if !ok {
			t.Errorf("step %q not found in config", name)
			continue
		}
		if s.Model != wantModel {
			t.Errorf("step %q: model=%q, want %q", name, s.Model, wantModel)
		}
	}
}

// T50 — projectRoot resolves to a directory containing ralph-steps.json and prompts/.
func TestProjectRoot_ContainsExpectedArtifacts(t *testing.T) {
	root := projectRoot(t)

	stepsPath := filepath.Join(root, "ralph-steps.json")
	if _, err := os.Stat(stepsPath); err != nil {
		t.Errorf("projectRoot %q: ralph-steps.json not found: %v", root, err)
	}

	promptsPath := filepath.Join(root, "prompts")
	if info, err := os.Stat(promptsPath); err != nil {
		t.Errorf("projectRoot %q: prompts/ not found: %v", root, err)
	} else if !info.IsDir() {
		t.Errorf("projectRoot %q: prompts is not a directory", root)
	}
}
