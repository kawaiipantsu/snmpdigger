package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// selector is a left/right cycled option picker.
type selector struct {
	label   string
	options []string
	idx     int
}

func newSelector(label string, options []string, current string) selector {
	s := selector{label: label, options: options}
	for i, o := range options {
		if strings.EqualFold(o, current) {
			s.idx = i
		}
	}
	return s
}

func (s *selector) value() string {
	if s.idx < 0 || s.idx >= len(s.options) {
		return ""
	}
	return s.options[s.idx]
}

func (s *selector) set(v string) {
	for i, o := range s.options {
		if strings.EqualFold(o, v) {
			s.idx = i
			return
		}
	}
}

func (s *selector) next() {
	if len(s.options) == 0 {
		return
	}
	s.idx = (s.idx + 1) % len(s.options)
}

func (s *selector) prev() {
	if len(s.options) == 0 {
		return
	}
	s.idx = (s.idx - 1 + len(s.options)) % len(s.options)
}

func (s *selector) render(st Styles, active bool) string {
	var b strings.Builder
	for i, o := range s.options {
		style := st.Selector
		token := "  " + o + "  "
		if i == s.idx {
			if active {
				style = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#0b0b0b")).Background(st.T.Accent).Bold(true)
			} else {
				style = st.SelectorActive
			}
			token = " [" + o + "] "
		}
		b.WriteString(style.Render(token))
	}
	return b.String()
}

// bar draws a horizontal meter of width w for value/maxv in [0,1] range scaled.
func bar(st Styles, ratio float64, w int, color lipgloss.Color) string {
	if w < 1 {
		w = 1
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	fill := int(ratio*float64(w) + 0.5)
	full := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", fill))
	empty := st.Dim.Render(strings.Repeat("░", w-fill))
	return full + empty
}

// padRight truncates or pads s to exactly w display columns.
func padRight(s string, w int) string {
	l := lipgloss.Width(s)
	if l == w {
		return s
	}
	if l > w {
		return truncate(s, w)
	}
	return s + strings.Repeat(" ", w-l)
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	// rune-aware trim
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// keyHint renders "key action" pairs for a help line.
func keyHint(st Styles, pairs ...[2]string) string {
	var parts []string
	for _, p := range pairs {
		parts = append(parts, st.Key.Render(p[0])+" "+st.Help.Render(p[1]))
	}
	return strings.Join(parts, st.Help.Render("  ·  "))
}
