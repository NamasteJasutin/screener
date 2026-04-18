package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestRenderPaneSmallSizesStable(t *testing.T) {
	testCases := []struct {
		name   string
		width  int
		height int
	}{
		{name: "one by one", width: 1, height: 1},
		{name: "two by two", width: 2, height: 2},
		{name: "narrow and short", width: 3, height: 2},
		{name: "zero size", width: 0, height: 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var out1, out2 string
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("RenderPane panicked for %dx%d: %v", tc.width, tc.height, r)
					}
				}()
				out1 = RenderPane("T", []string{"line"}, tc.width, tc.height)
				out2 = RenderPane("T", []string{"line"}, tc.width, tc.height)
			}()

			if out1 != out2 {
				t.Fatalf("RenderPane output is not stable for %dx%d", tc.width, tc.height)
			}

			const frameWidth = 2 /* border */ + 2 /* horizontal padding */
			const frameHeight = 2                 /* border */

			naturalContentWidth := lipgloss.Width("line")
			naturalContentHeight := 2 // title + at least one body row

			expectedWidth := max(tc.width, naturalContentWidth+frameWidth)
			expectedHeight := max(tc.height, naturalContentHeight+frameHeight)

			if got := lipgloss.Width(out1); got != expectedWidth {
				t.Fatalf("expected constrained width %d, got %d", expectedWidth, got)
			}
			if got := lipgloss.Height(out1); got != expectedHeight {
				t.Fatalf("expected constrained height %d, got %d", expectedHeight, got)
			}
		})
	}
}

func TestRenderPaneTruncatesBodyToHeight(t *testing.T) {
	out := RenderPane("Logs", []string{"one", "two", "three", "four"}, 40, 5)

	for _, want := range []string{"one", "two", "three"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q", want)
		}
	}
	if strings.Contains(out, "four") {
		t.Fatalf("expected output to truncate body line %q", "four")
	}
}

func TestRenderPaneFillsMissingBodyLines(t *testing.T) {
	width, height := 32, 6
	outShort := RenderPane("Title", []string{"one"}, width, height)
	outPadded := RenderPane("Title", []string{"one", "", "", ""}, width, height)

	if outShort != outPadded {
		t.Fatalf("expected missing body lines to be padded deterministically")
	}
}

func TestRenderPaneUsesPlaceholderForEmptyBody(t *testing.T) {
	out := RenderPane("Title", nil, 24, 4)
	if !strings.Contains(out, "-") {
		t.Fatalf("expected placeholder line %q for empty body", "-")
	}
}
