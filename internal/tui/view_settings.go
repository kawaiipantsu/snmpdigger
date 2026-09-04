package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kawaiipantsu/snmpdigger/internal/config"
)

type settingsView struct {
	cursor int
	path   string
	dirty  bool
}

const settingsCount = 10

func (v *settingsView) help() string {
	return "↑/↓ select · ←/→ adjust · s save · " + boolStr(v.dirty, "unsaved changes", "saved")
}

func (v *settingsView) pollOIDs(*Model) []string { return nil }

func (v *settingsView) update(m *Model, msg tea.Msg) tea.Cmd {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch km.String() {
	case "up", "k":
		if v.cursor > 0 {
			v.cursor--
		}
	case "down", "j":
		if v.cursor < settingsCount-1 {
			v.cursor++
		}
	case "left", "h":
		v.adjust(m, -1)
	case "right", "l":
		v.adjust(m, +1)
	case "s":
		if err := m.cfg.Save(); err != nil {
			return status("Save failed: "+err.Error(), stBad, false)
		}
		v.dirty = false
		return tea.Batch(status("Settings saved to "+v.path, stGood, false), m.reschedulePoll())
	}
	return nil
}

func (v *settingsView) adjust(m *Model, dir int) {
	c := m.cfg
	switch v.cursor {
	case 0:
		c.Poll.IntervalSeconds = clampInt(c.Poll.IntervalSeconds+dir, 1, 60)
	case 1:
		c.Poll.TimeoutSeconds = clampInt(c.Poll.TimeoutSeconds+dir, 1, 30)
	case 2:
		c.Poll.Retries = clampInt(c.Poll.Retries+dir, 0, 5)
	case 3:
		c.Poll.MaxOIDsPerReq = clampInt(c.Poll.MaxOIDsPerReq+dir*5, 1, 120)
	case 4:
		c.Poll.MaxRepetitions = clampInt(c.Poll.MaxRepetitions+dir*5, 1, 60)
	case 5:
		c.UI.GraphHistory = clampInt(c.UI.GraphHistory+dir*20, 20, 2000)
		m.graph.history = c.UI.GraphHistory
	case 6:
		c.UI.ShowNumericOID = !c.UI.ShowNumericOID
	case 7:
		c.UI.MaskSecrets = !c.UI.MaskSecrets
	case 8:
		c.UI.Theme = cycle([]string{"thugs", "ember", "matrix", "mono"}, c.UI.Theme, dir)
		m.applyTheme()
	case 9:
		c.UI.DefaultWalkScope = cycle([]string{"mib-2", "enterprises", "whole"}, c.UI.DefaultWalkScope, dir)
	}
	v.dirty = true
}

func (v *settingsView) view(m *Model) string {
	st := m.st
	c := m.cfg
	if v.path == "" {
		if p, err := config.Path(); err == nil {
			v.path = p
		}
	}

	rows := []struct {
		label, val, hint string
	}{
		{"Poll interval", fmt.Sprintf("%d s", c.Poll.IntervalSeconds), "how often live values refresh (1–5 typical)"},
		{"Request timeout", fmt.Sprintf("%d s", c.Poll.TimeoutSeconds), "per SNMP request"},
		{"Retries", fmt.Sprintf("%d", c.Poll.Retries), "resend on timeout"},
		{"Max OIDs / request", fmt.Sprintf("%d", c.Poll.MaxOIDsPerReq), "GET batching"},
		{"Max repetitions", fmt.Sprintf("%d", c.Poll.MaxRepetitions), "GETBULK tuning"},
		{"Graph history", fmt.Sprintf("%d samples", c.UI.GraphHistory), "ring buffer per graphed object"},
		{"Show numeric OID", onOff(c.UI.ShowNumericOID), "browser OID column"},
		{"Mask secrets", onOff(c.UI.MaskSecrets), "hide community / passphrases in header"},
		{"Theme", c.UI.Theme, "thugs · ember · matrix · mono"},
		{"Default walk scope", c.UI.DefaultWalkScope, "subtree walked on connect"},
	}

	var lines []string
	lines = append(lines, st.PanelTitle.Render("SETTINGS"), "")
	for i, r := range rows {
		pointer := "  "
		label := st.Dim.Render(padRight(r.label, 22))
		val := st.HeaderVal.Render(padRight("‹ "+r.val+" ›", 22))
		if i == v.cursor {
			pointer = st.Accent.Render("▶ ")
			label = st.Accent.Bold(true).Render(padRight(r.label, 22))
			val = lipgloss.NewStyle().Foreground(lipgloss.Color("#0b0b0b")).
				Background(st.T.Accent).Bold(true).Render(padRight(" ‹ "+r.val+" › ", 24))
		}
		lines = append(lines, pointer+label+" "+val+"  "+st.Help.Render(r.hint))
	}
	lines = append(lines, "", st.Dim.Render("config file: ")+st.HeaderVal.Render(v.path))
	if v.dirty {
		lines = append(lines, st.Warn.Render("● unsaved — press s to write"))
	} else {
		lines = append(lines, st.Good.Render("● saved"))
	}

	return st.Panel.Render(strings.Join(lines, "\n"))
}

func cycle(opts []string, cur string, dir int) string {
	idx := 0
	for i, o := range opts {
		if o == cur {
			idx = i
		}
	}
	idx = (idx + dir + len(opts)) % len(opts)
	return opts[idx]
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
func boolStr(b bool, t, f string) string {
	if b {
		return t
	}
	return f
}
