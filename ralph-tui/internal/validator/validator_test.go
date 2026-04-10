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
	writePrompt(t, dir, "setup.md", "project dir is {{PROJECT_DIR}}\n")
	writePrompt(t, dir, "feature.md", "implement issue {{ISSUE_ID}} in {{PROJECT_DIR}}\n")
	writePrompt(t, dir, "finalize.md", "finalize for project {{PROJECT_DIR}}\n")

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
	reserved := []string{"PROJECT_DIR", "MAX_ITER", "ITER", "STEP_NUM", "STEP_COUNT", "STEP_NAME"}
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
// available without being declared.
func TestValidate_BuiltinVarsInScope(t *testing.T) {
	dir := tempProject(t)
	writePrompt(t, dir, "p.md",
		"dir={{PROJECT_DIR}} max={{MAX_ITER}} num={{STEP_NUM}} count={{STEP_COUNT}} name={{STEP_NAME}}\n")
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"Work","isClaude":true,"model":"sonnet","promptFile":"p.md"}],
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
