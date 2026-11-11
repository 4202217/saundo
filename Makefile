# saundo verification and build targets.
# Recipes assume a POSIX shell (Git Bash); use mingw32-make, since make may not
# be on PATH. Run `mingw32-make check` before declaring a milestone complete.
#
# `mingw32-make build` goes through mygo so the exe carries resources/icon.png,
# the manifest and the version. scripts/build.sh then stops the running app,
# copies the exe to bin/saundo.exe, and starts it. `build-fast` skips the
# installer and leaves the copy to the same script.

# Local builds carry git describe, so a commit past a release sorts below it
# and the updater still offers that release. Override with
# `mingw32-make VERSION=0.4.1 build`.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GO ?= go
GOFMT ?= gofmt

.PHONY: build build-fast test race vet uivet i18n fmt check platforms

build:
	GO="$(GO)" VERSION="$(VERSION)" bash ./scripts/build.sh

# The same binary without the NSIS installer, for the development loop.
build-fast:
	GO="$(GO)" VERSION="$(VERSION)" bash ./scripts/build.sh --fast

test:
	$(GO) test ./...

# The race detector needs cgo and a C toolchain (gcc) on Windows.
race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

# mygo's native UI checks: a ui.Element or ui.Context must not outlive the
# build pass that made it, or be captured by a goroutine.
uivet:
	$(GO) tool mygo vet .

# The catalogs must agree on every message and its placeholders.
i18n:
	$(GO) tool mygo i18n check cmd/saundo

fmt:
	$(GOFMT) -w .

# Build and vet windows/amd64 and darwin/arm64 on any host, and reject
# OS-specific imports outside platform-tagged files.
platforms:
	GO="$(GO)" $(GO) run ./scripts/check-platforms

check:
	@command -v "$(GOFMT)" >/dev/null 2>&1 || { echo "gofmt not found; set GOFMT=... or put Go on PATH"; exit 1; }
	@unformatted="$$($(GOFMT) -l .)"; \
	if [ -n "$$unformatted" ]; then echo "unformatted files:"; echo "$$unformatted"; exit 1; fi; \
	$(GO) tool mygo i18n check cmd/saundo && $(GO) vet ./... && $(GO) tool mygo vet . && $(GO) test ./... && $(GO) run ./scripts/verify-map && GO="$(GO)" $(GO) run ./scripts/check-platforms
