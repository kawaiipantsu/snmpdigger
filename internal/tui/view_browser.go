package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kawaiipantsu/snmpdigger/internal/snmp"
)

type nodeRow struct {
	oid  string
	name string
	kind snmp.Kind
	v    snmp.Var
}

var sortKeys = []string{"oid", "name", "value", "type", "age"}
var kindFilters = []snmp.Kind{
	-1, snmp.KindInteger, snmp.KindCounter, snmp.KindGauge,
	snmp.KindTimeTicks, snmp.KindString, snmp.KindOID, snmp.KindIPAddress,
}

type browserView struct {
	tbl    table.Model
	search textinput.Model

	all       []nodeRow
	filtered  []nodeRow
	live      map[string]snmp.Var
	scopeRoot string

	query     string
	searching bool
	filterIdx int
	sortIdx   int
	sortAsc   bool
	walking   bool
	lastWalk  time.Time
	ready     bool
}

func newBrowserView(st Styles) browserView {
	ti := textinput.New()
	ti.Prompt = st.Accent.Render("/")
	ti.Placeholder = "filter by name, OID or value…"
	ti.CharLimit = 80

	t := table.New(
		table.WithFocused(true),
		table.WithHeight(10),
		// columns must exist before the first SetRows or bubbles/table panics
		table.WithColumns([]table.Column{
			{Title: "OID", Width: 24},
			{Title: "NAME", Width: 22},
			{Title: "TYPE", Width: 10},
			{Title: "VALUE", Width: 20},
			{Title: "AGE", Width: 5},
		}),
	)
	ts := table.DefaultStyles()
	ts.Header = ts.Header.Foreground(st.T.Accent).Bold(true).
		BorderStyle(lipgloss.NormalBorder()).BorderForeground(st.T.Border).BorderBottom(true)
	ts.Selected = ts.Selected.Foreground(lipgloss.Color("#0b0b0b")).Background(st.T.Accent).Bold(true)
	ts.Cell = ts.Cell.Foreground(st.T.Fg)
	t.SetStyles(ts)

	return browserView{
		tbl:       t,
		search:    ti,
		live:      map[string]snmp.Var{},
		scopeRoot: snmp.OIDmib2,
		sortAsc:   true,
	}
}

func (v *browserView) help() string {
	return "/ search · f type-filter · s sort · S dir · enter drill · bksp up · r re-walk · g graph · c connect"
}

func (v *browserView) pollOIDs(m *Model) []string {
	if !m.connected || len(v.filtered) == 0 {
		return nil
	}
	limit := m.cfg.Poll.MaxOIDsPerReq * 6
	if limit < 40 {
		limit = 40
	}
	out := make([]string, 0, limit)
	for _, r := range v.filtered {
		if r.kind == snmp.KindNoSuchObject {
			continue
		}
		out = append(out, r.oid)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (v *browserView) onConnect(m *Model) tea.Cmd {
	v.scopeRoot = scopeRootFor(m.cfg.UI.DefaultWalkScope)
	return v.startWalk(m)
}

func scopeRootFor(scope string) string {
	switch scope {
	case "enterprises":
		return snmp.OIDenterprises
	case "whole":
		return snmp.OIDinternet
	default:
		return snmp.OIDmib2
	}
}

func (v *browserView) startWalk(m *Model) tea.Cmd {
	if m.src == nil {
		return nil
	}
	v.walking = true
	return tea.Batch(
		status(fmt.Sprintf("Walking %s …", v.scopeRoot), stInfo, true),
		walkCmd(m.src, v.scopeRoot),
	)
}

func (v *browserView) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case walkResultMsg:
		v.walking = false
		if msg.err != nil {
			return status("Walk failed: "+msg.err.Error(), stBad, false)
		}
		v.all = v.all[:0]
		for _, x := range msg.vars {
			v.all = append(v.all, nodeRow{oid: x.OID, name: m.res.Name(x.OID), kind: x.Kind, v: x})
			v.live[x.OID] = x
		}
		v.lastWalk = time.Now()
		v.ready = true
		v.applyFilter()
		return status(fmt.Sprintf("Loaded %d objects under %s", len(v.all), v.scopeRoot), stGood, false)

	case pollResultMsg:
		if msg.scope == "browser" && msg.err == nil {
			for _, x := range msg.vars {
				v.live[x.OID] = x
			}
			v.refreshRows()
		}
		return nil

	case tea.KeyMsg:
		if v.searching {
			switch msg.String() {
			case "enter", "esc":
				v.searching = false
				v.search.Blur()
				if msg.String() == "esc" {
					v.query = ""
					v.search.SetValue("")
				}
				v.applyFilter()
				return nil
			}
			var cmd tea.Cmd
			v.search, cmd = v.search.Update(msg)
			v.query = v.search.Value()
			v.applyFilter()
			return cmd
		}

		switch msg.String() {
		case "/":
			v.searching = true
			v.search.Focus()
			return textinput.Blink
		case "f":
			v.filterIdx = (v.filterIdx + 1) % len(kindFilters)
			v.applyFilter()
			return nil
		case "s":
			v.sortIdx = (v.sortIdx + 1) % len(sortKeys)
			v.applyFilter()
			return nil
		case "S":
			v.sortAsc = !v.sortAsc
			v.applyFilter()
			return nil
		case "r":
			return v.startWalk(m)
		case "enter":
			if row := v.tbl.SelectedRow(); row != nil {
				v.scopeRoot = strings.TrimSpace(row[0])
				return v.startWalk(m)
			}
		case "backspace":
			parts := strings.Split(v.scopeRoot, ".")
			if len(parts) > 4 {
				v.scopeRoot = strings.Join(parts[:len(parts)-1], ".")
				return v.startWalk(m)
			}
		case "g":
			if row := v.tbl.SelectedRow(); row != nil {
				oid := strings.TrimSpace(row[0])
				for _, r := range v.filtered {
					if r.oid == oid {
						m.graph.setTarget(r.oid, r.name, r.kind)
						m.activeTab = tabGraph
						return status("Graphing "+r.name, stInfo, false)
					}
				}
			}
		}
	}

	var cmd tea.Cmd
	v.tbl, cmd = v.tbl.Update(msg)
	return cmd
}

func (v *browserView) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(v.query))
	kind := kindFilters[v.filterIdx]
	out := make([]nodeRow, 0, len(v.all))
	for _, r := range v.all {
		if kind != -1 && r.kind != kind {
			continue
		}
		if q != "" {
			hay := strings.ToLower(r.oid + " " + r.name + " " + r.v.Display())
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, r)
	}

	key := sortKeys[v.sortIdx]
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		less := false
		switch key {
		case "name":
			less = strings.ToLower(a.name) < strings.ToLower(b.name)
		case "type":
			less = a.kind.String() < b.kind.String()
		case "value":
			av, bv := v.currentVar(a.oid), v.currentVar(b.oid)
			if av.Kind.Numeric() && bv.Kind.Numeric() {
				less = av.Num < bv.Num
			} else {
				less = av.Display() < bv.Display()
			}
		case "age":
			less = v.currentVar(a.oid).Time.Before(v.currentVar(b.oid).Time)
		default:
			less = snmp.OIDLess(a.oid, b.oid)
		}
		if v.sortAsc {
			return less
		}
		return !less
	})

	v.filtered = out
	v.refreshRows()
}

func (v *browserView) currentVar(oid string) snmp.Var {
	if lv, ok := v.live[oid]; ok {
		return lv
	}
	for _, r := range v.all {
		if r.oid == oid {
			return r.v
		}
	}
	return snmp.Var{}
}

func (v *browserView) refreshRows() {
	rows := make([]table.Row, 0, len(v.filtered))
	now := time.Now()
	for _, r := range v.filtered {
		cur := v.currentVar(r.oid)
		age := "—"
		if !cur.Time.IsZero() {
			age = fmt.Sprintf("%ds", int(now.Sub(cur.Time).Seconds()))
		}
		rows = append(rows, table.Row{r.oid, r.name, cur.Kind.String(), cur.Display(), age})
	}
	v.tbl.SetRows(rows)
}

func (v *browserView) layout(m *Model) {
	w := m.cw
	oidW := clampInt(w*26/100, 16, 30)
	nameW := clampInt(w*26/100, 14, 34)
	typeW := 10
	ageW := 5
	valW := w - oidW - nameW - typeW - ageW - 6
	if valW < 10 {
		valW = 10
	}
	v.tbl.SetColumns([]table.Column{
		{Title: "OID", Width: oidW},
		{Title: "NAME", Width: nameW},
		{Title: "TYPE", Width: typeW},
		{Title: "VALUE", Width: valW},
		{Title: "AGE", Width: ageW},
	})
	v.tbl.SetWidth(w)
	v.tbl.SetHeight(clampInt(m.ch-3, 3, 400))
}

func (v *browserView) view(m *Model) string {
	st := m.st
	v.layout(m)
	if !m.connected {
		return centeredHint(m, "Not connected. Press "+st.Key.Render("c")+" to open the connection dialog.")
	}
	if v.walking && !v.ready {
		return centeredHint(m, m.spin.View()+" walking "+st.Accent.Render(v.scopeRoot)+" …")
	}

	filterName := "all"
	if kindFilters[v.filterIdx] != -1 {
		filterName = kindFilters[v.filterIdx].String()
	}
	dir := "▲"
	if !v.sortAsc {
		dir = "▼"
	}
	bar := lipgloss.JoinHorizontal(lipgloss.Top,
		st.Dim.Render("scope "), st.Accent.Render(v.scopeRoot),
		st.Dim.Render("   shown "), st.HeaderVal.Render(fmt.Sprintf("%d/%d", len(v.filtered), len(v.all))),
		st.Dim.Render("   type "), st.HeaderVal.Render(filterName),
		st.Dim.Render("   sort "), st.HeaderVal.Render(sortKeys[v.sortIdx]+dir),
	)

	var searchLine string
	if v.searching || v.query != "" {
		searchLine = v.search.View()
	} else {
		searchLine = st.Dim.Render("press / to search")
	}

	return lipgloss.JoinVertical(lipgloss.Left, bar, searchLine, v.tbl.View())
}

func (v *browserView) setTheme(st Styles) {
	ts := table.DefaultStyles()
	ts.Header = ts.Header.Foreground(st.T.Accent).Bold(true).
		BorderStyle(lipgloss.NormalBorder()).BorderForeground(st.T.Border).BorderBottom(true)
	ts.Selected = ts.Selected.Foreground(lipgloss.Color("#0b0b0b")).Background(st.T.Accent).Bold(true)
	ts.Cell = ts.Cell.Foreground(st.T.Fg)
	v.tbl.SetStyles(ts)
	v.search.Prompt = st.Accent.Render("/")
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
