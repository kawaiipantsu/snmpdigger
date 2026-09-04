# Overview

`snmpdigger` is a single static Go binary that is a full terminal UI first and a
head-less SNMP toolkit second. Launched with no arguments it opens an alt-screen
Bubble Tea interface and immediately prompts for a connection (host, port,
v1/v2c/v3 credentials) or an offline demo agent. Five tabs — **System**,
**Browser**, **Graph**, **Discovery**, **Settings** — cover device
identification, live MIB/OID browsing with search and filtering, live charts of
any counter or gauge, CIDR-range agent discovery, and persisted configuration
under `~/.config/snmpdigger/config.yaml`. Everything the TUI does is also
reachable as a plain subcommand for scripts and pipelines.
