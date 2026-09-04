package tui

import (
	"net"
	"testing"
	"time"

	g "github.com/gosnmp/gosnmp"
)

// TestTrapListenerRoundTrip starts the listener, sends a v2c trap to it and
// checks that buildTrapRecord decoded the identity + varbinds.
func TestTrapListenerRoundTrip(t *testing.T) {
	const addr = "127.0.0.1:16211"

	tl := g.NewTrapListener()
	recs := make(chan trapRecord, 4)
	tl.OnNewTrap = func(s *g.SnmpPacket, u *net.UDPAddr) {
		recs <- buildTrapRecord(s, u)
	}
	ready := tl.Listening()
	errCh := make(chan error, 1)
	go func() { errCh <- tl.Listen(addr) }()
	defer tl.Close()

	select {
	case <-ready:
	case err := <-errCh:
		t.Fatalf("listener failed to start: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not become ready")
	}

	client := &g.GoSNMP{
		Target: "127.0.0.1", Port: 16211, Version: g.Version2c,
		Community: "public", Timeout: 2 * time.Second, Retries: 0,
	}
	if err := client.Connect(); err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer client.Conn.Close()

	trap := g.SnmpTrap{
		Variables: []g.SnmpPDU{
			{Name: "1.3.6.1.6.3.1.1.4.1.0", Type: g.ObjectIdentifier, Value: "1.3.6.1.6.3.1.1.5.3"}, // linkDown
			{Name: "1.3.6.1.2.1.2.2.1.1.7", Type: g.Integer, Value: 7},
			{Name: "1.3.6.1.2.1.2.2.1.2.7", Type: g.OctetString, Value: "eth7"},
		},
	}
	if _, err := client.SendTrap(trap); err != nil {
		t.Fatalf("send trap: %v", err)
	}

	select {
	case r := <-recs:
		if r.version != "v2c" {
			t.Errorf("version = %q, want v2c", r.version)
		}
		if r.trapOID != "1.3.6.1.6.3.1.1.5.3" {
			t.Errorf("trapOID = %q, want linkDown OID", r.trapOID)
		}
		if len(r.vars) != 2 {
			t.Fatalf("got %d varbinds, want 2: %+v", len(r.vars), r.vars)
		}
		if r.vars[1].val != "eth7" {
			t.Errorf("varbind[1].val = %q, want eth7", r.vars[1].val)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no trap received")
	}
}
