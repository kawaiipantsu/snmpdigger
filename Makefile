# snmpdigger - build environment
# ------------------------------------------------------------------------------
# Common targets:
#   make build      native binary -> bin/snmpdigger
#   make cross      linux amd64 / 386 / armhf / arm64 -> dist/
#   make deb        .deb package per arch -> dist/*.deb
#   make dist       cross + deb + dist/SHA256SUMS
# ------------------------------------------------------------------------------

PKG        := snmpdigger
MODULE     := github.com/kawaiipantsu/snmpdigger
MAIN       := .
BIN_DIR    := bin
DIST_DIR   := dist
PREFIX     ?= /usr/local

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo v0.0.0-dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE       ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
DEB_VERSION := $(patsubst v%,%,$(VERSION))

VPREFIX    := $(MODULE)/internal/version
LDFLAGS    := -s -w \
	-X $(VPREFIX).Version=$(VERSION) \
	-X $(VPREFIX).Commit=$(COMMIT) \
	-X $(VPREFIX).Date=$(DATE)

GO         ?= go
export CGO_ENABLED := 0

# Cross-compile matrix: <goarch>[:<goarm>] and the Debian arch it maps to.
LINUX_ARCHES := amd64 386 armhf arm64
GOOS_amd64   := amd64
GOOS_386     := 386
GOOS_armhf   := arm
GOOS_arm64   := arm64
GOARM_armhf  := 7
DEBARCH_amd64 := amd64
DEBARCH_386   := i386
DEBARCH_armhf := armhf
DEBARCH_arm64 := arm64

.DEFAULT_GOAL := help
.PHONY: help build run test vet fmt tidy clean install uninstall cross deb dist snapshot version

help: ## Show this help
	@echo "$(PKG) $(VERSION) ($(COMMIT))"
	@echo
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[1m%-12s\033[0m %s\n", $$1, $$2}'

version: ## Print the resolved version/commit/date
	@echo "version = $(VERSION)"
	@echo "commit  = $(COMMIT)"
	@echo "date    = $(DATE)"

build: ## Build the native binary into bin/
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(PKG) $(MAIN)
	@echo "built $(BIN_DIR)/$(PKG) $(VERSION)"

run: ## Build and run (ARGS="..." to pass flags)
	$(GO) run -ldflags '$(LDFLAGS)' $(MAIN) $(ARGS)

test: ## Run the test suite
	$(GO) test ./...

vet: ## go vet
	$(GO) vet ./...

fmt: ## Format all Go sources in place
	gofmt -w $(shell find . -name '*.go' -not -path './vendor/*')

tidy: ## Tidy go.mod / go.sum
	$(GO) mod tidy

clean: ## Remove build output
	rm -rf $(BIN_DIR) $(DIST_DIR)

install: build ## Install to $(PREFIX)/bin and the man page
	install -Dm0755 $(BIN_DIR)/$(PKG) $(DESTDIR)$(PREFIX)/bin/$(PKG)
	install -Dm0644 packaging/$(PKG).1 $(DESTDIR)$(PREFIX)/share/man/man1/$(PKG).1

uninstall: ## Remove an installed copy
	rm -f $(DESTDIR)$(PREFIX)/bin/$(PKG)
	rm -f $(DESTDIR)$(PREFIX)/share/man/man1/$(PKG).1

# ------------------------------------------------------------------------------
# Cross compilation
# ------------------------------------------------------------------------------
cross: $(addprefix $(DIST_DIR)/$(PKG)_$(VERSION)_linux_,$(LINUX_ARCHES)) ## Build all Linux target binaries

# Template: $(1) = arch label (amd64|386|armhf|arm64)
define CROSS_rule
$(DIST_DIR)/$(PKG)_$(VERSION)_linux_$(1):
	@mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=$(GOOS_$(1)) GOARM=$(GOARM_$(1)) \
		$(GO) build -trimpath -ldflags '$(LDFLAGS)' \
		-o $(DIST_DIR)/$(PKG)_$(VERSION)_linux_$(1) $(MAIN)
	@echo "built $(DIST_DIR)/$(PKG)_$(VERSION)_linux_$(1)"
endef
$(foreach a,$(LINUX_ARCHES),$(eval $(call CROSS_rule,$(a))))

# ------------------------------------------------------------------------------
# Debian packages (built with dpkg-deb, no nfpm needed)
# ------------------------------------------------------------------------------
deb: cross ## Build a .deb for every Linux arch
	@$(foreach a,$(LINUX_ARCHES),$(MAKE) --no-print-directory _deb-one ARCH=$(a);)

_deb-one:
	@set -eu; \
	arch="$(ARCH)"; \
	debarch="$(DEBARCH_$(ARCH))"; \
	bin="$(DIST_DIR)/$(PKG)_$(VERSION)_linux_$$arch"; \
	stage="$(DIST_DIR)/deb/$(PKG)_$(DEB_VERSION)_$$debarch"; \
	echo "packaging $$stage.deb"; \
	rm -rf "$$stage"; \
	install -Dm0755 "$$bin" "$$stage/usr/bin/$(PKG)"; \
	install -Dm0644 packaging/copyright "$$stage/usr/share/doc/$(PKG)/copyright"; \
	gzip -9 -n -c packaging/changelog.Debian > "$$stage/usr/share/doc/$(PKG)/changelog.Debian.gz"; \
	chmod 0644 "$$stage/usr/share/doc/$(PKG)/changelog.Debian.gz"; \
	install -d "$$stage/usr/share/man/man1"; \
	gzip -9 -n -c packaging/$(PKG).1 > "$$stage/usr/share/man/man1/$(PKG).1.gz"; \
	chmod 0644 "$$stage/usr/share/man/man1/$(PKG).1.gz"; \
	size=$$(du -k -s "$$stage/usr" | cut -f1); \
	install -d "$$stage/DEBIAN"; \
	printf '%s\n' \
		"Package: $(PKG)" \
		"Version: $(DEB_VERSION)-1" \
		"Architecture: $$debarch" \
		"Maintainer: kawaiipantsu <12233528+kawaiipantsu@users.noreply.github.com>" \
		"Installed-Size: $$size" \
		"Section: net" \
		"Priority: optional" \
		"Homepage: https://github.com/kawaiipantsu/snmpdigger" \
		"Depends: libc6" \
		"Description: fancy TUI SNMP browser, live grapher and network discovery tool" \
		" snmpdigger is a terminal application for exploring SNMP agents. It walks" \
		" and browses the MIB/OID tree with search, filters and sorting, draws live" \
		" graphs (line, bars, gauge, sparkline, heatmap) of any counter or gauge," \
		" identifies devices from their system group and sweeps CIDR ranges to" \
		" discover SNMP-speaking hosts." \
		> "$$stage/DEBIAN/control"; \
	dpkg-deb --build --root-owner-group "$$stage" "$$stage.deb"; \
	echo "built $$stage.deb"

dist: cross deb ## Build binaries + packages + checksums
	@cd $(DIST_DIR) && sha256sum \
		$(PKG)_$(VERSION)_linux_* \
		$(foreach a,$(LINUX_ARCHES),deb/$(PKG)_$(DEB_VERSION)_$(DEBARCH_$(a)).deb) \
		> SHA256SUMS
	@echo "wrote $(DIST_DIR)/SHA256SUMS"

snapshot: cross ## tar.gz each cross binary
	@$(foreach a,$(LINUX_ARCHES), \
		tar -C $(DIST_DIR) -czf $(DIST_DIR)/$(PKG)_$(VERSION)_linux_$(a).tar.gz $(PKG)_$(VERSION)_linux_$(a) && \
		echo "wrote $(DIST_DIR)/$(PKG)_$(VERSION)_linux_$(a).tar.gz" ; )
