package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kawaiipantsu/snmpdigger/internal/config"
	"github.com/kawaiipantsu/snmpdigger/internal/snmp"
)

type statusLevel int

const (
	stInfo statusLevel = iota
	stGood
	stWarn
	stBad
)

// --- messages ---

type connectResultMsg struct {
	src      snmp.Source
	describe string
	target   string
	err      error
}

type discoverResultMsg struct {
	info snmp.SystemInfo
	err  error
}

type walkResultMsg struct {
	root string
	vars []snmp.Var
	err  error
}

type pollResultMsg struct {
	scope string
	vars  []snmp.Var
	at    time.Time
	err   error
}

type tickMsg time.Time
type clockMsg time.Time

type statusMsg struct {
	text  string
	level statusLevel
	busy  bool
}

type scanUpdateMsg struct {
	done  int
	total int
	found []snmp.Found
	err   error
	final bool
}

// --- commands ---

func connectCmd(cfg *config.Config, conn config.Connection, demo bool) tea.Cmd {
	return func() tea.Msg {
		if demo {
			src := snmp.NewDemo()
			_ = src.Connect()
			return connectResultMsg{src: src, describe: src.Describe(), target: src.Target()}
		}
		src, err := snmp.NewLive(conn, cfg.Poll)
		if err != nil {
			return connectResultMsg{err: err}
		}
		if err := src.Connect(); err != nil {
			return connectResultMsg{err: err}
		}
		return connectResultMsg{src: src, describe: src.Describe(), target: src.Target()}
	}
}

func discoverSystemCmd(src snmp.Source) tea.Cmd {
	return func() tea.Msg {
		info, err := snmp.Discover(src)
		return discoverResultMsg{info: info, err: err}
	}
}

func walkCmd(src snmp.Source, root string) tea.Cmd {
	return func() tea.Msg {
		vars, err := src.Walk(root)
		return walkResultMsg{root: root, vars: vars, err: err}
	}
}

func pollCmd(src snmp.Source, scope string, oids []string) tea.Cmd {
	if len(oids) == 0 || src == nil {
		return nil
	}
	return func() tea.Msg {
		vars, err := src.Get(oids)
		return pollResultMsg{scope: scope, vars: vars, at: time.Now(), err: err}
	}
}

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func clockTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return clockMsg(t) })
}

func status(text string, level statusLevel, busy bool) tea.Cmd {
	return func() tea.Msg { return statusMsg{text: text, level: level, busy: busy} }
}
