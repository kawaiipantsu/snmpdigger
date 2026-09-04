# CLI reference

With no arguments `snmpdigger` starts the TUI. With a subcommand it runs
head-less and prints to stdout.

| Command | Purpose |
|---|---|
| `snmpdigger discover <cidr>` | sweep an IP range for SNMP agents and list responders |
| `snmpdigger walk <host> [oid]` | GETBULK/GETNEXT walk a subtree (default: `mib-2`) |
| `snmpdigger get <host> <oid...>` | GET one or more specific OIDs |
| `snmpdigger identify <host>` | fetch and analyse the system group (vendor, role, layers) |
| `snmpdigger config path` | print the resolved config file location |
| `snmpdigger version` | print version, commit and build info |

**Shared flags:** `--community <str>` &middot; `--version v1|v2c|v3` &middot;
`--port <n>` (default 161) &middot; `--demo` (use the synthetic agent).

**SNMPv3 flags:** `--user` &middot; `--level noAuthNoPriv|authNoPriv|authPriv` &middot;
`--auth-proto MD5|SHA|SHA224|SHA256|SHA384|SHA512` &middot; `--auth-pass` &middot;
`--priv-proto DES|AES|AES192|AES256` &middot; `--priv-pass` &middot; `--context`.
