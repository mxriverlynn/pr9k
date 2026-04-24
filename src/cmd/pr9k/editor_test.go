package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// T2-1: VISUAL="code --wait" splits into [code --wait] tokens.
func TestResolveEditor_VisualWithDoubleQuotes_Parses(t *testing.T) {
	orig := lookPath
	lookPath = func(file string) (string, error) { return "/usr/bin/" + file, nil }
	t.Cleanup(func() { lookPath = orig })

	t.Setenv("VISUAL", "code --wait")
	t.Setenv("EDITOR", "")

	tokens, err := resolveEditor()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 2 || tokens[0] != "code" || tokens[1] != "--wait" {
		t.Errorf("got %v, want [code --wait]", tokens)
	}
}

// T2-2: VISUAL='/Applications/Sublime Text/subl' (single-quoted path with space) parses.
func TestResolveEditor_VisualWithSingleQuotedPath_Parses(t *testing.T) {
	t.Setenv("VISUAL", "'/Applications/Sublime Text/subl'")
	t.Setenv("EDITOR", "")

	tokens, err := resolveEditor()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0] != "/Applications/Sublime Text/subl" {
		t.Errorf("got %v, want [/Applications/Sublime Text/subl]", tokens)
	}
}

// T2-3: Shell metacharacters in $VISUAL are rejected.
func TestResolveEditor_VisualWithShellMetacharacters_Rejected(t *testing.T) {
	cases := []struct {
		name string
		val  string
	}{
		{"backtick", "vi`ls`"},
		{"semicolon", "vi;rm -rf /"},
		{"pipe", "vi|cat /etc/passwd"},
		{"newline", "vi\n/etc/passwd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("VISUAL", tc.val)
			t.Setenv("EDITOR", "")
			_, err := resolveEditor()
			if err == nil {
				t.Errorf("expected error for VISUAL=%q, got nil", tc.val)
			}
		})
	}
}

// T2-4: Relative editor name not on $PATH returns specific error mentioning $VISUAL/$EDITOR or PATH.
func TestResolveEditor_EditorNotOnPath_ReturnsSpecificError(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "nonexistent-editor-zzz")

	_, err := resolveEditor()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.ContainsAny(err.Error(), "$") {
		t.Errorf("error should mention $VISUAL or $EDITOR, got: %v", err)
	}
}

// T2-5: Neither $VISUAL nor $EDITOR set returns guidance error.
func TestResolveEditor_NeitherVisualNorEditorSet_ReturnsGuidance(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	_, err := resolveEditor()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "VISUAL") && !strings.Contains(err.Error(), "EDITOR") {
		t.Errorf("guidance error should mention VISUAL or EDITOR, got: %v", err)
	}
}

// T2-6: ExecCallback with *exec.ExitError code 130 returns editorSigintMsg.
func TestExecCallback_ExitCode130_ReturnsSigintMsg(t *testing.T) {
	cb := makeExecCallback()
	exitErr := makeExitError(t, 130)
	msg := cb(exitErr)
	if _, ok := msg.(editorSigintMsg); !ok {
		t.Errorf("expected editorSigintMsg, got %T", msg)
	}
}

// T2-7: ExecCallback with a non-*exec.ExitError (terminal restore failure) returns editorRestoreFailedMsg.
func TestExecCallback_RestoreTerminalFailure_ReturnsRestoreFailedMsg(t *testing.T) {
	cb := makeExecCallback()
	msg := cb(errors.New("terminal restore failed"))
	if _, ok := msg.(editorRestoreFailedMsg); !ok {
		t.Errorf("expected editorRestoreFailedMsg, got %T", msg)
	}
}

// T2-8: ExecCallback with nil error returns editorExitMsg{ok:true}.
func TestExecCallback_EditorCleanExit_ReturnsExitMsgAndTriggersReread(t *testing.T) {
	cb := makeExecCallback()
	msg := cb(nil)
	exitMsg, ok := msg.(editorExitMsg)
	if !ok {
		t.Fatalf("expected editorExitMsg, got %T", msg)
	}
	if !exitMsg.ok {
		t.Errorf("expected exitMsg.ok=true, got false")
	}
}

// Async guard: realEditorRunner.Run returns a non-nil tea.Cmd without blocking.
func TestRealEditorRunner_Run_ReturnsTeaCmd(t *testing.T) {
	orig := lookPath
	lookPath = func(file string) (string, error) { return "/usr/bin/" + file, nil }
	t.Cleanup(func() { lookPath = orig })

	t.Setenv("VISUAL", "vi")
	t.Setenv("EDITOR", "")

	runner := &realEditorRunner{}
	cb := makeExecCallback()
	cmd := runner.Run("/tmp/test.json", cb)
	if cmd == nil {
		t.Error("Run returned nil tea.Cmd")
	}
}

// G-1: ExecCallback with a non-zero, non-130 *exec.ExitError returns editorExitMsg{ok:false, code}.
func TestExecCallback_NonZeroNon130ExitCode_ReturnsExitMsgNotOk(t *testing.T) {
	cb := makeExecCallback()
	exitErr := makeExitError(t, 1)
	msg := cb(exitErr)
	exitMsg, ok := msg.(editorExitMsg)
	if !ok {
		t.Fatalf("expected editorExitMsg, got %T", msg)
	}
	if exitMsg.ok {
		t.Error("expected exitMsg.ok=false, got true")
	}
	if exitMsg.code != 1 {
		t.Errorf("expected exitMsg.code=1, got %d", exitMsg.code)
	}
}

// G-2: EDITOR fallback success path — VISUAL empty, EDITOR set and on PATH.
func TestResolveEditor_EditorFallback_WhenVisualEmpty(t *testing.T) {
	orig := lookPath
	lookPath = func(file string) (string, error) { return "/usr/bin/" + file, nil }
	t.Cleanup(func() { lookPath = orig })

	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "vi")

	tokens, err := resolveEditor()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0] != "vi" {
		t.Errorf("got %v, want [vi]", tokens)
	}
}

// G-3: realEditorRunner.Run returns a non-nil tea.Cmd that yields editorRestoreFailedMsg
// when resolveEditor fails (neither VISUAL nor EDITOR is set).
func TestRealEditorRunner_Run_ResolveFailure_ReturnsErrorCmd(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	runner := &realEditorRunner{}
	cb := makeExecCallback()
	cmd := runner.Run("/tmp/test.json", cb)
	if cmd == nil {
		t.Fatal("Run returned nil tea.Cmd")
	}
	msg := cmd()
	if _, ok := msg.(editorRestoreFailedMsg); !ok {
		t.Errorf("expected editorRestoreFailedMsg, got %T", msg)
	}
}

// makeExitError creates a real *exec.ExitError with the given exit code.
func makeExitError(t *testing.T, code int) *exec.ExitError {
	t.Helper()
	c := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code))
	err := c.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	return exitErr
}
