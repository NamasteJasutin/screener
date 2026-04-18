package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// ── Frame geometry constants ───────────────────────────────────────────────────
//
// Every RenderPane call with Width(w) and Padding(0,1) and RoundedBorder:
//   outer_width  = w + 2 (padding left+right) + 2 (border left+right) = w + 4
//   outer_height = h + 2 (border top+bottom)
//
// Callers must pass:
//   innerW = desiredOuterWidth  - PaneFrameW   (= desiredOuterWidth  - 4)
//   innerH = desiredOuterHeight - PaneFrameH   (= desiredOuterHeight - 2)

const (
	PaneFrameW = 4 // total horizontal frame overhead per pane (border+padding)
	PaneFrameH = 2 // total vertical   frame overhead per pane (border top+bottom)
)

// ── Active theme (set by model at startup / theme change) ─────────────────────

var Active = ThemeMatrix

// SetTheme updates the active theme. Called from the model when the theme changes.
func SetTheme(t Theme) { Active = t }

// ── Helpers that use the active theme ────────────────────────────────────────

func StyleGood(s string) string  { return Active.GoodStyle().Render(s) }
func StyleWarn(s string) string  { return Active.WarnStyle().Render(s) }
func StyleErr(s string) string   { return Active.ErrStyle().Render(s) }
func StyleMuted(s string) string { return Active.MutedStyle().Render(s) }
func StyleInfo(s string) string  { return Active.InfoStyle().Render(s) }

// ── Core pane renderer ────────────────────────────────────────────────────────

// RenderPane renders a bordered, titled pane.
// width and height are the INNER content dimensions (excluding frame).
// Outer rendered dimensions: (width + PaneFrameW) × (height + PaneFrameH).
func RenderPane(title string, body []string, width int, height int) string {
	return renderPane(title, body, width, height, false)
}

// RenderPaneFocused renders a pane with an optional focus ring.
func RenderPaneFocused(title string, body []string, width int, height int, focused bool) string {
	return renderPane(title, body, width, height, focused)
}

func renderPane(title string, body []string, width int, height int, focused bool) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	// Body: truncate or pad to height-2 rows (1 for title, 1 breathing room at bottom).
	bodyRows := max(height-2, 1)
	lines := make([]string, 0, bodyRows)
	for i := 0; i < len(body) && i < bodyRows; i++ {
		lines = append(lines, body[i])
	}
	if len(lines) == 0 {
		lines = append(lines, Active.MutedStyle().Render("-"))
	}
	for len(lines) < bodyRows {
		lines = append(lines, "")
	}

	ts := Active.TitleStyle()
	borderHex := Active.Border
	if focused {
		ts = Active.TitleFocusStyle()
		borderHex = Active.BorderFocus
	}

	content := ts.Render(title) + "\n" + strings.Join(lines, "\n")

	return Active.PaneStyle().
		Width(width).
		Height(height).
		BorderForeground(lipgloss.Color(borderHex)).
		Render(content)
}

// ── Status bar ─────────────────────────────────────────────────────────────────

// RenderStatusBar renders a full-width 1-line status bar.
// Sections: device | profile | launch state | context keys (right-aligned).
func RenderStatusBar(width int, device, profile, state, keys string) string {
	t := Active
	sep := t.StatusMutedStyle().Render(" │ ")

	// Left side — all inline elements use StatusBg so the bar is uniform.
	devStr := t.StatusGoodStyle().Render("●") + " " + device
	if device == "" || device == "simulated" || device == "no device" {
		devStr = t.StatusMutedStyle().Render("○ no device")
	}
	profStr := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.PaneFg)).
		Background(lipgloss.Color(t.StatusBg)).
		Render(profile)
	if profile == "" {
		profStr = t.StatusMutedStyle().Render("no profile")
	}
	left := devStr + sep + profStr + sep + stateBadge(t, state)

	// Right side (context keys)
	keysStr := t.StatusKeyStyle().Render(keys)

	// Compute padding between left and right.
	leftPlain := stripANSI(left)
	rightPlain := stripANSI(keysStr)
	used := len([]rune(leftPlain)) + len([]rune(rightPlain)) + 2
	pad := width - used
	if pad < 1 {
		pad = 1
	}
	content := left + strings.Repeat(" ", pad) + keysStr

	return lipgloss.NewStyle().
		Width(width).
		Background(lipgloss.Color(t.StatusBg)).
		Foreground(lipgloss.Color(t.StatusFg)).
		Render(content)
}

func stateBadge(t Theme, state string) string {
	switch state {
	case "idle":
		return t.StatusGoodStyle().Render("✓ ready")
	case "launching":
		return t.StatusWarnStyle().Render("⟳ launching")
	case "succeeded":
		return t.StatusGoodStyle().Render("✓ launched")
	case "failed":
		return t.StatusErrStyle().Render("✗ failed")
	case "canceled":
		return t.StatusMutedStyle().Render("⊘ canceled")
	case "timed_out":
		return t.StatusErrStyle().Render("✗ timed out")
	default:
		return t.StatusMutedStyle().Render(state)
	}
}

// ── ANSI helpers ───────────────────────────────────────────────────────────────

// StripANSIExport is the exported variant used by model.go for overlay centering.
func StripANSIExport(s string) string { return stripANSI(s) }

func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}


