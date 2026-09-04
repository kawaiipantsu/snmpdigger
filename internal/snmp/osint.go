package snmp

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// OSINT is the network-layer intelligence gathered about a target IP from
// public sources (reverse DNS + the RIPEstat data API). All fields are
// best-effort; Err holds the first failure if the lookups could not complete.
type OSINT struct {
	IP         string
	Private    bool
	PTR        string
	ASN        string
	ASNHolder  string
	Prefix     string
	RIR        string
	GeoCountry string
	GeoCity    string
	GeoLat     float64
	GeoLon     float64
	Err        string
}

// EnrichOSINT resolves reverse DNS and queries RIPEstat for the ASN/holder,
// announced prefix, RIR and MaxMind GeoLite location of ip.
func EnrichOSINT(ctx context.Context, ip string) OSINT {
	o := OSINT{IP: ip}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		o.Err = "not an IP address"
		return o
	}
	if parsed.To4() != nil && (isPrivateV4(parsed) || parsed.IsLoopback() || parsed.IsLinkLocalUnicast()) {
		o.Private = true
	}

	var wg sync.WaitGroup

	// reverse DNS (bounded)
	wg.Add(1)
	go func() {
		defer wg.Done()
		rctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		var r net.Resolver
		if names, err := r.LookupAddr(rctx, ip); err == nil && len(names) > 0 {
			o.PTR = strings.TrimSuffix(names[0], ".")
		}
	}()

	if !o.Private {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var pre struct {
				Data struct {
					Resource string `json:"resource"`
					ASNs     []struct {
						ASN    int    `json:"asn"`
						Holder string `json:"holder"`
					} `json:"asns"`
				} `json:"data"`
			}
			if err := ripestat(ctx, "prefix-overview", ip, &pre); err != nil {
				if o.Err == "" {
					o.Err = err.Error()
				}
				return
			}
			o.Prefix = pre.Data.Resource
			if len(pre.Data.ASNs) > 0 && pre.Data.ASNs[0].ASN > 0 {
				o.ASN = asnLabel(pre.Data.ASNs[0].ASN)
				o.ASNHolder = pre.Data.ASNs[0].Holder
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			var geo struct {
				Data struct {
					LocatedResources []struct {
						Locations []struct {
							Country   string  `json:"country"`
							City      string  `json:"city"`
							Latitude  float64 `json:"latitude"`
							Longitude float64 `json:"longitude"`
						} `json:"locations"`
					} `json:"located_resources"`
				} `json:"data"`
			}
			if err := ripestat(ctx, "maxmind-geo-lite", ip, &geo); err != nil {
				return
			}
			for _, lr := range geo.Data.LocatedResources {
				for _, l := range lr.Locations {
					o.GeoCountry, o.GeoCity = l.Country, l.City
					o.GeoLat, o.GeoLon = l.Latitude, l.Longitude
					return
				}
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			var rir struct {
				Data struct {
					Rirs []struct {
						Rir string `json:"rir"`
					} `json:"rirs"`
				} `json:"data"`
			}
			if err := ripestat(ctx, "rir", ip, &rir); err == nil && len(rir.Data.Rirs) > 0 {
				o.RIR = rir.Data.Rirs[0].Rir
			}
		}()
	}

	wg.Wait()
	return o
}

func asnLabel(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("AS%d", n)
}

// --- MAC OUI (small curated table) ---------------------------------------

var ouiVendors = map[string]string{
	"00:00:0C": "Cisco", "00:1B:54": "Cisco", "00:25:45": "Cisco", "F4:CF:E2": "Cisco",
	"00:1A:A1": "Cisco", "00:0A:B8": "Cisco", "E0:2F:6D": "Cisco",
	"00:0C:29": "VMware", "00:50:56": "VMware", "00:05:69": "VMware",
	"08:00:27": "VirtualBox", "52:54:00": "QEMU/KVM", "00:16:3E": "Xen",
	"00:15:5D": "Microsoft Hyper-V",
	"B8:27:EB": "Raspberry Pi", "DC:A6:32": "Raspberry Pi", "E4:5F:01": "Raspberry Pi",
	"00:0D:B9": "PC Engines", "00:1C:42": "Parallels",
	"00:1C:C0": "Intel", "00:1B:21": "Intel", "3C:FD:FE": "Intel", "A0:36:9F": "Intel",
	"00:E0:4C": "Realtek", "52:54:AB": "Realtek",
	"00:0C:42": "MikroTik", "4C:5E:0C": "MikroTik", "6C:3B:6B": "MikroTik",
	"C4:AD:34": "MikroTik", "48:8F:5A": "MikroTik", "18:FD:74": "MikroTik",
	"00:1D:AA": "MikroTik", "DC:2C:6E": "MikroTik", "2C:C8:1B": "MikroTik",
	"00:90:4C": "Juniper", "3C:8A:B0": "Juniper", "F0:1C:2D": "Juniper", "5C:5E:AB": "Juniper",
	"00:1F:12": "Juniper", "78:19:F7": "Juniper",
	"00:1B:17": "Palo Alto Networks", "00:1C:73": "Arista", "44:4C:A8": "Arista",
	"00:09:0F": "Fortinet", "08:5B:0E": "Fortinet", "70:4C:A5": "Fortinet", "90:6C:AC": "Fortinet",
	"00:17:C5": "SonicWall", "C0:EA:E4": "Sophos",
	"00:1A:8C": "Check Point", "00:1C:7F": "Check Point",
	"24:5A:4C": "Ubiquiti", "78:8A:20": "Ubiquiti", "F0:9F:C2": "Ubiquiti",
	"04:18:D6": "Ubiquiti", "DC:9F:DB": "Ubiquiti", "68:D7:9A": "Ubiquiti", "B4:FB:E4": "Ubiquiti",
	"00:1B:78": "HP", "3C:D9:2B": "HP/HPE", "98:F2:B3": "HPE", "00:26:55": "HP",
	"00:1E:0B": "HP", "94:57:A5": "HP", "00:80:A3": "Lantronix",
	"00:14:22": "Dell", "18:66:DA": "Dell", "B8:2A:72": "Dell", "F8:BC:12": "Dell", "84:2B:2B": "Dell",
	"00:1C:B3": "Apple", "3C:07:54": "Apple", "F0:18:98": "Apple", "A4:83:E7": "Apple",
	"00:00:5E": "IANA (VRRP/virtual)", "01:00:5E": "IPv4 multicast",
	"00:11:32": "Synology", "00:1D:73": "QNAP", "24:5E:BE": "QNAP",
	"00:04:96": "Extreme Networks", "00:E0:2B": "Extreme Networks",
	"D0:67:E5": "Huawei", "00:E0:FC": "Huawei", "78:D7:52": "Huawei", "48:46:FB": "Huawei",
	"00:1E:C9": "Dell", "00:1D:D8": "Microsoft", "00:03:47": "Intel",
}

// OUIVendor returns a best-effort vendor for a MAC address, or "".
func OUIVendor(mac string) string {
	mac = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(mac), "-", ":"))
	if len(mac) < 8 {
		return ""
	}
	return ouiVendors[mac[:8]]
}
