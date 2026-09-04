# Build & packaging

Requires Go 1.27+. All builds are `CGO_ENABLED=0` and produce a single static
binary with version metadata stamped via `-ldflags -X`.

| Target | Result |
|---|---|
| `make build` | native binary at `bin/snmpdigger` |
| `make run` | `go run` the app |
| `make test` / `make vet` | tests and `go vet` |
| `make cross` | `dist/` binaries for linux `amd64`, `386`, `armhf` (GOARM=7), `arm64` |
| `make deb` | one `.deb` per arch in `dist/deb/`, assembled with `dpkg-deb` |
| `make dist` | `cross` + `deb` + `SHA256SUMS` |
| `sudo make install` | copy to `/usr/local/bin` (`make uninstall` to remove) |
| `make clean` | wipe `bin/` and `dist/` |

Install a package directly with `sudo dpkg -i dist/deb/*.deb`. No runtime
dependencies; `snmptranslate` (net-snmp) is used for extra MIB name resolution
only if it happens to be on `PATH`.
