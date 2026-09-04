// Package cli implements snmpdigger's non-interactive subcommands (discover,
// walk, get, identify, config, version). The interactive TUI lives in
// internal/tui and is launched from package main when no subcommand is given.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kawaiipantsu/snmpdigger/internal/config"
	"github.com/kawaiipantsu/snmpdigger/internal/mib"
	"github.com/kawaiipantsu/snmpdigger/internal/snmp"
	"github.com/kawaiipantsu/snmpdigger/internal/version"
)

// Main dispatches a subcommand. args is the full os.Args slice; args[1] is the
// subcommand. It returns a process exit code.
func Main(args []string) int {
	if len(args) < 2 {
		usage(os.Stderr)
		return 2
	}
	switch args[1] {
	case "discover":
		return cmdDiscover(args[2:])
	case "walk":
		return cmdWalk(args[2:])
	case "get":
		return cmdGet(args[2:])
	case "identify":
		return cmdIdentify(args[2:])
	case "monitor":
		return cmdMonitor(args[2:])
	case "config":
		return cmdConfig(args[2:])
	case "version", "--version", "-v":
		fmt.Println(version.Full())
		return 0
	case "help", "-h", "--help":
		usage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "snmpdigger: unknown subcommand %q\n\n", args[1])
		usage(os.Stderr)
		return 2
	}
}

// ---------------------------------------------------------------------------
// shared connection flags
// ---------------------------------------------------------------------------

// ConnSpec holds the values of the connection flags shared by every subcommand
// (and reused by the TUI launcher in package main).
type ConnSpec struct {
	Community string
	Version   string
	Port      uint
	Timeout   int
	Retries   int

	User      string
	Level     string
	AuthProto string
	AuthPass  string
	PrivProto string
	PrivPass  string
	Context   string
}

// ConnFlags registers the common connection flags on fs and returns the spec
// they populate after fs.Parse.
func ConnFlags(fs *flag.FlagSet) *ConnSpec {
	c := &ConnSpec{}
	fs.StringVar(&c.Community, "community", "public", "SNMP v1/v2c community string")
	fs.StringVar(&c.Version, "snmp", "v2c", "SNMP version: v1, v2c or v3")
	fs.StringVar(&c.Version, "version", "v2c", "alias for --snmp")
	fs.UintVar(&c.Port, "port", 161, "agent UDP port")
	fs.IntVar(&c.Timeout, "timeout", 2, "per-request timeout in seconds")
	fs.IntVar(&c.Retries, "retries", 1, "retries on timeout")
	fs.StringVar(&c.User, "user", "", "SNMPv3 security name")
	fs.StringVar(&c.Level, "level", "authPriv", "SNMPv3 security level: noAuthNoPriv|authNoPriv|authPriv")
	fs.StringVar(&c.AuthProto, "auth-proto", "SHA", "SNMPv3 auth protocol: MD5|SHA|SHA224|SHA256|SHA384|SHA512")
	fs.StringVar(&c.AuthPass, "auth-pass", "", "SNMPv3 auth passphrase")
	fs.StringVar(&c.PrivProto, "priv-proto", "AES", "SNMPv3 privacy protocol: DES|AES|AES192|AES256")
	fs.StringVar(&c.PrivPass, "priv-pass", "", "SNMPv3 privacy passphrase")
	fs.StringVar(&c.Context, "context", "", "SNMPv3 context name")
	return c
}

// Connection builds a config.Connection for the given host from the spec.
func (c *ConnSpec) Connection(host string) config.Connection {
	return config.Connection{
		Host:        host,
		Port:        uint16(c.Port),
		Version:     normVer(c.Version),
		Community:   c.Community,
		Username:    c.User,
		SecLevel:    c.Level,
		AuthProto:   c.AuthProto,
		AuthPass:    c.AuthPass,
		PrivProto:   c.PrivProto,
		PrivPass:    c.PrivPass,
		ContextName: c.Context,
	}
}

// Poll builds a reasonable config.Poll for one-shot commands.
func (c *ConnSpec) Poll() config.Poll {
	t := c.Timeout
	if t < 1 {
		t = 1
	}
	return config.Poll{
		IntervalSeconds: 3,
		TimeoutSeconds:  t,
		Retries:         c.Retries,
		MaxOIDsPerReq:   40,
		MaxRepetitions:  20,
	}
}

func normVer(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "v1":
		return "v1"
	case "3", "v3":
		return "v3"
	default:
		return "v2c"
	}
}

// ---------------------------------------------------------------------------
// discover
// ---------------------------------------------------------------------------

func cmdDiscover(args []string) int {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	spec := ConnFlags(fs)
	concurrency := fs.Int("concurrency", 192, "number of parallel probes")
	communities := fs.String("communities", "", "comma-separated community list (overrides --community)")
	asn := fs.String("asn", "", "resolve and scan every IPv4 prefix announced by this AS (e.g. AS13335)")
	local := fs.Bool("local", false, "scan this host's private networks plus common LAN /24s")
	maxHosts := fs.Int("max-hosts", snmp.DefaultMaxHosts, "cap on addresses probed in one run")
	asJSON := fs.Bool("json", false, "emit results as JSON")
	fs.Usage = subUsage(fs, "discover <cidr> | --asn <n> | --local",
		"Sweep IPv4 addresses for SNMP agents and identify each responder from its\nsystem group. Give a CIDR/IP, an AS number (--asn), or --local.")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	comms := []string{spec.Community}
	if *communities != "" {
		comms = splitComma(*communities)
	}

	var targets []string
	var label string
	switch {
	case *local:
		targets = snmp.LocalScanTargets()
		label = "local networks (" + snmp.SummarizeTargets(targets) + ")"
	case strings.TrimSpace(*asn) != "":
		fmt.Fprintf(os.Stderr, "[+] Resolving prefixes for %s ...\n", *asn)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		info, err := snmp.ResolveASN(ctx, *asn)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "snmpdigger: %v\n", err)
			return 1
		}
		targets = info.Prefixes
		if *maxHosts == snmp.DefaultMaxHosts {
			*maxHosts = 1 << 21
		}
		label = fmt.Sprintf("%s (%s) — %d prefixes, %s", info.ASN, info.Holder, len(info.Prefixes), snmp.SummarizeTargets(info.Prefixes))
	case fs.NArg() >= 1:
		targets = []string{fs.Arg(0)}
		label = fs.Arg(0)
	default:
		fmt.Fprintln(os.Stderr, "snmpdigger: discover needs a <cidr>, --asn <n>, or --local")
		fs.Usage()
		return 2
	}

	opts := snmp.ScanOptions{
		Targets:     targets,
		Port:        uint16(spec.Port),
		Version:     normVer(spec.Version),
		Communities: comms,
		Base:        spec.Connection(""),
		Timeout:     time.Duration(spec.Timeout) * time.Second,
		Retries:     spec.Retries,
		Concurrency: *concurrency,
		MaxHosts:    *maxHosts,
	}

	fmt.Fprintf(os.Stderr, "[+] Scanning %s ...\n", label)
	lastPct := -1
	found, err := snmp.ScanCIDR(context.Background(), opts, func(done, total int) {
		if total == 0 {
			return
		}
		pct := done * 100 / total
		if pct != lastPct && pct%5 == 0 {
			fmt.Fprintf(os.Stderr, "\r[+] probed %d/%d (%d%%)      ", done, total, pct)
			lastPct = pct
		}
	})
	fmt.Fprintln(os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "snmpdigger: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "[+] Discovered %d SNMP device(s)\n", len(found))

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(found)
		return 0
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "IP\tDEVICE\tSNMP\tUPTIME\tINFO")
	fmt.Fprintln(tw, "----\t------\t----\t------\t----")
	for _, f := range found {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			f.IP, trunc(f.Short(), 20), f.Version, shortDur(f.Uptime), trunc(oneLine(f.SysDescr), 48))
	}
	_ = tw.Flush()
	return 0
}

// ---------------------------------------------------------------------------
// walk / get
// ---------------------------------------------------------------------------

func cmdWalk(args []string) int {
	fs := flag.NewFlagSet("walk", flag.ContinueOnError)
	spec := ConnFlags(fs)
	numeric := fs.Bool("numeric", false, "do not resolve MIB names")
	asJSON := fs.Bool("json", false, "emit results as JSON")
	fs.Usage = subUsage(fs, "walk <host> [oid]",
		"Walk a subtree (default 1.3.6.1.2.1 / mib-2) and print every binding.")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "snmpdigger: walk requires a <host> argument")
		fs.Usage()
		return 2
	}
	host := fs.Arg(0)
	oid := snmp.OIDmib2
	if fs.NArg() >= 2 {
		oid = fs.Arg(1)
	}

	src, err := snmp.NewLive(spec.Connection(host), spec.Poll())
	if err != nil {
		return errf(err)
	}
	defer src.Close()
	if err := src.Connect(); err != nil {
		return errf(err)
	}
	vars, err := src.Walk(oid)
	if err != nil {
		return errf(err)
	}
	emit(vars, resolver(*numeric), *asJSON)
	return 0
}

func cmdGet(args []string) int {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	spec := ConnFlags(fs)
	numeric := fs.Bool("numeric", false, "do not resolve MIB names")
	asJSON := fs.Bool("json", false, "emit results as JSON")
	fs.Usage = subUsage(fs, "get <host> <oid> [oid...]",
		"GET one or more objects and print their values.")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "snmpdigger: get requires a <host> and at least one <oid>")
		fs.Usage()
		return 2
	}
	host := fs.Arg(0)
	oids := fs.Args()[1:]

	src, err := snmp.NewLive(spec.Connection(host), spec.Poll())
	if err != nil {
		return errf(err)
	}
	defer src.Close()
	if err := src.Connect(); err != nil {
		return errf(err)
	}
	vars, err := src.Get(oids)
	if err != nil {
		return errf(err)
	}
	emit(vars, resolver(*numeric), *asJSON)
	return 0
}

// ---------------------------------------------------------------------------
// identify
// ---------------------------------------------------------------------------

func cmdIdentify(args []string) int {
	fs := flag.NewFlagSet("identify", flag.ContinueOnError)
	spec := ConnFlags(fs)
	asJSON := fs.Bool("json", false, "emit the SystemInfo struct as JSON")
	fs.Usage = subUsage(fs, "identify <host>",
		"Fetch and analyse the SNMP system group: identity, vendor, role,\nownership and a few security observations.")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "snmpdigger: identify requires a <host> argument")
		fs.Usage()
		return 2
	}

	src, err := snmp.NewLive(spec.Connection(fs.Arg(0)), spec.Poll())
	if err != nil {
		return errf(err)
	}
	defer src.Close()
	if err := src.Connect(); err != nil {
		return errf(err)
	}
	info, err := snmp.Discover(src)
	if err != nil {
		return errf(err)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(info)
		return 0
	}

	row := func(k, v string) {
		if strings.TrimSpace(v) == "" {
			v = "-"
		}
		fmt.Printf("  %-14s %s\n", k, v)
	}
	vendor := info.Vendor
	if info.Enterprise > 0 {
		if vendor == "" {
			vendor = "unknown"
		}
		vendor = fmt.Sprintf("%s (enterprise %d)", vendor, info.Enterprise)
	}

	fmt.Printf("SNMP identity for %s\n\n", src.Target())
	row("sysName", info.Name)
	row("role", info.Role)
	row("vendor", vendor)
	row("sysObjectID", info.ObjectID)
	row("uptime", humanDur(info.Uptime))
	row("sysContact", info.Contact)
	row("sysLocation", info.Location)
	row("services", strings.Join(info.Layers, ", "))
	row("rtt", info.RTT.Round(time.Millisecond).String())
	row("sysDescr", oneLine(info.Descr))
	if len(info.Notes) > 0 {
		fmt.Println("\nanalysis")
		for _, n := range info.Notes {
			fmt.Printf("  ! %s\n", n)
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// monitor
// ---------------------------------------------------------------------------

func cmdMonitor(args []string) int {
	fs := flag.NewFlagSet("monitor", flag.ContinueOnError)
	spec := ConnFlags(fs)
	interval := fs.Int("interval", 5, "seconds between polls")
	count := fs.Int("count", 0, "number of polls (0 = run until interrupted)")
	delta := fs.Bool("delta", false, "print per-second rate for counter values")
	numeric := fs.Bool("numeric", false, "do not resolve MIB names")
	fs.Usage = subUsage(fs, "monitor <host> <oid> [oid...]",
		"Poll one or more objects on a fixed interval and print a timestamped line\nper tick (a scriptable `watch` for SNMP).")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "snmpdigger: monitor requires a <host> and at least one <oid>")
		fs.Usage()
		return 2
	}
	host := fs.Arg(0)
	oids := fs.Args()[1:]

	src, err := snmp.NewLive(spec.Connection(host), spec.Poll())
	if err != nil {
		return errf(err)
	}
	defer src.Close()
	if err := src.Connect(); err != nil {
		return errf(err)
	}
	res := resolver(*numeric)

	if *interval < 1 {
		*interval = 1
	}
	labels := make([]string, len(oids))
	for i, o := range oids {
		labels[i] = o
		if res != nil {
			if n := res.Name(o); n != "" && n != o {
				labels[i] = n
			}
		}
	}

	prev := map[string]float64{}
	prevT := time.Time{}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	for tick := 0; *count == 0 || tick < *count; tick++ {
		now := time.Now()
		vars, err := src.Get(oids)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s  error: %v\n", now.Format("15:04:05"), err)
		} else {
			var b strings.Builder
			b.WriteString(now.Format("15:04:05"))
			for i, v := range vars {
				val := v.Display()
				if *delta && v.Kind.String() == "Counter" && !prevT.IsZero() {
					dt := now.Sub(prevT).Seconds()
					if dt > 0 {
						d := v.Num - prev[v.OID]
						if d < 0 {
							d = v.Num
						}
						val = fmt.Sprintf("%.2f/s", d/dt)
					}
				}
				prev[v.OID] = v.Num
				name := labels[i]
				if i >= len(labels) {
					name = v.OID
				}
				fmt.Fprintf(&b, "  %s=%s", name, val)
			}
			fmt.Println(b.String())
		}
		prevT = now
		if *count != 0 && tick == *count-1 {
			break
		}
		select {
		case <-sig:
			fmt.Fprintln(os.Stderr, "\ninterrupted")
			return 0
		case <-time.After(time.Duration(*interval) * time.Second):
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// config
// ---------------------------------------------------------------------------

func cmdConfig(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: snmpdigger config <path|show>")
		return 2
	}
	switch args[0] {
	case "path":
		p, err := config.Path()
		if err != nil {
			return errf(err)
		}
		fmt.Println(p)
		return 0
	case "show":
		if _, err := config.Load(); err != nil { // ensures the file exists
			return errf(err)
		}
		p, err := config.Path()
		if err != nil {
			return errf(err)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return errf(err)
		}
		fmt.Printf("# %s\n", p)
		os.Stdout.Write(b)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "snmpdigger: unknown config subcommand %q (want path|show)\n", args[0])
		return 2
	}
}

// ---------------------------------------------------------------------------
// output helpers
// ---------------------------------------------------------------------------

type varJSON struct {
	OID   string  `json:"oid"`
	Name  string  `json:"name,omitempty"`
	Type  string  `json:"type"`
	Value string  `json:"value"`
	Num   float64 `json:"num,omitempty"`
}

func resolver(numeric bool) *mib.Resolver {
	if numeric {
		return nil
	}
	return mib.New(true)
}

func emit(vars []snmp.Var, res *mib.Resolver, asJSON bool) {
	if asJSON {
		recs := make([]varJSON, 0, len(vars))
		for _, v := range vars {
			r := varJSON{OID: v.OID, Type: v.Kind.String(), Value: v.Display(), Num: v.Num}
			if res != nil {
				if n := res.Name(v.OID); n != "" && n != v.OID {
					r.Name = n
				}
			}
			recs = append(recs, r)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(recs)
		return
	}
	for _, v := range vars {
		fmt.Println(varLine(v, res))
	}
}

func varLine(v snmp.Var, res *mib.Resolver) string {
	if res != nil {
		if n := res.Name(v.OID); n != "" && n != v.OID {
			return fmt.Sprintf("%s  (%s) = %s: %s", v.OID, n, v.Kind.String(), v.Display())
		}
	}
	return fmt.Sprintf("%s = %s: %s", v.OID, v.Kind.String(), v.Display())
}

// ---------------------------------------------------------------------------
// misc helpers
// ---------------------------------------------------------------------------

func errf(err error) int {
	fmt.Fprintf(os.Stderr, "snmpdigger: %v\n", err)
	return 1
}

func subUsage(fs *flag.FlagSet, use, desc string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "usage: snmpdigger %s\n\n%s\n\nflags:\n", use, desc)
		fs.PrintDefaults()
	}
}

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.Join(strings.Fields(s), " ")
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:maxi(n, 0)])
	}
	return string(r[:n-1]) + "…"
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func shortDur(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	m := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd%02dh", days, h)
	case h > 0:
		return fmt.Sprintf("%dh%02dm", h, m)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

func humanDur(d time.Duration) string {
	if d <= 0 {
		return "unknown"
	}
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, h, m, s)
	}
	return fmt.Sprintf("%dh %dm %ds", h, m, s)
}

func usage(w io.Writer) {
	fmt.Fprint(w, `snmpdigger — SNMP discovery, browsing and live graphing (THUGS red)

USAGE
    snmpdigger                       launch the fullscreen TUI (connection dialog)
    snmpdigger --host <h> [flags]    launch the TUI and connect immediately
    snmpdigger --demo                launch the TUI against a synthetic agent
    snmpdigger <command> [flags]     run a one-shot command

COMMANDS
    discover <cidr>          sweep an IPv4 range for SNMP agents and identify them
    discover --asn <n>       resolve an AS number's prefixes and scan them all
    discover --local         scan this host's private nets + common LAN /24s
    walk <host> [oid]        walk a subtree (default 1.3.6.1.2.1) and print bindings
    get <host> <oid>...      GET one or more objects
    monitor <host> <oid>...  poll objects on an interval, one line per tick
    identify <host>          fetch and analyse the system group
    config path|show         print the config file path or its contents
    version                  print version information
    help                     show this help

COMMON FLAGS
    --community <s>   v1/v2c community string        (default "public")
    --snmp <v>        SNMP version: v1 | v2c | v3     (default "v2c")
    --port <n>        agent UDP port                  (default 161)
    --timeout <n>     per-request timeout in seconds  (default 2)
    --retries <n>     retries on timeout              (default 1)
  SNMPv3:
    --user <s>  --level <noAuthNoPriv|authNoPriv|authPriv>
    --auth-proto <MD5|SHA|SHA256|...>  --auth-pass <s>
    --priv-proto <DES|AES|AES256|...>  --priv-pass <s>  --context <s>

EXAMPLES
    snmpdigger discover 192.168.1.0/24
    snmpdigger discover --local --communities public,private
    snmpdigger discover --asn AS13335 --json
    snmpdigger walk 10.0.0.1 1.3.6.1.2.1.2.2
    snmpdigger monitor 10.0.0.1 1.3.6.1.2.1.2.2.1.10.2 --delta --interval 2
    snmpdigger identify 10.0.0.1 --snmp v3 --user monitor --auth-pass s3cret --priv-pass s3cret
`)
}
