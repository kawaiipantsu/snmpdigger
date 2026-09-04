package mib

import (
	"sort"
	"strings"
	"sync"
)

// CatObject is one object definition inside a catalogued MIB module.
type CatObject struct {
	OID    string // dotted numeric, no leading dot
	Name   string // object descriptor, e.g. "ifInOctets"
	Type   string // SMI type / textual convention
	Access string // read-only | read-write | read-create | not-accessible
	Descr  string // short human description
}

// CatModule is a catalogued MIB module and its notable objects.
type CatModule struct {
	Module     string
	Vendor     string
	Enterprise int // IANA PEN, 0 for pure standard modules
	Root       string
	Summary    string
	Objects    []CatObject
}

// VendorStd / VendorNetSNMP are the two "always offline" backbone groups.
const (
	VendorStd     = "IETF / Standard"
	VendorNetSNMP = "Net-SNMP / UCD"
)

var (
	catOnce   sync.Once
	catSorted []CatModule
)

// Catalog returns the full module list: standard modules first, then vendors
// alphabetically, modules alphabetical within a vendor.
func Catalog() []CatModule {
	catOnce.Do(func() {
		all := make([]CatModule, 0, len(catalogData))
		all = append(all, catalogData...)
		rank := func(v string) int {
			switch v {
			case VendorStd:
				return 0
			case VendorNetSNMP:
				return 1
			default:
				return 2
			}
		}
		sort.SliceStable(all, func(i, j int) bool {
			ri, rj := rank(all[i].Vendor), rank(all[j].Vendor)
			if ri != rj {
				return ri < rj
			}
			if all[i].Vendor != all[j].Vendor {
				return all[i].Vendor < all[j].Vendor
			}
			return all[i].Module < all[j].Module
		})
		catSorted = all
	})
	return catSorted
}

// Vendors lists distinct vendor names in display order (no synthetic "All").
func Vendors() []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range Catalog() {
		if !seen[m.Vendor] {
			seen[m.Vendor] = true
			out = append(out, m.Vendor)
		}
	}
	return out
}

// ModulesByVendor returns the modules for one vendor (all, if vendor == "" or "All").
func ModulesByVendor(vendor string) []CatModule {
	if vendor == "" || vendor == "All" {
		return Catalog()
	}
	var out []CatModule
	for _, m := range Catalog() {
		if m.Vendor == vendor {
			out = append(out, m)
		}
	}
	return out
}

// LookupCatalog finds an exact OID match across the catalogue.
func LookupCatalog(oid string) (CatModule, CatObject, bool) {
	oid = strings.TrimPrefix(strings.TrimSpace(oid), ".")
	for _, m := range Catalog() {
		for _, o := range m.Objects {
			if o.OID == oid {
				return m, o, true
			}
		}
	}
	return CatModule{}, CatObject{}, false
}

// SearchCatalog does a case-insensitive substring search over Name / OID /
// Descr, capped at 500 results, each Descr prefixed with "[MODULE] ".
func SearchCatalog(q string) []CatObject {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	var out []CatObject
	for _, m := range Catalog() {
		for _, o := range m.Objects {
			if strings.Contains(strings.ToLower(o.Name), q) ||
				strings.Contains(o.OID, q) ||
				strings.Contains(strings.ToLower(o.Descr), q) {
				o.Descr = "[" + m.Module + "] " + o.Descr
				out = append(out, o)
				if len(out) >= 500 {
					return out
				}
			}
		}
	}
	return out
}

// ro/rw/rc/na are terse access constants to keep the data literal compact.
const (
	ro = "read-only"
	rw = "read-write"
	rc = "read-create"
	na = "not-accessible"
)

var catalogData = func() []CatModule {
	var mods []CatModule
	mods = append(mods, stdModules...)
	mods = append(mods, netSnmpModules...)
	mods = append(mods, vendorModules...)
	return mods
}()

// ------------------------------------------------------------------ standard --

var stdModules = []CatModule{
	{
		Module: "SNMPv2-MIB", Vendor: VendorStd, Root: "1.3.6.1.2.1.1",
		Summary: "The system group and SNMP engine statistics every agent implements.",
		Objects: []CatObject{
			{"1.3.6.1.2.1.1.1.0", "sysDescr", "DisplayString", ro, "Textual description of the entity: hardware, OS and networking software. Often leaks kernel/version fingerprints."},
			{"1.3.6.1.2.1.1.2.0", "sysObjectID", "OBJECT IDENTIFIER", ro, "Vendor authoritative identification OID; the enterprise sub-arc identifies the manufacturer."},
			{"1.3.6.1.2.1.1.3.0", "sysUpTime", "TimeTicks", ro, "Time since the network-management portion of the system was last re-initialised, in 1/100 s."},
			{"1.3.6.1.2.1.1.4.0", "sysContact", "DisplayString", rw, "Contact person for this managed node, plus how to reach them."},
			{"1.3.6.1.2.1.1.5.0", "sysName", "DisplayString", rw, "Administratively assigned name; by convention the node's fully-qualified domain name."},
			{"1.3.6.1.2.1.1.6.0", "sysLocation", "DisplayString", rw, "Physical location of this node (e.g. 'rack 3, row B, DC1')."},
			{"1.3.6.1.2.1.1.7.0", "sysServices", "INTEGER", ro, "Bitmap of the OSI layers the node offers service at (L1..L7)."},
			{"1.3.6.1.2.1.1.9.1.2", "sysORID", "OBJECT IDENTIFIER", ro, "Authoritative identifier of a capability (MIB) the agent supports."},
			{"1.3.6.1.2.1.1.9.1.3", "sysORDescr", "DisplayString", ro, "Textual description of a supported capability / MIB module."},
			{"1.3.6.1.2.1.11.1.0", "snmpInPkts", "Counter32", ro, "Total SNMP messages delivered to the agent from the transport."},
			{"1.3.6.1.2.1.11.2.0", "snmpOutPkts", "Counter32", ro, "Total SNMP messages passed from the agent to the transport."},
			{"1.3.6.1.2.1.11.3.0", "snmpInBadVersions", "Counter32", ro, "SNMP messages for an unsupported version."},
			{"1.3.6.1.2.1.11.4.0", "snmpInBadCommunityNames", "Counter32", ro, "SNMP messages with a community string the agent did not recognise - a brute-force / scan indicator."},
			{"1.3.6.1.2.1.11.5.0", "snmpInBadCommunityUses", "Counter32", ro, "Messages using a known community for an operation it is not authorised for."},
			{"1.3.6.1.2.1.11.30.0", "snmpEnableAuthenTraps", "INTEGER", rw, "Whether the agent is permitted to emit authentication-failure traps (enabled(1)/disabled(2))."},
			{"1.3.6.1.2.1.11.31.0", "snmpSilentDrops", "Counter32", ro, "Requests dropped because the reply would exceed the transport's maximum message size."},
		},
	},
	{
		Module: "IF-MIB", Vendor: VendorStd, Root: "1.3.6.1.2.1.2",
		Summary: "Per-interface configuration, state and traffic/error counters (ifTable + ifXTable).",
		Objects: []CatObject{
			{"1.3.6.1.2.1.2.1.0", "ifNumber", "INTEGER", ro, "Number of network interfaces present on the system."},
			{"1.3.6.1.2.1.2.2.1.1", "ifIndex", "InterfaceIndex", ro, "Unique integer index for each interface; the instance suffix on every other ifTable column."},
			{"1.3.6.1.2.1.2.2.1.2", "ifDescr", "DisplayString", ro, "Vendor text for the interface (name, product name, hardware version)."},
			{"1.3.6.1.2.1.2.2.1.3", "ifType", "IANAifType", ro, "Interface type per IANAifType-MIB (6 = ethernetCsmacd, 24 = softwareLoopback, ...)."},
			{"1.3.6.1.2.1.2.2.1.4", "ifMtu", "INTEGER", ro, "Largest datagram that can be sent/received on the interface, in octets."},
			{"1.3.6.1.2.1.2.2.1.5", "ifSpeed", "Gauge32", ro, "Current bandwidth estimate in bits per second (capped at ~4.29 Gbps; use ifHighSpeed above that)."},
			{"1.3.6.1.2.1.2.2.1.6", "ifPhysAddress", "PhysAddress", ro, "Interface address at the protocol layer below IP (usually the MAC address)."},
			{"1.3.6.1.2.1.2.2.1.7", "ifAdminStatus", "INTEGER", rw, "Desired interface state: up(1), down(2), testing(3)."},
			{"1.3.6.1.2.1.2.2.1.8", "ifOperStatus", "INTEGER", ro, "Actual interface state: up(1), down(2), testing(3), dormant(5), lowerLayerDown(7)."},
			{"1.3.6.1.2.1.2.2.1.9", "ifLastChange", "TimeTicks", ro, "sysUpTime when the interface entered its current operational state."},
			{"1.3.6.1.2.1.2.2.1.10", "ifInOctets", "Counter32", ro, "Total octets received on the interface, including framing."},
			{"1.3.6.1.2.1.2.2.1.11", "ifInUcastPkts", "Counter32", ro, "Unicast packets delivered to a higher layer."},
			{"1.3.6.1.2.1.2.2.1.13", "ifInDiscards", "Counter32", ro, "Inbound packets discarded despite no error (e.g. to free buffer space)."},
			{"1.3.6.1.2.1.2.2.1.14", "ifInErrors", "Counter32", ro, "Inbound packets that contained errors preventing delivery."},
			{"1.3.6.1.2.1.2.2.1.16", "ifOutOctets", "Counter32", ro, "Total octets transmitted out of the interface, including framing."},
			{"1.3.6.1.2.1.2.2.1.17", "ifOutUcastPkts", "Counter32", ro, "Unicast packets requested to be transmitted."},
			{"1.3.6.1.2.1.2.2.1.19", "ifOutDiscards", "Counter32", ro, "Outbound packets discarded despite no error."},
			{"1.3.6.1.2.1.2.2.1.20", "ifOutErrors", "Counter32", ro, "Outbound packets not transmitted because of errors."},
			{"1.3.6.1.2.1.31.1.1.1.1", "ifName", "DisplayString", ro, "Short interface name as seen on the device console (e.g. 'Gi0/1', 'eth0')."},
			{"1.3.6.1.2.1.31.1.1.1.6", "ifHCInOctets", "Counter64", ro, "64-bit received-octets counter for high-speed interfaces."},
			{"1.3.6.1.2.1.31.1.1.1.10", "ifHCOutOctets", "Counter64", ro, "64-bit transmitted-octets counter for high-speed interfaces."},
			{"1.3.6.1.2.1.31.1.1.1.15", "ifHighSpeed", "Gauge32", ro, "Interface speed in units of 1,000,000 bits per second."},
			{"1.3.6.1.2.1.31.1.1.1.18", "ifAlias", "DisplayString", rw, "Operator-assigned interface description; persists across reboots."},
			{"1.3.6.1.2.1.31.1.1.1.17", "ifConnectorPresent", "TruthValue", ro, "true(1) if the interface has a physical connector."},
		},
	},
	{
		Module: "IP-MIB", Vendor: VendorStd, Root: "1.3.6.1.2.1.4",
		Summary: "IP layer counters and the interface address table.",
		Objects: []CatObject{
			{"1.3.6.1.2.1.4.1.0", "ipForwarding", "INTEGER", rw, "forwarding(1) if this entity acts as an IP router."},
			{"1.3.6.1.2.1.4.2.0", "ipDefaultTTL", "INTEGER", rw, "Default TTL inserted into IP headers when the transport does not supply one."},
			{"1.3.6.1.2.1.4.3.0", "ipInReceives", "Counter32", ro, "Total input datagrams received from all interfaces, including errors."},
			{"1.3.6.1.2.1.4.4.0", "ipInHdrErrors", "Counter32", ro, "Datagrams discarded due to IP header errors (bad checksum, version, TTL exceeded, ...)."},
			{"1.3.6.1.2.1.4.5.0", "ipInAddrErrors", "Counter32", ro, "Datagrams discarded because the destination address was not valid for this entity."},
			{"1.3.6.1.2.1.4.6.0", "ipForwDatagrams", "Counter32", ro, "Datagrams for which this entity was not the final destination and forwarding was attempted."},
			{"1.3.6.1.2.1.4.9.0", "ipInDelivers", "Counter32", ro, "Datagrams successfully delivered to IP user-protocols."},
			{"1.3.6.1.2.1.4.10.0", "ipOutRequests", "Counter32", ro, "Datagrams supplied to IP for transmission by local protocols (excludes forwarded traffic)."},
			{"1.3.6.1.2.1.4.20.1.1", "ipAdEntAddr", "IpAddress", ro, "An IP address of one of this entity's interfaces."},
			{"1.3.6.1.2.1.4.20.1.2", "ipAdEntIfIndex", "INTEGER", ro, "ifIndex the address is assigned to."},
			{"1.3.6.1.2.1.4.20.1.3", "ipAdEntNetMask", "IpAddress", ro, "Subnet mask associated with the address."},
			{"1.3.6.1.2.1.4.22.1.2", "ipNetToMediaPhysAddress", "PhysAddress", rw, "ARP table: media (MAC) address for an IP address on an interface."},
		},
	},
	{
		Module: "TCP-MIB", Vendor: VendorStd, Root: "1.3.6.1.2.1.6",
		Summary: "TCP-layer statistics and the connection table.",
		Objects: []CatObject{
			{"1.3.6.1.2.1.6.1.0", "tcpRtoAlgorithm", "INTEGER", ro, "Retransmission-timeout algorithm in use (vanj(4) = Van Jacobson)."},
			{"1.3.6.1.2.1.6.4.0", "tcpMaxConn", "INTEGER", ro, "Limit on the number of TCP connections the entity can support (-1 = dynamic)."},
			{"1.3.6.1.2.1.6.5.0", "tcpActiveOpens", "Counter32", ro, "Times TCP connections made a direct SYN-SENT -> ESTABLISHED transition (outbound connects)."},
			{"1.3.6.1.2.1.6.6.0", "tcpPassiveOpens", "Counter32", ro, "Times TCP connections made a direct SYN-RCVD transition from LISTEN (inbound connects)."},
			{"1.3.6.1.2.1.6.7.0", "tcpAttemptFails", "Counter32", ro, "Connection attempts that failed (SYN-SENT/SYN-RCVD back to CLOSED or LISTEN)."},
			{"1.3.6.1.2.1.6.8.0", "tcpEstabResets", "Counter32", ro, "Resets from ESTABLISHED or CLOSE-WAIT to CLOSED."},
			{"1.3.6.1.2.1.6.9.0", "tcpCurrEstab", "Gauge32", ro, "Connections currently in ESTABLISHED or CLOSE-WAIT."},
			{"1.3.6.1.2.1.6.10.0", "tcpInSegs", "Counter32", ro, "Segments received, including those in error."},
			{"1.3.6.1.2.1.6.11.0", "tcpOutSegs", "Counter32", ro, "Segments sent, excluding retransmitted octets."},
			{"1.3.6.1.2.1.6.12.0", "tcpRetransSegs", "Counter32", ro, "Segments retransmitted - a congestion / loss indicator."},
			{"1.3.6.1.2.1.6.13.1.1", "tcpConnState", "INTEGER", rw, "State of a TCP connection (listen(2), established(5), timeWait(11), ...)."},
		},
	},
	{
		Module: "UDP-MIB", Vendor: VendorStd, Root: "1.3.6.1.2.1.7",
		Summary: "UDP-layer datagram statistics.",
		Objects: []CatObject{
			{"1.3.6.1.2.1.7.1.0", "udpInDatagrams", "Counter32", ro, "UDP datagrams delivered to UDP users."},
			{"1.3.6.1.2.1.7.2.0", "udpNoPorts", "Counter32", ro, "Datagrams received for which there was no application at the destination port."},
			{"1.3.6.1.2.1.7.3.0", "udpInErrors", "Counter32", ro, "Datagrams that could not be delivered for reasons other than no application at the port."},
			{"1.3.6.1.2.1.7.4.0", "udpOutDatagrams", "Counter32", ro, "UDP datagrams sent from this entity."},
		},
	},
	{
		Module: "IP-FORWARD-MIB", Vendor: VendorStd, Root: "1.3.6.1.2.1.4.24",
		Summary: "The routing table (inetCidrRouteTable), protocol- and version-neutral.",
		Objects: []CatObject{
			{"1.3.6.1.2.1.4.24.7.1.7", "inetCidrRouteIfIndex", "InterfaceIndexOrZero", rc, "Outgoing ifIndex for a route entry."},
			{"1.3.6.1.2.1.4.24.7.1.8", "inetCidrRouteType", "INTEGER", rc, "Route type: local(3), remote(4), reject(2)."},
			{"1.3.6.1.2.1.4.24.7.1.9", "inetCidrRouteProto", "IANAipRouteProtocol", ro, "How the route was learned: local(2), netmgmt(3), ospf(13), bgp(14), ..."},
			{"1.3.6.1.2.1.4.24.7.1.4", "inetCidrRouteNextHop", "InetAddress", rc, "Next-hop address for a remote route."},
		},
	},
	{
		Module: "HOST-RESOURCES-MIB", Vendor: VendorStd, Root: "1.3.6.1.2.1.25",
		Summary: "Generic host state: uptime, memory, storage, per-CPU load, running software and devices.",
		Objects: []CatObject{
			{"1.3.6.1.2.1.25.1.1.0", "hrSystemUptime", "TimeTicks", ro, "Time since the host (not just the agent) was last booted, in 1/100 s."},
			{"1.3.6.1.2.1.25.1.5.0", "hrSystemNumUsers", "Gauge32", ro, "Number of user sessions currently logged in."},
			{"1.3.6.1.2.1.25.1.6.0", "hrSystemProcesses", "Gauge32", ro, "Number of process contexts currently loaded/running."},
			{"1.3.6.1.2.1.25.1.7.0", "hrSystemMaxProcesses", "INTEGER", ro, "Maximum number of process contexts the host supports (0 = no fixed limit)."},
			{"1.3.6.1.2.1.25.2.2.0", "hrMemorySize", "KBytes", ro, "Amount of physical RAM in the host, in KiB."},
			{"1.3.6.1.2.1.25.2.3.1.1", "hrStorageIndex", "INTEGER", ro, "Unique index for each logical storage area."},
			{"1.3.6.1.2.1.25.2.3.1.2", "hrStorageType", "AutonomousType", ro, "Storage class OID (RAM, virtual memory, fixed disk, removable disk, ...)."},
			{"1.3.6.1.2.1.25.2.3.1.3", "hrStorageDescr", "DisplayString", ro, "Text describing the storage area (mount point, 'Physical memory', swap, ...)."},
			{"1.3.6.1.2.1.25.2.3.1.4", "hrStorageAllocationUnits", "INTEGER", ro, "Size in bytes of a storage allocation unit; multiply Size/Used by this for bytes."},
			{"1.3.6.1.2.1.25.2.3.1.5", "hrStorageSize", "INTEGER", ro, "Total size of the storage area in allocation units."},
			{"1.3.6.1.2.1.25.2.3.1.6", "hrStorageUsed", "INTEGER", ro, "Allocation units currently in use."},
			{"1.3.6.1.2.1.25.3.3.1.2", "hrProcessorLoad", "INTEGER", ro, "Per-CPU load: average percent utilisation over the last minute (0-100)."},
			{"1.3.6.1.2.1.25.3.2.1.3", "hrDeviceDescr", "DisplayString", ro, "Textual description of a hardware device (CPU, disk, NIC, ...)."},
			{"1.3.6.1.2.1.25.3.2.1.5", "hrDeviceStatus", "INTEGER", ro, "Device operational status: running(2), warning(3), testing(4), down(5)."},
			{"1.3.6.1.2.1.25.4.2.1.2", "hrSWRunName", "InternationalDisplayString", ro, "Name of a running piece of software (executable / process name)."},
			{"1.3.6.1.2.1.25.4.2.1.7", "hrSWRunStatus", "INTEGER", ro, "running(1), runnable(2), notRunnable(3), invalid(4)."},
		},
	},
	{
		Module: "ENTITY-MIB", Vendor: VendorStd, Root: "1.3.6.1.2.1.47",
		Summary: "Physical inventory: chassis, modules, fans, PSUs, sensors and their serial/model/firmware.",
		Objects: []CatObject{
			{"1.3.6.1.2.1.47.1.1.1.1.2", "entPhysicalDescr", "DisplayString", ro, "Human description of a physical component."},
			{"1.3.6.1.2.1.47.1.1.1.1.5", "entPhysicalClass", "PhysicalClass", ro, "Component class: chassis(3), backplane(4), container(5), module(9), port(10), fan(7), powerSupply(6), sensor(8)."},
			{"1.3.6.1.2.1.47.1.1.1.1.7", "entPhysicalName", "DisplayString", ro, "The name as reported on the device console (e.g. 'GigabitEthernet1/0/1')."},
			{"1.3.6.1.2.1.47.1.1.1.1.11", "entPhysicalSerialNum", "DisplayString", ro, "Component serial number."},
			{"1.3.6.1.2.1.47.1.1.1.1.13", "entPhysicalModelName", "DisplayString", ro, "Vendor model / part number."},
			{"1.3.6.1.2.1.47.1.1.1.1.9", "entPhysicalFirmwareRev", "DisplayString", ro, "Firmware revision of the component."},
			{"1.3.6.1.2.1.47.1.1.1.1.10", "entPhysicalSoftwareRev", "DisplayString", ro, "Software revision running on the component."},
		},
	},
	{
		Module: "ENTITY-SENSOR-MIB", Vendor: VendorStd, Root: "1.3.6.1.2.1.99",
		Summary: "Generic sensor readings (temperature, voltage, current, fan RPM, optical dBm) keyed by entPhysicalIndex.",
		Objects: []CatObject{
			{"1.3.6.1.2.1.99.1.1.1.1", "entPhySensorType", "INTEGER", ro, "Sensor kind: voltsAC(3), voltsDC(4), amperes(5), watts(6), celsius(8), rpm(10), truthvalue(12), dBm(14)."},
			{"1.3.6.1.2.1.99.1.1.1.2", "entPhySensorScale", "INTEGER", ro, "SI prefix applied to the value (milli(8), units(9), kilo(10), ...)."},
			{"1.3.6.1.2.1.99.1.1.1.3", "entPhySensorPrecision", "INTEGER", ro, "Number of decimal places implied in entPhySensorValue."},
			{"1.3.6.1.2.1.99.1.1.1.4", "entPhySensorValue", "Integer32", ro, "Most recent sensor reading, interpreted with Scale and Precision."},
			{"1.3.6.1.2.1.99.1.1.1.5", "entPhySensorOperStatus", "INTEGER", ro, "ok(1), unavailable(2), nonoperational(3)."},
		},
	},
	{
		Module: "BRIDGE-MIB", Vendor: VendorStd, Root: "1.3.6.1.2.1.17",
		Summary: "802.1D bridging: base ports, spanning tree state and the forwarding (MAC) database.",
		Objects: []CatObject{
			{"1.3.6.1.2.1.17.1.2.0", "dot1dBaseNumPorts", "INTEGER", ro, "Number of ports controlled by this bridging entity."},
			{"1.3.6.1.2.1.17.1.4.1.2", "dot1dBasePortIfIndex", "INTEGER", ro, "ifIndex corresponding to a bridge port number."},
			{"1.3.6.1.2.1.17.2.15.1.3", "dot1dStpPortState", "INTEGER", ro, "STP port state: blocking(2), listening(3), learning(4), forwarding(5)."},
			{"1.3.6.1.2.1.17.4.3.1.1", "dot1dTpFdbAddress", "MacAddress", ro, "A MAC address seen in the filtering database."},
			{"1.3.6.1.2.1.17.4.3.1.2", "dot1dTpFdbPort", "INTEGER", ro, "Bridge port on which the MAC address was last seen."},
		},
	},
	{
		Module: "Q-BRIDGE-MIB", Vendor: VendorStd, Root: "1.3.6.1.2.1.17.7",
		Summary: "802.1Q VLAN configuration and per-port VLAN membership.",
		Objects: []CatObject{
			{"1.3.6.1.2.1.17.7.1.1.4.0", "dot1qNumVlans", "Gauge32", ro, "Number of VLANs currently configured on the device."},
			{"1.3.6.1.2.1.17.7.1.4.3.1.1", "dot1qVlanStaticName", "SnmpAdminString", rc, "Administratively assigned VLAN name."},
			{"1.3.6.1.2.1.17.7.1.4.5.1.1", "dot1qPvid", "VlanIndex", rw, "Port VLAN ID: the VLAN assigned to untagged frames received on a port."},
		},
	},
	{
		Module: "LLDP-MIB", Vendor: VendorStd, Root: "1.0.8802.1.1.2",
		Summary: "Link Layer Discovery Protocol: local identity and the table of directly-connected neighbours.",
		Objects: []CatObject{
			{"1.0.8802.1.1.2.1.3.3.0", "lldpLocSysName", "SnmpAdminString", ro, "System name advertised by this device on its links."},
			{"1.0.8802.1.1.2.1.3.7.1.3", "lldpLocPortId", "LldpPortId", ro, "Port identifier advertised for a local port."},
			{"1.0.8802.1.1.2.1.4.1.1.9", "lldpRemSysName", "SnmpAdminString", ro, "System name of the neighbour seen on a port - the backbone of topology discovery."},
			{"1.0.8802.1.1.2.1.4.1.1.7", "lldpRemPortId", "LldpPortId", ro, "Port identifier the neighbour advertised."},
			{"1.0.8802.1.1.2.1.4.1.1.5", "lldpRemChassisId", "LldpChassisId", ro, "Chassis identifier of the neighbour (often its base MAC)."},
		},
	},
	{
		Module: "POWER-ETHERNET-MIB", Vendor: VendorStd, Root: "1.3.6.1.2.1.105",
		Summary: "802.3af/at Power over Ethernet: per-port PoE admin state and total PSE power draw.",
		Objects: []CatObject{
			{"1.3.6.1.2.1.105.1.1.1.3", "pethPsePortAdminEnable", "TruthValue", rw, "true(1) if PoE is administratively enabled on the port."},
			{"1.3.6.1.2.1.105.1.1.1.6", "pethPsePortDetectionStatus", "INTEGER", ro, "PD detection status: searching(2), deliveringPower(3), fault(4)."},
			{"1.3.6.1.2.1.105.1.3.1.1.4", "pethMainPseUsagePower", "Gauge32", ro, "Total PoE power currently drawn from a PSE, in watts."},
		},
	},
	{
		Module: "UPS-MIB", Vendor: VendorStd, Enterprise: 0, Root: "1.3.6.1.2.1.33",
		Summary: "RFC 1628 vendor-neutral UPS model: battery, input, output and alarm state.",
		Objects: []CatObject{
			{"1.3.6.1.2.1.33.1.1.2.0", "upsIdentModel", "DisplayString", ro, "UPS model designation."},
			{"1.3.6.1.2.1.33.1.2.1.0", "upsBatteryStatus", "INTEGER", ro, "batteryNormal(2), batteryLow(3), batteryDepleted(4)."},
			{"1.3.6.1.2.1.33.1.2.2.0", "upsSecondsOnBattery", "INTEGER", ro, "Seconds the UPS has been running on battery (0 if on utility power)."},
			{"1.3.6.1.2.1.33.1.2.3.0", "upsEstimatedMinutesRemaining", "INTEGER", ro, "Estimated runtime remaining on battery, in minutes."},
			{"1.3.6.1.2.1.33.1.2.4.0", "upsEstimatedChargeRemaining", "INTEGER", ro, "Estimated battery charge remaining, as a percentage."},
			{"1.3.6.1.2.1.33.1.2.5.0", "upsBatteryVoltage", "INTEGER", ro, "Present battery voltage, in 0.1 V dc."},
			{"1.3.6.1.2.1.33.1.2.7.0", "upsBatteryTemperature", "INTEGER", ro, "Ambient temperature at the battery, in degrees Celsius."},
			{"1.3.6.1.2.1.33.1.3.3.1.3", "upsInputVoltage", "INTEGER", ro, "Present input line voltage, in RMS volts."},
			{"1.3.6.1.2.1.33.1.4.4.1.2", "upsOutputVoltage", "INTEGER", ro, "Present output voltage, in RMS volts."},
			{"1.3.6.1.2.1.33.1.4.4.1.5", "upsOutputPercentLoad", "INTEGER", ro, "Percentage of the UPS power capacity presently used on an output line."},
			{"1.3.6.1.2.1.33.1.6.1.0", "upsAlarmsPresent", "Gauge32", ro, "Number of active alarm conditions."},
		},
	},
	{
		Module: "Printer-MIB", Vendor: VendorStd, Enterprise: 0, Root: "1.3.6.1.2.1.43",
		Summary: "RFC 3805 vendor-neutral printer model: identity, page counts, input trays, marker supplies and alerts.",
		Objects: []CatObject{
			{"1.3.6.1.2.1.43.5.1.1.16.1", "prtGeneralPrinterName", "PrtLocalizedDescriptionStringTC", rw, "Administratively assigned printer name."},
			{"1.3.6.1.2.1.43.10.2.1.4.1.1", "prtMarkerLifeCount", "Counter32", ro, "Lifetime page/impression count for a marker (the 'meter')."},
			{"1.3.6.1.2.1.43.8.2.1.10", "prtInputCurrentLevel", "Integer32", ro, "Current media level in an input tray (-3 = unknown, -2 = unlimited)."},
			{"1.3.6.1.2.1.43.11.1.1.9", "prtMarkerSuppliesLevel", "Integer32", ro, "Current level of a supply (toner/ink); compare with prtMarkerSuppliesMaxCapacity."},
			{"1.3.6.1.2.1.43.16.5.1.2.1.1", "prtConsoleDisplayBufferText", "PrtConsoleDescriptionStringTC", ro, "Text currently shown on the printer's front-panel display."},
			{"1.3.6.1.2.1.43.18.1.1.8", "prtAlertDescription", "PrtLocalizedDescriptionStringTC", ro, "Human-readable text for an entry in the printer alert table."},
		},
	},
}

// ------------------------------------------------------------------ net-snmp --

var netSnmpModules = []CatModule{
	{
		Module: "UCD-SNMP-MIB", Vendor: VendorNetSNMP, Enterprise: 2021, Root: "1.3.6.1.4.1.2021",
		Summary: "Net-SNMP's Linux/BSD host metrics: load average, memory, disks, CPU time and the exec/extend tables.",
		Objects: []CatObject{
			{"1.3.6.1.4.1.2021.10.1.3.1", "laLoad.1", "DisplayString", ro, "1-minute load average as a string."},
			{"1.3.6.1.4.1.2021.10.1.3.2", "laLoad.2", "DisplayString", ro, "5-minute load average as a string."},
			{"1.3.6.1.4.1.2021.10.1.3.3", "laLoad.3", "DisplayString", ro, "15-minute load average as a string."},
			{"1.3.6.1.4.1.2021.10.1.5", "laLoadInt", "Integer32", ro, "Load average * 100 as an integer (easier to graph than laLoad)."},
			{"1.3.6.1.4.1.2021.4.5.0", "memTotalReal", "Integer32", ro, "Total physical RAM, in KiB."},
			{"1.3.6.1.4.1.2021.4.6.0", "memAvailReal", "Integer32", ro, "Physical RAM currently free, in KiB."},
			{"1.3.6.1.4.1.2021.4.11.0", "memTotalFree", "Integer32", ro, "Total free memory including swap, in KiB."},
			{"1.3.6.1.4.1.2021.4.13.0", "memShared", "Integer32", ro, "Shared memory, in KiB."},
			{"1.3.6.1.4.1.2021.4.14.0", "memBuffer", "Integer32", ro, "Memory used for kernel buffers, in KiB."},
			{"1.3.6.1.4.1.2021.4.15.0", "memCached", "Integer32", ro, "Memory used for the page cache, in KiB."},
			{"1.3.6.1.4.1.2021.9.1.2", "dskPath", "DisplayString", ro, "Mount point of a monitored filesystem."},
			{"1.3.6.1.4.1.2021.9.1.6", "dskTotal", "Integer32", ro, "Total size of the filesystem, in KiB."},
			{"1.3.6.1.4.1.2021.9.1.7", "dskAvail", "Integer32", ro, "Space available on the filesystem, in KiB."},
			{"1.3.6.1.4.1.2021.9.1.8", "dskUsed", "Integer32", ro, "Space used on the filesystem, in KiB."},
			{"1.3.6.1.4.1.2021.9.1.9", "dskPercent", "Integer32", ro, "Percentage of filesystem space used."},
			{"1.3.6.1.4.1.2021.9.1.10", "dskPercentNode", "Integer32", ro, "Percentage of inodes used."},
			{"1.3.6.1.4.1.2021.11.9.0", "ssCpuUser", "Integer32", ro, "Percentage of CPU time spent in user mode (last sampling interval)."},
			{"1.3.6.1.4.1.2021.11.10.0", "ssCpuSystem", "Integer32", ro, "Percentage of CPU time spent in system/kernel mode."},
			{"1.3.6.1.4.1.2021.11.11.0", "ssCpuIdle", "Integer32", ro, "Percentage of CPU time spent idle."},
			{"1.3.6.1.4.1.2021.11.50.0", "ssCpuRawUser", "Counter32", ro, "Raw ticks of user-mode CPU time since boot (rate-graph this)."},
			{"1.3.6.1.4.1.2021.11.52.0", "ssCpuRawSystem", "Counter32", ro, "Raw ticks of system-mode CPU time since boot."},
			{"1.3.6.1.4.1.2021.11.53.0", "ssCpuRawIdle", "Counter32", ro, "Raw ticks of idle CPU time since boot."},
			{"1.3.6.1.4.1.2021.11.57.0", "ssIORawSent", "Counter32", ro, "Blocks written to disk since boot."},
			{"1.3.6.1.4.1.2021.11.58.0", "ssIORawReceived", "Counter32", ro, "Blocks read from disk since boot."},
			{"1.3.6.1.4.1.2021.11.59.0", "ssRawInterrupts", "Counter32", ro, "Interrupts serviced since boot."},
			{"1.3.6.1.4.1.2021.11.60.0", "ssRawContexts", "Counter32", ro, "Context switches since boot."},
			{"1.3.6.1.4.1.2021.8.1.2", "extNames", "DisplayString", ro, "Name of a configured 'exec'/'extend' script."},
			{"1.3.6.1.4.1.2021.8.1.101", "extOutput", "DisplayString", ro, "First line of output from an exec script."},
			{"1.3.6.1.4.1.2021.8.1.100", "extResult", "Integer32", ro, "Exit code of an exec script."},
		},
	},
	{
		Module: "NET-SNMP-EXTEND-MIB", Vendor: VendorNetSNMP, Enterprise: 8072, Root: "1.3.6.1.4.1.8072.1.3.2",
		Summary: "Output of 'extend' directives - the modern way to expose custom scripts via Net-SNMP.",
		Objects: []CatObject{
			{"1.3.6.1.4.1.8072.1.3.2.3.1.1", "nsExtendOutput1Line", "DisplayString", ro, "First line of stdout from an extend command, indexed by command name."},
			{"1.3.6.1.4.1.8072.1.3.2.3.1.2", "nsExtendOutputFull", "DisplayString", ro, "Complete multi-line stdout from an extend command."},
			{"1.3.6.1.4.1.8072.1.3.2.3.1.4", "nsExtendResult", "Integer32", ro, "Exit status of an extend command."},
			{"1.3.6.1.4.1.8072.1.3.2.4.1.2", "nsExtendOutLine", "DisplayString", ro, "One line of an extend command's output (line-indexed table)."},
		},
	},
	{
		Module: "LM-SENSORS-MIB", Vendor: VendorNetSNMP, Enterprise: 2021, Root: "1.3.6.1.4.1.2021.13.16",
		Summary: "lm-sensors bridge: temperature, fan, voltage and misc sensor readings from the host's hwmon.",
		Objects: []CatObject{
			{"1.3.6.1.4.1.2021.13.16.2.1.2", "lmTempSensorsDevice", "DisplayString", ro, "Name of a temperature sensor as reported by lm-sensors."},
			{"1.3.6.1.4.1.2021.13.16.2.1.3", "lmTempSensorsValue", "Gauge32", ro, "Temperature reading in milli-degrees Celsius (divide by 1000)."},
			{"1.3.6.1.4.1.2021.13.16.3.1.3", "lmFanSensorsValue", "Gauge32", ro, "Fan speed in RPM."},
			{"1.3.6.1.4.1.2021.13.16.4.1.3", "lmVoltSensorsValue", "Gauge32", ro, "Voltage reading in milli-volts (divide by 1000)."},
		},
	},
}
