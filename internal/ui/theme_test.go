package ui

import (
	"strings"
	"testing"
)

// ── AllThemes / FindTheme ─────────────────────────────────────────────────────

func TestAllThemesReturns12(t *testing.T) {
	themes := AllThemes()
	if len(themes) != 12 {
		t.Fatalf("expected 12 themes, got %d", len(themes))
	}
}

func TestAllThemesUniqueNames(t *testing.T) {
	seen := map[string]bool{}
	for _, th := range AllThemes() {
		if seen[th.Name] {
			t.Fatalf("duplicate theme name: %q", th.Name)
		}
		seen[th.Name] = true
	}
}

func TestFindThemeReturnsCorrectTheme(t *testing.T) {
	for _, th := range AllThemes() {
		found := FindTheme(th.Name)
		if found.Name != th.Name {
			t.Fatalf("FindTheme(%q) = %q", th.Name, found.Name)
		}
	}
}

func TestFindThemeFallbackToMatrix(t *testing.T) {
	th := FindTheme("DoesNotExistXYZ")
	if th.Name != "Matrix" {
		t.Fatalf("expected Matrix fallback, got %q", th.Name)
	}
}

func TestAllThemesPaneBgNonEmpty(t *testing.T) {
	for _, th := range AllThemes() {
		if th.PaneBg == "" {
			t.Fatalf("theme %q has empty PaneBg", th.Name)
		}
		if th.PaneFg == "" {
			t.Fatalf("theme %q has empty PaneFg", th.Name)
		}
	}
}

func TestMatrixPaletteLength(t *testing.T) {
	for _, th := range AllThemes() {
		p := th.MatrixPalette()
		for i, v := range p {
			if v == "" {
				t.Fatalf("theme %q palette[%d] is empty", th.Name, i)
			}
		}
	}
}

// ── Style builders ────────────────────────────────────────────────────────────

func TestStyleBuildersProduceANSI(t *testing.T) {
	SetTheme(ThemeMatrix)
	// All style builders should produce non-empty strings.
	cases := []struct {
		name string
		fn   func() string
	}{
		{"GoodStyle", func() string { return ThemeMatrix.GoodStyle().Render("x") }},
		{"WarnStyle", func() string { return ThemeMatrix.WarnStyle().Render("x") }},
		{"ErrStyle", func() string { return ThemeMatrix.ErrStyle().Render("x") }},
		{"MutedStyle", func() string { return ThemeMatrix.MutedStyle().Render("x") }},
		{"InfoStyle", func() string { return ThemeMatrix.InfoStyle().Render("x") }},
		{"TitleStyle", func() string { return ThemeMatrix.TitleStyle().Render("x") }},
		{"TitleFocusStyle", func() string { return ThemeMatrix.TitleFocusStyle().Render("x") }},
		{"PaneStyle", func() string { return ThemeMatrix.PaneStyle().Render("x") }},
	}
	for _, tc := range cases {
		got := tc.fn()
		if got == "" {
			t.Errorf("%s produced empty string", tc.name)
		}
		// All should contain the input character
		if !strings.Contains(got, "x") {
			t.Errorf("%s lost the input character: %q", tc.name, got)
		}
	}
}

func TestAllThemesStyleBuildersNonEmpty(t *testing.T) {
	for _, th := range AllThemes() {
		if th.GoodStyle().Render("ok") == "" {
			t.Fatalf("theme %q GoodStyle empty", th.Name)
		}
		if th.WarnStyle().Render("ok") == "" {
			t.Fatalf("theme %q WarnStyle empty", th.Name)
		}
		if th.ErrStyle().Render("ok") == "" {
			t.Fatalf("theme %q ErrStyle empty", th.Name)
		}
		if th.InfoStyle().Render("ok") == "" {
			t.Fatalf("theme %q InfoStyle empty", th.Name)
		}
		if th.MatrixPalette()[4] == "" {
			t.Fatalf("theme %q MatrixPalette head empty", th.Name)
		}
	}
}
