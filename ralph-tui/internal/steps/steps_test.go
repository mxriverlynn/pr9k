package steps_test

import (
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

func TestLoadSteps_NonClaudeFieldsPopulated(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}

	// "Git push" is a non-claude step
	s := got[7]
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

func TestLoadSteps_MissingOptionalFieldsNoError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "configs"), 0755); err != nil {
		t.Fatal(err)
	}
	json := `[{"name":"Only Name","isClaude":false}]`
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
	if s.Model != "" || s.PromptFile != "" || len(s.Command) != 0 {
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

// T1 — PrependVars field validation
func TestLoadSteps_PrependVars(t *testing.T) {
	got, err := steps.LoadSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadSteps returned error: %v", err)
	}

	// "Feature work" is a claude iteration step — prependVars must be true
	if !got[0].PrependVars {
		t.Error("Feature work: expected PrependVars=true")
	}

	// "Git push" is a non-claude step — prependVars must be false
	if got[7].PrependVars {
		t.Error("Git push: expected PrependVars=false")
	}
}

func TestLoadFinalizeSteps_PrependVars(t *testing.T) {
	got, err := steps.LoadFinalizeSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadFinalizeSteps returned error: %v", err)
	}

	// "Deferred work" is a finalization claude step — prependVars must be false
	if got[0].PrependVars {
		t.Error("Deferred work: expected PrependVars=false")
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

func TestLoadFinalizeSteps_NonClaudeFieldsPopulated(t *testing.T) {
	got, err := steps.LoadFinalizeSteps(projectRoot(t))
	if err != nil {
		t.Fatalf("LoadFinalizeSteps returned error: %v", err)
	}

	// "Final git push" is a non-claude step
	s := got[2]
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

func TestBuildPrompt_PrependVarsTrue(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "feature.txt", "do the thing\n")
	step := steps.Step{PromptFile: "feature.txt", PrependVars: true}

	result, err := steps.BuildPrompt(dir, step, "42", "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "ISSUENUMBER=42\nSTARTINGSHA=abc123\ndo the thing\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestBuildPrompt_PrependVarsFalse(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "finalize.txt", "wrap it up\n")
	step := steps.Step{PromptFile: "finalize.txt", PrependVars: false}

	result, err := steps.BuildPrompt(dir, step, "99", "deadbeef")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "wrap it up\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestBuildPrompt_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	step := steps.Step{PromptFile: "missing.txt", PrependVars: false}

	_, err := steps.BuildPrompt(dir, step, "1", "sha")
	if err == nil {
		t.Fatal("expected error for missing prompt file, got nil")
	}
}

func TestBuildPrompt_RealNewlines(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "nl.txt", "content\n")
	step := steps.Step{PromptFile: "nl.txt", PrependVars: true}

	result, err := steps.BuildPrompt(dir, step, "7", "sha7")
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

func TestBuildPrompt_CorrectInterpolation(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "work.txt", "body\n")
	step := steps.Step{PromptFile: "work.txt", PrependVars: true}

	result, err := steps.BuildPrompt(dir, step, "123", "feedface")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(result, "ISSUENUMBER=123\nSTARTINGSHA=feedface\n") {
		t.Errorf("result does not start with expected variable lines: %q", result)
	}
}

// BuildPrompt gap tests

func TestBuildPrompt_EmptyFile_PrependVarsTrue(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "empty.txt", "")
	step := steps.Step{PromptFile: "empty.txt", PrependVars: true}

	result, err := steps.BuildPrompt(dir, step, "42", "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "ISSUENUMBER=42\nSTARTINGSHA=abc\n"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestBuildPrompt_EmptyFile_PrependVarsFalse(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "empty.txt", "")
	step := steps.Step{PromptFile: "empty.txt", PrependVars: false}

	result, err := steps.BuildPrompt(dir, step, "42", "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "" {
		t.Errorf("got %q, want empty string", result)
	}
}

func TestBuildPrompt_SpecialCharsInVars(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "work.txt", "body\n")
	step := steps.Step{PromptFile: "work.txt", PrependVars: true}

	// issueID contains a newline — BuildPrompt does no escaping, it is inserted verbatim
	result, err := steps.BuildPrompt(dir, step, "1\n2", "sha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "ISSUENUMBER=1\n2") {
		t.Errorf("expected literal newline in issueID to be preserved verbatim, got %q", result)
	}
}

func TestBuildPrompt_NoTrailingNewline(t *testing.T) {
	dir := makeTempProjectWithPrompt(t, "notail.txt", "no trailing newline")
	step := steps.Step{PromptFile: "notail.txt", PrependVars: true}

	result, err := steps.BuildPrompt(dir, step, "42", "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "ISSUENUMBER=42\nSTARTINGSHA=abc\nno trailing newline"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
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
