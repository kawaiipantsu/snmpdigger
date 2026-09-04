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

	outerW := m.cw - 1
	if outerW > 108 {
		outerW = 108 // keep the column readable on very wide terminals
	}
	innerW := outerW - 4 // minus border (2) and padding (2)
	valW := innerW - 15
	if valW < 8 {
		valW = 8
	}
	panel := st.Panel.Width(innerW)

	kv := func(k, val string, style lipgloss.Style) string {
		if strings.TrimSpace(val) == "" {
			return st.Dim.Render(padRight(k, 13)) + " " + st.Dim.Render("—")
		}
		return st.Dim.Render(padRight(k, 13)) + " " + style.Render(truncate(val, valW))
	}

	identity := strings.Join([]string{
		st.PanelTitle.Render("IDENTITY"),
		kv("sysName", s.Name, st.HeaderVal),
		kv("Role", s.Role, st.Accent),
		kv("Vendor", s.Vendor, st.HeaderVal),
		kv("sysObjectID", s.ObjectID, st.HeaderVal),
		kv("Uptime", humanDur(s.Uptime), st.Good),
		kv("Services", strings.Join(s.Layers, ", "), st.HeaderVal),
	}, "\n")

	contact := strings.Join([]string{
		st.PanelTitle.Render("LOCATION & OWNERSHIP"),
		kv("sysContact", s.Contact, st.HeaderVal),
		kv("sysLocation", s.Location, st.HeaderVal),
		kv("Reachable", boolWord(s.Reachable), boolStyle(st, s.Reachable)),
		kv("RTT", fmt.Sprintf("%d ms", s.RTT.Milliseconds()), st.HeaderVal),
	}, "\n")

	descrBody := lipgloss.NewStyle().Width(innerW).Render(orDashText(s.Descr))
	descr := st.PanelTitle.Render("sysDescr") + "\n" + st.Dim.Render(descrBody)

	analysisLines := []string{st.PanelTitle.Render("ANALYSIS")}
	if len(s.Notes) == 0 {
		analysisLines = append(analysisLines, st.Good.Render("• nothing unusual detected"))
	}
	for _, n := range s.Notes {
		analysisLines = append(analysisLines,
			st.Warn.Render("• ")+lipgloss.NewStyle().Foreground(st.T.Warn).Width(innerW-2).Render(n))
	}
	analysis := strings.Join(analysisLines, "\n")

	stack := lipgloss.JoinVertical(lipgloss.Left,
		panel.Render(identity),
		panel.Render(contact),
		panel.Render(descr),
		panel.Render(analysis),
	)
	return clip(stack, v.scroll, m.ch)
}

func boolWord(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func boolStyle(st Styles, b bool) lipgloss.Style {
	if b {
		return st.Good
	}
	return st.Bad
}

func orDashText(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
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
