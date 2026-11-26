<!-- reviewed-through: 2026-11-26 -->

# Implementation overview

Orientation for a session landing in this codebase: what saundo implements
today, where each piece lives, and how the parts talk. AGENTS.md carries the
rules; [architecture.md](architecture.md) the design rationale;
[protocol.md](protocol.md) and [devices.md](devices.md) the wire protocol and
receiver profiles; [ui.md](ui.md) the UI design language;
[verification.md](verification.md) the evidence. This page describes the code
as it is; update it in the same change that makes it wrong.

The native UI uses mygo (`github.com/egoist/mygo` v0.10.0). The
[mygo guides](https://mygo.run/zh/docs) cover windows, widgets, packaging, and
updates. The [`ui` package reference](https://pkg.go.dev/github.com/egoist/mygo@v0.10.0/ui)
documents the API.

## Runtime model

One process, four primary goroutine roles, no shared mutable state — data
moves through bounded channels. The engine also runs a mute worker and a
level-analysis worker, both behind bounded queues:

- **The mygo main thread** owns the tray flyout, the settings window, the
  tray icon, and all UI state. Engine snapshots arrive as `win.Update`
  callbacks (`internal/app`). While a session is connecting, pairing,
  streaming, or reconnecting, the main thread also holds
  `mygo.Power.KeepRunning` so macOS does not put the hidden tray app into
  App Nap; the hold is released when the session returns to idle or the
  receiver is taken over, and before `Engine.Stop` on quit. Other systems
  no-op.
- **The engine goroutine** (`internal/engine`, `Engine.run`) is the only
  caller of sender methods. It drains the command channel, runs the timers
  (reconnect backoff, the idle suspension and the long-idle rebuild, capture restart), renders the
  ambient-light JPEG off the sender's tick path, and publishes a
  `Snapshot` on every state change. A cohort member that dies
  signals the loop through the transport's `Dead` channel, and the loop
  runs `closeSession` itself, so the cohort teardown also happens on this
  goroutine — never inside a sender callback and never on another
  goroutine.
- **The sender** (`internal/airplay/sender`) is sans-i/o and owns no
  goroutine: the engine goroutine drives it. `internal/airplay/io` (package
  `raopio`) defines the six-method `IO` seam (TCP connect/send/close, UDP
  bind/send/close) and `Host`, the Go net implementation whose reader
  goroutines push connection, data, and close events through one `Events`
  channel; the engine loop pulls timers with `NextDeadline` and `Tick`. PTP
  runs its own loop, and a cohort adds one forwarder per member.
- **The capture goroutine** owns the platform capture leaf (WASAPI on
  Windows, the Core Audio tap on macOS), converts and resamples, and writes
  the ring buffer or the stereo fanout. It never touches the network.

The app also polls the platform Now Playing source every two seconds
(`internal/nowplaying`): SMTC on Windows, the MediaRemote adapter on macOS.
It feeds the Now Playing card and its transport buttons. The card's progress
clock (`internal/app/now_progress.go`) advances the position from the session's
playback timestamp, not from the poll's arrival time. `Session.PositionAt` is
that anchor: the macOS leaf sets it from MediaRemote's `timestampEpochMicros`,
where the elapsed value is stamped, while Windows leaves it zero because SMTC
exposes no sampling instant. Apple Music refreshes both values only every few
seconds and the elapsed value stays frozen between notifications, so polling
against the arrival time rewound the display and made the clocks bounce. Only a
session whose `Advancing` flag is set extrapolates:
MediaRemote can briefly publish `playing=true` with `playbackRate=0` and a cached
stamp, so the playing flag alone does not mean the timeline is moving. The
extrapolation is also capped at 30 seconds. Capture blocks are measured
into level and five-band frames (`internal/audio/analyze`) on the engine's
level pump goroutine, fed by a 100 ms ring from capture; `Engine.LevelFrame` reads them back with the presentation delay so
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
| `internal/app` | mygo UI: tray flyout (`flyout.go`, adaptive height, with the ambient-light card in `lamp.go`), settings window (`settings_window.go`, nine pages behind a 190 sidebar, with the 正在播放 page in `settings_window.go` and the 关于 page in `about.go`; its creation in `settings_window_create.go`), the log dialog (`log_dialog.go`), the ambient-light page and its idle standby row (`lamp.go`), the receiver-management page and dialogs (`receivers_page.go`, `receivers_dialogs.go`, including the disabled 本机 row for this host's own receiver), the update service and notes dialog (`update.go`, `update_dialog.go`, `update_history.go`, state shared with the settings 更新 page and the flyout chip; the same path on both platforms, each updating from its own signed manifest), the tray flyout window creation (`flyout_window.go`), window sizing and the text-scale zoom (`window_scale.go`), the tray menu model and native menu (`tray_menu.go`), the settings preview's sample card (`now_playing_preview.go`), tray and menu icons, receiver glyphs (the filled set in `icons_sf_gen.go`), the phase/state-face derivation and shared widgets with their motion primitives (`ui_kit.go`), palette mapping (`theme.go`), and the airgoplay→saundo data migration (`migrate.go`). The design language these implement is in [ui.md](ui.md) |
| `internal/engine` | UI-agnostic session controller: command handling (`commands.go` carries the command vocabulary), the session plan and its cohort transport (`plan.go`, `cohort.go`), per-target volume and channel commands and the target control projection (`target_controls.go`), receiver management (`receivers.go`, `receivers_cmd.go`), the stereo-combo lifecycle — link two receivers, unlink, swap sides in place (`combo.go`), silence meter (`silence.go`), the idle phase and its dark standby, capture level tap and delayed frames (`LevelFrame`), the local-output mute and its crash guard (`mute.go`; endpoint and guard-file calls run on a dedicated worker, never the event loop; a per-app tap takes its own mute instead), the multi-app selection (`captureApps`, persisted as bundle IDs), the ambient-light image cache and application (`engine.go`), diagnostics aggregation (`diagnostics.go`), the launch cache and duplicate/prefill policy (the ratio lands on every close and a pair or combo is sized per member), the capture retry backoff, snapshots |
| `internal/discovery` | mDNS browse through a logger that drops the client's repeated IPv6 bind failures, plus a directed unicast fallback (ARP neighbors and remembered hosts, every fifth cycle), TXT parsing, `_raop`/`_airplay` merge, stereo-group collapse (`tsid`/`igl`/`gpn`), offline marking (the window is at least one directed-pass period plus its timeout), and local marking when an advertised address belongs to this host (`device.Device.Local`) |
| `internal/capture` | WASAPI shared-mode loopback (`capture_windows.go`), the Windows per-app process-loopback leaf (`app_windows.go`, not offered in the product; see platforms.md), the macOS Core Audio process tap aggregated with the output device, including the selected apps' stereo mixdown resolved from bundle IDs and its own local mute (`capture_darwin.go`, `SetProcessMuted`), and its device mute (`mute_darwin.go`), format conversion (`convert.go`), the 30-second device-retry backoff, and the render endpoint's mute state (`mute_windows.go`) used by the connection mute |
| `internal/darwin` | The macOS FFI layer, pure Go through purego: the Objective-C bridge (`objc.go`), the login-Keychain SecItem calls (`keychain.go`), Core Audio bindings (`coreaudio.go`), and LaunchServices app identity (`appinfo.go`). Serves the darwin leaves only; the UI keeps to mygo |
| `internal/audio/ring` | Bounded overwrite-oldest SPSC `Buffer` and multi-sink `Fanout` (`KeepNewest` trim, drop counters) |
| `internal/audio/analyze` | Capture-block level and five-band Goertzel meter with a -50 dBFS noise gate, a floored per-band AGC, deadband smoothing, and a delayed frame ring for the Now Playing card |
| `internal/audio/resample` | Linear 48 kHz → 44.1 kHz conversion for interleaved stereo int16 |
| `internal/airplay/crypto` | Wire primitives: SHA-512/HMAC/HKDF, ChaCha20-Poly1305, X25519/Ed25519, SRP-6a, TLV8, bplist, Digest |
| `internal/airplay/alac` | ALAC frame encoder: uncompressed frames for real audio and the compact compressed silence frame (`EncodeCompactSilence`) |
| `internal/airplay/io` | Package `raopio`: the `IO` seam, event types, and the Go net `Host` |
| `internal/airplay/sender` | The AP2/AP1 state machine: pairing (`pairing.go`), RTSP control (`rtsp.go`), session and event channel (`ap2.go`), RTP/ALAC/pacing (`stream.go`), the stereo-to-mono channel mix (`channel.go`), live-edge start (`start_gate.go`), diagnostic counters (`stats.go`), and the SETUP-response Wi-Fi snapshot (`wifi.go`) |
| `internal/airplay/ptp` | `Follower` for one receiver that keeps its own grandmaster (`ptp.go`), `Followers` for several standalone receivers over one shared socket pair (`followers.go`), sender-side `Master` for stereo pairs (`master.go`) |
| `internal/airplay/creds` | HAP long-term credentials; DPAPI-protected JSON on Windows (`dpapi_windows.go`), Keychain-master-key AES-256-GCM on macOS (`dpapi_darwin.go`) |
| `internal/airplay/device` | Receiver description, auth method, receiver family, the Mac model-identifier table (`macmodel.go`, `MacFormFor`/`MacModelName`), the four HomePod identifiers (`homepodmodel.go`, `HomePodModelName`), the Apple TV identifiers (`appletvmodel.go`, `AppleTVModelName`), and the top-display capability gate (`TopDisplayCapable`) used by the engine's lamp and stereo-group paths |
| `internal/lamp` | Six three-color palette templates whose 600×600 carriers are flat color fields, the Lab k-means cover palette used by the preview (`cover.go`), and a lazy per-palette JPEG cache |
| `internal/receivers` | The receiver registry: `receivers.json` store (atomic, versioned, single writer), the pure merge/visibility/auto-target/latency functions, the local-receiver rule (forced hidden, no auto-connect), the manual-receiver spec, the settings migration, the cached stereo members and cached-device builder, the stereo-combo rows (a combo entry links two member keys; the merge synthesizes one stereo-group row over the members' own rows, and `DropLinked` hides the member rows at the projection point), and the duplicate policy. Rows key by deviceid, `stereo:<tsid>`, `combo:<hex>`, or `m:<uuid>` |
| `internal/nowplaying` | Windows SMTC reader (sessions, media properties, timeline, artwork) with shell-resolved app names and icons (`appidentity_windows.go`); macOS reads the system session through the vendored MediaRemote adapter (`nowplaying_darwin.go`, `native/mediaremote-adapter`) with LaunchServices identity; transport commands (per-platform build split) |
| `native/mediaremote-adapter` | Vendored BSD-3 helper (CMake, ObjC): a framework that reads Now Playing inside `/usr/bin/perl`, because MediaRemote rejects third-party processes since macOS 15.4; built by `scripts/build-adapter.sh` into `cmd/saundo/resources/darwin-arm64/adapter` — see [platforms.md](platforms.md) |
| `internal/settings` | JSON configuration store; the Now Playing panel options (`showNowPlayingPanel`, `playbackControls`, `coverColors`, `coverMode` (none/placeholder/media), `audioLevel`), the local-output mute (`muteLocalOnConnect`), the per-app capture selection (`captureAppIds`, `captureAppNames`; bundle IDs on macOS), the ambient light (`lamp.off`, `lamp.cover`, `lamp.preset`), and the idle standby (`standby.enabled`, `standby.idleMinutes`) all default on, with cover priority, the dune template, and a 30 minute standby; the receiver push is unconditional |
| `internal/diagnostics` | Rolling in-memory log with 5 MB file rotation |

The `Engine` methods are the UI's whole surface: `Start`, `Stop`,
`Snapshot`, `Refresh`, `Connect`, `Disconnect`, `SetTargetVolume`, `SubmitPIN`,
`DismissPasswordPrompt`, `DismissCredentialReset`, `SetReceiverPassword`, `SetCaptureEndpoint`,
`SetCaptureApps`, `SetLatency`, `ApplyLatency`, `SetLatencySettings`,
`SetStrictAuth`, `SetName`, `SetReconnect`,
`SetMuteLocal`, `SetNowPlaying`, `LevelFrame`, `SetLamp`, `SetStandby`, `SetTrayAnimation`, and the receiver-management commands
`AddReceiver`, `UpdateReceiver`, `RemoveReceiver`, `SetReceiverAutoConnect`,
`SetReceiverHidden`, `SetReceiverDelay`, and the stereo-combo commands
`LinkReceivers`, `UnlinkCombo`, `SwapComboSides`. `SetTargetVolume` stores and applies
one target's volume; the primary target's value also updates the global
default a target with no remembered volume adopts. `UpdateReceiver` with a `Channel` patch saves the side and applies it
to the live target's members; a stereo pair and a combo ignore a non-stereo value.

A stereo combo (`combo.go`) links two independent AirPlay 2 receivers into one
target: the combo key owns one volume, one auto-connect flag, one hidden flag,
and one delay; the members keep their own credentials and endpoints. The plan
treats a combo like a remembered stereo pair minus the pair's own protocol —
no group UUID, no SETPEERS, one input side per member, and each member times
against its own grandmaster (both hosts join the follower set, never a
sender-side master). Linking a receiver that plays ends its session first;
unlinking deletes the combo row and restores the member rows; swapping sides
reorders the registry members and hot-applies the opposite channel per member
without a rebuild. Removing a receiver unlinks any combo that held it; a
combo's launch flag returns to the surviving members. Linking seeds the combo
delay from the larger member delay and leaves the volume unset, so the first
session adopts each receiver's own level and then converges both members on
it. A member lost mid-stream is a target loss: the combo leaves the session
and a combo-only session ends and retries, while a multi-target session
continues with the other targets. A takeover of one member isolates the same
way; only a one-target session is marked taken over. A combo over two PIN-auth
receivers shares one PIN dialog, so only one on-screen code can be entered;
combos of PIN-paired receivers are a known limitation.
`SetLatency` applies on the next
session; `ApplyLatency` restarts a live one, but only after checking every
member's device — one missing or offline receiver fails the apply with the
error and leaves the running cohort alone, because the rebuild would open a
cohort that cannot launch. `SetLatencySettings` keeps the
app-only latency mode fields in the engine settings copy. User-facing
failures carry a `UserErrorKey` plus arguments in
`Snapshot.ErrorKey`/`ErrorArgs`; `internal/app/user_error.go` resolves them
against the catalog, so the engine never builds a localized sentence.
`Snapshot.Error` stays the technical fallback for a failure with no key. `SetLamp(off, cover,
preset)` saves the global `lamp.off`/`lamp.cover`/`lamp.preset` choice, applies
it to a live session, and is reapplied when a session starts streaming. The
snapshot carries both lists: `Devices` (the flyout, with hidden receivers
filtered out while every active target's row stays) and `Receivers` (every row
with its settings, no secrets — only whether a password exists); `SelectedID`
stays the primary target, `ActiveTargets` lists the live or wanted keys in
target order, `Active(key)` answers a row, and `ConnectedName` joins the
target names with ` + `. `Targets` carries one `TargetControl` per active key
(name, volume, channel, stereo flag, its `TargetState`, and a `TargetHealth`
that folds the target's members' retransmits, sent blocks, and underruns; a
stats slice that does not line up with the plan leaves health at zero rather
than attributing one member's counters to another), and `Target(id)`
answers one; a wanted key keeps an entry while a retry waits, so every active
row has a state, and the list is empty while idle. `TargetState` names the
row's role in the current attempt: `TargetJoining` for the key the user's
connect started or added (`sessionPlan.joinKey`, dropped when the launch
resolves), `TargetMember` for every other planned key, `TargetReopening` for a
wanted key while an automatic retry waits, and `TargetFailed` once it stops.
A rebuild carries the join key only while that target stays planned.
The snapshot also
carries `LampOff`, `LampCover`, and `LampPreset`, which both lamp surfaces
read, and `Standby` (the live dark phase) with the configured
`StandbyEnabled` and `StandbyMinutes`.

The app reads a session through one layer, `sessionMembers` in
`internal/app/session_members.go`: the active targets in snapshot order, each
with its device row and published controls, plus the joined session name and
the slowest member's effective latency. The tray, the connection page, the
settings sidebar, and the playback head's latency line read that layer instead
of a single selected device; the flyout already renders one row per member.
The connection page draws two sections from it: 已连接设备 lists the members
with their channel, volume, and state, naming retransmits and trailing-window
underruns only when notable, and 本次连接 reads `Snapshot.Diagnostics` for
uptime, stability, the session retransmit share, and reconnects while the
session streams. A dropped member leaves the list and appears as the engine
notice's reconnect row. Neither card restates a setting or a device constant. Two selection
semantics stay distinct: the engine's `SelectedID` is the primary target — the
session's first key, which guards the reconnect policy and anchors the tray's
retry — while the app's `selID` is the panel's view selection, seeded from the
last session's first target, moved by clicks, and kept on a listed device.

`Snapshot.Diagnostics` is the diagnostics model. The sender exposes
`Stats` (retransmits served, late pacer ticks, underruns, sent blocks, encrypt
and send errors, PTP sync packets skipped while the timeline was not ready,
last timing packet, ring buffer milliseconds, and the SETUP Wi-Fi snapshot);
the engine calls it on the event loop and folds it once per publish. An
underrun
is a packet padded with silence while the ring was still being fed real
audio and the source came back within the window — roughly two start
cushions after the last fed packet. A source that stays away is a pause: its
short fills are discarded and its silence run never counts, so connecting
with nothing playing and every pause read zero. The engine owns the group
rules: Wi-Fi, late
ticks, and buffer take the worst member, resends sum, and the dropped-sample
source sums the member rings' session drops. A stereo pair reports the oldest
member's sync age, and `SyncValid` is false when any member has not sent a
timing packet. The group shape comes from the
plan: one stereo target reports `GroupStereo`, one single `GroupSingle`, and
more than one target `GroupCohort` with the sender count as its size.

The stability verdict is a trailing-window reading, not a session total.
`sessionStability` (`internal/engine/stability.go`) samples each sender's
cumulative underruns, the session's dropped-sample count, and the members'
send and encrypt errors once per publish.
`RecentUnderruns`, `RecentDropped`, `RecentSendErrors`, and
`RecentEncryptErrors` are the growth over the last 60 seconds, and `Degraded`
reads those four; `TargetControl.Health.RecentUnderruns` uses
the same per-member samples. Joining a receiver or rebuilding the cohort can
pad a packet or two with silence, and that blip stops reading as 有卡顿 once
it ages out. A counter that moves backwards means its sender session
restarted, so its window starts over. While a session streams the engine
publishes diagnostics every 5 seconds (`statsWake`), which is what expires a
window on a quiet session; the engine folds the model on each snapshot
publish. The Wi-Fi object is the stream SETUP response's `wiFiMetrics` (capital F: `rssi`, `ssid`, and
`wifiBand` 1/2/3 for 2.4/5/6 GHz); it is a device-side snapshot that can be up
to 30 minutes old, and a missing object reads unknown.

A clean control close during a streaming session is a takeover, not a
retryable failure, but only while the event channel is still up: a receiver
whose event port never accepted the connection, or whose event channel closed
first, tears a session without one down after roughly 30 s, and that close
stays a retryable failure. `Sender` reports the takeover through `Events.Preempted`;
`Engine.onPreempted` sets `Snapshot.TakenOver` and clears the pending
reconnect, and the session's dead signal makes the engine goroutine run
`closeSession`, which stops the transport outside the sender callback. A
cohort forwards each member's preemption to the same callback, so one
member's takeover stops the whole cohort. A user connect or disconnect clears
the flag. A close carrying an error stays on the normal failure path and
signals dead the same way.

The registry remembers every first sighting, including a stereo pair under
its `stereo:<tsid>` key, so its hidden, delay, password, and auto-connect
settings survive the pair going offline. Any number of entries can carry
`AutoConnect`, and the flagged set is what a launch connects: the engine
opens them as one cohort in registry order. The set launches immediately
when every flagged row is addressable — a manual row, a discovered row
online, or a remembered row with a cached address, members, features, and
auth — and otherwise on the first snapshot that completes the set; a row
that no longer resolves or is structurally unusable (a stale unpaired
group, the local receiver) drops with a line, and a set reduced to nothing
leaves the launch idle. `settings.LastTargets` — the last session's target
keys, recorded in target order at every launch — seeds the UI's
pre-session selection. A user
connect or disconnect ends the launch wait, and a failed
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
receiver's DSP/DAC delay, clamped to 20–4000 ms. The app shows that value in
the flyout's playback head for the active receiver. A delay
change to the active receiver restarts its session; the engine keeps the
per-device delay data and the `UpdateReceiver` delay path for a later UI. The
per-device delay is the sync control across device types. The settings
receiver row keeps the volume slider and the channel segment; the per-device
delay row was removed from settings at the user's request. The flyout's live
device rows carry each target's volume slider, a single receiver's row adds
the icon-only channel button, and a stereo target draws no channel control
because its members play one side each; the playback card keeps the latency
switch and is left out when it has neither media nor that row.

Volume and stereo side are per target. `buildPlan` resolves each target's
volume from `VolumeByDevice` and its channel from the registry entry, forcing
stereo for a pair. A remembered volume reaches the members at cohort
construction; a target with no remembered value adopts each member's reported
`initialVolume` when the session launches. Each flyout device row's slider
controls that target and names it; a hardware knob report updates only that
target's value. The row's speaker button mutes by taking the target volume to
zero and restores the level it remembered in the app (`mutedVolume`, session
state only). A single receiver's channel button cycles stereo → left → right
and saves the patch; a stereo target has no channel button. A row's 断开连接 sends `DisconnectTarget`: the engine drops that key,
rebuilds the cohort with the rest while holding the mute, and the last key
takes the full-disconnect path. The flyout draws a member's mixer for as long
as the target is a member, so a rebuild never takes an established row's
controls away, and its action column keeps one width. The row's action is a
table on the published `TargetState`: the joiner offers 取消, a member and a
reopening target offer 断开连接, a failed target offers 重试, and a device with
no role falls back to the phase (connect, disabled connect, or offline). The app
keeps no session markers; the engine's plan carries the joiner
(`sessionPlan.joinKey`) and clears it when the launch resolves. A channel change saves to the registry and calls
`SetTargetChannel` on the live target's members without a rebuild, because the
sender mixes each packet after resampling and before ALAC. A pair ignores a
non-stereo channel patch. The app holds the
last streaming snapshot for up to 3 s while an apply restarts, so the flyout
keeps its streaming faces; only the latency row changes and a spinner follows
the playback label. Preset clicks debounce the restart for 300 ms and custom
edits for 500 ms, so rapid switching coalesces into one restart with the final
mode. The connection mute survives a rebuild: a latency change or an added
target keeps the local output muted across the gap, because the mute belongs
to the user's session rather than to the transport. Only a rebuild that cannot
open its new cohort restores it. A Connect on another online row while the
cohort streams adds that receiver and rebuilds every member from a new plan
with fresh per-member latency; an active row offers Disconnect. Adding a
target that cannot run — an unsupported clock mix, a local receiver, an
offline pair, or a FairPlay receiver — publishes its error and leaves the live
cohort playing, and the error bar shows it even while the session keeps
streaming. The wanted key set survives a failed launch, so Retry on any active
row retries the whole request. A rebuild whose new cohort cannot open restores
the endpoint, whether the failure is synchronous or reported at launch. A
member that closes while the cohort runs does not take the session with it:
the transport records the lost target, a cohort with other targets rebuilds
without it, and the lost target leaves the wanted set so no retry includes it.
A single-target session keeps the close-and-retry path. A plain launch failure
follows the same isolation rule once another independent target already
streams: the failed target leaves the session and the survivors rebuild, while
a failure with no survivor retries the whole set. A stereo pair is one target,
so a streaming sibling is not a survivor and the pair retries whole. A launch
failure with a refused password is not retried: the sender marks it (`PasswordRejected`, from the RTSP digest replay and the HAP pairing rejection), the engine publishes `ErrPasswordRejected`, opens the password prompt for that target, and schedules no reconnect. The dead arm then drops the refused member and rebuilds the survivors, so the rest of the session keeps playing; a single-target session keeps the prompt without a ladder. The isolated loss publishes a `SessionNotice` naming the target and its survivor count; the flyout shows it with 重试 and clears it on the next user connect or after 8 s. A join in progress needs no engine state: the flyout derives it from a `TargetJoining` member beside an existing one.

## A connect, end to end

1. The UI calls `Engine.Connect(id)`; the command reaches the engine
   goroutine. With a live cohort the command adds the target: the engine
   validates it, rebuilds the plan for every wanted target, and opens one new
   cohort, so the connection mute survives the gap. An active key is a no-op.
2. The engine resolves the target — a device or a collapsed stereo group —
   checks stored credentials, and picks the pairing path from the device's
   auth kind. `buildPlan` then derives every member, its latency, volume and
   channel, pair identity, and the one clock role. For the launch auto-connect, the
   registry's cached address, members, features, and auth let the engine
   connect before discovery answers; discovery refreshes the view in
   parallel. A remembered pair whose members both advertise as standalone
   receivers is skipped (`autoconnect.go`), because the user unpaired it; the
   launch starts idle so one receiver can be connected and the other added. A
   flagged row with no address holds the launch for discovery up to
   `autoPendingWait` (15 s), then drops with a log line and the rest launch;
   the flag stays on the dropped row for the next app start. The cohort's
   per-member `forward` goroutine stamps every event with the member whose
   sockets it arrived on, so a receiver that answers UDP from an interface
   other than the dialed one still reaches its own sender in a multi-member
   session; `HandleEvent` routes by that stamped host and drops only an
   event no member claims.
3. The sender runs the protocol: pair-setup (transient / on-screen PIN /
   receiver password) → pair-verify → encrypted control channel →
   `GET /info` → session SETUP (a password-protected Mac answers 401 and
   gets the Digest replay) → event channel → `RECORD` → PTP readiness
   (a standalone PTP receiver skips SETPEERS; a pair member still sends it) →
   stream SETUP → streaming.
4. Capture starts, or the fanout attaches one ring per cohort member; the one
   `StartGate` waits for every member, trims every ring to the newest 50 ms
   cushion, and seals the shared start epoch; RTP flows at 44100 frames per
   second. The ring holds eight seconds at 48 kHz stereo, enough for capture
   jitter and a stalled consumer; a silent source does not fill it.
5. While streaming: feedback every 2 s, receiver-volume polling per member,
   sync packets, and event-channel replies run on the sender's timers; a
   volume knob report updates only its target, and a channel change reaches
   the next packet. A silent source suspends the pacer after 5 s: the rings
   stop filling and no RTP goes out, while sync, feedback, volume polling,
   and the event channel stay alive. The first audible block resumes with a
   re-anchored timeline from the newest capture cushion and sends a `FLUSH`
   with the next seq/rtptime, so the receiver drops its stale buffer before
   the new point; both HomePods answer `200 OK`. The compact silence
   frame now fills only a packet that starves while the pacer is running (a
   capture hiccup); a receiver that cannot suspend keeps the continuous
   silence stream. The phase reads the source instead: after 5 s of digital
   silence the engine reports `AudioIdle`, and after the configured idle
   threshold counted from the last audible block it turns the top display
   dark. The first audible block resumes the pacer and restores the display;
   a session launched into silence suspends immediately. After a long source
   silence the first audible block rebuilds the cohort instead: a HomePod
   pair at 27.3 stops rendering the realtime stream after such a stretch and
   only a new session revives it (`longIdleRebuildAfter`, 30 minutes, in
   `internal/engine/long_idle.go`). A receiver that kept the continuous
   stream was never starved, so the rebuild skips it.
   The engine reapplies the ambient light once the session streams:
   the manual 关灯 sends the paused playback state; cover priority hands the
   slot to the media cover and keeps the display lit, and a template owns the
   slot with its image. A media pause keeps the current light. With cover
   priority on and no media cover, the sender stops sending an image and the
   receiver keeps its last light; a fresh session with no artwork shows the
   device's own white-center placeholder. The sender keeps one artwork slot:
   a template owns it or cover priority hands it to the media cover; off is a
   playback state independent of the slot. It resends the effective cover
   and state at every stream start, so a reconnect does not lose the top
   display. On a stereo pair member it sends an explicit state 1 at stream
   start, before the first cover, because the follower does not assume
   playing, and re-sends the session's first cover 2 s after that first send,
   because the follower's media-system sync can still be establishing the
   Now Playing item when the first light arrives. The cover leaves as a Now
   Playing image on the encrypted control
   channel (`SET_PARAMETER image/jpeg`); there is no separate artwork HTTP
   connection.

## On-disk state

All under `%APPDATA%\saundo\` on Windows and `~/Library/Application Support/saundo/` on macOS:

- `settings.json` — configuration (`internal/settings`), including the update preferences (automatic checks and downloads, skipped version, last check), the tray animation switch, the language and appearance choices, the Now Playing panel options, the local-output mute, the per-app capture selection (`captureAppIds`, `captureAppNames`), the last session's target combination (`lastTargets`), and the global ambient-light choice (`lamp.off`, `lamp.cover`, `lamp.preset`). The mute and the light default on; `lamp.off` defaults to false, `lamp.cover` to true, and the preset to `dune` (沙丘). An old `lamp.enabled` is inverted into `lamp.off`; an old `preset: "media"`, an empty preset, or an unknown value turns cover mode on and resets the template to `dune`; a valid template keeps cover priority off unless the file also stored `cover: true`. The legacy `origin` value is handled by the same fallback. The legacy `autoConnect` flag and `passwords` map stay decodable but unread: migration seeds the registry's auto-connect entry and moves passwords to the OS secret store. Writes are field-owned: `Store.Update` loads, merges, and saves under one lock; `MergeApp` and `MergeEngine` copy only the fields each side owns; a reflection test requires every field to be classified; and only the two startup saves write the whole file, because they run before the engine exists.
- `receivers.json` — the receiver registry (`internal/receivers`): one entry per remembered receiver, stereo pair, stereo combo (two member keys in playback order), or manually added receiver, with hidden, auto-connect, delay, the stereo channel preference (`channel`: `stereo`, `left`, or `right`; empty and unknown read as stereo), last address, cached member addresses, feature/auth bits, the target's own last session's retransmit ratio (a pair or combo also stores each member's own ratio), and sighting-time fields. Written atomically, versioned, and only by the engine goroutine (the startup migration runs before the engine exists). A file that fails to parse, cannot be read, or was written by a newer schema is never overwritten.
- `credentials.dat` — protected credential JSON keyed by receiver
  `deviceid` (`internal/airplay/creds`), holding the HAP long-term keys plus receiver passwords under the `password:` namespace. DPAPI protects it on Windows; on macOS a Keychain-held master key encrypts it with AES-256-GCM. A file that fails to decrypt is moved to `credentials.dat.corrupt-<UTC time>` and a fresh store is created in its place — but only when the blob itself failed authentication: a key-side failure such as a locked Keychain or a malformed master-key override keeps the file and serves the last known state instead of quarantining a store that would decrypt once the key returns. The next connect logs the quarantined path and says a receiver may need to be paired again.
- `mute-guard.json` — written before the engine mutes the local render
  endpoint and removed after it releases it. The connection mute is temporary
  and never outlives the session; an in-place restart such as a latency change
  holds it across the rebuild, and a crash or forced kill leaves the guard for
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
files; it runs inside `make check` and in CI. `go run ./scripts/check-arch`
enforces the import seams, the concurrency rules, and the dependency allowlist
from AGENTS.md.
`go run ./scripts/check-size` holds the line budget, and `go run
./scripts/verify-map` checks the verification skill's feature map against the
tree. All three run inside `make check` and in CI, together with
`go tool mygo i18n check` and `go vet` under the default, `updateharness`, and
`e2e` tag sets.
