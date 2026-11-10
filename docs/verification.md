<!-- reviewed-through: 2026-11-11 -->

# Verification

This document is the acceptance path for saundo. The automated suite covers
the crypto core and the full sender handshake against an in-process fake
receiver. Real-device behavior is verified with the steps below.

Dated results append below and are never rewritten. They are compressed,
never deleted, once every claim in a dated section has a durable home: a
named test pins it, devices.md or protocol.md records it, the code that owns
the behavior documents it, or a later dated entry supersedes it. The release
audit moves whatever qualifies to the Graduated section at the end of this
file — one entry per batch: date, surviving conclusion, pin. When the file
passes roughly 800 lines, compress at the next opportunity without waiting
for a release.

## Currently pending

Live verification still owed, consolidated from the dated notes below.
Historical "not verified" lines are frozen as of their date; this list is the
live state:

- Apple TV on-screen PIN pairing and stored-credential reconnect — the
  remaining definition-of-done gap.
- Apple TV realtime sessions: the silence suspend deliberately excludes
  AirPlay 1, and an Apple TV AP2 realtime session has not been run.
- A 30-minute soak at the 200 ms production preset with a stable audio
  device and no other app takeover. The 100 ms soak is withdrawn: the
  2026-11-10 presentation-latency-floor entry in Graduated shows the HomePod
  mini pair breaks up below roughly 200 ms whatever the sender.
- Local output mute: the connection page showed the switch on with
  本机已静音；断开或退出时恢复 during the 2026-11-11 live pass, and the
  engine session path passed (`TestLiveMuteLocal`); toggling the switch
  itself has only headless coverage.
- A two-computer takeover with both apps auto-reconnecting: the second
  should show the takeover state, the first should keep streaming, and
  neither should re-enter the reconnect loop. The single-machine two-session
  probe and the engine test cover the classification.
- The diagnostics data in a live session: the SETUP `wiFiMetrics` object and
  its band values, and a stereo pair's worst/sum aggregation. The parsing
  and aggregation are pinned by the sender and engine tests; no live device
  has reported `wiFiMetrics` yet, and no UI consumes the model.
- A real default-device switch during playback.
- Multi-room groups larger than two, a member dropping or reconnecting
  mid-pair, and software stereo groups built from independent receivers.
- The macOS port: per-app capture (the `Config.ProcessID` seam is ready) and
  signed/notarized mac updates (the mac updater is disabled).
- The Windows tray focus after the focus seam split (2026-11-09): the
  flyout shadow and the locale-driven titles were confirmed by a live run;
  the focus call itself was not exercised.
- The Now Playing card's app icon drawing in the live flyout (2026-11-09):
  the shell identity resolved live, but no session was on screen at capture
  time. `TestNowPlayingHeadShowsAppIconWhenAvailable` covers the card.
- Now Playing against live hardware: HomePod audio with the card shown, the
  meter compared against a real receiver, a live transport press, and a
  driven 正在播放 settings page. The DMAP push is unconditional, and the
  Mac receiver displayed the pushed cover on 2026-11-07.
- Audible output and the 50 ms cushion under real WASAPI scheduling jitter;
  a measured left/right offset on a physical pair.
- The ambient light's idle standby's long tail: a two hour idle soak, the
  30 minute default on a live pair, and a live capture of the flyout's 待机
  chip. The 1 minute and 5 minute dark states, the state 2 to state 1 round
  trip, and the stereo pair were verified on 2026-11-10 (see the HomePod
  idle standby entry in Graduated); the phase, command sequence, and
  settings are pinned by `internal/engine/standby_test.go`.
- Device churn while the adaptive-height flyout is open, and its upward growth.
- The tray menu's 设置… entry, and a settings-window redraw driven by an
  engine snapshot change.
- Latency modes: listening at standard and stable; the restart gap when a
  preset restarts a session; the 3 s streaming hold on a slow reconnect.
  The 200 ms preset was listened to with music, and the 160/180 ms custom
  values were heard on a HomePod mini pair (2026-11-10).
- Tray animation: toggling the setting while connecting or streaming, and
  changing Windows animation effects (Reduce Motion) while running.
- The settings 更新 page and notes dialog: the downloading, ready, failed,
  and history states, and install from the page. The available state was
  seen live on 2026-11-08.
- A live update from the previous release, and a clean Windows install of
  the current installer.
- Receiver management live paths: manual add/edit/remove, hide/show filtering,
  the per-receiver delay restart, receiver-password save and clear, and a
  manual receiver with no mDNS advertisement. Pinned by the receivers, engine,
  and app tests and the `settings-receivers-*` / `dialog-receiver-*` previews.
- The directed discovery fallback in the live app: the resolver is proven
  against both HomePods (`TestLiveUnicastQuery`), but the app's first sighting
  of the re-merged group and the stale-group prune are pinned by headless
  engine tests only.
- A live HomePod gated by an AirPlay password: the family split is pinned
  headlessly, but no password-protected HomePod has been configured.
- Ambient light, off behavior: whether audible audio keeps playing while
  state `2` holds the display dark, and the command on the 6,1 and
  first-generation HomePod.
- Ambient light, capability matrix: HomePod 2, HomePod 1, and a receiver
  below software 27.2. The HomePod mini is verified; `TopDisplayCapable`
  holds every unknown until these runs exist.
- Ambient light, receiver-side precedence: the receiver's choice between the
  light and its own track-info cover, and a simultaneous stereo-pair artwork
  repaint.
- Ambient light, reconnect and preset change: the top display survives a
  reconnect, and a preset change replaces the light. `TestLampAppliedWhenStreamingStarts`
  and the sender artwork tests pin the re-push path, not the receiver side.
- Ambient light, takeover: another controller taking over the session, and
  whether the light needs clearing.
- An ambient-light preview with two playing sessions, driven live; the card
  follows the pushed session and `TestLampPreviewFollowsThePushedSessionNotTheCard`
  pins the rule.

## Automated checks

Run `mingw32-make check` from the repository root before declaring a build
good; it chains the gofmt gate, `go vet`, and `go test ./...`. Without
mingw32-make:

```bash
go test ./...
go vet ./...
gofmt -l .
```

`go test ./internal/airplay/sender/` drives the complete AP2 transient and PIN
paths through a fake receiver. The receiver runs its own SRP server, decrypts
the control and event channels, decrypts RTP audio, decodes the uncompressed
ALAC frame, and byte-compares the samples with the ring input. It also checks:

- the first RTP packet's marker is `0xE0` and later markers are `0x60`;
- sequence numbers advance and timestamps start at the fixed latency;
- the event-channel reply is exactly `RTSP/1.0 200 OK`, `Server`, an optional
  `CSeq`, and a blank line;
- AP2 feedback is sent every two seconds;
- an NTP timing request is answered with the reference, receive, and send
  times in the pyatv layout;
- a `0x55` retransmit request is answered from the backlog with the original
  packet;
- stored credentials skip pair-setup on reconnect;
- TEARDOWN is sent on stop, and a silent receiver times out with an error.

The WASAPI test opens the default loopback endpoint for about 300 ms. It proves
the COM binding and format negotiation; it cannot prove audio content on a
silent desktop.

Opt-in live tests, skipped unless their environment variable is set:

```bash
SAUNDO_MDNS_TEST=1 go test -run TestBrowseIntegration -v ./internal/discovery/
SAUNDO_UNICAST_TEST=192.168.31.46,192.168.31.164 go test -run TestLiveUnicastQuery -v ./internal/discovery/
SAUNDO_UNICAST_BROWSE=1 go test -run TestLiveUnicastBrowse -v ./internal/discovery/
SAUNDO_DEVICE_TEST=1 go test -run TestLiveHomePodHandshake -v ./internal/airplay/sender/
SAUNDO_DEVICE_TEST=1 SAUNDO_DEVICE_LONG=1 go test -run TestLiveHomePodHandshake -v ./internal/airplay/sender/
```

The device test mutes the receiver and streams silence, then restores its
initial volume. Adding `SAUNDO_DEVICE_TONE=1` sends a four-second 440 Hz tone
at 50% and restores the volume; run it only when sound in the room is welcome.
The PTP capture test runs inside an active session:

```bash
SAUNDO_PTP_PEER=192.0.2.50 go test -run TestLivePTP -v ./internal/airplay/ptp/
```

## Build the application

```bash
go build -ldflags "-H windowsgui" -o saundo.exe ./cmd/saundo
```

`-H windowsgui` prevents a console window. `mingw32-make build` passes the
same flag, so `bin/saundo.exe` also opens without a terminal. For
development, `go run ./cmd/saundo` keeps the console and its log output. In
the GUI build a fatal startup error goes to `%APPDATA%\saundo\saundo.log`
instead of stderr.

## Real-device checklist

### 1. Discovery

1. Put the PC and the receiver on the same LAN. Disable client isolation.
2. Start saundo. Accept the Windows Firewall prompt for private networks.
3. A HomePod, Apple TV, or Mac with AirPlay Receiver enabled appears within
   about ten seconds. The subtitle shows the model and pairing path.

If nothing appears, check that mDNS UDP 5353 is not blocked, that the network
profile is Private, and that a VPN is not capturing the multicast route.

### 2. HomePod or Mac, transient pairing

1. Select the device and click 连接.
2. The log shows pair-setup M1 through M4, then `control channel now
   encrypted`, `session SETUP`, `event channel open`, `RECORD`, stream SETUP,
   and `streaming`.
3. Play audio on Windows. It should be audible within about one second.
4. Leave it running for at least 30 seconds, then five minutes. The session
   must not drop at the 25 to 30 second mark.

The transient path uses the fixed PIN `3939` and stops at M4. The shared secret
is the 64-byte SRP key; only its first 32 bytes are used for audio encryption.
Event-channel keys are derived from the same secret.

### 2a. Mac with Require Password

1. Select the Mac and connect. On first connect the app asks for the AirPlay
   password in a dialog, saves it for later sessions, and continues the
   connection. The settings page can change or clear it.
2. The log shows pair-setup M1 without Flags, M3, M5/M6, `credentials stored`,
   pair-verify, `control channel now encrypted`, and `GET /info ok`. The AP2
   session SETUP answers `401 Unauthorized`; the sender logs `digest challenge,
   re-sending SETUP with auth` and the replay returns 200.
3. The event channel opens, `RECORD` and `SETPEERS` return 200, the PTP follower
   synchronizes, the stream SETUP returns `dataPort` and `controlPort`, and the
   log shows `streaming`.
4. Reconnect. The second session skips pair-setup and runs pair-verify with the
   stored credentials.
5. If the Mac answers 403 to the AP1 `OPTIONS`, discovery selected the wrong
   path; it must advertise the HAP feature bits and `sf` bit 7 (`sf=0x284` on a
   Mac14,2) and pair as `AuthHAPPassword`.

### 3. Apple TV, on-screen PIN

1. Select the Apple TV and click 连接.
2. The TV displays a four-digit code and the app opens the 配对码 dialog.
3. Enter the code. The log shows M5 and M6, `credentials stored`, then
   pair-verify and streaming.
4. Disconnect and reconnect. The second connection must skip the PIN dialog
   and go straight to pair-verify.

Credentials live in `%APPDATA%\saundo\credentials.dat`, encrypted with
DPAPI. They are keyed by the receiver's `deviceid`.

### 3a. FairPlay TV (Xiaomi/Redmi)

1. The TV advertises `AppleTV3,2` over `_raop` and `_airplay`, `et=0,3,5`,
   and a working `/fp-setup`. Its escaped mDNS name is decoded to UTF-8 (a
   Redmi shows `REDMI 显示器 …`). Discovery marks it `FairPlay · 暂不支持`.
2. An iPhone Music app AirPlay to the TV produces sound, but a third-party
   sender does not: AP1 `SETUP` returns 400, the AP2 session `SETUP` returns
   500, and the stream-only AP2 `SETUP` accepts but no tested audio format
   plays. The TV's audio path requires Apple FairPlay, which this project does
   not implement. The app fails fast with a FairPlay message instead of
   starting a silent session.

### 4. Volume and mute

1. Move the volume slider while streaming. The receiver's own volume changes.
2. Set it to zero. The receiver mutes (`-144` dBFS sentinel).
3. Set it back to 100. The sent value is `0.000000` dBFS.

### 5. Capture endpoints

1. Leave 采集设备 at 跟随系统默认 and switch the Windows output device. The app
   reopens the loopback stream on the new default and the status line updates.
2. Pick a specific endpoint. The app keeps it when the default changes.
3. The drop counter increases only when the network stalls; a healthy session
   keeps it still.

### 6. Disconnect and reconnect

1. Click 断开. The receiver stops and receives TEARDOWN.
2. Reconnect to the same device. It streams again.
3. Click outside the panel or press the tray icon again. The panel hides and
   the app stays in the tray. The tray icon toggles the panel; right-click
   shows the menu for show, connect/disconnect, and quit.

### 7. Settings

- 发送端名称 changes the name the receiver shows on its next connection.
- 严格验证接收器 makes a bad server proof or signature abort the session.
- 启动时自动连接上次设备 reconnects the last device when discovery finds it.
- 更新通道 keeps the chosen 稳定版 or 测试版 across a restart.
- Settings are stored in `%APPDATA%\saundo\settings.json`.

## PTP and Windows Firewall

HomePod software 27 uses PTP over UDP ports 319 (event) and 320 (general).
The app binds both. Windows Firewall must allow inbound UDP to `saundo.exe`
on the private network, as it must for mDNS replies and NTP timing. The app
cannot synchronize the clock and will not start the stream if those packets are
blocked; the UI reports a PTP timeout with this hint.

## Tray flyout (2026-10-31)

- **Form change.** The ordinary 480x720 main window is gone. The full control
  panel is now a `mygo.NewFlyout` anchored to the tray icon: frameless,
  shadowed, 480x720 DIPs, with the full device list, volume, Now Playing,
  capture sources, settings, and logs. Its root paints `Theme.Background`,
  because a flyout is transparent by default. Normal launch opens it; login
  launch and `--tray` stay hidden in the tray.
- **Tray behavior.** A left click toggles the panel, a click outside or Escape
  dismisses it, and a right click opens the tray menu (show panel,
  connect/disconnect, quit). The tray menu is installed only for the duration
  of the right-click popup, so the primary click reaches `OnClick`.
- **Live window.** The running app reported window class `MyGoFlyoutShadow` at
  physical `710,2292..1670,3732`, directly above the taskbar at y=3744. A
  `PrintWindow` capture showed the full panel and its background, including
  the HomePods group row and the Now Playing line.
- **Toggle verified.** Clicking outside set the process's main window handle to
  zero (panel hidden). The app icon was in the notification-area overflow in
  this session; invoking its `saundo` button through UI Automation hid the
  panel, and invoking it again showed the panel with handle `18222904` and the
  same `MyGoFlyoutShadow` class. The right-click menu was not separately
  exercised.
- `gofmt -l .` and `go vet ./...` printed nothing; `go test -count=1 ./...`
  passed every package.

## Receiver migration and launch auto-connect (2026-11-04)

Built `bin/saundo.exe` from this tree and relaunched it twice on the
development PC with a real HomePod stereo pair on the LAN.

- **Migration ran in `Run()`.** The first launch of the new build logged
  `receiver migration: 0 passwords moved, 0 kept (DPAPI unavailable),
  auto-connect seeded=true`. The legacy `settings.json` carried
  `autoConnect=true` and a `lastDeviceId` naming the stereo pair; the
  migration put that flag on the pair's `stereo:<tsid>` registry entry,
  cleared the legacy flag, and wrote `receivers.json`.
- **The registry drove the next launch.** On the second launch
  `settings.json` had `autoConnect=false`, and the engine still
  auto-connected the same pair from its registry entry. No user action
  preceded either connection.
- **Discovered target waited for presence.** The pair appeared in discovery
  about four seconds after launch; the engine then opened both member
  sessions. Both SETPEERS replies were 200, the sender-supplied PTP clock
  synchronized, and both members reached encrypted RTP streaming at
  44100 Hz (`state: streaming`, `stereo group "HomePods": streaming to 2
  members`). The group row and its PTP session behaved as on the last
  verified build.
- **Evidence.** `%APPDATA%\saundo\saundo.log`, the 2026-11-04 01:31 and
  01:36 startup blocks; `receivers.json` after the first launch.
- **Not verified live.** Manual add/edit/remove, hide/show filtering, the
  per-receiver delay restart, receiver-password save and clear, and a manual
  receiver with no mDNS advertisement. Those paths are pinned by the
  `internal/receivers`, `internal/engine`, and `internal/app` tests, and
  their states render in the `settings-receivers-*` and
  `dialog-receiver-*` previews.

## Release 0.4.2 pre-release check (2026-11-04)

- **Maintainer smoke test.** The maintainer tested the build from this tree
  on their own PC before requesting the 0.4.2 release. The release request
  is the confirmation for the release gate, so no second local install and
  no clean-machine run were recorded.
- **Not covered here.** The live update page states, a live update from the
  previous release, and a clean Windows install remain in `Currently
  pending`.
- **Draft check.** The release workflow's check, draft, and build jobs
  passed (run 41758834319). The draft carried the installer, the update
  archive, the 0.4.1 delta, `install.ps1`, `SHA256SUMS`, and the manifest;
  `sha256sum -c SHA256SUMS` passed every listed file. The manifest is
  version 0.4.2 with a signature and the English notes. The installer is
  unsigned, so SmartScreen warns.
- **Public feed.** After publishing, the latest manifest returned 200 with
  version 0.4.2 and a signature, and the installer download returned 200.

## Ambient light, phase 1 (2026-11-05)

- **What landed.** `internal/lamp` renders the six presets (ember, dune, moss,
  tide, orchid, moon) as deterministic 600×600 sRGB JPEGs at quality 0.88 and
  caches each one lazily. `settings.Lamp` stores the global enabled flag and
  preset; an old file loads off + dune and an unknown preset normalizes to
  dune. Discovery parses `osvers`; `device.TopDisplayCapable` requires the
  HomePod family at software 27.2 or later and fails closed on an unknown
  model or version; the 5,1 runs returned 200 OK for both `SET_PARAMETER`
  and `POST /artwork/command` but displayed nothing, so it stays excluded.
  The engine's
  `SetLamp` saves the choice, renders off the sender's tick path, and reapplies
  it when a session starts streaming. The sender's `SetLampArtwork` sends the
  image over the existing `SET_PARAMETER` artwork path without touching DMAP
  text, waits for streaming, and deduplicates identical bytes. A stereo group
  fans the image out per capable member. The 氛围灯 settings page and the
  flyout's always-visible ambient card read one snapshot; neither gates on
  connection, and the tray menu gained no item.
- **Pinned by.** `TestPresetsMatchCatalog`, `TestGeneratedJPEGsAre600Square`,
  `TestGeneratorCachesPerPreset`,
  `TestNormalizeUnknownAndEmpty`, `TestLampDefaultsOnAndCover`,
  `TestLampOldFileLoadsDefaults`, `TestLampUnknownPresetNormalizesToCover`,
  `TestLampOldMediaPresetTurnsCoverOn`, `TestLampOldTemplatePresetTurnsCoverOff`,
  `TestLampRoundTrips`, `TestCoverPresetClustersTheCover`,
  `TestCoverPresetRejectsEmptyImages`, `TestCoverPresetIsDeterministic`,
  `TestTopDisplayCapable`, `TestParseOSVersionTXT`,
  `TestMergeKeepsOSVersion`, `TestSetLampArtworkSendsCover`,
  `TestSetLampArtworkWaitsForStreaming`, `TestSetLampArtworkDuplicateIsNoOp`,
  `TestSetLampArtworkDoesNotTouchDMAPText`, `TestSetLampArtworkClearSendsNothing`,
  `TestLampOffOnAppliesAndPersists`, `TestLampAppliedWhenStreamingStarts`,
  `TestLampUnknownPresetFallsBackToDefault`,
  `TestLampCoverKeepsPlayingOnPause`, `TestLampSkipsIncapableReceivers`,
  `TestLampTogglingTwiceLogsTheSkipOnce`,
  `TestGroupArtworkFansOutToBothCapableMembers`,
  `TestGroupArtworkSkipsIncapableMember`, `TestSettingsLampPrioritySegmentTurnsCoverOn`,
  `TestSettingsLampPrioritySegmentTurnsCoverOff`, `TestFlyoutLampCoverHidesTheGrid`,
  `TestFlyoutCloseCollapsesLampList`, `TestFlyoutLampOffRequiresTurningOn`,
  `TestLampOffIgnoresPriorityAndPreset`,
  `TestSettingsLampPageOffRequiresTurningOn`,
  `TestSettingsLampPickingPresetWhileCoverOnTurnsCoverOff`, and the app lamp
  tests named in ui.md.
- **Live on AudioAccessory5,1.** A temporary gate allowed the user's 5,1
  stereo pair. After Sound Glow was enabled in the HomePod settings, the same
  pair displayed the lagoon and dawn lights. The working request is the HTTP
  `/artwork/command` binary plist with a data URL over one keep-alive
  connection and the session headers; `SET_PARAMETER` and the RTSP artwork
  POST rendered nothing. 5,1 joined `TopDisplayCapable`.
- **Not verified live.** Still unverified: off/return-to-native, the 6,1
  artwork channel, the stereo pair's simultaneous rendering, and
  reconnect/takeover behavior.

## Ambient light preview and session following (2026-11-08)

- **The card follows the system's current session.** Reported live: a new
  media started playing, the top display's cover-following light moved to it,
  but the flyout card stayed on the old session. `setNowSnapshot` adopted a
  session only when the old one disappeared. It now follows the SMTC current
  session on every read; the switch arrow sets a pin that survives polls
  until a different session becomes current. `TestNowPlayingFollowsTheCurrentSession`
  pins the follow, the pin, and the takeover by a newly current session.
- **The light preview follows the push, not the card.** Reported live:
  switching the card to a paused session with the arrow changed the ambient
  preview while the HomePod kept the playing session's cover. The preview
  was derived from the card's selected session. `refreshNowLamp` now reads
  the session the engine pushes (`snapIndex`), and the card's own cover no
  longer feeds it. `TestLampPreviewFollowsThePushedSessionNotTheCard` pins a
  manual switch that leaves the preview on the pushed cover. The live app
  was not driven with two playing sessions in this unit; the HomePod
  observation above was user-reported.

## Release 0.5.0 pre-release check (2026-11-07)

- **Draft for real-device testing.** The maintainer requested a 0.5.0 draft
  and tests it on real devices before publishing. The release request is the
  local smoke-test confirmation; the draft's real-device test is the publish
  gate.
- **Draft assets (2026-11-07).** The release workflow's check, draft, and
  build jobs passed (run 42264388577). The draft carried the installer, the
  0.5.0 archive, the 0.4.2 and 0.4.1 deltas, `install.ps1`, `SHA256SUMS`, and
  the manifest; `sha256sum -c SHA256SUMS` passed every listed file. The
  manifest is version 0.5.0 with a signature and the English notes. The
  installer is unsigned, so SmartScreen warns.
- **Rebuilt draft assets (2026-11-07).** The mygo 0.8.0 rebuild's check,
  draft, and build jobs passed (run 42314253794). The draft carried the
  installer, the 0.5.0 archive, the 0.4.2 and 0.4.1 deltas, `install.ps1`,
  `SHA256SUMS`, and the manifest; `sha256sum -c SHA256SUMS` passed every
  listed file. The manifest is version 0.5.0 with a signature, the English
  notes, and the mygo 0.8.0 line. The installer is unsigned, so SmartScreen
  warns. This supersedes run 42264388577 and its assets.
- **Defaults (2026-11-07).** The local-output mute and ambient light default
  on; the receiver track info is always pushed and has no switch.
  `TestDefaultsEnableLocalMute`, `TestLampDefaultsOnAndCover`, and
  `TestLampOldFileLoadsDefaults` pin this.
- **CI gate and final draft (2026-11-07).** The defaults release commit
  `d1f6e76`'s workflow passed check, draft, and build (run 42339224999);
  the CI check job is the release gate because this machine's engine test
  binary fails its settings save with an impossible `json.SyntaxError`.
  The draft carried the installer, the 0.5.0 archive, the 0.4.2 and 0.4.1
  deltas, `install.ps1`, `SHA256SUMS`, and the manifest; `sha256sum -c
  SHA256SUMS` passed every listed file. The manifest is version 0.5.0 with
  a signature, the English notes, and the defaults and mygo 0.8.0 lines.
  The installer is unsigned, so SmartScreen warns.
- **Publish gate.** The maintainer's real-device test of the draft is still
  pending; publishing is not authorized yet.

## Failure modes and meanings

| Symptom | Likely cause |
|---|---|
| Connected, but silence | ALAC frame layout, audio key, or event-channel reply headers are wrong. Confirm the exact minimal 200 OK and the ALAC byte order. |
| Redmi/Xiaomi AppleTV3,2 cannot play audio | It requires Apple FairPlay audio, which this sender does not implement. The app fails fast with a FairPlay message; use an AirPlay 2 audio receiver. |
| Drops after 25 to 30 seconds | Event channel is not opened, not decrypted, not answered, or `/feedback` is not `RTSP/1.0`. |
| Pairing 403 | Receiver access control. A password-protected Mac must pair as `AuthHAPPassword`; a 401 on the AP2 SETUP is its Digest challenge. On an Apple TV set Allow Access to Anyone on the Same Network; set up a HomePod in the Home app. |
| Mac session SETUP OK, stream SETUP never answers | The Mac advertises PTP bit 41 and ignores an NTP-timed stream. Run the PTP follower. |
| Pairing 470 | The receiver wants a PIN but did not show one. The app falls back from transient to on-screen PIN once. |
| No audio after reconnect | Stored credentials are missing or corrupt; the app pairs afresh. Check the DPAPI file is readable by the same Windows user. |
| Timing requests unanswered | Windows Firewall blocks inbound UDP to the app. Allow the app on private networks. |
| Discovery empty | mDNS blocked, wrong network profile, VPN, or AP/client isolation. A receiver that answers directed queries but not multicast is found from the ARP neighbor table; one absent from both stays undiscovered. |

## Evidence to capture before changing protocol code

When behavior differs from this document, capture:

1. The diagnostics log from the app (诊断日志).
2. A packet capture filtered to the receiver's address.
3. Whether the receiver answered each RTSP method, and with which status.
4. Whether the event channel received and answered requests.

Then update [devices.md](devices.md) or [protocol.md](protocol.md) with the
finding — whichever describes the behavior — before changing the
protocol order or keys.

## Connection recovery and discovery names (2026-11-08)

- **Sticky failure after a dead member.** A stereo-group member closed the
  control channel during setup (`session failed: HomePod L：Connection closed
  by the device`, `saundo.log` 21:19:58 and 21:44:04). The engine kept
  `lastError` and `selectedID` after the attempt, so the flyout stayed in
  `phaseError`: the failed row offered 重试 and every other online row
  rendered a faint 连接 with no handler. `cmdDisconnect` and `connect` now
  clear the error when an attempt ends, and `actionFor` keeps other online
  receivers connectable in the error phase. Pinned by
  `TestUserDisconnectClearsConnectionError`,
  `TestConnectClearsPreviousError`, and
  `TestRowActionKeepsOtherReceiversConnectable`.
- **Cancel showed the previous failure.** `phaseOf` reports the reconnect
  wait before the error, so the stale banner was hidden until a cancel
  returned the state to idle. The cancel now clears the error before
  publishing the idle snapshot; the UI returns to 未连接. The headless error
  frame is `flyout-error-zh.png` in the session evidence.
- **Service name leaked into the display name.** hashicorp/mdns reports
  SRV/TXT/A records from both AirPlay services to either browse. Parsing an
  `_airplay` entry under the `_raop` label kept `._airplay._tcp` in the name,
  seen live as `"HomePod R._airplay._tcp"` at `saundo.log` 21:01:05 and
  21:01:07. `parseEntry` now derives the service from the instance suffix
  before trimming, so both directions resolve to the friendly name. Pinned
  by `TestParseEntryUsesTheAdvertisedService`.

## 2026-11-08 — the macOS port

darwin/arm64 ships from the same tree, `CGO_ENABLED=0`. Machine: a MacBook
Air on macOS 27.3 (build 26C5035r); the operator streamed to a HomePod mini.
The working notes live in the git-ignored `.audit/mac-port-feasibility.md`
and `.audit/mac-experiments.md`; what survives here is the durable state.

- **Build and gate.** `bash scripts/build.sh` produces a dev-signed
  `bin/saundo.app` (14.4 MB) plus a dmg; the full gate (gofmt, catalog
  check, `go vet`, `go tool mygo vet .`, `go test ./...`) is green — 21
  packages, `internal/app` headless on darwin included.
- **Live streaming.** Discovery, connect, and audio to the HomePod mini
  verified by the operator with the darwin capture path (2026-11-07).
- **Capture.** The Core Audio process tap aggregated with the default
  output device feeds the ring: a probe read 337,408 frames of speech
  (48 kHz stereo) over 8 s with zero errors, following the format the tap
  reports. A tap-only aggregate refuses to start
  (`kAudioHardwareIllegalOperationError`); the output device must be the
  sub-device and clock master, with the tap in the aggregate's tap list.
- **Now Playing.** MediaRemote returns empty data to third-party processes
  on 27.3 — callbacks fire with media playing, payload empty (measured) —
  so the session reads through the vendored adapter loaded inside
  `/usr/bin/perl`, a platform binary MediaRemote still accepts. Title,
  artist, album, position, duration, playing state, and artwork (110–196 KB
  JPEG, base64 in the JSONL stream) all arrive; send 0/1/4/5 and seek
  control the source app. One session only: no enumeration symbol exists
  (13 candidates probed at runtime) — the system widget's multi-source list
  rides mediaserverd's private XPC, so the card follows the current source
  and the switcher hides. The perl loader rejects relative framework paths,
  and takes the function and its parameter as separate argv entries.
- **Session identity.** LaunchServices resolves the localized app name
  ("Music"); `NSBundle localizedName` does not exist on this OS —
  `NSFileManager displayNameAtPath:` is the working call. Icons render via
  `NSWorkspace iconForFile:` and an ImageIO 128 px thumbnail to PNG.
- **Credentials.** The Keychain master key + AES-256-GCM round-trips. A
  rebuilt ad-hoc binary triggers one keychain ACL prompt per item; the
  dev-signed build (a stable identity from `SAUNDO_MAC_SIGN_IDENTITY`)
  ends them. Parallel test binaries hit the same
  ACL; headless runs set `SAUNDO_CREDS_MASTER_KEY` and the creds package
  decides keys from the environment instead.
- **Signing and distribution.** Ad-hoc builds are Gatekeeper-rejected when
  quarantined; recovery is System Settings → Privacy & Security → Open
  Anyway, once per build cdhash (measured; the right-click Open bypass is
  gone since Sequoia). `spctl -a` keeps printing `rejected` after the user
  allows it. Locally built copies carry build provenance and never see the
  flow. Signing the nested adapter framework prompts for the certificate
  key once, separately from the credentials key.

## 2026-11-09 — Windows side of the darwin port

Live run on Windows 11 Pro for Workstations (build 26300),
`bin/saundo.exe` 0.5.0 built from the merged tree, streaming to a HomePods
stereo pair. This covers the Windows half of the multi-platform review
above.

- **Gate and build.** `mingw32-make check` passes end to end: gofmt clean,
  315 catalog messages, `go vet`, `go tool mygo vet .`, `go test ./...`
  (all packages ok), the verify-map scan, and `check-platforms`
  (windows/amd64 and darwin/arm64 build and vet, and platform imports stay
  behind their tags). A second `go test -count=1` over `nowplaying`,
  `capture`, `app`, `engine`, and `discovery` passed without cache.
  `mingw32-make build` produced `saundo.exe` and the 0.5.0 installer; the
  app relaunched and rejoined its session.
- **Shell app identity.** `TestSMTCLiveSessions` read `MyGo Music.exe` as
  MyGo Music with a 4,614-byte 64×64 PNG icon in 0.28 s, the Start-menu
  enumeration included; `TestSMTCInfoLive` read "Snow" by 露崎春女. The
  headless `TestNowPlayingHeadShowsAppIconWhenAvailable` pins the card's
  icon slot.
- **Flyout.** A DPI-aware PrintWindow capture shows the Windows 11 flyout
  with its system shadow and rounded corners, no macOS popover. The window
  was `MyGoFlyoutShadow|Saundo` and the settings window
  `MyGoWindow|Settings · Saundo`; after the settings language selector
  stored follow-system, the flyout title changed to 声渡 at once, and the
  same `onLocaleChanged` sets the settings title. `capture-window.ps1` is
  DPI-unaware on this 1.75× display: it returns the top-left 420×625 DIP
  region of the 735×1094 physical window, which reads as a clipped
  screenshot, not a layout defect. Use a DPI-aware capture for evidence.
- **Not driven live.** The Now Playing card's icon drawing (the media
  session ended before a clean capture of the card), the settings page's
  new layout, per-app capture, the tray animation, and tray focus. Audible
  output was not confirmed; the log shows an active RTSP session with
  `audio resumed`/`audio idle` across the run.
- **Maintainer pre-release test (2026-11-09).** The v0.6.0-beta.1 release
  request is the maintainer's confirmation that the build was tested on
  their own PC.
- **UI review (2026-11-09).** The release gate rendered the 196-frame matrix
  (zh-CN and en, light and dark) at
  `.audit/ui-review-0.6.0-beta.1/frames/` and fixed 15 findings: three
  English subtitles that ellipsized at the 720 default window, the compact
  lamp segment's second English name for Template, bare panel in the tray
  menu and the remove dialog, Flyout/浮窗 in the macOS-only style setting,
  the Chinese audio-level term, two Chinese descriptions that kept clauses
  the English dropped, the signature and backoff wording, the About license
  label, a stale catalog allowlist entry, and the macOS popover subtitle's
  length. The catalog gates now ban the old strings. Two independent
  reviewers on different model families checked the fixes; the second
  verdict is clean. The macOS-only General row is not renderable on
  Windows; the reviewer measured the shortened subtitle at 354.5 DIP
  against the 415-DIP column at the 720 default. It still ellipsizes at the
  640 minimum, like other single-line settings subtitles.
- **Draft verification (2026-11-09).** The release workflow (run
  42506672553) passed `check`, `check-macos`, `draft`, `build`, and
  `build-macos`. The draft carried `install.ps1`, the Windows installer,
  the update archive, the three deltas, `update-windows-amd64.json`, the
  arm64 dmg, and `SHA256SUMS`; `sha256sum -c` passed for all six
  checksummed files. The manifest names 0.6.0-beta.1, is signed, and
  carries the ASCII English notes. The release is published as a
  prerelease: `releases/latest` still serves the signed 0.5.0 stable feed,
  `releases/download/v0.6.0-beta.1/update-windows-amd64.json` serves
  0.6.0-beta.1, and the beta installer URL returns 200. The beta channel
  resolves the prerelease through `ResolvePrereleaseFeed`; stable builds
  skip it.
- **Verification map (2026-11-09).** All six feature pages were audited
  from source after the port and corrected in their own commit: the update
  page is Windows-only, offline receivers are dropped rather than shown,
  the flyout ALAC line carries latency rather than the stereo tag, the
  media switch only hides the flyout card, the ambient off state ignores
  the segment and templates, the tray menu and window titles use the new
  copy, and the settings capture path is class plus pid. The live pass
  covered the doctor, the mDNS browse (HomePods and MacBook Air found with
  clean names), the fake-receiver protocol suite, and a live flyout
  capture. Connect/disconnect, settings switches, ambient-light actions,
  and update install were not driven because the operator's session was
  streaming; the headless matrix and protocol tests cover those paths.

## 2026-11-09 — 0.6.0 stable release

The stable release is the 0.6.0-beta.1 UI plus four changes: the engine
snapshot carries the flyout style, the macOS aggregate liveness false
positives are gone, the release copy review's blocking findings are fixed,
and the verification map is corrected.

- **Maintainer pre-release test (2026-11-09).** The 0.6.0 release request is
  the maintainer's confirmation. The running app streamed to the HomePods
  stereo pair through the audit; the rebuilt app (19:30) logged no further
  "aggregate device stopped" lines through 19:49.
- **UI review (2026-11-09).** The 196-frame matrix rendered at
  `/tmp/saundo-ui-review` (zh-CN/en, light/dark). An independent reviewer on
  a different model found two P0 truncations (the capture description and
  the receiver port hint), four P1 terminology issues (the Chinese receiver
  term, PC in the macOS build, the raw update error, the launch chip), and
  P2 polish. The P0/P1 and the terminology P2s are fixed in `0789520` and
  pinned; the flagged frames were re-rendered and read back. The pure-polish
  P2s (latency unit copy, update date label, mute "or quit", quotes, one
  Chinese full stop, What's new case, the plural hidden hint, redundant
  subtitles) are a follow-up.
- **Verification map (2026-11-09).** All six feature pages were audited from
  source and corrected in `b3e4eaa`; `go run ./scripts/verify-map` passes.
  The live pass was not re-driven because the app was streaming; the beta's
  live audit and the maintainer's test cover the same surfaces.
- **Candidate builds (2026-11-09).** `mygo build -platform windows/amd64
  -skip-installer` produced a 0.6.0 exe; `mygo build -platform darwin/arm64
  -skip-build-command -sign -` produced the app and dmg with the adapter at
  `Contents/Resources/adapter` and Info.plist 0.6.0. The local update
  manifests are unsigned because the update key is on the Windows PC; the
  CI draft signs with the repository secret and is verified in the draft
  verification commit.

- **Draft verification (2026-11-09).** The release workflow (run
  42615271600) passed `check`, `check-macos`, `draft`, `build`, and
  `build-macos`. The draft carried `install.ps1`, the 0.6.0 installer, the
  update archive, the three deltas, `update-windows-amd64.json`, the arm64
  dmg, and `SHA256SUMS`; `sha256sum -c` passed for all six checksummed
  files. The manifest names 0.6.0, is signed, and carries the ASCII English
  notes. The tag stays on `14f27cc`; publishing is the remaining gate.

- **Publish (2026-11-09).** `gh release edit v0.6.0 --draft=false`
  published the stable release. The public feed serves the signed 0.6.0
  manifest; the installer and dmg URLs follow to 200. A live update from an
  installed 0.5.0 build is still owed.

## 2026-11-11 — 0.6.1 UI mockup templates

- **Templates.** Both assets under `.claude/skills/ui-mockup/assets/` now
  describe the shipped interface. The settings template carries the current
  language and delay subtitles, no terminal periods on row hints, the
  shipped mute caveat, and the 150x28 select as its sample language row. The
  flyout template drops the footer's diagnostic-log button (the log is on the
  settings About page), adds the idle-standby state (待机 chip, Standby
  footer), and matches the catalog copy. The height rule is unchanged: 459
  base, 150 media card, 44 latency row, 90 ambient section, 48 error bar, 53
  for the expanded grid.
- **Renders.** 44 frames at `.audit/release-0.6.1/mockups/`: the settings
  target, normal, and edge states and the flyout idle, streaming, standby,
  empty, and error states, in zh-CN and English, light and dark, plus the
  media card and the ambient cover, template, and expanded states. Every
  frame was inspected for clipping, overflow, and wrapped copy; none was
  found.

## 2026-11-11 — 0.6.1 release audit

- **Docs audit.** The five live docs were checked against the tree. Corrected:
  the HomePod latency floor and the Mac model-identifier table (devices.md);
  the SETPEERS skip on PTP, the prefill and duplicate policy, the
  idle-standby wire behavior, and the OS secret-store wording (protocol.md);
  PTP for macOS, the directed discovery fallback, and the fourth concurrency
  role (architecture.md); the settings write ownership, cached auto-connect,
  the quiet mDNS logger, the Mac model table, the diagnostics sync fields,
  and `SetTrayAnimation` (implementation.md); the password row rule, the
  log-dialog motion, the static connect icon, and three stale test names
  (ui.md). Links resolve and every cited test name exists. The verification
  log compressed from 2050 to 937 lines: 31 dated batches graduated with
  their pins, the pending list corrected, and `go run ./scripts/verify-map`
  prints "map ok".
- **UI review (2026-11-11).** The 242-image matrix rendered at
  `.audit/release-0.6.1/previews` in zh-CN and English, light and dark:
  every settings page and flyout state, the local receiver row, the takeover
  state, and the 640 DIP lamp page. One P0: the English standby subtitle
  truncated at 640 DIP; it is shortened and the scrolled 640 previews pin
  it. One P2: the standby subtitles carried terminal periods; the
  punctuation gate lists both keys. An independent reviewer on a different
  model family confirmed both; it found the 许可/许可证 split (fixed) and
  the missing takeover render (added). No P0 or terminology finding remains.
- **Verification skill audit (2026-11-11).** Six feature pages were read
  from source and corrected in `51312ab`; `go run ./scripts/verify-map`
  prints "map ok".
- **Live pass (2026-11-11).** The 0.6.1 build (pid 62152, version 0.6.1)
  was launched in the tray and driven through UI Automation. Observed: the
  tray button's live accessible name, the flyout opening and Escape
  dismissal, the streaming flyout with the HomePods stereo pair, the filled
  glyph, volume, the 200 ms latency row, the 无媒体 chip, and the footer;
  the settings window at its 640 DIP minimum on 通用, 连接, 接收器, 音频,
  氛围灯, 关于, and 更新; the connection health card live (接收器处理
  最高 69 ms, 稳定性 良好, 本次会话); the diagnostic log dialog with its
  200 lines and 复制/关闭; the ambient standby row at 640 (10 分钟, the
  copy fits); and 更新 reading 0.6.1 · 稳定版 ✓ 已是最新. A 断开连接 then
  连接 cycle stopped both members and restored both streams. Not driven:
  the pairing dialogs, volume and latency changes, the update install, the
  standby dark state, takeover, the tray menu, and language switching.
  Evidence: `.audit/release-0.6.1/live/` (`doctor.txt`, the flyout and page
  PNGs, `log.txt`).
- **Harness fixes (2026-11-11).** The live pass exposed three verification
  harness defects, fixed and re-run here: the doctor matched the settings
  window by an English title and missed `设置 · 声渡` (now by class); the
  capture script was DPI-virtualized and cropped a flyout at monitor scale
  (now DPI-aware, 1050x1048 px from a 420 DIP panel); and the tray page
  described an exact brand accessible name, while the button's name is the
  brand plus the live tooltip (now matched by class and prefix).
- **Maintainer pre-release test (2026-11-11).** The 0.6.1 release request is
  the maintainer's confirmation that the build was tested on their own PC.
- **Draft verification (2026-11-11).** The release workflow (run
  42854757255) passed `check`, `check-macos`, `draft`, `build`, and
  `build-macos`. The draft carries `install.ps1`, the installer, the update
  archive, three deltas, `update-windows-amd64.json`, the arm64 dmg, and
  `SHA256SUMS`; `sha256sum -c` passed for all six checksummed files. The
  manifest names 0.6.1, is signed, and carries the ASCII English notes. The
  tag stays on `abe977b`; publishing is the remaining gate.

## Graduated

Dated batches compressed under the rule at the top of this file: every claim
has a durable home — a named test, devices.md or protocol.md, the code that
owns the behavior, or a later dated entry. Each entry keeps the date, the
surviving conclusion, and the pin.

- 2026-10-31, first device run: HomePod 27.2 (`AudioAccessory5,1`) accepts
  the stream only with `timingProtocol: PTP` and
  `timingPeerInfo`/`timingPeerList` — an NTP-timed stream is silently
  ignored. SETPEERS was part of that first acceptance; since 0.6.1 it is
  skipped on every PTP session (protocol.md §8). The rule lives in
  protocol.md and devices.md. The PTP follower held roughly 1 ms offset
  (boot-relative clock; only the offset matters). A muted 35-second session
  and a 440 Hz tone streamed clean, and the user confirmed hearing the tone.
  The real-window run (autoConnect to streaming, 断开/连接, volume 200s, tray
  restore from the hidden-icons overflow) was re-proven in the tray flyout
  section above.
- 2026-10-31, second batch — each claim pinned by its test: auto-reconnect
  backoff 1s/2s/5s/10s/15s, five attempts (engine tests); remembered volume
  and `initialVolume` adoption (`TestInitialVolumeAdoptedWhenUserDidNotSetOne`);
  async capture restart (`TestAsyncCaptureStartDoesNotBlock`,
  `TestCaptureRateChangeMidSession`); offline marking and `gpn`/`gcgl`/`igl`
  group fields (three discovery tests); log rotation (`TestRotation`);
  single-instance handover and the autostart registry toggle (live, both
  directions); Now Playing as DMAP (`TestNowPlayingRidesAsDMAP`); FairPlay
  fail-fast for `AppleTV3,x` (`TestLegacyAppleTVFailsFastForFairPlay`,
  `TestRedmiFairPlayReceiverAndNameDecoding`). The 30-minute soak
  (`TestLiveSoak`) sent exactly 79,380,224 frames — 44100 × 1800 — with zero
  ring drops; the handshake deadline extended to 30 s for a slow PTP clock
  (`TestPTPWaitToleratesSlowClock`), cold start 10.81 s
  (`TestLiveColdStartTiming`, acceptance < 15 s).
- 2026-10-31, build fixes: `-H windowsgui` links subsystem 2
  (`TestBuildTargetProducesGUIBinary`); the icon lives at
  `../../resources/icon.png` (`TestMyGoIconPathExists`); `bin/saundo.exe`
  comes from `mygo build` (same pin).
- 2026-10-31, fourth pass: connect selects the device being connected, and
  `lastDeviceId` persists only after streaming
  (`TestConnectSelectsTheDeviceBeingConnected`,
  `TestFailedLaunchDoesNotReplaceLastDevice`); capture ignores
  `DATA_DISCONTINUITY` and a successful capture clears a stale error
  (zero-error assertion in `TestLoopbackReceivesSamples`,
  `TestCaptureErrorClearsWhenCaptureRecovers`).
- 2026-10-31, live-edge start and pair lockstep: A session starts at the live
  edge; one `StartGate` trims every ring to the newest 50 ms cushion, seals
  the shared epoch on the last arrival, and both pair members begin in the
  same Tick. Pins: protocol.md §9, implementation.md,
  `TestStreamingStartsFromTheNewestCapture`, `TestPairStartsInLockstep`, the
  `TestStartGate*` tests, and the `TestKeepNewest*` /
  `TestFanoutKeepNewest*` tests.
- 2026-10-31, stereo groups: Discovery collapses the pair by `tsid`; each
  member gets its own AP2 session carrying `groupUUID`, and the sender
  supplies the PTP grandmaster timeline for the pair. Pins: devices.md
  §HomePod stereo pair, `TestStereoGroupSessionCarriesGroupUUIDAndPTPTimeline`,
  `TestSnapshotCollapsesStereoPair`, `TestIncompleteStereoGroupIsOffline`,
  and the ring/fanout tests.
- 2026-10-31, settings window split: Playback and session controls stay in
  the flyout; configuration moves to the settings window at 720×500 with a
  640×420 minimum. Pins: ui.md, `TestSettingsPagesRender`,
  `TestFlyoutKeepsPlaybackControlsOnly`.
- 2026-10-31, silence suspend: AP2 suspends RTP after 5 s of digital silence
  (the entry's original 2 s is superseded) and resumes on the first audible
  block, re-anchoring the timeline and trimming the 50 ms cushion;
  `SETRATEANCHORTIME` is unused. Pins: protocol.md §6, devices.md, and the
  engine/sender suspend tests.
- 2026-10-31 to 2026-11-06, Now Playing: The card reads SMTC or MediaRemote,
  follows the current session, draws the poster, meter, progress, and
  transport, and its settings switches only hide card parts; the receiver
  push is unconditional. Pins: ui.md, implementation.md,
  `TestNowPlayingCardShowsTrack`, `TestRefreshNowPlayingPushesWhileStreaming`,
  `TestNowPlayingCardControlsSendActions`, `TestNowPlayingCardHidesWithoutMedia`,
  `TestNowPlayingPlaceholderStandsOutFromTheCard`,
  `TestCoverFieldCompressesLightness`, `TestNowPlayingCoverModes`,
  `TestNowPlayingSettingsPersist`, `TestNowPlayingCoverRidesAsImage`.
- 2026-10-31, batch results (third pass): Per-app capture selects the
  process-loopback device through `Config.ProcessID` (the VAD wants float32
  stereo, zero buffer duration, and the LOOPBACK flag), receiver volume
  changes arrive through the one-second `GET /info` poll, per-device
  passwords reach the digest path, shell-resolved names and icons feed the
  Now Playing card, and a login or `--tray` start stays in the tray. Pins:
  platforms.md, implementation.md, `TestAudioApplicationsLive`,
  `TestReceiverVolumePollAdoptsHardwareChanges`,
  `TestLatencyAndPasswordReachSession`, `TestNowPlayingCommandForwards`,
  `TestNowPlayingRidesAsDMAP`, `TestMatchInstalledApp`,
  `TestAppNameResolvesUnpackagedExecutable`,
  `TestNowPlayingHeadShowsAppIconWhenAvailable`; the tray icon rule is in
  ui.md, and the 推送正在播放 switch is gone because the push became
  unconditional on 2026-11-07.
- 2026-11-01, Mac password and FairPlay: Mac HAP password pairing and
  pair-verify, and the FairPlay fail-fast, are verified. Pins: devices.md,
  `TestHAPPasswordPairsAndStreams`, `TestAP2DigestChallengesSessionSetup`,
  `TestUsesPTPSelectsHomePodAndMac`,
  `TestLegacyAppleTVFailsFastForFairPlay`,
  `TestRedmiFairPlayReceiverAndNameDecoding`,
  `TestMissingPasswordPromptsAndUsesIt`.
- 2026-11-01, adaptive flyout height: The flyout resizes to the device count
  and media state from the 459 base with no flexible spacer; the old
  500/444/388/470 heights and the 23-DIP spacer are superseded. Pins: ui.md,
  `TestFlyoutHeightForDeviceRows`, `TestFlyoutTargetHeightFitsPlayback`.
- 2026-11-02, Tray animation: The tray animates 12 frames at 100 ms; the
  static icon is frame 0, and Reduce Motion holds the face. Pins: ui.md,
  `TestTrayAnimationDefaultsOnAndRoundTrips`,
  `TestTrayAnimationSettingPersists`,
  `TestTrayAnimationCommandUpdatesSnapshotOnly`, `TestTrayAnimationFaces`,
  `TestSettingsPagesRender`.
- 2026-11-02, new network results: The AP2 bplist now encodes non-ASCII
  strings as UTF-16BE (`TestBPListNonASCIIString`), the volume readout no
  longer multiplies by 100 (`TestViewRendersDevicesAndStatus`), the update
  feed needs a public repository or host (releasing.md), and idle ring
  overflow no longer counts as a session drop
  (`TestSessionDroppedSamplesExcludeIdleOverflow` supersedes the removed
  `TestCaptureMetaHidesDropsWhenIdle`).
- 2026-11-03, Latency modes: Low is 200 ms, standard 1.5 s, stable 2 s, and
  custom 20–2000 ms; a live change restarts the session and the flyout holds
  streaming for up to 3 s. Pins: protocol.md, ui.md,
  `TestApplyLatencyRestartsActiveSession`,
  `TestLatencyApplyHoldsStreamingSnapshot`, `TestAudioModeTileSelects`.
- 2026-11-06 and 2026-11-07, ambient light cover: A raw-JPEG
  `/artwork/command` POST returns 400 and does not replace the cover; one
  `SET_PARAMETER image/jpeg` repaints the top display, and the POST trigger
  and `TCPArtwork` are gone. Pins: devices.md, protocol.md §10,
  `TestSetLampArtworkSendsCover`, `TestSetLampArtworkWaitsForStreaming`,
  `TestSetLampArtworkDuplicateIsNoOp`,
  `TestSetLampArtworkDoesNotTouchDMAPText`,
  `TestSetLampArtworkClearSendsNothing`.
- 2026-11-06, local output mute: The connection mute always unmutes the
  endpoint it owns; the loopback tap survived endpoint mute on Realtek
  48 kHz, but the tap point is driver-dependent; `mute-guard.json` recovers
  a crash. Pins: ui.md, platforms.md, `TestMuteLocal*`, `TestMuteGuard*`,
  `TestLiveMuteLocal`.
- 2026-11-06, stereo group name decoding: `gpn` arrives in RFC 1035 \DDD
  escapes, and `parseEntry` unescapes it. Pins: devices.md,
  `TestStereoGroupNameDecodesDNSEscapes`.
- 2026-11-06, directed discovery fallback: Discovery keeps multicast and
  adds a directed PTR pass every fifth cycle over the ARP table plus
  remembered seeds; a group unseen for 24 h is pruned. Pins: devices.md,
  implementation.md, `TestUnicastFallbackDiscoversStereoPair`,
  `TestUnicastSkipsInstanceWithoutAddress`, `TestUnicastRunsEveryFifthCycle`,
  the candidate/prune tests, and the live `TestLiveUnicastQuery`.
- 2026-11-07, ambient light playback-state off: Manual 关灯 is
  `updateMRPlaybackState: 2`, 开灯 is `1`, RTP stays up, and a media pause
  keeps the light. Pins: devices.md, protocol.md §10, docs/standby.md,
  `TestLivePlaybackState`, `TestSetPlaybackStateSendsCommand`.
- 2026-11-08, ambient template carriers: The six templates are three
  Lab-separated colors at 45/30/25 in flat carriers; dune is
  `#C08A00`/`#C47A1E`/`#7A3F14`, and the receiver's exact cluster count is
  not reversed. Pins: devices.md, `TestPresetCarriersRecoverTheirPalette`,
  `TestPresetColorsAreDistinct`, `TestPresetWeightsSumToOne`.
- 2026-11-08, flyout capture line removed: The playback card no longer draws
  the capture line; base 459, error 48, media/latency/ambient 150/44/90.
  Pins: ui.md, `TestFlyoutTargetHeightFitsPlayback`,
  `TestErrorStateShowsRetry`, `TestFlyoutHeightForDeviceRows`.
- 2026-11-08, UI copy and layout sweep: The catalog and preview gates hold;
  the compact lamp priority segment reads Cover / Template in English, and
  the card padding moved the flyout base to 459. Pins: ui-copy.md, ui.md,
  `TestCatalogHasNoDeadKeys`, `TestCatalogBannedTerms`,
  `TestCatalogUniqueValues`, `TestLatencyText`, `TestRenderPreviews`,
  `TestFlyoutHeightForDeviceRows`, `TestErrorStateShowsRetry`.
- 2026-11-09, receiver-reported latency: The stream SETUP response carries
  the receiver's own `arrivalToRenderLatencyMs`; saundo parses and reports it
  and never adds it to the configured latency. Pins: devices.md,
  protocol.md §4, `TestStreamSetupReportsReceiverLatency`,
  `TestStreamSetupWithoutReceiverLatency`,
  `TestReceiverLatencyUpdatesSnapshot`.
- 2026-11-09, connection health card: The 连接 page leads with the current
  receiver's glyph, name, model, and state chip, adds three read-only stats
  while streaming, and shows only identity and state when disconnected; an
  absent receiver value reads `—` / 接收器未上报. Pins: ui.md,
  `TestSettingsPagesRender`,
  `TestConnectionHealthCardUnknownAndDisconnected`.
- 2026-11-09, connection card copy review (three passes): The card names the
  two latency stages separately (延迟模式 / 接收器处理), uses 稳定性 and
  本次会话, never sums them, and keeps session drops across capture resets;
  labels are exact and ellipsized. Pins: ui.md, ui-copy.md,
  `TestConnectionHealthCardLabelsAreExact`,
  `TestConnectionHealthCardStability`,
  `TestConnectionHealthCardUnknownAndDisconnected`,
  `TestConnectionHealthCardShowsBaseLatency`,
  `TestConnectionHealthCardStereoGroup`,
  `TestSessionDroppedSamplesExcludeIdleOverflow`,
  `TestSessionDroppedSamplesSurviveCaptureReset`,
  `TestSessionDropsBeginAtStreamingState`, `TestSettingsPagesRender`,
  `TestCatalogBannedTerms`.
- 2026-11-09, receiver latency live on the stereo pair: Both members' SETUP
  reported `arrivalToRenderLatencyMs` (69 ms live), and the group forwards
  it. Pins: devices.md, `TestGroupForwardsReceiverLatency`.
- 2026-11-09, receiver family split: `device.Family` selects the HomePod
  `shk` and latency profile; `Auth` only selects pairing; a PIN-paired
  HomePod or Apple TV hides the password row unless one is saved. Pins:
  devices.md, ui.md, `TestHomePodPasswordKeepsHomePodProfile`,
  `TestFamilyFor`, `TestParseEntrySetsFamily`,
  `TestHomePodRowHidesPasswordOption`,
  `TestManualAndMacRowsKeepPasswordOption`, `TestReceiverPasswordEditable`.
- 2026-11-09, session takeover: A clean control close during streaming is
  Preempted and does not reconnect; a close carrying an error stays
  retryable; the group forwards member preemption. Pins: protocol.md §11,
  devices.md, `TestCleanControlCloseDuringStreamingPreempts`,
  `TestControlCloseWithErrorIsNotPreemption`, `TestTakeoverStopsReconnect`,
  `TestGroupForwardsPreemption`.
- 2026-11-09, settings write ownership: One writer per field;
  `Store.Update` merges under a lock; engine shutdown no longer restores the
  app's update channel. Pins: internal/settings/ownership.go,
  implementation.md, `TestEngineShutdownKeepsTheAppsChannelChoice`,
  `TestEverySettingHasOneWriter`, `TestMergeCopiesOnlyOwnedFields`,
  `TestAppSaveLeavesEngineFieldsAlone`, `TestChannelChoicePersists`.
- 2026-11-09, multi-platform review and gate: The darwin port's fixed
  defects — platform-tagged adapter staging, no committed signing identity,
  the cross-platform import gate, the corrected Core Audio ABI, the Now
  Playing adapter mutex, the Windows focus seam, and the removed aggregate
  liveness check — are pinned by platforms.md, `TestCoreAudioABI`,
  `TestParseStreamLineIgnoresNonDataEvents`, and `scripts/check-platforms`;
  the review's `go test -race` run was green. The popover activation
  limitation is moot because 0.6.1 removed the popover style.
- 2026-11-10, latency, loss, and connect sweep: 90 and 100 ms held clean at
  the transport for 60 s, 80 ms took one retransmit in 15 s, and two-packet
  prefill with two copies kept 80/90 ms stable under 3% loss and 20 ms
  jitter; the cached pair auto-connect reached streaming in 1.92 s, while a
  30-minute app run was invalid after another instance took over and the
  audio device was invalidated (`0x88890004`), leaving the 5-minute run as
  the longest clean one. Pins: devices.md §HomePod, protocol.md §6,
  implementation.md, and the sender/engine tests.
- 2026-11-10, the local receiver is not a target: The host's own AirPlay
  receiver is marked `Local`, hidden with a 本机 chip, never auto-connected,
  and refused by the engine and the socket check. Pins: devices.md, ui.md,
  `TestQueryMarksTheLocalReceiver`, `TestMergeHidesTheLocalReceiver`,
  `TestReceiverPageLocalRowIsDisabled`, `TestConnectRefusesTheLocalReceiver`,
  `TestAutoConnectSkipsTheLocalReceiver`, `TestRefusesASessionToThisHost`.
- 2026-11-10, HomePod idle standby: Five seconds of silence stops the pacer;
  the standby clock counts from the last push; the display darkens at
  1/5/10/30 minutes (default 30), and the first sound restores state 1 and
  audio. Pins: docs/standby.md, devices.md, `TestFlyoutLampShowsStandby`,
  `internal/engine/standby_test.go`.
- 2026-11-10, presentation latency floor on a HomePod mini pair: The
  receiver's render pipeline breaks up below roughly 200 ms (severe at
  100 ms, frequent at 160 ms, mild at 180 ms, gone at 200 ms) while the
  sender's counters stayed clean, so production stays at the 200 ms preset.
  Pins: devices.md §HomePod. Not established: which receiver-internal buffer
  underruns, and whether larger, compressed blocks would move the floor
  below 200 ms.
