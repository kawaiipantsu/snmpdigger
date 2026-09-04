// Package config loads and persists snmpdigger settings and the last used
// connection profile under $XDG_CONFIG_HOME/snmpdigger (default ~/.config/snmpdigger).
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Connection describes how to reach an SNMP agent. It is reused as the
// "last connection" persisted in the config file and as the payload the
// TUI connect dialog produces.
type Connection struct {
	Name    string `yaml:"name,omitempty"`
	Host    string `yaml:"host"`
	Port    uint16 `yaml:"port"`
	Version string `yaml:"version"` // v1 | v2c | v3

	// v1 / v2c
	Community string `yaml:"community,omitempty"`

	// v3
	Username    string `yaml:"username,omitempty"`
	SecLevel    string `yaml:"sec_level,omitempty"`  // noAuthNoPriv | authNoPriv | authPriv
	AuthProto   string `yaml:"auth_proto,omitempty"` // MD5 | SHA | SHA224 | SHA256 | SHA384 | SHA512
	AuthPass    string `yaml:"auth_pass,omitempty"`
	PrivProto   string `yaml:"priv_proto,omitempty"` // DES | AES | AES192 | AES256
	PrivPass    string `yaml:"priv_pass,omitempty"`
	ContextName string `yaml:"context,omitempty"`
}

// Poll controls the live polling engine.
type Poll struct {
	IntervalSeconds int `yaml:"interval_seconds"` // how often live values refresh
	TimeoutSeconds  int `yaml:"timeout_seconds"`  // per-request timeout
	Retries         int `yaml:"retries"`
	MaxOIDsPerReq   int `yaml:"max_oids_per_request"`
	MaxRepetitions  int `yaml:"max_repetitions"` // GETBULK tuning
}

// UI controls presentation.
type UI struct {
	Theme            string `yaml:"theme"`              // thugs | mono | matrix
	ShowNumericOID   bool   `yaml:"show_numeric_oid"`   // browser: show dotted OID column
	GraphHistory     int    `yaml:"graph_history"`      // samples kept per graphed OID
	DefaultWalkScope string `yaml:"default_walk_scope"` // mib-2 | enterprises | whole
	MaskSecrets      bool   `yaml:"mask_secrets"`       // hide community/passwords in header
}

// Config is the whole persisted document.
type Config struct {
	Poll   Poll         `yaml:"poll"`
	UI     UI           `yaml:"ui"`
	Last   Connection   `yaml:"last_connection"`
	Recent []Connection `yaml:"recent,omitempty"`
	Watch  []WatchEntry `yaml:"watch,omitempty"`

	// path is the resolved config file location; not serialized.
	path string
}

// WatchEntry is a single object pinned to the Watch tab.
type WatchEntry struct {
	OID  string `yaml:"oid"`
	Name string `yaml:"name,omitempty"`
	Rate bool   `yaml:"rate,omitempty"` // show per-second delta
}

// Default returns a fresh config with sane values.
func Default() *Config {
	return &Config{
		Poll: Poll{
			IntervalSeconds: 3,
			TimeoutSeconds:  2,
			Retries:         1,
			MaxOIDsPerReq:   40,
			MaxRepetitions:  20,
		},
		UI: UI{
			Theme:            "thugs",
			ShowNumericOID:   true,
			GraphHistory:     120,
			DefaultWalkScope: "mib-2",
			MaskSecrets:      true,
		},
		Last: Connection{
			Host:      "",
			Port:      161,
			Version:   "v2c",
			Community: "public",
			SecLevel:  "authPriv",
			AuthProto: "SHA",
			PrivProto: "AES",
		},
	}
}

// Dir is the directory holding the config file.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "snmpdigger"), nil
}

// Path is the full path to config.yaml.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.yaml"), nil
}

// Load reads the config file, creating it with defaults if absent.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	cfg := Default()
	cfg.path = p

	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		if werr := cfg.Save(); werr != nil {
			return cfg, fmt.Errorf("writing initial config: %w", werr)
		}
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", p, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", p, err)
	}
	cfg.path = p
	cfg.normalize()
	return cfg, nil
}

// Save writes the config back to disk atomically-ish (write temp, rename).
func (c *Config) Save() error {
	if c.path == "" {
		p, err := Path()
		if err != nil {
			return err
		}
		c.path = p
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	c.normalize()
	out, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// RememberConnection stores conn as Last and pushes it onto the de-duplicated
// recent list (most recent first, capped at 10).
func (c *Config) RememberConnection(conn Connection) {
	c.Last = conn
	key := func(x Connection) string {
		return fmt.Sprintf("%s|%d|%s|%s|%s", x.Host, x.Port, x.Version, x.Community, x.Username)
	}
	k := key(conn)
	out := []Connection{conn}
	for _, r := range c.Recent {
		if key(r) == k {
			continue
		}
		out = append(out, r)
		if len(out) >= 10 {
			break
		}
	}
	c.Recent = out
}

func (c *Config) normalize() {
	if c.Poll.IntervalSeconds < 1 {
		c.Poll.IntervalSeconds = 1
	}
	if c.Poll.IntervalSeconds > 3600 {
		c.Poll.IntervalSeconds = 3600
	}
	if c.Poll.TimeoutSeconds < 1 {
		c.Poll.TimeoutSeconds = 1
	}
	if c.Poll.Retries < 0 {
		c.Poll.Retries = 0
	}
	if c.Poll.MaxOIDsPerReq < 1 {
		c.Poll.MaxOIDsPerReq = 1
	}
	if c.Poll.MaxRepetitions < 1 {
		c.Poll.MaxRepetitions = 10
	}
	if c.UI.GraphHistory < 10 {
		c.UI.GraphHistory = 10
	}
	if c.UI.GraphHistory > 5000 {
		c.UI.GraphHistory = 5000
	}
	if c.Last.Port == 0 {
		c.Last.Port = 161
	}
	if c.Last.Version == "" {
		c.Last.Version = "v2c"
	}
	if c.UI.Theme == "" {
		c.UI.Theme = "thugs"
	}
	if c.UI.DefaultWalkScope == "" {
		c.UI.DefaultWalkScope = "mib-2"
	}
}
