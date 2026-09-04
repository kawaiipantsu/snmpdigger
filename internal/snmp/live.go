package snmp

import (
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	g "github.com/gosnmp/gosnmp"

	"github.com/kawaiipantsu/snmpdigger/internal/config"
)

// Live is a Source backed by a real SNMP agent through gosnmp.
type Live struct {
	conn config.Connection
	poll config.Poll

	mu     sync.Mutex
	client *g.GoSNMP
	isOpen bool
}

// NewLive builds a Live source from a connection profile and poll tuning.
func NewLive(conn config.Connection, poll config.Poll) (*Live, error) {
	if strings.TrimSpace(conn.Host) == "" {
		return nil, fmt.Errorf("host is required")
	}
	if conn.Port == 0 {
		conn.Port = 161
	}
	l := &Live{conn: conn, poll: poll}
	client, err := l.build()
	if err != nil {
		return nil, err
	}
	l.client = client
	return l, nil
}

func (l *Live) build() (*g.GoSNMP, error) {
	c := &g.GoSNMP{
		Target:         l.conn.Host,
		Port:           uint16(l.conn.Port),
		Timeout:        time.Duration(max(l.poll.TimeoutSeconds, 1)) * time.Second,
		Retries:        max(l.poll.Retries, 0),
		MaxOids:        max(l.poll.MaxOIDsPerReq, 1),
		MaxRepetitions: uint32(max(l.poll.MaxRepetitions, 1)),
	}

	switch strings.ToLower(l.conn.Version) {
	case "v1", "1":
		c.Version = g.Version1
		c.Community = l.conn.Community
	case "v3", "3":
		c.Version = g.Version3
		c.SecurityModel = g.UserSecurityModel
		usm := &g.UsmSecurityParameters{UserName: l.conn.Username}
		switch strings.ToLower(l.conn.SecLevel) {
		case "noauthnopriv", "noauth", "":
			c.MsgFlags = g.NoAuthNoPriv
		case "authnopriv":
			c.MsgFlags = g.AuthNoPriv
			usm.AuthenticationProtocol = authProto(l.conn.AuthProto)
			usm.AuthenticationPassphrase = l.conn.AuthPass
		default: // authPriv
			c.MsgFlags = g.AuthPriv
			usm.AuthenticationProtocol = authProto(l.conn.AuthProto)
			usm.AuthenticationPassphrase = l.conn.AuthPass
			usm.PrivacyProtocol = privProto(l.conn.PrivProto)
			usm.PrivacyPassphrase = l.conn.PrivPass
		}
		c.SecurityParameters = usm
		if l.conn.ContextName != "" {
			c.ContextName = l.conn.ContextName
		}
	default: // v2c
		c.Version = g.Version2c
		c.Community = l.conn.Community
	}
	return c, nil
}

func authProto(s string) g.SnmpV3AuthProtocol {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "MD5":
		return g.MD5
	case "SHA", "SHA1":
		return g.SHA
	case "SHA224":
		return g.SHA224
	case "SHA256":
		return g.SHA256
	case "SHA384":
		return g.SHA384
	case "SHA512":
		return g.SHA512
	default:
		return g.NoAuth
	}
}

func privProto(s string) g.SnmpV3PrivProtocol {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DES":
		return g.DES
	case "AES", "AES128":
		return g.AES
	case "AES192":
		return g.AES192
	case "AES256":
		return g.AES256
	case "AES192C":
		return g.AES192C
	case "AES256C":
		return g.AES256C
	default:
		return g.NoPriv
	}
}

// Connect opens the UDP session.
func (l *Live) Connect() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.isOpen {
		return nil
	}
	if err := l.client.Connect(); err != nil {
		return fmt.Errorf("connect %s: %w", l.Target(), err)
	}
	l.isOpen = true
	return nil
}

// Close tears down the session.
func (l *Live) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.isOpen || l.client.Conn == nil {
		return nil
	}
	l.isOpen = false
	return l.client.Conn.Close()
}

// Get retrieves oids, chunked by MaxOids.
func (l *Live) Get(oids []string) ([]Var, error) {
	if len(oids) == 0 {
		return nil, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.isOpen {
		if err := l.client.Connect(); err != nil {
			return nil, err
		}
		l.isOpen = true
	}

	chunk := l.client.MaxOids
	if chunk < 1 {
		chunk = 1
	}
	out := make([]Var, 0, len(oids))
	now := time.Now()
	for i := 0; i < len(oids); i += chunk {
		end := min(i+chunk, len(oids))
		batch := normalizeOIDs(oids[i:end])
		res, err := l.client.Get(batch)
		if err != nil {
			return out, err
		}
		for _, pdu := range res.Variables {
			out = append(out, pduToVar(pdu, now))
		}
	}
	return out, nil
}

// Walk performs a subtree walk.
func (l *Live) Walk(root string) ([]Var, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.isOpen {
		if err := l.client.Connect(); err != nil {
			return nil, err
		}
		l.isOpen = true
	}
	root = normalizeOID(root)
	now := time.Now()
	var (
		pdus []g.SnmpPDU
		err  error
	)
	if l.client.Version == g.Version1 {
		pdus, err = l.client.WalkAll(root)
	} else {
		pdus, err = l.client.BulkWalkAll(root)
	}
	if err != nil {
		return nil, err
	}
	out := make([]Var, 0, len(pdus))
	for _, pdu := range pdus {
		out = append(out, pduToVar(pdu, now))
	}
	SortVars(out)
	return out, nil
}

// Target is a compact descriptor.
func (l *Live) Target() string {
	return fmt.Sprintf("%s:%d %s", l.conn.Host, l.conn.Port, strings.ToLower(l.conn.Version))
}

// Describe returns the header connection string.
func (l *Live) Describe() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s:%d  proto=%s", l.conn.Host, l.conn.Port, strings.ToLower(l.conn.Version))
	switch strings.ToLower(l.conn.Version) {
	case "v3":
		fmt.Fprintf(&b, "  user=%s  level=%s  auth=%s  priv=%s",
			orDash(l.conn.Username), orDash(l.conn.SecLevel), orDash(l.conn.AuthProto), orDash(l.conn.PrivProto))
	default:
		fmt.Fprintf(&b, "  community=%s", orDash(l.conn.Community))
	}
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func normalizeOIDs(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = normalizeOID(s)
	}
	return out
}

func normalizeOID(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, ".") {
		return "." + s
	}
	return s
}

// VarFromPDU converts a raw gosnmp PDU to a Var. Used by the trap listener,
// which receives PDUs outside the normal Get/Walk path.
func VarFromPDU(pdu g.SnmpPDU) Var { return pduToVar(pdu, time.Now()) }

func pduToVar(pdu g.SnmpPDU, now time.Time) Var {
	oid := strings.TrimPrefix(pdu.Name, ".")
	v := Var{OID: oid, Raw: pdu.Value, Time: now}

	switch pdu.Type {
	case g.Integer:
		v.Kind = KindInteger
		v.Num = bigToF(pdu.Value)
	case g.Uinteger32:
		v.Kind = KindInteger
		v.Num = bigToF(pdu.Value)
	case g.Counter32, g.Counter64:
		v.Kind = KindCounter
		v.Num = bigToF(pdu.Value)
	case g.Gauge32:
		v.Kind = KindGauge
		v.Num = bigToF(pdu.Value)
	case g.TimeTicks:
		v.Kind = KindTimeTicks
		v.Num = bigToF(pdu.Value)
		v.Str = humanTicks(v.Num)
	case g.OctetString:
		v.Kind = KindString
		b, _ := pdu.Value.([]byte)
		v.Str = decodeOctetString(b)
	case g.ObjectIdentifier:
		v.Kind = KindOID
		v.Str = strings.TrimPrefix(fmt.Sprintf("%v", pdu.Value), ".")
	case g.IPAddress:
		v.Kind = KindIPAddress
		v.Str = fmt.Sprintf("%v", pdu.Value)
	case g.OpaqueFloat, g.OpaqueDouble:
		v.Kind = KindGauge
		v.Num = bigToF(pdu.Value)
	case g.NoSuchObject, g.NoSuchInstance, g.EndOfMibView:
		v.Kind = KindNoSuchObject
		v.Str = "no such object"
	case g.Null:
		v.Kind = KindNoSuchObject
		v.Str = "null"
	default:
		v.Kind = KindUnknown
		v.Str = fmt.Sprintf("%v", pdu.Value)
	}
	return v
}

func bigToF(val any) float64 {
	switch n := val.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case uint:
		return float64(n)
	case uint64:
		return float64(n)
	case uint32:
		return float64(n)
	case float64:
		return n
	case *big.Int:
		f := new(big.Float).SetInt(n)
		out, _ := f.Float64()
		return out
	default:
		bi := g.ToBigInt(val)
		if bi != nil {
			f := new(big.Float).SetInt(bi)
			out, _ := f.Float64()
			return out
		}
		return 0
	}
}

func decodeOctetString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	printable := true
	for _, c := range b {
		if (c < 0x20 || c > 0x7e) && c != '\t' && c != '\n' && c != '\r' {
			printable = false
			break
		}
	}
	if printable {
		return strings.TrimRight(string(b), "\x00")
	}
	// hex dump for binary payloads (MAC addresses, engine IDs, ...)
	parts := make([]string, len(b))
	for i, c := range b {
		parts[i] = fmt.Sprintf("%02x", c)
	}
	return strings.Join(parts, ":")
}

func humanTicks(ticks float64) string {
	// TimeTicks are hundredths of a second.
	d := time.Duration(ticks) * 10 * time.Millisecond
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %02dh %02dm %02ds", days, h, m, s)
	}
	return fmt.Sprintf("%02dh %02dm %02ds", h, m, s)
}
