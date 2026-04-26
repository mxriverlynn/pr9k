// Package workflowedit — visual chrome render tests.
//
// D# → test-name coverage matrix (expanded per commit; see feature-implementation-plan.md):
//
//	D-6  ANSI-stripped substring strategy:  (all render tests in this package use stripView)
//	D-7  bannerGen generation counter:      TestBannerGen_GenerationCounter
//	D-8  nowFn time-injection seam:          TestNowFn_InjectionSeam
//
// Subsequent commits add entries for D-1..D-51 as each surface lands.
package workflowedit

import (
	"testing"
	"time"
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
