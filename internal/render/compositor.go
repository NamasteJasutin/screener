package render

import (
	"strings"
	"unicode/utf8"
)

type OverlayBlock struct {
	X          int
	Y          int
	Content    string
	SeeThrough bool // if true, space cells let the background (rain) show through
}

func Compose(width, height int, background string, blocks ...OverlayBlock) string {
	if width <= 0 || height <= 0 {
		return "\x1b[H"
	}

	canvas := make([][]string, height)
	for y := 0; y < height; y++ {
		canvas[y] = make([]string, width)
		for x := 0; x < width; x++ {
			canvas[y][x] = " "
		}
	}

	bgLines := strings.Split(background, "\n")
	for y := 0; y < height && y < len(bgLines); y++ {
		cells := parseStyledCells(bgLines[y])
		if len(cells) > width {
			cells = cells[:width]
		}
		for x := 0; x < len(cells); x++ {
			canvas[y][x] = cells[x]
		}
	}

	for _, block := range blocks {
		if block.Content == "" {
			continue
		}
		x := block.X
		y := max(1, min(block.Y, height))
		lines := strings.Split(block.Content, "\n")
		for i, line := range lines {
			row := y + i
			if row < 1 || row > height {
				continue
			}
			if x > width {
				continue
			}

			drawX := x
			leftCrop := 0
			if drawX < 1 {
				leftCrop = 1 - drawX
				drawX = 1
			}

			cells := parseStyledCells(line)
			if leftCrop >= len(cells) {
				continue
			}
			if leftCrop > 0 {
				cells = cells[leftCrop:]
			}
			maxVisible := width - drawX + 1
			if maxVisible <= 0 {
				continue
			}
			if len(cells) > maxVisible {
				cells = cells[:maxVisible]
			}
			if len(cells) == 0 {
				continue
			}

			if block.SeeThrough {
				for ci, cell := range cells {
					col := drawX - 1 + ci
					if col >= width {
						break
					}
					// Skip space cells — let the rain character underneath show through.
					if stripANSI(cell) == " " {
						continue
					}
					canvas[row-1][col] = cell
				}
			} else {
				copy(canvas[row-1][drawX-1:], cells)
			}
		}
	}

	var out strings.Builder
	out.WriteString("\x1b[H")
	for y := 0; y < height; y++ {
		if y > 0 {
			out.WriteByte('\n')
		}
		for x := 0; x < width; x++ {
			out.WriteString(canvas[y][x])
		}
	}

	return out.String()
}

func parseStyledCells(s string) []string {
	if s == "" {
		return nil
	}

	var cells []string
	var pendingEscapes strings.Builder

	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			next, ok := ansiSequenceEnd(s, i)
			if !ok || next <= i {
				break
			}
			pendingEscapes.WriteString(s[i:next])
			i = next
			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			size = 1
		}

		var cell strings.Builder
		if pendingEscapes.Len() > 0 {
			cell.WriteString(pendingEscapes.String())
			pendingEscapes.Reset()
		}
		cell.WriteString(s[i : i+size])
		cells = append(cells, cell.String())
		i += size
	}

	if pendingEscapes.Len() > 0 && len(cells) > 0 {
		cells[len(cells)-1] += pendingEscapes.String()
	}

	return cells
}

func stripANSI(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			next, ok := ansiSequenceEnd(s, i)
			if ok && next > i {
				i = next
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			size = 1
		}
		out.WriteString(s[i : i+size])
		i += size
	}
	return out.String()
}

func ansiSequenceEnd(s string, start int) (int, bool) {
	if start+1 >= len(s) || s[start] != 0x1b {
		return start, false
	}

	lead := s[start+1]
	switch lead {
	case '[':
		i := start + 2
		for i < len(s) {
			b := s[i]
			if b >= 0x40 && b <= 0x7e {
				return i + 1, true
			}
			i++
		}
		return start, false
	case ']':
		i := start + 2
		for i < len(s) {
			if s[i] == 0x07 {
				return i + 1, true
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2, true
			}
			i++
		}
		return start, false
	default:
		return start + 2, true
	}
}


