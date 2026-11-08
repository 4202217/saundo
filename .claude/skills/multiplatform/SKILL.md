---
name: multiplatform
description: Develop and review saundo across Windows and macOS without breaking the other side. Use when a change touches a platform seam, build tags, credentials, capture, Now Playing, packaging, CI, or when a PR must prove both platforms still compile and keep the same behavior.
---

# Multi-platform development

saundo ships Windows and macOS from one Go tree. Most code is shared; OS
differences enter through the seams in [docs/platforms.md](../../../docs/platforms.md).
This skill is the working checklist for a change that crosses one.

## Gate before commit

| Host | Commands |
|---|---|
| Windows (Git Bash) | `mingw32-make check` then `mingw32-make build` |
| macOS | `make check` then `make build` (cmake installed) |
| Either host | `make platforms` |

`make platforms` builds and vets `windows/amd64` and `darwin/arm64` with
`CGO_ENABLED=0`, then rejects `saundo/internal/darwin`, `purego`, and
`golang.org/x/sys/windows` imports in files the other build compiles. The
same check runs in CI as the `platforms` job and inside `make check`. If
`make` is missing, run `go run ./scripts/check-platforms` directly.

## Decide where the change lives

Ask: does it talk to the OS beyond what mygo wraps?

- No. Write it in the shared packages. The diff should touch no
  `_windows`/`_darwin`/`_other` file, and both CI jobs test it.
- Yes. Extend a seam:
  1. Define the contract and its failure semantics in shared code. A failed
     read is "unknown", never zero; an unsupported platform fails closed.
  2. Implement the Windows leaf.
  3. Implement the darwin leaf, or keep the fail-closed stub and add the gap
     to the feature parity table in docs/platforms.md.
  4. Retag the `_other.go` stub (`!windows` becomes `!windows && !darwin`)
     when a darwin leaf appears.
  5. Test through the injection seam so one test covers both platforms.
  6. Update the parity table and, for device behavior, docs/devices.md or
     docs/verification.md.

Platform internals never leak upward. COM vtables, ObjC selectors, Core
Audio property codes, and Keychain calls stay inside the leaf packages. The
`platforms` check enforces the import direction.

## Packaging rules

- `cmd/saundo/mygo.json` must not list `resources` outside a platform
  directory. Put platform-only files under
  `cmd/saundo/resources/<goos>-<goarch>/`, which mygo ships only with that
  target. The MediaRemote adapter is built there by
  `scripts/build-adapter.sh`.
- No committed signing identity. macOS signs ad hoc by default; set
  `SAUNDO_MAC_SIGN_IDENTITY` for a stable local certificate, or pass
  `-sign <identity>`. CI and the release use ad hoc until a Developer ID
  exists.
- A clean Windows checkout must never need a macOS tool (cmake) or a
  macOS-only directory to build.

## What each side can prove

| Claim | Prove it on |
|---|---|
| Shared logic, protocol, ALAC, engine | Either host; both CI jobs run the tests |
| Windows compile, vet, tests | Windows CI and `make platforms` |
| macOS compile, vet, tests | macOS CI and `make platforms` |
| Tray, flyout shadow/focus, WASAPI, installer | Windows live run |
| Core Audio tap, TCC prompt, Local Network, Keychain ACL, `.app` bundle | macOS live run |
| Audible output to a real receiver | The user, on the platform being tested |

Never claim a behavior was verified on a platform you did not run. Record
the platform in docs/verification.md.

## Review checklist for a port PR

- Every new platform file has a build tag and a matching fail-closed stub.
- Shared code compiles for both targets; `make platforms` is green.
- A clean checkout of the other platform can build; no committed resource
  or signing identity points at this machine.
- The diff names the other platform's behavior it changes, if any.
- New tests use `testdata/` fixtures and run on the host; no network.
- docs/platforms.md's parity table and docs/verification.md are updated.
