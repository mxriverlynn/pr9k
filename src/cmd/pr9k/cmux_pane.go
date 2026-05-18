package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/cli"
	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/logger"
	"github.com/mxriverlynn/pr9k/src/internal/statusline"
	"github.com/mxriverlynn/pr9k/src/internal/steps"
	"github.com/spf13/cobra"
)

// workspaceDoneAckTimeout is the maximum time the orchestrator waits for all
// three display panes to send DoneAck after WorkspaceDone is broadcast.
const workspaceDoneAckTimeout = 5 * time.Second

// orchChannel is satisfied by *interactionchannel.Channel (Serve side) and by
// FakeInteractionChannel in tests. It is the minimum surface the orchestrator
// needs from the interaction channel.
type orchChannel interface {
	AwaitReady(ctx context.Context, timeout time.Duration) error
	Send(msg interactionchannel.Message) error
	Recv() <-chan interactionchannel.Message
	Close()
	SendStateHeader(msg interactionchannel.StateHeader)
	SendStateLog(msg interactionchannel.StateLog)
	SendStateFooter(msg interactionchannel.StateFooter)
}

// Compile-time assertion: *interactionchannel.Channel must satisfy orchChannel.
var _ orchChannel = (*interactionchannel.Channel)(nil)

// newCmuxPaneCmd returns the cobra sub-command for internal per-pane processes
// launched by cmux mode. Operators must not run this directly; it is invoked
// via SurfaceSpawn by RunPhase1.
func newCmuxPaneCmd() *cobra.Command {
	var role string
	cmd := &cobra.Command{
		Use:   "cmux-pane",
		Short: "Internal — launched by cmux mode, not for direct operator use",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch role {
			case "orchestrator", "header", "log", "footer":
			default:
				return fmt.Errorf("cmux-pane: invalid --role %q; must be one of orchestrator, header, log, footer", role)
			}

			socketPath := os.Getenv("PR9K_CMUX_SOCKET")
			if socketPath == "" {
				return errors.New("cmux-pane: PR9K_CMUX_SOCKET is not set; this command is launched internally by pr9k cmux mode")
			}

			ctx := cmd.Context()
			switch role {
			case "orchestrator":
				projectDir := os.Getenv("PR9K_PROJECT_DIR")
				return runCmuxOrchestrator(ctx, socketPath, projectDir)
			case "header":
				return runCmuxHeaderPane(ctx, socketPath)
			case "log":
				return runCmuxLogPane(ctx, socketPath)
			case "footer":
				return runCmuxFooterPane(ctx, socketPath)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "pane role: orchestrator, header, log, or footer (required)")
	_ = cmd.MarkFlagRequired("role")
	return cmd
}

// runCmuxOrchestrator is the entry point for the orchestrator pane process.
func runCmuxOrchestrator(ctx context.Context, socketPath string, projectDir string) error {
	return runCmuxOrchestratorWith(ctx, socketPath, projectDir, workspaceDoneAckTimeout, nil)
}

// runCmuxOrchestratorWith is the testable core of runCmuxOrchestrator. ch is
// the interaction channel; when nil, a real Serve channel is created on
// socketPath. ackTimeout controls how long the orchestrator waits for DoneAcks
// before proceeding (injectable for tests).
func runCmuxOrchestratorWith(ctx context.Context, socketPath, projectDir string, ackTimeout time.Duration, ch orchChannel) error {
	if projectDir == "" {
		return errors.New("cmux-pane: PR9K_PROJECT_DIR is not set; this command is launched internally by pr9k cmux mode")
	}

	log, err := logger.NewLogger(projectDir)
	if err != nil {
		return fmt.Errorf("cmux-pane: orchestrator: create logger: %w", err)
	}
	defer func() { _ = log.Close() }()

	// Create the per-run artifact directory eagerly, mirroring startup() in main.go.
	artifactDir := filepath.Join(projectDir, ".pr9k", "logs", log.RunStamp())
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		return fmt.Errorf("cmux-pane: orchestrator: create artifact dir: %w", err)
	}

	if ch == nil {
		var serveErr error
		ch, serveErr = interactionchannel.Serve(ctx, socketPath)
		if serveErr != nil {
			return fmt.Errorf("cmux-pane: orchestrator: serve: %w", serveErr)
		}
	}
	defer ch.Close()

	if err := ch.AwaitReady(ctx, interactionchannel.ReadyHandshakeTimeout); err != nil {
		return fmt.Errorf("cmux-pane: orchestrator: handshake: %w", err)
	}

	// Workflow runs here — real implementation ships in a later work unit.
	exitCode := 0

	// Broadcast WorkspaceDone to all three display panes.
	_ = ch.Send(interactionchannel.WorkspaceDone{ExitCode: exitCode})

	// Wait up to ackTimeout for all three display roles to acknowledge.
	awaitDoneAcks(ch.Recv(), ackTimeout)
	return nil
}

// awaitDoneAcks collects DoneAck messages from recv until all three display
// roles ("header", "log", "footer") have acked or timeout elapses. Passing a
// short timeout is safe: any non-acking pane will not block the orchestrator.
func awaitDoneAcks(recv <-chan interactionchannel.Message, timeout time.Duration) {
	acked := make(map[string]bool)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for len(acked) < 3 {
		select {
		case msg, ok := <-recv:
			if !ok {
				return
			}
			if ack, ok := msg.(interactionchannel.DoneAck); ok {
				acked[ack.Role] = true
			}
		case <-deadline.C:
			return
		}
	}
}

// runCmuxHeaderPane is the entry point for the header pane process. It dials
// the interaction channel, sends Ready, handles WorkspaceDone by transitioning
// to final-state render and sending DoneAck, then keeps running until killed
// (context cancel or process exit by the workspace teardown).
func runCmuxHeaderPane(ctx context.Context, socketPath string) error {
	return runCmuxDisplayPane(ctx, socketPath, "header")
}

// runCmuxLogPane is the entry point for the log pane process. Symmetric to
// runCmuxHeaderPane.
func runCmuxLogPane(ctx context.Context, socketPath string) error {
	return runCmuxDisplayPane(ctx, socketPath, "log")
}

// runCmuxDisplayPane implements the shared interaction-channel lifecycle for a
// non-footer display pane. It dials socketPath, sends Ready{role}, and loops
// reading messages. On WorkspaceDone it sends DoneAck and transitions to a
// final-state hold, looping on context or EOF until the workspace is dismissed.
func runCmuxDisplayPane(ctx context.Context, socketPath, role string) error {
	ch, err := interactionchannel.Dial(ctx, socketPath, role)
	if err != nil {
		// Graceful degradation: no interaction channel, run until ctx cancel.
		<-ctx.Done()
		return nil
	}
	defer ch.Close()

	if err := ch.Send(interactionchannel.Ready{Role: role}); err != nil {
		return nil
	}

	for {
		select {
		case msg, ok := <-ch.Recv():
			if !ok {
				// Socket closed by orchestrator after WorkspaceDone cycle — stay
				// alive so the final-state render is visible until dismissed.
				<-ctx.Done()
				return nil
			}
			if _, ok := msg.(interactionchannel.WorkspaceDone); ok {
				_ = ch.Send(interactionchannel.DoneAck{Role: role})
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// runCmuxFooterPane is the entry point for the footer pane process. It reads
// the workflow config via PR9K_PROJECT_DIR, constructs a statusline.Runner
// with a nil logger (D-9), and renders the runner output to the pane's stdout.
func runCmuxFooterPane(ctx context.Context, socketPath string) error {
	return runCmuxFooterPaneWith(ctx, socketPath, os.Stdout)
}

// runCmuxFooterPaneWith is the testable core of runCmuxFooterPane. out receives
// the status-line output lines; in production os.Stdout is passed.
func runCmuxFooterPaneWith(ctx context.Context, socketPath string, out io.Writer) error {
	projectDir := os.Getenv("PR9K_PROJECT_DIR")

	// Resolve workflowDir so relative script paths in statusLine.command work.
	// Gracefully degrade: if neither candidate exists the runner is no-op.
	var workflowDir string
	if projectDir != "" {
		if dir, err := cli.ResolveWorkflowDir(projectDir); err == nil {
			workflowDir = dir
		}
	}

	// Load the statusLine config. Gracefully degrade if config.json is absent.
	var slCfg *statusline.Config
	if workflowDir != "" {
		if sf, err := steps.LoadSteps(workflowDir); err == nil {
			slCfg = buildStatusLineConfig(sf.StatusLine)
		}
	}

	// Construct runner with nil logger per D-9: only on-disk log persistence is
	// dropped; script stderr is handled gracefully (logLine is a no-op when nil).
	runner := statusline.New(slCfg, workflowDir, projectDir, nil)

	// Dial the interaction channel.
	paneCh, dialErr := interactionchannel.Dial(ctx, socketPath, "footer")

	if !runner.Enabled() && dialErr != nil {
		// Nothing to do — no statusline runner and no interaction channel.
		return nil
	}

	if runner.Enabled() {
		runner.SetSender(func(_ interface{}) {
			if output := runner.LastOutput(); output != "" {
				_, _ = fmt.Fprintln(out, output)
			}
		})
		runner.Start(ctx)
		defer runner.Shutdown()
		runner.Trigger()
	}

	if dialErr != nil {
		// Statusline runner active but no interaction channel — run until ctx cancel.
		<-ctx.Done()
		return nil
	}
	defer paneCh.Close()

	if err := paneCh.Send(interactionchannel.Ready{Role: "footer"}); err != nil {
		<-ctx.Done()
		return nil
	}

	for {
		select {
		case msg, ok := <-paneCh.Recv():
			if !ok {
				// Socket closed by orchestrator — stay alive for final-state render.
				<-ctx.Done()
				return nil
			}
			if _, ok := msg.(interactionchannel.WorkspaceDone); ok {
				_ = paneCh.Send(interactionchannel.DoneAck{Role: "footer"})
				// Keep running until context is cancelled or process is killed.
				<-ctx.Done()
				return nil
			}
		case <-ctx.Done():
			return nil
		}
	}
}
