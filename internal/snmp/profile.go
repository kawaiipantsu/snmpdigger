package snmp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// --- profile structs ----------------------------------------------------

type IfAddr struct {
	IP      string
	Mask    string
	IfIndex int
}

type IfBrief struct {
	Index int
	Name  string
	Speed float64
	Admin int
	Oper  int
	MAC   string
}

type RouteEntry struct {
	Dest    string
	Mask    string
	NextHop string
	IfIndex int
	Proto   string
	Type    string
}

type ArpEntry struct {
	IP      string
	MAC     string
	IfIndex int
	Vendor  string
}

type LldpNeighbor struct {
	LocalIf    string
	RemSys     string
	RemPort    string
	RemChassis string
}

type StorageEntry struct {
	Descr     string
	SizeBytes int64
	UsedBytes int64
}

// NetworkProfile is the assembled SNMP-side picture of a device.
type NetworkProfile struct {
	FetchedAt time.Time

	// identity
	Name, Descr, ObjectID       string
	Vendor, Role                string
	Model, Serial, HWRev, SWRev string
	Contact, Location           string
	Uptime                      time.Duration
	Layers                      []string
	IPForwarding                bool

	// addressing / interfaces
	Addrs   []IfAddr
	IfTotal int
	IfUp    int
	Ifaces  []IfBrief

	// routing
	Routes         []RouteEntry
	RouteTruncated bool
	DefaultGW      string

	// neighbours
	ARP  []ArpEntry
	LLDP []LldpNeighbor

	// services / os
	TCPEstab  int
	ListenTCP []int
	Processes int
	Users     int
	MemBytes  int64
	Storage   []StorageEntry

	Notes  []string
	Errors []string
}

// BuildProfile walks the tables needed for a network profile. Every table is
// best-effort; failures are recorded in Errors and the rest of the profile is
// still returned.
func BuildProfile(src Source, sys SystemInfo) NetworkProfile {
	p := NetworkProfile{
		FetchedAt: time.Now(),
		Name:      sys.Name,
		Descr:     sys.Descr,
		ObjectID:  sys.ObjectID,
		Vendor:    sys.Vendor,
		Role:      sys.Role,
		Contact:   sys.Contact,
		Location:  sys.Location,
		Uptime:    sys.Uptime,
		Layers:    sys.Layers,
		Notes:     append([]string(nil), sys.Notes...),
	}

	walk := func(root string) map[string]map[string]Var {
		vs, err := src.Walk(root)
		if err != nil {
			p.Errors = append(p.Errors, root+": "+err.Error())
			return nil
		}
		return parseTable(vs, root)
	}
	getNum := func(oid string) (float64, bool) {
		vs, err := src.Get([]string{oid})
		if err != nil || len(vs) == 0 || vs[0].Kind == KindNoSuchObject {
			return 0, false
		}
		return vs[0].Num, true
	}

	// ipForwarding
	if n, ok := getNum("1.3.6.1.2.1.4.1.0"); ok {
		p.IPForwarding = n == 1
	}

	// ENTITY-MIB chassis row (class 3)
	if ent := walk("1.3.6.1.2.1.47.1.1.1.1"); ent != nil {
		for _, row := range ent {
			if row["5"].Num == 3 || (p.Model == "" && row["13"].Str != "") {
				if v := row["13"].Str; v != "" {
					p.Model = v
				}
				if v := row["11"].Str; v != "" {
					p.Serial = v
				}
				if v := row["10"].Str; v != "" {
					p.SWRev = v
				}
				if v := row["9"].Str; v != "" {
					p.HWRev = v
				}
				if row["5"].Num == 3 {
					break
				}
			}
		}
	}

	// ipAddrTable
	if t := walk("1.3.6.1.2.1.4.20.1"); t != nil {
		for _, row := range t {
			a := IfAddr{IP: row["1"].Str, Mask: row["3"].Str, IfIndex: int(row["2"].Num)}
			if a.IP == "" {
				a.IP = row["1"].Display()
			}
			if a.IP != "" {
				p.Addrs = append(p.Addrs, a)
			}
		}
		sort.Slice(p.Addrs, func(i, j int) bool { return ipLess(p.Addrs[i].IP, p.Addrs[j].IP) })
	}

	// interfaces: ifTable + ifName/ifHighSpeed from ifXTable
	ifT := walk("1.3.6.1.2.1.2.2.1")
	ifX := walk("1.3.6.1.2.1.31.1.1.1")
	if ifT != nil {
		idxs := make([]int, 0, len(ifT))
		for k := range ifT {
			if n, err := strconv.Atoi(k); err == nil {
				idxs = append(idxs, n)
			}
		}
		sort.Ints(idxs)
		for _, idx := range idxs {
			row := ifT[strconv.Itoa(idx)]
			b := IfBrief{
				Index: idx,
				Name:  row["2"].Str,
				Speed: row["5"].Num,
				Admin: int(row["7"].Num),
				Oper:  int(row["8"].Num),
				MAC:   row["6"].Str,
			}
			if ifX != nil {
				if xr, ok := ifX[strconv.Itoa(idx)]; ok {
					if xr["1"].Str != "" {
						b.Name = xr["1"].Str
					}
					if hs := xr["15"].Num; hs > 0 {
						b.Speed = hs * 1_000_000
					}
				}
			}
			p.Ifaces = append(p.Ifaces, b)
			p.IfTotal++
			if b.Oper == 1 {
				p.IfUp++
			}
		}
	}

	// routing: prefer ipCidrRouteTable, fall back to ipRouteTable
	const maxRoutes = 500
	addRoute := func(r RouteEntry) {
		if len(p.Routes) >= maxRoutes {
			p.RouteTruncated = true
			return
		}
		p.Routes = append(p.Routes, r)
		if r.Dest == "0.0.0.0" && p.DefaultGW == "" {
			p.DefaultGW = r.NextHop
		}
	}
	if t := walk("1.3.6.1.2.1.4.24.4.1"); len(t) > 0 {
		for _, row := range t {
			addRoute(RouteEntry{
				Dest: row["1"].Str, Mask: row["2"].Str, NextHop: row["5"].Str,
				IfIndex: int(row["6"].Num), Type: routeType(int(row["7"].Num)),
				Proto: routeProto(int(row["8"].Num)),
			})
		}
	} else if t := walk("1.3.6.1.2.1.4.21.1"); len(t) > 0 {
		for _, row := range t {
			addRoute(RouteEntry{
				Dest: row["1"].Str, Mask: row["11"].Str, NextHop: row["7"].Str,
				IfIndex: int(row["2"].Num), Type: routeType(int(row["8"].Num)),
				Proto: routeProto(int(row["9"].Num)),
			})
		}
	}
	sort.Slice(p.Routes, func(i, j int) bool { return ipLess(p.Routes[i].Dest, p.Routes[j].Dest) })

	// ARP / neighbour table
	if t := walk("1.3.6.1.2.1.4.22.1"); t != nil {
		for _, row := range t {
			e := ArpEntry{IP: row["3"].Str, MAC: row["2"].Str, IfIndex: int(row["1"].Num)}
			if e.IP == "" {
				e.IP = row["3"].Display()
			}
			e.Vendor = OUIVendor(e.MAC)
			if e.IP != "" || e.MAC != "" {
				p.ARP = append(p.ARP, e)
			}
		}
		sort.Slice(p.ARP, func(i, j int) bool { return ipLess(p.ARP[i].IP, p.ARP[j].IP) })
	}

	// LLDP remote table
	if t := walk("1.0.8802.1.1.2.1.4.1.1"); t != nil {
		for _, row := range t {
			n := LldpNeighbor{
				RemChassis: row["5"].Display(),
				RemPort:    row["7"].Display(),
				RemSys:     row["9"].Str,
			}
			if n.RemSys == "" && row["10"].Str != "" {
				n.RemSys = row["10"].Str
			}
			if n.RemSys != "" || n.RemChassis != "" {
				p.LLDP = append(p.LLDP, n)
			}
		}
	}

	// services / OS
	if n, ok := getNum("1.3.6.1.2.1.6.9.0"); ok {
		p.TCPEstab = int(n)
	}
	if n, ok := getNum("1.3.6.1.2.1.25.1.6.0"); ok {
		p.Processes = int(n)
	}
	if n, ok := getNum("1.3.6.1.2.1.25.1.5.0"); ok {
		p.Users = int(n)
	}
	if n, ok := getNum("1.3.6.1.2.1.25.2.2.0"); ok {
		p.MemBytes = int64(n) * 1024 // hrMemorySize is in KB
	}
	if t := walk("1.3.6.1.2.1.6.13.1"); t != nil { // tcpConnTable
		for _, row := range t {
			if int(row["1"].Num) == 2 { // listen
				port := int(row["3"].Num)
				if port > 0 {
					p.ListenTCP = append(p.ListenTCP, port)
				}
			}
		}
		p.ListenTCP = uniqInts(p.ListenTCP)
	}
	if t := walk("1.3.6.1.2.1.25.2.3.1"); t != nil { // hrStorageTable
		for _, row := range t {
			unit := row["4"].Num
			if unit <= 0 {
				unit = 1
			}
			s := StorageEntry{
				Descr:     row["3"].Str,
				SizeBytes: int64(row["5"].Num * unit),
				UsedBytes: int64(row["6"].Num * unit),
			}
			if s.Descr != "" && s.SizeBytes > 0 {
				p.Storage = append(p.Storage, s)
			}
		}
	}

	if p.IPForwarding && len(p.Routes) > 0 {
		p.Notes = append(p.Notes, fmt.Sprintf("IP forwarding enabled with %d routes - acting as a router", len(p.Routes)))
	}
	if len(p.ListenTCP) > 0 {
		p.Notes = append(p.Notes, fmt.Sprintf("%d listening TCP port(s) visible via SNMP", len(p.ListenTCP)))
	}
	return p
}

// --- helpers ----------------------------------------------------------

func parseTable(vars []Var, colBase string) map[string]map[string]Var {
	base := strings.TrimPrefix(colBase, ".") + "."
	out := map[string]map[string]Var{}
	for _, v := range vars {
		oid := strings.TrimPrefix(v.OID, ".")
		if !strings.HasPrefix(oid, base) {
			continue
		}
		rest := oid[len(base):]
		dot := strings.IndexByte(rest, '.')
		if dot < 0 {
			continue
		}
		col, inst := rest[:dot], rest[dot+1:]
		if out[inst] == nil {
			out[inst] = map[string]Var{}
		}
		out[inst][col] = v
	}
	return out
}

func routeProto(n int) string {
	switch n {
	case 2:
		return "local"
	case 3:
		return "static"
	case 4:
		return "icmp"
	case 8:
		return "rip"
	case 9:
		return "is-is"
	case 13:
		return "ospf"
	case 14:
		return "bgp"
	case 16:
		return "eigrp"
	default:
		return "proto-" + strconv.Itoa(n)
	}
}

func routeType(n int) string {
	switch n {
	case 3:
		return "direct"
	case 4:
		return "indirect"
	case 2:
		return "invalid"
	default:
		return "other"
	}
}

func uniqInts(in []int) []int {
	sort.Ints(in)
	out := in[:0]
	var last int = -1
	for _, x := range in {
		if x != last {
			out = append(out, x)
			last = x
		}
	}
	return out
}
