# AGENTS.md

Operating guide for coding agents working in this repository. Read this file before changing code. All project documentation is written in English; README.zh-CN.md and CHANGELOG.zh-CN.md are the Chinese counterparts of README.md and CHANGELOG.md, kept in sync with them.

## Project

saundo is an AirPlay 2 audio sender for Windows and macOS written in pure Go. It captures the system audio and streams it to HomePod, HomePod mini, Apple TV, and Mac receivers.

Scope:

- Audio only. No video, screen mirroring, or FairPlay. A plaintext AP1 fallback exists for `_raop`-only receivers, but AP1 is not a target.
- Pure Go, single process, single binary. `CGO_ENABLED=0`. Go 1.27.3, the toolchain mygo requires. Module: `saundo`.
- Native UI with mygo. No WebView.
- Distributed under GPL-3.0-only. See `LICENSE` and `NOTICE`.

## Read first

- `docs/architecture.md`: design decisions, technology choices, and risks.
- `docs/protocol.md`: the wire-level protocol — pairing, control channel, AP2 session, RTP, timing.
- `docs/devices.md`: per-receiver profiles — how each family advertises, pairs, and behaves.
- `docs/implementation.md`: the code map — runtime model, packages, flows, build.
- `docs/ui.md`: the UI design language — tokens, widgets, motion, windows.
- `docs/ui-copy.md`: the copy standard — terminology, state wording, units, punctuation.
- `docs/verification.md`: automated checks, live-test recipes, and the evidence log.
- `README.md`: the project overview for readers.
- `.claude/skills/multiplatform/SKILL.md`: the working checklist for a change that crosses a platform seam.

If the architecture document conflicts with the reference implementation or real-device behavior, follow the C++ reference and the device, then update the document.

## Commands

Make targets (Windows: Git Bash with `mingw32-make`; macOS: `make`, with cmake installed for the Now Playing adapter):

```bash
mingw32-make check  # gofmt gate + catalog and message checks + go vet under the default, updateharness, and e2e tags + mygo UI vet + architecture and size gates + go test + verification map + cross-platform gate — the milestone gate
mingw32-make platforms  # build + vet windows/amd64 and darwin/arm64 on either host, reject leaked platform imports
mingw32-make test   # go test ./...
mingw32-make test-mac-update  # macOS only: the local update harness (build, check, install, quit, bundle swap)
mingw32-make race   # go test -race ./... (needs a C toolchain for the race detector)
mingw32-make vet    # go vet ./...
mingw32-make uivet  # go tool mygo vet . (native UI lifetime checks)
mingw32-make arch   # scripts/check-arch: import seams, sans-I/O go/AfterFunc/Sleep, time.After/time.Tick, channel capacity, dependency allowlist
mingw32-make size   # scripts/check-size: grandfathered files may not grow or shrink without lowering the baseline; new non-test files stay at or below 800 lines
mingw32-make fmt    # gofmt -w .
mingw32-make build  # CGO_ENABLED=0 build into bin/saundo.exe, GUI subsystem, version stamped from git describe, so a commit past a release sorts below it and the updater still offers that release; scripts/build.sh then stops the running app and starts the new build
mingw32-make build-fast  # the same build without the NSIS installer, for the development loop
mingw32-make e2e-build  # non-installing build with the mygo automation plugin, for the verify skill's UI driving
```

Without mingw32-make, run the commands directly:

```bash
go test ./...
go vet ./...
go vet -tags updateharness ./cmd/saundo
go vet -tags e2e ./cmd/saundo
go tool mygo i18n check cmd/saundo
go tool mygo vet .
go run ./scripts/check-arch
go run ./scripts/check-size
go run ./scripts/verify-map
gofmt -l .
```

If `go` is not on PATH, add its `bin` directory to PATH. Passing `GO=` alone is not enough for `check`: the gofmt gate and mygo's internal `go` calls resolve from PATH, so pair it with `GOFMT=` or, preferably, add the directory to PATH. On Windows the usual path is `C:\Program Files\Go\bin`.

Run `mingw32-make check` (or all the commands listed above) before declaring a milestone complete. A compiling binary is not a completed feature.

## Architecture rules

Several of these rules are gates, not review notes. `scripts/check-arch` owns
the import seams, the sans-i/o goroutine, `AfterFunc`, and `Sleep` rule, the
`time.After` and `time.Tick` ban, the capacity rule for data channels,
whether the channel type is written out or named in the same package, and
the direct-dependency allowlist. `scripts/check-size` owns the line budget.
`device.Features` and `creds.Secret` put two fail-closed rules in the type
system. A rule added here without a gate is judgment-only.

- Keep the engine UI-agnostic. The UI talks to an `Engine` interface and channels. No IPC and no sidecar.
- Keep the sender sans-i/o, following `airplay2-sender-cpp`: `RaopIo` has six methods (TCP connect/send/close, UDP bind/send/close). The sender owns no sockets, timers, or threads; the host pushes connection, data, and close events into it, and the host loop pulls timers with `NextDeadline` and `Tick`.
- One engine event-loop goroutine drives the sender and owns session timers; the host's I/O goroutines own the protocol sockets, and PTP runs its own loop. Capture runs in its own goroutine. The UI runs on mygo's main thread. Do not share mutable state; use bounded queues and command channels.
- ALAC is mandatory on the AP2 realtime stream. Send ALAC frames, never PCM there: `internal/airplay/alac.EncodeUncompressed` for real audio, and `alac.EncodeCompactSilence` only for a fully starved packet (the one hand-built compressed frame, the rice coder's zero-run case). The plaintext AP1 fallback for `_raop`-only receivers is the only raw-PCM path, and AP1 is not a target.
- Use NTP for receivers that accept it. A HomePod on software 27 and a macOS
  AirPlay receiver keep their own PTP grandmaster and silently ignore an
  NTP-timed stream: when the model starts with `AudioAccessory` or `Mac` and
  feature bit 41 is set, or its features value is unknown (fail closed), run
  `internal/airplay/ptp.Follower`, advertise PTP
  in the session SETUP, send SETPEERS, wait for the clock to be ready, and use
  the 28-byte PTP sync packet. Keep timing behind the sender's `PTPClock`
  interface.
- Do not use stale critical-path libraries such as `go-wca` or `grandcat/zeroconf`. Use `hashicorp/mdns` for discovery and write the WASAPI and Core Audio bindings yourself. Now Playing on macOS reads through the vendored MediaRemote adapter (`native/mediaremote-adapter`), never from a third-party process directly. A new direct dependency must be added to `scripts/check-arch`'s allowlist on purpose.
- Store credentials with the OS secret store: DPAPI on Windows, the login-Keychain master key with AES-256-GCM on macOS (`internal/darwin`). Never log PINs, long-term keys, or session keys. `LTSK` is `creds.Secret`; every format verb prints `[redacted]`.

## Code standards

Error handling:

- A failed read, parse, or decode means "unknown" — never "empty", "zero", or "absent". Where a previous value exists, keep it.
- Gate decisions fail closed: unknown receiver features, unknown auth state, or unreadable credentials never silently take the permissive path. For example, a failed features parse must not route an `AudioAccessory` receiver to NTP. `device.Features` makes a failed parse the zero value and hides the bits behind `Known()` and `Has()`.

Tests:

- Every bug fix ships a test that fails without the fix. A test that already fails on the current tree is a bug; fix it at its cause.
- Test fixtures live in the owning package's `testdata/` directory: captured device logs, packet dumps, bplist blobs, saved transcripts. Tests read fixtures from `testdata/`, never from the network or from outside the repository. The go tool ignores `testdata/` when scanning packages.

Commits:

- The only reader is the next agent session, which cannot see this conversation; the commit body is the durable record.
- One commit has one goal, so a revert takes back one change and keeps the others.
- Subject: `<area>: <what is now true>`. Area is the package or layer (`sender`, `engine`, `capture`, `discovery`, `app`, `ptp`, `crypto`, `audio`, `build`, `docs`); the rest is keywords, not prose.
- End the body with one field per line: `Checks:` what ran and what it printed, not-run listed as not run; `Pinned-by:` the test that fails without the change (bug fixes name one); `Finding:` device findings — model, OS build, observed behavior, where the evidence lives; `Docs:` documentation updated in the same commit, its reviewed-through date bumped, and links resolving.

Reporting:

- State which checks ran, what they printed, and which did not run. An unrun check is listed as not run.

## Testing and workflow

- When real-device behavior differs, capture logs and packets first, then change code, and record the finding in `docs/verification.md` plus the matching profile in `docs/devices.md` or `docs/protocol.md` — in the same commit.
- After every code change, run the host build: `mingw32-make build` on Windows, `make build` on macOS (`./scripts/build.sh` when make is absent). It stops the running app, replaces the binary or app bundle, and relaunches it.
- Run the cross-platform gate before committing: `make platforms` (or `go run ./scripts/check-platforms`). It builds and vets windows/amd64 and darwin/arm64 on either host and fails when an OS-specific import leaks into shared code. `make check` includes it.
- macOS signing takes `SAUNDO_MAC_SIGN_IDENTITY` or `-sign`; no certificate is committed. CI and the release sign ad hoc until a Developer ID exists.
- Ask the user only for genuine product decisions. Proceed on reversible engineering work.
- Do not commit unless the user explicitly asks.
- Keep all documentation in English.
- AGENTS.md grows only by graduation: when the same mistake costs a third fix, distill it into Code standards here. Everything else starts in a lower-tier doc.
