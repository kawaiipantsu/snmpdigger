package tui

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kawaiipantsu/snmpdigger/internal/snmp"
)

// --- messages / commands -------------------------------------------------

type profileMsg struct {
	profile snmp.NetworkProfile
}
type osintMsg struct {
	osint snmp.OSINT
}

func profileCmd(src snmp.Source, sys snmp.SystemInfo) tea.Cmd {
	return func() tea.Msg { return profileMsg{profile: snmp.BuildProfile(src, sys)} }
}

func osintCmd(host string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		ip := host
		if net.ParseIP(host) == nil {
			if ips, err := net.DefaultResolver.LookupHost(ctx, host); err == nil {
				for _, cand := range ips {
					if net.ParseIP(cand) != nil {
						ip = cand
						break
					}
				}
			}
		}
		return osintMsg{osint: snmp.EnrichOSINT(ctx, ip)}
	}
}

// --- view --------------------------------------------------------------

type summaryView struct {
	profile    *snmp.NetworkProfile
	osint      *snmp.OSINT
	building   bool
	osintWait  bool
	scroll     int
	exported   string
	targetHost string
}

func newSummaryView() summaryView { return summaryView{} }

func (v *summaryView) setTheme(Styles)          {}
func (v *summaryView) pollOIDs(*Model) []string { return nil }

func (v *summaryView) help() string {
	return "↑/↓/pgup/pgdn scroll · r rebuild · e export report"
}

func (v *summaryView) onConnected(m *Model) tea.Cmd { return v.rebuild(m) }

func (v *summaryView) rebuild(m *Model) tea.Cmd {
	if m.src == nil {
		return nil
	}
	v.building = true
	v.profile = nil
	v.osint = nil
	v.osintWait = false
	v.exported = ""
	v.scroll = 0
	v.targetHost = strings.SplitN(m.connTarget, ":", 2)[0]
	return tea.Batch(status("Building network profile…", stInfo, true),
		profileCmd(m.src, m.sys))
}

func (v *summaryView) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case profileMsg:
		p := msg.profile
		v.profile = &p
		v.building = false
		if v.targetHost != "" && v.targetHost != "demo" {
			v.osintWait = true
			return tea.Batch(status("Profile built — looking up ASN / geo…", stInfo, true),
				osintCmd(v.targetHost))
		}
		return status("Network profile ready", stGood, false)

	case osintMsg:
		o := msg.osint
		v.osint = &o
		v.osintWait = false
		return status("Network profile ready (incl. ASN / geo)", stGood, false)

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			return v.rebuild(m)
		case "e":
			return v.export()
		case "up", "k":
			if v.scroll > 0 {
				v.scroll--
			}
		case "down", "j":
			v.scroll++
		case "pgup":
			v.scroll = maxInt(0, v.scroll-15)
		case "pgdown":
			v.scroll += 15
		case "home", "g":
			v.scroll = 0
		}
	}
	return nil
}

type reportSection struct {
	title string
	rows  []string
}

func (v *summaryView) sections(spin string) []reportSection {
	if v.profile == nil {
		return nil
	}
	p := v.profile
	kv := func(k, val string) string {
		if strings.TrimSpace(val) == "" {
			val = "—"
		}
		return fmt.Sprintf("  %-16s %s", k, val)
	}
	var secs []reportSection

	secs = append(secs, reportSection{"IDENTITY", []string{
		kv("sysName", p.Name),
		kv("vendor", p.Vendor),
		kv("model", joinNonEmpty(" · ", p.Model, p.Serial)),
		kv("software", joinNonEmpty(" · ", p.SWRev, p.HWRev)),
		kv("role", p.Role),
		kv("sysObjectID", p.ObjectID),
		kv("uptime", humanDur(p.Uptime)),
		kv("sysContact", p.Contact),
		kv("sysLocation", p.Location),
		kv("services", strings.Join(p.Layers, ", ")),
		kv("sysDescr", oneLineText(p.Descr)),
	}})

	// network position (OSINT)
	np := []string{}
	if v.targetHost == "demo" {
		np = append(np, "  (demo mode — no external lookups)")
	} else if v.osintWait {
		np = append(np, "  "+spin+" resolving reverse DNS, ASN and geo…")
	} else if v.osint == nil {
		np = append(np, "  not looked up yet")
	} else {
		o := v.osint
		np = append(np,
			kv("target", v.targetHost),
			kv("reverse DNS", o.PTR))
		if o.Private {
			np = append(np, "  "+"address is RFC1918 / private — no public intelligence")
		} else {
			np = append(np,
				kv("ASN", joinNonEmpty("  ", o.ASN, o.ASNHolder)),
				kv("prefix", o.Prefix),
				kv("RIR", strings.ToUpper(o.RIR)),
				kv("geo", geoLine(o)))
			if o.Err != "" {
				np = append(np, "  lookup note: "+o.Err)
			}
		}
	}
	secs = append(secs, reportSection{"NETWORK POSITION", np})

	// addressing
	addr := make([]string, 0, len(p.Addrs)+1)
	for _, a := range p.Addrs {
		addr = append(addr, fmt.Sprintf("  %-18s / %-15s  if%d", a.IP, dashText(a.Mask), a.IfIndex))
	}
	if len(addr) == 0 {
		addr = []string{"  (no ipAddrTable data)"}
	}
	secs = append(secs, reportSection{"ADDRESSING", addr})

	// interfaces
	ifs := []string{fmt.Sprintf("  %d interfaces · %d up", p.IfTotal, p.IfUp)}
	for i, b := range p.Ifaces {
		if i >= 14 {
			ifs = append(ifs, fmt.Sprintf("  … %d more (see Interfaces tab)", len(p.Ifaces)-i))
			break
		}
		ifs = append(ifs, fmt.Sprintf("  if%-3d %-16s %-9s %-5s %s",
			b.Index, truncate(b.Name, 16), shortSpeed(b.Speed), statusWord(b.Oper), dashText(b.MAC)))
	}
	secs = append(secs, reportSection{"INTERFACES", ifs})

	// routing
	rt := []string{
		fmt.Sprintf("  forwarding: %v   default gw: %s   routes: %d%s",
			p.IPForwarding, dashText(p.DefaultGW), len(p.Routes), boolStr(p.RouteTruncated, " (truncated)", "")),
	}
	for i, r := range p.Routes {
		if i >= 16 {
			rt = append(rt, fmt.Sprintf("  … %d more routes", len(p.Routes)-i))
			break
		}
		rt = append(rt, fmt.Sprintf("  %-18s / %-15s -> %-15s  %-6s %s",
			r.Dest, dashText(r.Mask), dashText(r.NextHop), r.Proto, r.Type))
	}
	if len(p.Routes) == 0 {
		rt = append(rt, "  (no routing table exposed)")
	}
	secs = append(secs, reportSection{"ROUTING", rt})

	// neighbours
	nb := []string{}
	for i, a := range p.ARP {
		if i >= 16 {
			nb = append(nb, fmt.Sprintf("  … %d more ARP entries", len(p.ARP)-i))
			break
		}
		nb = append(nb, fmt.Sprintf("  %-16s %-18s if%-3d %s", a.IP, dashText(a.MAC), a.IfIndex, a.Vendor))
	}
	for _, l := range p.LLDP {
		nb = append(nb, fmt.Sprintf("  LLDP  %-24s port %s  chassis %s",
			dashText(l.RemSys), dashText(l.RemPort), dashText(l.RemChassis)))
	}
	if len(nb) == 0 {
		nb = []string{"  (no ARP / LLDP neighbours)"}
	}
	secs = append(secs, reportSection{"NEIGHBOURS", nb})

	// services / os
	svc := []string{
		kv("TCP established", fmt.Sprintf("%d", p.TCPEstab)),
		kv("listening TCP", portsList(p.ListenTCP)),
		kv("processes", fmt.Sprintf("%d", p.Processes)),
		kv("users", fmt.Sprintf("%d", p.Users)),
		kv("memory", byteSize(p.MemBytes)),
	}
	for _, s := range p.Storage {
		pct := 0
		if s.SizeBytes > 0 {
			pct = int(s.UsedBytes * 100 / s.SizeBytes)
		}
		svc = append(svc, fmt.Sprintf("  %-20s %s / %s  (%d%%)",
			truncate(s.Descr, 20), byteSize(s.UsedBytes), byteSize(s.SizeBytes), pct))
	}
	secs = append(secs, reportSection{"SERVICES / OS", svc})

	// analysis
	an := []string{}
	for _, n := range p.Notes {
		an = append(an, "  • "+n)
	}
	if v.osint != nil && !v.osint.Private && v.osint.ASN != "" {
		an = append(an, "  • SNMP is reachable on a publicly-routed address ("+v.osint.ASN+" "+v.osint.ASNHolder+")")
	}
	if len(an) == 0 {
		an = []string{"  • nothing notable"}
	}
	secs = append(secs, reportSection{"ANALYSIS", an})

	if len(p.Errors) > 0 {
		secs = append(secs, reportSection{"COLLECTION ERRORS", prefixAll("  ", p.Errors)})
	}
	return secs
}

func (v *summaryView) view(m *Model) string {
	st := m.st
	if !m.connected {
		return centeredHint(m, "Not connected. Press "+st.Key.Render("c")+" to connect, then this tab profiles the target.")
	}
	if v.building || v.profile == nil {
		return centeredHint(m, m.spin.View()+" walking the device and assembling its profile…")
	}

	var lines []string
	head := st.PanelTitle.Render("NETWORK PROFILE") +
		st.Dim.Render(fmt.Sprintf("   %s · built %s", v.targetHost, v.profile.FetchedAt.Format("15:04:05")))
	if v.exported != "" {
		head += st.Good.Render("   ·  exported → " + v.exported)
	}
	lines = append(lines, head, "")

	for _, s := range v.sections(m.spin.View()) {
		lines = append(lines, st.Accent.Bold(true).Render("── "+s.title+" "+strings.Repeat("─", maxInt(0, 40-len(s.title)))))
		for _, r := range s.rows {
			lines = append(lines, st.Dim.Render(r))
		}
		lines = append(lines, "")
	}

	total := len(lines)
	if v.scroll > total-3 {
		v.scroll = maxInt(0, total-3)
	}
	body := clip(strings.Join(lines, "\n"), v.scroll, m.ch-1)
	more := ""
	if v.scroll+m.ch-1 < total {
		more = st.Dim.Render(fmt.Sprintf("  ▼ %d more lines — ↓ / pgdn", total-(v.scroll+m.ch-1)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, more)
}

func (v *summaryView) export() tea.Cmd {
	if v.profile == nil {
		return status("profile not built yet", stWarn, false)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# snmpdigger network profile — %s\n# built %s\n\n",
		v.targetHost, v.profile.FetchedAt.Format(time.RFC3339))
	for _, s := range v.sections("") {
		fmt.Fprintf(&b, "## %s\n", s.title)
		for _, r := range s.rows {
			fmt.Fprintf(&b, "%s\n", strings.TrimRight(r, " "))
		}
		b.WriteString("\n")
	}
	path, err := exportText("profile", b.String())
	if err != nil {
		return status("export failed: "+err.Error(), stBad, false)
	}
	v.exported = path
	return status("exported network profile → "+path, stGood, false)
}

// --- helpers ---------------------------------------------------------

func geoLine(o *snmp.OSINT) string {
	parts := []string{}
	if o.GeoCity != "" {
		parts = append(parts, o.GeoCity)
	}
	if o.GeoCountry != "" {
		parts = append(parts, o.GeoCountry)
	}
	loc := strings.Join(parts, ", ")
	if o.GeoLat != 0 || o.GeoLon != 0 {
		loc += fmt.Sprintf("  (%.4f, %.4f)", o.GeoLat, o.GeoLon)
	}
	return loc
}

func joinNonEmpty(sep string, parts ...string) string {
	out := parts[:0]
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

func prefixAll(pre string, in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = pre + s
	}
	return out
}

func portsList(ps []int) string {
	if len(ps) == 0 {
		return "—"
	}
	s := make([]string, 0, len(ps))
	for i, p := range ps {
		if i >= 20 {
			s = append(s, fmt.Sprintf("… +%d", len(ps)-i))
			break
		}
		s = append(s, fmt.Sprintf("%d", p))
	}
	return strings.Join(s, " ")
}

func byteSize(n int64) string {
	f := float64(n)
	switch {
	case n <= 0:
		return "—"
	case f >= 1<<40:
		return fmt.Sprintf("%.1f TB", f/(1<<40))
	case f >= 1<<30:
		return fmt.Sprintf("%.1f GB", f/(1<<30))
	case f >= 1<<20:
		return fmt.Sprintf("%.1f MB", f/(1<<20))
	case f >= 1<<10:
		return fmt.Sprintf("%.1f KB", f/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
