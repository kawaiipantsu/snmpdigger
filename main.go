// Command snmpdigger is a TUI-first SNMP discovery, browsing and live-graphing
// tool. Run with no arguments to launch the fullscreen interface; run with a
// subcommand (discover, walk, get, identify, config, version) for one-shot use.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kawaiipantsu/snmpdigger/internal/cli"
	"github.com/kawaiipantsu/snmpdigger/internal/config"
	"github.com/kawaiipantsu/snmpdigger/internal/tui"
)

func main() {
	// A bare first argument that is not a flag is treated as a subcommand.
	if len(os.Args) >= 2 && !strings.HasPrefix(os.Args[1], "-") {
		os.Exit(cli.Main(os.Args))
	}
	// Otherwise: flags (or nothing) -> launch the TUI.
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "-h", "-help", "--help":
			os.Exit(cli.Main([]string{os.Args[0], "help"}))
		case "-v", "-version", "--version":
			os.Exit(cli.Main([]string{os.Args[0], "version"}))
		}
	}
	os.Exit(runTUI(os.Args[1:]))
}

func runTUI(argv []string) int {
	fs := flag.NewFlagSet("snmpdigger", flag.ContinueOnError)
	demo := fs.Bool("demo", false, "run the TUI against a synthetic in-process SNMP agent")
	host := fs.String("host", "", "connect to this host on startup, skipping the connection dialog")
	spec := cli.ConnFlags(fs)
	if err := fs.Parse(argv); err != nil {
		return 2
	}

	opts := tui.Options{Demo: *demo}
	if strings.TrimSpace(*host) != "" {
		var conn config.Connection = spec.Connection(*host)
		opts.AutoConnect = &conn
	}

	if err := tui.Run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "snmpdigger: %v\n", err)
		return 1
	}
	return 0
}
