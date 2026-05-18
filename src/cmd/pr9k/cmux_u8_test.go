package main

import (
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/uichrome"
	"github.com/mxriverlynn/pr9k/src/internal/version"
)

// ---------------------------------------------------------------------------
// D-22: SetStatusLineActive(true) called in footer renderer constructor
// ---------------------------------------------------------------------------

// TestCmuxFooterRenderer_SetStatusLineActive_CalledInInit verifies that
// newCmuxFooterRenderer unconditionally calls SetStatusLineActive(true) on the
// KeyHandler so the ? key always opens the inline help expansion (D-22).
func TestCmuxFooterRenderer_SetStatusLineActive_CalledInInit(t *testing.T) {
	r := newCmuxFooterRenderer()
	if !r.keyHandler.StatusLineActive() {
		t.Error("newCmuxFooterRenderer must call SetStatusLineActive(true) on the KeyHandler (D-22)")
	}
}

// ---------------------------------------------------------------------------
// D-12: ? toggles inline help expansion; esc collapses it
// ---------------------------------------------------------------------------

// TestCmuxFooterRenderer_QuestionMark_TogglesHelpOn verifies that ? sets
// helpExpanded=true.
func TestCmuxFooterRenderer_QuestionMark_TogglesHelpOn(t *testing.T) {
	r := newCmuxFooterRenderer()
	r.SetSize(80, 24)
	r.HandleKey("?")
	if !r.helpExpanded {
		t.Error("expected helpExpanded=true after pressing '?'")
	}
}

// TestCmuxFooterRenderer_Esc_CollapsesHelp verifies that esc collapses the
// inline help expansion.
func TestCmuxFooterRenderer_Esc_CollapsesHelp(t *testing.T) {
	r := newCmuxFooterRenderer()
	r.SetSize(80, 24)
	r.HandleKey("?")
	r.HandleKey("esc")
	if r.helpExpanded {
		t.Error("expected helpExpanded=false after pressing 'esc'")
	}
}

// TestCmuxFooterRenderer_HelpExpanded_RenderIsNonEmpty verifies that Render
// returns a non-empty string when help is expanded above threshold.
func TestCmuxFooterRenderer_HelpExpanded_RenderIsNonEmpty(t *testing.T) {
	r := newCmuxFooterRenderer()
	r.SetSize(uichrome.MinTerminalWidth, uichrome.MinTerminalHeight)
	r.HandleKey("?")
	got := r.Render("")
	if got == "" {
		t.Error("Render must return non-empty string when help is expanded")
	}
}

// TestCmuxFooterRenderer_HelpExpanded_ShortPane_DoesNotPanic verifies that
// help expansion on a very short pane (just above threshold) does not panic
// and returns a non-empty string (D-12: bounded to available pane height).
func TestCmuxFooterRenderer_HelpExpanded_ShortPane_DoesNotPanic(t *testing.T) {
	r := newCmuxFooterRenderer()
	// Exactly at threshold — just enough height.
	r.SetSize(uichrome.MinTerminalWidth, uichrome.MinTerminalHeight)
	r.HandleKey("?")
	got := r.Render("")
	if got == "" {
		t.Error("Render must return non-empty string even for minimum-size pane")
	}
}

// ---------------------------------------------------------------------------
// D-18: version label "pr9k v<version.Version>" in footer corner
// ---------------------------------------------------------------------------

// TestCmuxFooterRenderer_VersionLabel_MatchesVersion verifies that the rendered
// output contains the version label in the expected format (D-18).
func TestCmuxFooterRenderer_VersionLabel_MatchesVersion(t *testing.T) {
	r := newCmuxFooterRenderer()
	r.SetSize(80, 24)
	got := r.Render("")
	want := "pr9k v" + version.Version
	if !strings.Contains(got, want) {
		t.Errorf("expected render to contain %q; got:\n%s", want, got)
	}
}

// ---------------------------------------------------------------------------
// D-11: minimum-size advisory below 60×16
// ---------------------------------------------------------------------------

// TestCmuxFooterRenderer_MinSizeAdvisory_BelowThreshold_Width verifies that
// the footer renders the minimum-size advisory when width is below 60 (D-11).
func TestCmuxFooterRenderer_MinSizeAdvisory_BelowThreshold_Width(t *testing.T) {
	r := newCmuxFooterRenderer()
	r.SetSize(uichrome.MinTerminalWidth-1, uichrome.MinTerminalHeight)
	got := r.Render("")
	if !strings.Contains(got, "≥") {
		t.Errorf("expected min-size advisory below width threshold; got: %q", got)
	}
	if !strings.Contains(got, "60") {
		t.Errorf("advisory must mention minimum width 60; got: %q", got)
	}
}

// TestCmuxFooterRenderer_MinSizeAdvisory_BelowThreshold_Height verifies that
// the footer renders the minimum-size advisory when height is below 16.
func TestCmuxFooterRenderer_MinSizeAdvisory_BelowThreshold_Height(t *testing.T) {
	r := newCmuxFooterRenderer()
	r.SetSize(uichrome.MinTerminalWidth, uichrome.MinTerminalHeight-1)
	got := r.Render("")
	if !strings.Contains(got, "≥") {
		t.Errorf("expected min-size advisory below height threshold; got: %q", got)
	}
}

// TestCmuxFooterRenderer_MinSizeAdvisory_AtThreshold_NoAdvisory verifies that
// the footer renders normally at exactly 60×16 (threshold is exclusive).
func TestCmuxFooterRenderer_MinSizeAdvisory_AtThreshold_NoAdvisory(t *testing.T) {
	r := newCmuxFooterRenderer()
	r.SetSize(uichrome.MinTerminalWidth, uichrome.MinTerminalHeight)
	got := r.Render("")
	if strings.Contains(got, "make this pane") {
		t.Errorf("must NOT show advisory at threshold size; got: %q", got)
	}
}

// ---------------------------------------------------------------------------
// D-11: minimum-size advisory for the other three panes
// ---------------------------------------------------------------------------

// TestCmuxHeaderPaneRender_MinSizeAdvisory_BelowThreshold verifies that the
// header pane render function shows the advisory below 60×16.
func TestCmuxHeaderPaneRender_MinSizeAdvisory_BelowThreshold(t *testing.T) {
	got := renderCmuxHeaderPaneContent(uichrome.MinTerminalWidth-1, uichrome.MinTerminalHeight)
	if !strings.Contains(got, "≥") {
		t.Errorf("header pane must show min-size advisory below threshold; got: %q", got)
	}
}

// TestCmuxHeaderPaneRender_AtThreshold_NoAdvisory verifies that the header
// pane render function does NOT show the advisory at the exact threshold.
func TestCmuxHeaderPaneRender_AtThreshold_NoAdvisory(t *testing.T) {
	got := renderCmuxHeaderPaneContent(uichrome.MinTerminalWidth, uichrome.MinTerminalHeight)
	if strings.Contains(got, "make this pane") {
		t.Errorf("header pane must NOT show advisory at threshold; got: %q", got)
	}
}

// TestCmuxLogPaneRender_MinSizeAdvisory_BelowThreshold verifies that the log
// pane render function shows the advisory below 60×16.
func TestCmuxLogPaneRender_MinSizeAdvisory_BelowThreshold(t *testing.T) {
	got := renderCmuxLogPaneContent(uichrome.MinTerminalWidth-1, uichrome.MinTerminalHeight)
	if !strings.Contains(got, "≥") {
		t.Errorf("log pane must show min-size advisory below threshold; got: %q", got)
	}
}

// TestCmuxLogPaneRender_AtThreshold_NoAdvisory verifies that the log pane
// render function does NOT show the advisory at the exact threshold.
func TestCmuxLogPaneRender_AtThreshold_NoAdvisory(t *testing.T) {
	got := renderCmuxLogPaneContent(uichrome.MinTerminalWidth, uichrome.MinTerminalHeight)
	if strings.Contains(got, "make this pane") {
		t.Errorf("log pane must NOT show advisory at threshold; got: %q", got)
	}
}

// TestCmuxOrchestratorPaneRender_MinSizeAdvisory_BelowThreshold verifies that
// the orchestrator pane render function shows the advisory below 60×16.
func TestCmuxOrchestratorPaneRender_MinSizeAdvisory_BelowThreshold(t *testing.T) {
	got := renderCmuxOrchestratorPaneContent(uichrome.MinTerminalWidth-1, uichrome.MinTerminalHeight)
	if !strings.Contains(got, "≥") {
		t.Errorf("orchestrator pane must show min-size advisory below threshold; got: %q", got)
	}
}

// TestCmuxOrchestratorPaneRender_AtThreshold_NoAdvisory verifies that the
// orchestrator pane render function does NOT show the advisory at threshold.
func TestCmuxOrchestratorPaneRender_AtThreshold_NoAdvisory(t *testing.T) {
	got := renderCmuxOrchestratorPaneContent(uichrome.MinTerminalWidth, uichrome.MinTerminalHeight)
	if strings.Contains(got, "make this pane") {
		t.Errorf("orchestrator pane must NOT show advisory at threshold; got: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Behavior gap tests (recommended additions from test-plan.md)
// ---------------------------------------------------------------------------

// TestCmuxFooterRenderer_Render_NormalMode_IncludesStatusLine verifies that
// Render includes the statusLine text and version label in normal (non-expanded) mode.
func TestCmuxFooterRenderer_Render_NormalMode_IncludesStatusLine(t *testing.T) {
	r := newCmuxFooterRenderer()
	r.SetSize(80, 24)
	got := r.Render("[15:04] step 3/8")
	if !strings.Contains(got, "[15:04] step 3/8") {
		t.Errorf("expected render to contain statusLine text; got: %q", got)
	}
	if !strings.Contains(got, "pr9k v"+version.Version) {
		t.Errorf("expected render to contain version label; got: %q", got)
	}
}

// TestCmuxFooterRenderer_HelpExpanded_SuppressesStatusLine verifies that when
// help is expanded, the statusLine content is suppressed and help text is shown.
func TestCmuxFooterRenderer_HelpExpanded_SuppressesStatusLine(t *testing.T) {
	r := newCmuxFooterRenderer()
	r.SetSize(80, 24)
	r.HandleKey("?")
	got := r.Render("step 3/8")
	if !strings.Contains(got, "quit") {
		t.Errorf("expected help content (quit) while expanded; got: %q", got)
	}
	if strings.Contains(got, "step 3/8") {
		t.Errorf("statusLine must be suppressed while help is expanded; got: %q", got)
	}
	if !strings.Contains(got, "pr9k v"+version.Version) {
		t.Errorf("expected version label while help is expanded; got: %q", got)
	}
}

// TestCmuxFooterRenderer_MinSizeAdvisory_NoVersionLabel verifies that below
// the minimum-size threshold, the advisory is the sole output — no version label.
func TestCmuxFooterRenderer_MinSizeAdvisory_NoVersionLabel(t *testing.T) {
	r := newCmuxFooterRenderer()
	r.SetSize(uichrome.MinTerminalWidth-1, uichrome.MinTerminalHeight)
	got := r.Render("anything")
	if strings.Contains(got, "pr9k v") {
		t.Errorf("version label must not appear in advisory mode; got: %q", got)
	}
	if got != cmuxMinSizeAdvisory() {
		t.Errorf("expected advisory-only output; got: %q", got)
	}
}
