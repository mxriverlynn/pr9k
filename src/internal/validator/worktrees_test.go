package validator_test

import (
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/validator"
)

// TestValidate_Worktrees_HappyPath_WithBlock validates that a config with
// the worktrees block (enabled:true, autoCleanup:true) is accepted.
func TestValidate_Worktrees_HappyPath_WithBlock(t *testing.T) {
	t.Parallel()
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"s1","isClaude":false,"command":["echo","ok"]}],
		"finalize": [],
		"worktrees": {"enabled": true, "autoCleanup": true}
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_Worktrees_HappyPath_BothFalse validates that worktrees block
// with both fields false is accepted (the default-off state).
func TestValidate_Worktrees_HappyPath_BothFalse(t *testing.T) {
	t.Parallel()
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"s1","isClaude":false,"command":["echo","ok"]}],
		"finalize": [],
		"worktrees": {"enabled": false, "autoCleanup": false}
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_Worktrees_HappyPath_NoBlock validates that a config without
// the worktrees block continues to validate (back-compat).
func TestValidate_Worktrees_HappyPath_NoBlock(t *testing.T) {
	t.Parallel()
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"s1","isClaude":false,"command":["echo","ok"]}],
		"finalize": []
	}`)
	errs := validator.Validate(dir)
	requireNoErrors(t, errs)
}

// TestValidate_Worktrees_AutoCleanupWithoutEnabled rejects autoCleanup:true
// when enabled is absent or false — fatal error required.
func TestValidate_Worktrees_AutoCleanupWithoutEnabled(t *testing.T) {
	t.Parallel()
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"s1","isClaude":false,"command":["echo","ok"]}],
		"finalize": [],
		"worktrees": {"autoCleanup": true}
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "autoCleanup")
	fatal := 0
	for _, e := range errs {
		if e.IsFatal() {
			fatal++
		}
	}
	if fatal == 0 {
		t.Error("expected at least one fatal error for autoCleanup without enabled")
	}
}

// TestValidate_Worktrees_AutoCleanupEnabledFalse rejects autoCleanup:true
// when enabled is explicitly false.
func TestValidate_Worktrees_AutoCleanupEnabledFalse(t *testing.T) {
	t.Parallel()
	dir := tempProject(t)
	writeStepsJSON(t, dir, `{
		"initialize": [],
		"iteration": [{"name":"s1","isClaude":false,"command":["echo","ok"]}],
		"finalize": [],
		"worktrees": {"enabled": false, "autoCleanup": true}
	}`)
	errs := validator.Validate(dir)
	requireError(t, errs, "autoCleanup")
}
