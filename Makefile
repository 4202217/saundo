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
EXE := $(if $(filter windows,$(shell $(GO) env GOOS)),.exe,)

.PHONY: build build-fast e2e-build test race vet uivet i18n fmt arch size check platforms test-mac-update

build:
	@case " $${GOFLAGS:-} " in *preview*) echo "build: GOFLAGS must not carry the preview tag" >&2; exit 2;; esac
	GO="$(GO)" VERSION="$(VERSION)" bash ./scripts/build.sh

# The same binary without the NSIS installer, for the development loop.
build-fast:
	@case " $${GOFLAGS:-} " in *preview*) echo "build: GOFLAGS must not carry the preview tag" >&2; exit 2;; esac
	GO="$(GO)" VERSION="$(VERSION)" bash ./scripts/build.sh --fast

# A non-installing build that links the mygo automation plugin, so the verify
# skill can drive the app over MYGO_AUTOMATION. Never shipped, and it leaves
# bin/saundo.exe and the running app alone.
e2e-build:
	$(GO) build -tags e2e -o bin/saundo-e2e$(EXE) ./cmd/saundo

test:
	$(GO) test ./...

# The local macOS update harness: builds two versions with a throwaway key,
# then drives check, install, quit, and the bundle swap. macOS only.
test-mac-update:
	bash ./scripts/test-mac-update.sh

# The race detector needs cgo and a C toolchain (gcc) on Windows.
race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

# mygo's native UI checks: a ui.Element or ui.Context must not outlive the
# build pass that made it, or be captured by a goroutine. The preview views
# only compile under the preview tag, so they need their own pass.
uivet:
	$(GO) tool mygo vet .
	GOFLAGS=-tags=preview $(GO) tool mygo vet .

# The catalogs must agree on every message and its placeholders.
i18n:
	$(GO) tool mygo i18n check cmd/saundo

# The architecture seams: import direction, go statements, AfterFunc, and
# Sleep in sans-I/O packages, time.After/time.Tick, data channels made
# without a visible capacity whether the channel type is written out or named
# in the package, and direct dependencies that must be on the allowlist.
arch:
	$(GO) run ./scripts/check-arch

# The line budget: grandfathered files may not grow, new non-test files stay
# at or below 800 lines. Generated files are exempt.
size:
	$(GO) run ./scripts/check-size

fmt:
	$(GOFMT) -w .

# Render the UI state matrix and record a manifest. Non-gating until Phase 4.
ui-baseline:
	$(GO) run ./scripts/check-ui render -out .audit/ui/baseline

# Build and vet windows/amd64 and darwin/arm64 on any host, and reject
# OS-specific imports outside platform-tagged files.
platforms:
	GO="$(GO)" $(GO) run ./scripts/check-platforms

check:
	@command -v "$(GOFMT)" >/dev/null 2>&1 || { echo "gofmt not found; set GOFMT=... or put Go on PATH"; exit 1; }
	@unformatted="$$($(GOFMT) -l .)"; \
	if [ -n "$$unformatted" ]; then echo "unformatted files:"; echo "$$unformatted"; exit 1; fi; \
	$(GO) tool mygo i18n check cmd/saundo && $(GO) vet ./... && $(GO) vet -tags preview ./... && $(GO) vet -tags updateharness ./cmd/saundo && $(GO) vet -tags e2e ./cmd/saundo && $(GO) tool mygo vet . && GOFLAGS=-tags=preview $(GO) tool mygo vet . && $(GO) run ./scripts/check-arch && $(GO) run ./scripts/check-size && $(GO) test ./... && $(GO) test -tags preview ./internal/app -count=1 && $(GO) run ./scripts/check-ui catalog && $(GO) run ./scripts/check-ui wasm && $(GO) run ./scripts/verify-map && GO="$(GO)" $(GO) run ./scripts/check-platforms
