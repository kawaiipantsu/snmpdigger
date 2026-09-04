package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kawaiipantsu/snmpdigger/internal/snmp"
)

type systemView struct {
	scroll int
}

func (v *systemView) help() string {
	return "↑/↓ scroll   ·   r re-scan identity"
}

func (v *systemView) pollOIDs(m *Model) []string {
	if !m.connected {
		return nil
	}
	return []string{
		snmp.OIDsysUpTime, snmp.OIDsysContact, snmp.OIDsysName,
		snmp.OIDsysLocation, snmp.OIDsysDescr,
	}
}

func (v *systemView) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if v.scroll > 0 {
				v.scroll--
			}
		case "down", "j":
			v.scroll++
		case "r":
			if m.connected {
				return tea.Batch(
					status("Re-scanning device identity…", stInfo, true),
					discoverSystemCmd(m.src),
				)
			}
		}
	case pollResultMsg:
		if msg.scope == "system" && msg.err == nil {
			for _, x := range msg.vars {
				switch x.OID {
				case snmp.OIDsysUpTime:
					m.sys.Uptime = time.Duration(x.Num) * 10 * time.Millisecond
				case snmp.OIDsysContact:
					m.sys.Contact = x.Str
				case snmp.OIDsysName:
					m.sys.Name = x.Str
				case snmp.OIDsysLocation:
					m.sys.Location = x.Str
				case snmp.OIDsysDescr:
					m.sys.Descr = x.Str
				}
			}
		}
	}
	return nil
}

func (v *systemView) view(m *Model) string {
	st := m.st
	if !m.connected {
		return centeredHint(m, "Not connected. Press "+st.Key.Render("c")+" to open the connection dialog.")
	}
	s := m.sys

	kv := func(k, val string) string {
		if val == "" {
			val = st.Dim.Render("—")
		}
		return st.Dim.Render(padRight(k, 14)) + " " + val
	}

	identity := []string{
		st.PanelTitle.Render("IDENTITY"),
		kv("sysName", st.HeaderVal.Render(s.Name)),
		kv("Role", st.Accent.Render(s.Role)),
		kv("Vendor", valueOrDim(st, s.Vendor)),
		kv("sysObjectID", valueOrDim(st, s.ObjectID)),
		kv("Uptime", st.Good.Render(humanDur(s.Uptime))),
		kv("Services", valueOrDim(st, strings.Join(s.Layers, ", "))),
	}

	contact := []string{
		st.PanelTitle.Render("LOCATION & OWNERSHIP"),
		kv("sysContact", valueOrDim(st, s.Contact)),
		kv("sysLocation", valueOrDim(st, s.Location)),
		kv("Reachable", boolBadge(st, s.Reachable)),
		kv("RTT", valueOrDim(st, fmt.Sprintf("%d ms", s.RTT.Milliseconds()))),
	}

	descrWrap := lipgloss.NewStyle().Width(m.cw - 6).Render(s.Descr)
	descr := []string{
		st.PanelTitle.Render("sysDescr"),
		valueOrDim(st, descrWrap),
	}

	analysis := []string{st.PanelTitle.Render("ANALYSIS")}
	if len(s.Notes) == 0 {
		analysis = append(analysis, st.Good.Render("• nothing unusual detected"))
	}
	for _, n := range s.Notes {
		analysis = append(analysis, st.Warn.Render("• "+n))
	}

	col := lipgloss.NewStyle().Width((m.cw / 2) - 3)
	left := col.Render(st.Panel.Render(strings.Join(identity, "\n")) + "\n" +
		st.Panel.Render(strings.Join(analysis, "\n")))
	right := col.Render(st.Panel.Render(strings.Join(contact, "\n")) + "\n" +
		st.Panel.Render(strings.Join(descr, "\n")))

	grid := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	return clip(grid, v.scroll, m.ch)
}

func valueOrDim(st Styles, s string) string {
	if strings.TrimSpace(s) == "" {
		return st.Dim.Render("—")
	}
	return st.HeaderVal.Render(s)
}

func boolBadge(st Styles, b bool) string {
	if b {
		return st.Good.Render("yes")
	}
	return st.Bad.Render("no")
}

func humanDur(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	mn := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %02dh %02dm %02ds", days, h, mn, s)
	}
	return fmt.Sprintf("%02dh %02dm %02ds", h, mn, s)
}

func centeredHint(m *Model, text string) string {
	return lipgloss.Place(m.cw, m.ch, lipgloss.Center, lipgloss.Center, text)
}

// clip renders content offset by scroll lines, capped to height rows.
func clip(content string, scroll, height int) string {
	lines := strings.Split(content, "\n")
	if scroll > len(lines)-1 {
		scroll = len(lines) - 1
	}
	if scroll < 0 {
		scroll = 0
	}
	lines = lines[scroll:]
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}
