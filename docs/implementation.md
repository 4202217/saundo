<!-- reviewed-through: 2026-11-09 -->

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
  (reconnect backoff, silence suspend, capture restart), renders the
  ambient-light JPEG off the sender's tick path, and publishes a
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
- **The capture goroutine** owns the platform capture leaf (WASAPI on
  Windows, the Core Audio tap on macOS), converts and resamples, and writes
  the ring buffer or the stereo fanout. It never touches the network.

The app also polls the platform Now Playing source every two seconds
(`internal/nowplaying`): SMTC on Windows, the MediaRemote adapter on macOS.
It feeds the Now Playing card and its transport buttons. Capture blocks are measured
into level and five-band frames (`internal/audio/analyze`) on the capture
goroutine; `Engine.LevelFrame` reads them back with the presentation delay so
the card's meter follows the audio being heard. The bands gate energy below
-50 dBFS and stop the per-band AGC at a floor 18 dB above the gate, and the
envelopes ignore changes under 0.02 of the bar's range, so dither and room
tone leave the bars steady and quiet passages compress instead of filling
them.

The receiver push carries the same track's title, artist, album, and SMTC
thumbnail as a DMAP `SET_PARAMETER` followed by an image `SET_PARAMETER`
whenever a session is streaming; there is no switch, and the card's display
setting does not gate it. The app keys each push on the text plus an FNV-1a
hash of the cover bytes, so a thumbnail that arrives or changes after the
text is sent again in the same session.

Each session's AUMID is resolved through the shell: the Start menu's
Applications folder is enumerated once, and a session matches an item by
exact AUMID, executable file name or display name, or the token before a
profile suffix. The item supplies the localized name and a 64×64 PNG icon.
An app outside that folder falls back to its running executable and version
resource, then to the known-app map; without an icon the card shows the name
alone. The result is cached per AUMID, so the two-second poll does not touch
the shell again.

Each poll adopts the system's current session as the card's selection; the
switch arrow pins another session until the current session changes or the
pinned one disappears. The light preview reads that same current session's
cover — the one the receiver push sends — so a manual card switch on the
flyout cannot move the preview away from the top display.

## Package map

| Package | Responsibility |
|---|---|
| `cmd/saundo` | Entry point; `mygo.json` carries name, version, and the embedded icon (`resources/icon.png`) |
| `internal/app` | mygo UI: tray flyout (`flyout.go`, adaptive height, with the ambient-light card in `lamp.go`), settings window (`settings_window.go`, nine pages behind a 190 sidebar, with the 正在播放 page in `settings_window.go` and the 关于 page in `about.go`), the log dialog (`log_dialog.go`), the ambient-light page (`lamp.go`), the receiver-management page and dialogs (`receivers_page.go`, `receivers_dialogs.go`), the update service and notes dialog (`update.go`, `update_dialog.go`, `update_history.go`, state shared with the settings 更新 page and the flyout chip; disabled on darwin until a signed mac feed exists), the tray flyout window creation and the style hot-swap (`flyout_window.go`), tray and menu icons, the phase/state-face derivation and shared widgets with their motion primitives (`ui_kit.go`), palette mapping (`theme.go`), and the airgoplay→saundo data migration (`migrate.go`). The design language these implement is in [ui.md](ui.md) |
| `internal/engine` | UI-agnostic session controller: command handling, transport lifecycle, stereo groups (`group.go`), receiver management (`receivers.go`, `receivers_cmd.go`), silence meter and suspend (`silence.go`), capture level tap and delayed frames (`LevelFrame`), the local-output mute and its crash guard (`mute.go`), the ambient-light image cache and application (`engine.go`), diagnostics aggregation (`diagnostics.go`), snapshots |
| `internal/discovery` | mDNS browse plus a directed unicast fallback (ARP neighbors and remembered hosts, every fifth cycle), TXT parsing, `_raop`/`_airplay` merge, stereo-group collapse (`tsid`/`igl`/`gpn`), offline marking |
| `internal/capture` | WASAPI shared-mode loopback (`capture_windows.go`), per-app process loopback (`app_windows.go`), the macOS Core Audio process tap aggregated with the output device (`capture_darwin.go`) and its device mute (`mute_darwin.go`), format conversion (`convert.go`), and the render endpoint's mute state (`mute_windows.go`) used by the connection mute |
| `internal/darwin` | The macOS FFI layer, pure Go through purego: the Objective-C bridge (`objc.go`), the login-Keychain SecItem calls (`keychain.go`), Core Audio bindings (`coreaudio.go`), and LaunchServices app identity (`appinfo.go`). Serves the darwin leaves only; the UI keeps to mygo |
| `internal/audio/ring` | Bounded overwrite-oldest SPSC `Buffer` and multi-sink `Fanout` (`KeepNewest` trim, drop counters) |
| `internal/audio/analyze` | Capture-block level and five-band Goertzel meter with a -50 dBFS noise gate, a floored per-band AGC, deadband smoothing, and a delayed frame ring for the Now Playing card |
| `internal/audio/resample` | Linear 48 kHz → 44.1 kHz conversion for interleaved stereo int16 |
| `internal/airplay/crypto` | Wire primitives: SHA-512/HMAC/HKDF, ChaCha20-Poly1305, X25519/Ed25519, SRP-6a, TLV8, bplist, Digest |
| `internal/airplay/alac` | Uncompressed ALAC frame encoder (`EncodeUncompressed`) |
| `internal/airplay/io` | Package `raopio`: the `IO` seam, event types, and the Go net `Host` |
| `internal/airplay/sender` | The AP2/AP1 state machine: pairing (`pairing.go`), RTSP control (`rtsp.go`), session and event channel (`ap2.go`), RTP/ALAC/pacing (`stream.go`), live-edge start (`start_gate.go`), diagnostic counters (`stats.go`), and the SETUP-response Wi-Fi snapshot (`wifi.go`) |
| `internal/airplay/ptp` | `Follower` for receivers that keep their own grandmaster (`ptp.go`), sender-side `Master` for stereo pairs (`master.go`) |
| `internal/airplay/creds` | HAP long-term credentials; DPAPI-protected JSON on Windows (`dpapi_windows.go`), Keychain-master-key AES-256-GCM on macOS (`dpapi_darwin.go`) |
| `internal/airplay/device` | Receiver description, auth method, receiver family, and the top-display capability gate (`TopDisplayCapable`) shared by discovery, engine, and UI |
| `internal/lamp` | Six three-color palette templates whose 600×600 carriers are flat color fields, the Lab k-means cover palette used by the preview (`cover.go`), and a lazy per-palette JPEG cache |
| `internal/receivers` | The receiver registry: `receivers.json` store (atomic, versioned, single writer), the pure merge/visibility/auto-target/latency functions, the manual-receiver spec, and the settings migration. Rows key by deviceid, `stereo:<tsid>`, or `m:<uuid>` |
| `internal/nowplaying` | Windows SMTC reader (sessions, media properties, timeline, artwork) with shell-resolved app names and icons (`appidentity_windows.go`); macOS reads the system session through the vendored MediaRemote adapter (`nowplaying_darwin.go`, `native/mediaremote-adapter`) with LaunchServices identity; transport commands (per-platform build split) |
| `native/mediaremote-adapter` | Vendored BSD-3 helper (CMake, ObjC): a framework that reads Now Playing inside `/usr/bin/perl`, because MediaRemote rejects third-party processes since macOS 15.4; built by `scripts/build-adapter.sh` into `cmd/saundo/resources/darwin-arm64/adapter` — see [platforms.md](platforms.md) |
| `internal/settings` | JSON configuration store; the Now Playing panel options (`showNowPlayingPanel`, `playbackControls`, `coverColors`, `coverMode` (none/placeholder/media), `audioLevel`), the local-output mute (`muteLocalOnConnect`), the flyout style (`flyoutPopover`, macOS), and the ambient light (`lamp.off`, `lamp.cover`, `lamp.preset`) all default on, with cover priority and the dune template; the receiver push is unconditional |
| `internal/diagnostics` | Rolling in-memory log with 5 MB file rotation |

The `Engine` methods are the UI's whole surface: `Start`, `Stop`,
`Snapshot`, `Refresh`, `Connect`, `Disconnect`, `SetVolume`, `SubmitPIN`,
`DismissPasswordPrompt`, `SetReceiverPassword`, `SetCaptureEndpoint`,
`SetCaptureProcess`, `SetLatency`, `ApplyLatency`, `SetLatencySettings`,
`SetStrictAuth`, `SetName`, `SetReconnect`,
`SetMuteLocal`, `SetNowPlaying`, `LevelFrame`, `SetLamp`, and the receiver-management commands
`AddReceiver`, `UpdateReceiver`, `RemoveReceiver`, `SetReceiverAutoConnect`,
`SetReceiverHidden`, `SetReceiverDelay`. `SetLatency` applies on the next
session; `ApplyLatency` restarts a live one; `SetLatencySettings` keeps the
app-only latency mode fields in the engine settings copy. `SetLamp(off, cover,
preset)` saves the global `lamp.off`/`lamp.cover`/`lamp.preset` choice, applies
it to a live session, and is reapplied when a session starts streaming. The
snapshot carries both lists: `Devices` (the flyout, with hidden receivers
filtered out while the active session's row stays) and `Receivers` (every row
with its settings, no secrets — only whether a password exists); it also
carries `LampOff`, `LampCover`, and `LampPreset`, which both lamp surfaces
read.

`Snapshot.Diagnostics` is the diagnostics model. The sender exposes
`Stats` (retransmits served, late pacer ticks, underruns, sent blocks, last
timing packet, ring buffer milliseconds, and the SETUP Wi-Fi snapshot); the
engine calls it on the event loop and folds it once per publish. The engine
owns the group rules: Wi-Fi, late ticks, and buffer take the worst member,
resends and underruns sum, the overflow figure sums the members' session
drops, and stability uses the max drop the connection card already shows. A
stereo pair's clock reads PTP master and its offset and drift show —. The
engine folds the model on each snapshot publish; no UI consumes it yet. The
Wi-Fi object is the
stream SETUP response's `wiFiMetrics` (capital F: `rssi`, `ssid`, and
`wifiBand` 1/2/3 for 2.4/5/6 GHz); it is a device-side snapshot that can be up
to 30 minutes old, and a missing object reads unknown.

A clean control close during a streaming session is a takeover, not a
retryable failure. `Sender` reports it through `Events.Preempted`;
`Engine.onPreempted` sets `Snapshot.TakenOver`, clears the pending reconnect,
and stops the session. A stereo group forwards each member's preemption to
the same callback, so one member's takeover stops the pair. A user connect or
disconnect clears the flag. A close carrying an error stays on the normal
failure path.

The registry remembers every first sighting, including a stereo pair under
its `stereo:<tsid>` key, so its hidden, delay, password, and auto-connect
settings survive the pair going offline. Exactly one entry carries
`AutoConnect`; at launch the engine connects that target once, immediately
for a manual row and on the first snapshot that reports a discovered one
online. A user connect or disconnect ends the launch wait, and a failed
launch falls into the ordinary reconnect policy. The global
`settings.AutoConnect` flag is a migration input only.

Each remembered row also stores its last advertised host and port (a stereo
group stores the leader's) and a `lastSeen` timestamp. Discovery seeds its
directed pass from those addresses, so a restart can probe a receiver whose
multicast never reaches the PC. A `stereo:<tsid>` row unseen for a day is
pruned with its auto-connect flag: splitting and re-merging a pair mints a
new tsid, so the old key never returns. A row without `lastSeen` is stamped on
the first launch and is not pruned at upgrade.

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
   block. The engine reapplies the ambient light once the session streams:
   the manual 关灯 sends the paused playback state; cover priority hands the
   slot to the media cover and keeps the display lit, and a template owns the
   slot with its image. A media pause keeps the current light. With cover
   priority on and no media cover, the sender stops sending an image and the
   receiver keeps its last light; a fresh session with no artwork shows the
   device's own white-center placeholder. The sender keeps one artwork slot:
   a template owns it or cover priority hands it to the media cover; off is a
   playback state independent of the slot. It resends the effective cover
   and state at every stream start, so a reconnect does not lose the top
   display. The cover leaves as a Now Playing image on the encrypted control
   channel (`SET_PARAMETER image/jpeg`); there is no separate artwork HTTP
   connection.

## On-disk state

All under `%APPDATA%\saundo\` on Windows and `~/Library/Application Support/saundo/` on macOS:

- `settings.json` — configuration (`internal/settings`), including the update preferences (automatic checks and downloads, skipped version, last check), the tray animation switch, the flyout style (`flyoutPopover`, macOS), the language and appearance choices, the Now Playing panel options, the local-output mute, and the global ambient-light choice (`lamp.off`, `lamp.cover`, `lamp.preset`). The mute and the light default on; `lamp.off` defaults to false, `lamp.cover` to true, and the preset to `dune` (沙丘). An old `lamp.enabled` is inverted into `lamp.off`; an old `preset: "media"`, an empty preset, or an unknown value turns cover mode on and resets the template to `dune`; a valid template keeps cover priority off unless the file also stored `cover: true`. The legacy `origin` value is handled by the same fallback. The legacy `autoConnect` flag and `passwords` map stay decodable but unread: migration seeds the registry's auto-connect entry and moves passwords to DPAPI.
- `receivers.json` — the receiver registry (`internal/receivers`): one entry per remembered receiver, stereo pair, or manually added receiver, with hidden, auto-connect, delay, last address, and sighting-time fields. Written atomically, versioned, and only by the engine goroutine (the startup migration runs before the engine exists). A file that fails to parse, cannot be read, or was written by a newer schema is never overwritten.
- `credentials.dat` — protected credential JSON keyed by receiver
  `deviceid` (`internal/airplay/creds`), holding the HAP long-term keys plus receiver passwords under the `password:` namespace. DPAPI protects it on Windows; on macOS a Keychain-held master key encrypts it with AES-256-GCM.
- `mute-guard.json` — written before the engine mutes the local render
  endpoint and removed after it releases it. The connection mute is temporary
  and never outlives the session; a crash or forced kill leaves the guard for
  the next launch to release the endpoint (`internal/engine/mute.go`).
- `saundo.log` (+ `saundo.log.1` rotation) — diagnostics.
- The first run after the airgoplay rename copies `settings.json` and
  `credentials.dat` from `%APPDATA%\airgoplay` on Windows (`internal/app/migrate.go`).
  The same startup runs the receiver migration: plaintext passwords move
  into DPAPI (a password already in DPAPI wins), and the old auto-connect
  flag plus `lastDeviceId` seed one auto-connect entry. Both steps are
  idempotent.

## Build and packaging

`mingw32-make build` on Windows and `make build` on macOS run
`scripts/build.sh`, which branches on `uname -s`. Both run `mygo build` with
`MYGO_LDFLAGS` stamping `saundo/internal/app.Version` from `git describe`,
the icon and manifest embedded from `cmd/saundo/mygo.json`. The app reports
the stamp to the updater, so a local build past a release sorts below it and
still offers that release; `VERSION` overrides the stamp. The Windows branch
builds `windows/amd64`, stops the running app, copies `saundo.exe` (GUI
subsystem) and the NSIS `saundo-setup-*.exe` into `bin/`, and starts the new
build; `--fast` skips the installer. The macOS branch builds `darwin/arm64`,
stages the MediaRemote adapter, signs ad hoc unless `SAUNDO_MAC_SIGN_IDENTITY`
names an identity, replaces `bin/saundo.app`, and opens it.
`mingw32-make platforms` (or `go run ./scripts/check-platforms`) builds and
vets both targets on either host and rejects platform imports outside tagged
files; it runs inside `make check` and in CI.
