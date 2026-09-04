package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kawaiipantsu/snmpdigger/internal/config"
	"github.com/kawaiipantsu/snmpdigger/internal/snmp"
)

// feed runs one Update round and returns the model, ignoring the command.
func feed(t *testing.T, m *Model, msg tea.Msg) *Model {
	t.Helper()
	next, _ := m.Update(msg)
	mm, ok := next.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", next)
	}
	return mm
}

// TestRenderAllTabs drives the model without a TTY: size it, connect it to the
// synthetic demo agent, push a walk + a poll, then render every tab. Any panic
// in a view or in the chrome fails the test.
func TestRenderAllTabs(t *testing.T) {
	m := newModel(config.Default(), Options{Demo: true})
	m = feed(t, m, tea.WindowSizeMsg{Width: 140, Height: 44})

	src := snmp.NewDemo()
	if err := src.Connect(); err != nil {
		t.Fatalf("demo connect: %v", err)
	}
	m = feed(t, m, connectResultMsg{src: src, describe: src.Describe(), target: src.Target()})
	if !m.connected {
		t.Fatal("model did not register as connected")
	}

	info, err := snmp.Discover(src)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	m = feed(t, m, discoverResultMsg{info: info})

	vars, err := src.Walk(snmp.OIDmib2)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(vars) == 0 {
		t.Fatal("demo walk returned nothing")
	}
	m = feed(t, m, walkResultMsg{root: snmp.OIDmib2, vars: vars})

	poll, _ := src.Get([]string{snmp.OIDsysUpTime, "1.3.6.1.2.1.2.2.1.10.2"})
	m = feed(t, m, pollResultMsg{scope: "browser", vars: poll})
	m = feed(t, m, pollResultMsg{scope: "system", vars: poll})

	for tab := tabSystem; int(tab) < len(tabNames); tab++ {
		m.activeTab = tab
		out := m.View()
		if strings.TrimSpace(out) == "" {
			t.Fatalf("tab %s rendered empty", tabNames[tab])
		}
		if lines := strings.Count(out, "\n") + 1; lines != 44 {
			t.Fatalf("tab %s rendered %d lines, want 44", tabNames[tab], lines)
		}
	}
}

// TestKeyRouting exercises the global key handler and a few view keys.
func TestKeyRouting(t *testing.T) {
	m := newModel(config.Default(), Options{Demo: true})
	m = feed(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	src := snmp.NewDemo()
	_ = src.Connect()
	m = feed(t, m, connectResultMsg{src: src, describe: src.Describe(), target: src.Target()})

	key := func(s string) tea.KeyMsg {
		switch s {
		case "tab":
			return tea.KeyMsg{Type: tea.KeyTab}
		case "enter":
			return tea.KeyMsg{Type: tea.KeyEnter}
		default:
			return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
		}
	}

	m = feed(t, m, key("2")) // Browser
	if m.activeTab != tabBrowser {
		t.Fatalf("digit key did not switch to Browser (got %v)", m.activeTab)
	}
	m = feed(t, m, key("]"))
	if m.activeTab != tabGraph {
		t.Fatalf("] did not advance to Graph (got %v)", m.activeTab)
	}
	m = feed(t, m, key("["))
	if m.activeTab != tabBrowser {
		t.Fatalf("[ did not go back to Browser (got %v)", m.activeTab)
	}
	// open + close the connection modal
	m = feed(t, m, key("c"))
	if !m.showConnect {
		t.Fatal("c did not open the connection modal")
	}
	m = feed(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.showConnect {
		t.Fatal("esc did not close the connection modal")
	}
	_ = m.View()
}
