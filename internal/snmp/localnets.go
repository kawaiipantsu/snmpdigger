package snmp

import (
	"fmt"
	"net"
	"sort"
)

// commonLANSeeds are /24s people very commonly run even when this host is not
// currently attached to them - handy for a "scan my usual networks" sweep.
var commonLANSeeds = []string{
	"192.168.0.0/24", "192.168.1.0/24", "192.168.2.0/24", "192.168.10.0/24",
	"192.168.20.0/24", "192.168.50.0/24", "192.168.100.0/24", "192.168.178.0/24",
	"10.0.0.0/24", "10.0.1.0/24", "10.1.1.0/24", "10.10.10.0/24",
	"172.16.0.0/24", "172.16.1.0/24",
}

// LocalInterfaceNetworks returns the private IPv4 networks this host is directly
// attached to (one CIDR per interface address). Networks shorter than /22 are
// narrowed to a /24 around the host address so the sweep stays quick.
func LocalInterfaceNetworks() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip4 := ipnet.IP.To4()
		if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
			continue
		}
		if !isPrivateV4(ip4) {
			continue
		}
		ones, _ := ipnet.Mask.Size()
		var cidr string
		if ones >= 22 {
			net4 := ip4.Mask(ipnet.Mask)
			cidr = fmt.Sprintf("%s/%d", net4.String(), ones)
		} else {
			cidr = fmt.Sprintf("%d.%d.%d.0/24", ip4[0], ip4[1], ip4[2])
		}
		if _, dup := seen[cidr]; dup {
			continue
		}
		seen[cidr] = struct{}{}
		out = append(out, cidr)
	}
	return out
}

// LocalScanTargets is the target list for the Discovery "LOCAL" preset: this
// host's attached private networks plus the common LAN /24 seeds, de-duplicated.
func LocalScanTargets() []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(c string) {
		if _, dup := seen[c]; dup {
			return
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	for _, c := range LocalInterfaceNetworks() {
		add(c)
	}
	for _, c := range commonLANSeeds {
		add(c)
	}
	sort.Slice(out, func(i, j int) bool { return OIDLess(out[i], out[j]) })
	return out
}

func isPrivateV4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	switch {
	case ip[0] == 10:
		return true
	case ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31:
		return true
	case ip[0] == 192 && ip[1] == 168:
		return true
	case ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127: // CGNAT (RFC 6598)
		return true
	default:
		return false
	}
}

// SummarizeTargets renders a short human description of a target list.
func SummarizeTargets(targets []string) string {
	if len(targets) == 0 {
		return "no targets"
	}
	var hosts int64
	for _, t := range targets {
		if n, err := cidrHostCount(t); err == nil {
			hosts += n
		}
	}
	if len(targets) == 1 {
		return fmt.Sprintf("%s (%s addresses)", targets[0], humanCount(hosts))
	}
	return fmt.Sprintf("%d ranges · %s addresses", len(targets), humanCount(hosts))
}

func humanCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	default:
		return fmt.Sprintf("%d", n)
	}
}
