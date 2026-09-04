package snmp

import "fmt"

// vendorByEnterprise maps a subset of IANA "SMI Network Management Private
// Enterprise Codes" (the number N in 1.3.6.1.4.1.N) to a readable vendor name.
// It is intentionally small - enough to identify the devices most people run
// into on a mixed network.
var vendorByEnterprise = map[int]string{
	2:     "IBM",
	9:     "Cisco Systems",
	11:    "Hewlett-Packard / HPE",
	23:    "Novell",
	42:    "Sun Microsystems / Oracle",
	43:    "3Com",
	63:    "Apple",
	171:   "D-Link",
	207:   "Allied Telesis",
	232:   "HP (Compaq)",
	244:   "Lantronix",
	253:   "Xerox",
	311:   "Microsoft",
	318:   "APC / Schneider Electric",
	368:   "Xylan",
	637:   "Alcatel-Lucent Enterprise",
	674:   "Dell",
	1588:  "Brocade / Broadcom",
	1916:  "Extreme Networks",
	1991:  "Foundry Networks",
	2011:  "Huawei",
	2021:  "UC Davis (Net-SNMP UCD-SNMP-MIB)",
	2352:  "Redback / Ericsson",
	2435:  "Brother",
	2636:  "Juniper Networks",
	3224:  "Palo Alto Networks",
	3375:  "F5 Networks",
	3417:  "Broadcom",
	3495:  "Nortel",
	3955:  "Linksys",
	4526:  "Netgear",
	5528:  "NetBotz",
	5624:  "Ecos / Eltek",
	6027:  "Force10 Networks",
	6486:  "Alcatel",
	6574:  "Konica Minolta",
	6876:  "VMware",
	8072:  "Net-SNMP",
	8741:  "Watchguard",
	9600:  "Riverbed",
	10002: "Ubiquiti (UBNT legacy)",
	11863: "TP-Link",
	12356: "Fortinet",
	12532: "SonicWall",
	14179: "Aruba Networks",
	14525: "Trapeze / Juniper WLAN",
	14823: "Aruba (HPE)",
	14988: "MikroTik",
	16974: "MikroTik (RouterBOARD)",
	17163: "Stratacom",
	18334: "Konica Minolta",
	19046: "Lenovo",
	20858: "Grandstream",
	21091: "Rittal",
	22610: "Vertiv / Emerson (Liebert)",
	23782: "Sophos",
	25506: "H3C",
	26543: "IBM",
	26928: "Ruckus Wireless",
	28507: "AVM (FRITZ!Box)",
	30065: "Arista Networks",
	30303: "Datto",
	31163: "QNAP",
	32473: "Example / documentation (RFC 5612)",
	35424: "MikroTik (SwitchOS)",
	41112: "Ubiquiti Networks",
	47196: "TrueNAS / iXsystems",
	50002: "Synology",
	52642: "Cambium Networks",
	54595: "Teltonika",
}

// VendorName resolves an enterprise number to a vendor, or a generic label.
func VendorName(enterprise int) string {
	if enterprise == 0 {
		return ""
	}
	if v, ok := vendorByEnterprise[enterprise]; ok {
		return v
	}
	return fmt.Sprintf("enterprise %d (unregistered in local table)", enterprise)
}
