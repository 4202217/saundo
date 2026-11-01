# saundo verification and build targets.
# Recipes assume a POSIX shell (Git Bash); mingw32-make works too.
# Run `make check` before declaring a milestone complete.
#
# `make build` goes through mygo so the exe carries resources/icon.png,
# the manifest and the version. scripts/build.sh then stops the running app,
# copies the exe to bin/saundo.exe, and starts it.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GO ?= go
GOFMT ?= gofmt

.PHONY: build test race vet uivet fmt check

build:
	GO="$(GO)" VERSION="$(VERSION)" ./scripts/build.sh

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

fmt:
	$(GOFMT) -w .

check:
	@unformatted="$$($(GOFMT) -l .)"; \
	if [ -n "$$unformatted" ]; then echo "unformatted files:"; echo "$$unformatted"; exit 1; fi; \
	$(GO) vet ./... && $(GO) tool mygo vet . && $(GO) test ./...
