<!-- reviewed-through: 2026-11-09 -->

# Multi-platform development

saundo ships Windows and macOS from one Go codebase. About 91% of the Go
lines are platform-neutral (counted over non-test files in November 2026;
re-measure by comparing `find . -name '*.go' -not -path './.git/*' | xargs
wc -l` against the windows-tagged set), and every OS difference enters
through a narrow seam. Most features are therefore written once and land on
both platforms; only the seam needs a second implementation. This page maps
the seams and the rules for extending them. The code map:
[implementation.md](implementation.md). The UI's design references:
[ui.md](ui.md).

## The platform model

- **Shared core, no platform code.** `internal/airplay` (protocol, sender,
  ALAC, PTP), `internal/audio`, `internal/engine`, `internal/lamp`,
  `internal/settings`, `internal/receivers`, `internal/diagnostics`, and the
  whole of `internal/app`'s UI (mygo is cross-platform: tray, flyout,
  windows, notifications) compile everywhere and know nothing about the OS.
- **Per-platform leaf packages.** A handful of packages carry one file per
  platform behind build tags, all with a fail-closed `*_other.go` stub for
  platforms without an implementation. The stub returns an error or a
  disabled capability, never a silent zero.
- **`internal/darwin`.** The macOS FFI layer, pure Go through purego: the
  Objective-C bridge, the login-Keychain SecItem calls, and the Core Audio C
  API. It serves the darwin leaves only; nothing above the seams imports it.

## The seams

| Seam | Shared contract | Windows | macOS | Other platforms |
|---|---|---|---|---|
| System capture | `capture.Start`, `capture.Config`, `capture.Source` (`capture.go`) | `capture_windows.go`: WASAPI shared-mode loopback; per-app via the process-loopback device | `capture_darwin.go`: Core Audio process tap aggregated with the output device | `capture_other.go`: fail-closed |
| Credentials at rest | `creds.Store`, `protect`/`unprotect` | `dpapi_windows.go`: CryptProtectData | `dpapi_darwin.go`: AES-256-GCM, master key in the login Keychain | `dpapi_other.go`: fail-closed |
| Mute local | `capture.EndpointMuted`, `capture.SetEndpointMuted` | `mute_windows.go`: `IAudioEndpointVolume` | `mute_darwin.go`: `kAudioDevicePropertyMute` on the output device | `mute_other.go`: fail-closed |
| Now playing | `nowplaying.Read` and the control/snapshot set (`nowplaying.go`) | `*_windows.go`: SMTC; names and icons resolved from the shell's AppsFolder (`appidentity_windows.go`) | `nowplaying_darwin.go`: the vendored MediaRemote adapter; names from LaunchServices, icons via `NSWorkspace` (`internal/darwin/appinfo.go`) | `nowplaying_other.go`: fail-closed |
| Neighbor table | `discovery.neighborHosts` | `neighbors_windows.go`: `GetIpNetTable` | fail-closed empty (remembered endpoints cover offline connect) | `neighbors_other.go`: fail-closed |
| Engine wiring | injected function fields (`engine.go:290` `captureStartFn`, `engine.go:395-398` the endpoint/mute set, `engine.go:374` `lampJPEG`) | the constructor binds the capture and mute leaves | same constructor, same leaves | the constructor binds the stubs |
| App wiring | `app.go:113-114` `readNowPlaying`/`pushNowPlaying`, `update.go` `updateEngine` | bound to `nowplaying.Read` and `mygo.Updater` | `nowplaying.Read` fails closed; `update.go` disables updates | stubs |

## Adding a feature

Ask one question first: does the feature talk to the OS beyond what mygo
already wraps?

- **No** — write it in the shared packages. Engine logic, sender behavior,
  settings, receiver handling, and any mygo widget need no platform
  thought; the diff should touch no file with a `_windows`/`_darwin`/
  `_other` sibling. Both CI jobs test it.
- **Yes** — extend a seam, in this order:
  1. Define the contract in shared code (a `capture.Config` field, an
     engine function field, or a package function) and its failure
     semantics. A failed read is "unknown", never a zero; an unsupported
     platform fails closed.
  2. Implement the Windows leaf.
  3. Implement the darwin leaf, or leave the fail-closed stub and record
     the gap in the parity table below.
  4. Retag the `*_other.go` stubs: `//go:build !windows` becomes
     `//go:build !windows && !darwin` once a darwin file exists, or the
     darwin build sees two implementations.
  5. Test through the injection seam (the harness fakes in
     `internal/engine`'s tests are the pattern), so one test suite covers
     both platforms.
  6. Add the row to the parity table, and a line to
     [devices.md](devices.md) or [verification.md](verification.md) if a
     real device behaved differently from the code's assumption.
  7. Run `make platforms`. Both targets must build and vet before the
     change is done.

Platform internals never leak upward: COM vtables, ObjC selectors, and
Core Audio property codes stay inside the leaf packages.

## Feature parity

| Feature | Windows | macOS |
|---|---|---|
| System audio capture | WASAPI shared-mode loopback | Core Audio process tap + private aggregate device (macOS 14.4+) |
| Per-application capture | process-loopback device (Windows 10 2004+) | not implemented; the seam is ready (`capture.Config.ProcessID`, `AppCaptureSupported`) |
| Mute local on connect | render-endpoint mute; loopback interaction is driver-dependent | output-device mute; capture is unaffected either way |
| Now Playing card | SMTC | not implemented; `nowplaying` fails closed and callers degrade |
| Credentials at rest | DPAPI (per user) | login-Keychain master key + AES-256-GCM (per user) |
| Receiver discovery | mDNS + the ARP neighbor table | mDNS; the neighbor table returns empty |
| Auto-update | signed feed through `mygo.Updater` | disabled: the build is ad-hoc signed and the feed carries Windows assets; re-enable with a signed, notarized mac feed |
| Packaging | `saundo.exe` + NSIS installer | `saundo.app` + dmg, Info.plist from `mygo.json` `macos.infoPlist`, ad-hoc signed unless `SAUNDO_MAC_SIGN_IDENTITY` names one |
| Flyout style | system-rounded flyout with its window shadow | rounded flyout or NSPopover; the style setting is macOS-only |

## macOS runtime notes

- **TCC audio capture** is keyed to the bundle (`com.saundo.app`); the
  prompt appears because `mygo.json` sets
  `NSAudioCaptureUsageDescription`. A bare binary outside a bundle gets no
  reliable prompt.
- **Local Network privacy** (macOS 15+): mDNS, RTSP and PTP are dropped
  until the user allows the app under Privacy & Security → Local Network.
  Discovering nothing is the symptom; the flyout should link the setting.
- **Distribution without notarization** (measured on macOS 27.3,
  November 2026): a downloaded copy carries the quarantine attribute, so
  the first launch is blocked; the recovery is System Settings → Privacy &
  Security → Open Anyway and one authentication — the Sequoia-era flow,
  since the old right-click "Open" bypass is gone. `spctl -a` keeps
  printing `rejected` even after the user allows it; the allow is a user
  decision, not a signature change. Every rebuilt or re-downloaded
  version has a new cdhash, so each release repeats the flow once per
  machine. Locally built copies carry build provenance and the running
  app itself never sees any of this. Mitigations that skip Gatekeeper:
  `brew install --cask` (brew does not quarantine by default), a terminal
  `curl` download, or `xattr -dr com.apple.quarantine`. The updater stays
  off until a signed, notarized feed exists.
- **Local signing**: set `SAUNDO_MAC_SIGN_IDENTITY` to an Apple Development
  or Developer ID identity for a stable local signature; without it the
  build signs ad hoc. No certificate is committed, so CI and other Macs
  never fail `codesign` on an identity they do not have.
- **Keychain in tests**: parallel test binaries touching an item created
  by another binary trigger an ACL prompt. Headless runs set
  `SAUNDO_CREDS_MASTER_KEY` (64 hex chars) and the creds package decides
  keys from the environment instead of the Keychain. The app never sets
  it; a malformed value fails closed.

## Build and checks

- **Windows**: `mingw32-make check` and `mingw32-make build` from Git Bash.
- **macOS**: `make check` and `make build` — the same checks (gofmt,
  catalog, vet, `go tool mygo vet .`, tests) plus the app bundle. cmake
  builds the Now Playing adapter.
- **Either host**: `make platforms` (`go run ./scripts/check-platforms`)
  builds and vets windows/amd64 and darwin/arm64 with `CGO_ENABLED=0` and
  rejects `saundo/internal/darwin`, `purego`, and `golang.org/x/sys/windows`
  imports in files the other build compiles. `make check` includes it, and
  CI runs it as the `platforms` job.
- Both host builds run through `scripts/build.sh`, which branches on
  `uname -s`: `mygo build -platform windows/amd64` or
  `-platform darwin/arm64`, version stamped from git describe. The macOS
  branch calls `scripts/build-adapter.sh` first; the adapter lands in
  `cmd/saundo/resources/darwin-arm64/adapter`, which mygo ships only with
  the darwin/arm64 target, so a clean Windows checkout never needs it.
- **Signing**: `cmd/saundo/mygo.json` commits no certificate. The macOS
  build signs ad hoc unless `SAUNDO_MAC_SIGN_IDENTITY` names a local
  identity; CI and the release pass `-sign -` until a Developer ID exists.
- CI mirrors the split: `.github/workflows/ci.yml` runs `check` on
  windows-latest, `check-macos` on macos-latest, and `platforms` on
  ubuntu-latest; `release.yml` builds the installer and the dmg on their
  own runners. `cmd/saundo/build_test.go` asserts both branches of the
  build script and that `mygo.json` declares no platform-blind resource or
  signing identity.
- The race detector needs a C toolchain on either platform and is not run
  in CI.
