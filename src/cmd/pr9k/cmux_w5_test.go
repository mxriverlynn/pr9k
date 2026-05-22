// W-5 tests: pane-side abort rendering and mid-run cmux error classification.
//
// Behaviors verified:
//
//	B1: runCmuxDisplayPaneWith renders RunAbortedToken and returns when the
//	    orchestrator socket closes (stall/disconnect path — stalledCh fires).
//	B2: runCmuxDisplayPaneWith renders RunAbortedToken and returns on
//	    WorkspaceDone{Aborted: true}.
//	B3: runCmuxFooterMachineWith renders RunAbortedToken and returns on a
//	    stalledCh fire.
//	B4: runCmuxFooterMachineWith renders RunAbortedToken and returns on
//	    WorkspaceDone{Aborted: true}.
//	B5: classifyMidRunCmuxError maps a recognized *CmuxError code to a named
//	    AbortCause.
//	B6: classifyMidRunCmuxError maps an unrecognized *CmuxError code to an
//	    unclassified cause that preserves the raw code verbatim.
package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
)

// ---------------------------------------------------------------------------
// B1: display pane returns on stall/disconnect (stalledCh fires via DialWith)
// ---------------------------------------------------------------------------

// TestDisplayPaneW5_StalledCh_RendersAbortTokenAndReturns verifies that
// runCmuxDisplayPaneWith renders a final line containing RunAbortedToken and
// returns (does not block on ctx.Done()) when the orchestrator socket closes.
// The socket close fires the onLost callback registered via DialWith, which
// closes the pane's internal stalledCh.
func TestDisplayPaneW5_StalledCh_RendersAbortTokenAndReturns(t *testing.T) {
	socketPath := shortSockPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var buf syncBuffer
	paneErr := make(chan error, 1)
	server := startServerAndWaitForRole(t, ctx, socketPath, "header", func() {
		go func() { paneErr <- runCmuxDisplayPaneWith(ctx, socketPath, "header", &buf) }()
	})

	// Close the server socket — simulates orchestrator death. The pane's
	// DialWith onLost callback fires, closing the internal stalledCh.
	server.Close()

	select {
	case err := <-paneErr:
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runCmuxDisplayPaneWith did not return after server close (stalledCh path)")
	}

	if !strings.Contains(buf.String(), interactionchannel.RunAbortedToken) {
		t.Errorf("expected %q in output, got: %q", interactionchannel.RunAbortedToken, buf.String())
	}
}

// ---------------------------------------------------------------------------
// B2: display pane returns on WorkspaceDone{Aborted: true}
// ---------------------------------------------------------------------------

// TestDisplayPaneW5_WorkspaceDoneAborted_RendersAbortTokenAndReturns verifies
// that runCmuxDisplayPaneWith renders RunAbortedToken and returns (does not
// block on ctx.Done()) when WorkspaceDone{Aborted: true} is received.
func TestDisplayPaneW5_WorkspaceDoneAborted_RendersAbortTokenAndReturns(t *testing.T) {
	socketPath := shortSockPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var buf syncBuffer
	paneErr := make(chan error, 1)
	server := startServerAndWaitForRole(t, ctx, socketPath, "header", func() {
		go func() { paneErr <- runCmuxDisplayPaneWith(ctx, socketPath, "header", &buf) }()
	})
	defer server.Close()

	_ = server.Send(interactionchannel.WorkspaceDone{ExitCode: 1, Aborted: true})

	select {
	case err := <-paneErr:
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runCmuxDisplayPaneWith did not return after WorkspaceDone{Aborted:true}")
	}

	if !strings.Contains(buf.String(), interactionchannel.RunAbortedToken) {
		t.Errorf("expected %q in output, got: %q", interactionchannel.RunAbortedToken, buf.String())
	}
}

// ---------------------------------------------------------------------------
// B3: footer machine returns on stalledCh fire
// ---------------------------------------------------------------------------

// runMachineFixtureW5 is like runMachineFixture but accepts a stalledCh,
// allowing W-5 tests to fire the stall path directly.
func runMachineFixtureW5(
	t *testing.T,
	src FooterKeySource,
	ch footerPaneCh,
	out *syncBuffer,
	stalledCh <-chan struct{},
) (cancel context.CancelFunc, done <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		runCmuxFooterMachineWith(ctx, src, ch, out, stalledCh)
	}()
	return cancel, doneCh
}

// TestFooterMachineW5_StalledCh_RendersAbortTokenAndReturns verifies that
// runCmuxFooterMachineWith renders RunAbortedToken and returns when stalledCh
// is closed — it must NOT block on ctx.Done().
func TestFooterMachineW5_StalledCh_RendersAbortTokenAndReturns(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	fch := interactionchannel.NewFakeInteractionChannel()
	var out syncBuffer

	stalledCh := make(chan struct{})
	cancel, done := runMachineFixtureW5(t, src, fch, &out, stalledCh)
	defer cancel()

	close(stalledCh)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runCmuxFooterMachineWith did not return after stalledCh close")
	}

	if !strings.Contains(out.String(), interactionchannel.RunAbortedToken) {
		t.Errorf("expected %q in output, got: %q", interactionchannel.RunAbortedToken, out.String())
	}
}

// ---------------------------------------------------------------------------
// B4: footer machine returns on WorkspaceDone{Aborted: true}
// ---------------------------------------------------------------------------

// TestFooterMachineW5_WorkspaceDoneAborted_RendersAbortTokenAndReturns verifies
// that runCmuxFooterMachineWith renders RunAbortedToken and returns on
// WorkspaceDone{Aborted: true} — it must NOT block on ctx.Done().
func TestFooterMachineW5_WorkspaceDoneAborted_RendersAbortTokenAndReturns(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	fch := interactionchannel.NewFakeInteractionChannel()
	var out syncBuffer

	stalledCh := make(chan struct{}) // never closed in this test
	cancel, done := runMachineFixtureW5(t, src, fch, &out, stalledCh)
	defer cancel()

	fch.InjectMessage(interactionchannel.WorkspaceDone{ExitCode: 1, Aborted: true})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runCmuxFooterMachineWith did not return after WorkspaceDone{Aborted:true}")
	}

	if !strings.Contains(out.String(), interactionchannel.RunAbortedToken) {
		t.Errorf("expected %q in output, got: %q", interactionchannel.RunAbortedToken, out.String())
	}
}

// ---------------------------------------------------------------------------
// B5: classifyMidRunCmuxError maps recognized codes to named AbortCauses
// ---------------------------------------------------------------------------

// TestClassifyMidRunCmuxError_AuthRequired_ClassifiesAuth verifies that
// auth_required maps to AbortCauseCmuxAuth.
func TestClassifyMidRunCmuxError_AuthRequired_ClassifiesAuth(t *testing.T) {
	cause := classifyMidRunCmuxError(&cmuxctl.CmuxError{Code: "auth_required", Message: "needs password"})
	if cause != AbortCauseCmuxAuth {
		t.Errorf("expected %q, got %q", AbortCauseCmuxAuth, cause)
	}
}

// TestClassifyMidRunCmuxError_AuthFailed_ClassifiesAuth verifies that
// auth_failed maps to AbortCauseCmuxAuth.
func TestClassifyMidRunCmuxError_AuthFailed_ClassifiesAuth(t *testing.T) {
	cause := classifyMidRunCmuxError(&cmuxctl.CmuxError{Code: "auth_failed", Message: "bad password"})
	if cause != AbortCauseCmuxAuth {
		t.Errorf("expected %q, got %q", AbortCauseCmuxAuth, cause)
	}
}

// TestClassifyMidRunCmuxError_AuthUnconfigured_ClassifiesAuth verifies that
// auth_unconfigured maps to AbortCauseCmuxAuth.
func TestClassifyMidRunCmuxError_AuthUnconfigured_ClassifiesAuth(t *testing.T) {
	cause := classifyMidRunCmuxError(&cmuxctl.CmuxError{Code: "auth_unconfigured", Message: "no auth"})
	if cause != AbortCauseCmuxAuth {
		t.Errorf("expected %q, got %q", AbortCauseCmuxAuth, cause)
	}
}

// TestClassifyMidRunCmuxError_MethodNotFound_ClassifiesMethodNotFound verifies
// that method_not_found maps to AbortCauseCmuxMethodNotFound.
func TestClassifyMidRunCmuxError_MethodNotFound_ClassifiesMethodNotFound(t *testing.T) {
	cause := classifyMidRunCmuxError(&cmuxctl.CmuxError{Code: "method_not_found", Message: "no such method"})
	if cause != AbortCauseCmuxMethodNotFound {
		t.Errorf("expected %q, got %q", AbortCauseCmuxMethodNotFound, cause)
	}
}

// TestClassifyMidRunCmuxError_UnknownMethod_ClassifiesMethodNotFound verifies
// that unknown_method maps to AbortCauseCmuxMethodNotFound.
func TestClassifyMidRunCmuxError_UnknownMethod_ClassifiesMethodNotFound(t *testing.T) {
	cause := classifyMidRunCmuxError(&cmuxctl.CmuxError{Code: "unknown_method", Message: "no such method"})
	if cause != AbortCauseCmuxMethodNotFound {
		t.Errorf("expected %q, got %q", AbortCauseCmuxMethodNotFound, cause)
	}
}

// TestClassifyMidRunCmuxError_PlaintextAccessDenied_ClassifiesAccessDenied
// verifies that a PlaintextError with IsAccessDenied() maps to
// AbortCauseCmuxAccessDenied.
func TestClassifyMidRunCmuxError_PlaintextAccessDenied_ClassifiesAccessDenied(t *testing.T) {
	cause := classifyMidRunCmuxError(&cmuxctl.PlaintextError{Raw: "ERROR: Access denied by cmux"})
	if cause != AbortCauseCmuxAccessDenied {
		t.Errorf("expected %q, got %q", AbortCauseCmuxAccessDenied, cause)
	}
}

// ---------------------------------------------------------------------------
// B6: classifyMidRunCmuxError preserves raw code for unrecognized errors
// ---------------------------------------------------------------------------

// TestClassifyMidRunCmuxError_UnrecognizedCode_PreservesRawCode verifies that
// an unrecognized *CmuxError code produces an unclassified AbortCause that
// contains the raw code verbatim.
func TestClassifyMidRunCmuxError_UnrecognizedCode_PreservesRawCode(t *testing.T) {
	rawCode := "some_weird_code_xyz"
	cause := classifyMidRunCmuxError(&cmuxctl.CmuxError{Code: rawCode, Message: "something went wrong"})
	if !strings.Contains(string(cause), rawCode) {
		t.Errorf("expected raw code %q in unclassified cause, got: %q", rawCode, cause)
	}
}
