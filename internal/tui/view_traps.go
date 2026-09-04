package tui

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	g "github.com/gosnmp/gosnmp"

	"github.com/kawaiipantsu/snmpdigger/internal/snmp"
)

// --- messages ---------------------------------------------------------------

type trapMsg struct {
	rec     *trapRecord
	err     error
	stopped bool
}

func waitTrap(ch chan trapMsg) tea.Cmd {
	return func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return trapMsg{stopped: true}
		}
		return m
	}
}

// --- record ---------------------------------------------------------------

type trapVarb struct{ oid, name, typ, val string }

type trapRecord struct {
	at        time.Time
	src       string
	version   string
	community string
	inform    bool
	trapOID   string
	trapName  string
	uptime    string
	vars      []trapVarb
}

const (
	oidSnmpTrapOID = "1.3.6.1.6.3.1.1.4.1.0"
	oidSysUpTime0  = "1.3.6.1.2.1.1.3.0"
)

var v1GenericTrap = map[int]string{
	0: "coldStart", 1: "warmStart", 2: "linkDown", 3: "linkUp",
	4: "authenticationFailure", 5: "egpNeighborLoss", 6: "enterpriseSpecific",
}

// buildTrapRecord converts a received packet into a raw record (OIDs only; names
// are filled in on the main loop where the MIB resolver lives).
func buildTrapRecord(s *g.SnmpPacket, u *net.UDPAddr) trapRecord {
	r := trapRecord{
		at:        time.Now(),
		community: s.Community,
		inform:    s.IsInform,
	}
	if u != nil {
		r.src = u.IP.String()
	}
	switch s.Version {
	case g.Version1:
		r.version = "v1"
	case g.Version3:
		r.version = "v3"
	default:
		r.version = "v2c"
	}

	if s.Version == g.Version1 {
		gt := v1GenericTrap[s.GenericTrap]
		if gt == "" {
			gt = fmt.Sprintf("generic-%d", s.GenericTrap)
		}
		r.trapName = gt
		if s.GenericTrap == 6 {
			r.trapOID = strings.TrimPrefix(s.Enterprise, ".") + ".0." + fmt.Sprintf("%d", s.SpecificTrap)
		} else {
			r.trapOID = fmt.Sprintf("1.3.6.1.6.3.1.1.5.%d", s.GenericTrap+1)
		}
		if s.Timestamp > 0 {
			r.uptime = humanTrapTicks(float64(s.Timestamp))
		}
		for _, pdu := range s.Variables {
			r.vars = append(r.vars, pduToVarb(pdu))
		}
		return r
	}

	// v2c / v3
	for _, pdu := range s.Variables {
		name := strings.TrimPrefix(pdu.Name, ".")
		switch name {
		case strings.TrimPrefix(oidSnmpTrapOID, "."):
			r.trapOID = strings.TrimPrefix(fmt.Sprintf("%v", pdu.Value), ".")
		case strings.TrimPrefix(oidSysUpTime0, "."):
			r.uptime = humanTrapTicks(snmp.VarFromPDU(pdu).Num)
		default:
			r.vars = append(r.vars, pduToVarb(pdu))
		}
	}
	return r
}

func pduToVarb(pdu g.SnmpPDU) trapVarb {
	v := snmp.VarFromPDU(pdu)
	return trapVarb{oid: v.OID, typ: v.Kind.String(), val: v.Display()}
}

func humanTrapTicks(ticks float64) string {
	d := time.Duration(ticks) * 10 * time.Millisecond
	return d.Round(time.Second).String()
}

// --- view ---------------------------------------------------------------

type trapView struct {
	listening bool
	port      int
	err       string

	traps  []trapRecord
	sel    int
	top    int
	follow bool

	filter    textinput.Model
	filtering bool
	query     string
	exported  string

	tl *g.TrapListener
	ch chan trapMsg
}

var trapPortPresets = []int{162, 1162, 10162, 16200, 32162}

func newTrapView(st Styles, port int) trapView {
	ti := textinput.New()
	ti.Prompt = st.Accent.Render("/ ")
	ti.Placeholder = "filter by source / trap / oid…"
	if port <= 0 {
		port = 162
	}
	return trapView{port: port, follow: true, filter: ti}
}

func (v *trapView) setTheme(st Styles)       { v.filter.Prompt = st.Accent.Render("/ ") }
func (v *trapView) pollOIDs(*Model) []string { return nil }

func (v *trapView) help() string {
	if v.filtering {
		return "type to filter · enter/esc done"
	}
	st := "stopped"
	if v.listening {
		st = "LISTENING"
	}
	return fmt.Sprintf("l %s · P port (%d) · / filter · x clear · e export · f follow:%v",
		map[bool]string{true: "stop", false: "start"}[v.listening], v.port, v.follow) + "   [" + st + "]"
}

func (v *trapView) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case trapMsg:
		if msg.stopped {
			v.listening = false
			v.tl = nil
			if msg.err != nil {
				v.err = friendlyBindErr(msg.err, v.port)
				return status("Trap listener: "+v.err, stBad, false)
			}
			return nil
		}
		if msg.rec != nil {
			v.enrich(m, msg.rec)
			atEnd := v.sel >= len(v.traps)-1
			v.traps = append(v.traps, *msg.rec)
			if len(v.traps) > 1000 {
				v.traps = v.traps[len(v.traps)-1000:]
			}
			if v.follow || atEnd {
				v.sel = len(v.traps) - 1
			}
		}
		if v.listening {
			return waitTrap(v.ch)
		}
		return nil

	case tea.KeyMsg:
		if v.filtering {
			switch msg.String() {
			case "enter", "esc":
				v.filtering = false
				v.filter.Blur()
				if msg.String() == "esc" {
					v.query = ""
					v.filter.SetValue("")
				}
				return nil
			}
			var cmd tea.Cmd
			v.filter, cmd = v.filter.Update(msg)
			v.query = v.filter.Value()
			return cmd
		}
		vis := v.visible()
		switch msg.String() {
		case "l":
			return v.toggle(m)
		case "P":
			if !v.listening {
				v.port = nextPreset(v.port)
			}
		case "f":
			v.follow = !v.follow
		case "up", "k":
			if v.sel > 0 {
				v.sel--
				v.follow = false
			}
		case "down", "j":
			if v.sel < len(vis)-1 {
				v.sel++
			}
		case "/":
			v.filtering = true
			v.filter.Focus()
			return textinput.Blink
		case "x":
			v.traps = nil
			v.sel = 0
			return status("cleared trap log", stInfo, false)
		case "e":
			return v.export()
		}
	}
	return nil
}

func (v *trapView) toggle(m *Model) tea.Cmd {
	if v.listening {
		if v.tl != nil {
			v.tl.Close()
		}
		v.listening = false
		return status("Stopped trap listener", stInfo, false)
	}
	tl := g.NewTrapListener()
	ch := make(chan trapMsg, 512)
	v.ch = ch
	v.tl = tl
	v.err = ""
	tl.OnNewTrap = func(s *g.SnmpPacket, u *net.UDPAddr) {
		r := buildTrapRecord(s, u)
		select {
		case ch <- trapMsg{rec: &r}:
		default:
		}
	}
	addr := fmt.Sprintf("0.0.0.0:%d", v.port)
	go func() {
		err := tl.Listen(addr) // blocks until Close()
		ch <- trapMsg{err: err, stopped: true}
	}()
	v.listening = true
	m.cfg.UI.TrapPort = v.port
	_ = m.cfg.Save()
	return tea.Batch(
		status(fmt.Sprintf("Listening for SNMP traps/informs on udp/%d…", v.port), stInfo, true),
		waitTrap(ch),
	)
}

func (v *trapView) enrich(m *Model, r *trapRecord) {
	if r.trapOID != "" {
		if n := m.res.Name(r.trapOID); n != "" && n != r.trapOID {
			r.trapName = n
		}
	}
	for i := range r.vars {
		if n := m.res.Name(r.vars[i].oid); n != "" && n != r.vars[i].oid {
			r.vars[i].name = n
		}
	}
}

func (v *trapView) visible() []trapRecord {
	q := strings.ToLower(strings.TrimSpace(v.query))
	if q == "" {
		return v.traps
	}
	out := make([]trapRecord, 0, len(v.traps))
	for _, r := range v.traps {
		hay := strings.ToLower(r.src + " " + r.trapOID + " " + r.trapName + " " + r.community)
		for _, vb := range r.vars {
			hay += " " + strings.ToLower(vb.oid+" "+vb.name+" "+vb.val)
		}
		if strings.Contains(hay, q) {
			out = append(out, r)
		}
	}
	return out
}

func (v *trapView) export() tea.Cmd {
	if len(v.traps) == 0 {
		return status("no traps received yet", stWarn, false)
	}
	rows := make([][]string, 0, len(v.traps))
	for _, r := range v.traps {
		parts := make([]string, 0, len(r.vars))
		for _, vb := range r.vars {
			nm := vb.name
			if nm == "" {
				nm = vb.oid
			}
			parts = append(parts, nm+"="+vb.val)
		}
		kind := "trap"
		if r.inform {
			kind = "inform"
		}
		rows = append(rows, []string{
			r.at.Format(time.RFC3339), r.src, r.version, kind, r.community,
			r.trapOID, r.trapName, r.uptime, strings.Join(parts, " | "),
		})
	}
	path, err := exportCSV("traps", []string{
		"time", "source", "version", "kind", "community",
		"trap_oid", "trap_name", "uptime", "varbinds",
	}, rows)
	if err != nil {
		return status("export failed: "+err.Error(), stBad, false)
	}
	v.exported = path
	return status(fmt.Sprintf("exported %d traps → %s", len(v.traps), path), stGood, false)
}

func (v *trapView) view(m *Model) string {
	st := m.st
	w, h := m.cw, m.ch

	var head string
	switch {
	case v.err != "":
		head = st.Bad.Render("✖ " + v.err)
	case v.listening:
		head = st.Good.Render(fmt.Sprintf("● LISTENING  udp/%d", v.port))
	default:
		head = st.Dim.Render(fmt.Sprintf("○ stopped  ·  press %s to listen on udp/%d  (%s to change port)",
			st.Key.Render("l"), v.port, st.Key.Render("P")))
	}
	head += st.Dim.Render(fmt.Sprintf("   ·  %d received", len(v.traps)))
	if v.exported != "" {
		head += st.Good.Render("  ·  exported → " + v.exported)
	}
	if v.port < 1024 {
		head += st.Dim.Render("   (udp<1024 needs root or setcap cap_net_bind_service)")
	}

	vis := v.visible()
	if len(vis) == 0 {
		hint := "No traps yet. Point a device's trap sink at this host."
		if !v.listening {
			hint = "Not listening. Press " + st.Key.Render("l") + " to start."
		}
		return clip(lipgloss.JoinVertical(lipgloss.Left, head, "", centeredHint(m, hint)), 0, h)
	}
	if v.sel >= len(vis) {
		v.sel = len(vis) - 1
	}

	cT, cSrc, cVer, cName := 12, 16, 5, 26
	th := st.Accent.Bold(true).Render(
		padRight("TIME", cT) + padRight("SOURCE", cSrc) + padRight("VER", cVer) +
			padRight("TRAP", cName) + "VARBINDS")

	const detailH = 8
	bodyRows := h - 3 - detailH
	if bodyRows < 3 {
		bodyRows = 3
	}
	if v.sel < v.top {
		v.top = v.sel
	}
	if v.sel >= v.top+bodyRows {
		v.top = v.sel - bodyRows + 1
	}

	var b strings.Builder
	b.WriteString(th + "\n")
	end := minInt(len(vis), v.top+bodyRows)
	for i := v.top; i < end; i++ {
		r := vis[i]
		name := r.trapName
		if name == "" {
			name = r.trapOID
		}
		line := padRight(r.at.Format("15:04:05"), cT) +
			padRight(truncate(r.src, cSrc-1), cSrc) +
			padRight(r.version, cVer) +
			padRight(truncate(name, cName-1), cName) +
			truncate(varbSummary(r), maxInt(10, w-cT-cSrc-cVer-cName))
		if i == v.sel {
			line = st.TableSel.Render(padRight(" "+stripToWidth(line, w-2), w-1))
		} else if r.inform {
			line = st.Warn.Render(line)
		} else {
			line = st.Dim.Render(line)
		}
		b.WriteString(line + "\n")
	}

	// detail panel
	sel := vis[v.sel]
	kind := "trap"
	if sel.inform {
		kind = "inform"
	}
	dl := []string{
		st.PanelTitle.Render("TRAP DETAIL") + st.Dim.Render(fmt.Sprintf("   %s · %s · %s · community %s · uptime %s",
			sel.src, sel.version, kind, dashText(sel.community), dashText(sel.uptime))),
		st.Dim.Render("trap  ") + st.HeaderVal.Render(dashText(sel.trapName)) + st.Dim.Render("  "+sel.trapOID),
	}
	shown := sel.vars
	if len(shown) > detailH-2 {
		shown = shown[:detailH-2]
	}
	for _, vb := range shown {
		nm := vb.name
		if nm == "" {
			nm = vb.oid
		}
		dl = append(dl, "  "+st.Accent.Render(truncate(nm, 30))+st.Dim.Render("  "+vb.oid+"  ")+
			st.HeaderVal.Render(truncate(vb.val, maxInt(10, w-46))))
	}
	if len(sel.vars) == 0 {
		dl = append(dl, st.Dim.Render("  (no varbinds)"))
	}
	detail := st.Panel.Width(w - 2).Render(strings.Join(dl, "\n"))

	var filterLine string
	if v.filtering || v.query != "" {
		filterLine = v.filter.View() + "\n"
	}
	return clip(lipgloss.JoinVertical(lipgloss.Left, head, "", filterLine+b.String(), detail), 0, h)
}

func varbSummary(r trapRecord) string {
	if len(r.vars) == 0 {
		return "—"
	}
	parts := make([]string, 0, len(r.vars))
	for _, vb := range r.vars {
		nm := vb.name
		if nm == "" {
			nm = lastSeg(vb.oid)
		}
		parts = append(parts, nm+"="+vb.val)
	}
	return strings.Join(parts, "  ")
}

func lastSeg(oid string) string {
	if i := strings.LastIndexByte(oid, '.'); i >= 0 && i+1 < len(oid) {
		return oid[i+1:]
	}
	return oid
}

func nextPreset(p int) int {
	for i, x := range trapPortPresets {
		if x == p {
			return trapPortPresets[(i+1)%len(trapPortPresets)]
		}
	}
	return trapPortPresets[0]
}

func friendlyBindErr(err error, port int) string {
	s := err.Error()
	if strings.Contains(s, "permission denied") {
		return fmt.Sprintf("permission denied binding udp/%d — run as root, `setcap cap_net_bind_service=+ep ./snmpdigger`, or press P for a high port", port)
	}
	if strings.Contains(s, "address already in use") {
		return fmt.Sprintf("udp/%d is already in use (another trap receiver?)", port)
	}
	return s
}
