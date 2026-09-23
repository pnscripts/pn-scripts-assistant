# Everything between the source and the file somebody installs.
#
# One command, in one order, that anybody can run: make release. It was a
# sequence somebody had to remember — build this, check that, do not forget
# the tests — and a sequence somebody has to remember is a sequence that gets
# a step skipped on the evening it matters.
#
# Each step is also a target of its own, because the whole thing takes minutes
# and "run the tests again" should not mean running the packaging again.

SHELL := /bin/bash
ROOT := $(shell pwd)
VERSION := $(shell tr -d ' \t\n\r' < VERSION)
PACKAGES := $(ROOT)/build/packages
GO := CGO_ENABLED=1 go

.PHONY: help clean check test build package validate checksums release version install-deps

help:
	@echo "  make release   everything below, in order, ending in files to publish"
	@echo "  make check     formatting and vet"
	@echo "  make test      every test"
	@echo "  make build     the binary, into build/"
	@echo "  make package   the .deb"
	@echo "  make validate  lintian and the desktop entry"
	@echo "  make clean     remove build/"
	@echo "  make version   what this would be released as"

version:
	@echo "declared:  $(VERSION)   (VERSION)"
	@echo "package:   $$(bash scripts/build-packages.sh --print-version 2>/dev/null || echo '?')"
	@echo "commit:    $$(git rev-parse --short HEAD 2>/dev/null || echo 'not a repository')"

clean:
	rm -rf build

# Formatting and vet, which is the cheapest check there is and the one most
# worth failing on: a build that has not been formatted has not been read.
check:
	@echo "→ gofmt"
	@test -z "$$(gofmt -l clients/cmd clients/internal)" || \
		{ echo "not formatted:"; gofmt -l clients/cmd clients/internal; exit 1; }
	@echo "→ go vet"
	@cd clients && $(GO) vet ./...
	@bash scripts/version-order-test.sh

test:
	@echo "→ go test"
	@cd clients && $(GO) test ./... -p 2

# The ordinary build, for looking at. The one that ships is built inside the
# packaging script, with -trimpath and the version stamped in.
build:
	@mkdir -p build
	@cd clients && $(GO) build -trimpath -o "$(ROOT)/build/pn-scripts-assistant" ./cmd/pn-scripts-assistant
	@"$(ROOT)/build/pn-scripts-assistant" version

package:
	@bash scripts/build-packages.sh deb

# What the package says about itself, from the outside. Run after packaging
# and again in CI, because a package that installs is not the same as a
# package that is correct.
validate:
	@set -e; \
	deb="$$(ls $(PACKAGES)/pn-scripts-assistant_*_amd64.deb 2>/dev/null | head -1)"; \
	test -n "$$deb" || { echo "no package built"; exit 1; }; \
	echo "→ $$deb"; \
	dpkg-deb --info "$$deb" | sed -n '3,12p'; \
	echo "→ the files it carries"; \
	carried="$$(dpkg-deb --contents "$$deb")"; \
	echo "$$carried" | grep -E 'usr/(bin|share/(applications|doc|man))' | awk '{print $$1, $$6}'; \
	for required in usr/bin/pn-scripts-assistant \
		usr/share/applications/pn-scripts-assistant.desktop \
		usr/share/doc/pn-scripts-assistant/copyright \
		usr/share/doc/pn-scripts-assistant/changelog.gz \
		usr/share/man/man1/pn-scripts-assistant.1.gz; do \
		echo "$$carried" | grep -q "$$required" || { echo "missing: $$required"; exit 1; }; \
	done; \
	if command -v lintian >/dev/null; then \
		echo "→ lintian"; \
		lintian --fail-on error,warning --tag-display-limit 0 "$$deb"; \
		echo "   clean"; \
	else \
		echo "!  lintian is not installed; the package was not checked"; \
	fi

# What somebody downloading a file needs to know it is the file that was
# built. Written beside the packages, named for the version.
checksums:
	@cd $(PACKAGES) && sha256sum *.deb > SHA256SUMS && cat SHA256SUMS

release: clean check test package validate checksums
	@echo
	@echo "  Released $(VERSION)"
	@echo "  Files:   $(PACKAGES)"
	@ls -lh $(PACKAGES) | tail -n +2 | awk '{print "    " $$9, $$5}'
	@echo
	@echo "  Install with: sudo apt install ./$$(cd $(PACKAGES) && ls *.deb)"
