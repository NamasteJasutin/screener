package ui

import "charm.land/lipgloss/v2"

// Theme holds all color tokens used by the TUI.
// All colors are stored as hex strings (e.g. "#FF0000" or ANSI index "196").
// Use lipgloss.Color(t.PaneBg) wherever a color.Color is required.
type Theme struct {
	Name string
	Dark bool // false = light theme

	// Pane surfaces
	PaneBg string
	PaneFg string

	// Borders
	Border      string // unfocused
	BorderFocus string // focused

	// Typography
	TitleFg      string
	TitleFgFocus string
	Muted        string

	// Semantic
	Good string // success / supported / live
	Warn string // warning
	Err  string // error / incompatible
	Info string // command / info text

	// Status bar (1-line bottom strip)
	StatusBg  string
	StatusFg  string
	StatusKey string

	// Matrix rain — 5 hex strings from dim tail to bright head
	// Index 0 = tail/dim, index 4 = head/bright  (matches SetPalette convention)
	MatrixHead string
	MatrixHi   string
	MatrixMid  string
	MatrixLo   string
	MatrixTail string
}

// ── 12 canonical themes ───────────────────────────────────────────────────────
// Counts:
//   Dark pure (6):       Matrix, Dracula, TokyoNight, Nord, CatppuccinMocha, GruvboxDark
//   Dark lighter (4):    OneDark, SolarizedDark, AyuDark, MaterialOcean
//   Extremely light (2): SolarizedLight, Snow
// Total: 12

var (
	// 1 ── Matrix  (the default — true hacker green-on-black)
	ThemeMatrix = Theme{
		Name: "Matrix", Dark: true,
		PaneBg: "#0D1117", PaneFg: "#C9D1D9",
		Border: "#21262D", BorderFocus: "#00FF41",
		TitleFg: "#E6EDF3", TitleFgFocus: "#00FF41",
		Muted: "#484F58",
		Good: "#3FB950", Warn: "#D29922", Err: "#F85149", Info: "#79C0FF",
		StatusBg: "#161B22", StatusFg: "#8B949E", StatusKey: "#00FF41",
		MatrixHead: "#D8FFD8", MatrixHi: "#52D152",
		MatrixMid: "#1F8A1F", MatrixLo: "#0F4D0F", MatrixTail: "#0A2F0A",
	}

	// 2 ── Dracula  (purple night)
	ThemeDracula = Theme{
		Name: "Dracula", Dark: true,
		PaneBg: "#282A36", PaneFg: "#F8F8F2",
		Border: "#44475A", BorderFocus: "#BD93F9",
		TitleFg: "#F8F8F2", TitleFgFocus: "#FF79C6",
		Muted: "#6272A4",
		Good: "#50FA7B", Warn: "#FFB86C", Err: "#FF5555", Info: "#8BE9FD",
		StatusBg: "#191A21", StatusFg: "#6272A4", StatusKey: "#BD93F9",
		MatrixHead: "#50FA7B", MatrixHi: "#2EC055",
		MatrixMid: "#1B9E3F", MatrixLo: "#0F6B2A", MatrixTail: "#084019",
	}

	// 3 ── Tokyo Night  (deep indigo-blue)
	ThemeTokyoNight = Theme{
		Name: "TokyoNight", Dark: true,
		PaneBg: "#1A1B2E", PaneFg: "#A9B1D6",
		Border: "#24283B", BorderFocus: "#7AA2F7",
		TitleFg: "#C0CAF5", TitleFgFocus: "#7AA2F7",
		Muted: "#565F89",
		Good: "#9ECE6A", Warn: "#E0AF68", Err: "#F7768E", Info: "#7DCFFF",
		StatusBg: "#16161E", StatusFg: "#545C7E", StatusKey: "#7AA2F7",
		MatrixHead: "#9ECE6A", MatrixHi: "#6DA043",
		MatrixMid: "#4A7030", MatrixLo: "#2D461D", MatrixTail: "#18260F",
	}

	// 4 ── Nord  (arctic blue-grey)
	ThemeNord = Theme{
		Name: "Nord", Dark: true,
		PaneBg: "#2E3440", PaneFg: "#ECEFF4",
		Border: "#3B4252", BorderFocus: "#88C0D0",
		TitleFg: "#ECEFF4", TitleFgFocus: "#88C0D0",
		Muted: "#4C566A",
		Good: "#A3BE8C", Warn: "#EBCB8B", Err: "#BF616A", Info: "#81A1C1",
		StatusBg: "#242933", StatusFg: "#616E88", StatusKey: "#88C0D0",
		MatrixHead: "#88C0D0", MatrixHi: "#5A9BA9",
		MatrixMid: "#3B7882", MatrixLo: "#245560", MatrixTail: "#103540",
	}

	// 5 ── Catppuccin Mocha  (cozy lavender dark)
	ThemeCatppuccinMocha = Theme{
		Name: "CatppuccinMocha", Dark: true,
		PaneBg: "#1E1E2E", PaneFg: "#CDD6F4",
		Border: "#313244", BorderFocus: "#CBA6F7",
		TitleFg: "#CDD6F4", TitleFgFocus: "#CBA6F7",
		Muted: "#585B70",
		Good: "#A6E3A1", Warn: "#FAB387", Err: "#F38BA8", Info: "#89DCEB",
		StatusBg: "#181825", StatusFg: "#6C7086", StatusKey: "#CBA6F7",
		MatrixHead: "#A6E3A1", MatrixHi: "#72B86D",
		MatrixMid: "#4D8F4A", MatrixLo: "#2F6530", MatrixTail: "#1A3D1B",
	}

	// 6 ── Gruvbox Dark  (warm amber-brown)
	ThemeGruvboxDark = Theme{
		Name: "GruvboxDark", Dark: true,
		PaneBg: "#282828", PaneFg: "#EBDBB2",
		Border: "#3C3836", BorderFocus: "#D79921",
		TitleFg: "#EBDBB2", TitleFgFocus: "#FABD2F",
		Muted: "#504945",
		Good: "#B8BB26", Warn: "#FE8019", Err: "#FB4934", Info: "#83A598",
		StatusBg: "#1D2021", StatusFg: "#7C6F64", StatusKey: "#D79921",
		MatrixHead: "#B8BB26", MatrixHi: "#8A8D1C",
		MatrixMid: "#606312", MatrixLo: "#3D3F0B", MatrixTail: "#242604",
	}

	// 7 ── One Dark  (medium — slightly lighter dark)
	ThemeOneDark = Theme{
		Name: "OneDark", Dark: true,
		PaneBg: "#282C34", PaneFg: "#ABB2BF",
		Border: "#3E4451", BorderFocus: "#61AFEF",
		TitleFg: "#ABB2BF", TitleFgFocus: "#61AFEF",
		Muted: "#5C6370",
		Good: "#98C379", Warn: "#E5C07B", Err: "#E06C75", Info: "#56B6C2",
		StatusBg: "#21252B", StatusFg: "#5C6370", StatusKey: "#61AFEF",
		MatrixHead: "#98C379", MatrixHi: "#6A8F52",
		MatrixMid: "#4B6B38", MatrixLo: "#2F4422", MatrixTail: "#1A2712",
	}

	// 8 ── Solarized Dark  (warm muted classic — lighter dark)
	ThemeSolarizedDark = Theme{
		Name: "SolarizedDark", Dark: true,
		PaneBg: "#002B36", PaneFg: "#839496",
		Border: "#073642", BorderFocus: "#268BD2",
		TitleFg: "#93A1A1", TitleFgFocus: "#268BD2",
		Muted: "#586E75",
		Good: "#859900", Warn: "#B58900", Err: "#DC322F", Info: "#2AA198",
		StatusBg: "#00212B", StatusFg: "#586E75", StatusKey: "#268BD2",
		MatrixHead: "#859900", MatrixHi: "#5E6C00",
		MatrixMid: "#404B00", MatrixLo: "#272D00", MatrixTail: "#141800",
	}

	// 9 ── Ayu Dark  (dark but on the lighter side — warm gold accent)
	ThemeAyuDark = Theme{
		Name: "AyuDark", Dark: true,
		PaneBg: "#0D1018", PaneFg: "#B3B1AD",
		Border: "#1A1F29", BorderFocus: "#E6B450",
		TitleFg: "#E6E1CF", TitleFgFocus: "#E6B450",
		Muted: "#3D424D",
		Good: "#7FD962", Warn: "#E6B450", Err: "#F07178", Info: "#73D0FF",
		StatusBg: "#0A0E14", StatusFg: "#3D424D", StatusKey: "#E6B450",
		MatrixHead: "#7FD962", MatrixHi: "#57A842",
		MatrixMid: "#3C7D2C", MatrixLo: "#26531B", MatrixTail: "#14300E",
	}

	// 10 ── Material Ocean  (dark blue-grey — on the lighter dark side)
	ThemeMaterialOcean = Theme{
		Name: "MaterialOcean", Dark: true,
		PaneBg: "#0F111A", PaneFg: "#8F93A2",
		Border: "#1A1C25", BorderFocus: "#82AAFF",
		TitleFg: "#EEFFFF", TitleFgFocus: "#82AAFF",
		Muted: "#3B3F51",
		Good: "#C3E88D", Warn: "#FFCB6B", Err: "#F07178", Info: "#89DDFF",
		StatusBg: "#090B10", StatusFg: "#3B3F51", StatusKey: "#82AAFF",
		MatrixHead: "#C3E88D", MatrixHi: "#8ABF5E",
		MatrixMid: "#608D3E", MatrixLo: "#3F5E27", MatrixTail: "#233514",
	}

	// 11 ── Solarized Light  (very light — warm off-white)
	ThemeSolarizedLight = Theme{
		Name: "SolarizedLight", Dark: false,
		PaneBg: "#FDF6E3", PaneFg: "#657B83",
		Border: "#EEE8D5", BorderFocus: "#268BD2",
		TitleFg: "#586E75", TitleFgFocus: "#268BD2",
		Muted: "#93A1A1",
		Good: "#859900", Warn: "#B58900", Err: "#DC322F", Info: "#2AA198",
		StatusBg: "#EEE8D5", StatusFg: "#839496", StatusKey: "#268BD2",
		MatrixHead: "#268BD2", MatrixHi: "#1A6CA3",
		MatrixMid: "#114E78", MatrixLo: "#093455", MatrixTail: "#041D33",
	}

	// 12 ── Snow  (extremely light — pure white dev mode)
	ThemeSnow = Theme{
		Name: "Snow", Dark: false,
		PaneBg: "#F8F9FA", PaneFg: "#212529",
		Border: "#DEE2E6", BorderFocus: "#0D6EFD",
		TitleFg: "#212529", TitleFgFocus: "#0D6EFD",
		Muted: "#ADB5BD",
		Good: "#198754", Warn: "#CC8800", Err: "#DC3545", Info: "#0DCAF0",
		StatusBg: "#E9ECEF", StatusFg: "#6C757D", StatusKey: "#0D6EFD",
		MatrixHead: "#198754", MatrixHi: "#126644",
		MatrixMid: "#0B4A30", MatrixLo: "#062F1F", MatrixTail: "#021A11",
	}
)

// AllThemes returns all 12 themes in canonical order.
func AllThemes() []Theme {
	return []Theme{
		ThemeMatrix,
		ThemeDracula,
		ThemeTokyoNight,
		ThemeNord,
		ThemeCatppuccinMocha,
		ThemeGruvboxDark,
		ThemeOneDark,
		ThemeSolarizedDark,
		ThemeAyuDark,
		ThemeMaterialOcean,
		ThemeSolarizedLight,
		ThemeSnow,
	}
}

// FindTheme returns the theme with the given name, or ThemeMatrix as fallback.
func FindTheme(name string) Theme {
	for _, t := range AllThemes() {
		if t.Name == name {
			return t
		}
	}
	return ThemeMatrix
}

// ── Per-theme Lip Gloss style builders ───────────────────────────────────────
// These are called each render frame to build fresh styles from the active theme.

func (t Theme) PaneStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Background(lipgloss.Color(t.PaneBg)).
		Foreground(lipgloss.Color(t.PaneFg)).
		BorderBackground(lipgloss.Color(t.PaneBg))
}

func (t Theme) TitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color(t.TitleFg)).
		Background(lipgloss.Color(t.PaneBg))
}

func (t Theme) TitleFocusStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color(t.TitleFgFocus)).
		Background(lipgloss.Color(t.PaneBg))
}

// Pane-context styles: foreground + PaneBg to ensure opaque cells in overlays.
func (t Theme) MutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Muted)).Background(lipgloss.Color(t.PaneBg))
}

func (t Theme) GoodStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Good)).Background(lipgloss.Color(t.PaneBg))
}

func (t Theme) WarnStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Warn)).Background(lipgloss.Color(t.PaneBg))
}

func (t Theme) ErrStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Err)).Background(lipgloss.Color(t.PaneBg))
}

func (t Theme) InfoStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Info)).Background(lipgloss.Color(t.PaneBg))
}

// Status-bar styles: use StatusBg so inline elements match the bar background.
func (t Theme) StatusMutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Muted)).Background(lipgloss.Color(t.StatusBg))
}

func (t Theme) StatusGoodStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Good)).Background(lipgloss.Color(t.StatusBg))
}

func (t Theme) StatusWarnStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Warn)).Background(lipgloss.Color(t.StatusBg))
}

func (t Theme) StatusErrStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Err)).Background(lipgloss.Color(t.StatusBg))
}

func (t Theme) StatusKeyStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.StatusKey)).Background(lipgloss.Color(t.StatusBg))
}

// MatrixPalette returns the 5-element hex palette for the matrix rain renderer.
// Order: [0]=tail/dim ... [4]=head/bright
func (t Theme) MatrixPalette() [5]string {
	return [5]string{
		t.MatrixTail,
		t.MatrixLo,
		t.MatrixMid,
		t.MatrixHi,
		t.MatrixHead,
	}
}
