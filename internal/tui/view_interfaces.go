package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kawaiipantsu/snmpdigger/internal/snmp"
	"github.com/kawaiipantsu/snmpdigger/internal/tui/charts"
)

// IF-MIB / ifXTable column OIDs (no instance suffix).
const (
	colIfDescr   = "1.3.6.1.2.1.2.2.1.2"
	colIfType    = "1.3.6.1.2.1.2.2.1.3"
	colIfMtu     = "1.3.6.1.2.1.2.2.1.4"
	colIfSpeed   = "1.3.6.1.2.1.2.2.1.5"
	colIfMac     = "1.3.6.1.2.1.2.2.1.6"
	colIfAdmin   = "1.3.6.1.2.1.2.2.1.7"
	colIfOper    = "1.3.6.1.2.1.2.2.1.8"
	colIfInOct   = "1.3.6.1.2.1.2.2.1.10"
	colIfInDisc  = "1.3.6.1.2.1.2.2.1.13"
	colIfInErr   = "1.3.6.1.2.1.2.2.1.14"
	colIfOutOct  = "1.3.6.1.2.1.2.2.1.16"
	colIfOutDisc = "1.3.6.1.2.1.2.2.1.19"
	colIfOutErr  = "1.3.6.1.2.1.2.2.1.20"
	colIfName    = "1.3.6.1.2.1.31.1.1.1.1"
	colIfHCIn    = "1.3.6.1.2.1.31.1.1.1.6"
	colIfHCOut   = "1.3.6.1.2.1.31.1.1.1.10"
	colIfHiSpeed = "1.3.6.1.2.1.31.1.1.1.15"
	colIfAlias   = "1.3.6.1.2.1.31.1.1.1.18"

	rootIfTable  = "1.3.6.1.2.1.2"
	rootIfXTable = "1.3.6.1.2.1.31.1.1.1"
)

type ifRow struct {
	idx           int
	name, alias   string
	admin, oper   int
	speedbps      float64
	mtu           int
	mac           string
	hc            bool
	inOct, outOct float64
	inErr, outErr float64
	inDsc, outDsc float64

	inbps, outbps   float64
	inHist, outHist []float64
	prevIn, prevOut float64
	tPrev           time.Time
	havePrev        bool
}

func (r *ifRow) inUtil() float64  { return utilPct(r.inbps, r.speedbps) }
func (r *ifRow) outUtil() float64 { return utilPct(r.outbps, r.speedbps) }

func utilPct(bps, speed float64) float64 {
	if speed <= 0 {
		return 0
	}
	return bps / speed * 100
}

type interfacesView struct {
	rows      map[int]*ifRow
	order     []int
	top       int
	sel       int
	filter    textinput.Model
	filtering bool
	query     string
	upOnly    bool
	sortKey   int
	walking   int // pending walk count
	loaded    bool
	exported  string
}

var ifSortKeys = []string{"index", "name", "in-util", "out-util", "errors"}

func newInterfacesView(st Styles) interfacesView {
	ti := textinput.New()
	ti.Prompt = st.Accent.Render("/ ")
	ti.Placeholder = "filter by name / alias…"
	return interfacesView{rows: map[int]*ifRow{}, filter: ti}
}

func (v *interfacesView) help() string {
	if v.filtering {
		return "type to filter · enter/esc done"
	}
	return "↑/↓ select · / filter · f up-only · s sort · g graph · space → watch · e export · r re-walk"
}

func (v *interfacesView) setTheme(st Styles) { v.filter.Prompt = st.Accent.Render("/ ") }

func (v *interfacesView) onConnect(m *Model) tea.Cmd {
	if m.src == nil {
		return nil
	}
	v.rows = map[int]*ifRow{}
	v.order = nil
	v.loaded = false
	v.walking = 2
	return tea.Batch(
		status("Loading interface table…", stInfo, true),
		walkCmd(m.src, rootIfTable),
		walkCmd(m.src, rootIfXTable),
	)
}

func (v *interfacesView) row(idx int) *ifRow {
	r := v.rows[idx]
	if r == nil {
		r = &ifRow{idx: idx}
		v.rows[idx] = r
		v.order = append(v.order, idx)
		sort.Ints(v.order)
	}
	return r
}

func (v *interfacesView) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case walkResultMsg:
		if !strings.HasPrefix(msg.root, rootIfTable) && !strings.HasPrefix(msg.root, "1.3.6.1.2.1.31") {
			return nil
		}
		if v.walking > 0 {
			v.walking--
		}
		if msg.err == nil {
			for _, x := range msg.vars {
				v.ingestWalk(x)
			}
		}
		if v.walking == 0 {
			v.loaded = true
			return status(fmt.Sprintf("Interface table: %d ports", len(v.order)), stGood, false)
		}
		return nil

	case pollResultMsg:
		if msg.scope == "interfaces" && msg.err == nil {
			now := time.Now()
			for _, x := range msg.vars {
				v.ingestPoll(x)
			}
			v.computeRates(now)
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
		case "up", "k":
			if v.sel > 0 {
				v.sel--
			}
		case "down", "j":
			if v.sel < len(vis)-1 {
				v.sel++
			}
		case "/":
			v.filtering = true
			v.filter.Focus()
			return textinput.Blink
		case "f":
			v.upOnly = !v.upOnly
			v.sel = 0
		case "s":
			v.sortKey = (v.sortKey + 1) % len(ifSortKeys)
		case "r":
			return v.onConnect(m)
		case "e":
			return v.export()
		case "g":
			if r := v.selRow(); r != nil {
				oid := oidWithIdx(colIfInOct, r.idx)
				if r.hc {
					oid = oidWithIdx(colIfHCIn, r.idx)
				}
				m.graph.setTarget(oid, "ifIn "+r.label(), snmp.KindCounter)
				m.activeTab = tabGraph
				return status("Graphing ifIn "+r.label(), stInfo, false)
			}
		case "space":
			if r := v.selRow(); r != nil {
				inC, outC := colIfInOct, colIfOutOct
				if r.hc {
					inC, outC = colIfHCIn, colIfHCOut
				}
				m.watch.add(oidWithIdx(inC, r.idx), "ifIn "+r.label(), snmp.KindCounter)
				m.watch.add(oidWithIdx(outC, r.idx), "ifOut "+r.label(), snmp.KindCounter)
				return status("Watching in/out octets for "+r.label(), stGood, false)
			}
		}
	}
	return nil
}

func (v *interfacesView) ingestWalk(x snmp.Var) {
	col, idx, ok := splitLeaf(x.OID)
	if !ok || idx <= 0 {
		return
	}
	if !strings.HasPrefix(col, "1.3.6.1.2.1.2.2.1.") && !strings.HasPrefix(col, "1.3.6.1.2.1.31.1.1.1.") {
		return
	}
	r := v.row(idx)
	switch col {
	case colIfName:
		r.name = x.Str
	case colIfDescr:
		if r.name == "" {
			r.name = x.Str
		}
	case colIfAlias:
		r.alias = x.Str
	case colIfMac:
		r.mac = x.Str
	case colIfMtu:
		r.mtu = int(x.Num)
	case colIfAdmin:
		r.admin = int(x.Num)
	case colIfOper:
		r.oper = int(x.Num)
	case colIfType:
		// keep numeric type only; not displayed directly
	case colIfSpeed:
		if r.speedbps == 0 {
			r.speedbps = x.Num
		}
	case colIfHiSpeed:
		if x.Num > 0 {
			r.speedbps = x.Num * 1_000_000
		}
	case colIfInOct:
		r.inOct = x.Num
	case colIfOutOct:
		r.outOct = x.Num
	case colIfHCIn:
		r.inOct = x.Num
		r.hc = true
	case colIfHCOut:
		r.outOct = x.Num
		r.hc = true
	case colIfInErr:
		r.inErr = x.Num
	case colIfOutErr:
		r.outErr = x.Num
	case colIfInDisc:
		r.inDsc = x.Num
	case colIfOutDisc:
		r.outDsc = x.Num
	}
}

func (v *interfacesView) ingestPoll(x snmp.Var) {
	col, idx, ok := splitLeaf(x.OID)
	if !ok {
		return
	}
	r := v.rows[idx]
	if r == nil {
		return
	}
	switch col {
	case colIfInOct, colIfHCIn:
		r.inOct = x.Num
	case colIfOutOct, colIfHCOut:
		r.outOct = x.Num
	case colIfOper:
		r.oper = int(x.Num)
	case colIfInErr:
		r.inErr = x.Num
	case colIfOutErr:
		r.outErr = x.Num
	case colIfInDisc:
		r.inDsc = x.Num
	case colIfOutDisc:
		r.outDsc = x.Num
	}
}

func (v *interfacesView) computeRates(now time.Time) {
	for _, idx := range v.order {
		r := v.rows[idx]
		if r == nil {
			continue
		}
		if !r.havePrev {
			r.havePrev = true
			r.tPrev = now
			r.prevIn, r.prevOut = r.inOct, r.outOct
			continue
		}
		dt := now.Sub(r.tPrev).Seconds()
		if dt <= 0 {
			dt = 1
		}
		r.tPrev = now
		r.pushRate(dt)
	}
}

// pushRate derives bit/s from the delta between the last two cumulative counter
// samples held on the row.
func (r *ifRow) pushRate(dt float64) {
	din := r.inOct - r.prevIn
	dout := r.outOct - r.prevOut
	if din < 0 {
		din = r.inOct
	}
	if dout < 0 {
		dout = r.outOct
	}
	r.inbps = din * 8 / dt
	r.outbps = dout * 8 / dt
	r.prevIn = r.inOct
	r.prevOut = r.outOct
	r.inHist = ringPush(r.inHist, r.inbps, 60)
	r.outHist = ringPush(r.outHist, r.outbps, 60)
}

func ringPush(s []float64, v float64, n int) []float64 {
	s = append(s, v)
	if len(s) > n {
		s = s[len(s)-n:]
	}
	return s
}

func (v *interfacesView) pollOIDs(m *Model) []string {
	if !m.connected || !v.loaded {
		return nil
	}
	out := make([]string, 0, len(v.order)*5)
	for _, idx := range v.order {
		r := v.rows[idx]
		inC, outC := colIfInOct, colIfOutOct
		if r.hc {
			inC, outC = colIfHCIn, colIfHCOut
		}
		out = append(out,
			oidWithIdx(inC, idx), oidWithIdx(outC, idx),
			oidWithIdx(colIfOper, idx),
			oidWithIdx(colIfInErr, idx), oidWithIdx(colIfOutErr, idx))
	}
	return out
}

func (v *interfacesView) visible() []*ifRow {
	q := strings.ToLower(strings.TrimSpace(v.query))
	out := make([]*ifRow, 0, len(v.order))
	for _, idx := range v.order {
		r := v.rows[idx]
		if r == nil {
			continue
		}
		if v.upOnly && r.oper != 1 {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(r.name+" "+r.alias), q) {
			continue
		}
		out = append(out, r)
	}
	switch ifSortKeys[v.sortKey] {
	case "name":
		sort.SliceStable(out, func(i, j int) bool { return out[i].name < out[j].name })
	case "in-util":
		sort.SliceStable(out, func(i, j int) bool { return out[i].inUtil() > out[j].inUtil() })
	case "out-util":
		sort.SliceStable(out, func(i, j int) bool { return out[i].outUtil() > out[j].outUtil() })
	case "errors":
		sort.SliceStable(out, func(i, j int) bool {
			return (out[i].inErr + out[i].outErr) > (out[j].inErr + out[j].outErr)
		})
	}
	return out
}

func (v *interfacesView) selRow() *ifRow {
	vis := v.visible()
	if v.sel >= 0 && v.sel < len(vis) {
		return vis[v.sel]
	}
	return nil
}

func (v *interfacesView) export() tea.Cmd {
	if len(v.order) == 0 {
		return status("no interface data yet", stWarn, false)
	}
	rows := make([][]string, 0, len(v.order))
	for _, idx := range v.order {
		r := v.rows[idx]
		rows = append(rows, []string{
			strconv.Itoa(r.idx), r.name, r.alias,
			statusWord(r.admin), statusWord(r.oper),
			fmt.Sprintf("%.0f", r.speedbps),
			fmt.Sprintf("%.0f", r.inbps), fmt.Sprintf("%.0f", r.outbps),
			fmt.Sprintf("%.1f", r.inUtil()), fmt.Sprintf("%.1f", r.outUtil()),
			fmt.Sprintf("%.0f", r.inErr), fmt.Sprintf("%.0f", r.outErr),
			fmt.Sprintf("%.0f", r.inDsc), fmt.Sprintf("%.0f", r.outDsc),
			r.mac, strconv.Itoa(r.mtu),
		})
	}
	path, err := exportCSV("interfaces", []string{
		"ifIndex", "name", "alias", "admin", "oper", "speed_bps",
		"in_bps", "out_bps", "in_util_pct", "out_util_pct",
		"in_errors", "out_errors", "in_discards", "out_discards", "mac", "mtu",
	}, rows)
	if err != nil {
		return status("export failed: "+err.Error(), stBad, false)
	}
	v.exported = path
	return status("exported interface table → "+path, stGood, false)
}

func (v *interfacesView) view(m *Model) string {
	st := m.st
	if !m.connected {
		return centeredHint(m, "Not connected. Press "+st.Key.Render("c")+" to open the connection dialog.")
	}
	if !v.loaded && v.walking > 0 {
		return centeredHint(m, m.spin.View()+" loading interface table…")
	}
	vis := v.visible()
	if len(vis) == 0 {
		return centeredHint(m, "no interfaces match the current filter")
	}
	if v.sel >= len(vis) {
		v.sel = len(vis) - 1
	}

	w := m.cw
	// column widths
	cIdx, cName, cAlias, cSt, cSpd, cRate, cUtil, cErr := 4, 16, 18, 9, 9, 12, 8, 11
	if w < 116 {
		cName, cAlias = 13, 12
	}
	head := st.Accent.Bold(true).Render(
		padRight("IDX", cIdx) + padRight("INTERFACE", cName) + padRight("ALIAS", cAlias) +
			padRight("ADM/OPR", cSt) + padRight("SPEED", cSpd) +
			padRight("IN bps", cRate) + padRight("OUT bps", cRate) +
			padRight("IN%", cUtil) + padRight("OUT%", cUtil) +
			padRight("ERR i/o", cErr) + "DISC i/o")

	bodyRows := m.ch - 3
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
	b.WriteString(head + "\n")
	end := minInt(len(vis), v.top+bodyRows)
	upCount := 0
	for _, r := range vis {
		if r.oper == 1 {
			upCount++
		}
	}
	for i := v.top; i < end; i++ {
		r := vis[i]
		line := padRight(strconv.Itoa(r.idx), cIdx) +
			padRight(truncate(r.name, cName-1), cName) +
			padRight(truncate(dashText(r.alias), cAlias-1), cAlias) +
			padRight(operPair(r.admin, r.oper), cSt) +
			padRight(shortSpeed(r.speedbps), cSpd) +
			padRight(charts.HumanNum(r.inbps), cRate) +
			padRight(charts.HumanNum(r.outbps), cRate) +
			padRight(utilStr(r.inUtil()), cUtil) +
			padRight(utilStr(r.outUtil()), cUtil) +
			padRight(fmt.Sprintf("%s/%s", charts.HumanNum(r.inErr), charts.HumanNum(r.outErr)), cErr) +
			fmt.Sprintf("%s/%s", charts.HumanNum(r.inDsc), charts.HumanNum(r.outDsc))

		switch {
		case i == v.sel:
			line = st.TableSel.Render(padRight(" "+stripToWidth(line, w-2), w-1))
		case r.oper != 1 && r.admin == 1:
			line = st.Bad.Render(line) // admin up but oper down
		case maxf(r.inUtil(), r.outUtil()) >= 80:
			line = st.Warn.Render(line)
		default:
			line = st.Dim.Render(line)
		}
		b.WriteString(line + "\n")
	}

	// selected-interface detail strip
	foot := st.Dim.Render(fmt.Sprintf("%d interfaces · %d up · sort %s", len(vis), upCount, ifSortKeys[v.sortKey]))
	if r := v.selRow(); r != nil {
		pal := charts.Palette{Line: st.T.Accent, Dim: st.T.Faint, Good: st.T.Good, Warn: st.T.Warn, Bad: st.T.Bad, Text: st.T.Fg}
		sw := minInt(w-4, 80)
		foot = st.Accent.Render(r.label()) + st.Dim.Render(fmt.Sprintf("  mac %s  mtu %d  ", dashText(r.mac), r.mtu)) + "\n" +
			st.Dim.Render("in  ") + charts.Sparkline(r.inHist, sw, pal) + st.Dim.Render("  "+charts.HumanNum(r.inbps)+" b/s") + "\n" +
			st.Dim.Render("out ") + charts.Sparkline(r.outHist, sw, pal) + st.Dim.Render("  "+charts.HumanNum(r.outbps)+" b/s")
	}

	var filterLine string
	if v.filtering || v.query != "" {
		filterLine = v.filter.View()
	}
	out := b.String()
	if filterLine != "" {
		out = filterLine + "\n" + out
	}
	return clip(lipgloss.JoinVertical(lipgloss.Left, out, foot), 0, m.ch)
}

// --- helpers ---

func (r *ifRow) label() string {
	if r.name != "" {
		return r.name
	}
	return "if" + strconv.Itoa(r.idx)
}

func splitLeaf(oid string) (col string, idx int, ok bool) {
	oid = strings.TrimPrefix(strings.TrimSpace(oid), ".")
	i := strings.LastIndexByte(oid, '.')
	if i < 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(oid[i+1:])
	if err != nil {
		return "", 0, false
	}
	return oid[:i], n, true
}

func oidWithIdx(col string, idx int) string { return col + "." + strconv.Itoa(idx) }

func statusWord(v int) string {
	switch v {
	case 1:
		return "up"
	case 2:
		return "down"
	case 3:
		return "testing"
	default:
		return "?"
	}
}

func operPair(admin, oper int) string {
	sym := func(v int) string {
		switch v {
		case 1:
			return "▲"
		case 2:
			return "▼"
		default:
			return "•"
		}
	}
	return sym(admin) + "/" + sym(oper)
}

func shortSpeed(bps float64) string {
	switch {
	case bps <= 0:
		return "—"
	case bps >= 1e9:
		return fmt.Sprintf("%.0fG", bps/1e9)
	case bps >= 1e6:
		return fmt.Sprintf("%.0fM", bps/1e6)
	case bps >= 1e3:
		return fmt.Sprintf("%.0fk", bps/1e3)
	default:
		return fmt.Sprintf("%.0f", bps)
	}
}

func utilStr(p float64) string {
	if p <= 0 {
		return "—"
	}
	if p > 999 {
		return ">999"
	}
	return fmt.Sprintf("%.1f", p)
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func stripToWidth(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	return truncate(s, w)
}
