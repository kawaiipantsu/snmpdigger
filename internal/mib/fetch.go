package mib

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kawaiipantsu/snmpdigger/internal/config"
)

// MIBCacheDir is where downloaded raw MIB modules are stored
// ($XDG_CONFIG_HOME/snmpdigger/mibs). It is created if missing.
func MIBCacheDir() (string, error) {
	base, err := config.Dir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "mibs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// RemoteMIB is one downloadable MIB module.
type RemoteMIB struct {
	Vendor string
	Module string
	URL    string
}

const remoteBase = "https://mibs.observium.org/mib/"

// RemoteIndex is a curated list of well-known raw-MIB URLs for the brands the
// offline Catalog() covers. The base mirror (mibs.observium.org) serves plain
// text files named after the module. Best-effort - some may 404 over time.
func RemoteIndex() []RemoteMIB {
	mods := []struct{ vendor, module string }{
		// standard
		{"IETF / Standard", "SNMPv2-MIB"},
		{"IETF / Standard", "IF-MIB"},
		{"IETF / Standard", "IP-MIB"},
		{"IETF / Standard", "IP-FORWARD-MIB"},
		{"IETF / Standard", "TCP-MIB"},
		{"IETF / Standard", "UDP-MIB"},
		{"IETF / Standard", "HOST-RESOURCES-MIB"},
		{"IETF / Standard", "ENTITY-MIB"},
		{"IETF / Standard", "ENTITY-SENSOR-MIB"},
		{"IETF / Standard", "BRIDGE-MIB"},
		{"IETF / Standard", "Q-BRIDGE-MIB"},
		{"IETF / Standard", "LLDP-MIB"},
		{"IETF / Standard", "POWER-ETHERNET-MIB"},
		{"IETF / Standard", "UPS-MIB"},
		{"IETF / Standard", "Printer-MIB"},
		// net-snmp
		{"Net-SNMP / UCD", "UCD-SNMP-MIB"},
		{"Net-SNMP / UCD", "NET-SNMP-MIB"},
		{"Net-SNMP / UCD", "NET-SNMP-EXTEND-MIB"},
		{"Net-SNMP / UCD", "LM-SENSORS-MIB"},
		// firewalls
		{"Fortinet", "FORTINET-FORTIGATE-MIB"},
		{"Fortinet", "FORTINET-CORE-MIB"},
		{"Palo Alto Networks", "PAN-COMMON-MIB"},
		{"Cisco", "CISCO-FIREWALL-MIB"},
		{"SonicWall", "SONICWALL-FIREWALL-MIB"},
		{"Check Point", "CHECKPOINT-MIB"},
		{"WatchGuard", "WATCHGUARD-SYSTEM-STATISTICS-MIB"},
		{"Sophos", "SFOS-FIREWALL-MIB"},
		{"Juniper Networks", "JUNIPER-SRX5000-SPU-MONITORING-MIB"},
		// routing / switching
		{"Cisco", "CISCO-PROCESS-MIB"},
		{"Cisco", "CISCO-MEMORY-POOL-MIB"},
		{"Cisco", "CISCO-ENVMON-MIB"},
		{"Cisco", "CISCO-CDP-MIB"},
		{"Juniper Networks", "JUNIPER-MIB"},
		{"MikroTik", "MIKROTIK-MIB"},
		{"Huawei", "HUAWEI-ENTITY-EXTENT-MIB"},
		{"Arista Networks", "ARISTA-GENERAL-MIB"},
		{"Extreme Networks", "EXTREME-SOFTWARE-MONITOR-MIB"},
		{"Nokia (Alcatel-Lucent SR)", "TIMETRA-CHASSIS-MIB"},
	}
	out := make([]RemoteMIB, 0, len(mods))
	for _, m := range mods {
		out = append(out, RemoteMIB{
			Vendor: m.vendor,
			Module: m.module,
			URL:    remoteBase + m.module + ".txt",
		})
	}
	return out
}

// SearchRemote filters RemoteIndex by a case-insensitive substring over the
// vendor and module names.
func SearchRemote(q string) []RemoteMIB {
	q = strings.ToLower(strings.TrimSpace(q))
	all := RemoteIndex()
	if q == "" {
		return all
	}
	var out []RemoteMIB
	for _, r := range all {
		if strings.Contains(strings.ToLower(r.Module), q) ||
			strings.Contains(strings.ToLower(r.Vendor), q) {
			out = append(out, r)
		}
	}
	return out
}

// DownloadMIB fetches r.URL and writes it to <cache>/<Module>.mib. It does not
// parse the file. The context bounds the whole request.
func DownloadMIB(ctx context.Context, r RemoteMIB) (string, error) {
	dir, err := MIBCacheDir()
	if err != nil {
		return "", err
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "snmpdigger/mib-fetch")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: HTTP %d", r.URL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8 MiB cap
	if err != nil {
		return "", err
	}
	if len(body) == 0 {
		return "", fmt.Errorf("%s: empty response", r.URL)
	}

	path := filepath.Join(dir, r.Module+".mib")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

// CachedMIBs lists the basenames (without extension) of *.mib / *.txt files
// already present in the MIB cache directory.
func CachedMIBs() []string {
	dir, err := MIBCacheDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".mib" && ext != ".txt" {
			continue
		}
		out = append(out, strings.TrimSuffix(name, filepath.Ext(name)))
	}
	sort.Strings(out)
	return out
}
