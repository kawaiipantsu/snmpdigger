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

// discovery focus slots
const (
	dfMode = iota
	dfTarget
	dfComm
	dfVer
	dfButton
	dfResults
)

type discoveryView struct {
	mode   selector // CIDR | ASN | LOCAL
	target textinput.Model
	comm   textinput.Model
	ver    selector
	focus  int

	scanning  bool
	resolving bool
	done      int
	total     int
	started   time.Time
	found     []snmp.Found
	tbl       table.Model
	err       string
	note      string
	exported  string

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
	t := table.New(table.WithFocused(false), table.WithHeight(10))
	applyTableTheme(&t, st)
	return discoveryView{
		mode:   newSelector("mode", []string{"CIDR", "ASN", "LOCAL"}, "CIDR"),
		target: mk("192.168.1.0/24", "10.0.0.0/24  or  AS13335", 30),
		comm:   mk(orDefault(last.Community, "public"), "public,private", 22),
		ver:    newSelector("ver", []string{"v1", "v2c"}, "v2c"),
		tbl:    t,
		focus:  dfTarget,
	}
}

func (v *discoveryView) curMode() string { return v.mode.value() }

func (v *discoveryView) slots() []int {
	if v.curMode() == "LOCAL" {
		return []int{dfMode, dfComm, dfVer, dfButton, dfResults}
	}
	return []int{dfMode, dfTarget, dfComm, dfVer, dfButton, dfResults}
}

func (v *discoveryView) help() string {
	if v.scanning {
		return "esc cancel scan   ·   scanning… (large ranges take a while)"
	}
	if v.resolving {
		return "resolving ASN prefixes…"
	}
	return "tab/↑/↓ field · ←/→ change · enter scan / connect row · esc leave field · [ ] switch tab · e export"
}

func (v *discoveryView) pollOIDs(*Model) []string { return nil }

func (v *discoveryView) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case asnResolvedMsg:
		v.resolving = false
		if msg.err != nil {
			v.err = msg.err.Error()
			return status("ASN lookup failed: "+msg.err.Error(), stBad, false)
		}
		v.note = fmt.Sprintf("%s (%s) → %d IPv4 prefixes, %s",
			msg.info.ASN, dashText(msg.info.Holder), len(msg.info.Prefixes),
			snmp.SummarizeTargets(msg.info.Prefixes))
		return v.launch(m, msg.info.Prefixes, 1<<21)

	case scanUpdateMsg:
		v.done, v.total = msg.done, msg.total
		if msg.final {
			v.scanning = false
			if msg.err != nil && len(msg.found) == 0 {
				v.err = msg.err.Error()
				return status("Discovery error: "+msg.err.Error(), stBad, false)
			}
			v.found = msg.found
			v.fillTable()
			if len(v.found) > 0 {
				v.setFocus(dfResults)
			}
			suffix := ""
			if msg.err != nil {
				suffix = " (stopped early: " + msg.err.Error() + ")"
			}
			return status(fmt.Sprintf("Discovery complete — %d SNMP device(s) in %s%s · enter on a row to connect",
				len(v.found), time.Since(v.started).Round(time.Millisecond), suffix), stGood, false)
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
		case "esc":
			// step off any text field so [ ] / digit keys work again
			v.err = ""
			if len(v.found) > 0 {
				v.setFocus(dfResults)
			} else {
				v.setFocus(dfButton)
			}
			return nil
		case "tab":
			v.moveFocus(1)
			return nil
		case "shift+tab":
			v.moveFocus(-1)
			return nil
		case "up", "k", "down", "j", "pgup", "pgdown":
			if v.focus == dfResults {
				var cmd tea.Cmd
				v.tbl, cmd = v.tbl.Update(msg)
				return cmd
			}
			if msg.String() == "up" || msg.String() == "k" || msg.String() == "pgup" {
				v.moveFocus(-1)
			} else {
				v.moveFocus(1)
			}
			return nil
		case "left":
			if v.focus == dfMode {
				v.mode.prev()
				v.fixFocus()
			} else if v.focus == dfVer {
				v.ver.prev()
			}
			return nil
		case "right":
			if v.focus == dfMode {
				v.mode.next()
				v.fixFocus()
			} else if v.focus == dfVer {
				v.ver.next()
			}
			return nil
		case "e":
			if v.focus != dfTarget && v.focus != dfComm {
				return v.export()
			}
		case "enter":
			if v.focus == dfResults {
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
		case dfTarget:
			v.target, cmd = v.target.Update(msg)
		case dfComm:
			v.comm, cmd = v.comm.Update(msg)
		}
		return cmd
	}
	return nil
}

func (v *discoveryView) moveFocus(dir int) {
	sl := v.slots()
	cur := 0
	for i, s := range sl {
		if s == v.focus {
			cur = i
		}
	}
	cur = (cur + dir + len(sl)) % len(sl)
	v.setFocus(sl[cur])
}

func (v *discoveryView) fixFocus() {
	for _, s := range v.slots() {
		if s == v.focus {
			v.setFocus(v.focus)
			return
		}
	}
	v.setFocus(dfMode)
}

func (v *discoveryView) setFocus(f int) {
	v.focus = f
	v.target.Blur()
	v.comm.Blur()
	v.tbl.Blur()
	switch f {
	case dfTarget:
		v.target.Focus()
	case dfComm:
		v.comm.Focus()
	case dfResults:
		v.tbl.Focus()
	}
}

func (v *discoveryView) connectTarget() (config.Connection, bool) {
	r := v.tbl.SelectedRow()
	if r == nil {
		return config.Connection{}, false
	}
	for _, f := range v.found {
		if f.IP == r[0] {
			return config.Connection{Host: f.IP, Port: f.Port, Version: f.Version, Community: f.Community}, true
		}
	}
	return config.Connection{}, false
}

func (v *discoveryView) communities() []string {
	var out []string
	for _, c := range strings.Split(v.comm.Value(), ",") {
		if c = strings.TrimSpace(c); c != "" {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		out = []string{"public"}
	}
	return out
}

func (v *discoveryView) startScan(m *Model) tea.Cmd {
	v.err = ""
	v.note = ""
	v.exported = ""
	switch v.curMode() {
	case "ASN":
		asn := strings.TrimSpace(v.target.Value())
		if asn == "" {
			v.err = "enter an AS number, e.g. AS13335"
			return nil
		}
		v.resolving = true
		return tea.Batch(
			status("Resolving prefixes for "+asn+" …", stInfo, true),
			resolveASNCmd(asn),
		)
	case "LOCAL":
		targets := snmp.LocalScanTargets()
		if len(targets) == 0 {
			v.err = "no local networks detected"
			return nil
		}
		v.note = "local + common LAN ranges: " + snmp.SummarizeTargets(targets)
		return v.launch(m, targets, snmp.DefaultMaxHosts)
	default: // CIDR
		cidr := strings.TrimSpace(v.target.Value())
		if cidr == "" {
			v.err = "enter a CIDR range or IP"
			return nil
		}
		return v.launch(m, []string{cidr}, snmp.DefaultMaxHosts)
	}
}

// launch kicks off the concurrent sweep over the given targets.
func (v *discoveryView) launch(m *Model, targets []string, maxHosts int) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	v.ch = make(chan scanUpdateMsg, 256)
	v.scanning = true
	v.done, v.total = 0, 0
	v.found = nil
	v.started = time.Now()
	v.tbl.SetRows(nil)

	opts := snmp.ScanOptions{
		Targets:     targets,
		Port:        161,
		Version:     v.ver.value(),
		Communities: v.communities(),
		Base:        m.cfg.Last,
		Timeout:     time.Duration(m.cfg.Poll.TimeoutSeconds) * time.Second,
		Retries:     0,
		Concurrency: 192,
		MaxHosts:    maxHosts,
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
		status("Scanning "+snmp.SummarizeTargets(targets)+" for SNMP agents…", stInfo, true),
		waitScan(ch),
	)
}

func waitScan(ch chan scanUpdateMsg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func (v *discoveryView) export() tea.Cmd {
	if len(v.found) == 0 {
		return status("nothing to export yet", stWarn, false)
	}
	rows := make([][]string, 0, len(v.found))
	for _, f := range v.found {
		rows = append(rows, []string{
			f.IP, f.Version, f.Community, f.Short(), f.SysName,
			f.SysObjectID, f.Vendor, f.Role, shortDur(f.Uptime),
			fmt.Sprintf("%d", f.RTT.Milliseconds()), oneLineText(f.SysDescr),
			oneLineText(f.SysContact), oneLineText(f.SysLocation),
		})
	}
	path, err := exportCSV("discovery",
		[]string{"ip", "snmp", "community", "device", "sysName", "sysObjectID",
			"vendor", "role", "uptime", "rtt_ms", "sysDescr", "sysContact", "sysLocation"},
		rows)
	if err != nil {
		return status("export failed: "+err.Error(), stBad, false)
	}
	v.exported = path
	return status("exported "+fmt.Sprintf("%d", len(v.found))+" rows → "+path, stGood, false)
}

func (v *discoveryView) fillTable() {
	rows := make([]table.Row, 0, len(v.found))
	for _, f := range v.found {
		rows = append(rows, table.Row{
			f.IP, truncate(f.Short(), 22), f.Version,
			shortDur(f.Uptime), truncate(f.SysName, 20), truncate(oneLineText(f.SysDescr), 70),
		})
	}
	v.tbl.SetRows(rows)
}

func (v *discoveryView) layout(m *Model, formH int) {
	w := m.cw
	v.tbl.SetWidth(w)
	v.tbl.SetHeight(clampInt(m.ch-formH-2, 3, 400))
	v.tbl.SetColumns([]table.Column{
		{Title: "IP", Width: 15},
		{Title: "DEVICE", Width: 22},
		{Title: "SNMP", Width: 5},
		{Title: "UPTIME", Width: 12},
		{Title: "SYSNAME", Width: 20},
		{Title: "INFO", Width: clampInt(w-15-22-5-12-20-2*6, 10, 90)},
	})
}

func (v *discoveryView) view(m *Model) string {
	st := m.st

	lbl := func(f int) lipgloss.Style {
		if v.focus == f {
			return lipgloss.NewStyle().Foreground(st.T.Accent).Bold(true)
		}
		return st.Dim
	}
	fieldBox := func(f int, ti textinput.Model, w int) string {
		box := st.Field
		if v.focus == f {
			box = st.FieldActive
		}
		return box.Render(padRight(ti.View(), w))
	}

	// Row 1: mode + target
	row1 := lipgloss.JoinHorizontal(lipgloss.Top,
		lbl(dfMode).Render("mode "), v.mode.render(st, v.focus == dfMode))
	if v.curMode() != "LOCAL" {
		tl := "range "
		if v.curMode() == "ASN" {
			tl = "AS no "
		}
		row1 = lipgloss.JoinHorizontal(lipgloss.Top, row1,
			st.Dim.Render("   "+tl), fieldBox(dfTarget, v.target, 34))
	} else {
		row1 = lipgloss.JoinHorizontal(lipgloss.Top, row1,
			st.Dim.Render("   "), st.Dim.Render("(scans this host's private nets + common LAN /24s)"))
	}

	// Row 2: communities + version
	row2 := lipgloss.JoinHorizontal(lipgloss.Top,
		lbl(dfComm).Render("communities "), fieldBox(dfComm, v.comm, 25),
		st.Dim.Render("   version "), v.ver.render(st, v.focus == dfVer))

	// Row 3: button + status
	btnStyle := lipgloss.NewStyle().Foreground(st.T.Dim).Padding(0, 2).
		Border(lipgloss.RoundedBorder()).BorderForeground(st.T.Faint)
	if v.focus == dfButton {
		btnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#0b0b0b")).
			Background(st.T.Accent).Bold(true).Padding(0, 2).
			Border(lipgloss.RoundedBorder()).BorderForeground(st.T.Accent)
	}
	label := "▶ Start scan"
	if v.curMode() == "LOCAL" {
		label = "▶ Scan local"
	}
	btn := btnStyle.Render(label)

	var stat string
	switch {
	case v.resolving:
		stat = m.spin.View() + st.Dim.Render(" resolving ASN prefixes via RIPEstat…")
	case v.scanning:
		ratio := 0.0
		if v.total > 0 {
			ratio = float64(v.done) / float64(v.total)
		}
		stat = m.spin.View() + " " + bar(st, ratio, minInt(m.cw-40, 40), st.T.Accent) +
			fmt.Sprintf("  %d/%d  %d found", v.done, v.total, len(v.found))
	case v.err != "":
		stat = st.Bad.Render("✖ " + v.err)
	case v.exported != "":
		stat = st.Good.Render("✔ exported → " + v.exported)
	case v.note != "":
		stat = st.Dim.Render(v.note)
	case len(v.found) > 0:
		stat = st.Good.Render(fmt.Sprintf("✔ %d device(s) · e to export CSV", len(v.found)))
	default:
		stat = st.Dim.Render("CIDR / ASN / LOCAL · also `snmpdigger discover <cidr>|--asn N|--local`")
	}
	row3 := lipgloss.JoinHorizontal(lipgloss.Top, btn, "  ", stat)

	panelInner := lipgloss.JoinVertical(lipgloss.Left,
		st.PanelTitle.Render("NETWORK DISCOVERY"), row1, row2, row3)
	panel := st.Panel.Width(m.cw - 2).Render(panelInner)
	formH := lipgloss.Height(panel)

	v.layout(m, formH)
	return lipgloss.JoinVertical(lipgloss.Left, panel, v.tbl.View())
}

func (v *discoveryView) setTheme(st Styles) { applyTableTheme(&v.tbl, st) }

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

func dashText(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
