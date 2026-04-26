package workflowedit

// Tests for WU-5 session-header render (issue #188).
// D5 5-slot layout: title · dirty indicator · banner · [N more warnings] · findings summary.
//
// D# → test-name coverage matrix:
//
//	D13  dirty indicator ● in Green:           TestSessionHeader_DirtyIndicatorWhenDirty
//	D13  browse-only suppresses dirty:         TestSessionHeader_DirtyIndicatorSuppressedWhenReadOnly
//	D14  banner priority chain:                TestSessionHeader_BannerPriority
//	D16  findings summary format:              TestSessionHeader_FindingsSummaryFormat
//	WU-5 validation indicator states:          TestSessionHeader_ValidationIndicator
//	D17  overflow priority order:              TestSessionHeader_OverflowPriority

import (
	"strings"
	"testing"
)

// TestSessionHeader_DirtyIndicatorWhenDirty verifies ● appears in the session header
// when IsDirty() is true (D13). Uses IsDirty(), not m.dirty cache (D-11).
func TestSessionHeader_DirtyIndicatorWhenDirty(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	// Make IsDirty() return true by diverging doc from diskDoc.
	m.doc.Steps[0].Name = "modified"
	m.dirty = false // stale cache — render must ignore this

	got := stripStr(m.renderSessionHeader())
	if !strings.Contains(got, "●") {
		t.Errorf("dirty indicator ● not found in session header %q", got)
	}
}

// TestSessionHeader_DirtyIndicatorSuppressedWhenReadOnly verifies that browse-only
// mode (banners.isReadOnly) suppresses the dirty indicator (D18).
func TestSessionHeader_DirtyIndicatorSuppressedWhenReadOnly(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.doc.Steps[0].Name = "modified" // IsDirty() is true
	m.banners.isReadOnly = true

	got := stripStr(m.renderSessionHeader())
	if strings.Contains(got, "●") {
		t.Errorf("dirty indicator should be suppressed when read-only, got %q", got)
	}
}

// TestSessionHeader_BannerPriority verifies the D14 priority chain: when multiple
// banners are asserted, the highest-priority one (read-only > ext > sym > ...) wins
// and the suppressed count appears as "[N more warnings]" (D15).
func TestSessionHeader_BannerPriority(t *testing.T) {
	m := newTestModel()
	m.banners = bannerState{
		isReadOnly:         true,
		isExternalWorkflow: true,
		isSymlink:          true,
	}

	got := stripStr(m.renderSessionHeader())

	if !strings.Contains(got, "[ro]") {
		t.Errorf("banner priority: [ro] should appear as primary banner, got %q", got)
	}
	if !strings.Contains(got, "more warnings") {
		t.Errorf("banner priority: [N more warnings] affordance missing, got %q", got)
	}
	// [ext] should not appear as an independent banner (it is suppressed by [ro] priority).
	if strings.HasPrefix(got, "[ext]") {
		t.Errorf("banner priority: [ext] should not be primary banner when [ro] is active, got %q", got)
	}
}

// TestSessionHeader_FindingsSummaryFormat verifies the D16 format string:
// "<F> fatal · <W> warn" for non-zero counts, empty for zero.
func TestSessionHeader_FindingsSummaryFormat(t *testing.T) {
	m := newTestModel()
	m.findingsPanel.entries = []findingEntry{
		{text: "config error: schema: step model is required", isFatal: true},
		{text: "config error: schema: another fatal", isFatal: true},
		{text: "config error: schema: third fatal", isFatal: true},
		{text: "config warning: something minor", isFatal: false},
	}

	got := stripStr(m.renderSessionHeader())

	if !strings.Contains(got, "3 fatal") {
		t.Errorf("findings summary: want '3 fatal', got %q", got)
	}
	if !strings.Contains(got, "1 warn") {
		t.Errorf("findings summary: want '1 warn', got %q", got)
	}

	// Zero-findings case: summary should be empty (no "0 fatal" noise).
	m2 := newTestModel()
	got2 := stripStr(m2.renderSessionHeader())
	if strings.Contains(got2, "fatal") || strings.Contains(got2, "warn") {
		t.Errorf("findings summary: should be absent when zero findings, got %q", got2)
	}
}

// TestSessionHeader_ValidationIndicator verifies the three validation states (WU-5):
// "Validating…" during validation, "Validated ✓" after pass, "Validation failed" after fail.
func TestSessionHeader_ValidationIndicator(t *testing.T) {
	// State 1: Validating…
	m := newTestModel()
	m.validateInProgress = true
	got := stripStr(m.renderSessionHeader())
	if !strings.Contains(got, "Validating") {
		t.Errorf("validation indicator: want 'Validating' during validation, got %q", got)
	}

	// State 2: Validated ✓
	ok := true
	m2 := newTestModel()
	m2.lastValidateOK = &ok
	got2 := stripStr(m2.renderSessionHeader())
	if !strings.Contains(got2, "Validated") {
		t.Errorf("validation indicator: want 'Validated' after passing validation, got %q", got2)
	}

	// State 3: Validation failed
	notOK := false
	m3 := newTestModel()
	m3.lastValidateOK = &notOK
	got3 := stripStr(m3.renderSessionHeader())
	if !strings.Contains(got3, "Validation failed") {
		t.Errorf("validation indicator: want 'Validation failed' after failing validation, got %q", got3)
	}

	// No indicator before any validation has run.
	m4 := newTestModel()
	// m4.lastValidateOK is nil by default
	got4 := stripStr(m4.renderSessionHeader())
	if strings.Contains(got4, "Validat") {
		t.Errorf("validation indicator: should be absent before any validation, got %q", got4)
	}
}

// TestSessionHeader_OverflowPriority verifies D17: when the row is too narrow to fit
// all slots, [N more warnings] is dropped before the findings summary, and the primary
// banner remains visible.
func TestSessionHeader_OverflowPriority(t *testing.T) {
	// Use width=24 (innerW=22): narrow enough that [N more warnings] (~18 chars) cannot
	// fit alongside the path (13), dirty (2), and banner (5), but the banner alone fits.
	m := newLoadedModelWithWidth(24, 24, sampleStep("s1"))
	m.doc.Steps[0].Name = "modified" // dirty
	m.banners = bannerState{
		isReadOnly: true,
		isSymlink:  true, // two banners → [1 more warnings] at full width
	}
	m.findingsPanel.entries = []findingEntry{{text: "err", isFatal: true}}

	got := stripStr(m.renderSessionHeader())

	// Primary banner must still be present.
	if !strings.Contains(got, "[ro]") {
		t.Errorf("overflow: primary banner [ro] must remain at narrow width, got %q", got)
	}

	// [N more warnings] must be dropped (D17 step 1) at this narrow width.
	// The full [1 more warnings] string is 17 chars; at innerW=22 with path+dirty+banner=21,
	// there is no room for it.
	if strings.Contains(got, "more warnings") {
		t.Errorf("overflow: [N more warnings] should be dropped at narrow width, got %q", got)
	}
}
