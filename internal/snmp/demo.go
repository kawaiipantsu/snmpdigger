package snmp

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"strings"
	"time"
)

// Demo is a fully synthetic Source. It fabricates a plausible Linux-ish agent
// with a live IF-MIB, host-resources and UCD-SNMP style counters so the whole
// TUI can be exercised without a real device (`snmpdigger --demo`).
type Demo struct {
	start time.Time
	nodes []demoNode
	index map[string]*demoNode
	rng   *rand.Rand
}

type demoNode struct {
	oid  string
	kind Kind
	// val yields the value at time t; exactly one of num/str is meaningful per kind.
	val func(d *Demo, t time.Time) (num float64, str string)
}

// NewDemo constructs the synthetic agent.
func NewDemo() *Demo {
	d := &Demo{
		start: time.Now().Add(-72 * time.Hour), // pretend it has been up for 3 days
		index: map[string]*demoNode{},
		rng:   rand.New(rand.NewSource(0xC0FFEE)),
	}
	d.build()
	return d
}

func constStr(s string) func(*Demo, time.Time) (float64, string) {
	return func(*Demo, time.Time) (float64, string) { return 0, s }
}
func constNum(n float64) func(*Demo, time.Time) (float64, string) {
	return func(*Demo, time.Time) (float64, string) { return n, "" }
}

func (d *Demo) add(oid string, k Kind, f func(*Demo, time.Time) (float64, string)) {
	d.nodes = append(d.nodes, demoNode{oid: oid, kind: k, val: f})
}

func (d *Demo) build() {
	up := func(_ *Demo, t time.Time) (float64, string) {
		return math.Floor(t.Sub(d.start).Seconds() * 100), ""
	}

	// --- system group (SNMPv2-MIB) ---
	d.add("1.3.6.1.2.1.1.1.0", KindString, constStr("SNMPDigger Demo Agent - Linux forge 6.1.0-37-amd64 #1 SMP x86_64 GNU/Linux"))
	d.add("1.3.6.1.2.1.1.2.0", KindOID, constStr("1.3.6.1.4.1.8072.3.2.10"))
	d.add("1.3.6.1.2.1.1.3.0", KindTimeTicks, up)
	d.add("1.3.6.1.2.1.1.4.0", KindString, constStr("noc@thugs.red"))
	d.add("1.3.6.1.2.1.1.5.0", KindString, constStr("forge.lab.thugs.red"))
	d.add("1.3.6.1.2.1.1.6.0", KindString, constStr("Rack B-07, THUGS Datacenter, Copenhagen DK"))
	d.add("1.3.6.1.2.1.1.7.0", KindInteger, constNum(76)) // services bitmask: L3+L4+app

	// --- IF-MIB ---
	ifaces := []struct {
		descr   string
		typ     int
		speed   float64
		mac     string
		inRate  float64 // bytes/sec baseline
		outRate float64
	}{
		{"lo", 24, 10_000_000, "00:00:00:00:00:00", 1500, 1500},
		{"eth0", 6, 1_000_000_000, "52:54:00:a1:b2:c3", 384_000, 128_000},
		{"eth1", 6, 1_000_000_000, "52:54:00:a1:b2:c4", 12_000, 40_000},
		{"wg0", 131, 0, "00:00:00:00:00:00", 6_500, 9_800},
	}
	d.add("1.3.6.1.2.1.2.1.0", KindInteger, constNum(float64(len(ifaces))))
	for i, f := range ifaces {
		idx := i + 1
		col := func(c int) string { return fmt.Sprintf("1.3.6.1.2.1.2.2.1.%d.%d", c, idx) }
		f := f
		d.add(col(1), KindInteger, constNum(float64(idx)))
		d.add(col(2), KindString, constStr(f.descr))
		d.add(col(3), KindInteger, constNum(float64(f.typ)))
		d.add(col(4), KindInteger, constNum(1500))
		d.add(col(5), KindGauge, constNum(f.speed))
		d.add(col(6), KindString, constStr(f.mac))
		d.add(col(7), KindInteger, constNum(1)) // adminStatus up
		d.add(col(8), KindInteger, constNum(1)) // operStatus up
		d.add(col(10), KindCounter, d.counter(f.descr+"-in", f.inRate, 0.35))
		d.add(col(11), KindCounter, d.counter(f.descr+"-inpkts", f.inRate/512, 0.35))
		d.add(col(14), KindCounter, d.counter(f.descr+"-inerr", 0.002, 3))
		d.add(col(16), KindCounter, d.counter(f.descr+"-out", f.outRate, 0.35))
		d.add(col(17), KindCounter, d.counter(f.descr+"-outpkts", f.outRate/512, 0.35))
		d.add(col(20), KindCounter, d.counter(f.descr+"-outerr", 0.001, 3))
	}

	// --- HOST-RESOURCES-MIB ---
	d.add("1.3.6.1.2.1.25.1.1.0", KindTimeTicks, up)
	d.add("1.3.6.1.2.1.25.1.5.0", KindGauge, d.wobble("users", 4, 2, 900))
	d.add("1.3.6.1.2.1.25.1.6.0", KindGauge, d.wobble("procs", 240, 30, 120))
	d.add("1.3.6.1.2.1.25.2.2.0", KindInteger, constNum(16_384_000)) // KB

	// --- UCD-SNMP-MIB: load + cpu + memory ---
	d.add("1.3.6.1.4.1.2021.10.1.3.1", KindString, d.loadStr("la1", 0.9, 0.4, 137))
	d.add("1.3.6.1.4.1.2021.10.1.3.2", KindString, d.loadStr("la5", 0.8, 0.25, 373))
	d.add("1.3.6.1.4.1.2021.10.1.3.3", KindString, d.loadStr("la15", 0.7, 0.15, 971))
	d.add("1.3.6.1.4.1.2021.11.9.0", KindInteger, d.wobble("cpuUser", 22, 12, 60))
	d.add("1.3.6.1.4.1.2021.11.10.0", KindInteger, d.wobble("cpuSystem", 8, 4, 45))
	d.add("1.3.6.1.4.1.2021.11.11.0", KindInteger, d.cpuIdle())
	d.add("1.3.6.1.4.1.2021.4.5.0", KindInteger, constNum(16_384_000))
	d.add("1.3.6.1.4.1.2021.4.6.0", KindInteger, d.wobble("memAvail", 6_100_000, 900_000, 210))

	// --- synthetic environmental sensors (LM-SENSORS-ish namespace) ---
	d.add("1.3.6.1.4.1.2021.13.16.2.1.3.1", KindGauge, d.sine("tempCpu", 48000, 9000, 180))   // milli-C
	d.add("1.3.6.1.4.1.2021.13.16.2.1.3.2", KindGauge, d.sine("tempBoard", 33000, 4000, 300)) // milli-C
	d.add("1.3.6.1.4.1.2021.13.16.3.1.3.1", KindGauge, d.wobble("fan1", 4200, 350, 40))
	d.add("1.3.6.1.4.1.2021.13.16.3.1.3.2", KindGauge, d.wobble("fan2", 3900, 300, 55))
	d.add("1.3.6.1.4.1.2021.13.16.4.1.3.1", KindGauge, d.sine("volt12", 12100, 60, 90)) // milli-V

	// --- SNMP engine stats ---
	d.add("1.3.6.1.2.1.11.1.0", KindCounter, d.counter("snmpInPkts", 3.2, 1.5))
	d.add("1.3.6.1.2.1.11.2.0", KindCounter, d.counter("snmpOutPkts", 3.2, 1.5))
	d.add("1.3.6.1.2.1.11.15.0", KindCounter, d.counter("snmpInGetRequests", 2.1, 1.5))

	for i := range d.nodes {
		d.index[d.nodes[i].oid] = &d.nodes[i]
	}
}

// counter returns a monotonic counter growing at ratePerSec with +/- jitter%.
func (d *Demo) counter(seed string, ratePerSec, jitter float64) func(*Demo, time.Time) (float64, string) {
	phase := seedFloat(seed)
	return func(_ *Demo, t time.Time) (float64, string) {
		elapsed := t.Sub(d.start).Seconds()
		// A strictly increasing base plus a bounded ripple. The ripple's slope
		// (|d/dt| = ratePerSec * 2.5/23 ≈ 0.11*ratePerSec) never exceeds the
		// base slope, so the counter is monotonic between samples - as a real
		// SNMP Counter32/64 must be until it wraps. (jitter is retained in the
		// signature for call-site readability but not used in this model.)
		ripple := ratePerSec * 2.5 * (1 + math.Sin(elapsed/23+phase*6.28))
		v := ratePerSec*elapsed + ripple
		return math.Mod(math.Floor(v), 1.8446744073709552e19), ""
	}
}

// wobble returns a gauge doing a bounded random-ish walk around base.
func (d *Demo) wobble(seed string, base, amp, periodSec float64) func(*Demo, time.Time) (float64, string) {
	phase := seedFloat(seed)
	return func(_ *Demo, t time.Time) (float64, string) {
		x := t.Sub(d.start).Seconds()
		v := base +
			amp*math.Sin(x/periodSec+phase*6.28) +
			amp*0.35*math.Sin(x/(periodSec/3.7)+phase) +
			amp*0.15*math.Sin(x/(periodSec/11.3)+phase*2)
		if v < 0 {
			v = 0
		}
		return math.Floor(v), ""
	}
}

// sine is a clean sinusoid, handy for volts/temps.
func (d *Demo) sine(seed string, base, amp, periodSec float64) func(*Demo, time.Time) (float64, string) {
	phase := seedFloat(seed)
	return func(_ *Demo, t time.Time) (float64, string) {
		x := t.Sub(d.start).Seconds()
		return math.Round(base + amp*math.Sin(x/periodSec+phase*6.28)), ""
	}
}

func (d *Demo) cpuIdle() func(*Demo, time.Time) (float64, string) {
	u := d.wobble("cpuUser", 22, 12, 60)
	s := d.wobble("cpuSystem", 8, 4, 45)
	return func(dd *Demo, t time.Time) (float64, string) {
		un, _ := u(dd, t)
		sn, _ := s(dd, t)
		idle := 100 - un - sn
		if idle < 0 {
			idle = 0
		}
		return math.Floor(idle), ""
	}
}

func (d *Demo) loadStr(seed string, base, amp, periodSec float64) func(*Demo, time.Time) (float64, string) {
	w := d.wobble(seed, base*100, amp*100, periodSec)
	return func(dd *Demo, t time.Time) (float64, string) {
		n, _ := w(dd, t)
		return 0, fmt.Sprintf("%.2f", n/100)
	}
}

func seedFloat(s string) float64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return float64(h.Sum32()%997) / 997
}

// --- Source implementation ---

func (d *Demo) Connect() error { return nil }
func (d *Demo) Close() error   { return nil }

func (d *Demo) Get(oids []string) ([]Var, error) {
	now := time.Now()
	out := make([]Var, 0, len(oids))
	for _, raw := range oids {
		oid := strings.TrimPrefix(strings.TrimSpace(raw), ".")
		n, ok := d.index[oid]
		if !ok {
			out = append(out, Var{OID: oid, Kind: KindNoSuchObject, Str: "no such object", Time: now})
			continue
		}
		out = append(out, d.sample(n, now))
	}
	return out, nil
}

func (d *Demo) Walk(root string) ([]Var, error) {
	now := time.Now()
	root = strings.TrimPrefix(strings.TrimSpace(root), ".")
	prefix := root + "."
	out := make([]Var, 0, 64)
	for i := range d.nodes {
		n := &d.nodes[i]
		if n.oid == root || strings.HasPrefix(n.oid, prefix) {
			out = append(out, d.sample(n, now))
		}
	}
	SortVars(out)
	return out, nil
}

func (d *Demo) sample(n *demoNode, now time.Time) Var {
	num, str := n.val(d, now)
	v := Var{OID: n.oid, Kind: n.kind, Time: now, Num: num, Str: str}
	switch n.kind {
	case KindTimeTicks:
		v.Str = humanTicks(num)
		v.Raw = uint32(num)
	case KindString, KindOID, KindIPAddress:
		v.Raw = str
	default:
		v.Raw = int64(num)
	}
	return v
}

func (d *Demo) Target() string { return "demo:161 v2c" }
func (d *Demo) Describe() string {
	return "demo://synthetic-agent  proto=v2c  community=public  (offline demo mode)"
}
