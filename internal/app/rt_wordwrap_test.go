package app

import (
	"strings"
	"testing"
	"github.com/NamasteJasutin/screener/internal/ui"
)

// RT: Verify whether lipgloss word-wraps profile names that exceed pane content width,
// pushing the scroll indicator off-screen.
func TestRT_LipglossWordWrapsLongProfileNames(t *testing.T) {
	// At m.width=92: leftW=28, leftInnerW=24, content area=24 cols
	// Profile name "TV Console - Main Display 21" = 28 chars
	// "  " prefix + 28 char name = 30 chars > 24 → lipgloss word-wraps

	longName := "TV Console - Main Display 21"  // 28 chars
	prefix := "  "
	line := prefix + longName  // 30 visible chars

	t.Logf("line visible len: %d", len([]rune(line)))
	t.Logf("pane content width at m.width=92: 24 cols")

	// Simulate what renderPane does: join lines, render with Width(24)
	bodyLines := []string{line, "indicator would be here"}
	rendered := ui.RenderPane("Title", bodyLines, 24, 6)
	plain := stripANSIForTest(rendered)
	t.Logf("rendered pane (plain):\n%s", plain)

	// If lipgloss wraps the long line, "indicator" will be absent or truncated
	if !strings.Contains(plain, "indicator") {
		t.Errorf("RT-WORDWRAP CONFIRMED: long profile name caused lipgloss word-wrap that displaced indicator")
		t.Errorf("This is a real bug: profile names must be truncated to fit pane content width")
	}
}
