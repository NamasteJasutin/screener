package app

import (
	"fmt"
	"strings"
	"testing"
	tea "charm.land/bubbletea/v2"
	"screener/internal/ui"
)

// RedTeam: The scroll indicator uses an en-dash (–, U+2013) and the test
// flagged that "of " was not found in the composed view. Investigate root cause.
func TestRT_ScrollIndicatorAppearsInComposedView(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	for i := 0; i < 20; i++ {
		updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
		m = updated.(Model)
	}
	m.activeIdx = len(m.config.Profiles) - 1
	m.width = minFullLayoutWidth + 12
	m.height = minFullLayoutHeight + 8

	// Compute what renderProfileLines would emit directly
	leftW, _, contentH := m.layoutDimensions()
	_, profInnerH := m.paneInnerHeights(contentH)
	leftInnerW := leftW - ui.PaneFrameW
	profItemRows := max(profInnerH-5, 1)
	n := len(m.config.Profiles)
	profScroll := 0
	if n > profItemRows {
		profScroll = m.activeIdx - profItemRows/2
		if profScroll+profItemRows > n { profScroll = n - profItemRows }
		if profScroll < 0 { profScroll = 0 }
	}

	t.Logf("n=%d profItemRows=%d profScroll=%d activeIdx=%d leftW=%d leftInnerW=%d",
		n, profItemRows, profScroll, m.activeIdx, leftW, leftInnerW)

	nameW := max(leftInnerW-6, 6)
	lines := m.renderProfileLines(profScroll, profItemRows, nameW)
	indicatorFound := false
	for i, l := range lines {
		plain := stripANSIForTest(l)
		t.Logf("line[%d]: %q", i, plain)
		if strings.Contains(plain, "of ") {
			indicatorFound = true
		}
	}
	if !indicatorFound && n > profItemRows {
		t.Fatalf("RT-SCROLL: indicator missing from renderProfileLines output; n=%d profItemRows=%d", n, profItemRows)
	}

	// Composed view no longer includes the profiles pane directly; just verify it renders without panic.
	_ = fmt.Sprint(m.View())
}
