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

	"github.com/kawaiipantsu/snmpdigger/internal/mib"
	"github.com/kawaiipantsu/snmpdigger/internal/snmp"
)

type catMode int

const (
	catBrowse catMode = iota
	catSearch
	catFetch
)

type catFetchDoneMsg struct {
	module string
	path   string
	err    error
}

type catalogView struct {
	mode catMode

	vendors []string // "All" + mib.Vendors()
	vfilt   int

	mods []mib.CatModule
	sel  int // index into mods
	top  int // left-pane scroll offset (module rows)

	tbl     table.Model
	objRow  int
	results []mib.CatObject // active search results

	search textinput.Model

	remote   []mib.RemoteMIB
	rfilt    []mib.RemoteMIB
	rcur     int
	cached   map[string]bool
	fetching string
	cacheDir string
}

func newCatalogView(st Styles) catalogView {
	ti := textinput.New()
	ti.Prompt = st.Accent.Render("/ ")
	ti.Placeholder = "search every catalogued object…"
	ti.CharLimit = 80

	t := table.New(table.WithFocused(true), table.WithHeight(10))
	applyTableTheme(&t, st)

	v := catalogView{
		vendors: append([]string{"All"}, mib.Vendors()...),
		search:  ti,
		remote:  mib.RemoteIndex(),
		cached:  map[string]bool{},
	}
	if d, err := mib.MIBCacheDir(); err == nil {
		v.cacheDir = d
	}
	v.rfilt = v.remote
	v.refreshVendor()
	v.refreshCache()
	return v
}

func (v *catalogView) help() string {
	switch v.mode {
	case catSearch:
		return "type to search · ↑/↓ move · enter → browse live · esc back"
	case catFetch:
		return "type to filter · ↑/↓ move · enter download MIB · esc back"
	default:
		return "↑/↓ module · v vendor filter · / search · F fetch MIBs · enter → browse live · g → graph"
	}
}

func (v *catalogView) capturing() bool {
	return (v.mode == catSearch || v.mode == catFetch) && v.search.Focused()
}

func (v *catalogView) pollOIDs(*Model) []string { return nil }

func (v *catalogView) setTheme(st Styles) {
	applyTableTheme(&v.tbl, st)
	v.search.Prompt = st.Accent.Render("/ ")
}

func (v *catalogView) refreshVendor() {
	name := v.vendors[v.vfilt]
	v.mods = mib.ModulesByVendor(name)
	if v.sel >= len(v.mods) {
		v.sel = 0
	}
	v.top = 0
	v.fillModuleTable()
}

func (v *catalogView) refreshCache() {
	for k := range v.cached {
		delete(v.cached, k)
	}
	for _, name := range mib.CachedMIBs() {
		v.cached[strings.ToUpper(name)] = true
	}
}

func (v *catalogView) fillModuleTable() {
	v.objRow = 0
	v.tbl.SetColumns([]table.Column{
		{Title: "OID", Width: 28},
		{Title: "NAME", Width: 26},
		{Title: "TYPE", Width: 16},
		{Title: "ACCESS", Width: 12},
	})
	if v.sel < 0 || v.sel >= len(v.mods) {
		v.tbl.SetRows(nil)
		return
	}
	rows := make([]table.Row, 0, len(v.mods[v.sel].Objects))
	for _, o := range v.mods[v.sel].Objects {
		rows = append(rows, table.Row{o.OID, o.Name, o.Type, o.Access})
	}
	v.tbl.SetRows(rows)
	v.tbl.SetCursor(0)
}

func (v *catalogView) fillSearchTable() {
	v.tbl.SetColumns([]table.Column{
		{Title: "OID", Width: 30},
		{Title: "NAME", Width: 26},
		{Title: "TYPE", Width: 14},
		{Title: "DESCRIPTION", Width: 60},
	})
	rows := make([]table.Row, 0, len(v.results))
	for _, o := range v.results {
		rows = append(rows, table.Row{o.OID, o.Name, o.Type, o.Descr})
	}
	v.tbl.SetRows(rows)
	v.tbl.SetCursor(0)
}

func (v *catalogView) selectedObject() (mib.CatObject, bool) {
	row := v.tbl.SelectedRow()
	if row == nil {
		return mib.CatObject{}, false
	}
	oid := row[0]
	if v.mode == catSearch {
		for _, o := range v.results {
			if o.OID == oid {
				return o, true
			}
		}
		return mib.CatObject{}, false
	}
	if v.sel >= 0 && v.sel < len(v.mods) {
		for _, o := range v.mods[v.sel].Objects {
			if o.OID == oid {
				return o, true
			}
		}
	}
	return mib.CatObject{}, false
}

func (v *catalogView) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case catFetchDoneMsg:
		v.fetching = ""
		v.refreshCache()
		if msg.err != nil {
			return status("MIB download failed: "+msg.err.Error(), stBad, false)
		}
		return status("Downloaded "+msg.module+" → "+msg.path, stGood, false)

	case tea.KeyMsg:
		switch v.mode {
		case catSearch:
			return v.keySearch(m, msg)
		case catFetch:
			return v.keyFetch(m, msg)
		default:
			return v.keyBrowse(m, msg)
		}
	}
	return nil
}

func (v *catalogView) keyBrowse(m *Model, msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "/":
		v.mode = catSearch
		v.search.SetValue("")
		v.search.Focus()
		v.results = nil
		v.fillSearchTable()
		return textinput.Blink
	case "F":
		v.mode = catFetch
		v.search.SetValue("")
		v.search.Focus()
		v.rfilt = v.remote
		v.rcur = 0
		v.refreshCache()
		return textinput.Blink
	case "v":
		v.vfilt = (v.vfilt + 1) % len(v.vendors)
		v.refreshVendor()
		return status("Catalog vendor: "+v.vendors[v.vfilt], stInfo, false)
	case "V":
		v.vfilt = (v.vfilt - 1 + len(v.vendors)) % len(v.vendors)
		v.refreshVendor()
		return nil
	case "up", "k":
		if v.sel > 0 {
			v.sel--
			v.fillModuleTable()
		}
		return nil
	case "down", "j":
		if v.sel < len(v.mods)-1 {
			v.sel++
			v.fillModuleTable()
		}
		return nil
	case "pgup":
		v.sel = clampInt(v.sel-8, 0, maxInt(0, len(v.mods)-1))
		v.fillModuleTable()
		return nil
	case "pgdown":
		v.sel = clampInt(v.sel+8, 0, maxInt(0, len(v.mods)-1))
		v.fillModuleTable()
		return nil
	case "enter":
		return v.activateObject(m)
	case "g":
		if o, ok := v.selectedObject(); ok && m.connected {
			m.graph.setTarget(o.OID, o.Name, snmp.KindUnknown)
			m.activeTab = tabGraph
			return status("Graphing "+o.Name, stInfo, false)
		}
		return status("connect first to graph "+dashObj(v), stWarn, false)
	}
	var cmd tea.Cmd
	v.tbl, cmd = v.tbl.Update(msg)
	return cmd
}

func (v *catalogView) activateObject(m *Model) tea.Cmd {
	o, ok := v.selectedObject()
	if !ok {
		return nil
	}
	if !m.connected {
		v.mode = catSearch
		v.search.SetValue(o.OID)
		v.results = mib.SearchCatalog(o.OID)
		v.fillSearchTable()
		return status("not connected — object OID "+o.OID, stWarn, false)
	}
	m.browser.scopeRoot = o.OID
	m.activeTab = tabBrowser
	return tea.Batch(
		status("Walking "+o.Name+" ("+o.OID+") on "+m.connTarget, stInfo, true),
		m.browser.startWalk(m),
	)
}

func (v *catalogView) keySearch(m *Model, msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		v.mode = catBrowse
		v.search.Blur()
		v.fillModuleTable()
		return nil
	case "enter":
		return v.activateObject(m)
	case "up", "ctrl+p":
		v.tbl.MoveUp(1)
		return nil
	case "down", "ctrl+n":
		v.tbl.MoveDown(1)
		return nil
	}
	var cmd tea.Cmd
	v.search, cmd = v.search.Update(msg)
	v.results = mib.SearchCatalog(v.search.Value())
	v.fillSearchTable()
	return cmd
}

func (v *catalogView) keyFetch(m *Model, msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		v.mode = catBrowse
		v.search.Blur()
		v.fillModuleTable()
		return nil
	case "up", "ctrl+p":
		if v.rcur > 0 {
			v.rcur--
		}
		return nil
	case "down", "ctrl+n":
		if v.rcur < len(v.rfilt)-1 {
			v.rcur++
		}
		return nil
	case "enter":
		if v.rcur >= 0 && v.rcur < len(v.rfilt) {
			r := v.rfilt[v.rcur]
			v.fetching = r.Module
			return tea.Batch(
				status("Downloading "+r.Module+" …", stInfo, true),
				downloadMIBCmd(r),
			)
		}
		return nil
	}
	var cmd tea.Cmd
	v.search, cmd = v.search.Update(msg)
	v.rfilt = mib.SearchRemote(v.search.Value())
	if v.search.Value() == "" {
		v.rfilt = v.remote
	}
	if v.rcur >= len(v.rfilt) {
		v.rcur = maxInt(0, len(v.rfilt)-1)
	}
	return cmd
}

func downloadMIBCmd(r mib.RemoteMIB) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		path, err := mib.DownloadMIB(ctx, r)
		return catFetchDoneMsg{module: r.Module, path: path, err: err}
	}
}

// ---- rendering ----

func (v *catalogView) view(m *Model) string {
	st := m.st
	switch v.mode {
	case catSearch:
		return v.viewSearch(m)
	case catFetch:
		return v.viewFetch(m)
	}

	leftW := clampInt(m.cw/3, 24, 40)
	rightW := m.cw - leftW - 3

	left := v.renderModuleList(st, leftW, m.ch-2)

	var right string
	if v.sel >= 0 && v.sel < len(v.mods) {
		mod := v.mods[v.sel]
		v.tbl.SetWidth(rightW)
		v.tbl.SetHeight(clampInt(m.ch-9, 3, 400))
		head := lipgloss.JoinVertical(lipgloss.Left,
			st.PanelTitle.Render(mod.Module)+st.Dim.Render("   "+mod.Vendor),
			st.Dim.Render("root ")+st.HeaderVal.Render(mod.Root)+st.Dim.Render("   objects ")+st.HeaderVal.Render(fmt.Sprintf("%d", len(mod.Objects))),
			st.Dim.Render(truncate(mod.Summary, rightW)),
		)
		desc := ""
		if o, ok := v.selectedObject(); ok {
			desc = st.Accent.Render(o.Name) + st.Dim.Render("  "+o.OID+"  ["+o.Type+", "+o.Access+"]") + "\n" +
				lipgloss.NewStyle().Width(rightW).Render(o.Descr)
		}
		right = lipgloss.JoinVertical(lipgloss.Left,
			head, "", v.tbl.View(), "",
			st.Panel.Width(rightW).Render(desc))
	} else {
		right = centeredHint(m, "no modules for this vendor filter")
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(leftW).Render(left),
		st.Dim.Render(" │ "),
		lipgloss.NewStyle().Width(rightW).Render(right),
	)
	return body
}

func (v *catalogView) renderModuleList(st Styles, w, h int) string {
	title := st.PanelTitle.Render("MIB CATALOG") + st.Dim.Render("  vendor: ") + st.HeaderVal.Render(v.vendors[v.vfilt])
	if h < 4 {
		h = 4
	}
	rowsAvail := h - 2

	// keep selection visible
	if v.sel < v.top {
		v.top = v.sel
	}
	if v.sel >= v.top+rowsAvail {
		v.top = v.sel - rowsAvail + 1
	}

	var b strings.Builder
	b.WriteString(title + "\n\n")
	lastVendor := ""
	printed := 0
	for i := v.top; i < len(v.mods) && printed < rowsAvail; i++ {
		mod := v.mods[i]
		if mod.Vendor != lastVendor {
			lastVendor = mod.Vendor
			b.WriteString(st.Dim.Render("▸ "+strings.ToUpper(mod.Vendor)) + "\n")
			printed++
			if printed >= rowsAvail {
				break
			}
		}
		line := "  " + truncate(mod.Module, w-4)
		if i == v.sel {
			line = st.TableSel.Render(padRight(" "+truncate(mod.Module, w-4), w-1))
		} else {
			line = st.Dim.Render(line)
		}
		b.WriteString(line + "\n")
		printed++
	}
	b.WriteString("\n" + st.Help.Render(fmt.Sprintf("%d/%d modules", v.sel+1, len(v.mods))))
	return b.String()
}

func (v *catalogView) viewSearch(m *Model) string {
	st := m.st
	v.tbl.SetWidth(m.cw - 2)
	v.tbl.SetHeight(clampInt(m.ch-6, 3, 400))
	head := st.PanelTitle.Render("CATALOG SEARCH") + "   " + v.search.View()
	sub := st.Dim.Render(fmt.Sprintf("%d match(es) across %d modules", len(v.results), len(mib.Catalog())))
	var desc string
	if o, ok := v.selectedObject(); ok {
		desc = st.Panel.Width(m.cw - 2).Render(
			st.Accent.Render(o.Name) + st.Dim.Render("  "+o.OID) + "\n" +
				lipgloss.NewStyle().Width(m.cw-6).Render(o.Descr))
	}
	return lipgloss.JoinVertical(lipgloss.Left, head, sub, "", v.tbl.View(), "", desc)
}

func (v *catalogView) viewFetch(m *Model) string {
	st := m.st
	head := st.PanelTitle.Render("FETCH / DOWNLOAD MIBs") + "   " + v.search.View()
	win := clampInt(m.ch-7, 4, 400)
	start := 0
	if v.rcur >= win {
		start = v.rcur - win + 1
	}
	end := minInt(len(v.rfilt), start+win)

	var b strings.Builder
	for i := start; i < end; i++ {
		r := v.rfilt[i]
		mark := "  "
		if v.cached[strings.ToUpper(r.Module)] {
			mark = st.Good.Render("✓ ")
		}
		if r.Module == v.fetching {
			mark = st.Accent.Render(m.spin.View() + " ")
		}
		line := fmt.Sprintf("%-30s %-22s %s",
			truncate(r.Module, 30), truncate(r.Vendor, 22), truncate(r.URL, maxInt(10, m.cw-58)))
		if i == v.rcur {
			b.WriteString(st.TableSel.Render(padRight(" "+line, m.cw-4)) + "\n")
		} else {
			b.WriteString(mark + st.Dim.Render(line) + "\n")
		}
	}
	note := st.Help.Render("raw MIB saved to " + v.cacheDir + "  (parsing not yet implemented)")
	count := st.Dim.Render(fmt.Sprintf("%d/%d remote modules · %d cached", v.rcur+1, len(v.rfilt), len(v.cached)))
	return lipgloss.JoinVertical(lipgloss.Left, head, count, "", b.String(), note)
}

func dashObj(v *catalogView) string {
	if o, ok := v.selectedObject(); ok {
		return o.Name
	}
	return "object"
}
