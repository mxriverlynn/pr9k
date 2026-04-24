package workflowedit

import (
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

func TestLog_SessionStart_HasPathsNoEnv(t *testing.T) {
	line := fmtSessionStart("/my/workflow", "/my/project")
	if !strings.Contains(line, "/my/workflow") {
		t.Errorf("session_start missing workflowDir, got: %q", line)
	}
	if !strings.Contains(line, "/my/project") {
		t.Errorf("session_start missing projectDir, got: %q", line)
	}
	if !strings.HasPrefix(line, "session_start") {
		t.Errorf("session_start line has wrong prefix, got: %q", line)
	}
}

func TestLog_WorkflowSaved_HasPathAndDuration(t *testing.T) {
	dur := 250 * time.Millisecond
	line := fmtWorkflowSaved("/my/workflow/config.json", dur, true)
	if !strings.Contains(line, "/my/workflow/config.json") {
		t.Errorf("workflow_saved missing path, got: %q", line)
	}
	if !strings.Contains(line, "250") {
		t.Errorf("workflow_saved missing duration_ms, got: %q", line)
	}
	if !strings.Contains(line, "success") {
		t.Errorf("workflow_saved missing success outcome, got: %q", line)
	}
	lineF := fmtWorkflowSaved("/my/workflow/config.json", dur, false)
	if !strings.Contains(lineF, "failure") {
		t.Errorf("workflow_saved missing failure outcome, got: %q", lineF)
	}
}

func TestLog_SaveFailed_ReasonFromClosedEnumeration(t *testing.T) {
	validReasons := map[string]bool{
		"validator_fatals":        true,
		"permission_error":        true,
		"disk_full":               true,
		"cross_device":            true,
		"conflict_detected":       true,
		"symlink_escape":          true,
		"target_not_regular_file": true,
		"parse_error":             true,
		"other":                   true,
	}
	cases := []workflowio.SaveErrorKind{
		workflowio.SaveErrorValidatorFatals,
		workflowio.SaveErrorPermission,
		workflowio.SaveErrorDiskFull,
		workflowio.SaveErrorEXDEV,
		workflowio.SaveErrorConflictDetected,
		workflowio.SaveErrorSymlinkEscape,
		workflowio.SaveErrorTargetNotRegularFile,
		workflowio.SaveErrorParse,
		workflowio.SaveErrorOther,
	}
	for _, kind := range cases {
		line := fmtSaveFailed(kind)
		if !strings.Contains(line, "reason=") {
			t.Errorf("save_failed line missing reason= for kind %d: %q", kind, line)
			continue
		}
		parts := strings.SplitN(line, "reason=", 2)
		if len(parts) < 2 || parts[1] == "" {
			t.Errorf("save_failed line malformed for kind %d: %q", kind, line)
			continue
		}
		reason := strings.Fields(parts[1])[0]
		if !validReasons[reason] {
			t.Errorf("save_failed reason %q for kind %d not in closed enumeration", reason, kind)
		}
	}
}

func TestLog_EditorOpened_FirstTokenOnly(t *testing.T) {
	// VISUAL="/opt/Sublime Text/subl --wait" → logged as "subl" only.
	line := fmtEditorOpened(`/opt/Sublime Text/subl --wait`, 0, time.Second)
	if strings.Contains(line, "--wait") {
		t.Errorf("editor_opened must not log --wait arg, got: %q", line)
	}
	if strings.Contains(line, "Sublime Text") {
		t.Errorf("editor_opened must not log full path with spaces, got: %q", line)
	}
	if !strings.Contains(line, "subl") {
		t.Errorf("editor_opened must log editor binary name, got: %q", line)
	}
}

// allSessionFormatLines collects every log line format function can emit.
// The sentinel value is NOT passed to any format function — it must never appear.
func allSessionFormatLines() []string {
	return []string{
		fmtSessionStart("/workflow/dir", "/project/dir"),
		fmtWorkflowSaved("/workflow/config.json", 100*time.Millisecond, true),
		fmtWorkflowSaved("/workflow/config.json", 100*time.Millisecond, false),
		fmtSaveFailed(workflowio.SaveErrorValidatorFatals),
		fmtSaveFailed(workflowio.SaveErrorPermission),
		fmtSaveFailed(workflowio.SaveErrorDiskFull),
		fmtSaveFailed(workflowio.SaveErrorEXDEV),
		fmtSaveFailed(workflowio.SaveErrorConflictDetected),
		fmtSaveFailed(workflowio.SaveErrorSymlinkEscape),
		fmtSaveFailed(workflowio.SaveErrorTargetNotRegularFile),
		fmtSaveFailed(workflowio.SaveErrorParse),
		fmtSaveFailed(workflowio.SaveErrorOther),
		fmtEditorOpened("vim", 0, 50*time.Millisecond),
		fmtEditorSigint(),
		fmtQuitClean(),
		fmtQuitDiscarded(),
		fmtQuitCancelled(),
		fmtSharedInstallDetected(),
	}
}

func TestLog_NoContainerEnvValuesEver(t *testing.T) {
	// containerEnv entries have IsLiteral=true and a Value field.
	containerEnvValue := "CONTAINER_SECRET_SENTINEL_0xDEADBEEF"
	_ = workflowmodel.EnvEntry{Key: "MY_KEY", Value: containerEnvValue, IsLiteral: true}

	// No format function accepts containerEnv values — none can emit them.
	for _, line := range allSessionFormatLines() {
		if strings.Contains(line, containerEnvValue) {
			t.Errorf("log line leaked containerEnv value: %q", line)
		}
	}
}

func TestLog_NoEnvEntryValuesEver(t *testing.T) {
	// env passthrough entries: IsLiteral=false, Value empty; Key is the var name.
	envKeyName := "MY_PASSTHROUGH_ENV_KEY_SENTINEL_54321"
	_ = workflowmodel.EnvEntry{Key: envKeyName, IsLiteral: false}

	for _, line := range allSessionFormatLines() {
		if strings.Contains(line, envKeyName) {
			t.Errorf("log line leaked env key name: %q", line)
		}
	}
}

func TestLog_NoPromptFileContentsEver(t *testing.T) {
	promptContent := "MY_SECRET_PROMPT_CONTENT_SENTINEL_ABCD1234"
	// Prompt file content is read from disk; it must never appear in log lines.
	for _, line := range allSessionFormatLines() {
		if strings.Contains(line, promptContent) {
			t.Errorf("log line leaked prompt file content: %q", line)
		}
	}
}
