package workflow

import (
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

// TP-001: sessionBlacklist populated when a claude step times out and the
// pipeline captured a partial session_id from a result event.
func TestTimeout_SessionBlacklist_PopulatedOnTimedOutStepWithSessionID(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()
	r.timeoutGraceOverride = 200 * time.Millisecond

	const wantSession = "test-sess-tp001"
	// Emit a valid result event with session_id, then ignore SIGTERM so the
	// timeout goroutine must escalate to SIGKILL.
	script := `printf '{"type":"result","subtype":"success","session_id":"test-sess-tp001","is_error":false,"result":"ok"}\n'; ` +
		`trap '' TERM; while true; do sleep 0.05; done`

	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunSandboxedStep("session-step", []string{"sh", "-c", script}, SandboxOptions{
			CaptureMode:    ui.CaptureResult,
			TimeoutSeconds: 1,
		})
	}()

	select {
	case err := <-stepDone:
		if err == nil {
			t.Fatal("expected non-nil error from timed-out step, got nil")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RunSandboxedStep did not complete within 10 seconds")
	}

	if !r.WasTimedOut() {
		t.Error("WasTimedOut should be true after timeout fired")
	}

	if !r.SessionBlacklisted(wantSession) {
		t.Errorf("SessionBlacklisted(%q) should be true after timeout, but was false", wantSession)
	}
}

// TP-002: SessionBlacklist NOT populated when the step times out but emits no
// session_id (the stats.SessionID guard prevents inserting the empty-string key).
func TestTimeout_SessionBlacklist_NotPopulatedWhenNoSessionID(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()
	r.timeoutGraceOverride = 200 * time.Millisecond

	// No NDJSON output at all — the aggregator never sees a session_id.
	script := `trap '' TERM; while true; do sleep 0.05; done`

	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunSandboxedStep("no-session-step", []string{"sh", "-c", script}, SandboxOptions{
			CaptureMode:    ui.CaptureResult,
			TimeoutSeconds: 1,
		})
	}()

	select {
	case err := <-stepDone:
		if err == nil {
			t.Fatal("expected non-nil error from timed-out step, got nil")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RunSandboxedStep did not complete within 10 seconds")
	}

	if !r.WasTimedOut() {
		t.Error("WasTimedOut should be true after timeout fired")
	}

	sessions := r.BlacklistedSessions()
	for _, id := range sessions {
		if id == "" {
			t.Error("SessionBlacklist must not contain the empty-string key")
		}
	}
	if len(sessions) != 0 {
		t.Errorf("SessionBlacklist should be empty (no session_id emitted), got len=%d", len(sessions))
	}
}

// TP-007: Zero timeoutSeconds is a no-op — the per-step timeout goroutine is
// never started, so a step that takes a few hundred milliseconds completes
// normally without being killed.
func TestTimeout_ZeroMeansNoTimeout(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()
	// With graceOverride=10ms: if the guard was absent, time.After(0) would fire
	// immediately and SIGKILL would arrive within ~10ms — killing sleep 0.2.
	r.timeoutGraceOverride = 10 * time.Millisecond

	err := r.RunStepFull("no-timeout-step", []string{"sh", "-c", "sleep 0.2"}, ui.CaptureLastLine, 0)
	if err != nil {
		t.Fatalf("RunStepFull with zero timeout returned unexpected error: %v", err)
	}
	if r.WasTimedOut() {
		t.Error("WasTimedOut should be false when TimeoutSeconds is 0")
	}
}

// TP-008: timeoutFired is reset to false at the start of each RunStepFull call
// so that subsequent steps are not erroneously marked as timed out.
func TestTimeout_TimeoutFiredResetBetweenRunStepFullCalls(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()
	r.timeoutGraceOverride = 200 * time.Millisecond

	// First call: timeout fires and SIGKILL escalation kills the process.
	script1 := `trap '' TERM; while true; do sleep 0.05; done`
	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunStepFull("timeout-step", []string{"sh", "-c", script1}, ui.CaptureLastLine, 1)
	}()
	select {
	case err := <-stepDone:
		if err == nil {
			t.Fatal("first call: expected non-nil error after timeout+SIGKILL, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first call: RunStepFull did not complete within 5 seconds")
	}
	if !r.WasTimedOut() {
		t.Error("after first call: WasTimedOut should be true")
	}

	// Second call: a plain echo; the timeoutFired flag must be cleared on entry.
	if err := r.RunStepFull("ok-step", []string{"echo", "ok"}, ui.CaptureLastLine, 0); err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if r.WasTimedOut() {
		t.Error("after second call: WasTimedOut should be false — timeoutFired must be reset on entry")
	}
}

// TP-010 (MED): timeoutFired is reset at the start of each RunSandboxedStep
// call. SessionBlacklist state accumulated during the first call is preserved.
func TestTimeout_TimeoutFiredResetBetweenRunSandboxedStepCalls(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()
	r.timeoutGraceOverride = 200 * time.Millisecond

	// First call: timeout fires (no pipeline — CaptureMode defaults to CaptureLastLine).
	script1 := `trap '' TERM; while true; do sleep 0.05; done`
	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunSandboxedStep("timeout-step", []string{"sh", "-c", script1}, SandboxOptions{
			TimeoutSeconds: 1,
		})
	}()
	select {
	case <-stepDone:
	case <-time.After(5 * time.Second):
		t.Fatal("first call: RunSandboxedStep did not complete within 5 seconds")
	}
	if !r.WasTimedOut() {
		t.Error("after first call: WasTimedOut should be true")
	}

	// Second call: succeeds with zero timeout; flag must be reset.
	if err := r.RunSandboxedStep("ok-step", []string{"true"}, SandboxOptions{}); err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if r.WasTimedOut() {
		t.Error("after second call: WasTimedOut should be false — timeoutFired must be reset on entry")
	}

	// sessionBlacklist state from the first call is preserved (no session_id was
	// emitted, so it remains empty — the important thing is it is not wiped).
	if n := len(r.BlacklistedSessions()); n != 0 {
		t.Errorf("sessionBlacklist should still be empty after both calls, got len=%d", n)
	}
}

// TP-011 (MED): inner select on procDone vs grace time.After — when the
// subprocess exits within the grace window after SIGTERM, SIGKILL is not sent.
// Uses a recording+forwarding terminator to observe which signals were delivered.
func TestTimeout_GraceWindowNoSIGKILL(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()
	r.timeoutGraceOverride = 500 * time.Millisecond

	var sigsLock sync.Mutex
	var sigs []syscall.Signal

	// The terminator records every signal it receives and forwards it to
	// currentProc (read at call time so we don't need a pre-captured pointer).
	terminator := func(sig syscall.Signal) error {
		sigsLock.Lock()
		sigs = append(sigs, sig)
		sigsLock.Unlock()
		r.processMu.Lock()
		proc := r.currentProc
		r.processMu.Unlock()
		if proc != nil {
			return proc.Signal(sig)
		}
		return nil
	}

	// Script exits on SIGTERM — should die within the 500ms grace window so
	// SIGKILL is never needed.
	script := `trap 'exit 1' TERM; while true; do sleep 0.01; done`
	err := r.RunSandboxedStep("grace-step", []string{"sh", "-c", script}, SandboxOptions{
		Terminator:     terminator,
		TimeoutSeconds: 1,
	})
	if err == nil {
		t.Fatal("expected non-nil error (killed by timeout), got nil")
	}
	if !r.WasTimedOut() {
		t.Error("WasTimedOut should be true after timeout fired")
	}

	sigsLock.Lock()
	gotSigs := make([]syscall.Signal, len(sigs))
	copy(gotSigs, sigs)
	sigsLock.Unlock()

	// Only SIGTERM should have been sent; procDone fires (process exited on
	// SIGTERM) before the 500ms grace timer, so SIGKILL is never sent.
	for _, s := range gotSigs {
		if s == syscall.SIGKILL {
			t.Errorf("SIGKILL was sent; process should have exited within the grace window after SIGTERM")
		}
	}
	if len(gotSigs) == 0 {
		t.Error("no signals recorded; expected at least SIGTERM from the timeout goroutine")
	}
}

// TP-019 (LOW): When no opts.Terminator is installed (host steps via RunStepFull),
// the timeout goroutine falls back to proc.Signal/proc.Kill and currentTerminator
// is observed nil throughout the call.
func TestTimeout_NilTerminatorFallback(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()
	r.timeoutGraceOverride = 200 * time.Millisecond

	script := `trap '' TERM; while true; do sleep 0.05; done`

	observedNonNilTerm := make(chan bool, 1)
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			r.processMu.Lock()
			term := r.currentTerminator
			r.processMu.Unlock()
			if term != nil {
				observedNonNilTerm <- true
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
		observedNonNilTerm <- false
	}()

	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunStepFull("host-step", []string{"sh", "-c", script}, ui.CaptureLastLine, 1)
	}()

	select {
	case err := <-stepDone:
		if err == nil {
			t.Error("expected non-nil error after SIGKILL, got nil")
		}
		if !r.WasTimedOut() {
			t.Error("WasTimedOut should be true after timeout")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunStepFull did not complete within 5 seconds")
	}

	if <-observedNonNilTerm {
		t.Error("currentTerminator was observed non-nil during RunStepFull; host steps must never install a terminator")
	}
}

// TP-020 (LOW): timeoutGracePeriod constant is 10 seconds, matching the
// "10 seconds" documented in docs/how-to/setting-step-timeouts.md.
func TestTimeoutGracePeriod_IsTenSeconds(t *testing.T) {
	want := 10 * time.Second
	if timeoutGracePeriod != want {
		t.Errorf("timeoutGracePeriod = %v, want %v — update docs/how-to/setting-step-timeouts.md if this changes", timeoutGracePeriod, want)
	}
}

// TOT-R1: ClearTimeoutFlag resets timeoutFired to false. Verifies the executor
// side of the onTimeout=continue fix — without this, a residual timeoutFired
// would leak across dispatcher boundaries.
func TestRunner_ClearTimeoutFlag_ResetsTimedOutFlag(t *testing.T) {
	r, log, _ := newCapturingRunner(t)
	defer func() { _ = log.Close() }()
	r.timeoutGraceOverride = 200 * time.Millisecond

	stepDone := make(chan error, 1)
	go func() {
		stepDone <- r.RunStepFull("slow", []string{"sh", "-c", "trap '' TERM; sleep 5"}, ui.CaptureLastLine, 1)
	}()

	select {
	case <-stepDone:
	case <-time.After(5 * time.Second):
		t.Fatal("RunStepFull did not complete within 5 seconds")
	}

	if !r.WasTimedOut() {
		t.Fatal("WasTimedOut should be true after the timer fires")
	}

	r.ClearTimeoutFlag()
	if r.WasTimedOut() {
		t.Error("WasTimedOut should be false after ClearTimeoutFlag()")
	}

	// Idempotent: calling again does not panic or change state.
	r.ClearTimeoutFlag()
	if r.WasTimedOut() {
		t.Error("WasTimedOut should remain false after a second ClearTimeoutFlag()")
	}
}
