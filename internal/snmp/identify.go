package snmp

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// System-group OIDs (SNMPv2-MIB / RFC1213).
const (
	OIDsysDescr    = "1.3.6.1.2.1.1.1.0"
	OIDsysObjectID = "1.3.6.1.2.1.1.2.0"
	OIDsysUpTime   = "1.3.6.1.2.1.1.3.0"
	OIDsysContact  = "1.3.6.1.2.1.1.4.0"
	OIDsysName     = "1.3.6.1.2.1.1.5.0"
	OIDsysLocation = "1.3.6.1.2.1.1.6.0"
	OIDsysServices = "1.3.6.1.2.1.1.7.0"

	// Common roots.
	OIDmib2        = "1.3.6.1.2.1"
	OIDenterprises = "1.3.6.1.4.1"
	OIDinternet    = "1.3.6.1"
)

// SystemInfo is the identity/analysis derived from the system group.
type SystemInfo struct {
	Descr    string
	ObjectID string
	Contact  string
	Name     string
	Location string
	Services int
	Uptime   time.Duration

	Vendor     string   // resolved from the enterprise number in sysObjectID
	Enterprise int      // enterprise number
	Role       string   // best-effort device role
	Layers     []string // decoded sysServices layers
	Reachable  bool
	RTT        time.Duration
	Notes      []string // extra analysis lines
}

// Discover fetches and analyses the system group from src.
func Discover(src Source) (SystemInfo, error) {
	start := time.Now()
	vars, err := src.Get([]string{
		OIDsysDescr, OIDsysObjectID, OIDsysUpTime,
		OIDsysContact, OIDsysName, OIDsysLocation, OIDsysServices,
	})
	if err != nil {
		return SystemInfo{}, err
	}
	info := SystemInfo{Reachable: true, RTT: time.Since(start)}
	for _, v := range vars {
		switch v.OID {
		case OIDsysDescr:
			info.Descr = v.Str
		case OIDsysObjectID:
			info.ObjectID = strings.TrimPrefix(v.Str, ".")
		case OIDsysUpTime:
			info.Uptime = time.Duration(v.Num) * 10 * time.Millisecond
		case OIDsysContact:
			info.Contact = v.Str
		case OIDsysName:
			info.Name = v.Str
		case OIDsysLocation:
			info.Location = v.Str
		case OIDsysServices:
			info.Services = int(v.Num)
		}
	}
	info.analyse()
	return info, nil
}

var reEnterprise = regexp.MustCompile(`^1\.3\.6\.1\.4\.1\.(\d+)`)

func (s *SystemInfo) analyse() {
	if m := reEnterprise.FindStringSubmatch(s.ObjectID); m != nil {
		s.Enterprise, _ = strconv.Atoi(m[1])
		s.Vendor = VendorName(s.Enterprise)
	}
	s.Layers = decodeServices(s.Services)
	s.Role = guessRole(s.Descr, s.ObjectID, s.Services)

	if s.Contact == "" {
		s.Notes = append(s.Notes, "sysContact is empty - no admin/owner published")
	}
	if s.Location == "" {
		s.Notes = append(s.Notes, "sysLocation is empty - physical placement unknown")
	}
	if strings.Contains(strings.ToLower(s.Descr), "linux") {
		s.Notes = append(s.Notes, "kernel string exposed in sysDescr (host fingerprint leak)")
	}
	if s.Enterprise == 8072 {
		s.Notes = append(s.Notes, "Net-SNMP agent (enterprise 8072) - likely a general purpose server")
	}
}

// decodeServices turns the sysServices bitmask into readable OSI layer names.
// Value = sum over layers L of 2^(L-1) for each supported layer 1..7.
func decodeServices(v int) []string {
	if v <= 0 {
		return nil
	}
	names := map[int]string{
		1: "physical",
		2: "datalink/subnet",
		3: "internet/routing",
		4: "end-to-end/transport",
		7: "applications",
	}
	var out []string
	for l := 1; l <= 7; l++ {
		if v&(1<<(l-1)) != 0 {
			if n, ok := names[l]; ok {
				out = append(out, fmt.Sprintf("L%d %s", l, n))
			} else {
				out = append(out, fmt.Sprintf("L%d", l))
			}
		}
	}
	return out
}

type rolePattern struct {
	re   *regexp.Regexp
	role string
}

var rolePatterns = []rolePattern{
	{regexp.MustCompile(`(?i)ios[ -]?xe|ios[ -]?xr|\bIOS\b|catalyst|nx-?os`), "Cisco network device (router/switch)"},
	{regexp.MustCompile(`(?i)routeros|mikrotik`), "MikroTik RouterOS device"},
	{regexp.MustCompile(`(?i)juniper|junos`), "Juniper JUNOS device"},
	{regexp.MustCompile(`(?i)arista|\bEOS\b`), "Arista switch"},
	{regexp.MustCompile(`(?i)fortigate|fortios|palo alto|pan-os|sonicwall|checkpoint`), "Firewall / security appliance"},
	{regexp.MustCompile(`(?i)pfsense|opnsense`), "BSD software firewall"},
	{regexp.MustCompile(`(?i)laserjet|officejet|lexmark|kyocera|ricoh|xerox|brother|printer`), "Network printer / MFP"},
	{regexp.MustCompile(`(?i)\bUPS\b|smart-?ups|powerware|eaton|apc\b`), "UPS / power device"},
	{regexp.MustCompile(`(?i)synology|diskstation|qnap|truenas|freenas|netapp|isilon`), "NAS / storage appliance"},
	{regexp.MustCompile(`(?i)vmware|esxi|vsphere`), "VMware hypervisor"},
	{regexp.MustCompile(`(?i)proxmox|pve`), "Proxmox VE hypervisor"},
	{regexp.MustCompile(`(?i)ubiquiti|unifi|edgeos|edgerouter`), "Ubiquiti device"},
	{regexp.MustCompile(`(?i)aruba|hpe|procurve|comware`), "HPE/Aruba network device"},
	{regexp.MustCompile(`(?i)windows`), "Microsoft Windows host"},
	{regexp.MustCompile(`(?i)linux`), "Linux host / server"},
	{regexp.MustCompile(`(?i)freebsd|openbsd|netbsd`), "BSD host"},
	{regexp.MustCompile(`(?i)darwin|mac ?os`), "macOS host"},
}

func guessRole(descr, objectID string, services int) string {
	for _, p := range rolePatterns {
		if p.re.MatchString(descr) {
			return p.role
		}
	}
	switch {
	case services&(1<<2) != 0 && services&(1<<6) == 0:
		return "Layer-3 device (router / L3 switch)"
	case services&(1<<1) != 0 && services&(1<<2) == 0:
		return "Layer-2 device (switch / bridge)"
	case services&(1<<6) != 0:
		return "Host / application server"
	}
	if objectID != "" {
		return "Unknown - enterprise " + objectID
	}
	return "Unknown SNMP device"
}
