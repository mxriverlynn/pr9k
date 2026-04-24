package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/shlex"

	"github.com/mxriverlynn/pr9k/src/internal/workflowedit"
)

// lookPath is exec.LookPath, overridable in tests.
var lookPath = exec.LookPath

// Message types produced by the ExecCallback three-way type switch (F-107).

type editorExitMsg struct {
	ok   bool
	code int
}

type editorSigintMsg struct{}

type editorRestoreFailedMsg struct {
	err error
}

// realEditorRunner is the production EditorRunner implementation (D-6, D-36).
type realEditorRunner struct{}

func (r *realEditorRunner) Run(filePath string, cb workflowedit.ExecCallback) tea.Cmd {
	tokens, err := resolveEditor()
	if err != nil {
		return func() tea.Msg {
			return editorRestoreFailedMsg{err: err}
		}
	}
	args := append(tokens[1:], filePath)
	editorCmd := exec.Command(tokens[0], args...)
	return tea.ExecProcess(editorCmd, func(err error) tea.Msg {
		return cb(err)
	})
}

// resolveEditor resolves $VISUAL then $EDITOR, splits with shlex, rejects shell
// metacharacters (D-22, D-33), and verifies that relative paths exist on $PATH.
func resolveEditor() ([]string, error) {
	for _, envVar := range []string{"VISUAL", "EDITOR"} {
		val := os.Getenv(envVar)
		if val == "" {
			continue
		}
		if err := rejectShellMeta(val); err != nil {
			return nil, err
		}
		tokens, err := shlex.Split(val)
		if err != nil {
			return nil, fmt.Errorf("resolveEditor: cannot parse $%s: %w", envVar, err)
		}
		if len(tokens) == 0 {
			continue
		}
		bin := tokens[0]
		if !filepath.IsAbs(bin) {
			if _, err := lookPath(bin); err != nil {
				return nil, fmt.Errorf("resolveEditor: %q not found on $PATH — set $VISUAL or $EDITOR to a reachable editor", bin)
			}
		}
		return tokens, nil
	}
	return nil, errors.New("resolveEditor: neither $VISUAL nor $EDITOR is set — export VISUAL=<editor> to configure your editor")
}

// rejectShellMeta rejects shell metacharacters from the raw env var value (D-33).
// The value is split by shlex and passed to exec.Command (never a shell), so
// most metacharacters are inert. We still block the common injection vectors
// as defence-in-depth.
func rejectShellMeta(val string) error {
	for _, meta := range []string{"`", ";", "|", "$", "\n"} {
		if strings.Contains(val, meta) {
			return fmt.Errorf("resolveEditor: value contains shell metacharacter %q — set a safe editor path", meta)
		}
	}
	return nil
}

// makeExecCallback returns the three-way ExecCallback switch (F-107):
//   - nil        → editorExitMsg{ok: true}
//   - ExitError 130 → editorSigintMsg (D-7: routes to quit-confirm, not re-read)
//   - ExitError other → editorExitMsg{ok: false, code}
//   - other error    → editorRestoreFailedMsg (terminal may be degraded, no re-read)
func makeExecCallback() workflowedit.ExecCallback {
	return func(err error) tea.Msg {
		var exitErr *exec.ExitError
		switch {
		case err == nil:
			return editorExitMsg{ok: true}
		case errors.As(err, &exitErr) && exitErr.ExitCode() == 130:
			return editorSigintMsg{}
		case errors.As(err, &exitErr):
			return editorExitMsg{ok: false, code: exitErr.ExitCode()}
		default:
			return editorRestoreFailedMsg{err: err}
		}
	}
}
