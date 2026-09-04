package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kawaiipantsu/snmpdigger/internal/config"
	"github.com/kawaiipantsu/snmpdigger/internal/snmp"
)

type discoveryView struct {
	cidr  textinput.Model
	comm  textinput.Model
	ver   selector
	focus int // 0 cidr, 1 comm, 2 version, 3 button

	scanning bool
	done     int
	total    int
	started  time.Time
	found    []snmp.Found
	tbl      table.Model
	err      string

	ch     chan scanUpdateMsg
	cancel context.CancelFunc
}

func newDiscoveryView(st Styles, last config.Connection) discoveryView {
	mk := func(v, ph string, w int) textinput.Model {
		ti := textinput.New()
		ti.SetValue(v)
		ti.Placeholder = ph
		ti.Prompt = ""
		ti.Width = w
		return ti
	}
	t := table.New(table.WithFocused(false), table.WithHeight(10),
		table.WithColumns([]table.Column{
			{Title: "IP", Width: 15}, {Title: "DEVICE", Width: 20}, {Title: "SNMP", Width: 5},
			{Title: "UPTIME", Width: 14}, {Title: "SYSNAME", Width: 18}, {Title: "INFO", Width: 20},
		}))
	applyTableTheme(&t, st)
	return discoveryView{
		cidr: mk("192.168.1.0/24", "10.0.0.0/24", 24),
		comm: mk(orDefault(last.Community, "public"), "public,private", 24),
		ver:  newSelector("ver", []string{"v1", "v2c"}, "v2c"),
		tbl:  t,
	}
}

func (v *discoveryView) help() string {
	if v.scanning {
		return "esc cancel scan   ·   scanning…"
	}
	return "tab/↑/↓ move · ←/→ version · enter: start scan — or, on a result row, open a pre-filled connect dialog · ctrl+r rescan"
}

func (v *discoveryView) pollOIDs(*Model) []string { return nil }

func (v *discoveryView) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case scanUpdateMsg:
		v.done, v.total = msg.done, msg.total
		if msg.err != nil && msg.final {
			v.scanning = false
			v.err = msg.err.Error()
			return status("Discovery error: "+msg.err.Error(), stBad, false)
		}
		if msg.final {
			v.scanning = false
			v.found = msg.found
			v.fillTable()
			if len(v.found) > 0 {
				v.focus = 3 // jump to the results so enter = fast-connect
				v.applyFocus()
			}
			return status(fmt.Sprintf("Discovery complete — %d SNMP device(s) in %s · enter on a row to connect",
				len(v.found), time.Since(v.started).Round(time.Millisecond)), stGood, false)
		}
		return waitScan(v.ch)

	case tea.KeyMsg:
		if v.scanning {
			if msg.String() == "esc" && v.cancel != nil {
				v.cancel()
				return status("Cancelling discovery…", stWarn, true)
			}
			return nil
		}
		switch msg.String() {
		case "tab":
			v.focus = (v.focus + 1) % 4
			v.applyFocus()
			return nil
		case "shift+tab":
			v.focus = (v.focus + 3) % 4
			v.applyFocus()
			return nil
		case "up", "down", "pgup", "pgdown":
			if v.focus == 3 { // navigate the results table
				var cmd tea.Cmd
				v.tbl, cmd = v.tbl.Update(msg)
				return cmd
			}
			if msg.String() == "up" {
				v.focus = (v.focus + 3) % 4
			} else {
				v.focus = (v.focus + 1) % 4
			}
			v.applyFocus()
			return nil
		case "left":
			if v.focus == 2 {
				v.ver.prev()
			}
			return nil
		case "right":
			if v.focus == 2 {
				v.ver.next()
			}
			return nil
		case "enter":
			if v.focus == 3 {
				if conn, ok := v.connectTarget(); ok {
					return func() tea.Msg { return openConnectMsg{conn: conn} }
				}
				return status("no discovered host selected", stWarn, false)
			}
			return v.startScan(m)
		case "ctrl+r":
			return v.startScan(m)
		}
		var cmd tea.Cmd
		switch v.focus {
		case 0:
			v.cidr, cmd = v.cidr.Update(msg)
		case 1:
			v.comm, cmd = v.comm.Update(msg)
		}
		return cmd
	}
	return nil
}

func (v *discoveryView) applyFocus() {
	v.cidr.Blur()
	v.comm.Blur()
	v.tbl.Blur()
	switch v.focus {
	case 0:
		v.cidr.Focus()
	case 1:
		v.comm.Focus()
	case 3:
		v.tbl.Focus()
	}
}

func (v *discoveryView) selectedRow() string {
	r := v.tbl.SelectedRow()
	if r == nil {
		return ""
	}
	return r[0]
}

// ConnectTarget returns a Connection for the highlighted result, if any.
func (v *discoveryView) connectTarget() (config.Connection, bool) {
	r := v.tbl.SelectedRow()
	if r == nil {
		return config.Connection{}, false
	}
	ip := r[0]
	for _, f := range v.found {
		if f.IP == ip {
			return config.Connection{
				Host: f.IP, Port: f.Port, Version: f.Version, Community: f.Community,
			}, true
		}
	}
	return config.Connection{}, false
}

func (v *discoveryView) startScan(m *Model) tea.Cmd {
	cidr := strings.TrimSpace(v.cidr.Value())
	if cidr == "" {
		v.err = "enter a CIDR range or IP"
		return nil
	}
	var comms []string
	for _, c := range strings.Split(v.comm.Value(), ",") {
		if c = strings.TrimSpace(c); c != "" {
			comms = append(comms, c)
		}
	}
	if len(comms) == 0 {
		comms = []string{"public"}
	}

	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	v.ch = make(chan scanUpdateMsg, 128)
	v.scanning = true
	v.err = ""
	v.done, v.total = 0, 0
	v.found = nil
	v.started = time.Now()
	v.tbl.SetRows(nil)

	opts := snmp.ScanOptions{
		CIDR:        cidr,
		Port:        161,
		Version:     v.ver.value(),
		Communities: comms,
		Base:        m.cfg.Last,
		Timeout:     time.Duration(m.cfg.Poll.TimeoutSeconds) * time.Second,
		Retries:     0,
		Concurrency: 128,
	}

	ch := v.ch
	go func() {
		found, err := snmp.ScanCIDR(ctx, opts, func(d, t int) {
			select {
			case ch <- scanUpdateMsg{done: d, total: t}:
			default:
			}
		})
		ch <- scanUpdateMsg{done: len(found), total: len(found), found: found, err: err, final: true}
	}()

	return tea.Batch(
		status("Scanning "+cidr+" for SNMP agents…", stInfo, true),
		waitScan(ch),
	)
}

func waitScan(ch chan scanUpdateMsg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func (v *discoveryView) fillTable() {
	rows := make([]table.Row, 0, len(v.found))
	for _, f := range v.found {
		rows = append(rows, table.Row{
			f.IP, truncate(f.Short(), 20), f.Version,
			shortDur(f.Uptime), truncate(f.SysName, 18), truncate(f.SysDescr, 60),
		})
	}
	v.tbl.SetRows(rows)
}

func (v *discoveryView) layout(m *Model) {
	w := m.cw
	v.tbl.SetWidth(w)
	v.tbl.SetHeight(clampInt(m.ch-9, 3, 400))
	v.tbl.SetColumns([]table.Column{
		{Title: "IP", Width: 15},
		{Title: "DEVICE", Width: 20},
		{Title: "SNMP", Width: 5},
		{Title: "UPTIME", Width: 14},
		{Title: "SYSNAME", Width: 18},
		{Title: "INFO", Width: clampInt(w-15-20-5-14-18-2*6, 10, 80)},
	})
}

func (v *discoveryView) view(m *Model) string {
	st := m.st
	v.layout(m)

	fld := func(idx int, label string, ti textinput.Model) string {
		lbl := st.Label
		box := st.Field
		if v.focus == idx {
			lbl = st.LabelActive
			box = st.FieldActive
		}
		return lipgloss.JoinHorizontal(lipgloss.Top,
			lbl.Render(label+" "), "  ", box.Render(padRight(ti.View(), ti.Width+1)))
	}
	verLbl := st.Label
	if v.focus == 2 {
		verLbl = st.LabelActive
	}
	verRow := lipgloss.JoinHorizontal(lipgloss.Top,
		verLbl.Render("Version "), "  ", v.ver.render(st, v.focus == 2))

	btnStyle := lipgloss.NewStyle().Foreground(st.T.Dim).Padding(0, 2).
		Border(lipgloss.RoundedBorder()).BorderForeground(st.T.Faint)
	if v.focus == 3 {
		btnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#0b0b0b")).
			Background(st.T.Accent).Bold(true).Padding(0, 2).
			Border(lipgloss.RoundedBorder()).BorderForeground(st.T.Accent)
	}
	btn := btnStyle.Render("▶ Start scan")

	form := lipgloss.JoinVertical(lipgloss.Left,
		fld(0, "CIDR / IP", v.cidr),
		fld(1, "Communities", v.comm),
		verRow,
		"",
		btn,
	)

	var progress string
	switch {
	case v.scanning:
		ratio := 0.0
		if v.total > 0 {
			ratio = float64(v.done) / float64(v.total)
		}
		progress = m.spin.View() + " " + bar(st, ratio, minInt(m.cw-24, 48), st.T.Accent) +
			fmt.Sprintf("  %d/%d  (%d found)", v.done, v.total, len(v.found))
	case v.err != "":
		progress = st.Bad.Render("✖ " + v.err)
	case len(v.found) > 0:
		progress = st.Good.Render(fmt.Sprintf("✔ %d SNMP device(s) discovered", len(v.found)))
	default:
		progress = st.Dim.Render("enter a range and press Start scan (or `snmpdigger discover <cidr>` on the CLI)")
	}

	panel := st.Panel.Render(lipgloss.JoinVertical(lipgloss.Left,
		st.PanelTitle.Render("NETWORK DISCOVERY"), "", form, "", progress))

	return lipgloss.JoinVertical(lipgloss.Left, panel, "", v.tbl.View())
}

func (v *discoveryView) setTheme(st Styles) {
	applyTableTheme(&v.tbl, st)
}

func applyTableTheme(t *table.Model, st Styles) {
	ts := table.DefaultStyles()
	ts.Header = ts.Header.Foreground(st.T.Accent).Bold(true).
		BorderStyle(lipgloss.NormalBorder()).BorderForeground(st.T.Border).BorderBottom(true)
	ts.Selected = ts.Selected.Foreground(lipgloss.Color("#0b0b0b")).Background(st.T.Accent).Bold(true)
	ts.Cell = ts.Cell.Foreground(st.T.Fg)
	t.SetStyles(ts)
}

func shortDur(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	mn := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd%02dh%02dm", days, h, mn)
	}
	return fmt.Sprintf("%02dh%02dm", h, mn)
}
