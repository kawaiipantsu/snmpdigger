// Package mib resolves numeric OIDs to human readable names. It works entirely
// from a built-in table and, when the net-snmp `snmptranslate` binary is present
// on the host, opportunistically enriches names it does not already know.
package mib

import (
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Resolver caches OID -> name lookups.
type Resolver struct {
	mu            sync.RWMutex
	cache         map[string]string
	haveTranslate bool
	translatePath string
	useTranslate  bool
}

// New builds a Resolver, detecting snmptranslate.
func New(useTranslate bool) *Resolver {
	r := &Resolver{cache: map[string]string{}, useTranslate: useTranslate}
	if p, err := exec.LookPath("snmptranslate"); err == nil {
		r.haveTranslate = true
		r.translatePath = p
	}
	return r
}

// HasTranslate reports whether external MIB translation is available.
func (r *Resolver) HasTranslate() bool { return r.haveTranslate && r.useTranslate }

// Name returns a short label for oid, e.g. "ifInOctets.3" or "sysDescr.0".
// It never returns an empty string - worst case it echoes the numeric OID.
func (r *Resolver) Name(oid string) string {
	oid = strings.TrimPrefix(strings.TrimSpace(oid), ".")
	if oid == "" {
		return ""
	}
	r.mu.RLock()
	if v, ok := r.cache[oid]; ok {
		r.mu.RUnlock()
		return v
	}
	r.mu.RUnlock()

	name := r.resolve(oid)

	r.mu.Lock()
	r.cache[oid] = name
	r.mu.Unlock()
	return name
}

func (r *Resolver) resolve(oid string) string {
	if v, ok := builtin[oid]; ok {
		return v
	}
	// longest-prefix match against the builtin table, appending the trailing
	// numeric instance/index components.
	parts := strings.Split(oid, ".")
	for i := len(parts) - 1; i >= 1; i-- {
		prefix := strings.Join(parts[:i], ".")
		if base, ok := builtin[prefix]; ok {
			suffix := strings.Join(parts[i:], ".")
			return base + "." + suffix
		}
	}
	if r.HasTranslate() {
		if n := r.viaSnmptranslate(oid); n != "" {
			return n
		}
	}
	return oid
}

func (r *Resolver) viaSnmptranslate(oid string) string {
	ctxDone := make(chan struct{})
	var out []byte
	go func() {
		defer close(ctxDone)
		cmd := exec.Command(r.translatePath, "-Of", "."+oid)
		b, err := cmd.Output()
		if err == nil {
			out = b
		}
	}()
	select {
	case <-ctxDone:
	case <-time.After(700 * time.Millisecond):
		return ""
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return ""
	}
	// e.g. .iso.org.dod.internet.mgmt.mib-2.system.sysDescr.0
	segs := strings.Split(strings.TrimPrefix(line, "."), ".")
	// walk from the end, keep trailing numeric instance parts, take the first
	// non-numeric component as the name.
	var trailing []string
	for i := len(segs) - 1; i >= 0; i-- {
		if isNumeric(segs[i]) {
			trailing = append([]string{segs[i]}, trailing...)
			continue
		}
		name := segs[i]
		if name == "" || isNumericPath(name) {
			return ""
		}
		if len(trailing) > 0 {
			return name + "." + strings.Join(trailing, ".")
		}
		return name
	}
	return ""
}

func isNumeric(s string) bool {
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

func isNumericPath(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && c != '.' {
			return false
		}
	}
	return true
}
