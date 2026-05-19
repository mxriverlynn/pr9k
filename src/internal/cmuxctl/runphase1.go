package cmuxctl

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// resolveExecutable returns the path to the current binary, resolved through
// any symlinks. Used to construct the per-pane command (Assumption A5).
func resolveExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// SanitizeBasename applies the D11 sanitization rules to a directory basename:
//   - Accepted characters: [a-zA-Z0-9._-]; everything else becomes "-".
//   - Consecutive hyphens are collapsed to one.
//   - Leading and trailing hyphens are trimmed.
//   - Empty result after trimming falls back to the literal string "repo".
//
// The caller must never print the raw input to operator-visible output (D-23).
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
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	result = strings.Trim(result, "-")
	if result == "" {
		return "repo"
	}
	return result
}

// composeWorkspaceLabel composes the human-facing workspace title from an
// already-sanitized basename and a fresh UTC nanosecond timestamp (parent spec
// D29). In cmux v2 a workspace is identified by an opaque handle, not this
// string; the label is only a display title and a unique socket-file component.
func composeWorkspaceLabel(sanitized string) string {
	ts := time.Now().UTC().Format("20060102T150405.000000000Z")
	return "pr9k-" + sanitized + "-" + ts
}

// shellQuote single-quote-escapes s for safe inclusion in a /bin/sh command
// line (cmux runs a surface's initial_command through the user's shell).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// paneCommand builds the initial_command for a display pane. cmux v2
// surface.split has no initial_env param, so the pane environment is embedded
// in the command itself; workspace.create's first surface gets the same form
// for uniformity. `exec` replaces the shell so the pane process is the
// cmux-pane subprocess directly.
func paneCommand(exe, role string, env map[string]string) string {
	var b strings.Builder
	// Deterministic order keeps the command stable and testable.
	for _, k := range []string{"PR9K_CMUX_SOCKET", "PR9K_PROJECT_DIR"} {
		if v, ok := env[k]; ok {
			b.WriteString(k)
			b.WriteByte('=')
			b.WriteString(shellQuote(v))
			b.WriteByte(' ')
		}
	}
	b.WriteString("exec ")
	b.WriteString(shellQuote(exe))
	b.WriteString(" cmux-pane --role=")
	b.WriteString(role)
	return b.String()
}

// expectedDisplaySurfaces is the number of display panes RunPhase1 creates
// (log + header + footer). The dismissal observer fires if the live surface
// count drops below this.
const expectedDisplaySurfaces = 3

// Phase1Hooks lets the cmd layer inject the orchestrator behaviour without
// cmuxctl importing the interaction-channel/workflow packages (layering).
//
// Under Architecture A (decision-log D-R1) the in-pane pr9k process IS the
// orchestrator: it must Serve the interaction-channel socket the display panes
// will Dial, and it must run the workflow. Both responsibilities are supplied
// here so RunPhase1 can sequence them correctly relative to workspace/pane
// creation (Serve must happen before the panes are created).
type Phase1Hooks struct {
	// BeforePanes runs after the pane socket path is computed but BEFORE the
	// workspace and panes are created, so the orchestrator can Serve the
	// socket the panes will Dial. socketPath is the value passed to every pane
	// via PR9K_CMUX_SOCKET. A non-nil error aborts the run before any cmux
	// workspace is created.
	BeforePanes func(socketPath string) error

	// Run executes the orchestrator workflow after the panes are created. When
	// non-nil, RunPhase1 runs it with a context that is cancelled if the
	// operator dismisses the workspace; after Run returns of its own accord
	// the workspace stays open showing the final state until the operator
	// dismisses it (or a signal cancels ctx). When nil, RunPhase1 just blocks
	// on the dismissal observer (workspace-lifecycle-only mode, used by tests).
	Run func(ctx context.Context, ws Workspace) error
}

// RunPhase1 implements the cmux-mode workspace lifecycle against the cmux v2
// API (rework R, architecture A).
//
// Architecture A: the pr9k process the operator launched inside a cmux pane IS
// the orchestrator. There is no separate/hidden orchestrator pane (cmux v2 has
// no surface.hide). RunPhase1 creates a workspace whose first surface is the
// "log" pane, then splits in "header" and "footer". Each surface runs
// `pr9k cmux-pane --role=<role>` and talks back to this process over the
// interaction-channel Unix socket (unchanged).
//
// Setup:
//  1. Capture the current workspace handle (best-effort focus restore).
//  2. workspace.create (title pr9k-<sanitized>-<ts>, working_directory
//     projectDir, initial_command = log pane) → workspace handle.
//  3. surface.split up → header pane; surface.split down → footer pane.
//  4. Start the dismissal observer (workspace gone, or surface count < 3) and
//     block until dismissal or ctx cancellation.
//
// Teardown (sync.Once; runs once from any path): close the pr9k workspace,
// best-effort re-select the prior workspace, join the observer.
func RunPhase1(ctx context.Context, client CmuxClient, projectDir string, out io.Writer, dismissalCfg DismissalConfig, hooks Phase1Hooks) (returnErr error) {
	stderr := dismissalCfg.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	// Step 1: capture current workspace for focus restore (best-effort).
	prior, err := client.WorkspaceCurrent(ctx)
	if err != nil {
		prior = Workspace{}
	}

	// Step 2: compose the display label (D-23: only the sanitized form is ever
	// printed; the raw basename is not referenced after this point).
	sanitized := SanitizeBasename(filepath.Base(projectDir))
	label := composeWorkspaceLabel(sanitized)

	exe, err := resolveExecutable()
	if err != nil {
		return fmt.Errorf("cmuxctl: resolve binary path: %w", err)
	}
	socketPath := filepath.Join(projectDir, ".pr9k", "cmux-pane-"+label+".sock")
	paneEnv := map[string]string{
		"PR9K_CMUX_SOCKET": socketPath,
		"PR9K_PROJECT_DIR": projectDir,
	}

	// Step 2a (Architecture A, D-R1): the orchestrator must Serve the socket
	// the panes will Dial BEFORE the panes are created. A failure here aborts
	// before any cmux workspace exists, so there is nothing to tear down.
	if hooks.BeforePanes != nil {
		if err := hooks.BeforePanes(socketPath); err != nil {
			return fmt.Errorf("cmuxctl: orchestrator serve: %w", err)
		}
	}

	// Step 2b: create the workspace; first surface is the log pane.
	ws, err := client.WorkspaceCreate(ctx, WorkspaceCreateOpts{
		Title:            label,
		WorkingDirectory: projectDir,
		InitialCommand:   paneCommand(exe, "log", paneEnv),
		InitialEnv:       paneEnv,
	})
	if err != nil {
		return fmt.Errorf("cmuxctl: workspace create: %w", err)
	}

	dbg("RunPhase1: workspace.create OK — handle id=%q ref=%q socket=%q", ws.ID, ws.Ref, socketPath)

	// Step 3: print the workspace-label confirmation (spec D2, D-23).
	_, _ = fmt.Fprintf(out, "pr9k workspace: %s\n", label)

	var (
		teardownOnce sync.Once
		teardownErr  error
		obs          *DismissalObserver
	)
	runTeardown := func() {
		teardownOnce.Do(func() {
			if obs != nil {
				obs.SetShuttingDown()
				obs.Cancel()
			}
			closeCtx := context.Background()
			if err := client.WorkspaceClose(closeCtx, ws); err != nil {
				_, _ = fmt.Fprintf(stderr, "pr9k: orphan workspace %q could not be closed; dismiss it manually via cmux's controls\n", label)
				teardownErr = err
			}
			if !prior.Empty() {
				_ = client.WorkspaceSelect(closeCtx, prior)
			}
			if obs != nil {
				obs.Wait()
			}
		})
	}
	defer func() {
		runTeardown()
		if returnErr == nil && teardownErr != nil {
			returnErr = fmt.Errorf("cmuxctl: workspace close failed: %w", teardownErr)
		}
	}()

	// Step 4: split in the header and footer panes.
	for _, sp := range []struct {
		role string
		dir  SplitDirection
	}{
		{"header", SplitUp},
		{"footer", SplitDown},
	} {
		if _, err := client.SurfaceSplit(ctx, SplitOpts{
			Workspace:        ws,
			Direction:        sp.dir,
			WorkingDirectory: projectDir,
			InitialCommand:   paneCommand(exe, sp.role, paneEnv),
		}); err != nil {
			return fmt.Errorf("cmuxctl: split %s pane: %w", sp.role, err)
		}
		dbg("RunPhase1: surface.split %s OK", sp.role)
	}

	// Step 5: observe for dismissal. One watcher goroutine forwards the single
	// dismissal event to dismissCh (cap 1, non-blocking) and cancels runCtx so
	// an operator-initiated workspace close stops the workflow. runCtx is a
	// child of ctx and is cancelled on return (deferred), so the goroutine
	// always exits — no leak in either mode.
	obs = StartDismissalObserver(ctx, client, ws, expectedDisplaySurfaces, dismissalCfg)
	dismissCh := make(chan DismissalEvent, 1)
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()
	go func() {
		select {
		case <-runCtx.Done():
		case ev := <-obs.Ch:
			select {
			case dismissCh <- ev:
			default:
			}
			runCancel()
		}
	}()

	fatalErr := func() error {
		return fmt.Errorf("cmuxctl: dismissal poll fatal: %d consecutive timeouts; workspace %s may need manual cleanup", maxConsecutiveTimeouts, label)
	}

	// Workspace-lifecycle-only mode (nil Run): block until dismissal or ctx.
	if hooks.Run == nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt := <-dismissCh:
			if evt.Fatal {
				return fatalErr()
			}
			return nil
		}
	}

	// Architecture A: this process is the orchestrator. Run the workflow;
	// runCtx is cancelled if the operator dismisses the workspace mid-run.
	dbg("RunPhase1: entering hooks.Run (orchestrator); observer poll=%s", dismissalCfg.PollInterval)
	runErr := hooks.Run(runCtx, ws)
	dbg("RunPhase1: hooks.Run returned err=%v (runCtx.Err=%v ctx.Err=%v)", runErr, runCtx.Err(), ctx.Err())

	// If the operator dismissed (cancelling Run), tear down now.
	select {
	case evt := <-dismissCh:
		if evt.Fatal {
			return fatalErr()
		}
		return runErr
	default:
	}
	if runErr != nil {
		return runErr
	}

	// Workflow finished on its own: keep the workspace open showing the final
	// state until the operator dismisses it (or a signal cancels ctx).
	select {
	case <-ctx.Done():
		return ctx.Err()
	case evt := <-dismissCh:
		if evt.Fatal {
			return fatalErr()
		}
		return nil
	}
}
