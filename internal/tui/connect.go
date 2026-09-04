package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kawaiipantsu/snmpdigger/internal/config"
)

type fldKind int

const (
	fHost fldKind = iota
	fPort
	fVersion
	fCommunity
	fUser
	fSecLevel
	fAuthProto
	fAuthPass
	fPrivProto
	fPrivPass
	fContext
	fConnect
	fDemo
	fCancel
)

type connectModel struct {
	st Styles

	host, port, community             textinput.Model
	user, authPass, privPass, context textinput.Model

	version   selector
	secLevel  selector
	authProto selector
	privProto selector

	order []fldKind
	focus int

	showSecrets bool
	err         string
}

func newConnectModel(st Styles, last config.Connection) connectModel {
	mk := func(val, ph string, width int) textinput.Model {
		ti := textinput.New()
		ti.SetValue(val)
		ti.Placeholder = ph
		ti.Prompt = ""
		ti.Width = width
		return ti
	}
	port := last.Port
	if port == 0 {
		port = 161
	}
	m := connectModel{
		st:        st,
		host:      mk(last.Host, "10.0.0.1 / device.example.net", 34),
		port:      mk(strconv.Itoa(int(port)), "161", 8),
		community: mk(last.Community, "public", 24),
		user:      mk(last.Username, "snmpuser", 24),
		authPass:  mk(last.AuthPass, "auth passphrase", 24),
		privPass:  mk(last.PrivPass, "priv passphrase", 24),
		context:   mk(last.ContextName, "(optional)", 24),
		version:   newSelector("Version", []string{"v1", "v2c", "v3"}, orDefault(last.Version, "v2c")),
		secLevel:  newSelector("Sec level", []string{"noAuthNoPriv", "authNoPriv", "authPriv"}, orDefault(last.SecLevel, "authPriv")),
		authProto: newSelector("Auth", []string{"MD5", "SHA", "SHA224", "SHA256", "SHA384", "SHA512"}, orDefault(last.AuthProto, "SHA")),
		privProto: newSelector("Priv", []string{"DES", "AES", "AES192", "AES256"}, orDefault(last.PrivProto, "AES")),
	}
	m.authPass.EchoMode = textinput.EchoPassword
	m.privPass.EchoMode = textinput.EchoPassword
	m.community.EchoMode = textinput.EchoPassword
	m.rebuild()
	m.applyFocus()
	return m
}

func orDefault(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

// rebuild recomputes the focus order given the chosen version / security level.
func (m *connectModel) rebuild() {
	o := []fldKind{fHost, fPort, fVersion}
	switch m.version.value() {
	case "v3":
		o = append(o, fUser, fSecLevel)
		switch m.secLevel.value() {
		case "authNoPriv":
			o = append(o, fAuthProto, fAuthPass)
		case "authPriv":
			o = append(o, fAuthProto, fAuthPass, fPrivProto, fPrivPass)
		}
		o = append(o, fContext)
	default:
		o = append(o, fCommunity)
	}
	o = append(o, fConnect, fDemo, fCancel)
	m.order = o
	if m.focus >= len(o) {
		m.focus = len(o) - 1
	}
}

func (m *connectModel) applyFocus() {
	for _, ti := range []*textinput.Model{&m.host, &m.port, &m.community, &m.user, &m.authPass, &m.privPass, &m.context} {
		ti.Blur()
	}
	switch m.current() {
	case fHost:
		m.host.Focus()
	case fPort:
		m.port.Focus()
	case fCommunity:
		m.community.Focus()
	case fUser:
		m.user.Focus()
	case fAuthPass:
		m.authPass.Focus()
	case fPrivPass:
		m.privPass.Focus()
	case fContext:
		m.context.Focus()
	}
}

func (m *connectModel) current() fldKind { return m.order[m.focus] }

func (m *connectModel) move(delta int) {
	m.focus = (m.focus + delta + len(m.order)) % len(m.order)
	m.applyFocus()
}

// connectIntent is returned to the parent when the form is submitted.
type connectIntent struct {
	conn   config.Connection
	demo   bool
	cancel bool
}

func (m *connectModel) update(msg tea.Msg) (tea.Cmd, *connectIntent) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, nil
	}
	switch km.String() {
	case "esc":
		return nil, &connectIntent{cancel: true}
	case "tab", "down":
		m.move(1)
		return nil, nil
	case "shift+tab", "up":
		m.move(-1)
		return nil, nil
	case "ctrl+s":
		m.toggleSecrets()
		return nil, nil
	case "left":
		m.selector(-1)
		return nil, nil
	case "right":
		m.selector(1)
		return nil, nil
	case "enter", "ctrl+g":
		switch m.current() {
		case fCancel:
			return nil, &connectIntent{cancel: true}
		case fDemo:
			return nil, &connectIntent{demo: true}
		case fConnect:
			return m.submit()
		default:
			if km.String() == "ctrl+g" {
				return m.submit()
			}
			m.move(1)
			return nil, nil
		}
	}

	// route rune input to the focused text field
	var cmd tea.Cmd
	switch m.current() {
	case fHost:
		m.host, cmd = m.host.Update(msg)
	case fPort:
		m.port, cmd = m.port.Update(msg)
	case fCommunity:
		m.community, cmd = m.community.Update(msg)
	case fUser:
		m.user, cmd = m.user.Update(msg)
	case fAuthPass:
		m.authPass, cmd = m.authPass.Update(msg)
	case fPrivPass:
		m.privPass, cmd = m.privPass.Update(msg)
	case fContext:
		m.context, cmd = m.context.Update(msg)
	}
	return cmd, nil
}

func (m *connectModel) toggleSecrets() {
	m.showSecrets = !m.showSecrets
	mode := textinput.EchoPassword
	if m.showSecrets {
		mode = textinput.EchoNormal
	}
	m.authPass.EchoMode = mode
	m.privPass.EchoMode = mode
	m.community.EchoMode = mode
}

func (m *connectModel) selector(delta int) {
	pick := func(s *selector) {
		if delta > 0 {
			s.next()
		} else {
			s.prev()
		}
	}
	switch m.current() {
	case fVersion:
		pick(&m.version)
		m.rebuild()
		m.applyFocus()
	case fSecLevel:
		pick(&m.secLevel)
		m.rebuild()
		m.applyFocus()
	case fAuthProto:
		pick(&m.authProto)
	case fPrivProto:
		pick(&m.privProto)
	}
}

func (m *connectModel) submit() (tea.Cmd, *connectIntent) {
	host := strings.TrimSpace(m.host.Value())
	if host == "" {
		m.err = "host is required"
		return nil, nil
	}
	port, err := strconv.Atoi(strings.TrimSpace(m.port.Value()))
	if err != nil || port < 1 || port > 65535 {
		m.err = "port must be 1-65535"
		return nil, nil
	}
	conn := config.Connection{
		Host:    host,
		Port:    uint16(port),
		Version: m.version.value(),
	}
	if m.version.value() == "v3" {
		conn.Username = strings.TrimSpace(m.user.Value())
		conn.SecLevel = m.secLevel.value()
		conn.AuthProto = m.authProto.value()
		conn.AuthPass = m.authPass.Value()
		conn.PrivProto = m.privProto.value()
		conn.PrivPass = m.privPass.Value()
		conn.ContextName = strings.TrimSpace(m.context.Value())
		if conn.Username == "" {
			m.err = "v3 requires a username"
			return nil, nil
		}
	} else {
		conn.Community = m.community.Value()
		if conn.Community == "" {
			m.err = "community string is required"
			return nil, nil
		}
	}
	m.err = ""
	return nil, &connectIntent{conn: conn}
}

func (m *connectModel) view(w, h int) string {
	st := m.st
	title := st.ModalTitle.Render("  SNMP CONNECTION  ")

	row := func(k fldKind, label, field string) string {
		lbl := st.Label
		if m.current() == k {
			lbl = st.LabelActive
		}
		return lipgloss.JoinHorizontal(lipgloss.Top,
			lbl.Render(label+" "), "  ", field)
	}
	tf := func(k fldKind, ti textinput.Model) string {
		box := st.Field
		if m.current() == k {
			box = st.FieldActive
		}
		return box.Render(padRight(ti.View(), ti.Width+1))
	}

	var lines []string
	lines = append(lines, row(fHost, "Host", tf(fHost, m.host)))
	lines = append(lines, row(fPort, "Port", tf(fPort, m.port)))
	lines = append(lines, row(fVersion, "Version", m.version.render(st, m.current() == fVersion)))

	if m.version.value() == "v3" {
		lines = append(lines, row(fUser, "Username", tf(fUser, m.user)))
		lines = append(lines, row(fSecLevel, "Sec level", m.secLevel.render(st, m.current() == fSecLevel)))
		switch m.secLevel.value() {
		case "authNoPriv":
			lines = append(lines, row(fAuthProto, "Auth proto", m.authProto.render(st, m.current() == fAuthProto)))
			lines = append(lines, row(fAuthPass, "Auth pass", tf(fAuthPass, m.authPass)))
		case "authPriv":
			lines = append(lines, row(fAuthProto, "Auth proto", m.authProto.render(st, m.current() == fAuthProto)))
			lines = append(lines, row(fAuthPass, "Auth pass", tf(fAuthPass, m.authPass)))
			lines = append(lines, row(fPrivProto, "Priv proto", m.privProto.render(st, m.current() == fPrivProto)))
			lines = append(lines, row(fPrivPass, "Priv pass", tf(fPrivPass, m.privPass)))
		}
		lines = append(lines, row(fContext, "Context", tf(fContext, m.context)))
	} else {
		lines = append(lines, row(fCommunity, "Community", tf(fCommunity, m.community)))
	}

	btn := func(k fldKind, text string) string {
		s := lipgloss.NewStyle().Foreground(st.T.Dim).Padding(0, 2).
			Border(lipgloss.RoundedBorder()).BorderForeground(st.T.Faint)
		if m.current() == k {
			s = lipgloss.NewStyle().Foreground(lipgloss.Color("#0b0b0b")).
				Background(st.T.Accent).Bold(true).Padding(0, 2).
				Border(lipgloss.RoundedBorder()).BorderForeground(st.T.Accent)
		}
		return s.Render(text)
	}
	buttons := lipgloss.JoinHorizontal(lipgloss.Top,
		btn(fConnect, "Connect"), "  ", btn(fDemo, "Demo mode"), "  ", btn(fCancel, "Cancel"))

	help := keyHint(st,
		[2]string{"tab", "next"}, [2]string{"←/→", "change"},
		[2]string{"ctrl+s", "show secrets"}, [2]string{"ctrl+g", "connect"}, [2]string{"esc", "cancel"})

	body := strings.Join(lines, "\n") + "\n\n" + buttons
	if m.err != "" {
		body += "\n\n" + st.Bad.Render("✖ "+m.err)
	}
	body += "\n\n" + help

	card := st.Modal.Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body))
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, card)
}

// lastConn builds a config.Connection from the current field values (best
// effort, no validation). app.go calls this right after a successful connect
// to persist the profile.
func (m *connectModel) lastConn() config.Connection {
	port := 161
	if p, err := strconv.Atoi(strings.TrimSpace(m.port.Value())); err == nil && p > 0 && p < 65536 {
		port = p
	}
	conn := config.Connection{
		Host:    strings.TrimSpace(m.host.Value()),
		Port:    uint16(port),
		Version: m.version.value(),
	}
	if m.version.value() == "v3" {
		conn.Username = strings.TrimSpace(m.user.Value())
		conn.SecLevel = m.secLevel.value()
		conn.AuthProto = m.authProto.value()
		conn.AuthPass = m.authPass.Value()
		conn.PrivProto = m.privProto.value()
		conn.PrivPass = m.privPass.Value()
		conn.ContextName = strings.TrimSpace(m.context.Value())
	} else {
		conn.Community = m.community.Value()
	}
	return conn
}

// summary is a compact one-liner describing the pending form values.
func (m *connectModel) summary() string {
	if m.version.value() == "v3" {
		return fmt.Sprintf("%s:%s v3/%s user=%s", m.host.Value(), m.port.Value(), m.secLevel.value(), m.user.Value())
	}
	return fmt.Sprintf("%s:%s %s", m.host.Value(), m.port.Value(), m.version.value())
}
