package render

import (
	"strings"
	"testing"
)

// ── ansiSequenceEnd edge cases ────────────────────────────────────────────────

func TestAnsiSequenceEndCSIComplete(t *testing.T) {
	// \x1b[32m: ESC '[' '3' '2' 'm'(finalizer) -> end = index after 'm' = 5
	s := "\x1b[32mX"
	end, ok := ansiSequenceEnd(s, 0)
	if !ok {
		t.Fatal("expected ok for CSI sequence")
	}
	if end != 5 {
		t.Fatalf("expected end=5 (past finalizer 'm'), got %d", end)
	}
}

func TestAnsiSequenceEndCSIIncomplete(t *testing.T) {
	// ESC [ without finalizer — should return false
	s := "\x1b[32"
	_, ok := ansiSequenceEnd(s, 0)
	if ok {
		t.Fatal("expected !ok for incomplete CSI sequence")
	}
}

func TestAnsiSequenceEndOSCWithBEL(t *testing.T) {
	// ESC ] 0 ; title BEL
	s := "\x1b]0;title\x07rest"
	end, ok := ansiSequenceEnd(s, 0)
	if !ok {
		t.Fatal("expected ok for OSC sequence with BEL")
	}
	if end != 10 {
		t.Fatalf("expected end=10, got %d (s=%q)", end, s)
	}
}

func TestAnsiSequenceEndOSCWithST(t *testing.T) {
	// ESC ] content ESC \  (String Terminator)
	s := "\x1b]title\x1b\\rest"
	end, ok := ansiSequenceEnd(s, 0)
	if !ok {
		t.Fatal("expected ok for OSC sequence with ST")
	}
	if end != 9 {
		t.Fatalf("expected end=9, got %d (s=%q)", end, s)
	}
}

func TestAnsiSequenceEndOSCIncomplete(t *testing.T) {
	// Unterminated OSC
	s := "\x1b]0;title"
	_, ok := ansiSequenceEnd(s, 0)
	if ok {
		t.Fatal("expected !ok for unterminated OSC")
	}
}

func TestAnsiSequenceEndOtherLead(t *testing.T) {
	// ESC A (Fe sequence) — 2-byte
	s := "\x1b=rest"
	end, ok := ansiSequenceEnd(s, 0)
	if !ok {
		t.Fatal("expected ok for 2-byte escape")
	}
	if end != 2 {
		t.Fatalf("expected end=2, got %d", end)
	}
}

func TestAnsiSequenceEndShortString(t *testing.T) {
	// Only ESC at end — can't have lead
	s := "\x1b"
	_, ok := ansiSequenceEnd(s, 0)
	if ok {
		t.Fatal("expected !ok for lone ESC at end")
	}
}

func TestAnsiSequenceEndNotAtStart(t *testing.T) {
	// Position not at ESC
	s := "ABC\x1b[31mD"
	_, ok := ansiSequenceEnd(s, 0)
	if ok {
		t.Fatal("expected !ok when not at ESC position")
	}
}

func TestParseStyledCellsWithOSC(t *testing.T) {
	// OSC sequences should be handled (accumulated as pending escape)
	input := "\x1b]0;title\x07hello"
	cells := parseStyledCells(input)
	// Should produce cells for "hello" — the OSC part may be attached to first cell
	joined := strings.Join(cells, "")
	if !strings.Contains(joined, "hello") {
		t.Fatalf("expected 'hello' in cells: %q", joined)
	}
}

func TestStripANSIWithOSC(t *testing.T) {
	input := "\x1b]0;title\x07text\x1b[32mcolor\x1b[0m"
	got := stripANSI(input)
	if !strings.Contains(got, "text") {
		t.Fatalf("stripANSI should preserve text after OSC: %q -> %q", input, got)
	}
	if !strings.Contains(got, "color") {
		t.Fatalf("stripANSI should preserve color text: %q -> %q", input, got)
	}
}

func TestComposeEmptyBackground(t *testing.T) {
	// Empty background string with overlay
	out := Compose(5, 2, "", OverlayBlock{X: 1, Y: 1, Content: "AB"})
	if !strings.Contains(out, "AB") {
		t.Fatalf("expected overlay content with empty bg: %q", out)
	}
}

func TestComposeYBeyondHeightIsClampedNotDropped(t *testing.T) {
	// Compose clamps Y to [1,height] so Y=99 with height=3 -> row=3 (last row).
	out := Compose(5, 3, ".....\n.....\n.....", OverlayBlock{X: 1, Y: 99, Content: "XY"})
	// Content is placed at last row, not dropped.
	if !strings.Contains(out, "X") {
		t.Fatalf("expected clamped overlay to appear at last row: %q", out)
	}
}

func TestComposeMultipleOverlays(t *testing.T) {
	bg := ".....\n.....\n....."
	out := Compose(5, 3, bg,
		OverlayBlock{X: 1, Y: 1, Content: "A"},
		OverlayBlock{X: 3, Y: 2, Content: "B"},
		OverlayBlock{X: 5, Y: 3, Content: "C"},
	)
	if !strings.Contains(out, "A") {
		t.Fatalf("missing overlay A: %q", out)
	}
	if !strings.Contains(out, "B") {
		t.Fatalf("missing overlay B: %q", out)
	}
	if !strings.Contains(out, "C") {
		t.Fatalf("missing overlay C: %q", out)
	}
}

func TestComposeZeroSize(t *testing.T) {
	// Zero-size canvas — must not panic
	out := Compose(0, 0, "", OverlayBlock{X: 1, Y: 1, Content: "X"})
	_ = out
}

func TestComposeEmptyOverlay(t *testing.T) {
	out := Compose(5, 2, ".....\n.....", OverlayBlock{X: 1, Y: 1, Content: ""})
	// Empty overlay — background should be unchanged
	plain := stripANSI(strings.TrimPrefix(out, "\x1b[H"))
	if plain != ".....\n....." {
		t.Fatalf("empty overlay changed background: %q", plain)
	}
}
