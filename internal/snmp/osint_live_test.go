package snmp

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestOSINTLive hits RIPEstat + DNS; run it with SNMPDIGGER_LIVE=1.
func TestOSINTLive(t *testing.T) {
	if os.Getenv("SNMPDIGGER_LIVE") == "" {
		t.Skip("set SNMPDIGGER_LIVE=1 to run network lookups")
	}
	ctx, c := context.WithTimeout(context.Background(), 20*time.Second)
	defer c()

	o := EnrichOSINT(ctx, "8.8.8.8")
	t.Logf("8.8.8.8 -> ASN=%s holder=%q prefix=%s RIR=%s geo=%s/%s PTR=%q err=%q",
		o.ASN, o.ASNHolder, o.Prefix, o.RIR, o.GeoCountry, o.GeoCity, o.PTR, o.Err)
	if o.Private {
		t.Error("8.8.8.8 flagged private")
	}
	if o.ASN == "" {
		t.Error("8.8.8.8 ASN not resolved")
	}
	if !EnrichOSINT(ctx, "10.0.1.9").Private {
		t.Error("10.0.1.9 should be private")
	}
}

func TestOSINTPrivateNoNetwork(t *testing.T) {
	o := EnrichOSINT(context.Background(), "192.168.1.1")
	if !o.Private {
		t.Fatal("192.168.1.1 should be private")
	}
	if o.ASN != "" || o.Prefix != "" {
		t.Errorf("private IP should not get ASN/prefix: %+v", o)
	}
}

func TestOUIVendor(t *testing.T) {
	cases := map[string]string{
		"52:54:00:12:34:56": "QEMU/KVM",
		"B8:27:EB:aa:bb:cc": "Raspberry Pi",
		"4c:5e:0c:11:22:33": "MikroTik",
		"00:0c:29:44:55:66": "VMware",
		"de:ad:be:ef:00:01": "",
	}
	for mac, want := range cases {
		if got := OUIVendor(mac); got != want {
			t.Errorf("OUIVendor(%s)=%q want %q", mac, got, want)
		}
	}
}
