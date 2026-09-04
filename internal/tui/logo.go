package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The header logo is a small "terminal window" emblem: a row of prompt dots,
// the SNMP / DIGGER wordmark and a shell prompt. The chrome is a real lipgloss
// border so it always stays aligned regardless of glyph widths in the terminal.
func renderLogo(s Styles) string {
	accent := lipgloss.NewStyle().Foreground(s.T.Accent).Bold(true)
	white := lipgloss.NewStyle().Foreground(s.T.Fg).Bold(true)

	inner := lipgloss.JoinVertical(lipgloss.Left,
		accent.Render("o o o"),
		white.Render("SNMP"),
		accent.Render("DIGGER"),
		lipgloss.NewStyle().Foreground(s.T.Faint).Render("> dig_"),
	)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.T.Accent).
		Padding(0, 1).
		Width(8).
		Render(inner)
}

func logoDims(s Styles) (w, h int) {
	r := renderLogo(s)
	return lipgloss.Width(r), lipgloss.Height(r)
}

// bannerWordmark is the wide ASCII wordmark for the disconnected splash screen.
var bannerWordmark = []string{
	`  ███████ ███    ██ ███    ███ ██████`,
	`  ██      ████   ██ ████  ████ ██   ██`,
	`  ███████ ██ ██  ██ ██ ████ ██ ██████`,
	`       ██ ██  ██ ██ ██  ██  ██ ██`,
	`  ███████ ██   ████ ██      ██ ██`,
	``,
	`  ██████  ██  ██████  ██████  ███████ ██████`,
	`  ██   ██ ██ ██       ██       ██     ██   ██`,
	`  ██   ██ ██ ██   ███ ██   ███ █████  ██████`,
	`  ██   ██ ██ ██    ██ ██    ██ ██     ██   ██`,
	`  ██████  ██  ██████  ██████  ███████ ██   ██`,
}

func renderWordmark(s Styles) string {
	white := lipgloss.NewStyle().Foreground(s.T.Fg).Bold(true)
	red := lipgloss.NewStyle().Foreground(s.T.Accent).Bold(true)
	var out []string
	for i, l := range bannerWordmark {
		if i < 5 {
			out = append(out, white.Render(l))
		} else {
			out = append(out, red.Render(l))
		}
	}
	return strings.Join(out, "\n")
}
