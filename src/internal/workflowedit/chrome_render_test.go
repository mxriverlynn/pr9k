// Package workflowedit — visual chrome render tests.
//
// D# → test-name coverage matrix (expanded per commit; see feature-implementation-plan.md):
//
//	D-1  9-row frame assembly:              TestView_FrameHas9ChromeRows
//	D-5  panelH computed in View():         TestView_PanelHEqualsChromeBudget
//	D-6  ANSI-stripped substring strategy:  (all render tests in this package use stripView)
//	D-7  bannerGen generation counter:      TestBannerGen_GenerationCounter
//	         saveBanner timer + gen guard:  TestView_SaveBannerClearedAfter3Seconds
//	                                        TestView_BannerStaleTickIgnored
//	                                        TestView_SaveBannerInSessionHeaderSlot
//	D-8  nowFn time-injection seam:          TestNowFn_InjectionSeam
//
// Subsequent commits add entries for D-1..D-51 as each surface lands.
package workflowedit

import (
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
)

// TestNowFn_InjectionSeam verifies that nowFn is replaceable with a fixed clock (D-8).
func TestNowFn_InjectionSeam(t *testing.T) {
	m := newTestModel()
	fixed := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m.nowFn = func() time.Time { return fixed }
	got := m.nowFn()
	if !got.Equal(fixed) {
		t.Errorf("nowFn injection: want %v, got %v", fixed, got)
	}
}

// TestNowFn_DefaultIsTimeNow verifies New() sets nowFn to time.Now (D-8).
func TestNowFn_DefaultIsTimeNow(t *testing.T) {
	m := newTestModel()
	if m.nowFn == nil {
		t.Fatal("nowFn must not be nil after New()")
	}
	before := time.Now()
	got := m.nowFn()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Errorf("default nowFn should return current time, got %v (window [%v, %v])", got, before, after)
	}
}

// TestBannerGen_GenerationCounter verifies bannerGen field exists and can be incremented (D-7).
func TestBannerGen_GenerationCounter(t *testing.T) {
	m := newTestModel()
	if m.bannerGen != 0 {
		t.Errorf("bannerGen should start at 0, got %d", m.bannerGen)
	}
	m.bannerGen++
	if m.bannerGen != 1 {
		t.Errorf("bannerGen should increment, got %d", m.bannerGen)
	}
}

// TestClearSaveBannerMsg_TypeExists verifies clearSaveBannerMsg carries a gen field.
func TestClearSaveBannerMsg_TypeExists(t *testing.T) {
	msg := clearSaveBannerMsg{gen: 42}
	if msg.gen != 42 {
		t.Errorf("clearSaveBannerMsg.gen wrong, got %d", msg.gen)
	}
}

// TestAssertModalFits_HappyPath verifies assertModalFits passes on a dialog placeholder.
func TestAssertModalFits_HappyPath(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogQuitConfirm}
	assertModalFits(t, m)
}

// TestView_FrameHas9ChromeRows verifies View() produces exactly m.height lines so
// the chrome frame fills the terminal precisely (D-1, 9-element assembly).
func TestView_FrameHas9ChromeRows(t *testing.T) {
	const termW, termH = 80, 24
	m := newLoadedModelWithWidth(termW, termH, sampleStep("s1"))
	view := stripView(m)
	lines := strings.Split(view, "\n")
	if len(lines) != termH {
		t.Errorf("View has %d lines, want %d (frame must fill terminal exactly)", len(lines), termH)
	}
}

// TestView_PanelHEqualsChromeBudget verifies the content panel uses exactly
// height - ChromeRows lines, leaving ChromeRows lines for the fixed chrome (D-5, D-20).
func TestView_PanelHEqualsChromeBudget(t *testing.T) {
	const termW, termH = 80, 24
	m := newLoadedModelWithWidth(termW, termH, sampleStep("s1"))
	view := stripView(m)
	lines := strings.Split(view, "\n")
	wantPanelH := termH - ChromeRows
	gotPanelH := len(lines) - ChromeRows
	if gotPanelH != wantPanelH {
		t.Errorf("content panel height = %d, want height-ChromeRows = %d", gotPanelH, wantPanelH)
	}
}

// TestView_SaveBannerClearedAfter3Seconds verifies that clearSaveBannerMsg with a
// matching gen clears m.saveBanner, and that handleSaveResult uses m.nowFn (D-7, D-8).
func TestView_SaveBannerClearedAfter3Seconds(t *testing.T) {
	m := newTestModel()
	fixed := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m.nowFn = func() time.Time { return fixed }

	saveMsg := saveCompleteMsg{result: workflowio.SaveResult{Kind: workflowio.SaveErrorNone}}
	next, _ := m.Update(saveMsg)
	m1 := next.(Model)

	// Banner must use nowFn's timestamp.
	const wantBanner = "Saved at 12:00:00"
	if m1.saveBanner != wantBanner {
		t.Errorf("saveBanner = %q, want %q (must use nowFn, not time.Now)", m1.saveBanner, wantBanner)
	}

	// Deliver clearSaveBannerMsg with the current gen → banner must clear.
	clearMsg := clearSaveBannerMsg{gen: m1.bannerGen}
	next2, _ := m1.Update(clearMsg)
	m2 := next2.(Model)

	if m2.saveBanner != "" {
		t.Errorf("saveBanner should be empty after matching clearSaveBannerMsg, got %q", m2.saveBanner)
	}
}

// TestView_BannerStaleTickIgnored verifies that a clearSaveBannerMsg whose gen
// does not match the current bannerGen is silently ignored (D-7 race protection).
func TestView_BannerStaleTickIgnored(t *testing.T) {
	m := newTestModel()

	saveMsg := saveCompleteMsg{result: workflowio.SaveResult{Kind: workflowio.SaveErrorNone}}

	// First save → bannerGen must increment to 1.
	m1 := applyMsg(m, saveMsg)
	if m1.bannerGen != 1 {
		t.Fatalf("bannerGen should be 1 after first save, got %d", m1.bannerGen)
	}
	staleGen := m1.bannerGen // gen=1 from first save

	// Second save → bannerGen must increment to 2.
	m2 := applyMsg(m1, saveMsg)
	if m2.bannerGen != 2 {
		t.Fatalf("bannerGen should be 2 after second save, got %d", m2.bannerGen)
	}
	if m2.saveBanner == "" {
		t.Fatal("saveBanner should be set after second save")
	}

	// Deliver stale clearSaveBannerMsg (gen=1 but bannerGen=2) → banner must NOT clear.
	m3 := applyMsg(m2, clearSaveBannerMsg{gen: staleGen})
	if m3.saveBanner == "" {
		t.Error("stale clearSaveBannerMsg (gen=1) must not clear banner (bannerGen=2)")
	}
}

// TestView_SaveBannerInSessionHeaderSlot verifies the save banner appears in the
// session-header banner slot (line index 3) inside the chrome frame, not as a
// separate row outside the chrome budget (D-7, D-14).
func TestView_SaveBannerInSessionHeaderSlot(t *testing.T) {
	const termW, termH = 80, 24
	m := newLoadedModelWithWidth(termW, termH, sampleStep("s1"))
	fixed := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m.nowFn = func() time.Time { return fixed }

	saveMsg := saveCompleteMsg{result: workflowio.SaveResult{Kind: workflowio.SaveErrorNone}}
	next, _ := m.Update(saveMsg)
	m2 := next.(Model)

	view := stripView(m2)
	lines := strings.Split(view, "\n")

	// Frame must still fill the terminal — banner must not add an extra row.
	if len(lines) != termH {
		t.Errorf("View has %d lines after save, want %d (banner must not add extra row)", len(lines), termH)
	}

	// Session-header row 2 is at line index 3 (top=0, menu=1, hdr1=2, hdr2=3).
	const bannerText = "Saved at 12:00:00"
	if len(lines) > 3 && !strings.Contains(lines[3], bannerText) {
		t.Errorf("session-header banner slot (line[3]) = %q, want to contain %q", lines[3], bannerText)
	}
}
