// Package charts renders small live data visualisations as plain strings for a
// Bubble Tea / lipgloss TUI: braille line charts, vertical bar charts,
// sparklines, gauges, big seven-segment numbers and a spectrogram-style
// heatmap. It has no dependency on the parent tui package so it can be reused.
package charts

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette carries the colours a chart may use.
type Palette struct {
	Line lipgloss.Color
	Dim  lipgloss.Color
	Good lipgloss.Color
	Warn lipgloss.Color
	Bad  lipgloss.Color
	Text lipgloss.Color
}

func fg(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

// --- helpers ---

func minMax(v []float64) (mn, mx float64) {
	mn, mx = math.Inf(1), math.Inf(-1)
	for _, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			continue
		}
		if x < mn {
			mn = x
		}
		if x > mx {
			mx = x
		}
	}
	if math.IsInf(mn, 0) {
		mn, mx = 0, 1
	}
	if mn == mx {
		mx = mn + 1
	}
	return
}

// HumanNum formats a number with SI-ish suffixes for compact display.
func HumanNum(f float64) string {
	neg := ""
	if f < 0 {
		neg = "-"
		f = -f
	}
	switch {
	case f >= 1e12:
		return fmt.Sprintf("%s%.2fT", neg, f/1e12)
	case f >= 1e9:
		return fmt.Sprintf("%s%.2fG", neg, f/1e9)
	case f >= 1e6:
		return fmt.Sprintf("%s%.2fM", neg, f/1e6)
	case f >= 1e3:
		return fmt.Sprintf("%s%.2fk", neg, f/1e3)
	case f == math.Trunc(f):
		return fmt.Sprintf("%s%d", neg, int64(f))
	default:
		return fmt.Sprintf("%s%.2f", neg, f)
	}
}

// --- Sparkline ---

var sparkRunes = []rune("▁▂▃▄▅▆▇█")

// Sparkline renders values into a single row of width columns.
func Sparkline(values []float64, width int, p Palette) string {
	if width < 1 {
		width = 1
	}
	v := lastN(values, width)
	if len(v) == 0 {
		return fg(p.Dim).Render(strings.Repeat("·", width))
	}
	mn, mx := minMax(v)
	var b strings.Builder
	for _, x := range v {
		idx := int((x - mn) / (mx - mn) * float64(len(sparkRunes)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sparkRunes) {
			idx = len(sparkRunes) - 1
		}
		b.WriteRune(sparkRunes[idx])
	}
	out := b.String()
	if pad := width - len([]rune(out)); pad > 0 {
		out = strings.Repeat(" ", pad) + out
	}
	return fg(p.Line).Render(out)
}

// --- vertical Bars ---

var barRunes = []rune(" ▁▂▃▄▅▆▇█")

// Bars renders the most recent values as vertical bars.
func Bars(values []float64, width, height int, p Palette) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	v := lastN(values, width)
	if len(v) == 0 {
		return strings.Repeat(fg(p.Dim).Render(strings.Repeat("·", width))+"\n", height)
	}
	mn, mx := minMax(v)
	if mn > 0 {
		mn = 0 // bars read better from a zero baseline when data is positive
	}
	rows := make([]string, height)
	for _, x := range v {
		frac := (x - mn) / (mx - mn)
		if frac < 0 {
			frac = 0
		}
		filled := frac * float64(height)
		for r := 0; r < height; r++ {
			// r counted from top; bottom row is height-1
			level := float64(height - 1 - r)
			var ch rune
			switch {
			case filled >= level+1:
				ch = '█'
			case filled <= level:
				ch = ' '
			default:
				part := filled - level // 0..1
				ci := int(part*8 + 0.5)
				if ci < 1 {
					ci = 1
				}
				if ci > 8 {
					ci = 8
				}
				ch = barRunes[ci]
			}
			rows[r] += string(ch)
		}
	}
	col := colorFor(v[len(v)-1], mn, mx, p)
	for i := range rows {
		rows[i] = fg(col).Render(padRow(rows[i], width))
	}
	return strings.Join(rows, "\n")
}

// --- Gauge ---

// Gauge renders a horizontal meter plus the numeric value.
func Gauge(value, lo, hi float64, width int, p Palette) string {
	if width < 10 {
		width = 10
	}
	if hi <= lo {
		hi = lo + 1
	}
	ratio := (value - lo) / (hi - lo)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	label := fmt.Sprintf(" %s ", HumanNum(value))
	track := width - lipgloss.Width(label) - 2
	if track < 4 {
		track = 4
	}
	fill := int(ratio*float64(track) + 0.5)
	col := colorFor(value, lo, hi, p)
	meter := fg(col).Render(strings.Repeat("█", fill)) +
		fg(p.Dim).Render(strings.Repeat("─", track-fill))
	ends := fg(p.Dim).Render("[") + meter + fg(p.Dim).Render("]")
	return ends + fg(p.Text).Render(label)
}

// --- Big number ---

var bigFont = map[rune][]string{
	'0': {"███", "█ █", "█ █", "█ █", "███"},
	'1': {"  █", "  █", "  █", "  █", "  █"},
	'2': {"███", "  █", "███", "█  ", "███"},
	'3': {"███", "  █", "███", "  █", "███"},
	'4': {"█ █", "█ █", "███", "  █", "  █"},
	'5': {"███", "█  ", "███", "  █", "███"},
	'6': {"███", "█  ", "███", "█ █", "███"},
	'7': {"███", "  █", "  █", "  █", "  █"},
	'8': {"███", "█ █", "███", "█ █", "███"},
	'9': {"███", "█ █", "███", "  █", "███"},
	'.': {"   ", "   ", "   ", "   ", " █ "},
	',': {"   ", "   ", "   ", " █ ", "█  "},
	'-': {"   ", "   ", "███", "   ", "   "},
	':': {"   ", " █ ", "   ", " █ ", "   "},
	'%': {"█ █", "  █", " █ ", "█  ", "█ █"},
	' ': {"   ", "   ", "   ", "   ", "   "},
	'/': {"  █", "  █", " █ ", "█  ", "█  "},
	'k': {"   ", "█ █", "██ ", "█ █", "█ █"},
	'M': {"   ", "█ █", "███", "█ █", "█ █"},
	'G': {"   ", "███", "█  ", "█ █", "███"},
	'T': {"   ", "███", " █ ", " █ ", " █ "},
}

// BigNumber renders a string (digits/punctuation) as 5-row block glyphs. Any
// unsupported rune is dropped.
func BigNumber(s string, p Palette) string {
	rows := make([]string, 5)
	for _, r := range s {
		g, ok := bigFont[r]
		if !ok {
			continue
		}
		for i := 0; i < 5; i++ {
			rows[i] += g[i] + " "
		}
	}
	for i := range rows {
		rows[i] = fg(p.Line).Render(rows[i])
	}
	return strings.Join(rows, "\n")
}

// --- Heatmap (spectrogram style) ---

var heatRamp = []string{"#12203a", "#153e6b", "#1f7a8c", "#35c98b", "#c9d65a", "#f2b134", "#e2223b"}

// Heatmap renders recent values as coloured columns filled bottom-up; hotter
// values shift the whole column toward red.
func Heatmap(values []float64, width, height int, p Palette) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	v := lastN(values, width)
	if len(v) == 0 {
		return strings.Repeat(fg(p.Dim).Render(strings.Repeat("·", width))+"\n", height)
	}
	mn, mx := minMax(v)
	rows := make([][]string, height)
	for r := range rows {
		rows[r] = make([]string, len(v))
	}
	for c, x := range v {
		frac := (x - mn) / (mx - mn)
		if frac < 0 {
			frac = 0
		}
		if frac > 1 {
			frac = 1
		}
		rampIdx := int(frac * float64(len(heatRamp)-1))
		col := lipgloss.Color(heatRamp[rampIdx])
		lit := int(frac*float64(height) + 0.5)
		for r := 0; r < height; r++ {
			level := height - 1 - r
			if level < lit {
				rows[r][c] = fg(col).Render("█")
			} else {
				rows[r][c] = fg(p.Dim).Render("·")
			}
		}
	}
	var b strings.Builder
	for r := 0; r < height; r++ {
		b.WriteString(strings.Join(rows[r], ""))
		if r < height-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// --- Braille line chart ---

// Line renders series as a braille line chart of the given cell dimensions.
func Line(series []float64, width, height int, p Palette) string {
	if width < 2 {
		width = 2
	}
	if height < 2 {
		height = 2
	}
	v := lastN(series, width*2)
	if len(v) < 2 {
		return strings.Repeat(fg(p.Dim).Render(strings.Repeat(" ", width))+"\n", height)
	}
	mn, mx := minMax(v)
	pxW, pxH := width*2, height*4
	grid := make([][]bool, pxH)
	for i := range grid {
		grid[i] = make([]bool, pxW)
	}
	plot := func(x, y int) {
		if x >= 0 && x < pxW && y >= 0 && y < pxH {
			grid[y][x] = true
		}
	}
	yOf := func(val float64) int {
		f := (val - mn) / (mx - mn)
		y := int((1 - f) * float64(pxH-1))
		if y < 0 {
			y = 0
		}
		if y >= pxH {
			y = pxH - 1
		}
		return y
	}
	xOf := func(i int) int {
		return int(float64(i) / float64(len(v)-1) * float64(pxW-1))
	}
	for i := 1; i < len(v); i++ {
		x0, y0 := xOf(i-1), yOf(v[i-1])
		x1, y1 := xOf(i), yOf(v[i])
		linePx(x0, y0, x1, y1, plot)
	}

	dots := []rune("⠀")
	_ = dots
	var b strings.Builder
	for cy := 0; cy < height; cy++ {
		for cx := 0; cx < width; cx++ {
			var mask int
			type bit struct{ r, c, v int }
			for _, bt := range []bit{
				{0, 0, 0x01}, {1, 0, 0x02}, {2, 0, 0x04},
				{0, 1, 0x08}, {1, 1, 0x10}, {2, 1, 0x20},
				{3, 0, 0x40}, {3, 1, 0x80},
			} {
				py := cy*4 + bt.r
				px := cx*2 + bt.c
				if py < pxH && px < pxW && grid[py][px] {
					mask |= bt.v
				}
			}
			b.WriteRune(rune(0x2800 + mask))
		}
		if cy < height-1 {
			b.WriteByte('\n')
		}
	}
	return fg(p.Line).Render(b.String())
}

func linePx(x0, y0, x1, y1 int, plot func(x, y int)) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx := 1
	if x0 >= x1 {
		sx = -1
	}
	sy := 1
	if y0 >= y1 {
		sy = -1
	}
	err := dx + dy
	for {
		plot(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// --- shared small helpers ---

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func lastN(v []float64, n int) []float64 {
	if n <= 0 || len(v) <= n {
		return v
	}
	return v[len(v)-n:]
}

func padRow(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		return string(r[:w])
	}
	return s + strings.Repeat(" ", w-len(r))
}

func colorFor(v, lo, hi float64, p Palette) lipgloss.Color {
	if hi <= lo {
		return p.Line
	}
	f := (v - lo) / (hi - lo)
	switch {
	case f >= 0.85:
		return p.Bad
	case f >= 0.65:
		return p.Warn
	default:
		return p.Line
	}
}

// Axis renders a compact left-hand min/max/last legend for a chart of h rows.
func Axis(series []float64, h int, p Palette) string {
	if len(series) == 0 {
		return ""
	}
	mn, mx := minMax(series)
	last := series[len(series)-1]
	lines := make([]string, h)
	for i := range lines {
		lines[i] = ""
	}
	if h >= 1 {
		lines[0] = fg(p.Text).Render(HumanNum(mx))
	}
	if h >= 2 {
		lines[h-1] = fg(p.Text).Render(HumanNum(mn))
	}
	if h >= 3 {
		lines[h/2] = fg(p.Dim).Render(HumanNum(last))
	}
	w := 0
	for _, l := range lines {
		if lw := lipgloss.Width(l); lw > w {
			w = lw
		}
	}
	for i := range lines {
		lines[i] = lines[i] + strings.Repeat(" ", w-lipgloss.Width(lines[i]))
	}
	return strings.Join(lines, "\n")
}
