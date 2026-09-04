// Package version carries build-time identification for snmpdigger.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// These are overridden at build time via -ldflags "-X ...".
var (
	// Version is the semantic version (e.g. v0.1.0 or v0.1.0-3-gabc123-dirty).
	Version = "dev"
	// Commit is the short git SHA.
	Commit = "none"
	// Date is the RFC3339 build timestamp.
	Date = "unknown"
)

// Name is the canonical program name.
const Name = "snmpdigger"

// Full returns a one-line human readable version string.
func Full() string {
	return fmt.Sprintf("%s %s (commit %s, built %s, %s/%s, %s)",
		Name, Version, Commit, Date, runtime.GOOS, runtime.GOARCH, runtime.Version())
}

// Short returns just the version token, falling back to the module version that
// `go install` stamps when -ldflags were not supplied.
func Short() string {
	if Version != "dev" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return Version
}
