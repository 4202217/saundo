<!-- reviewed-through: 2026-11-04 -->

# Implementation overview

Orientation for a session landing in this codebase: what saundo implements
today, where each piece lives, and how the parts talk. AGENTS.md carries the
rules; [architecture.md](architecture.md) the design rationale;
[protocol.md](protocol.md) and [devices.md](devices.md) the wire protocol and
receiver profiles; [ui.md](ui.md) the UI design language;
[verification.md](verification.md) the evidence. This page describes the code
as it is; update it in the same change that makes it wrong.

## Runtime model

One process, four goroutine roles, no shared mutable state — data moves
through bounded channels:

- **The mygo main thread** owns the tray flyout, the settings window, the
  tray icon, and all UI state. Engine snapshots arrive as `win.Update`
  callbacks (`internal/app`).
- **The engine goroutine** (`internal/engine`, `Engine.run`) is the only
  caller of sender methods. It drains the command channel, runs the timers
  (reconnect backoff, silence suspend, capture restart), and publishes a
  `Snapshot` on every state change. A stereo group whose member dies
  signals the loop through the transport's `Dead` channel, and the loop
  runs `closeSession` itself, so the group teardown also happens on this
  goroutine — never inside a sender callback and never on another
  goroutine.
- **The sender event loop** (`internal/airplay/sender`) owns all protocol
  sockets and timers. It is sans-i/o: `internal/airplay/io` (package
  `raopio`) defines the six-method `IO` seam (TCP connect/send/close, UDP
  bind/send/close) and `Host`, the Go net implementation that pushes
  connection events, received packets, and ticks back in through one
  `Events` channel.
- **The capture goroutine** owns WASAPI (COM-initialized), converts and
  resamples, and writes the ring buffer or the stereo fanout. It never
  touches the network.

The app also polls the Windows SMTC session every two seconds
(`internal/nowplaying`) for the Now Playing row.

## Package map

| Package | Responsibility |
|---|---|
| `cmd/saundo` | Entry point; `mygo.json` carries name, version, and the embedded icon (`resources/icon.png`) |
| `internal/app` | mygo UI: tray flyout (`flyout.go`, adaptive height), settings window (`settings_window.go`, seven pages behind a 190 sidebar, with the 关于 page in `about.go`), the receiver-management page and dialogs (`receivers_page.go`, `receivers_dialogs.go`), the update service and notes dialog (`update.go`, `update_dialog.go`, `update_history.go`, state shared with the settings 更新 page and the flyout chip), tray and menu icons, the phase/state-face derivation and shared widgets with their motion primitives (`ui_kit.go`), palette mapping (`theme.go`), and the airgoplay→saundo data migration (`migrate.go`). The design language these implement is in [ui.md](ui.md) |
| `internal/engine` | UI-agnostic session controller: command handling, transport lifecycle, stereo groups (`group.go`), receiver management (`receivers.go`, `receivers_cmd.go`), silence meter and suspend (`silence.go`), snapshots |
| `internal/discovery` | mDNS browse, TXT parsing, `_raop`/`_airplay` merge, stereo-group collapse (`tsid`/`igl`/`gpn`), offline marking |
| `internal/capture` | WASAPI shared-mode loopback (`capture_windows.go`), per-app process loopback (`app_windows.go`), format conversion (`convert.go`) |
| `internal/audio/ring` | Bounded overwrite-oldest SPSC `Buffer` and multi-sink `Fanout` (`KeepNewest` trim, drop counters) |
| `internal/audio/resample` | Linear 48 kHz → 44.1 kHz conversion for interleaved stereo int16 |
| `internal/airplay/crypto` | Wire primitives: SHA-512/HMAC/HKDF, ChaCha20-Poly1305, X25519/Ed25519, SRP-6a, TLV8, bplist, Digest |
| `internal/airplay/alac` | Uncompressed ALAC frame encoder (`EncodeUncompressed`) |
| `internal/airplay/io` | Package `raopio`: the `IO` seam, event types, and the Go net `Host` |
| `internal/airplay/sender` | The AP2/AP1 state machine: pairing (`pairing.go`), RTSP control (`rtsp.go`), session and event channel (`ap2.go`), RTP/ALAC/pacing (`stream.go`), live-edge start (`start_gate.go`) |
| `internal/airplay/ptp` | `Follower` for receivers that keep their own grandmaster (`ptp.go`), sender-side `Master` for stereo pairs (`master.go`) |
| `internal/airplay/creds` | HAP long-term credentials; DPAPI-protected JSON (`dpapi_windows.go`) |
| `internal/airplay/device` | Receiver description and auth enum shared by discovery, engine, and UI |
| `internal/receivers` | The receiver registry: `receivers.json` store (atomic, versioned, single writer), the pure merge/visibility/auto-target/latency functions, the manual-receiver spec, and the settings migration. Rows key by deviceid, `stereo:<tsid>`, or `m:<uuid>` |
| `internal/nowplaying` | Windows SMTC reader for the Now Playing row (Windows/other build split) |
| `internal/settings` | JSON configuration store |
| `internal/diagnostics` | Rolling in-memory log with 5 MB file rotation |

The `Engine` methods are the UI's whole surface: `Start`, `Stop`,
`Snapshot`, `Refresh`, `Connect`, `Disconnect`, `SetVolume`, `SubmitPIN`,
`DismissPasswordPrompt`, `SetReceiverPassword`, `SetCaptureEndpoint`,
`SetCaptureProcess`, `SetLatency`, `ApplyLatency`, `SetLatencySettings`,
`SetStrictAuth`, `SetName`, `SetReconnect`,
`SetShowNowPlaying`, `SetNowPlaying`, and the receiver-management commands
`AddReceiver`, `UpdateReceiver`, `RemoveReceiver`, `SetReceiverAutoConnect`,
`SetReceiverHidden`, `SetReceiverDelay`. `SetLatency` applies on the next
session; `ApplyLatency` restarts a live one; `SetLatencySettings` keeps the
app-only latency mode fields in the engine settings copy. The snapshot
carries both lists: `Devices` (the flyout, with hidden receivers filtered
out while the active session's row stays) and `Receivers` (every row with
its settings, no secrets — only whether a password exists).

The registry remembers every first sighting, including a stereo pair under
its `stereo:<tsid>` key, so its hidden, delay, password, and auto-connect
settings survive the pair going offline. Exactly one entry carries
`AutoConnect`; at launch the engine connects that target once, immediately
for a manual row and on the first snapshot that reports a discovered one
online. A user connect or disconnect ends the launch wait, and a failed
launch falls into the ordinary reconnect policy. The global
`settings.AutoConnect` flag is a migration input only.

The engine's SETUP latency for a receiver is the global base plus that
receiver's DSP/DAC delay, clamped to 20–4000 ms. The app composes the
flyout's ALAC line from the same two values for the active receiver; a delay
change to the active receiver restarts its session, debounced 500 ms like the
custom latency control. The app holds the
last streaming snapshot for up to 3 s while an apply restarts, so the flyout
keeps its streaming faces; only the latency row changes and a spinner follows
the playback label. Preset clicks debounce the restart for 300 ms and custom
edits for 500 ms, so rapid switching coalesces into one restart with the final
mode.

## A connect, end to end

1. The UI calls `Engine.Connect(id)`; the command reaches the engine
   goroutine.
2. The engine resolves the target — a device or a collapsed stereo group
   (`openSession` / `openGroupSession`) — checks stored credentials, and
   picks the pairing path from the device's auth kind.
3. The sender runs the protocol: pair-setup (transient / on-screen PIN /
   receiver password) → pair-verify → encrypted control channel →
   `GET /info` → session SETUP (a password-protected Mac answers 401 and
   gets the Digest replay) → event channel → `RECORD` → SETPEERS and PTP
   readiness → stream SETUP → streaming.
4. Capture starts, or the fanout attaches one ring per pair member; the
   `StartGate` trims every ring to the newest 50 ms cushion and seals the
   shared start epoch; RTP flows at 44100 frames per second.
5. While streaming: feedback every 2 s, receiver-volume polling, sync
   packets, and event-channel replies run on the sender's timers; after 2 s
   of digital silence the engine suspends the pacer until the next audible
   block.

## On-disk state

All under `%APPDATA%\saundo\`:

- `settings.json` — configuration (`internal/settings`), including the update preferences (automatic checks and downloads, skipped version, last check) and the tray animation switch. The legacy `autoConnect` flag and `passwords` map stay decodable but unread: migration seeds the registry's auto-connect entry and moves passwords to DPAPI.
- `receivers.json` — the receiver registry (`internal/receivers`): one entry per remembered receiver, stereo pair, or manually added receiver, with hidden, auto-connect, and delay settings. Written atomically, versioned, and only by the engine goroutine (the startup migration runs before the engine exists). A file that fails to parse, cannot be read, or was written by a newer schema is never overwritten.
- `credentials.dat` — DPAPI-protected credential JSON keyed by receiver
  `deviceid` (`internal/airplay/creds`), holding the HAP long-term keys plus receiver passwords under the `password:` namespace.
- `saundo.log` (+ `saundo.log.1` rotation) — diagnostics.
- The first run after the airgoplay rename copies `settings.json` and
  `credentials.dat` from `%APPDATA%\airgoplay` (`internal/app/migrate.go`).
  The same startup runs the receiver migration: plaintext passwords move
  into DPAPI (a password already in DPAPI wins), and the old auto-connect
  flag plus `lastDeviceId` seed one auto-connect entry. Both steps are
  idempotent.

## Build and packaging

`mingw32-make build` runs `scripts/build.sh`: `mygo build -platform windows/amd64`
with `MYGO_LDFLAGS` stamping `saundo/internal/app.Version` from
`git describe`, the icon and manifest embedded from
`cmd/saundo/mygo.json`. The app reports the stamp to the updater, so a local
build past a release sorts below it and still offers that release; `VERSION`
overrides the stamp. The script stops the running app, copies
`saundo.exe` (GUI subsystem) and the NSIS `saundo-setup-*.exe` into
`bin/`, and starts the new build.
