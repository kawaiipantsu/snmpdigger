package snmp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ASNInfo is a resolved autonomous system.
type ASNInfo struct {
	ASN      string
	Holder   string
	Prefixes []string // IPv4 CIDRs announced by the AS
}

// TotalHosts is the (arithmetic) number of usable addresses across all prefixes.
func (a ASNInfo) TotalHosts() int64 {
	var n int64
	for _, p := range a.Prefixes {
		c, err := cidrHostCount(p)
		if err == nil {
			n += c
		}
	}
	return n
}

// ResolveASN looks up the IPv4 prefixes announced by an AS number using the
// public RIPEstat data API (no key required). Input may be "AS13335", "as13335"
// or "13335".
func ResolveASN(ctx context.Context, asn string) (ASNInfo, error) {
	num := strings.TrimSpace(asn)
	num = strings.TrimPrefix(strings.ToUpper(num), "AS")
	if num == "" || !isDigits(num) {
		return ASNInfo{}, fmt.Errorf("%q is not an AS number", asn)
	}

	info := ASNInfo{ASN: "AS" + num}

	// 1) announced prefixes
	var pfx struct {
		Data struct {
			Prefixes []struct {
				Prefix string `json:"prefix"`
			} `json:"prefixes"`
		} `json:"data"`
	}
	if err := ripestat(ctx, "announced-prefixes", num, &pfx); err != nil {
		return ASNInfo{}, err
	}
	seen := map[string]struct{}{}
	for _, p := range pfx.Data.Prefixes {
		cidr := strings.TrimSpace(p.Prefix)
		if cidr == "" || strings.Contains(cidr, ":") { // IPv4 only
			continue
		}
		if _, dup := seen[cidr]; dup {
			continue
		}
		seen[cidr] = struct{}{}
		info.Prefixes = append(info.Prefixes, cidr)
	}
	sort.Slice(info.Prefixes, func(i, j int) bool { return OIDLess(info.Prefixes[i], info.Prefixes[j]) })

	// 2) holder name (best effort, non-fatal)
	var meta struct {
		Data struct {
			Holder string `json:"holder"`
		} `json:"data"`
	}
	if err := ripestat(ctx, "as-overview", num, &meta); err == nil {
		info.Holder = meta.Data.Holder
	}

	if len(info.Prefixes) == 0 {
		return info, fmt.Errorf("AS%s announces no IPv4 prefixes (or lookup returned nothing)", num)
	}
	return info, nil
}

func ripestat(ctx context.Context, endpoint, resource string, out any) error {
	url := fmt.Sprintf("https://stat.ripe.net/data/%s/data.json?resource=AS%s&sourceapp=snmpdigger", endpoint, resource)
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "snmpdigger/asn-lookup")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ASN lookup (%s): %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ASN lookup (%s): HTTP %d", endpoint, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
