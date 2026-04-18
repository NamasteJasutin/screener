package render

import (
	"math"
	"math/rand"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

type matrixColumn struct {
	head  float64
	speed float64
	trail int
}

type MatrixRain struct {
	width         int
	height        int
	columns       []matrixColumn
	rng           *rand.Rand
	reducedMotion bool
	asciiMode     bool
	charset       []rune
	asciiCharset  []rune
	kanaCharset   []rune
	palette       [5]string     // current fg palette (tail→head)
	bgColor       string        // hex background for all rain cells (matches theme PaneBg)
	styles        [5]lipgloss.Style
	tick          uint64
	// Dirty-flag cache: View() re-renders only when the rain state changed.
	cachedView  string
	cachedLines int
	viewDirty   bool
}

// defaultPalette is the classic green Matrix palette (5 tones, tail to head).
var defaultPalette = [5]string{
	"#0A2F0A", "#0F4D0F", "#1F8A1F", "#52D152", "#D8FFD8",
}

func NewMatrixRain(width, height int) *MatrixRain {
	r := &MatrixRain{
		width:        max(width, 1),
		height:       max(height, 1),
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
		viewDirty:   true,
		asciiCharset: []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@#$%&*+-=!?"),
		kanaCharset: []rune(
			"ｦｧｨｩｪｫｬｭｮｯｰｱｲｳｴｵｶｷｸｹｺｻｼｽｾｿﾀﾁﾂﾃﾄﾅﾆﾇﾈﾉ" +
				"ﾊﾋﾌﾍﾎﾏﾐﾑﾒﾓﾔﾕﾖﾗﾘﾙﾚﾛﾜﾝ0123456789",
		),
		palette: defaultPalette,
		styles:  paletteToStyles(defaultPalette, ""),
	}
	r.charset = r.kanaCharset
	r.columns = make([]matrixColumn, r.width)
	for i := range r.columns {
		r.resetColumn(i)
	}
	return r
}

func (m *MatrixRain) SetReducedMotion(enabled bool) {
	if m.reducedMotion == enabled {
		return
	}
	m.reducedMotion = enabled
	m.viewDirty = true
	for i := range m.columns {
		if enabled {
			m.columns[i].speed = max(0.16, m.columns[i].speed*0.5)
		} else {
			m.columns[i].speed = min(1.65, m.columns[i].speed*1.9)
		}
	}
}

// SetPalette updates the rain colors from a 5-element hex string palette
// (index 0 = tail/dim, index 4 = head/bright).
func (m *MatrixRain) SetPalette(palette [5]string) {
	m.palette = palette
	m.styles = paletteToStyles(palette, m.bgColor)
	m.viewDirty = true
}

// SetBackground sets the background color applied to every rain cell so that
// the rain is always opaque and matches the active theme's surface color.
func (m *MatrixRain) SetBackground(hex string) {
	if m.bgColor == hex {
		return
	}
	m.bgColor = hex
	m.styles = paletteToStyles(m.palette, hex)
	m.viewDirty = true
}

func paletteToStyles(p [5]string, bg string) [5]lipgloss.Style {
	base := func(fg string) lipgloss.Style {
		s := lipgloss.NewStyle().Foreground(lipgloss.Color(fg))
		if bg != "" {
			s = s.Background(lipgloss.Color(bg))
		}
		return s
	}
	return [5]lipgloss.Style{
		base(p[0]),
		base(p[1]),
		base(p[2]),
		base(p[3]),
		base(p[4]).Bold(true),
	}
}


func (m *MatrixRain) SetASCIIMode(enabled bool) {
	m.asciiMode = enabled
	m.viewDirty = true
	if enabled {
		m.charset = m.asciiCharset
		return
	}
	m.charset = m.kanaCharset
}

func (m *MatrixRain) Resize(width, height int) {
	m.viewDirty = true
	newWidth := max(width, 1)
	newHeight := max(height, 1)

	if newWidth != m.width {
		columns := make([]matrixColumn, newWidth)
		copy(columns, m.columns)
		m.columns = columns
		for i := m.width; i < newWidth; i++ {
			m.resetColumn(i)
		}
	}

	m.width = newWidth
	m.height = newHeight

	for i := range m.columns {
		if m.columns[i].trail > max(2, m.height-1) {
			m.columns[i].trail = max(2, m.height-1)
		}
	}
}

func (m *MatrixRain) Tick() {
	m.tick++
	if m.reducedMotion && m.tick%2 == 0 {
		return
	}
	m.viewDirty = true

	for i := range m.columns {
		m.columns[i].head += m.columns[i].speed

		if !m.reducedMotion && m.rng.Intn(64) == 0 {
			m.columns[i].speed += (m.rng.Float64() - 0.5) * 0.14
			m.columns[i].speed = clamp(m.columns[i].speed, 0.45, 1.65)
		}

		headY := int(math.Floor(m.columns[i].head))
		if headY-m.columns[i].trail > m.height+2 {
			m.resetColumn(i)
		}
	}
}

func (m *MatrixRain) View(lines int) string {
	h := m.height
	if lines > 0 {
		h = min(lines, m.height)
	}
	if h <= 0 || m.width <= 0 {
		return ""
	}
	// Return cached output when nothing has changed since the last render.
	if !m.viewDirty && m.cachedView != "" && m.cachedLines == h {
		return m.cachedView
	}
	m.viewDirty = false
	m.cachedLines = h

	chars := make([][]rune, h)
	tones := make([][]uint8, h)
	for y := 0; y < h; y++ {
		chars[y] = make([]rune, m.width)
		tones[y] = make([]uint8, m.width)
		for x := 0; x < m.width; x++ {
			chars[y][x] = ' '
		}
	}

	for x := 0; x < len(m.columns) && x < m.width; x++ {
		col := m.columns[x]
		headY := int(math.Floor(col.head))
		for d := 0; d < col.trail; d++ {
			y := headY - d
			if y < 0 || y >= h {
				continue
			}
			tone := m.trailTone(d, col.trail)
			if tone >= tones[y][x] {
				tones[y][x] = tone
				chars[y][x] = m.pickChar()
			}
		}
	}

	// Pre-build the blank cell once (background-colored space or plain space).
	blankCell := " "
	if m.bgColor != "" {
		blankCell = lipgloss.NewStyle().Background(lipgloss.Color(m.bgColor)).Render(" ")
	}

	out := make([]string, h)
	for y := 0; y < h; y++ {
		var row strings.Builder
		row.Grow(m.width * 12)
		for x := 0; x < m.width; x++ {
			tone := tones[y][x]
			if tone == 0 {
				row.WriteString(blankCell)
				continue
			}
			row.WriteString(m.styles[tone-1].Render(string(chars[y][x])))
		}
		out[y] = row.String()
	}
	m.cachedView = strings.Join(out, "\n")
	return m.cachedView
}

func (m *MatrixRain) resetColumn(i int) {
	speedMin, speedMax := 0.45, 1.65
	if m.reducedMotion {
		speedMin, speedMax = 0.16, 0.75
	}

	trailMin := max(3, m.height/6)
	trailMax := max(trailMin+1, m.height/2)

	m.columns[i] = matrixColumn{
		head:  -float64(m.rng.Intn(m.height + trailMax + 8)),
		speed: speedMin + m.rng.Float64()*(speedMax-speedMin),
		trail: trailMin + m.rng.Intn(max(1, trailMax-trailMin)),
	}
}

func (m *MatrixRain) pickChar() rune {
	if len(m.charset) == 0 {
		return ' '
	}
	return m.charset[m.rng.Intn(len(m.charset))]
}

func (m *MatrixRain) trailTone(distance, trail int) uint8 {
	if distance <= 0 {
		return 5
	}
	if trail <= 1 {
		return 4
	}
	ratio := float64(distance) / float64(trail)
	switch {
	case ratio < 0.2:
		return 4
	case ratio < 0.45:
		return 3
	case ratio < 0.7:
		return 2
	default:
		return 1
	}
}

// clamp constrains v to [lo, hi]. Uses Go 1.21+ built-in min/max.
func clamp(v, lo, hi float64) float64 { return max(lo, min(v, hi)) }
