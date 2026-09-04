package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The header logo is a self-contained 6-row x 10-col "terminal window" emblem
// (concept 4 from the brand sheet): three traffic-light dots, the SNMP / DIGGER
// wordmark and a shell prompt. It draws its own chrome so it does not depend on
// a lipgloss border.
func renderLogo(s Styles) string {
	accent := lipgloss.NewStyle().Foreground(s.T.Accent).Bold(true)
	white := lipgloss.NewStyle().Foreground(s.T.Fg).Bold(true)
	frame := lipgloss.NewStyle().Foreground(s.T.Border)

	rows := []string{
		frame.Render("╔════════╗"),
		frame.Render("║") + accent.Render("● ● ●") + frame.Render("   ║"),
		frame.Render("║ ") + white.Render("SNMP") + frame.Render("   ║"),
		frame.Render("║ ") + accent.Render("DIGGER") + frame.Render(" ║"),
		frame.Render("║ ") + accent.Render("▶") + s.Dim.Render(" dig_") + frame.Render("  ║"),
		frame.Render("╚════════╝"),
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
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
