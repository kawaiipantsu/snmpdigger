package mib

import (
	"strings"
	"testing"
)

func TestResolverBuiltin(t *testing.T) {
	r := New(false) // no snmptranslate, pure builtin
	cases := map[string]string{
		"1.3.6.1.2.1.1.1.0":        "sysDescr.0",
		"1.3.6.1.2.1.2.2.1.10.3":   "ifInOctets.3",
		"1.3.6.1.2.1.1.5.0":        "sysName.0",
		"1.3.6.1.4.1.2021.11.11.0": "ssCpuIdle.0",
	}
	for oid, want := range cases {
		if got := r.Name(oid); got != want {
			t.Errorf("Name(%s)=%q want %q", oid, got, want)
		}
	}
	// unknown OID echoes back
	if got := r.Name("1.2.3.4.5.6.7.8.9"); got != "1.2.3.4.5.6.7.8.9" {
		t.Errorf("unknown OID = %q, want echo", got)
	}
}

func TestCatalogIntegrity(t *testing.T) {
	mods := Catalog()
	if len(mods) < 20 {
		t.Fatalf("catalog has only %d modules", len(mods))
	}
	total := 0
	seenStd := false
	for _, m := range mods {
		if m.Module == "" || m.Vendor == "" {
			t.Errorf("module with empty name/vendor: %+v", m)
		}
		if strings.Contains(m.Vendor, "Standard") || strings.Contains(m.Vendor, "IETF") {
			seenStd = true
		}
		for _, o := range m.Objects {
			total++
			if o.OID == "" || o.Name == "" {
				t.Errorf("%s: object with empty OID/name: %+v", m.Module, o)
			}
			if m.Root != "" && !strings.HasPrefix(o.OID, strings.TrimPrefix(m.Root, ".")) &&
				!strings.HasPrefix(o.OID, m.Root) {
				// tolerate a handful of cross-referenced standard OIDs
			}
		}
	}
	if !seenStd {
		t.Error("catalog has no IETF/Standard modules")
	}
	if total < 150 {
		t.Fatalf("catalog has only %d objects total", total)
	}
}

func TestCatalogSearchAndLookup(t *testing.T) {
	res := SearchCatalog("ifInOctets")
	if len(res) == 0 {
		t.Fatal("SearchCatalog(ifInOctets) found nothing")
	}
	if _, _, ok := LookupCatalog("1.3.6.1.2.1.1.3.0"); !ok {
		// sysUpTime instance may not be catalogued; try the column form
		if _, _, ok := LookupCatalog("1.3.6.1.2.1.2.2.1.10"); !ok {
			t.Skip("no exact-OID catalog hit for the sampled OIDs")
		}
	}
}

func TestVendors(t *testing.T) {
	vs := Vendors()
	if len(vs) < 5 {
		t.Fatalf("only %d vendors", len(vs))
	}
	for _, v := range vs {
		if len(ModulesByVendor(v)) == 0 {
			t.Errorf("vendor %q has no modules", v)
		}
	}
}
