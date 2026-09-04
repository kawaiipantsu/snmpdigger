package tui

import (
	"math"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/kawaiipantsu/snmpdigger/internal/config"
	"github.com/kawaiipantsu/snmpdigger/internal/snmp"
)

// TestCaptureFrames renders fully populated frames of individual tabs and writes
// them (raw ANSI) to the files named by the SNMPDIGGER_SHOT_* env vars. It is a
// no-op during a normal `go test` run - it only does work when those vars are
// set, which is how the README screenshots are produced.
//
//	SNMPDIGGER_SHOT_W, SNMPDIGGER_SHOT_H  - terminal size (default 128x34)
//	SNMPDIGGER_SHOT_BROWSER               - path for the Browser tab frame
//	SNMPDIGGER_SHOT_GRAPH                 - path for the Graph tab frame
//	SNMPDIGGER_SHOT_SYSTEM                - path for the System tab frame
func TestCaptureFrames(t *testing.T) {
	anyShot := false
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "SNMPDIGGER_SHOT_") && !strings.HasSuffix(e, "=") {
			anyShot = true
			break
		}
	}
	if !anyShot {
		t.Skip("set SNMPDIGGER_SHOT_* to capture frames")
	}

	// Force full colour: in a non-TTY test process lipgloss/termenv would
	// otherwise detect no colour support and strip every SGR sequence.
	lipgloss.SetColorProfile(termenv.TrueColor)

	w := envInt("SNMPDIGGER_SHOT_W", 128)
	h := envInt("SNMPDIGGER_SHOT_H", 34)

	m := newModel(config.Default(), Options{Demo: true})
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: w, Height: h})

	src := snmp.NewDemo()
	if err := src.Connect(); err != nil {
		t.Fatalf("demo connect: %v", err)
	}
	m, _ = updateModel(m, connectResultMsg{src: src, describe: src.Describe(), target: src.Target()})

	info, err := snmp.Discover(src)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	m, _ = updateModel(m, discoverResultMsg{info: info})

	vars, err := src.Walk(snmp.OIDmib2)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	m, _ = updateModel(m, walkResultMsg{root: snmp.OIDmib2, vars: vars})

	// a couple of live poll rounds so the Browser shows fresh values + ages
	for i := 0; i < 3; i++ {
		pv, _ := src.Get(m.browser.pollOIDs(m))
		m, _ = updateModel(m, pollResultMsg{scope: "browser", vars: pv, at: time.Now()})
	}
	sysv, _ := src.Get([]string{
		snmp.OIDsysUpTime, snmp.OIDsysContact, snmp.OIDsysName,
		snmp.OIDsysLocation, snmp.OIDsysDescr,
	})
	m, _ = updateModel(m, pollResultMsg{scope: "system", vars: sysv, at: time.Now()})

	// feed the Graph a real time series (CPU temperature, a gauge that waves)
	const tempOID = "1.3.6.1.4.1.2021.13.16.2.1.3.1"
	m.graph.setTarget(tempOID, "lmTempSensorsValue.1", snmp.KindGauge)
	m.graph.gkind = gLine
	base := time.Now().Add(-90 * time.Second)
	for i := 0; i < 90; i++ {
		val := 48000 + 9000*math.Sin(float64(i)/9) + 1600*math.Sin(float64(i)/2.3)
		m, _ = updateModel(m, pollResultMsg{
			scope: "graph",
			vars:  []snmp.Var{{OID: tempOID, Kind: snmp.KindGauge, Num: val, Time: base.Add(time.Duration(i) * time.Second)}},
			at:    base.Add(time.Duration(i) * time.Second),
		})
	}

	// Summary: build the network profile from the demo agent
	m.summary.targetHost = "demo"
	m, _ = updateModel(m, profileMsg{profile: snmp.BuildProfile(src, info)})

	// Interfaces: load the if-table and run a few poll rounds so rates appear
	_ = m.ifaces.onConnect(m)
	ift, _ := src.Walk("1.3.6.1.2.1.2")
	m, _ = updateModel(m, walkResultMsg{root: "1.3.6.1.2.1.2", vars: ift})
	ifx, _ := src.Walk("1.3.6.1.2.1.31.1.1.1")
	m, _ = updateModel(m, walkResultMsg{root: "1.3.6.1.2.1.31.1.1.1", vars: ifx})
	ifBase := time.Now().Add(-6 * time.Second)
	for i := 0; i < 3; i++ {
		pv, _ := src.Get(m.ifaces.pollOIDs(m))
		m, _ = updateModel(m, pollResultMsg{scope: "interfaces", vars: pv, at: ifBase.Add(time.Duration(i*3) * time.Second)})
	}

	// Watch: pin a few objects and poll them
	m.watch.add("1.3.6.1.2.1.2.2.1.10.2", "ifIn eth0", snmp.KindCounter)
	m.watch.add("1.3.6.1.2.1.2.2.1.16.2", "ifOut eth0", snmp.KindCounter)
	m.watch.add("1.3.6.1.4.1.2021.11.11.0", "ssCpuIdle", snmp.KindInteger)
	for i := 0; i < 4; i++ {
		pv, _ := src.Get(m.watch.pollOIDs(m))
		m, _ = updateModel(m, pollResultMsg{scope: "watch", vars: pv, at: time.Now().Add(time.Duration(i*3) * time.Second)})
	}

	// Traps: fake a listening session with a few received traps
	m.traps.listening = true
	tBase := time.Date(2026, 9, 4, 13, 36, 40, 0, time.Local)
	m.traps.traps = []trapRecord{
		{at: tBase, src: "10.0.1.9", version: "v2c", community: "public",
			trapOID: "1.3.6.1.6.3.1.1.5.4", trapName: "linkUp", uptime: "4h12m0s",
			vars: []trapVarb{{oid: "1.3.6.1.2.1.2.2.1.1.3", name: "ifIndex.3", typ: "Integer", val: "3"},
				{oid: "1.3.6.1.2.1.2.2.1.7.3", name: "ifAdminStatus.3", typ: "Integer", val: "1"}}},
		{at: tBase.Add(9 * time.Second), src: "10.0.1.9", version: "v2c", community: "public",
			trapOID: "1.3.6.1.6.3.1.1.5.3", trapName: "linkDown", uptime: "4h12m9s",
			vars: []trapVarb{{oid: "1.3.6.1.2.1.2.2.1.1.7", name: "ifIndex.7", typ: "Integer", val: "7"}}},
		{at: tBase.Add(23 * time.Second), src: "192.168.7.1", version: "v1", community: "trapcomm",
			trapOID: "1.3.6.1.4.1.9.0.1", trapName: "enterpriseSpecific", uptime: "31m2s",
			vars: []trapVarb{{oid: "1.3.6.1.4.1.9.9.13.1.3.1.3.1", name: "ciscoEnvMonTemperatureState", typ: "Integer", val: "3"}}},
		{at: tBase.Add(31 * time.Second), src: "10.0.2.50", version: "v2c", community: "public", inform: true,
			trapOID: "1.3.6.1.6.3.1.1.5.1", trapName: "coldStart", uptime: "0h00m4s"},
	}
	m.traps.sel = len(m.traps.traps) - 1

	// scroll the Catalog down so a mid-list module is selected (exercises the
	// left-pane scroll offset that used to overflow)
	m.activeTab = tabCatalog
	for i := 0; i < 17; i++ {
		m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")}) // → object pane
	for i := 0; i < 3; i++ {
		m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyDown}) // move object cursor
	}

	m.statusLvl = stGood
	m.now = time.Date(2026, 9, 4, 13, 37, 12, 0, time.Local)

	statusFor := map[tabID]string{
		tabSystem:     "identified forge.lab.thugs.red · Linux host / server · polling every 3s",
		tabInterfaces: "interface table: 4 ports · polling every 3s",
		tabBrowser:    "walk complete — 214 objects under 1.3.6.1.2.1 · polling every 3s",
		tabGraph:      "graphing lmTempSensorsValue.1 · 90 samples · line chart",
		tabWatch:      "3 objects watched · saved to ~/.config/snmpdigger/config.yaml",
		tabSummary:    "network profile ready · demo agent",
		tabTraps:      "listening for SNMP traps on udp/162 · 4 received",
		tabDiscovery:  "ready — pick CIDR / ASN / LOCAL and press Start scan",
		tabCatalog:    "catalog: 31 modules · 340 objects · offline reference",
	}

	shots := []struct {
		env string
		tab tabID
	}{
		{"SNMPDIGGER_SHOT_BROWSER", tabBrowser},
		{"SNMPDIGGER_SHOT_GRAPH", tabGraph},
		{"SNMPDIGGER_SHOT_SYSTEM", tabSystem},
		{"SNMPDIGGER_SHOT_CATALOG", tabCatalog},
		{"SNMPDIGGER_SHOT_DISCOVERY", tabDiscovery},
		{"SNMPDIGGER_SHOT_INTERFACES", tabInterfaces},
		{"SNMPDIGGER_SHOT_WATCH", tabWatch},
		{"SNMPDIGGER_SHOT_SUMMARY", tabSummary},
		{"SNMPDIGGER_SHOT_TRAPS", tabTraps},
	}
	for _, s := range shots {
		path := os.Getenv(s.env)
		if path == "" {
			continue
		}
		m.activeTab = s.tab
		if txt, ok := statusFor[s.tab]; ok {
			m.statusText = txt
		}
		frame := m.View()
		if err := os.WriteFile(path, []byte(frame+"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("wrote %s (%d bytes)", path, len(frame))
	}
}

func updateModel(m *Model, msg tea.Msg) (*Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(*Model), cmd
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		n := 0
		for _, c := range v {
			if c < '0' || c > '9' {
				return def
			}
			n = n*10 + int(c-'0')
		}
		if n > 0 {
			return n
		}
	}
	return def
}
