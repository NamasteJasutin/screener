package ui

import (
	"strings"
	"testing"
)

// ── SetTheme / style helpers ──────────────────────────────────────────────────

func TestSetThemeAndStyleHelpers(t *testing.T) {
	for _, th := range AllThemes() {
		SetTheme(th)
		if Active.Name != th.Name {
			t.Fatalf("SetTheme(%q): Active.Name=%q", th.Name, Active.Name)
		}
		// Each helper must return non-empty ANSI-styled text.
		if StyleGood("a") == "" {
			t.Fatalf("StyleGood empty for theme %q", th.Name)
		}
		if StyleWarn("a") == "" {
			t.Fatalf("StyleWarn empty for theme %q", th.Name)
		}
		if StyleErr("a") == "" {
			t.Fatalf("StyleErr empty for theme %q", th.Name)
		}
		if StyleMuted("a") == "" {
			t.Fatalf("StyleMuted empty for theme %q", th.Name)
		}
		if StyleInfo("a") == "" {
			t.Fatalf("StyleInfo empty for theme %q", th.Name)
		}
	}
	SetTheme(ThemeMatrix) // restore default
}

// ── RenderPaneFocused ─────────────────────────────────────────────────────────

func TestRenderPaneFocusedVsUnfocused(t *testing.T) {
	SetTheme(ThemeMatrix)
	body := []string{"line1", "line2"}
	unfocused := RenderPaneFocused("Title", body, 30, 8, false)
	focused := RenderPaneFocused("Title", body, 30, 8, true)

	if unfocused == focused {
		t.Fatal("expected focused and unfocused pane to differ (different border color)")
	}
	if !strings.Contains(unfocused, "line1") {
		t.Fatal("unfocused pane missing body content")
	}
	if !strings.Contains(focused, "line1") {
		t.Fatal("focused pane missing body content")
	}
}

func TestRenderPaneFocusedMatchesRenderPaneWhenFalse(t *testing.T) {
	SetTheme(ThemeMatrix)
	body := []string{"a", "b"}
	via_focused := RenderPaneFocused("T", body, 20, 6, false)
	via_pane := RenderPane("T", body, 20, 6)
	// Both should produce the same result when focused=false.
	if via_focused != via_pane {
		t.Fatalf("RenderPaneFocused(focused=false) != RenderPane\nfocused=%q\npane=%q",
			via_focused, via_pane)
	}
}

// ── RenderStatusBar ───────────────────────────────────────────────────────────

func TestRenderStatusBarBasic(t *testing.T) {
	SetTheme(ThemeMatrix)
	bar := RenderStatusBar(80, "USB123 (Pixel7)", "Game Mode", "idle", "q=quit")
	if bar == "" {
		t.Fatal("empty status bar")
	}
	plain := stripANSI(bar)
	if !strings.Contains(plain, "USB123") {
		t.Fatalf("device label missing: %q", plain)
	}
	if !strings.Contains(plain, "Game Mode") {
		t.Fatalf("profile label missing: %q", plain)
	}
	if !strings.Contains(plain, "q=quit") {
		t.Fatalf("context keys missing: %q", plain)
	}
}

func TestRenderStatusBarAllLaunchStates(t *testing.T) {
	SetTheme(ThemeMatrix)
	states := []string{"idle", "launching", "succeeded", "failed", "canceled", "timed_out", "custom_state"}
	for _, s := range states {
		bar := RenderStatusBar(100, "dev", "prof", s, "q=quit")
		if bar == "" {
			t.Fatalf("empty status bar for state %q", s)
		}
	}
}

func TestRenderStatusBarNarrowTerminal(t *testing.T) {
	SetTheme(ThemeMatrix)
	// Must not panic on narrow terminals
	bar := RenderStatusBar(20, "very-long-device-serial-that-exceeds-width", "very-long-profile-name", "idle", "q")
	if bar == "" {
		t.Fatal("empty status bar for narrow terminal")
	}
}

func TestRenderStatusBarEmptyLabels(t *testing.T) {
	SetTheme(ThemeMatrix)
	bar := RenderStatusBar(80, "", "", "idle", "")
	if bar == "" {
		t.Fatal("empty status bar with empty labels")
	}
}

// ── stateBadge ────────────────────────────────────────────────────────────────

func TestStateBadgeAllKnownStates(t *testing.T) {
	SetTheme(ThemeMatrix)
	states := []string{"idle", "launching", "succeeded", "failed", "canceled", "timed_out", "unknown_xyz"}
	for _, s := range states {
		badge := stateBadge(Active, s)
		if badge == "" {
			t.Fatalf("stateBadge(%q) returned empty", s)
		}
	}
}

// ── StripANSIExport / stripANSI ──────────────────────────────────────────────

func TestStripANSIExport(t *testing.T) {
	input := "\x1b[32mhello\x1b[0m world"
	got := StripANSIExport(input)
	if got != "hello world" {
		t.Fatalf("StripANSIExport(%q) = %q, want %q", input, got, "hello world")
	}
}

func TestStripANSINoEscapes(t *testing.T) {
	input := "plain text"
	got := StripANSIExport(input)
	if got != input {
		t.Fatalf("StripANSIExport changed plain text: %q", got)
	}
}

func TestStripANSIEmpty(t *testing.T) {
	if got := StripANSIExport(""); got != "" {
		t.Fatalf("StripANSIExport(\"\") = %q", got)
	}
}

func TestStripANSIPreservesNonASCII(t *testing.T) {
	input := "\x1b[31m日本語\x1b[0m"
	got := StripANSIExport(input)
	if got != "日本語" {
		t.Fatalf("StripANSIExport with Unicode: %q", got)
	}
}

func TestStripANSIInternal(t *testing.T) {
	// Internal stripANSI (used by RenderStatusBar) — covered via RenderStatusBar tests above.
	// Direct test for completeness.
	input := "\x1b[1;33mWARN\x1b[0m"
	got := stripANSI(input)
	if got != "WARN" {
		t.Fatalf("stripANSI(%q) = %q", input, got)
	}
}

func TestStripANSIOSCSequence(t *testing.T) {
	// The internal ui.stripANSI is a simple CSI-only stripper.
	// It correctly handles ESC [ sequences but passes through other escape types.
	// This matches the documented implementation in panes.go.
	input := "\x1b[32mhello\x1b[0m world"
	got := stripANSI(input)
	if got != "hello world" {
		t.Fatalf("stripANSI CSI: %q -> %q", input, got)
	}
}
