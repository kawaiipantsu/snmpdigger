package tui

import "github.com/charmbracelet/lipgloss"

// Theme is a named colour set.
type Theme struct {
	Name   string
	Fg     lipgloss.Color
	Dim    lipgloss.Color
	Faint  lipgloss.Color
	Accent lipgloss.Color // primary brand colour (THUGS red)
	Alt    lipgloss.Color // secondary accent
	Good   lipgloss.Color
	Warn   lipgloss.Color
	Bad    lipgloss.Color
	Border lipgloss.Color
	Panel  lipgloss.Color
}

var themes = map[string]Theme{
	"thugs": {
		Name: "thugs", Fg: "#e6e6e6", Dim: "#8a8a8a", Faint: "#4a4a4a",
		Accent: "#e2223b", Alt: "#ff5c5c", Good: "#35c98b", Warn: "#f2b134",
		Bad: "#e2223b", Border: "#5a1620", Panel: "#1a0e10",
	},
	"matrix": {
		Name: "matrix", Fg: "#c8ffc8", Dim: "#4f9f4f", Faint: "#2b552b",
		Accent: "#35c98b", Alt: "#7cff7c", Good: "#35c98b", Warn: "#f2b134",
		Bad: "#ff5555", Border: "#1f4f1f", Panel: "#0b160b",
	},
	"mono": {
		Name: "mono", Fg: "#e6e6e6", Dim: "#9a9a9a", Faint: "#565656",
		Accent: "#ffffff", Alt: "#c0c0c0", Good: "#d0d0d0", Warn: "#a8a8a8",
		Bad: "#ffffff", Border: "#3a3a3a", Panel: "#141414",
	},
}

// ThemeByName returns the named theme or the thugs default.
func ThemeByName(n string) Theme {
	if t, ok := themes[n]; ok {
		return t
	}
	return themes["thugs"]
}

// Styles is the full set of pre-built lipgloss styles for one theme.
type Styles struct {
	T Theme

	App         lipgloss.Style
	LogoBox     lipgloss.Style
	LogoText    lipgloss.Style
	HeaderBox   lipgloss.Style
	HeaderKey   lipgloss.Style
	HeaderVal   lipgloss.Style
	HeaderTitle lipgloss.Style

	TabBar      lipgloss.Style
	TabActive   lipgloss.Style
	TabInactive lipgloss.Style

	Content    lipgloss.Style
	Panel      lipgloss.Style
	PanelTitle lipgloss.Style

	Footer      lipgloss.Style
	StatusText  lipgloss.Style
	FooterRight lipgloss.Style
	Thugs       lipgloss.Style
	Clock       lipgloss.Style

	Modal          lipgloss.Style
	ModalTitle     lipgloss.Style
	Label          lipgloss.Style
	LabelActive    lipgloss.Style
	Field          lipgloss.Style
	FieldActive    lipgloss.Style
	Selector       lipgloss.Style
	SelectorActive lipgloss.Style

	TableHead lipgloss.Style
	TableSel  lipgloss.Style
	TableCell lipgloss.Style

	Key    lipgloss.Style
	Help   lipgloss.Style
	Good   lipgloss.Style
	Warn   lipgloss.Style
	Bad    lipgloss.Style
	Dim    lipgloss.Style
	Accent lipgloss.Style
	Big    lipgloss.Style
}

// NewStyles builds styles for theme t.
func NewStyles(t Theme) Styles {
	base := lipgloss.NewStyle().Foreground(t.Fg)
	s := Styles{T: t}

	s.App = lipgloss.NewStyle()
	s.LogoBox = lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).BorderForeground(t.Accent).
		Padding(0, 1).Foreground(t.Accent).Bold(true)
	s.LogoText = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)

	s.HeaderBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(t.Border).
		Padding(0, 1)
	s.HeaderKey = lipgloss.NewStyle().Foreground(t.Dim)
	s.HeaderVal = lipgloss.NewStyle().Foreground(t.Fg).Bold(true)
	s.HeaderTitle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)

	s.TabBar = lipgloss.NewStyle().Padding(0, 0)
	s.TabActive = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#0b0b0b")).Background(t.Accent).
		Bold(true).Padding(0, 2)
	s.TabInactive = lipgloss.NewStyle().
		Foreground(t.Dim).Padding(0, 2)

	s.Content = lipgloss.NewStyle()
	s.Panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(t.Border).Padding(0, 1)
	s.PanelTitle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)

	s.Footer = lipgloss.NewStyle().Foreground(t.Dim)
	s.StatusText = lipgloss.NewStyle().Foreground(t.Fg)
	s.FooterRight = lipgloss.NewStyle().Foreground(t.Dim)
	s.Thugs = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	s.Clock = lipgloss.NewStyle().Foreground(t.Fg)

	s.Modal = lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).BorderForeground(t.Accent).
		Padding(1, 3).Background(t.Panel)
	s.ModalTitle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true).
		Underline(true)
	s.Label = lipgloss.NewStyle().Foreground(t.Dim).Width(16).Align(lipgloss.Right)
	s.LabelActive = lipgloss.NewStyle().Foreground(t.Accent).Bold(true).
		Width(16).Align(lipgloss.Right)
	s.Field = lipgloss.NewStyle().Foreground(t.Fg).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(t.Faint)
	s.FieldActive = lipgloss.NewStyle().Foreground(t.Fg).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(t.Accent)
	s.Selector = lipgloss.NewStyle().Foreground(t.Fg)
	s.SelectorActive = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)

	s.TableHead = lipgloss.NewStyle().Foreground(t.Accent).Bold(true).
		BorderBottom(true).BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(t.Border)
	s.TableSel = lipgloss.NewStyle().Foreground(lipgloss.Color("#0b0b0b")).
		Background(t.Accent).Bold(true)
	s.TableCell = base

	s.Key = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	s.Help = lipgloss.NewStyle().Foreground(t.Dim)
	s.Good = lipgloss.NewStyle().Foreground(t.Good)
	s.Warn = lipgloss.NewStyle().Foreground(t.Warn)
	s.Bad = lipgloss.NewStyle().Foreground(t.Bad)
	s.Dim = lipgloss.NewStyle().Foreground(t.Dim)
	s.Accent = lipgloss.NewStyle().Foreground(t.Accent)
	s.Big = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)

	return s
}
