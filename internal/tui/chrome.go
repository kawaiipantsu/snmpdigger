package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/kawaiipantsu/snmpdigger/internal/version"
)

// --- header (top-right, extended width) ---

func (m *Model) renderHeader(width int) string {
	st := m.st
	logo := renderLogo(st)
	logoW := lipgloss.Width(logo)
	logoH := lipgloss.Height(logo)

	boxInner := width - logoW - 3
	if boxInner < 10 {
		boxInner = 10
	}

	var status string
	if m.connected {
		status = st.Good.Render("● CONNECTED")
	} else {
		status = st.Bad.Render("● NO CONNECTION")
	}

	target := m.connTarget
	if target == "" {
		target = "—"
	}
	desc := m.connDesc
	if desc == "" {
		desc = "press  c  to open the SNMP connection dialog"
	}
	if m.cfg.UI.MaskSecrets {
		desc = maskSecrets(desc)
	}

	sysName := m.sys.Name
	if sysName == "" {
		sysName = st.Dim.Render("unidentified")
	} else {
		sysName = st.HeaderVal.Render(sysName)
	}

	lines := []string{
		lipgloss.JoinHorizontal(lipgloss.Top,
			st.HeaderTitle.Render("SNMP CONNECTION"), "   ", status),
		st.HeaderKey.Render("target  ") + st.HeaderVal.Render(target) +
			st.HeaderKey.Render("    host  ") + sysName,
		st.HeaderKey.Render("params  ") + st.Dim.Render(truncate(desc, boxInner-8)),
		st.HeaderKey.Render("role    ") + st.Accent.Render(truncate(orDash2(m.sys.Role), boxInner-8)),
	}
	for len(lines) < logoH-2 {
		lines = append(lines, "")
	}

	box := st.HeaderBox.Width(boxInner).Height(logoH - 2).
		Render(strings.Join(lines, "\n"))

	return lipgloss.JoinHorizontal(lipgloss.Top, logo, " ", box)
}

// --- tab bar ---

func (m *Model) renderTabs(width int) string {
	st := m.st
	var parts []string
	for i, name := range tabNames {
		label := fmt.Sprintf("F%d %s", i+1, name)
		if tabID(i) == m.activeTab {
			parts = append(parts, st.TabActive.Render(label))
		} else {
			parts = append(parts, st.TabInactive.Render(label))
		}
	}
	bar := strings.Join(parts, st.Dim.Render("│"))
	rule := st.Dim.Render(strings.Repeat("─", maxInt(0, width-lipgloss.Width(bar)-1)))
	return lipgloss.JoinHorizontal(lipgloss.Bottom, bar, " ", rule)
}

// --- footer ---

func (m *Model) renderFooter(width int) string {
	st := m.st

	var icon string
	switch {
	case m.statusBusy:
		icon = st.Accent.Render(m.spin.View()) + " "
	case m.statusLvl == stGood:
		icon = st.Good.Render("✔ ")
	case m.statusLvl == stWarn:
		icon = st.Warn.Render("▲ ")
	case m.statusLvl == stBad:
		icon = st.Bad.Render("✖ ")
	default:
		icon = st.Dim.Render("· ")
	}
	left := icon + st.StatusText.Render(m.statusText)

	clock := m.now.Format("15:04:05")
	right := st.FooterRight.Render(fmt.Sprintf("%s %s (c) 2026 ", version.Name, version.Short())) +
		st.Thugs.Render("THUGS") + " " + st.Clock.Render(clock)

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		left = truncate(left, maxInt(1, width-lipgloss.Width(right)-1))
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) renderHelpLine(width int) string {
	st := m.st
	var h string
	switch m.activeTab {
	case tabSystem:
		h = m.system.help()
	case tabInterfaces:
		h = m.ifaces.help()
	case tabBrowser:
		h = m.browser.help()
	case tabGraph:
		h = m.graph.help()
	case tabSummary:
		h = m.summary.help()
	case tabWatch:
		h = m.watch.help()
	case tabTraps:
		h = m.traps.help()
	case tabDiscovery:
		h = m.discovery.help()
	case tabCatalog:
		h = m.catalog.help()
	case tabSettings:
		h = m.settings.help()
	}
	global := st.Dim.Render("  ·  ") + st.Key.Render("F1-F10") + st.Help.Render(" tabs") +
		st.Dim.Render("  ·  ") + st.Key.Render("c") + st.Help.Render(" connect") +
		st.Dim.Render("  ·  ") + st.Key.Render("q") + st.Help.Render(" quit")
	return truncate(st.Help.Render(h)+global, width)
}

func maskSecrets(s string) string {
	for _, key := range []string{"community=", "auth=", "priv="} {
		idx := strings.Index(s, key)
		if idx < 0 {
			continue
		}
		start := idx + len(key)
		end := start
		for end < len(s) && s[end] != ' ' {
			end++
		}
		if end > start {
			s = s[:start] + strings.Repeat("•", end-start) + s[end:]
		}
	}
	return s
}

func orDash2(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
