package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kawaiipantsu/snmpdigger/internal/snmp"
	"github.com/kawaiipantsu/snmpdigger/internal/tui/charts"
)

var graphKindNames = []string{"line", "bars", "sparkline", "gauge", "bignum", "heatmap"}

const (
	gLine = iota
	gBars
	gSpark
	gGauge
	gBig
	gHeat
)

type gsample struct {
	t time.Time
	v float64
}

type graphView struct {
	oid  string
	name string
	kind snmp.Kind

	gkind    int
	rate     bool
	samples  []gsample
	rawPrev  float64
	havePrev bool
	obsMin   float64
	obsMax   float64
	history  int

	picking bool
	pin     textinput.Model
	pitems  []nodeRow
	pfilt   []nodeRow
	pcur    int
}

func newGraphView(st Styles, history int) graphView {
	ti := textinput.New()
	ti.Prompt = st.Accent.Render("oid› ")
	ti.Placeholder = "type to filter numeric objects…"
	if history < 10 {
		history = 120
	}
	return graphView{gkind: gLine, history: history, pin: ti, obsMin: math.Inf(1), obsMax: math.Inf(-1)}
}

func (v *graphView) help() string {
	if v.picking {
		return "type to filter · ↑/↓ move · enter pick · esc cancel"
	}
	return "o pick OID · t chart type · d rate/raw (counters) · x clear · c connect"
}

func (v *graphView) pollOIDs(m *Model) []string {
	if !m.connected || v.oid == "" {
		return nil
	}
	return []string{v.oid}
}

func (v *graphView) setTarget(oid, name string, kind snmp.Kind) {
	v.oid, v.name, v.kind = oid, name, kind
	v.samples = v.samples[:0]
	v.havePrev = false
	v.obsMin, v.obsMax = math.Inf(1), math.Inf(-1)
	v.rate = kind == snmp.KindCounter
}

func (v *graphView) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case pollResultMsg:
		if msg.scope == "graph" && msg.err == nil {
			for _, x := range msg.vars {
				if x.OID == v.oid {
					v.ingest(x)
				}
			}
		}
		return nil

	case walkResultMsg:
		// keep a fresh copy of numeric objects for the picker
		if msg.err == nil {
			v.pitems = v.pitems[:0]
			for _, x := range msg.vars {
				if x.Kind.Numeric() {
					v.pitems = append(v.pitems, nodeRow{oid: x.OID, name: m.res.Name(x.OID), kind: x.Kind, v: x})
				}
			}
		}
		return nil

	case tea.KeyMsg:
		if v.picking {
			return v.pickerKey(m, msg)
		}
		switch msg.String() {
		case "o":
			v.openPicker(m)
			return textinput.Blink
		case "t":
			v.gkind = (v.gkind + 1) % len(graphKindNames)
		case "d":
			if v.kind == snmp.KindCounter {
				v.rate = !v.rate
				v.samples = v.samples[:0]
				v.havePrev = false
			}
		case "x":
			v.samples = v.samples[:0]
			v.havePrev = false
			v.obsMin, v.obsMax = math.Inf(1), math.Inf(-1)
		}
	}
	return nil
}

func (v *graphView) ingest(x snmp.Var) {
	val := x.Num
	if v.rate && v.kind == snmp.KindCounter {
		if !v.havePrev {
			v.rawPrev = x.Num
			v.havePrev = true
			return
		}
		dt := 1.0
		if n := len(v.samples); n > 0 {
			dt = x.Time.Sub(v.samples[n-1].t).Seconds()
		}
		if dt <= 0 {
			dt = 1
		}
		delta := x.Num - v.rawPrev
		if delta < 0 { // counter wrap
			delta = x.Num
		}
		val = delta / dt
		v.rawPrev = x.Num
	}
	v.samples = append(v.samples, gsample{t: x.Time, v: val})
	if len(v.samples) > v.history {
		v.samples = v.samples[len(v.samples)-v.history:]
	}
	if val < v.obsMin {
		v.obsMin = val
	}
	if val > v.obsMax {
		v.obsMax = val
	}
}

func (v *graphView) values() []float64 {
	out := make([]float64, len(v.samples))
	for i, s := range v.samples {
		out[i] = s.v
	}
	return out
}

// --- picker ---

func (v *graphView) openPicker(m *Model) {
	v.picking = true
	v.pin.SetValue("")
	v.pin.Focus()
	if len(v.pitems) == 0 {
		for _, r := range m.browser.all {
			if r.kind.Numeric() {
				v.pitems = append(v.pitems, r)
			}
		}
	}
	v.pfilt = v.pitems
	v.pcur = 0
}

func (v *graphView) pickerKey(m *Model, msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		v.picking = false
		v.pin.Blur()
		return nil
	case "enter":
		if v.pcur >= 0 && v.pcur < len(v.pfilt) {
			r := v.pfilt[v.pcur]
			v.setTarget(r.oid, r.name, r.kind)
		}
		v.picking = false
		v.pin.Blur()
		return status("Graphing "+v.name, stInfo, false)
	case "up", "ctrl+p":
		if v.pcur > 0 {
			v.pcur--
		}
		return nil
	case "down", "ctrl+n":
		if v.pcur < len(v.pfilt)-1 {
			v.pcur++
		}
		return nil
	}
	var cmd tea.Cmd
	v.pin, cmd = v.pin.Update(msg)
	q := strings.ToLower(strings.TrimSpace(v.pin.Value()))
	v.pfilt = v.pfilt[:0]
	for _, r := range v.pitems {
		if q == "" || strings.Contains(strings.ToLower(r.oid+" "+r.name), q) {
			v.pfilt = append(v.pfilt, r)
		}
	}
	if v.pcur >= len(v.pfilt) {
		v.pcur = maxInt(0, len(v.pfilt)-1)
	}
	return cmd
}

func (v *graphView) pickerView(m *Model) string {
	st := m.st
	rows := []string{st.ModalTitle.Render("  PICK AN OBJECT TO GRAPH  "), "", v.pin.View(), ""}
	shown := v.pfilt
	const win = 12
	start := 0
	if v.pcur >= win {
		start = v.pcur - win + 1
	}
	end := minInt(len(shown), start+win)
	for i := start; i < end; i++ {
		r := shown[i]
		plain := fmt.Sprintf("%-26s  %-24s  %s", truncate(r.name, 26), truncate(r.oid, 24), r.kind.String())
		if i == v.pcur {
			rows = append(rows, st.TableSel.Render(" "+padRight(plain, 56)+" "))
		} else {
			rows = append(rows, " "+st.Dim.Render(plain))
		}
	}
	if len(shown) == 0 {
		rows = append(rows, st.Dim.Render("  (walk the Browser tab first to populate objects)"))
	}
	card := st.Modal.Render(strings.Join(rows, "\n"))
	return lipgloss.Place(m.cw, m.ch, lipgloss.Center, lipgloss.Center, card)
}

func (v *graphView) view(m *Model) string {
	st := m.st
	if !m.connected {
		return centeredHint(m, "Not connected. Press "+st.Key.Render("c")+" to open the connection dialog.")
	}
	if v.picking {
		return v.pickerView(m)
	}
	if v.oid == "" {
		return centeredHint(m, "No object selected. Press "+st.Key.Render("o")+" to pick one, or "+st.Key.Render("g")+" from the Browser tab.")
	}

	pal := charts.Palette{
		Line: st.T.Accent, Dim: st.T.Faint, Good: st.T.Good,
		Warn: st.T.Warn, Bad: st.T.Bad, Text: st.T.Fg,
	}
	vals := v.values()

	unit := ""
	if v.rate {
		unit = "/s"
	}
	cur := math.NaN()
	if len(vals) > 0 {
		cur = vals[len(vals)-1]
	}
	mode := "raw"
	if v.rate {
		mode = "rate"
	}
	head := lipgloss.JoinHorizontal(lipgloss.Top,
		st.Accent.Render(v.name), st.Dim.Render("  "+v.oid),
		st.Dim.Render("   type "), st.HeaderVal.Render(graphKindNames[v.gkind]),
		st.Dim.Render("   mode "), st.HeaderVal.Render(mode),
		st.Dim.Render("   samples "), st.HeaderVal.Render(fmt.Sprintf("%d", len(vals))),
	)

	areaH := m.ch - 4
	if areaH < 4 {
		areaH = 4
	}
	areaW := m.cw - 2

	var body string
	switch v.gkind {
	case gBig:
		big := "—"
		if !math.IsNaN(cur) {
			big = charts.HumanNum(cur)
		}
		spark := charts.Sparkline(vals, minInt(areaW, 60), pal)
		body = lipgloss.JoinVertical(lipgloss.Left,
			"", charts.BigNumber(big+unit, pal), "",
			st.Dim.Render("trend  ")+spark,
			st.Dim.Render(fmt.Sprintf("min %s   max %s", numOr(v.obsMin), numOr(v.obsMax))),
		)
	case gGauge:
		lo, hi := v.gaugeRange()
		g := charts.Gauge(orZero(cur), lo, hi, minInt(areaW, 70), pal)
		body = lipgloss.JoinVertical(lipgloss.Left, "",
			g, "",
			st.Dim.Render(fmt.Sprintf("scale %s … %s", charts.HumanNum(lo), charts.HumanNum(hi))),
			"", st.Dim.Render("history  ")+charts.Sparkline(vals, minInt(areaW, 70), pal))
	case gSpark:
		body = lipgloss.JoinVertical(lipgloss.Left, "",
			charts.Sparkline(vals, areaW, pal), "",
			st.Dim.Render(fmt.Sprintf("last %s%s   min %s   max %s",
				numOr(cur), unit, numOr(v.obsMin), numOr(v.obsMax))))
	case gBars:
		body = charts.Bars(vals, minInt(areaW, 120), areaH, pal)
	case gHeat:
		body = charts.Heatmap(vals, minInt(areaW, 160), areaH, pal)
	default: // line
		axis := charts.Axis(vals, areaH, pal)
		aw := lipgloss.Width(axis)
		line := charts.Line(vals, areaW-aw-1, areaH, pal)
		body = lipgloss.JoinHorizontal(lipgloss.Top, axis, " ", line)
	}

	if len(vals) < 2 {
		body = lipgloss.JoinVertical(lipgloss.Left, body, "",
			st.Dim.Render(m.spin.View()+" collecting samples every "+
				fmt.Sprintf("%ds", m.cfg.Poll.IntervalSeconds)+" …"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, head, "", body)
}

func (v *graphView) gaugeRange() (float64, float64) {
	name := strings.ToLower(v.name)
	if strings.Contains(name, "idle") || strings.Contains(name, "load") ||
		strings.Contains(name, "cpu") || strings.Contains(name, "percent") {
		return 0, 100
	}
	lo := 0.0
	hi := v.obsMax
	if math.IsInf(hi, 0) || hi <= 0 {
		hi = 1
	}
	return lo, hi * 1.15
}

func (v *graphView) setTheme(st Styles) { v.pin.Prompt = st.Accent.Render("oid› ") }

func numOr(f float64) string {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return "—"
	}
	return charts.HumanNum(f)
}
func orZero(f float64) float64 {
	if math.IsNaN(f) {
		return 0
	}
	return f
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
