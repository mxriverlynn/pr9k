package cmuxctl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

// ErrWorkspaceExists is returned (or wrapped) when a workspace name is already
// in use. RunPhase1 uses it as the retry trigger for spec D12 collision retry.
var ErrWorkspaceExists = errors.New("cmuxctl: workspace already exists")

// SanitizeBasename applies the D11 sanitization rules to a directory basename:
//   - Accepted characters: [a-zA-Z0-9._-]; everything else becomes "-".
//   - Consecutive hyphens are collapsed to one.
//   - Leading and trailing hyphens are trimmed.
//   - Empty result after trimming falls back to the literal string "repo".
//
// The caller must never print the input to operator-visible output (D-23).
func SanitizeBasename(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	result := b.String()

	// Collapse consecutive hyphens.
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	result = strings.Trim(result, "-")

	if result == "" {
		return "repo"
	}
	return result
}

// composeWorkspaceName composes the workspace name from an already-sanitized
// basename and a fresh UTC nanosecond timestamp per parent spec D29.
// The result is always of the form pr9k-<sanitized>-<timestamp>.
func composeWorkspaceName(sanitized string) string {
	ts := time.Now().UTC().Format("20060102T150405.000000000Z")
	return "pr9k-" + sanitized + "-" + ts
}

// RunPhase1 implements the workspace lifecycle setup half of cmux Phase 1:
//
//  1. Capture the current workspace via WorkspaceCurrent (spec D10; empty result
//     is recorded as "no prior workspace" without error).
//  2. Sanitize filepath.Base(projectDir) per D11 (SanitizeBasename), compose the
//     workspace name pr9k-<sanitized>-<timestamp>.
//  3. WorkspaceCreate; on duplicate-name error retry once with a new timestamp
//     (spec D12). Second collision returns an error.
//  4. Print the workspace-name confirmation to out per spec D2.
//  5. Spawn four panes orchestrator-first (D-3):
//     a. SurfaceSplit → orchPaneID; SurfaceSpawn orchestrator (sh -c 'tail -f /dev/null').
//     b. SurfaceHide orchestrator.
//     c. SurfaceSplit + SurfaceSpawn for header, log, footer (shell one-liners per D-4).
//  6. Start the dismissal-observation goroutine (D6, D9, D22) and block until
//     either a dismissal event is received or ctx is cancelled.
//
// dismissalCfg controls the polling cadence and per-call timeout; zero values
// use package defaults (500ms interval, 5s timeout).
//
// Any error from a cmux RPC after workspace creation is returned directly;
// partial-setup teardown (#224) is wired in a later issue.
//
// out receives only the sanitized workspace name (D-23: the pre-sanitized
// filepath.Base(projectDir) must never appear in operator-visible output).
func RunPhase1(ctx context.Context, client CmuxClient, projectDir string, out io.Writer, dismissalCfg DismissalConfig) error {
	// Step 1: capture current workspace (spec D10).
	priorWorkspace, err := client.WorkspaceCurrent(ctx)
	if err != nil {
		priorWorkspace = "" // record as "no prior workspace" on error
	}
	_ = priorWorkspace // consumed by teardown in #224

	// Step 2: compose workspace name.
	// D-23: SanitizeBasename consumes filepath.Base(projectDir); the result
	// (sanitized) is the only form that may appear in operator output.
	sanitized := SanitizeBasename(filepath.Base(projectDir))
	workspaceName := composeWorkspaceName(sanitized)

	// Step 3: create workspace with one collision retry (spec D12).
	if err := client.WorkspaceCreate(ctx, workspaceName); err != nil {
		if !errors.Is(err, ErrWorkspaceExists) {
			return fmt.Errorf("cmuxctl: workspace create: %w", err)
		}
		// Retry with a new timestamp.
		workspaceName = composeWorkspaceName(sanitized)
		if err := client.WorkspaceCreate(ctx, workspaceName); err != nil {
			return fmt.Errorf("cmuxctl: workspace create (retry): %w", err)
		}
	}

	// Step 4: print workspace-name confirmation per spec D2.
	// D-23: only the composed workspaceName (which contains only the sanitized form)
	// is printed; the pre-sanitized basename is never referenced after this point.
	_, _ = fmt.Fprintf(out, "pr9k workspace: %s\n", workspaceName)

	// Step 5a: spawn orchestrator pane (D-3, D-4).
	orchPaneID, err := client.SurfaceSplit(ctx, SplitOpts{})
	if err != nil {
		return fmt.Errorf("cmuxctl: split orchestrator pane: %w", err)
	}
	if err := client.SurfaceSpawn(ctx, orchPaneID, []string{"sh", "-c", "tail -f /dev/null"}); err != nil {
		return fmt.Errorf("cmuxctl: spawn orchestrator: %w", err)
	}

	// Step 5b: hide orchestrator pane.
	if err := client.SurfaceHide(ctx, orchPaneID); err != nil {
		return fmt.Errorf("cmuxctl: hide orchestrator pane: %w", err)
	}

	// Step 5c-5e: spawn header, log, footer panes (D-3, D-4).
	// Each uses POSIX-portable tail -f /dev/null (not GNU sleep infinity).
	visiblePanes := []struct{ role, label string }{
		{"header", "header \xe2\x80\x94 Phase 1 placeholder"},
		{"log", "log \xe2\x80\x94 Phase 1 placeholder"},
		{"footer", "footer \xe2\x80\x94 Phase 1 placeholder"},
	}
	for _, p := range visiblePanes {
		paneID, err := client.SurfaceSplit(ctx, SplitOpts{})
		if err != nil {
			return fmt.Errorf("cmuxctl: split %s pane: %w", p.role, err)
		}
		oneliner := `printf "` + p.label + `\n" && tail -f /dev/null`
		if err := client.SurfaceSpawn(ctx, paneID, []string{"sh", "-c", oneliner}); err != nil {
			return fmt.Errorf("cmuxctl: spawn %s pane: %w", p.role, err)
		}
	}

	// Step 6: start dismissal-observation goroutine and block until dismissal or
	// context cancellation (spec D9, D22). Teardown (#224) is wired later.
	obs := StartDismissalObserver(ctx, client, workspaceName, dismissalCfg)
	defer obs.Wait()
	defer obs.Cancel()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case evt := <-obs.Ch:
		if evt.Fatal {
			return fmt.Errorf("cmuxctl: dismissal poll fatal: %d consecutive timeouts; workspace %s may need manual cleanup", maxConsecutiveTimeouts, workspaceName)
		}
		return nil
	}
}
