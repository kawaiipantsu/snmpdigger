package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kawaiipantsu/snmpdigger/internal/config"
	"github.com/kawaiipantsu/snmpdigger/internal/snmp"
	"github.com/kawaiipantsu/snmpdigger/internal/tui/charts"
)

type watchItem struct {
	oid  string
	name string
	kind snmp.Kind
	rate bool

	cur      float64
	curStr   string
	prev     float64
	havePrev bool
	tPrev    time.Time
	hist     []float64
	updated  time.Time
	obsMin   float64
	obsMax   float64
}

type watchView struct {
	cfg       *config.Config
	items     []*watchItem
	sel       int
	filter    textinput.Model
	filtering bool
	query     string
	exported  string
}

func newWatchView(st Styles, cfg *config.Config) watchView {
	ti := textinput.New()
	ti.Prompt = st.Accent.Render("/ ")
	ti.Placeholder = "filter watched objects…"
	v := watchView{cfg: cfg, filter: ti}
	for _, e := range cfg.Watch {
		v.items = append(v.items, &watchItem{oid: e.OID, name: e.Name, rate: e.Rate, kind: snmp.KindUnknown})
	}
	return v
}

func (v *watchView) setTheme(st Styles) { v.filter.Prompt = st.Accent.Render("/ ") }

func (v *watchView) help() string {
	if v.filtering {
		return "type to filter · enter/esc done"
	}
	return "↑/↓ select · space/d unwatch · r rate/raw · g graph · e export · x clear all"
}

// add pins an OID to the watch list and persists it.
func (v *watchView) add(oid, name string, kind snmp.Kind) {
	oid = strings.TrimPrefix(strings.TrimSpace(oid), ".")
	for _, it := range v.items {
		if it.oid == oid {
			return
		}
	}
	v.items = append(v.items, &watchItem{
		oid: oid, name: name, kind: kind, rate: kind == snmp.KindCounter,
	})
	v.persist()
}

func (v *watchView) persist() {
	if v.cfg == nil {
		return
	}
	out := make([]config.WatchEntry, 0, len(v.items))
	for _, it := range v.items {
		out = append(out, config.WatchEntry{OID: it.oid, Name: it.name, Rate: it.rate})
	}
	v.cfg.Watch = out
	_ = v.cfg.Save()
}

func (v *watchView) pollOIDs(m *Model) []string {
	if !m.connected || len(v.items) == 0 {
		return nil
	}
	out := make([]string, 0, len(v.items))
	for _, it := range v.items {
		out = append(out, it.oid)
	}
	return out
}

func (v *watchView) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case pollResultMsg:
		if msg.scope == "watch" && msg.err == nil {
			now := time.Now()
			for _, x := range msg.vars {
				v.ingest(x, now)
			}
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
		case "space", "d", "delete", "backspace":
			if it := v.selItem(); it != nil {
				v.removeOID(it.oid)
				return status("unwatched "+it.name, stInfo, false)
			}
		case "x":
			v.items = nil
			v.sel = 0
			v.persist()
			return status("watch list cleared", stInfo, false)
		case "r":
			if it := v.selItem(); it != nil {
				it.rate = !it.rate
				it.havePrev = false
				it.hist = nil
				v.persist()
			}
		case "e":
			return v.export()
		case "g", "enter":
			if it := v.selItem(); it != nil {
				m.graph.setTarget(it.oid, it.name, it.kind)
				m.activeTab = tabGraph
				return status("Graphing "+it.name, stInfo, false)
			}
		}
	}
	return nil
}

func (v *watchView) ingest(x snmp.Var, now time.Time) {
	for _, it := range v.items {
		if it.oid != x.OID {
			continue
		}
		if it.kind == snmp.KindUnknown {
			it.kind = x.Kind
		}
		it.updated = now
		it.curStr = x.Display()
		val := x.Num
		if it.rate && x.Kind == snmp.KindCounter {
			if !it.havePrev {
				it.prev = x.Num
				it.havePrev = true
				it.tPrev = now
				return
			}
			dt := now.Sub(it.tPrev).Seconds()
			if dt <= 0 {
				dt = 1
			}
			d := x.Num - it.prev
			if d < 0 {
				d = x.Num
			}
			val = d / dt
			it.prev = x.Num
			it.tPrev = now
		}
		it.cur = val
		if !it.havePrev || val < it.obsMin {
			it.obsMin = val
		}
		if val > it.obsMax {
			it.obsMax = val
		}
		it.hist = ringPush(it.hist, val, 120)
		return
	}
}

func (v *watchView) visible() []*watchItem {
	q := strings.ToLower(strings.TrimSpace(v.query))
	if q == "" {
		return v.items
	}
	out := make([]*watchItem, 0, len(v.items))
	for _, it := range v.items {
		if strings.Contains(strings.ToLower(it.name+" "+it.oid), q) {
			out = append(out, it)
		}
	}
	return out
}

func (v *watchView) selItem() *watchItem {
	vis := v.visible()
	if v.sel >= 0 && v.sel < len(vis) {
		return vis[v.sel]
	}
	return nil
}

func (v *watchView) removeOID(oid string) {
	out := v.items[:0]
	for _, it := range v.items {
		if it.oid != oid {
			out = append(out, it)
		}
	}
	v.items = out
	if v.sel >= len(v.items) {
		v.sel = maxInt(0, len(v.items)-1)
	}
	v.persist()
}

func (v *watchView) export() tea.Cmd {
	if len(v.items) == 0 {
		return status("watch list is empty", stWarn, false)
	}
	rows := make([][]string, 0, len(v.items))
	for _, it := range v.items {
		mode := "raw"
		if it.rate {
			mode = "rate/s"
		}
		rows = append(rows, []string{
			it.oid, it.name, it.kind.String(), mode,
			fmt.Sprintf("%v", it.cur), it.curStr,
			fmt.Sprintf("%v", it.obsMin), fmt.Sprintf("%v", it.obsMax),
		})
	}
	path, err := exportCSV("watch",
		[]string{"oid", "name", "type", "mode", "value", "display", "min", "max"}, rows)
	if err != nil {
		return status("export failed: "+err.Error(), stBad, false)
	}
	v.exported = path
	return status("exported watch list → "+path, stGood, false)
}

func (v *watchView) view(m *Model) string {
	st := m.st
	if len(v.items) == 0 {
		return centeredHint(m, "Nothing watched yet. Press "+st.Key.Render("space")+
			" on an object in Browser or Interfaces to pin it here.")
	}
	vis := v.visible()
	if len(vis) == 0 {
		return centeredHint(m, "no watched objects match the filter")
	}
	if v.sel >= len(vis) {
		v.sel = len(vis) - 1
	}

	pal := charts.Palette{Line: st.T.Accent, Dim: st.T.Faint, Good: st.T.Good, Warn: st.T.Warn, Bad: st.T.Bad, Text: st.T.Fg}
	w := m.cw
	sparkW := clampInt(w-58, 12, 60)

	var b strings.Builder
	b.WriteString(st.Accent.Bold(true).Render(
		padRight("OBJECT", 26)+padRight("VALUE", 16)+padRight("MODE", 7)+padRight("TREND", sparkW+2)+"AGE") + "\n")

	now := time.Now()
	for i, it := range vis {
		age := "—"
		if !it.updated.IsZero() {
			age = fmt.Sprintf("%ds", int(now.Sub(it.updated).Seconds()))
		}
		val := it.curStr
		if it.rate {
			val = charts.HumanNum(it.cur) + "/s"
		}
		mode := "raw"
		if it.rate {
			mode = "rate"
		}
		nameCell := truncate(it.name, 25)
		line := padRight(nameCell, 26) + padRight(truncate(val, 15), 16) + padRight(mode, 7)
		spark := charts.Sparkline(it.hist, sparkW, pal)
		if i == v.sel {
			b.WriteString(st.TableSel.Render(padRight(" "+line, 49+2)) + " " + spark + "  " + st.Dim.Render(age) + "\n")
			b.WriteString(st.Dim.Render("   "+it.oid+"   min "+charts.HumanNum(it.obsMin)+"  max "+charts.HumanNum(it.obsMax)) + "\n")
		} else {
			b.WriteString(st.Dim.Render(line) + spark + "  " + st.Dim.Render(age) + "\n")
		}
	}

	foot := st.Dim.Render(fmt.Sprintf("%d watched · saved to %s", len(v.items), shortConfigPath()))
	if v.exported != "" {
		foot = st.Good.Render("exported → " + v.exported)
	}

	var filterLine string
	if v.filtering || v.query != "" {
		filterLine = v.filter.View() + "\n"
	}
	return clip(lipgloss.JoinVertical(lipgloss.Left, filterLine+b.String(), "", foot), 0, m.ch)
}

func shortConfigPath() string {
	p, err := config.Path()
	if err != nil {
		return "config"
	}
	return p
}
