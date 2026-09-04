package tui

import (
	"math"
	"os"
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
	if os.Getenv("SNMPDIGGER_SHOT_BROWSER") == "" &&
		os.Getenv("SNMPDIGGER_SHOT_GRAPH") == "" &&
		os.Getenv("SNMPDIGGER_SHOT_SYSTEM") == "" {
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

	m.statusText = "walk complete — 214 objects under 1.3.6.1.2.1 · polling every 3s"
	m.statusLvl = stGood
	m.now = time.Date(2026, 9, 4, 13, 37, 12, 0, time.Local)

	shots := []struct {
		env string
		tab tabID
	}{
		{"SNMPDIGGER_SHOT_BROWSER", tabBrowser},
		{"SNMPDIGGER_SHOT_GRAPH", tabGraph},
		{"SNMPDIGGER_SHOT_SYSTEM", tabSystem},
	}
	for _, s := range shots {
		path := os.Getenv(s.env)
		if path == "" {
			continue
		}
		m.activeTab = s.tab
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
