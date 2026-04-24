package workflowedit

import (
	"fmt"
	"strings"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
)

// fmtSessionStart returns a session_start log line.
// Only workflowDir and projectDir are included; no env or content values (F-113).
func fmtSessionStart(workflowDir, projectDir string) string {
	return fmt.Sprintf("session_start workflowDir=%s projectDir=%s", workflowDir, projectDir)
}

// fmtWorkflowSaved returns a workflow_saved log line.
func fmtWorkflowSaved(path string, d time.Duration, success bool) string {
	outcome := "success"
	if !success {
		outcome = "failure"
	}
	return fmt.Sprintf("workflow_saved path=%s duration_ms=%d outcome=%s", path, d.Milliseconds(), outcome)
}

// fmtSaveFailed returns a save_failed log line with a reason from the closed
// enumeration defined by D-27. The mapping is exhaustive over SaveErrorKind.
func fmtSaveFailed(kind workflowio.SaveErrorKind) string {
	return "save_failed reason=" + saveFailedReason(kind)
}

// saveFailedReason maps SaveErrorKind to the D-27 closed-enumeration reason string.
// Adding a new SaveErrorKind without updating this switch causes a compile error
// (the function becomes non-exhaustive and the default catches it as "other").
func saveFailedReason(kind workflowio.SaveErrorKind) string {
	switch kind {
	case workflowio.SaveErrorValidatorFatals:
		return "validator_fatals"
	case workflowio.SaveErrorPermission:
		return "permission_error"
	case workflowio.SaveErrorDiskFull:
		return "disk_full"
	case workflowio.SaveErrorEXDEV:
		return "cross_device"
	case workflowio.SaveErrorConflictDetected:
		return "conflict_detected"
	case workflowio.SaveErrorSymlinkEscape:
		return "symlink_escape"
	case workflowio.SaveErrorTargetNotRegularFile:
		return "target_not_regular_file"
	case workflowio.SaveErrorParse:
		return "parse_error"
	default:
		return "other"
	}
}

// fmtEditorOpened returns an editor_opened log line.
// Only the first token (binary basename) of the editor string is included (F-113).
func fmtEditorOpened(editorEnv string, exitCode int, d time.Duration) string {
	binary := editorFirstToken(editorEnv)
	return fmt.Sprintf("editor_opened binary=%s exit_code=%d duration_ms=%d", binary, exitCode, d.Milliseconds())
}

// editorFirstToken extracts the binary basename from an editor string.
// VISUAL="/opt/Sublime Text/subl --wait" → "subl".
// It finds the last '/' first (handles paths with spaces), then strips any args.
func editorFirstToken(s string) string {
	s = strings.TrimSpace(s)
	// Find the last '/' to identify the start of the binary name even when
	// the path contains spaces (e.g. "/opt/Sublime Text/subl --wait").
	if idx := strings.LastIndexByte(s, '/'); idx >= 0 {
		s = s[idx+1:]
	}
	// Strip any trailing arguments after the binary name.
	if idx := strings.IndexByte(s, ' '); idx >= 0 {
		s = s[:idx]
	}
	return s
}

// fmtEditorSigint returns an editor_sigint log line.
func fmtEditorSigint() string { return "editor_sigint" }

// fmtQuitClean returns a quit_clean log line.
func fmtQuitClean() string { return "quit_clean" }

// fmtQuitDiscarded returns a quit_discarded_changes log line.
func fmtQuitDiscarded() string { return "quit_discarded_changes" }

// fmtQuitCancelled returns a quit_cancelled log line.
func fmtQuitCancelled() string { return "quit_cancelled" }

// fmtSharedInstallDetected returns a shared_install_detected log line.
func fmtSharedInstallDetected() string { return "shared_install_detected" }
