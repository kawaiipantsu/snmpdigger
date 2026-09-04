package snmp

import (
	"context"
	"testing"
	"time"
)

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
