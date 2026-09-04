<p align="center">
  <img src="assets/banner.png" alt="snmpdigger — SNMP DIGGER" width="820">
</p>

<h1 align="center">snmpdigger</h1>

<p align="center">
  <em>DISCOVER &middot; GRAPH &middot; INFORM — SNMP devices. Deeper insights.</em><br>
  <em>A fully keyboard-driven terminal UI that walks, identifies, graphs and hunts SNMP agents.</em><br>
  <a href="https://github.com/kawaiipantsu/snmpdigger"><strong>github.com/kawaiipantsu/snmpdigger</strong></a>
</p>

<p align="center">
  <img alt="Go 1.27+" src="https://img.shields.io/badge/Go-1.27%2B-00add8">
  <img alt="TUI: Bubble Tea" src="https://img.shields.io/badge/TUI-Bubble%20Tea-d75fd7">
  <img alt="SNMP v1 v2c v3" src="https://img.shields.io/badge/SNMP-v1%20%C2%B7%20v2c%20%C2%B7%20v3-1f7a8c">
  <img alt="single binary" src="https://img.shields.io/badge/binary-single%20%C2%B7%20static-35c98b">
  <img alt="platform linux deb" src="https://img.shields.io/badge/platform-linux%20%C2%B7%20deb-0e1013">
  <img alt="license MIT" src="https://img.shields.io/badge/license-MIT-e2223b">
</p>

---

Run `snmpdigger` with no arguments and it boots straight into a full-screen TUI:
a square terminal-window logo top-left, an extended-width header that keeps the
live connection string in view, a tab bar, and a one-line footer with a spinner,
progress bars and a ticking clock. It opens on the **Discovery** tab — nothing is
forced, so you can scan a range or browse the offline MIB catalog before you
connect to anything. Press `c` (or pick a discovered host) for the connection
dialog — host, port, **v1 / v2c / v3**, community string or the whole v3
user / security-level / auth / priv credential set — or run with `--demo` and the
app talks to a synthetic agent so you can wander the entire interface with no
device on the wire.

From there: it **walks** the MIB tree and lets you browse every object by
numeric OID *and* human name, live-polling values every 1–5 seconds. It
**identifies** what it is talking to — vendor from the enterprise number, device
role, decoded OSI service layers, and an analysis panel that calls out an empty
`sysContact` or a kernel string leaking through `sysDescr`. It **graphs** any
counter or gauge live as a braille line chart, bars, a sparkline, a gauge, a
huge big-number readout or a heatmap. And it **discovers** — point it at a CIDR
range and it sweeps the network for anything that answers SNMP.

> **SIMPLE PROTOCOL. MASSIVE EXPOSURE.**

<p align="center">
  <code>snmpdigger</code> &nbsp;|&nbsp; <code>snmpdigger --demo</code> &nbsp;|&nbsp; <code>snmpdigger discover 192.168.1.0/24</code>
</p>

<p align="center">
  <img src="assets/screen-browser.png" width="49%" alt="Browser tab — walking the MIB tree, live values">
  <img src="assets/screen-graph.png" width="49%" alt="Graph tab — live braille line chart of a counter">
</p>
<p align="center">
  <img src="assets/screen-catalog.png" width="49%" alt="Catalog tab — offline MIB module and object reference">
  <img src="assets/screen-discovery.png" width="49%" alt="Discovery tab — CIDR sweep for SNMP agents">
</p>

## What's in the box

| | |
|---|---|
| **Boot** | no args &rarr; alt-screen TUI, opening on the Discovery tab — the connection dialog is never forced. `c` opens it (pre-filled from a discovered row); `--demo` swaps in a synthetic live agent, no device required. |
| **System** | pulls the system group and *fingerprints* the host: vendor from the `sysObjectID` enterprise number, best-effort role (Cisco / MikroTik / Juniper / Fortinet / printer / UPS / NAS / hypervisor / Linux / Windows / …), decoded `sysServices` OSI layers, uptime, `sysContact` / `sysLocation` / admin — plus an analysis panel flagging missing contacts and fingerprint leaks. Live-updates. |
| **Browser** | walks the MIB/OID tree on connect; table of **numeric OID + human name + type + value + age**, polled every 1–5 s. `/` fuzzy search &middot; `f` filter by type (counter / gauge / string / …) &middot; `s` `S` sort + direction &middot; `enter` drill into a subtree &middot; `backspace` up &middot; `g` throw the selected object at the Graph tab. |
| **Graph** | pick any numeric OID, watch it live. Chart types: **line** (braille), **bars**, **sparkline**, **gauge**, huge **big-number**, **heatmap**. `t` cycles type &middot; `d` toggles raw vs per-second rate for counters &middot; `o` opens a fuzzy object picker. |
| **Discovery** | the default landing tab. Feed it an IP **CIDR range**; it sweeps the range for SNMP agents, probing multiple community strings, and lists what answered — IP, device, version, uptime, `sysName`, `sysDescr`. `enter` on a row opens the connection dialog **pre-filled** with what the scan learned, so you just add credentials and connect. Head-less too: `snmpdigger discover 192.168.1.0/24`. |
| **Catalog** | an offline reference of known MIB modules and their notable objects — the full IETF/standard set (SNMPv2-MIB, IF-MIB, IP/TCP/UDP-MIB, HOST-RESOURCES-MIB, ENTITY-MIB, BRIDGE/Q-BRIDGE, LLDP, UPS-MIB, Printer-MIB…) plus the big firewall and network vendors (Cisco, Juniper, MikroTik, Fortinet, Palo Alto, Check Point, SonicWall, Sophos, WatchGuard, Arista, HPE/Aruba, Huawei, Nokia, Extreme). Browse by vendor, search across everything, `enter` to walk that subtree on the live device or `g` to graph it. Fetch additional vendor MIBs on demand into `~/.config/snmpdigger/mibs`. |
| **Settings** | poll interval, timeouts, retries, GETBULK tuning, graph history depth, theme (`thugs` light-grey/cyan/dark · `ember` · `matrix` · `mono`), default walk scope, secret masking. Persisted to `~/.config/snmpdigger/config.yaml` (XDG-aware). |
| **CLI** | `discover` &middot; `walk` &middot; `get` &middot; `identify` &middot; `config path` &middot; `version` — the TUI's engine without the screen. Shared flags: `--community`, `--version v1\|v2c\|v3`, `--port`, `--demo`, and the v3 set `--user --level --auth-proto --auth-pass --priv-proto --priv-pass --context`. |
| **Under it** | **Go 1.27**, `CGO_ENABLED=0`, one static binary. TUI on [Bubble Tea](https://github.com/charmbracelet/bubbletea) / Bubbles / Lipgloss; SNMP via [gosnmp](https://github.com/gosnmp/gosnmp). Zero runtime deps — net-snmp's `snmptranslate` is *optional* and only used to enrich MIB names when present. |

## Quick start

```bash
git clone https://github.com/kawaiipantsu/snmpdigger.git && cd snmpdigger
make build

./bin/snmpdigger --demo     # kick the tyres with a synthetic agent, no device needed
./bin/snmpdigger            # for real — drops the SNMP connection dialog on launch
```

Head-less, straight from the shell:

```bash
snmpdigger discover 192.168.1.0/24
snmpdigger identify 10.0.0.1 --community public
snmpdigger walk 10.0.0.1 1.3.6.1.2.1.2.2         # ifTable
snmpdigger get  10.0.0.1 1.3.6.1.2.1.1.3.0       # sysUpTime
snmpdigger walk 10.0.0.1 --version v3 --user monitor \
  --level authPriv --auth-proto SHA --auth-pass '***' \
  --priv-proto AES --priv-pass '***'
```

Shared flags: `--community` &middot; `--version v1|v2c|v3` &middot; `--port` &middot; `--demo`,
plus the v3 set `--user --level --auth-proto --auth-pass --priv-proto --priv-pass --context`.

### Packages

```bash
make cross            # linux/amd64, linux/386, linux/armhf, linux/arm64  -> dist/
make deb              # .deb for all four arches via dpkg-deb             -> dist/deb/
make dist             # cross + deb + SHA256SUMS

sudo make install                 # /usr/local/bin/snmpdigger
sudo dpkg -i dist/deb/*.deb        # or the Debian way
```

## How it works

One Bubble Tea model owns the screen. A poll ticker fires on your configured
interval and issues SNMP `GET`s for exactly the OIDs the active tab cares about;
the Browser walk and the Discovery sweep run as cancellable background jobs that
stream progress back into the footer. MIB names resolve from a built-in table of
the standard trees, with `snmptranslate` consulted only for the leftovers.
Charts are hand-rolled — braille rasterisation for the line chart, block-glyph
columns for bars and heatmap, a 5-row block font for the big-number readout — so
there is nothing to render but text.

The chrome is fixed: a square terminal-window logo top-left, an extended-width
connection header top-right (target, identified host, decoded role), the tab bar,
the active view, and a one-line footer — live status and progress on the left,
`snmpdigger <version> (c) 2026 ` + **THUGS** + a ticking clock on the right.

<p align="center">
  <img src="assets/screen-system.png" width="88%" alt="System tab — device identity, ownership, sysDescr and analysis findings">
</p>

## Layout

```
main.go                  entry point: no args -> TUI, else CLI dispatch
internal/
  cli/                   head-less subcommands (discover · walk · get · identify · …)
  config/                ~/.config/snmpdigger/config.yaml load/save (XDG-aware)
  mib/                   OID <-> name resolver: built-in tree + optional snmptranslate
  snmp/                  gosnmp transport, walk/get, system-group identify, CIDR sweep
  tui/                   Bubble Tea model, chrome, tabs and the five views
  tui/charts/            braille line · bars · sparkline · gauge · big-number · heatmap
packaging/               Debian control template, man page, copyright
assets/                  banner + brand art
Makefile                 build · cross · deb · dist · install
```

## Docs

[Overview](docs/overview.md) &middot;
[Keys &amp; hotkeys](docs/keys.md) &middot;
[CLI reference](docs/cli.md) &middot;
[Build &amp; packaging](docs/build.md)

## License

MIT — see [LICENSE](LICENSE).

<p align="center"><sub>built for <a href="https://thugs.red">thugs.red</a> &middot; SIMPLE PROTOCOL. MASSIVE EXPOSURE.</sub></p>
