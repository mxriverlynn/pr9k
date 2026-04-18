package validator_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/validator"
)

// ----------------------------------------------------------------------------
// Test helpers
// ----------------------------------------------------------------------------

// tempProject creates a temp directory that looks like a ralph-tui project root.
func tempProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeStepsJSON writes content to ralph-steps.json in dir.
func writeStepsJSON(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// writePrompt writes a prompt file under dir/prompts/.
func writePrompt(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "prompts", name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// writeScript writes an executable script under dir/scripts/.
func writeScript(t *testing.T, dir, name string) {
	t.Helper()
	p := filepath.Join(dir, "scripts", name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho ok\n"), 0755); err != nil {
		t.Fatal(err)
	}
}

// hasError returns true if any error in errs has an Error() string that
// contains the given substring.
func hasError(errs []validator.Error, sub string) bool {
	for _, e := range errs {
		if strings.Contains(e.Error(), sub) {
			return true
		}
	}
	return false
}

// requireNoErrors fails the test if errs is non-empty.
func requireNoErrors(t *testing.T, errs []validator.Error) {
	t.Helper()
	if len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("unexpected error: %s", e.Error())
		}
		t.FailNow()
	}
}

// requireError fails the test if no error in errs contains sub.
func requireError(t *testing.T, errs []validator.Error, sub string) {
	t.Helper()
	if !hasError(errs, sub) {
		t.Errorf("expected an error containing %q; got:", sub)
		for _, e := range errs {
			t.Errorf("  %s", e.Error())
		}
	}
}

// ----------------------------------------------------------------------------
// Happy paths
// ----------------------------------------------------------------------------

// TestValidate_HappyPath_Minimal validates that a minimal valid config (all three
// phases present, one iteration step with bare command) produces no errors.
func TestValidate_HappyPath_Minimal(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Step 1","isClaude":false,"command":["echo","hello"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_HappyPath_TargetConfig validates a comprehensive config exercising
// all features: captureAs, variable references in prompt files and command args,
// multiple phases, breakLoopIfEmpty.
func TestValidate_HappyPath_TargetConfig(t *testing.T) {
	dir := tempProject(t)

	// Prompt files with variable references in scope.
	// Note: {{PROJECT_DIR}} and {{WORKFLOW_DIR}} are banned from prompt files
	// (Rule B); use other in-scope variables instead.
	writePrompt(t, dir, "setup.md", "step {{STEP_NAME}}\n")
	writePrompt(t, dir, "feature.md", "implement issue {{ISSUE_ID}}\n")
	writePrompt(t, dir, "finalize.md", "finalize step {{STEP_NUM}} of {{STEP_COUNT}}\n")

	// A script for the non-claude init step.
	writeScript(t, dir, "get_issue")

	writeStepsJSON(t, dir, `{
		"initialize": [
			{
				"name": "Setup",
				"isClaude": true,
				"model": "sonnet",
				"promptFile": "setup.md"
			}
		],
		"iteration": [
			{
				"name": "Get issue",
				"isClaude": false,
				"command": ["scripts/get_issue"],
				"captureAs": "ISSUE_ID",
				"breakLoopIfEmpty": true
			},
			{
				"name": "Feature work",
				"isClaude": true,
				"model": "sonnet",
				"promptFile": "feature.md"
			}
		],
		"finalize": [
			{
				"name": "Finalize",
				"isClaude": true,
				"model": "sonnet",
				"promptFile": "finalize.md"
			},
			{
				"name": "Git push",
				"isClaude": false,
				"command": ["git", "push"]
			}
		]
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_HappyPath_EmptyInitializeAndFinalize confirms empty initialize and
// finalize arrays are valid as long as iteration has at least one step.
func TestValidate_HappyPath_EmptyInitializeAndFinalize(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_HappyPath_InitializeCaptureUsedInIteration confirms that a variable
// captured in an initialize step is in scope for iteration steps.
func TestValidate_HappyPath_InitializeCaptureUsedInIteration(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "use.md", "value: {{SETUP_OUT}}\n")
	writeScript(t, dir, "setup")

	writeStepsJSON(t, dir, `{
		"initialize": [
			{"name":"Setup","isClaude":false,"command":["scripts/setup"],"captureAs":"SETUP_OUT"}
		],
		"iteration": [
			{"name":"Use","isClaude":true,"model":"sonnet","promptFile":"use.md"}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_HappyPath_InitializeCaptureVisibleInLaterInitializeStep confirms
// that captureAs from an earlier initialize step is in scope for a later one.
func TestValidate_HappyPath_InitializeCaptureVisibleInLaterInitializeStep(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "stepA")
	writePrompt(t, dir, "stepB.md", "got {{STEP_A_OUT}}\n")

	writeStepsJSON(t, dir, `{
		"initialize": [
			{"name":"Step A","isClaude":false,"command":["scripts/stepA"],"captureAs":"STEP_A_OUT"},
			{"name":"Step B","isClaude":true,"model":"sonnet","promptFile":"stepB.md"}
		],
		"iteration": [
			{"name":"Work","isClaude":false,"command":["echo"]}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_HappyPath_IterationCaptureVisibleInLaterIterationStep confirms
// that captureAs from an earlier iteration step is in scope for a later one.
func TestValidate_HappyPath_IterationCaptureVisibleInLaterIterationStep(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "get_issue")
	writePrompt(t, dir, "work.md", "process {{ISSUE_ID}}\n")

	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Get","isClaude":false,"command":["scripts/get_issue"],"captureAs":"ISSUE_ID","breakLoopIfEmpty":true},
			{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"work.md"}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_HappyPath_IterBuiltinInScope confirms that {{ITER}} is valid in
// iteration steps.
func TestValidate_HappyPath_IterBuiltinInScope(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Step","isClaude":false,"command":["echo","{{ITER}}"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// ----------------------------------------------------------------------------
// Category 1 — file presence and parseability
// ----------------------------------------------------------------------------

func TestValidate_MissingFile(t *testing.T) {
	errs := validator.Validate("/nonexistent/path")
	requireError(t, errs, "could not read")
}

func TestValidate_MalformedJSON(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `not valid json`)

	errs := validator.Validate(dir)
	requireError(t, errs, "malformed JSON")
}

func TestValidate_MissingInitializeKey(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, `"initialize"`)
}

func TestValidate_MissingIterationKey(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{"initialize":[],"finalize":[]}`)

	errs := validator.Validate(dir)
	requireError(t, errs, `"iteration"`)
}

func TestValidate_MissingFinalizeKey(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}]
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, `"finalize"`)
}

// TestValidate_UnknownTopLevelField verifies that extra fields cause a parse error.
func TestValidate_UnknownTopLevelField(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": [],
		"extra": true
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "malformed JSON")
}

// TestValidate_UnknownStepField verifies that extra fields on a step cause a parse error.
func TestValidate_UnknownStepField(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"],"prependVars":true}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "malformed JSON")
}

// ----------------------------------------------------------------------------
// Category 2 — schema shape
// ----------------------------------------------------------------------------

func TestValidate_EmptyStepName(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "name must not be empty")
}

func TestValidate_DuplicateStepNameInPhase(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Work","isClaude":false,"command":["echo"]},
			{"name":"Work","isClaude":false,"command":["echo"]}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "duplicate step name")
}

// TestValidate_DuplicateNameAcrossPhases confirms that duplicate names across
// different phases are allowed.
func TestValidate_DuplicateNameAcrossPhases(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [{"name":"Push","isClaude":false,"command":["echo"]}],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": [{"name":"Push","isClaude":false,"command":["echo"]}]
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

func TestValidate_MissingIsClaudeField(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","command":["echo"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "isClaude is required")
}

// TestValidate_ClaudeStepMissingPromptFile verifies that a claude step without
// promptFile is an error.
func TestValidate_ClaudeStepMissingPromptFile(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"model":"sonnet"}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "non-empty promptFile")
}

// TestValidate_ClaudeStepMissingModel verifies that a claude step without model
// is an error.
func TestValidate_ClaudeStepMissingModel(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "p.md", "hello\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"promptFile":"p.md"}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "non-empty model")
}

// TestValidate_ClaudeStepHasCommand verifies that a claude step with command is
// an error.
func TestValidate_ClaudeStepHasCommand(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "p.md", "hello\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"p.md","command":["echo"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "must not have command")
}

// TestValidate_NonClaudeStepMissingCommand verifies that a non-claude step
// without command is an error.
func TestValidate_NonClaudeStepMissingCommand(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "non-empty command array")
}

// TestValidate_NonClaudeStepHasPromptFile verifies that a non-claude step with
// promptFile is an error.
func TestValidate_NonClaudeStepHasPromptFile(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "p.md", "hello\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"],"promptFile":"p.md"}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "must not have promptFile")
}

// TestValidate_CaptureAsEmptyString verifies that captureAs explicitly set to ""
// is an error.
func TestValidate_CaptureAsEmptyString(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"],"captureAs":""}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "captureAs must not be empty")
}

func TestValidate_CaptureAsShadowsReservedName(t *testing.T) {
	reserved := []string{"WORKFLOW_DIR", "PROJECT_DIR", "MAX_ITER", "ITER", "STEP_NUM", "STEP_COUNT", "STEP_NAME"}
	for _, name := range reserved {
		t.Run(name, func(t *testing.T) {
			dir := tempProject(t)
			writeStepsJSON(t, dir, fmt.Sprintf(`{
				"initialize": [],
				"iteration": [{"name":"Work","isClaude":false,"command":["echo"],"captureAs":"%s"}],
				"finalize": []
			}`, name))

			errs := validator.Validate(dir)
			requireError(t, errs, "shadows reserved built-in variable")
		})
	}
}

func TestValidate_DuplicateCaptureAsInPhase(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "s")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Step A","isClaude":false,"command":["scripts/s"],"captureAs":"RESULT"},
			{"name":"Step B","isClaude":false,"command":["scripts/s"],"captureAs":"RESULT"}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "duplicate captureAs")
}

// TestValidate_DuplicateCaptureAsAcrossPhases confirms that duplicate captureAs
// names across different phases are allowed.
func TestValidate_DuplicateCaptureAsAcrossPhases(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "s")
	writeStepsJSON(t, dir, `{
		"initialize": [
			{"name":"Init","isClaude":false,"command":["scripts/s"],"captureAs":"RESULT"}
		],
		"iteration": [
			{"name":"Iter","isClaude":false,"command":["scripts/s"],"captureAs":"RESULT"}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_BreakLoopIfEmptyRequiresCaptureAs verifies the breakLoopIfEmpty
// constraint.
func TestValidate_BreakLoopIfEmptyRequiresCaptureAs(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"],"breakLoopIfEmpty":true}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "breakLoopIfEmpty requires captureAs")
}

// TestValidate_BreakLoopIfEmptyOnlyInIteration verifies that breakLoopIfEmpty
// outside of the iteration phase is an error.
func TestValidate_BreakLoopIfEmptyOnlyInIteration(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "s")
	writeStepsJSON(t, dir, `{
		"initialize": [
			{"name":"Init","isClaude":false,"command":["scripts/s"],"captureAs":"OUT","breakLoopIfEmpty":true}
		],
		"iteration": [
			{"name":"Work","isClaude":false,"command":["echo"]}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "only valid in the iteration phase")
}

// TestValidate_BreakLoopIfEmptyRejectedInFinalize verifies that breakLoopIfEmpty
// in the finalize phase is also an error (same guard clause, distinct phase argument).
func TestValidate_BreakLoopIfEmptyRejectedInFinalize(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "s")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Work","isClaude":false,"command":["echo"]}
		],
		"finalize": [
			{"name":"Fin","isClaude":false,"command":["scripts/s"],"captureAs":"OUT","breakLoopIfEmpty":true}
		]
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "only valid in the iteration phase")
}

// TestValidate_SkipIfCaptureEmpty_ValidReference verifies that referencing a
// capture bound by an earlier iteration step is accepted.
func TestValidate_SkipIfCaptureEmpty_ValidReference(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "verdict")
	writePrompt(t, dir, "fix.md", "fix it")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Check","isClaude":false,"command":["scripts/verdict"],"captureAs":"VERDICT"},
			{"name":"Fix","isClaude":true,"model":"sonnet","promptFile":"fix.md","skipIfCaptureEmpty":"VERDICT"}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	if validator.FatalErrorCount(errs) > 0 {
		t.Errorf("expected no fatal errors, got: %v", errs)
	}
}

// TestValidate_SkipIfCaptureEmpty_UnknownCapture verifies that referencing a
// name not bound by any earlier captureAs is rejected.
func TestValidate_SkipIfCaptureEmpty_UnknownCapture(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "fix.md", "fix it")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Fix","isClaude":true,"model":"sonnet","promptFile":"fix.md","skipIfCaptureEmpty":"NONEXISTENT"}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "not bound by any earlier captureAs")
}

// TestValidate_SkipIfCaptureEmpty_ForwardReference verifies that a step cannot
// reference a capture defined by a *later* step (scope is incremental).
func TestValidate_SkipIfCaptureEmpty_ForwardReference(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "verdict")
	writePrompt(t, dir, "fix.md", "fix it")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Fix","isClaude":true,"model":"sonnet","promptFile":"fix.md","skipIfCaptureEmpty":"VERDICT"},
			{"name":"Check","isClaude":false,"command":["scripts/verdict"],"captureAs":"VERDICT"}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "not bound by any earlier captureAs")
}

// TestValidate_SkipIfCaptureEmpty_InFinalize verifies that skipIfCaptureEmpty
// is rejected in the finalize phase.
func TestValidate_SkipIfCaptureEmpty_InFinalize(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "s")
	writePrompt(t, dir, "fix.md", "fix it")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Work","isClaude":false,"command":["echo"]}
		],
		"finalize": [
			{"name":"Check","isClaude":false,"command":["scripts/s"],"captureAs":"OUT"},
			{"name":"Fix","isClaude":true,"model":"sonnet","promptFile":"fix.md","skipIfCaptureEmpty":"OUT"}
		]
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "only valid in the iteration phase")
}

// TestValidate_SkipIfCaptureEmpty_EmptyString verifies that setting
// skipIfCaptureEmpty to an empty string is rejected with the dedicated error
// and does NOT also fire the "not bound by any earlier captureAs" branch.
func TestValidate_SkipIfCaptureEmpty_EmptyString(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "fix.md", "fix it")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Fix","isClaude":true,"model":"sonnet","promptFile":"fix.md","skipIfCaptureEmpty":""}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "skipIfCaptureEmpty must not be empty when set")
	if hasError(errs, "not bound by any earlier captureAs") {
		t.Error("expected no 'not bound by any earlier captureAs' error for empty-string case")
	}
}

// TestValidate_SkipIfCaptureEmpty_InInitialize verifies that skipIfCaptureEmpty
// is rejected in the initialize phase (symmetric to the finalize test).
func TestValidate_SkipIfCaptureEmpty_InInitialize(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "setup")
	writePrompt(t, dir, "fix.md", "fix it")
	writeStepsJSON(t, dir, `{
		"initialize": [
			{"name":"Setup","isClaude":false,"command":["scripts/setup"],"captureAs":"OUT"},
			{"name":"Fix","isClaude":true,"model":"sonnet","promptFile":"fix.md","skipIfCaptureEmpty":"OUT"}
		],
		"iteration": [
			{"name":"Work","isClaude":false,"command":["echo"]}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "only valid in the iteration phase")
}

// TestValidate_SkipIfCaptureEmpty_MultipleReferents verifies that multiple steps
// may reference the same captured variable without triggering scope errors.
func TestValidate_SkipIfCaptureEmpty_MultipleReferents(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "verdict")
	writePrompt(t, dir, "fix1.md", "fix 1")
	writePrompt(t, dir, "fix2.md", "fix 2")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Check","isClaude":false,"command":["scripts/verdict"],"captureAs":"OUT"},
			{"name":"Fix1","isClaude":true,"model":"sonnet","promptFile":"fix1.md","skipIfCaptureEmpty":"OUT"},
			{"name":"Fix2","isClaude":true,"model":"sonnet","promptFile":"fix2.md","skipIfCaptureEmpty":"OUT"}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	if validator.FatalErrorCount(errs) > 0 {
		t.Errorf("expected no fatal errors, got: %v", errs)
	}
}

// TestValidate_SkipIfCaptureEmpty_InitializeCapture verifies that referencing a
// capture bound in the initialize phase is rejected. The runtime captureStates
// map is populated per-iteration only, so an initialize-phase capture would
// silently never trigger the skip.
func TestValidate_SkipIfCaptureEmpty_InitializeCapture(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "setup")
	writePrompt(t, dir, "fix.md", "fix it")
	writeStepsJSON(t, dir, `{
		"initialize": [
			{"name":"Setup","isClaude":false,"command":["scripts/setup"],"captureAs":"INIT_OUT"}
		],
		"iteration": [
			{"name":"Fix","isClaude":true,"model":"sonnet","promptFile":"fix.md","skipIfCaptureEmpty":"INIT_OUT"}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "not bound by any earlier captureAs")
}

// ----------------------------------------------------------------------------
// Category 3 — phase-size checks
// ----------------------------------------------------------------------------

func TestValidate_EmptyIterationArray(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{"initialize":[],"iteration":[],"finalize":[]}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "iteration array must have at least 1 step")
}

// TestValidate_SingleIterationStepIsValid confirms that exactly one step is enough.
func TestValidate_SingleIterationStepIsValid(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// ----------------------------------------------------------------------------
// Category 4 — referenced files exist
// ----------------------------------------------------------------------------

func TestValidate_MissingPromptFile(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"missing.md"}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, `"missing.md" not found`)
}

func TestValidate_MissingRelativeCommand(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["scripts/nonexistent"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "not found")
}

func TestValidate_MissingBareCommand(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["this-command-definitely-does-not-exist-42"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "not found")
}

func TestValidate_BareCommandInPath(t *testing.T) {
	dir := tempProject(t)
	// "echo" is always in PATH; should produce no error for command[0].
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo","hello"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

func TestValidate_AbsoluteCommandExists(t *testing.T) {
	dir := tempProject(t)
	// Use /bin/sh which exists on all Unix systems.
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["/bin/sh","-c","echo hi"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

func TestValidate_AbsoluteCommandMissing(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["/nonexistent/bin/tool"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "not found")
}

// ----------------------------------------------------------------------------
// Category 5 — variable reference resolution
// ----------------------------------------------------------------------------

func TestValidate_UnresolvedReferenceInPrompt(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "p.md", "need {{UNKNOWN_VAR}}\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"p.md"}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "{{UNKNOWN_VAR}}")
}

func TestValidate_UnresolvedReferenceInCommand(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo","{{MISSING}}"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "{{MISSING}}")
}

// TestValidate_BuiltinVarsInScope confirms that all initialize seeds are
// available without being declared. Note: {{PROJECT_DIR}} and {{WORKFLOW_DIR}}
// are banned from prompt files (Rule B) — use command steps to verify those.
func TestValidate_BuiltinVarsInScope(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "p.md",
		"max={{MAX_ITER}} num={{STEP_NUM}} count={{STEP_COUNT}} name={{STEP_NAME}}\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"p.md"}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_BuiltinDirVarsValidInCommandScope confirms that {{PROJECT_DIR}}
// and {{WORKFLOW_DIR}} remain valid in command argv (non-claude steps).
func TestValidate_BuiltinDirVarsValidInCommandScope(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Show","isClaude":false,"command":["cat","{{PROJECT_DIR}}/README.md"]},
			{"name":"Art","isClaude":false,"command":["cat","{{WORKFLOW_DIR}}/ralph-art.txt"]}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_IterBuiltinNotInInitialize confirms that {{ITER}} referenced in
// an initialize step is a validation error.
func TestValidate_IterBuiltinNotInInitialize(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "init.md", "iter={{ITER}}\n")
	writeStepsJSON(t, dir, `{
		"initialize": [{"name":"Init","isClaude":true,"model":"sonnet","promptFile":"init.md"}],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "{{ITER}}")
}

// TestValidate_IterBuiltinNotInFinalize confirms that {{ITER}} referenced in a
// finalize step is a validation error.
func TestValidate_IterBuiltinNotInFinalize(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "fin.md", "iter={{ITER}}\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": [{"name":"Fin","isClaude":true,"model":"sonnet","promptFile":"fin.md"}]
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "{{ITER}}")
}

// TestValidate_InitCaptureVisibleInFinalize confirms that a variable captured
// via captureAs in an initialize step is in scope for finalize steps (persistent
// scope propagation).
func TestValidate_InitCaptureVisibleInFinalize(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "init")
	writePrompt(t, dir, "fin.md", "value={{INIT_OUT}}\n")

	writeStepsJSON(t, dir, `{
		"initialize": [
			{"name":"Init","isClaude":false,"command":["scripts/init"],"captureAs":"INIT_OUT"}
		],
		"iteration": [
			{"name":"Work","isClaude":false,"command":["echo"]}
		],
		"finalize": [
			{"name":"Use","isClaude":true,"model":"sonnet","promptFile":"fin.md"}
		]
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_IterCaptureNotInFinalize confirms that a variable captured in the
// iteration phase is NOT available in the finalize phase.
func TestValidate_IterCaptureNotInFinalize(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "get")
	writePrompt(t, dir, "fin.md", "value={{ITER_RESULT}}\n")

	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Get","isClaude":false,"command":["scripts/get"],"captureAs":"ITER_RESULT"}
		],
		"finalize": [
			{"name":"Use","isClaude":true,"model":"sonnet","promptFile":"fin.md"}
		]
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "{{ITER_RESULT}}")
}

// TestValidate_UnresolvedInStepBeforeProducer confirms that a step referencing a
// variable before the step that produces it is an error.
func TestValidate_UnresolvedInStepBeforeProducer(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "s")
	writePrompt(t, dir, "early.md", "need {{RESULT}}\n")

	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Early","isClaude":true,"model":"sonnet","promptFile":"early.md"},
			{"name":"Producer","isClaude":false,"command":["scripts/s"],"captureAs":"RESULT"}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "{{RESULT}}")
}

// TestValidate_IterCaptureVisibleAfterProducer is the positive counterpart to the
// test above: the step referencing RESULT comes after the step that produces it.
func TestValidate_IterCaptureVisibleAfterProducer(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "s")
	writePrompt(t, dir, "late.md", "need {{RESULT}}\n")

	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Producer","isClaude":false,"command":["scripts/s"],"captureAs":"RESULT"},
			{"name":"Consumer","isClaude":true,"model":"sonnet","promptFile":"late.md"}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_InitializeCaptureNotVisibleBeforeProducer confirms that a step
// referencing an initialize-phase variable before it is declared is an error.
func TestValidate_InitializeCaptureNotVisibleBeforeProducer(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "s")
	writePrompt(t, dir, "early.md", "need {{EARLY_OUT}}\n")

	writeStepsJSON(t, dir, `{
		"initialize": [
			{"name":"Consumer","isClaude":true,"model":"sonnet","promptFile":"early.md"},
			{"name":"Producer","isClaude":false,"command":["scripts/s"],"captureAs":"EARLY_OUT"}
		],
		"iteration": [
			{"name":"Work","isClaude":false,"command":["echo"]}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	requireError(t, errs, "{{EARLY_OUT}}")
}

// ----------------------------------------------------------------------------
// Error collection — multiple errors in one pass
// ----------------------------------------------------------------------------

// TestValidate_CollectsMultipleErrors confirms that validation does not stop at
// the first error; it reports all problems found.
func TestValidate_CollectsMultipleErrors(t *testing.T) {
	dir := tempProject(t)
	// Two steps: both have empty names and missing isClaude.
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"","command":["echo"]},
			{"name":"","command":["echo"]}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	if len(errs) < 2 {
		t.Errorf("expected at least 2 errors, got %d: %v", len(errs), errs)
	}
}

// TestValidate_CollectsErrorsAcrossPhases confirms that errors in multiple phases
// are all returned together.
func TestValidate_CollectsErrorsAcrossPhases(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [{"name":"","command":["echo"]}],
		"iteration": [{"name":"","command":["echo"]}],
		"finalize": [{"name":"","command":["echo"]}]
	}`)

	errs := validator.Validate(dir)
	phases := map[string]bool{}
	for _, e := range errs {
		phases[e.Phase] = true
	}
	for _, ph := range []string{"initialize", "iteration", "finalize"} {
		if !phases[ph] {
			t.Errorf("expected an error from phase %q, but none found", ph)
		}
	}
}

// TestValidate_FileAndSchemaErrorsCollected confirms that a config with both a
// missing prompt file and a schema violation produces errors for both.
func TestValidate_FileAndSchemaErrorsCollected(t *testing.T) {
	dir := tempProject(t)
	// Step has duplicate name AND references a missing prompt file.
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"missing.md"},
			{"name":"Work","isClaude":false,"command":["echo"]}
		],
		"finalize": []
	}`)

	errs := validator.Validate(dir)
	hasFile := hasError(errs, "not found")
	hasSchema := hasError(errs, "duplicate step name")
	if !hasFile {
		t.Error("expected a file-not-found error")
	}
	if !hasSchema {
		t.Error("expected a duplicate-name schema error")
	}
}

// ----------------------------------------------------------------------------
// Error type — formatting
// ----------------------------------------------------------------------------

func TestError_FormatWithStepName(t *testing.T) {
	e := validator.Error{
		Category: "schema",
		Phase:    "iteration",
		StepName: "My Step",
		Problem:  "isClaude is required",
	}
	got := e.Error()
	want := `config error: schema: iteration step "My Step": isClaude is required`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestError_FormatWithoutStepName(t *testing.T) {
	e := validator.Error{
		Category: "file",
		Phase:    "config",
		StepName: "",
		Problem:  "could not read /path/to/file: no such file or directory",
	}
	got := e.Error()
	want := `config error: file: config: could not read /path/to/file: no such file or directory`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ----------------------------------------------------------------------------
// Category 10 — env passthrough names
// ----------------------------------------------------------------------------

// TestValidate_Env_MissingKey confirms that an absent "env" key is valid (treated
// as empty list).
func TestValidate_Env_MissingKey(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_Env_EmptyArray confirms that env: [] is valid.
func TestValidate_Env_EmptyArray(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": [],
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_Env_SingleValid confirms that a single valid name is accepted.
func TestValidate_Env_SingleValid(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": ["GITHUB_TOKEN"],
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_Env_MultipleValid confirms that multiple valid names are accepted.
func TestValidate_Env_MultipleValid(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": ["GITHUB_TOKEN", "AWS_ACCESS_KEY_ID"],
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_Env_OverlapWithBuiltins confirms that overlap with built-in
// variable names (e.g. ANTHROPIC_API_KEY) is harmless.
func TestValidate_Env_OverlapWithBuiltins(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": ["ANTHROPIC_API_KEY"],
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_Env_DuplicatesAllowed confirms that duplicate names within the
// env list are harmless.
func TestValidate_Env_DuplicatesAllowed(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": ["MY_VAR", "MY_VAR"],
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_Env_NonArrayValue confirms that env: "FOO" (non-array) causes a
// parse error.
func TestValidate_Env_NonArrayValue(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": "GITHUB_TOKEN",
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "malformed JSON")
}

// TestValidate_Env_NonStringElement confirms that env: [123] (non-string
// element) causes a parse error.
func TestValidate_Env_NonStringElement(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": [123],
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "malformed JSON")
}

// TestValidate_Env_EmptyStringElement confirms that an empty string element is
// rejected.
func TestValidate_Env_EmptyStringElement(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": [""],
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "env name must not be empty")
}

// TestValidate_Env_StartsWithDigit confirms that a name starting with a digit
// is rejected.
func TestValidate_Env_StartsWithDigit(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": ["1BAD"],
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "not a valid identifier")
}

// TestValidate_Env_HyphenNotAllowed confirms that a name containing a hyphen is
// rejected.
func TestValidate_Env_HyphenNotAllowed(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": ["FOO-BAR"],
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "not a valid identifier")
}

// TestValidate_Env_SandboxReserved confirms that sandbox-reserved names are
// rejected with a reason message. Parameterized over reserved names.
func TestValidate_Env_SandboxReserved(t *testing.T) {
	reserved := []string{"CLAUDE_CONFIG_DIR", "HOME"}
	for _, name := range reserved {
		t.Run(name, func(t *testing.T) {
			dir := tempProject(t)
			writeStepsJSON(t, dir, fmt.Sprintf(`{
				"env": [%q],
				"initialize": [],
				"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
				"finalize": []
			}`, name))
			errs := validator.Validate(dir)
			requireError(t, errs, "reserved by the sandbox")
		})
	}
}

// TestValidate_Env_Denylist confirms that denylisted names are rejected.
// Parameterized over all eight denylisted names.
func TestValidate_Env_Denylist(t *testing.T) {
	denylisted := []string{
		"PATH", "USER", "LOGNAME", "SSH_AUTH_SOCK",
		"LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES", "DYLD_LIBRARY_PATH",
	}
	for _, name := range denylisted {
		t.Run(name, func(t *testing.T) {
			dir := tempProject(t)
			writeStepsJSON(t, dir, fmt.Sprintf(`{
				"env": [%q],
				"initialize": [],
				"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
				"finalize": []
			}`, name))
			errs := validator.Validate(dir)
			requireError(t, errs, "denylisted: would break container isolation")
		})
	}
}

// ----------------------------------------------------------------------------
// captureAs on claude steps (D6 — Rule A removed)
// ----------------------------------------------------------------------------

// TestValidate_ClaudeStepWithCaptureAsIsClean confirms that a step with
// isClaude:true and a non-empty captureAs is accepted (D6: captureAs on claude
// steps binds to result.result, not docker stdout).
func TestValidate_ClaudeStepWithCaptureAsIsClean(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "p.md", "hello\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"p.md","captureAs":"RESULT"}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_ClaudeStepWithoutCaptureAsIsClean confirms that a claude
// step without captureAs is accepted.
func TestValidate_ClaudeStepWithoutCaptureAsIsClean(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "p.md", "hello\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"p.md"}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_NonClaudeStepWithCaptureAsIsClean confirms that a
// non-claude step with captureAs is accepted.
func TestValidate_NonClaudeStepWithCaptureAsIsClean(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "s")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["scripts/s"],"captureAs":"X"}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// ----------------------------------------------------------------------------
// Rule B — prompt-token ban
// ----------------------------------------------------------------------------

// TestValidate_RuleB_WorkflowDirInPromptIsError confirms that {{WORKFLOW_DIR}}
// in a prompt file referenced by a claude step is rejected. The error names
// only the token that was actually found.
func TestValidate_RuleB_WorkflowDirInPromptIsError(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "p.md", "install dir: {{WORKFLOW_DIR}}\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"p.md"}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "{{WORKFLOW_DIR}}")
	requireError(t, errs, "not valid inside prompt files")
	requireError(t, errs, "expand to host paths that do not exist inside the sandbox")
}

// TestValidate_RuleB_ProjectDirInPromptIsError confirms that {{PROJECT_DIR}}
// in a prompt file referenced by a claude step is rejected. The error names
// only the token that was actually found.
func TestValidate_RuleB_ProjectDirInPromptIsError(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "p.md", "target repo: {{PROJECT_DIR}}\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"p.md"}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "{{PROJECT_DIR}}")
	requireError(t, errs, "not valid inside prompt files")
}

// TestValidate_RuleB_BothTokensInPromptEmitsError confirms that a prompt
// containing both tokens emits at least one error.
func TestValidate_RuleB_BothTokensInPromptEmitsError(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "p.md", "workflow={{WORKFLOW_DIR}} project={{PROJECT_DIR}}\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"p.md"}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	if !hasError(errs, "not valid inside prompt files") {
		t.Errorf("expected at least one sandbox prompt-token error; got: %v", errs)
	}
}

// TestValidate_RuleB_PromptWithNeitherTokenIsClean confirms that a prompt file
// without either banned token is accepted.
func TestValidate_RuleB_PromptWithNeitherTokenIsClean(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "p.md", "no banned tokens here\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"p.md"}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_RuleB_WorkflowDirInCommandArgIsClean confirms that
// {{WORKFLOW_DIR}} in a non-claude command step argv is NOT flagged by Rule B.
func TestValidate_RuleB_WorkflowDirInCommandArgIsClean(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Art","isClaude":false,"command":["cat","{{WORKFLOW_DIR}}/ralph-art.txt"]}
		],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// ----------------------------------------------------------------------------
// Rule C — command + captureAs + forbidden token
// ----------------------------------------------------------------------------

// TestValidate_RuleC_ProjectDirInArgWithCaptureAsIsError confirms that a
// command step referencing {{PROJECT_DIR}} in argv with captureAs set is
// rejected.
func TestValidate_RuleC_ProjectDirInArgWithCaptureAsIsError(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Read","isClaude":false,"command":["cat","{{PROJECT_DIR}}/README.md"],"captureAs":"README"}
		],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "captureAs on a command step that references {{WORKFLOW_DIR}} or {{PROJECT_DIR}} is not allowed")
	requireError(t, errs, "stale value inside the sandbox")
}

// TestValidate_RuleC_WorkflowDirInArgWithCaptureAsIsError confirms that a
// command step referencing {{WORKFLOW_DIR}} in argv with captureAs set is
// rejected.
func TestValidate_RuleC_WorkflowDirInArgWithCaptureAsIsError(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Art","isClaude":false,"command":["cat","{{WORKFLOW_DIR}}/ralph-art.txt"],"captureAs":"ART"}
		],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "captureAs on a command step that references {{WORKFLOW_DIR}} or {{PROJECT_DIR}} is not allowed")
}

// TestValidate_RuleC_ProjectDirInArgWithoutCaptureAsIsClean confirms that a
// command step referencing {{PROJECT_DIR}} in argv WITHOUT captureAs is fine.
func TestValidate_RuleC_ProjectDirInArgWithoutCaptureAsIsClean(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Show","isClaude":false,"command":["cat","{{PROJECT_DIR}}/README.md"]}
		],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_RuleC_CaptureAsWithoutForbiddenTokenIsClean confirms that a
// command step with captureAs but no {{WORKFLOW_DIR}}/{{PROJECT_DIR}} in argv
// is not affected by Rule C.
func TestValidate_RuleC_CaptureAsWithoutForbiddenTokenIsClean(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "get_issue")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Get","isClaude":false,"command":["scripts/get_issue"],"captureAs":"ISSUE_ID","breakLoopIfEmpty":true}
		],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TP-003: Env validation continues after invalid entry — mixed list
func TestValidate_Env_ContinuesAfterInvalidEntry(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": ["", "VALID_NAME", "PATH"],
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	if !hasError(errs, "env name must not be empty") {
		t.Error("expected error for empty env name")
	}
	if !hasError(errs, "denylisted") {
		t.Error(`expected error for denylisted env name "PATH"`)
	}
	if hasError(errs, "VALID_NAME") {
		t.Error("expected no error for valid env name VALID_NAME")
	}
	if len(errs) < 2 {
		t.Errorf("expected at least 2 errors (empty + denylisted), got %d: %v", len(errs), errs)
	}
}

// TP-004: claude step with captureAs in initialize phase is valid (D6)
func TestValidate_ClaudeStepWithCaptureAsInInitializePhaseIsClean(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "init.md", "setting up\n")
	writeStepsJSON(t, dir, `{
		"initialize": [{"name":"Setup","isClaude":true,"model":"sonnet","promptFile":"init.md","captureAs":"SETUP_OUT"}],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TP-005: Rule B — prompt-token ban in initialize phase
func TestValidate_RuleB_WorkflowDirInInitializePromptIsError(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "init.md", "install dir: {{WORKFLOW_DIR}}\n")
	writeStepsJSON(t, dir, `{
		"initialize": [{"name":"Setup","isClaude":true,"model":"sonnet","promptFile":"init.md"}],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "not valid inside prompt files")
}

// TP-006: Rule B — prompt-token ban in finalize phase
func TestValidate_RuleB_ProjectDirInFinalizePromptIsError(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "fin.md", "target repo: {{PROJECT_DIR}}\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": [{"name":"Fin","isClaude":true,"model":"sonnet","promptFile":"fin.md"}]
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "not valid inside prompt files")
}

// TP-007: Rule C — forbidden token in command[0] position
func TestValidate_RuleC_ForbiddenTokenInCommandZeroIsError(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [
			{"name":"Run","isClaude":false,"command":["{{WORKFLOW_DIR}}/scripts/run","arg1"],"captureAs":"OUT"}
		],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "captureAs on a command step that references {{WORKFLOW_DIR}} or {{PROJECT_DIR}} is not allowed")
}

// TP-008: Env errors do not block phase validation
func TestValidate_Env_ErrorsDoNotBlockPhaseValidation(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": ["PATH"],
		"initialize": [],
		"iteration": [{"name":"","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	if !hasError(errs, "denylisted") {
		t.Error(`expected error for denylisted env name "PATH"`)
	}
	if !hasError(errs, "name must not be empty") {
		t.Error("expected error for empty step name in iteration phase")
	}
}

// ----------------------------------------------------------------------------
// D6 — claude step with captureAs in finalize phase is valid
// ----------------------------------------------------------------------------

// TestValidate_ClaudeStepWithCaptureAsInFinalizePhaseIsClean confirms that
// captureAs on a claude step is accepted in the finalize phase (D6).
func TestValidate_ClaudeStepWithCaptureAsInFinalizePhaseIsClean(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "fin.md", "wrapping up\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": [{"name":"Wrap","isClaude":true,"model":"sonnet","promptFile":"fin.md","captureAs":"WRAP_OUT"}]
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// ----------------------------------------------------------------------------
// WARN-006: Rule C — captureAs + forbidden token in initialize phase
// WARN-007: Rule C — captureAs + forbidden token in finalize phase
// ----------------------------------------------------------------------------

// TestValidate_RuleC_ForbiddenTokenInInitializePhaseIsError confirms that Rule C
// fires in the initialize phase (not only iteration).
func TestValidate_RuleC_ForbiddenTokenInInitializePhaseIsError(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [
			{"name":"Setup","isClaude":false,"command":["cat","{{PROJECT_DIR}}/config.json"],"captureAs":"CONFIG"}
		],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "captureAs on a command step that references {{WORKFLOW_DIR}} or {{PROJECT_DIR}} is not allowed")
}

// TestValidate_RuleC_ForbiddenTokenInFinalizePhaseIsError confirms that Rule C
// fires in the finalize phase (not only iteration).
func TestValidate_RuleC_ForbiddenTokenInFinalizePhaseIsError(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": [
			{"name":"Archive","isClaude":false,"command":["cat","{{WORKFLOW_DIR}}/ralph-art.txt"],"captureAs":"ART"}
		]
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "captureAs on a command step that references {{WORKFLOW_DIR}} or {{PROJECT_DIR}} is not allowed")
}

// ----------------------------------------------------------------------------
// WARN-009: Rule B — escaped tokens must not produce a false positive
// ----------------------------------------------------------------------------

// TestValidate_RuleB_EscapedTokenIsNotFlaggedAsFalsePositive confirms that
// {{{{WORKFLOW_DIR}}}} (which renders as the literal text {{WORKFLOW_DIR}} after
// substitution) does not trigger Rule B, because vars.ExtractReferences skips
// escape sequences.
func TestValidate_RuleB_EscapedTokenIsNotFlaggedAsFalsePositive(t *testing.T) {
	dir := tempProject(t)
	// {{{{WORKFLOW_DIR}}}} is the escape sequence for literal {{WORKFLOW_DIR}}.
	writePrompt(t, dir, "p.md", "show literal: {{{{WORKFLOW_DIR}}}}\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"p.md"}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// ----------------------------------------------------------------------------
// SUGG-003: Early-return guard — multiple missing phases produce errors without crash
// ----------------------------------------------------------------------------

// TestValidate_MultipleMissingPhasesNoScopeWalkCrash confirms that when more
// than one required top-level array is absent, all missing-key errors are
// reported and the validator returns without attempting a scope walk (which
// would crash on nil phase pointers).
func TestValidate_MultipleMissingPhasesNoScopeWalkCrash(t *testing.T) {
	dir := tempProject(t)
	// Both "initialize" and "finalize" are missing; only "iteration" is present.
	writeStepsJSON(t, dir, `{
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}]
	}`)
	errs := validator.Validate(dir)
	if !hasError(errs, `"initialize"`) {
		t.Error(`expected error for missing "initialize" key`)
	}
	if !hasError(errs, `"finalize"`) {
		t.Error(`expected error for missing "finalize" key`)
	}
	// Confirm the validator did not panic (reaching here means it didn't).
}

// ----------------------------------------------------------------------------
// SUGG-004: Env name regex — spaces and dots are rejected
// ----------------------------------------------------------------------------

// TestValidate_Env_SpaceNotAllowed confirms that an env name containing a space
// is rejected by the identifier regex.
func TestValidate_Env_SpaceNotAllowed(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": ["MY VAR"],
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "not a valid identifier")
}

// TestValidate_Env_DotNotAllowed confirms that an env name containing a dot is
// rejected by the identifier regex.
func TestValidate_Env_DotNotAllowed(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": ["my.var"],
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "not a valid identifier")
}

// ----------------------------------------------------------------------------
// SEC-001: Path traversal — prompt path escaping prompts directory is rejected
// ----------------------------------------------------------------------------

// TestValidate_PromptPathTraversalIsError confirms that a promptFile value
// containing ".." that escapes the prompts/ directory is rejected by the
// containment check, not treated as a simple missing-file error.
func TestValidate_PromptPathTraversalIsError(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"../../secret.txt"}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "escapes prompts directory")
}

// ----------------------------------------------------------------------------
// statusLine validation
// ----------------------------------------------------------------------------

func minimalWithStatusLine(extra string) string {
	return fmt.Sprintf(`{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": [],
		%s
	}`, extra)
}

func TestValidate_StatusLine_Absent(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":false,"command":["echo"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

func TestValidate_StatusLine_ValidFull(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "status.sh")
	writeStepsJSON(t, dir, minimalWithStatusLine(`"statusLine":{"type":"command","command":"scripts/status.sh","refreshIntervalSeconds":5}`))
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

func TestValidate_StatusLine_CommandMissing(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, minimalWithStatusLine(`"statusLine":{}`))
	errs := validator.Validate(dir)
	requireError(t, errs, "command must not be empty")
}

func TestValidate_StatusLine_CommandEmpty(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, minimalWithStatusLine(`"statusLine":{"command":""}`))
	errs := validator.Validate(dir)
	requireError(t, errs, "command must not be empty")
}

func TestValidate_StatusLine_CommandNotFound(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, minimalWithStatusLine(`"statusLine":{"command":"scripts/nonexistent.sh"}`))
	errs := validator.Validate(dir)
	requireError(t, errs, "not found")
}

func TestValidate_StatusLine_RefreshIntervalAbsent(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "status.sh")
	writeStepsJSON(t, dir, minimalWithStatusLine(`"statusLine":{"command":"scripts/status.sh"}`))
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

func TestValidate_StatusLine_RefreshIntervalZero(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "status.sh")
	writeStepsJSON(t, dir, minimalWithStatusLine(`"statusLine":{"command":"scripts/status.sh","refreshIntervalSeconds":0}`))
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

func TestValidate_StatusLine_RefreshIntervalNegative(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "status.sh")
	writeStepsJSON(t, dir, minimalWithStatusLine(`"statusLine":{"command":"scripts/status.sh","refreshIntervalSeconds":-1}`))
	errs := validator.Validate(dir)
	requireError(t, errs, "refreshIntervalSeconds must be >= 0")
}

func TestValidate_StatusLine_TypeCommand(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "status.sh")
	writeStepsJSON(t, dir, minimalWithStatusLine(`"statusLine":{"type":"command","command":"scripts/status.sh"}`))
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

func TestValidate_StatusLine_TypeBogus(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "status.sh")
	writeStepsJSON(t, dir, minimalWithStatusLine(`"statusLine":{"type":"bogus","command":"scripts/status.sh"}`))
	errs := validator.Validate(dir)
	requireError(t, errs, `type must be "command" (or omitted)`)
}

func TestValidate_StatusLine_UnknownField(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "status.sh")
	writeStepsJSON(t, dir, minimalWithStatusLine(`"statusLine":{"command":"scripts/status.sh","unknownField":"oops"}`))
	errs := validator.Validate(dir)
	requireError(t, errs, "unknown field")
}

// ----------------------------------------------------------------------------
// T4: Validator emits statusLine errors with Category="statusline", Phase="config", no StepName
// ----------------------------------------------------------------------------

func TestValidate_StatusLine_StructuredErrorLabels(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "status.sh")
	writeStepsJSON(t, dir, minimalWithStatusLine(`"statusLine":{"type":"bogus","command":"scripts/status.sh"}`))
	errs := validator.Validate(dir)

	for _, e := range errs {
		if e.Category == "statusline" {
			if e.Phase != "config" {
				t.Errorf("expected Phase %q, got %q", "config", e.Phase)
			}
			if e.StepName != "" {
				t.Errorf("expected empty StepName, got %q", e.StepName)
			}
			return
		}
	}
	t.Errorf("expected a statusline error, got: %v", errs)
}

// ----------------------------------------------------------------------------
// T5: Validator collects statusLine errors alongside phase errors
// ----------------------------------------------------------------------------

func TestValidate_StatusLine_ErrorsCollectedAlongsidePhaseErrors(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","command":["echo"]}],
		"finalize": [],
		"statusLine": {"type":"bogus","command":"echo"}
	}`)
	errs := validator.Validate(dir)
	if !hasError(errs, `type must be`) {
		t.Error("expected a statusLine type error")
	}
	if !hasError(errs, "isClaude is required") {
		t.Error("expected an isClaude schema error")
	}
}

// ----------------------------------------------------------------------------
// T6: Validator resolves statusLine command via exec.LookPath when bare-named
// ----------------------------------------------------------------------------

func TestValidate_StatusLine_BareCommandInPath(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, minimalWithStatusLine(`"statusLine":{"command":"echo"}`))
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// ----------------------------------------------------------------------------
// containerEnv validation
// ----------------------------------------------------------------------------

func minimalWithContainerEnv(t *testing.T) string {
	t.Helper()
	return `{
		"initialize": [],
		"iteration": [{"name":"Step 1","isClaude":false,"command":["echo","hi"]}],
		"finalize": []
	}`
}

// TestValidate_ContainerEnv_Valid verifies that a well-formed containerEnv block
// produces no errors.
func TestValidate_ContainerEnv_Valid(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"containerEnv": {"GOPATH": "/tmp/go", "GOCACHE": "/tmp/gocache"},
		"initialize": [],
		"iteration": [{"name":"S","isClaude":false,"command":["echo","ok"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_ContainerEnv_RejectsCLAUDE_CONFIG_DIR verifies that using the
// sandbox-reserved key "CLAUDE_CONFIG_DIR" is rejected as a fatal error.
func TestValidate_ContainerEnv_RejectsCLAUDE_CONFIG_DIR(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"containerEnv": {"CLAUDE_CONFIG_DIR": "/foo"},
		"initialize": [],
		"iteration": [{"name":"S","isClaude":false,"command":["echo","ok"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, `"CLAUDE_CONFIG_DIR" is reserved by the sandbox`)
	for _, e := range errs {
		if !e.IsFatal() {
			continue
		}
		if strings.Contains(e.Error(), "CLAUDE_CONFIG_DIR") {
			return
		}
	}
	t.Error("expected CLAUDE_CONFIG_DIR rejection to be a fatal error")
}

// TestValidate_ContainerEnv_RejectsKeyWithEquals verifies that a key containing
// "=" is rejected as a fatal error.
func TestValidate_ContainerEnv_RejectsKeyWithEquals(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"containerEnv": {"BAD=KEY": "value"},
		"initialize": [],
		"iteration": [{"name":"S","isClaude":false,"command":["echo","ok"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "must not contain '='")
}

// TestValidate_ContainerEnv_RejectsValueWithNewline verifies that a value
// containing a newline character is rejected as a fatal error.
func TestValidate_ContainerEnv_RejectsValueWithNewline(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"containerEnv": {"MYVAR": "line1\nline2"},
		"initialize": [],
		"iteration": [{"name":"S","isClaude":false,"command":["echo","ok"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "newline or NUL")
}

// TestValidate_ContainerEnv_RejectsValueWithNUL verifies that a value
// containing a NUL character is rejected as a fatal error. A NUL byte in a
// JSON string is invalid JSON, so the rejection may come from the JSON parser
// ("malformed JSON") or from the containerEnv validator ("newline or NUL").
// Either is an acceptable fatal rejection.
func TestValidate_ContainerEnv_RejectsValueWithNUL(t *testing.T) {
	dir := tempProject(t)
	content := "{\"containerEnv\":{\"MYVAR\":\"val\x00ue\"},\"initialize\":[],\"iteration\":[{\"name\":\"S\",\"isClaude\":false,\"command\":[\"echo\",\"ok\"]}],\"finalize\":[]}"
	writeStepsJSON(t, dir, content)
	errs := validator.Validate(dir)
	// The NUL byte is either rejected by the JSON parser or the containerEnv validator.
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "newline or NUL") || strings.Contains(e.Error(), "malformed JSON") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'newline or NUL' or 'malformed JSON' error; got: %v", errs)
	}
	if validator.FatalErrorCount(errs) == 0 {
		t.Errorf("expected at least one fatal error for NUL in value; got: %v", errs)
	}
}

// TestValidate_ContainerEnv_EnvCollisionEmitsInfo verifies that a containerEnv
// key that also appears in the env allowlist emits an INFO notice, not a fatal error.
func TestValidate_ContainerEnv_EnvCollisionEmitsInfo(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"env": ["MY_TOKEN"],
		"containerEnv": {"MY_TOKEN": "forced-value"},
		"initialize": [],
		"iteration": [{"name":"S","isClaude":false,"command":["echo","ok"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	// Must not produce a fatal error.
	if validator.FatalErrorCount(errs) > 0 {
		t.Errorf("env+containerEnv collision must not produce a fatal error; got %d fatal error(s): %v", validator.FatalErrorCount(errs), errs)
	}
	// Must produce an info notice.
	found := false
	for _, e := range errs {
		if e.Severity == validator.SeverityInfo && strings.Contains(e.Error(), "MY_TOKEN") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an INFO notice for env+containerEnv collision on MY_TOKEN; got: %v", errs)
	}
}

// TestValidate_ContainerEnv_SecretLookingNameEmitsWarning verifies that a
// containerEnv key ending in _TOKEN, _KEY, or _SECRET emits a warning (non-fatal).
func TestValidate_ContainerEnv_SecretLookingNameEmitsWarning(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"containerEnv": {"GITHUB_TOKEN": "ghp_literal", "DB_KEY": "abc", "SIGNING_SECRET": "xyz"},
		"initialize": [],
		"iteration": [{"name":"S","isClaude":false,"command":["echo","ok"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	// Must not produce any fatal errors.
	if validator.FatalErrorCount(errs) > 0 {
		t.Errorf("secret-looking names must not produce fatal errors; got: %v", errs)
	}
	// Must produce exactly 3 warnings (one per secret-looking key).
	warnCount := 0
	for _, e := range errs {
		if e.Severity == validator.SeverityWarning {
			warnCount++
		}
	}
	if warnCount != 3 {
		t.Errorf("expected 3 warnings for secret-looking keys, got %d: %v", warnCount, errs)
	}
}

// TestValidate_ContainerEnv_UnknownFieldRejected verifies that an unknown top-level
// field adjacent to containerEnv is rejected by the strict decoder.
func TestValidate_ContainerEnv_UnknownFieldRejected(t *testing.T) {
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"containerEnv": {"GOPATH": "/tmp"},
		"unknownField": "bad",
		"initialize": [],
		"iteration": [{"name":"S","isClaude":false,"command":["echo","ok"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "malformed JSON")
}

// --- TP-006: Error.IsFatal ---

func TestError_IsFatal(t *testing.T) {
	cases := []struct {
		severity string
		want     bool
	}{
		{"", true},
		{validator.SeverityError, true},
		{validator.SeverityWarning, false},
		{validator.SeverityInfo, false},
	}
	for _, tc := range cases {
		e := validator.Error{Severity: tc.severity, Category: "test", Phase: "config", Problem: "p"}
		got := e.IsFatal()
		if got != tc.want {
			t.Errorf("severity=%q: IsFatal()=%v, want %v", tc.severity, got, tc.want)
		}
	}
}

// --- TP-007: FatalErrorCount ---

func TestFatalErrorCount(t *testing.T) {
	t.Run("mixed severities counts only fatal", func(t *testing.T) {
		errs := []validator.Error{
			{Severity: "", Category: "test", Phase: "p", Problem: "a"},
			{Severity: validator.SeverityError, Category: "test", Phase: "p", Problem: "b"},
			{Severity: validator.SeverityWarning, Category: "test", Phase: "p", Problem: "c"},
			{Severity: validator.SeverityInfo, Category: "test", Phase: "p", Problem: "d"},
		}
		got := validator.FatalErrorCount(errs)
		if got != 2 {
			t.Errorf("FatalErrorCount = %d, want 2", got)
		}
	})
	t.Run("nil slice returns 0", func(t *testing.T) {
		got := validator.FatalErrorCount(nil)
		if got != 0 {
			t.Errorf("FatalErrorCount(nil) = %d, want 0", got)
		}
	})
}

// --- TP-008: Error.Error() prefix and contents (file-level and step-level, all severities) ---

func TestError_ErrorString(t *testing.T) {
	cases := []struct {
		name         string
		e            validator.Error
		wantPrefix   string
		wantContains []string
	}{
		{
			name:         "error file-level",
			e:            validator.Error{Severity: validator.SeverityError, Category: "file", Phase: "config", Problem: "missing"},
			wantPrefix:   "config error:",
			wantContains: []string{"file", "config", "missing"},
		},
		{
			name:         "error step-level",
			e:            validator.Error{Severity: validator.SeverityError, Category: "schema", Phase: "iteration", StepName: "My Step", Problem: "bad field"},
			wantPrefix:   "config error:",
			wantContains: []string{"schema", "iteration", "My Step", "bad field"},
		},
		{
			name:         "warning file-level",
			e:            validator.Error{Severity: validator.SeverityWarning, Category: "containerEnv", Phase: "config", Problem: "looks like a secret"},
			wantPrefix:   "config warning:",
			wantContains: []string{"containerEnv", "config", "looks like a secret"},
		},
		{
			name:         "warning step-level",
			e:            validator.Error{Severity: validator.SeverityWarning, Category: "containerEnv", Phase: "config", StepName: "Feature work", Problem: "something"},
			wantPrefix:   "config warning:",
			wantContains: []string{"containerEnv", "config", "Feature work", "something"},
		},
		{
			name:         "info file-level",
			e:            validator.Error{Severity: validator.SeverityInfo, Category: "containerEnv", Phase: "config", Problem: "also in allowlist"},
			wantPrefix:   "config info:",
			wantContains: []string{"containerEnv", "config", "also in allowlist"},
		},
		{
			name:         "info step-level",
			e:            validator.Error{Severity: validator.SeverityInfo, Category: "env", Phase: "iteration", StepName: "Deploy", Problem: "notice"},
			wantPrefix:   "config info:",
			wantContains: []string{"env", "iteration", "Deploy", "notice"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.e.Error()
			if !strings.HasPrefix(got, tc.wantPrefix) {
				t.Errorf("Error() = %q, want prefix %q", got, tc.wantPrefix)
			}
			for _, sub := range tc.wantContains {
				if !strings.Contains(got, sub) {
					t.Errorf("Error() = %q, want contains %q", got, sub)
				}
			}
		})
	}
}

// --- captureMode validation tests ---

// TestValidate_CaptureMode_InvalidValue verifies that an unrecognized captureMode
// value (anything other than "", "lastLine", "fullStdout") is rejected.
func TestValidate_CaptureMode_InvalidValue(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "get-thing")
	writeStepsJSON(t, dir, `{
		"initialize":[],
		"iteration":[{"name":"Fetch","isClaude":false,"command":["scripts/get-thing"],"captureAs":"THING","captureMode":"bogus"}],
		"finalize":[]
	}`)

	errs := validator.Validate(dir)
	if !hasError(errs, "captureMode") {
		t.Errorf("expected captureMode rejection error, got: %v", errs)
	}
	if !hasError(errs, "bogus") {
		t.Errorf("expected error to mention the invalid value %q, got: %v", "bogus", errs)
	}
}

// TestValidate_CaptureMode_OnClaudeStep verifies that setting captureMode on a
// claude step is rejected (they route through the claudestream aggregator).
func TestValidate_CaptureMode_OnClaudeStep(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "work.md", "do the thing")
	writeStepsJSON(t, dir, `{
		"initialize":[],
		"iteration":[{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"work.md","captureMode":"fullStdout"}],
		"finalize":[]
	}`)

	errs := validator.Validate(dir)
	if !hasError(errs, "captureMode") {
		t.Errorf("expected captureMode-on-claude error, got: %v", errs)
	}
}

// TP-004: TestValidate_CaptureMode_InvalidValue_StepNameAttribution verifies
// that a captureMode validation error carries correct StepName, Category,
// Phase, and IsFatal attributes, and that Error() includes the quoted step name.
func TestValidate_CaptureMode_InvalidValue_StepNameAttribution(t *testing.T) {
	dir := tempProject(t)
	writeScript(t, dir, "get-thing")
	writeStepsJSON(t, dir, `{
		"initialize":[],
		"iteration":[{"name":"Fetch","isClaude":false,"command":["scripts/get-thing"],"captureAs":"THING","captureMode":"bogus"}],
		"finalize":[]
	}`)

	errs := validator.Validate(dir)

	var captureErr *validator.Error
	for i := range errs {
		if errs[i].Category == "schema" && strings.Contains(errs[i].Problem, "captureMode") {
			captureErr = &errs[i]
			break
		}
	}
	if captureErr == nil {
		t.Fatalf("expected a schema captureMode error, got: %v", errs)
	}
	if captureErr.StepName != "Fetch" {
		t.Errorf("StepName = %q, want %q", captureErr.StepName, "Fetch")
	}
	if captureErr.Category != "schema" {
		t.Errorf("Category = %q, want %q", captureErr.Category, "schema")
	}
	if captureErr.Phase != "iteration" {
		t.Errorf("Phase = %q, want %q", captureErr.Phase, "iteration")
	}
	if !captureErr.IsFatal() {
		t.Errorf("IsFatal() = false, want true")
	}
	got := captureErr.Error()
	if !strings.Contains(got, `"Fetch"`) {
		t.Errorf("Error() = %q, want quoted step name \"Fetch\"", got)
	}
}

// TP-005: TestValidate_CaptureMode_InvalidOnClaudeStep_CollectsBoth verifies
// that both the "invalid value" and the "must not be set on claude steps"
// errors are collected rather than short-circuited.
func TestValidate_CaptureMode_InvalidOnClaudeStep_CollectsBoth(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "x.md", "do the thing")
	writeStepsJSON(t, dir, `{
		"initialize":[],
		"iteration":[{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"x.md","captureMode":"garbage"}],
		"finalize":[]
	}`)

	errs := validator.Validate(dir)

	if !hasError(errs, "not valid") {
		t.Errorf("expected 'not valid' captureMode error; got: %v", errs)
	}
	if !hasError(errs, "must not be set on claude") {
		t.Errorf("expected 'must not be set on claude' captureMode error; got: %v", errs)
	}
}

// --- TP-012: step-level Error() includes quoted step name ---

func TestError_StepLevel_QuotedStepName(t *testing.T) {
	e := validator.Error{
		Severity: validator.SeverityWarning,
		Category: "containerEnv",
		Phase:    "config",
		StepName: "Feature work",
		Problem:  "literal value committed to repo",
	}
	got := e.Error()
	if !strings.HasPrefix(got, "config warning:") {
		t.Errorf("Error() = %q, want prefix %q", got, "config warning:")
	}
	if !strings.Contains(got, `"Feature work"`) {
		t.Errorf("Error() = %q, want quoted step name \"Feature work\"", got)
	}
}
