package render

import (
	"regexp"
	"strings"
	"testing"
)

func TestComposeRendersLineBasedOverlayWithoutCursorPositioning(t *testing.T) {
	out := Compose(5, 2, ".....\n.....", OverlayBlock{X: 2, Y: 1, Content: "AB"})

	if regexp.MustCompile(`\x1b\[[0-9]+;[0-9]+H`).MatchString(out) {
		t.Fatalf("unexpected cursor positioning escape in output: %q", out)
	}

	want := "\x1b[H.AB..\n....."
	if out != want {
		t.Fatalf("unexpected composed output: got %q want %q", out, want)
	}
}

func TestComposeXGreaterThanWidthDrawsNothing(t *testing.T) {
	out := Compose(5, 1, ".....", OverlayBlock{X: 6, Y: 1, Content: "ABC"})

	if out != "\x1b[H....." {
		t.Fatalf("unexpected output when X > width: %q", out)
	}
}

func TestComposeXLessThanOneLeftCrops(t *testing.T) {
	out := Compose(5, 1, ".....", OverlayBlock{X: -1, Y: 1, Content: "ABCDE"})
	want := "\x1b[HCDE.."

	if out != want {
		t.Fatalf("unexpected left-cropped output: got %q want %q", out, want)
	}
}

func TestComposeOutputWidthNeverExceedsAvailableColumns(t *testing.T) {
	out := Compose(4, 1, "....", OverlayBlock{
		X:       3,
		Y:       1,
		Content: "\x1b[32mABCDE\x1b[0m",
	})

	if got := stripANSI(strings.TrimPrefix(out, "\x1b[H")); got != "..AB" {
		t.Fatalf("overlay exceeds available width or wrong clip: got %q", got)
	}
	if !strings.Contains(out, "\x1b[32m") {
		t.Fatalf("expected styling escapes to be preserved in clipped output: %q", out)
	}
}

func TestComposePreservesBackgroundAndOverlayStyling(t *testing.T) {
	out := Compose(5, 1, "\x1b[34mabc\x1b[0m..", OverlayBlock{X: 2, Y: 1, Content: "\x1b[32mXY\x1b[0m"})

	if !strings.Contains(out, "\x1b[34m") {
		t.Fatalf("expected background styling to be preserved: %q", out)
	}
	if !strings.Contains(out, "\x1b[32m") {
		t.Fatalf("expected overlay styling to be preserved: %q", out)
	}
	if got := stripANSI(strings.TrimPrefix(out, "\x1b[H")); got != "aXY.." {
		t.Fatalf("unexpected composed visible content: got %q", got)
	}
}

func TestComposeANSIContentRemainsSequenceSafe(t *testing.T) {
	out := Compose(3, 1, "\x1b[31mabc\x1b[0m", OverlayBlock{X: 2, Y: 1, Content: "\x1b[32mXYZ\x1b[0m"})
	assertNoBrokenANSISequences(t, out)
}

func assertNoBrokenANSISequences(t *testing.T, s string) {
	t.Helper()
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			i++
			continue
		}
		next, ok := ansiSequenceEnd(s, i)
		if !ok || next <= i {
			t.Fatalf("broken ANSI sequence at byte %d in %q", i, s)
		}
		i = next
	}
}
