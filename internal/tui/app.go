// Package tui implements the snmpdigger terminal UI: a square logo, an
// extended-width connection header, a tab bar (System / Browser / Graph /
// Discovery / Catalog / Settings), a live content area and a one-line status
// footer.
package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kawaiipantsu/snmpdigger/internal/config"
	"github.com/kawaiipantsu/snmpdigger/internal/mib"
	"github.com/kawaiipantsu/snmpdigger/internal/snmp"
)

type tabID int

const (
	tabSystem tabID = iota
	tabBrowser
	tabGraph
	tabDiscovery
	tabCatalog
	tabSettings
)

var tabNames = []string{"System", "Browser", "Graph", "Discovery", "Catalog", "Settings"}

// Options configures a TUI run.
type Options struct {
	Demo        bool
	AutoConnect *config.Connection
}

// Run starts the Bubble Tea program and blocks until the user quits.
func Run(opts Options) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	m := newModel(cfg, opts)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// Model is the root Bubble Tea model.
type Model struct {
	cfg *config.Config
	res *mib.Resolver
	st  Styles

	w, h   int
	cw, ch int

	src        snmp.Source
	connected  bool
	connDesc   string
	connTarget string
	sys        snmp.SystemInfo

	activeTab tabID

	showConnect bool
	connect     connectModel

	system    systemView
	browser   browserView
	graph     graphView
	discovery discoveryView
	catalog   catalogView
	settings  settingsView

	spin       spinner.Model
	now        time.Time
	statusText string
	statusLvl  statusLevel
	statusBusy bool

	autoConn    *config.Connection
	pendingConn config.Connection
	demo        bool
}

func newModel(cfg *config.Config, opts Options) *Model {
	st := NewStyles(ThemeByName(cfg.UI.Theme))
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(st.T.Accent)

	m := &Model{
		cfg:        cfg,
		res:        mib.New(true),
		st:         st,
		activeTab:  tabSystem,
		connect:    newConnectModel(st, cfg.Last),
		browser:    newBrowserView(st),
		graph:      newGraphView(st, cfg.UI.GraphHistory),
		discovery:  newDiscoveryView(st, cfg.Last),
		catalog:    newCatalogView(st),
		settings:   settingsView{},
		spin:       sp,
		now:        time.Now(),
		statusText: "ready",
		autoConn:   opts.AutoConnect,
		demo:       opts.Demo,
	}
	if p, err := config.Path(); err == nil {
		m.settings.path = p
	}
	if opts.Demo || opts.AutoConnect != nil {
		m.showConnect = false
	} else {
		m.showConnect = true
	}
	return m
}

// Init satisfies tea.Model.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spin.Tick, clockTick(), tickEvery(m.pollInterval())}
	switch {
	case m.demo:
		cmds = append(cmds, status("Starting demo agent…", stInfo, true),
			connectCmd(m.cfg, config.Connection{}, true))
	case m.autoConn != nil:
		m.pendingConn = *m.autoConn
		cmds = append(cmds, status("Connecting to "+m.autoConn.Host+"…", stInfo, true),
			connectCmd(m.cfg, *m.autoConn, false))
	default:
		cmds = append(cmds, status("Enter SNMP connection details", stInfo, false))
	}
	return tea.Batch(cmds...)
}

func (m *Model) pollInterval() time.Duration {
	s := m.cfg.Poll.IntervalSeconds
	if s < 1 {
		s = 1
	}
	return time.Duration(s) * time.Second
}

// reschedulePoll is a hook for Settings; the tick loop already re-reads the
// interval each fire, so nothing extra is needed.
func (m *Model) reschedulePoll() tea.Cmd { return nil }

func (m *Model) applyTheme() {
	m.st = NewStyles(ThemeByName(m.cfg.UI.Theme))
	m.spin.Style = lipgloss.NewStyle().Foreground(m.st.T.Accent)
	m.browser.setTheme(m.st)
	m.graph.setTheme(m.st)
	m.discovery.setTheme(m.st)
	m.catalog.setTheme(m.st)
	if m.showConnect {
		m.connect = newConnectModel(m.st, m.cfg.Last)
	}
}

func scopeName(t tabID) string {
	switch t {
	case tabSystem:
		return "system"
	case tabBrowser:
		return "browser"
	case tabGraph:
		return "graph"
	default:
		return ""
	}
}

// Update satisfies tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case clockMsg:
		m.now = time.Time(msg)
		return m, clockTick()

	case tickMsg:
		cmds := []tea.Cmd{tickEvery(m.pollInterval())}
		if c := m.pollActive(); c != nil {
			cmds = append(cmds, c)
		}
		return m, tea.Batch(cmds...)

	case statusMsg:
		m.statusText, m.statusLvl, m.statusBusy = msg.text, msg.level, msg.busy
		return m, nil

	case connectResultMsg:
		return m, m.onConnectResult(msg)

	case discoverResultMsg:
		if msg.err != nil {
			return m, status("Identify failed: "+msg.err.Error(), stBad, false)
		}
		m.sys = msg.info
		return m, status("Identified: "+dash(m.sys.Role), stGood, false)

	case walkResultMsg:
		cmds := []tea.Cmd{
			m.browser.update(m, msg),
			m.graph.update(m, msg),
		}
		return m, tea.Batch(cmds...)

	case pollResultMsg:
		if msg.err != nil {
			m.statusText = "poll error: " + msg.err.Error()
			m.statusLvl = stWarn
		}
		cmds := []tea.Cmd{
			m.system.update(m, msg),
			m.browser.update(m, msg),
			m.graph.update(m, msg),
		}
		return m, tea.Batch(cmds...)

	case scanUpdateMsg:
		return m, m.discovery.update(m, msg)

	case tea.KeyMsg:
		return m, m.onKey(msg)
	}

	// forward everything else (cursor blink, etc.) to the focused surface
	if m.showConnect {
		cmd, _ := m.connect.update(msg)
		return m, cmd
	}
	return m, m.updateActive(msg)
}

func (m *Model) onKey(msg tea.KeyMsg) tea.Cmd {
	if msg.String() == "ctrl+c" {
		return tea.Quit
	}

	if m.showConnect {
		cmd, intent := m.connect.update(msg)
		if intent == nil {
			return cmd
		}
		if intent.cancel {
			m.showConnect = false
			if !m.connected {
				return status("No active connection — press c to connect", stWarn, false)
			}
			return status("connection dialog closed", stInfo, false)
		}
		m.showConnect = false
		if intent.demo {
			m.pendingConn = config.Connection{}
			return tea.Batch(status("Starting demo agent…", stInfo, true),
				connectCmd(m.cfg, config.Connection{}, true))
		}
		m.pendingConn = intent.conn
		return tea.Batch(status("Connecting to "+intent.conn.Host+"…", stInfo, true),
			connectCmd(m.cfg, intent.conn, false))
	}

	if !m.capturing() {
		switch msg.String() {
		case "q":
			return tea.Quit
		case "c":
			m.connect = newConnectModel(m.st, m.cfg.Last)
			m.showConnect = true
			return status("Enter SNMP connection details", stInfo, false)
		case "]", "tab", "shift+right":
			m.activeTab = (m.activeTab + 1) % tabID(len(tabNames))
			return m.onTabSwitch()
		case "[", "shift+tab", "shift+left":
			m.activeTab = (m.activeTab - 1 + tabID(len(tabNames))) % tabID(len(tabNames))
			return m.onTabSwitch()
		case "1", "2", "3", "4", "5", "6":
			m.activeTab = tabID(msg.String()[0] - '1')
			return m.onTabSwitch()
		}
	}

	return m.updateActive(msg)
}

func (m *Model) onTabSwitch() tea.Cmd {
	cmds := []tea.Cmd{status(tabNames[m.activeTab]+" view", stInfo, false)}
	if c := m.pollActive(); c != nil {
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}

func (m *Model) capturing() bool {
	if m.showConnect {
		return true
	}
	switch m.activeTab {
	case tabBrowser:
		return m.browser.searching
	case tabGraph:
		return m.graph.picking
	case tabDiscovery:
		return m.discovery.scanning || m.discovery.focus <= 1
	case tabCatalog:
		if c, ok := any(&m.catalog).(interface{ capturing() bool }); ok {
			return c.capturing()
		}
	}
	return false
}

func (m *Model) updateActive(msg tea.Msg) tea.Cmd {
	switch m.activeTab {
	case tabSystem:
		return m.system.update(m, msg)
	case tabBrowser:
		return m.browser.update(m, msg)
	case tabGraph:
		return m.graph.update(m, msg)
	case tabDiscovery:
		return m.discovery.update(m, msg)
	case tabCatalog:
		return m.catalog.update(m, msg)
	case tabSettings:
		return m.settings.update(m, msg)
	}
	return nil
}

func (m *Model) pollActive() tea.Cmd {
	if !m.connected || m.src == nil {
		return nil
	}
	var oids []string
	switch m.activeTab {
	case tabSystem:
		oids = m.system.pollOIDs(m)
	case tabBrowser:
		oids = m.browser.pollOIDs(m)
	case tabGraph:
		oids = m.graph.pollOIDs(m)
	}
	if len(oids) == 0 {
		return nil
	}
	return pollCmd(m.src, scopeName(m.activeTab), oids)
}

func (m *Model) onConnectResult(msg connectResultMsg) tea.Cmd {
	if msg.err != nil {
		m.connected = false
		m.showConnect = true
		m.connect = newConnectModel(m.st, m.cfg.Last)
		return status("Connection failed: "+msg.err.Error(), stBad, false)
	}
	m.src = msg.src
	m.connected = true
	m.connDesc = msg.describe
	m.connTarget = msg.target
	m.showConnect = false
	m.sys = snmp.SystemInfo{}

	if !m.demo && m.pendingConn.Host != "" {
		m.cfg.RememberConnection(m.pendingConn)
		_ = m.cfg.Save()
	}

	return tea.Batch(
		status("Connected — identifying device and walking the tree…", stGood, true),
		discoverSystemCmd(m.src),
		m.browser.onConnect(m),
		m.pollActive(),
	)
}

// View satisfies tea.Model.
func (m *Model) View() string {
	if m.w == 0 || m.h == 0 {
		return "starting snmpdigger…"
	}
	m.cw = m.w

	header := m.renderHeader(m.w)
	tabs := m.renderTabs(m.w)
	help := m.renderHelpLine(m.w)
	footer := m.renderFooter(m.w)

	chrome := lipgloss.Height(header) + lipgloss.Height(tabs) + 2 // help + footer
	m.ch = m.h - chrome
	if m.ch < 3 {
		m.ch = 3
	}

	var body string
	if m.showConnect {
		body = m.connect.view(m.cw, m.ch)
	} else {
		body = m.activeView()
	}
	body = fitBlock(body, m.cw, m.ch)

	return lipgloss.JoinVertical(lipgloss.Left, header, tabs, body, help, footer)
}

func (m *Model) activeView() string {
	switch m.activeTab {
	case tabSystem:
		return m.system.view(m)
	case tabBrowser:
		return m.browser.view(m)
	case tabGraph:
		return m.graph.view(m)
	case tabDiscovery:
		return m.discovery.view(m)
	case tabCatalog:
		return m.catalog.view(m)
	case tabSettings:
		return m.settings.view(m)
	}
	return ""
}

// fitBlock forces s to exactly w columns wide (per line) and h rows tall.
func fitBlock(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, ln := range lines {
		if lipgloss.Width(ln) > w {
			lines[i] = truncate(ln, w)
		}
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unknown"
	}
	return s
}
