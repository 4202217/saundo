# AGENTS.md

Operating guide for coding agents working in this repository. Read this file before changing code. All project documentation is written in English; README.zh-CN.md and CHANGELOG.zh-CN.md are the Chinese counterparts of README.md and CHANGELOG.md, kept in sync with them.

## Project

saundo is a Windows-only AirPlay 2 audio sender written in pure Go. It captures Windows system audio and streams it to HomePod, HomePod mini, Apple TV, and macOS AirPlay receivers.

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

If the architecture document conflicts with the reference implementation or real-device behavior, follow the C++ reference and the device, then update the document.

## Commands

Make targets (run from Git Bash with `mingw32-make`; `make` is not on PATH):

```bash
mingw32-make check  # gofmt gate + go vet + mygo UI vet + go test — the milestone gate
mingw32-make test   # go test ./...
mingw32-make race   # go test -race ./... (needs a C toolchain for the race detector)
mingw32-make vet    # go vet ./...
mingw32-make uivet  # go tool mygo vet . (native UI lifetime checks)
mingw32-make fmt    # gofmt -w .
mingw32-make build  # CGO_ENABLED=0 build into bin/saundo.exe, GUI subsystem, version stamped from git describe, so a commit past a release sorts below it and the updater still offers that release; scripts/build.sh then stops the running app and starts the new build
```

Without mingw32-make, run the commands directly:

```bash
go test ./...
go vet ./...
go tool mygo vet .
gofmt -l .
```

If `go` is not on PATH, use `C:\Program Files\Go\bin\go.exe`, add that directory to PATH, or pass it to mingw32-make: `mingw32-make GO="C:\Program Files\Go\bin\go.exe" check`.

Run `mingw32-make check` (or all four commands) before declaring a milestone complete. A compiling binary is not a completed feature.

## Architecture rules

- Keep the engine UI-agnostic. The UI talks to an `Engine` interface and channels. No IPC and no sidecar.
- Keep the sender sans-i/o, following `airplay2-sender-cpp`: `RaopIo` has six methods (TCP connect/send/close, UDP bind/send/close). The sender owns no sockets, timers, or threads; the host pushes data, connection events, and ticks into it.
- One sender event-loop goroutine owns all protocol sockets and timers. Capture runs in its own goroutine. The UI runs on mygo's main thread. Do not share mutable state; use bounded queues and command channels.
- ALAC is mandatory on the realtime stream. Send uncompressed ALAC frames, never PCM. Use `internal/airplay/alac.EncodeUncompressed`.
- Use NTP for receivers that accept it. A HomePod on software 27 and a macOS
  AirPlay receiver keep their own PTP grandmaster and silently ignore an
  NTP-timed stream: when feature bit 41 is set and the model starts with
  `AudioAccessory` or `Mac`, run `internal/airplay/ptp.Follower`, advertise PTP
  in the session SETUP, send SETPEERS, wait for the clock to be ready, and use
  the 28-byte PTP sync packet. Keep timing behind the sender's `PTPClock`
  interface.
- Do not use stale critical-path libraries such as `go-wca` or `grandcat/zeroconf`. Use `hashicorp/mdns` for discovery and write the WASAPI binding yourself.
- Store credentials with DPAPI. Never log PINs, long-term keys, or session keys.

## Code standards

Error handling:

- A failed read, parse, or decode means "unknown" — never "empty", "zero", or "absent". Where a previous value exists, keep it.
- Gate decisions fail closed: unknown receiver features, unknown auth state, or unreadable credentials never silently take the permissive path. For example, a failed features parse must not route an `AudioAccessory` receiver to NTP.

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
- After every code change, run `mingw32-make build` (`./scripts/build.sh` when mingw32-make is absent) so the local binary matches the tree. It stops the running app, replaces `bin/saundo.exe`, and relaunches it.
- Ask the user only for genuine product decisions. Proceed on reversible engineering work.
- Do not commit unless the user explicitly asks.
- Keep all documentation in English.
- AGENTS.md grows only by graduation: when the same mistake costs a third fix, distill it into Code standards here. Everything else starts in a lower-tier doc.
