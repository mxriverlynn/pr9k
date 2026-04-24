package workflowedit

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestViewport_OutlineWidth_MinimumOf20 verifies the outline is never narrower than 20.
func TestViewport_OutlineWidth_MinimumOf20(t *testing.T) {
	// 40% of 40 = 16, below the minimum.
	got := outlineWidth(40)
	if got != 20 {
		t.Errorf("outlineWidth(40) = %d, want 20 (minimum)", got)
	}
	// 40% of 0 = 0, still minimum.
	got = outlineWidth(0)
	if got != 20 {
		t.Errorf("outlineWidth(0) = %d, want 20 (minimum)", got)
	}
}

// TestViewport_DetailWidth_RemainingAfterOutline verifies the detail takes the rest.
func TestViewport_DetailWidth_RemainingAfterOutline(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	const totalWidth = 100
	next, _ := m.Update(tea.WindowSizeMsg{Width: totalWidth, Height: 25})
	got := next.(Model)
	ow := outlineWidth(totalWidth) // should be 40
	wantDetail := totalWidth - ow  // 60
	if got.detail.width != wantDetail {
		t.Errorf("detail width = %d, want %d", got.detail.width, wantDetail)
	}
}

// TestViewport_OutlineWidth_CappedAt40 verifies the outline is never wider than 40.
func TestViewport_OutlineWidth_CappedAt40(t *testing.T) {
	got := outlineWidth(200)
	if got != 40 {
		t.Errorf("outlineWidth(200) = %d, want 40 (maximum)", got)
	}
}

// TestMouse_Wheel_LeftColumn_ScrollsOutline verifies pointer-side routing.
func TestMouse_Wheel_LeftColumn_ScrollsOutline(t *testing.T) {
	m := newLoadedModelWithWidth(100, 25, sampleStep("a"))
	// Outline occupies columns 0..39 for width=100.
	msg := tea.MouseMsg{X: 10, Button: tea.MouseButtonWheelDown}
	got := applyMsg(m, msg)
	if got.outline.scrolls < 1 {
		t.Error("outline.scrolls should increment for X=10 (left column)")
	}
	if got.detail.scrolls != 0 {
		t.Error("detail.scrolls should not change for X=10")
	}
}

// TestMouse_Wheel_RightColumn_ScrollsDetail verifies pointer-side routing.
func TestMouse_Wheel_RightColumn_ScrollsDetail(t *testing.T) {
	m := newLoadedModelWithWidth(100, 25, sampleStep("a"))
	// Detail occupies columns 40..99 for width=100.
	msg := tea.MouseMsg{X: 60, Button: tea.MouseButtonWheelDown}
	got := applyMsg(m, msg)
	if got.detail.scrolls < 1 {
		t.Error("detail.scrolls should increment for X=60 (right column)")
	}
	if got.outline.scrolls != 0 {
		t.Error("outline.scrolls should not change for X=60")
	}
}
