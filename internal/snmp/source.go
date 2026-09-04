// Package snmp provides a small abstraction over an SNMP agent so the TUI can
// talk to a real device (via gosnmp) or a synthetic demo agent through one
// interface.
package snmp

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Kind classifies a value for display / graphing decisions.
type Kind int

const (
	KindUnknown Kind = iota
	KindInteger
	KindCounter // Counter32 / Counter64 - monotonic, graph as rate
	KindGauge   // Gauge32 - absolute, graph as-is
	KindTimeTicks
	KindString
	KindOID
	KindIPAddress
	KindBytes
	KindNoSuchObject
)

func (k Kind) String() string {
	switch k {
	case KindInteger:
		return "Integer"
	case KindCounter:
		return "Counter"
	case KindGauge:
		return "Gauge"
	case KindTimeTicks:
		return "TimeTicks"
	case KindString:
		return "String"
	case KindOID:
		return "OID"
	case KindIPAddress:
		return "IPAddress"
	case KindBytes:
		return "Bytes"
	case KindNoSuchObject:
		return "n/a"
	default:
		return "?"
	}
}

// Numeric reports whether the kind can be plotted on a graph.
func (k Kind) Numeric() bool {
	switch k {
	case KindInteger, KindCounter, KindGauge, KindTimeTicks:
		return true
	default:
		return false
	}
}

// Var is a single resolved SNMP variable binding.
type Var struct {
	OID  string // dotted numeric, no leading dot
	Kind Kind
	Raw  any       // original typed value (int64, uint64, string, []byte, ...)
	Num  float64   // best-effort numeric projection (valid when Kind.Numeric())
	Str  string    // display string
	Time time.Time // when sampled
}

// Display returns the human-facing value string.
func (v Var) Display() string {
	if v.Str != "" {
		return v.Str
	}
	if v.Kind.Numeric() {
		return trimFloat(v.Num)
	}
	return fmt.Sprintf("%v", v.Raw)
}

func trimFloat(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%.2f", f)
}

// Source is anything that can answer SNMP queries.
type Source interface {
	// Connect establishes / validates the session.
	Connect() error
	// Close releases resources.
	Close() error
	// Get retrieves the given OIDs (chunked internally as needed).
	Get(oids []string) ([]Var, error)
	// Walk performs a subtree walk rooted at oid (GETBULK where possible).
	Walk(oid string) ([]Var, error)
	// Target is a short "host:port v2c" style descriptor.
	Target() string
	// Describe is a longer multi-field connection string for the header.
	Describe() string
}

// SortVars orders bindings by numeric OID.
func SortVars(vs []Var) {
	sort.Slice(vs, func(i, j int) bool { return OIDLess(vs[i].OID, vs[j].OID) })
}

// OIDLess compares two dotted OIDs component-by-component numerically.
func OIDLess(a, b string) bool {
	as := strings.Split(strings.TrimPrefix(a, "."), ".")
	bs := strings.Split(strings.TrimPrefix(b, "."), ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x := atoiSafe(as[i])
		y := atoiSafe(bs[i])
		if x != y {
			return x < y
		}
	}
	return len(as) < len(bs)
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}
