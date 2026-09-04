package snmp

import (
	"context"
	"net"
	"testing"
	"time"
)

func mustIP(s string) net.IP       { return net.ParseIP(s) }
func contextTODO() context.Context { return context.TODO() }

func TestOIDLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.3.6.1.2.1.2.2.1.10.2", "1.3.6.1.2.1.2.2.1.10.10", true}, // numeric, not lexical
		{"1.3.6.1.2.1.1.3.0", "1.3.6.1.2.1.1.10.0", true},
		{"1.3.6.1.2.1.1", "1.3.6.1.2.1.1.1", true},
		{"1.3.6.1.4.1", "1.3.6.1.2.1", false},
	}
	for _, c := range cases {
		if got := OIDLess(c.a, c.b); got != c.want {
			t.Errorf("OIDLess(%q,%q)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestDemoSourceWalkGet(t *testing.T) {
	d := NewDemo()
	if err := d.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	vars, err := d.Walk(OIDmib2)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(vars) < 20 {
		t.Fatalf("demo walk of mib-2 returned only %d vars", len(vars))
	}
	// system group must be present and typed
	got, err := d.Get([]string{OIDsysDescr, OIDsysObjectID, OIDsysUpTime})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("get returned %d vars, want 3", len(got))
	}
	if got[0].Kind != KindString || got[0].Str == "" {
		t.Errorf("sysDescr = %+v, want non-empty string", got[0])
	}
	if got[2].Kind != KindTimeTicks || got[2].Num <= 0 {
		t.Errorf("sysUpTime = %+v, want positive TimeTicks", got[2])
	}
}

func TestDemoCountersAdvance(t *testing.T) {
	d := NewDemo()
	oid := "1.3.6.1.2.1.2.2.1.10.2" // eth0 ifInOctets, a synthetic counter
	a, _ := d.Get([]string{oid})
	time.Sleep(30 * time.Millisecond)
	b, _ := d.Get([]string{oid})
	if len(a) != 1 || len(b) != 1 {
		t.Fatal("counter get failed")
	}
	if b[0].Num < a[0].Num {
		t.Errorf("counter went backwards: %v -> %v", a[0].Num, b[0].Num)
	}
}

func TestExpandCIDR(t *testing.T) {
	hosts, err := expandCIDR("192.168.1.0/30")
	if err != nil {
		t.Fatalf("expandCIDR: %v", err)
	}
	// /30 => 4 addrs, minus network + broadcast => 2 usable
	if len(hosts) != 2 || hosts[0] != "192.168.1.1" || hosts[1] != "192.168.1.2" {
		t.Fatalf("got %v, want [192.168.1.1 192.168.1.2]", hosts)
	}
	if _, err := expandCIDR("10.0.0.0/8"); err == nil {
		t.Error("expected /8 to be rejected as too large")
	}
	one, err := expandCIDR("10.1.2.3")
	if err != nil || len(one) != 1 || one[0] != "10.1.2.3" {
		t.Fatalf("bare IP: got %v err %v", one, err)
	}
}

func TestScanCIDRNoResponders(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// 192.0.2.0/29 is TEST-NET-1 - guaranteed no SNMP here
	found, err := ScanCIDR(ctx, ScanOptions{
		CIDR: "192.0.2.0/29", Communities: []string{"public"},
		Timeout: 200 * time.Millisecond, Concurrency: 8,
	}, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("expected no responders in TEST-NET-1, got %d", len(found))
	}
}

func TestGuessRole(t *testing.T) {
	cases := map[string]string{
		"Cisco IOS Software, C2960 Software": "Cisco network device (router/switch)",
		"RouterOS RB750":                     "MikroTik RouterOS device",
		"FortiGate-60F v7.2.5":               "Firewall / security appliance",
		"Linux fw01 6.1.0-37-amd64":          "Linux host / server",
	}
	for descr, want := range cases {
		if got := guessRole(descr, "", 0); got != want {
			t.Errorf("guessRole(%q)=%q want %q", descr, got, want)
		}
	}
}

func TestCIDRHostCount(t *testing.T) {
	cases := map[string]int64{
		"192.168.1.0/24": 254,
		"10.0.0.0/30":    2,
		"10.0.0.0/31":    2,
		"10.0.0.5/32":    1,
		"10.1.2.3":       1,
		"172.16.0.0/16":  65534,
	}
	for cidr, want := range cases {
		got, err := cidrHostCount(cidr)
		if err != nil {
			t.Fatalf("cidrHostCount(%s): %v", cidr, err)
		}
		if got != want {
			t.Errorf("cidrHostCount(%s)=%d want %d", cidr, got, want)
		}
	}
}

func TestWalkCIDRStreaming(t *testing.T) {
	var got []string
	if err := walkCIDR("192.168.5.0/29", func(ip string) bool { got = append(got, ip); return true }); err != nil {
		t.Fatal(err)
	}
	// /29 => 8 addrs minus network+broadcast => 6
	if len(got) != 6 || got[0] != "192.168.5.1" || got[5] != "192.168.5.6" {
		t.Fatalf("got %v", got)
	}
	// early stop
	n := 0
	_ = walkCIDR("10.9.0.0/24", func(string) bool { n++; return n < 5 })
	if n != 5 {
		t.Fatalf("early-stop walked %d, want 5", n)
	}
}

func TestIsPrivateV4(t *testing.T) {
	priv := []string{"10.1.2.3", "172.16.0.1", "172.31.255.1", "192.168.0.1", "100.64.0.1"}
	pub := []string{"8.8.8.8", "1.1.1.1", "172.15.0.1", "172.32.0.1", "192.169.0.1"}
	for _, s := range priv {
		if !isPrivateV4(mustIP(s)) {
			t.Errorf("%s should be private", s)
		}
	}
	for _, s := range pub {
		if isPrivateV4(mustIP(s)) {
			t.Errorf("%s should be public", s)
		}
	}
}

func TestResolveASNParseErrors(t *testing.T) {
	for _, bad := range []string{"", "banana", "AS", "AS12x"} {
		if _, err := ResolveASN(contextTODO(), bad); err == nil {
			t.Errorf("ResolveASN(%q) should error", bad)
		}
	}
}

func TestLocalScanTargetsValid(t *testing.T) {
	for _, cidr := range LocalScanTargets() {
		if _, err := cidrHostCount(cidr); err != nil {
			t.Errorf("LocalScanTargets produced invalid %q: %v", cidr, err)
		}
	}
}

func TestBuildProfileDemo(t *testing.T) {
	d := NewDemo()
	sys, err := Discover(d)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	p := BuildProfile(d, sys)
	if p.Name == "" {
		t.Error("profile missing sysName")
	}
	if !p.IPForwarding {
		t.Error("demo sets ipForwarding=1")
	}
	if len(p.Addrs) != 3 {
		t.Errorf("want 3 ipAddr rows, got %d: %+v", len(p.Addrs), p.Addrs)
	}
	if len(p.Routes) < 3 || p.DefaultGW != "10.0.1.1" {
		t.Errorf("routes=%d defaultGW=%q", len(p.Routes), p.DefaultGW)
	}
	if p.IfTotal != 4 {
		t.Errorf("want 4 interfaces, got %d", p.IfTotal)
	}
	var rpi bool
	for _, a := range p.ARP {
		if a.Vendor == "Raspberry Pi" {
			rpi = true
		}
	}
	if !rpi {
		t.Errorf("expected an OUI-resolved ARP vendor, got %+v", p.ARP)
	}
	if len(p.Storage) != 3 {
		t.Errorf("want 3 storage rows, got %d", len(p.Storage))
	}
}

func TestParseTable(t *testing.T) {
	vs := []Var{
		{OID: "1.3.6.1.2.1.4.20.1.1.10.0.0.1", Str: "10.0.0.1", Kind: KindIPAddress},
		{OID: "1.3.6.1.2.1.4.20.1.2.10.0.0.1", Num: 2, Kind: KindInteger},
		{OID: "1.3.6.1.2.1.4.20.1.3.10.0.0.1", Str: "255.255.255.0", Kind: KindIPAddress},
		{OID: "1.3.6.1.2.1.99.0.0", Str: "ignore me"},
	}
	tbl := parseTable(vs, "1.3.6.1.2.1.4.20.1")
	row := tbl["10.0.0.1"]
	if row == nil || row["1"].Str != "10.0.0.1" || row["2"].Num != 2 || row["3"].Str != "255.255.255.0" {
		t.Fatalf("parseTable = %+v", tbl)
	}
}
