package ui

import (
	"sync"
	"testing"
)

func newTestHandler(t *testing.T) (*KeyHandler, *bool, chan StepAction) {
	t.Helper()
	cancelCalled := false
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(func() { cancelCalled = true }, actions)
	return h, &cancelCalled, actions
}

// --- SetMode ---

func TestSetMode_Quitting_UpdatesShortcutLine(t *testing.T) {
	h, _, _ := newTestHandler(t)

	h.SetMode(ModeQuitting)

	if h.ShortcutLine() != QuittingLine {
		t.Errorf("expected QuittingLine, got %q", h.ShortcutLine())
	}
}

func TestSetMode_Error_UpdatesShortcutLine(t *testing.T) {
	h, _, _ := newTestHandler(t)

	h.SetMode(ModeError)

	if h.ShortcutLine() != ErrorShortcuts {
		t.Errorf("expected error shortcuts, got %q", h.ShortcutLine())
	}
}

// TP-003: SetMode(ModeNormal) sets NormalShortcuts (direct test through ShortcutLine accessor).
func TestSetMode_Normal_UpdatesShortcutLine(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.SetMode(ModeError)

	h.SetMode(ModeNormal)

	if h.ShortcutLine() != NormalShortcuts {
		t.Errorf("expected NormalShortcuts after SetMode(ModeNormal), got %q", h.ShortcutLine())
	}
}

// TP-004: SetMode(ModeQuitConfirm) sets QuitConfirmPrompt.
func TestSetMode_QuitConfirm_UpdatesShortcutLine(t *testing.T) {
	h, _, _ := newTestHandler(t)

	h.SetMode(ModeQuitConfirm)

	if h.ShortcutLine() != QuitConfirmPrompt {
		t.Errorf("expected QuitConfirmPrompt after SetMode(ModeQuitConfirm), got %q", h.ShortcutLine())
	}
}

// --- Constructor ---

func TestNewKeyHandler_InitialState(t *testing.T) {
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(func() {}, actions)

	if h.ShortcutLine() != NormalShortcuts {
		t.Errorf("expected NormalShortcuts, got %q", h.ShortcutLine())
	}
	if h.Actions != actions {
		t.Error("expected Actions to be the provided channel")
	}
	// TP-006: Mode() accessor must report ModeNormal at construction time.
	if h.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal initial mode, got %v", h.Mode())
	}
}

// --- ForceQuit ---

// TestForceQuit_CallsCancelAndInjectsActionQuit verifies that ForceQuit calls
// the cancel function and sends ActionQuit to the Actions channel.
func TestForceQuit_CallsCancelAndInjectsActionQuit(t *testing.T) {
	h, cancelCalled, actions := newTestHandler(t)

	h.ForceQuit()

	if !*cancelCalled {
		t.Error("expected cancel to be called by ForceQuit")
	}

	select {
	case action := <-actions:
		if action != ActionQuit {
			t.Errorf("expected ActionQuit, got %v", action)
		}
	default:
		t.Error("expected ActionQuit to be in Actions channel after ForceQuit")
	}

	if h.Mode() != ModeQuitting {
		t.Errorf("expected ModeQuitting after ForceQuit, got %v", h.Mode())
	}
}

// TestForceQuit_NilCancel_NoPanic verifies that ForceQuit is safe when cancel is nil.
func TestForceQuit_NilCancel_NoPanic(t *testing.T) {
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)

	h.ForceQuit() // must not panic

	select {
	case action := <-actions:
		if action != ActionQuit {
			t.Errorf("expected ActionQuit, got %v", action)
		}
	default:
		t.Error("expected ActionQuit in channel even when cancel is nil")
	}
}

// TestForceQuit_FullChannel_NoPanic verifies that ForceQuit does not block or
// panic when the Actions channel is already full.
func TestForceQuit_FullChannel_NoPanic(t *testing.T) {
	actions := make(chan StepAction, 1)
	actions <- ActionContinue // fill the channel
	h := NewKeyHandler(func() {}, actions)

	h.ForceQuit() // must not block or panic
}

// TestForceQuit_SetsModeQuitting_FromNormal verifies that ForceQuit flips mode to
// ModeQuitting and updates the footer even when called from ModeNormal (the signal path).
func TestForceQuit_SetsModeQuitting_FromNormal(t *testing.T) {
	h, _, _ := newTestHandler(t)

	h.ForceQuit()

	if h.Mode() != ModeQuitting {
		t.Errorf("expected ModeQuitting after ForceQuit, got %v", h.Mode())
	}
	if h.ShortcutLine() != QuittingLine {
		t.Errorf("expected QuittingLine after ForceQuit, got %q", h.ShortcutLine())
	}
}

// TestSetMode_NextConfirm_UpdatesShortcutLine verifies SetMode(ModeNextConfirm).
func TestSetMode_NextConfirm_UpdatesShortcutLine(t *testing.T) {
	h, _, _ := newTestHandler(t)

	h.SetMode(ModeNextConfirm)

	if h.ShortcutLine() != NextConfirmPrompt {
		t.Errorf("expected NextConfirmPrompt, got %q", h.ShortcutLine())
	}
}

// TestSetMode_Done_UpdatesShortcutLine verifies SetMode(ModeDone).
func TestSetMode_Done_UpdatesShortcutLine(t *testing.T) {
	h, _, _ := newTestHandler(t)

	h.SetMode(ModeDone)

	if h.ShortcutLine() != DoneShortcuts {
		t.Errorf("expected DoneShortcuts, got %q", h.ShortcutLine())
	}
}

// TestForceQuit_SetsModeQuitting_FromNextConfirm covers SIGINT during the skip
// confirmation prompt.
func TestForceQuit_SetsModeQuitting_FromNextConfirm(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.SetMode(ModeNextConfirm)

	h.ForceQuit()

	if h.Mode() != ModeQuitting {
		t.Errorf("expected ModeQuitting after ForceQuit, got %v", h.Mode())
	}
	if h.ShortcutLine() != QuittingLine {
		t.Errorf("expected QuittingLine after ForceQuit, got %q", h.ShortcutLine())
	}
}

// TestForceQuit_SetsModeQuitting_FromDone covers SIGINT during the post-workflow
// done screen.
func TestForceQuit_SetsModeQuitting_FromDone(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.SetMode(ModeDone)

	h.ForceQuit()

	if h.Mode() != ModeQuitting {
		t.Errorf("expected ModeQuitting after ForceQuit, got %v", h.Mode())
	}
	if h.ShortcutLine() != QuittingLine {
		t.Errorf("expected QuittingLine after ForceQuit, got %q", h.ShortcutLine())
	}
}

// TestForceQuit_SetsModeQuitting_FromError verifies that ForceQuit flips mode to
// ModeQuitting and updates the footer even when called from ModeError (the signal path).
func TestForceQuit_SetsModeQuitting_FromError(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.SetMode(ModeError)

	h.ForceQuit()

	if h.Mode() != ModeQuitting {
		t.Errorf("expected ModeQuitting after ForceQuit, got %v", h.Mode())
	}
	if h.ShortcutLine() != QuittingLine {
		t.Errorf("expected QuittingLine after ForceQuit, got %q", h.ShortcutLine())
	}
}

// T2 — ForceQuit is idempotent: calling it twice does not panic, and the
// second ActionQuit send is dropped because the channel already holds one.
func TestForceQuit_Idempotent_CalledTwice(t *testing.T) {
	cancelCount := 0
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(func() { cancelCount++ }, actions)

	h.ForceQuit()
	h.ForceQuit() // must not panic or block

	if cancelCount != 2 {
		t.Errorf("expected cancel called 2 times, got %d", cancelCount)
	}

	// Exactly one ActionQuit should be in the channel (second send dropped).
	if len(actions) != 1 {
		t.Errorf("expected exactly 1 ActionQuit in channel, got %d", len(actions))
	}
	action := <-actions
	if action != ActionQuit {
		t.Errorf("expected ActionQuit, got %v", action)
	}
}

// --- Race detector: concurrent ShortcutLine access ---

// TestShortcutLine_ConcurrentRead_NoRace simulates the Update goroutine
// reading ShortcutLine (via the mutex-protected accessor) concurrently while
// the workflow goroutine cycles modes. A second goroutine concurrently reads
// via Mode(). Verifies the handler is race-free under go test -race.
// Run with: go test -race ./...
func TestShortcutLine_ConcurrentRead_NoRace(t *testing.T) {
	h, _, _ := newTestHandler(t)

	stop := make(chan struct{})
	// Simulate the Update goroutine: continuously reads ShortcutLine.
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				_ = h.ShortcutLine()
			}
		}
	}()
	// Second concurrent reader via Mode accessor.
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				_ = h.Mode()
			}
		}
	}()

	// Workflow goroutine: cycle through all six modes for ≥500 iterations.
	modes := []Mode{ModeNormal, ModeError, ModeQuitConfirm, ModeNextConfirm, ModeDone, ModeQuitting}
	for i := range 500 {
		h.SetMode(modes[i%len(modes)])
	}

	close(stop)
}

// TestForceQuit_FromSignalPath_FootersShowQuitting simulates the OS signal path
// (SIGINT/SIGTERM) calling ForceQuit from a background goroutine while the
// handler starts in ModeNormal. The footer must show "Quitting..." after the
// call, matching the q→y confirmation path. Race-detector-safe.
func TestForceQuit_FromSignalPath_FootersShowQuitting(t *testing.T) {
	h, _, actions := newTestHandler(t)
	done := make(chan struct{})

	go func() {
		h.ForceQuit()
		close(done)
	}()

	<-done
	<-actions // drain ActionQuit injected by ForceQuit

	if h.Mode() != ModeQuitting {
		t.Errorf("expected ModeQuitting after signal-path ForceQuit, got %v", h.Mode())
	}
	if h.ShortcutLine() != QuittingLine {
		t.Errorf("expected QuittingLine after signal-path ForceQuit, got %q", h.ShortcutLine())
	}
}

// TP-002: ForceQuit may be called concurrently from different goroutines
// (TUI event loop vs. OS signal handler). Run with -race.
func TestForceQuit_ConcurrentAccess_NoRace(t *testing.T) {
	// Capacity large enough to absorb all non-blocking ForceQuit sends without
	// the goroutines ever blocking on the channel.
	actions := make(chan StepAction, 2000)
	h := NewKeyHandler(func() {}, actions)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		modes := []Mode{ModeQuitConfirm, ModeNextConfirm, ModeDone}
		for i := 0; i < 500; i++ {
			h.SetMode(modes[i%len(modes)])
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			h.ForceQuit()
		}
	}()

	wg.Wait()
	// Pass means: no panic and go test -race found no data races.
}
