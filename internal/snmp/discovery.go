package snmp

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kawaiipantsu/snmpdigger/internal/config"
)

// ScanOptions configures a sweep for SNMP agents across one or more ranges.
type ScanOptions struct {
	CIDR        string   // single range (kept for convenience)
	Targets     []string // one or more CIDRs / IPs; takes precedence over CIDR
	Port        uint16
	Version     string            // v1 | v2c | v3
	Communities []string          // v1/v2c: probe each until one answers
	Base        config.Connection // v3: security params (host is overridden per target)
	Timeout     time.Duration
	Retries     int
	Concurrency int
	MaxHosts    int // hard cap on addresses probed (0 = default 262144)
}

// DefaultMaxHosts caps how many addresses a single scan will probe.
const DefaultMaxHosts = 1 << 18

// Found is a discovered agent plus its identity.
type Found struct {
	IP          string
	Port        uint16
	Version     string
	Community   string // which community answered (v1/v2c)
	SysName     string
	SysDescr    string
	SysObjectID string
	SysContact  string
	SysLocation string
	Vendor      string
	Role        string
	Uptime      time.Duration
	RTT         time.Duration
}

// Short is a one-word-ish device label for tables.
func (f Found) Short() string {
	if f.Vendor != "" && !strings.HasPrefix(f.Vendor, "enterprise ") {
		return f.Vendor
	}
	if f.SysName != "" {
		return f.SysName
	}
	return "SNMP device"
}

// ScanCIDR sweeps opts.Targets (or opts.CIDR) and returns responders. progress,
// if non-nil, is called as (completed, total) after every host probe. Addresses
// are streamed into the worker pool so even an ASN's worth of prefixes stays
// bounded in memory; probing stops after opts.MaxHosts addresses.
func ScanCIDR(ctx context.Context, opts ScanOptions, progress func(done, total int)) ([]Found, error) {
	targets := opts.Targets
	if len(targets) == 0 && opts.CIDR != "" {
		targets = []string{opts.CIDR}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no scan targets given")
	}
	if opts.Port == 0 {
		opts.Port = 161
	}
	if opts.Version == "" {
		opts.Version = "v2c"
	}
	if len(opts.Communities) == 0 {
		opts.Communities = []string{"public"}
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 800 * time.Millisecond
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 64
	}
	if opts.MaxHosts <= 0 {
		opts.MaxHosts = DefaultMaxHosts
	}

	// cheap arithmetic total (no materialisation), capped at MaxHosts
	var grand int64
	for _, t := range targets {
		n, err := cidrHostCount(t)
		if err != nil {
			return nil, err
		}
		grand += n
	}
	total := int(grand)
	if grand > int64(opts.MaxHosts) {
		total = opts.MaxHosts
	}
	if total == 0 {
		return nil, fmt.Errorf("targets contain no usable host addresses")
	}
	if opts.Concurrency > total {
		opts.Concurrency = total
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []Found
		done    int
	)
	jobs := make(chan string, opts.Concurrency)

	worker := func() {
		defer wg.Done()
		for ip := range jobs {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if f, ok := probe(ctx, ip, opts); ok {
				mu.Lock()
				results = append(results, f)
				mu.Unlock()
			}
			mu.Lock()
			done++
			d := done
			mu.Unlock()
			if progress != nil {
				progress(d, total)
			}
		}
	}

	wg.Add(opts.Concurrency)
	for i := 0; i < opts.Concurrency; i++ {
		go worker()
	}

	// producer: stream host IPs from every target, honouring ctx + MaxHosts
	sent := 0
	seen := make(map[string]struct{}, 4096)
producer:
	for _, t := range targets {
		stop := false
		_ = walkCIDR(t, func(ip string) bool {
			select {
			case <-ctx.Done():
				stop = true
				return false
			default:
			}
			if _, dup := seen[ip]; dup {
				return true
			}
			seen[ip] = struct{}{}
			select {
			case <-ctx.Done():
				stop = true
				return false
			case jobs <- ip:
				sent++
				if sent >= opts.MaxHosts {
					stop = true
					return false
				}
				return true
			}
		})
		if stop {
			break producer
		}
	}
	close(jobs)
	wg.Wait()

	sort.Slice(results, func(i, j int) bool { return ipLess(results[i].IP, results[j].IP) })
	if err := ctx.Err(); err != nil {
		return results, err
	}
	return results, nil
}

func probe(ctx context.Context, ip string, opts ScanOptions) (Found, bool) {
	comms := opts.Communities
	if strings.EqualFold(opts.Version, "v3") {
		comms = []string{""} // single pass, creds come from opts.Base
	}
	for _, community := range comms {
		select {
		case <-ctx.Done():
			return Found{}, false
		default:
		}
		conn := opts.Base
		conn.Host = ip
		conn.Port = opts.Port
		conn.Version = opts.Version
		if !strings.EqualFold(opts.Version, "v3") {
			conn.Community = community
		}
		poll := config.Poll{
			TimeoutSeconds: int(opts.Timeout.Round(time.Second) / time.Second),
			Retries:        opts.Retries,
			MaxOIDsPerReq:  10,
			MaxRepetitions: 10,
		}
		if poll.TimeoutSeconds < 1 {
			poll.TimeoutSeconds = 1
		}
		live, err := NewLive(conn, poll)
		if err != nil {
			continue
		}
		// tighten timeout below one second where possible
		live.client.Timeout = opts.Timeout
		start := time.Now()
		if err := live.Connect(); err != nil {
			live.Close()
			continue
		}
		vars, err := live.Get([]string{
			OIDsysDescr, OIDsysObjectID, OIDsysUpTime,
			OIDsysContact, OIDsysName, OIDsysLocation,
		})
		rtt := time.Since(start)
		live.Close()
		if err != nil || !hasAnswer(vars) {
			continue
		}

		f := Found{IP: ip, Port: opts.Port, Version: opts.Version, Community: community, RTT: rtt}
		si := SystemInfo{}
		for _, v := range vars {
			switch v.OID {
			case OIDsysDescr:
				f.SysDescr = v.Str
			case OIDsysObjectID:
				f.SysObjectID = strings.TrimPrefix(v.Str, ".")
			case OIDsysUpTime:
				f.Uptime = time.Duration(v.Num) * 10 * time.Millisecond
			case OIDsysContact:
				f.SysContact = v.Str
			case OIDsysName:
				f.SysName = v.Str
			case OIDsysLocation:
				f.SysLocation = v.Str
			}
		}
		si.Descr = f.SysDescr
		si.ObjectID = f.SysObjectID
		si.analyse()
		f.Vendor = si.Vendor
		f.Role = si.Role
		return f, true
	}
	return Found{}, false
}

func hasAnswer(vars []Var) bool {
	for _, v := range vars {
		if v.Kind != KindNoSuchObject && (v.Str != "" || v.Num != 0 || v.Raw != nil) {
			return true
		}
	}
	return false
}

// expandCIDR returns usable host IPs. Network and broadcast addresses are
// dropped for IPv4 prefixes shorter than /31. A bare IP (no mask) yields itself.
func expandCIDR(cidr string) ([]string, error) {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		return nil, fmt.Errorf("empty CIDR / address")
	}
	if !strings.Contains(cidr, "/") {
		if ip := net.ParseIP(cidr); ip != nil {
			return []string{ip.String()}, nil
		}
		return nil, fmt.Errorf("%q is not an IP or CIDR", cidr)
	}
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("only IPv4 CIDR ranges are supported")
	}
	ones, bits := ipnet.Mask.Size()
	if bits-ones > 16 {
		return nil, fmt.Errorf("range /%d is too large (max 65536 hosts)", ones)
	}

	var out []string
	cur := make(net.IP, len(ipnet.IP))
	copy(cur, ipnet.IP)
	for ipnet.Contains(cur) {
		out = append(out, cur.String())
		incIP(cur)
	}
	if len(out) > 2 && bits-ones >= 2 {
		out = out[1 : len(out)-1] // drop network + broadcast
	}
	return out, nil
}

func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

// cidrHostCount returns how many usable host addresses a CIDR (or bare IP) holds,
// without materialising them. Network + broadcast are excluded for IPv4 prefixes
// of /30 or shorter.
func cidrHostCount(cidr string) (int64, error) {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		return 0, fmt.Errorf("empty CIDR / address")
	}
	if !strings.Contains(cidr, "/") {
		if net.ParseIP(cidr) != nil {
			return 1, nil
		}
		return 0, fmt.Errorf("%q is not an IP or CIDR", cidr)
	}
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, err
	}
	if ip.To4() == nil {
		return 0, fmt.Errorf("only IPv4 ranges are supported (%s)", cidr)
	}
	ones, bits := ipnet.Mask.Size()
	host := bits - ones
	if host >= 31 { // guard against overflow / absurd inputs
		return 1 << 30, nil
	}
	n := int64(1) << uint(host)
	if host >= 2 {
		n -= 2 // network + broadcast
	}
	return n, nil
}

// walkCIDR streams every usable host address in a CIDR (or bare IP) to fn.
// Returning false from fn stops the walk early.
func walkCIDR(cidr string, fn func(ip string) bool) error {
	cidr = strings.TrimSpace(cidr)
	if !strings.Contains(cidr, "/") {
		if ip := net.ParseIP(cidr); ip != nil {
			fn(ip.String())
			return nil
		}
		return fmt.Errorf("%q is not an IP or CIDR", cidr)
	}
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	if ip.To4() == nil {
		return fmt.Errorf("only IPv4 ranges are supported (%s)", cidr)
	}
	ones, bits := ipnet.Mask.Size()
	dropEnds := (bits - ones) >= 2

	cur := make(net.IP, len(ipnet.IP))
	copy(cur, ipnet.IP)
	network := make(net.IP, len(cur))
	copy(network, cur)

	first := true
	for ipnet.Contains(cur) {
		next := make(net.IP, len(cur))
		copy(next, cur)
		incIP(next)
		isBroadcast := !ipnet.Contains(next)

		skip := false
		if dropEnds {
			if first || isBroadcast {
				skip = true
			}
		}
		if !skip {
			if !fn(cur.String()) {
				return nil
			}
		}
		first = false
		cur = next
	}
	return nil
}

func ipLess(a, b string) bool {
	ia, ib := net.ParseIP(a).To4(), net.ParseIP(b).To4()
	if ia == nil || ib == nil {
		return a < b
	}
	for i := 0; i < 4; i++ {
		if ia[i] != ib[i] {
			return ia[i] < ib[i]
		}
	}
	return false
}
