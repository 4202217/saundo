# Verification

This document is the acceptance path for saundo. The automated suite covers
the crypto core and the full sender handshake against an in-process fake
receiver. Real-device behavior is verified with the steps below.

## Automated checks

Run all three from the repository root before declaring a build good:

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

`-H windowsgui` prevents a console window. `make build` passes the same flag,
so `bin/saundo.exe` also opens without a terminal. For development, `go run
./cmd/saundo` keeps the console and its log output. In the GUI build a fatal
startup error goes to `%APPDATA%\saundo\saundo.log` instead of stderr.

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
- Settings are stored in `%APPDATA%\saundo\settings.json`.

## Results from 2026-10-31

On a LAN with two HomePods on software 27.2 (model `AudioAccessory5,1`):

- mDNS found both HomePods and merged their `_raop` and `_airplay`
  advertisements into one device each, with the friendly names `HomePod L` and
  `HomePod R`.
- HAP transient pairing, the encrypted control channel, session SETUP, the
  event channel, RECORD, and stream SETUP all succeeded against HomePod L.
- HomePod software 27 required PTP. With `timingProtocol: NTP` it accepted SETUP
  and RECORD but silently ignored the stream SETUP. With `timingProtocol: PTP`,
  `timingPeerInfo`/`timingPeerList`, and SETPEERS, it accepted the stream and
  began streaming.
- The PTP follower synchronized to the HomePod's own grandmaster: about 8
  Sync/Follow_Up exchanges per second, a stable offset within roughly 1 ms, and
  delay responses with a 4 to 11 ms round trip. The HomePod's PTP clock is
  boot-relative, not Unix epoch; only the offset matters.
- A 35-second muted session kept the event channel alive, answered feedback
  every two seconds, and sent 1,543,520 frames of ALAC silence without a drop.
- A four-second 440 Hz tone at 50% volume was then sent to HomePod L. The
  session stayed in streaming, and the receiver's volume was restored to its
  initial 10% (`initialVolume: -27` dBFS) before TEARDOWN.

The application itself was then exercised through its real window and tray:

- With `autoConnect` enabled in `%APPDATA%\saundo\settings.json`, the app
  discovered HomePod L, created the PTP follower, opened WASAPI loopback at
  48 kHz, paired with HAP transient, and reached `正在播放到 HomePod L` with
  `ALAC · 44.1 kHz · 立体声 · PTP · 已同步`. The log showed the stream SETUP,
  feedback every two seconds, and no drop for more than six minutes.
- The window's 断开 button stopped the session (`state: idle`,
  `session stopped`). The 连接 button re-paired and streamed again.
- Moving the volume slider sent `SET_PARAMETER volume` and the HomePod answered
  200. The slider was returned to the receiver's original 10%.
- Closing the window hid it and left the process running. The tray icon was
  found in the Windows hidden-icons overflow with the tooltip
  `saundo · 正在播放到 HomePod L`, and clicking it restored the window.
- A second run exercised the complete system-audio path. With the app streaming
  to HomePod L, the app volume was set to 50% through the slider and the HomePod
  acknowledged the change. The PC output was unmuted at 69%, a four-second
  440 Hz WAV was played through the default device, and the app captured it with
  WASAPI loopback, resampled it, encoded ALAC, and sent it over the PTP session.
  The session stayed live. The PC output was muted again and the app volume was
  returned to about 10%. The user confirmed hearing the four-second tone on
  HomePod L.

The Windows Firewall prompt appeared on launch. The session worked because the
PTP and mDNS traffic was solicited, but approving the app on the private network
is recommended so unsolicited event and timing traffic is never blocked.

Not verified on hardware: Apple TV on-screen PIN pairing and stored-credential
reconnect, macOS receivers, and a real default-device change during playback.
Audible output is confirmed by the user.

## Batch results (2026-10-31, second pass)

Each item below shipped with the test that pins it. `make check` (gofmt gate,
`go vet`, `go test ./...`) is green after the batch unless noted.

- **Auto-reconnect.** The engine retries a dropped or failed session with a
  1s/2s/5s/10s/15s backoff, capped at five attempts, then surfaces
  自动重连 5 次后停止. A user-initiated disconnect never schedules a retry,
  and toggling 断开后自动重连 off cancels a pending retry. Pinned by five
  engine unit tests (`internal/engine/engine_test.go`). A hardware drop was
  not reproduced, so the live path is covered by the unit tests only.
- **Per-device volume and receiver-volume adoption.** A remembered device
  reconnects at its stored volume; a first connection adopts the receiver's
  own volume from `initialVolume` instead of forcing 0 dB. Pinned by the
  sender test `TestInitialVolumeAdoptedWhenUserDidNotSetOne` and three engine
  tests.
- **Asynchronous capture restart.** Switching capture endpoints no longer
  blocks the event loop on WASAPI initialization; a queued restart supersedes
  the in-flight one. Pinned by `TestAsyncCaptureStartDoesNotBlock`, and a
  mid-session capture-rate change updates the sender and resets the ring
  without touching the session (`TestCaptureRateChangeMidSession`). A real
  default-device switch during playback is still on the manual list.
- **Device online status and AirPlay groups.** Discovery marks a device
  offline after three missed query cycles, prunes it after 90 seconds, parses
  `gpn`/`gcgl`/`igl`, and merges group fields across the two services. The UI
  shows 离线 and the group name. Pinned by three discovery tests.
- **Log rotation and copy.** The log rotates at 5 MB to `saundo.log.1`;
  复制日志 puts the visible log on the clipboard (verified live: the
  clipboard contained the `saundo dev` line). Rotation pinned by
  `TestRotation`.
- **Single instance.** `mygo.App.RequestSingleInstanceLock` runs first; a
  second launch hands over and exits 0, leaving one process (verified live).
- **Autostart toggle.** 开机自动启动 writes and removes the
  `HKCU\...\Run` value `saundo = "...saundo.exe" --mygo-opened-at-login`
  through `mygo.App.SetOpenAtLogin` (verified live, both directions).
- **Now Playing send path.** `SetNowPlaying` rides as DMAP
  `application/x-dmap-tagged` with `minm`/`asal`/`asar` tags; pinned by
  `TestNowPlayingRidesAsDMAP`. The Windows SMTC source landed in the third
  pass; see the Now Playing display section below.
- **Legacy Apple TV receivers.** A Xiaomi TV advertising `AppleTV3,2`
  accepts `pair-pin-start` (200) but rejects pair-setup M1 with HTTP 400:
  pre-HomeKit legacy pairing. Later live testing showed its audio requires
  FairPlay, so discovery maps `AppleTV3,x` to `AuthLegacyPin` and the sender
  fails fast with a FairPlay message. Pinned by
  `TestLegacyAppleTVFailsFastForFairPlay` and
  `TestRedmiFairPlayReceiverAndNameDecoding`.
- **30-minute soak.** `TestLiveSoak` streamed silence to HomePod L for exactly
  30 minutes: 79,380,224 frames — precisely 44100 × 1800, one packet pace the
  whole way — with zero ring drops, the PTP clock ready throughout, feedback
  answered every 2 seconds, and no session drop.
- **Cold start after a long session.** Immediately after the 30-minute soak,
  a fresh session needed more than the 10-second handshake window to
  re-establish PTP twice in a row, so the handshake deadline now extends to
  30 seconds while waiting for the clock (pinned by
  `TestPTPWaitToleratesSlowClock`). With that window, the measured cold start
  to streaming was 10.81 s (`TestLiveColdStartTiming`, acceptance < 15 s).

## PTP and Windows Firewall

HomePod software 27 uses PTP over UDP ports 319 (event) and 320 (general).
The app binds both. Windows Firewall must allow inbound UDP to `saundo.exe`
on the private network, as it must for mDNS replies and NTP timing. The app
cannot synchronize the clock and will not start the stream if those packets are
blocked; the UI reports a PTP timeout with this hint.

## Batch results (2026-10-31, third pass)

- **Application capture (per-app audio).** `capture.Config.ProcessID` selects
  the process-loopback virtual device (`VAD\Process_Loopback`) through
  `ActivateAudioInterfaceAsync` with a Go-implemented
  `IActivateAudioInterfaceCompletionHandler`. Application listing unions
  visible windows with audio-session processes, deduplicates to same-exe
  process-tree roots, and marks which are actively rendering. Verified live:
  11 applications listed with names and window titles; a tone rendered by
  `powershell.exe` was captured per-app — 43,680 real audio frames at 48 kHz
  (`TestAudioApplicationsLive`). Findings recorded: the VAD requires float32
  stereo, zero buffer duration, and the LOOPBACK flag; it rejects
  `Initialize` with E_INVALIDARG until the target's audio graph is up (a
  brief in-place retry bridges that); and `AudioApplications` needs
  `IAudioSessionManager2`, whose IID is
  `77AA99A0-1BD6-484F-8BC7-2C654C9A9B6F` (an earlier draft used a wrong tail
  and got E_NOINTERFACE).
- **Receiver volume two-way sync.** While streaming, the sender polls
  `GET /info` every second and reads `initialVolume`, which AirPlay 2
  receivers update as their hardware volume moves (the AirFlash volume
  worker's source). A moved knob reaches the UI through
  `Events.VolumeChanged`, mirrors into the per-device memory, and the slider
  follows. Pinned by `TestReceiverVolumePollAdoptsHardwareChanges`.
- **Latency UI.** The 播放延迟 slider (20-2000 ms) drives the stream SETUP's
  `latencyMin`/`latencyMax` and the sync-packet timeline on both PTP and NTP
  sessions; pinned by `TestLatencyAppliesToStreamSetup` and
  `TestLatencyAndPasswordReachSession`.
- **Password receivers.** The receiver-password field stores a per-device password
  that `SetAuth` passes to the RTSP digest path (pinned by
  `TestLatencyAndPasswordReachSession`).
- **Now Playing source.** A new `internal/nowplaying` package reads the
  Windows SMTC session (RoGetActivationFactory → RequestAsync →
  GetCurrentSession → GetMediaPropertiesAsync, all awaited by polling
  IAsyncInfo status, no event delegates). The app polls every two seconds
  when 推送正在播放 is on and forwards title/artist/album to the receiver;
  forwarding is pinned by `TestNowPlayingCommandForwardsWhenEnabled` and
  `TestNowPlayingRidesAsDMAP`. A real Chrome Media Session was later read live;
  see the Now Playing display section below.
- **Silent tray start.** `mygo.App.WasOpenedAtLogin()` hides the window at
  startup (`--tray` forces the same); the app runs from the tray and the
  window returns from the tray icon (verified live with the packaged binary).
- **Packaging.** `cmd/saundo/mygo.json` (name, identifier, version 0.3.0,
  slug, and icon `../../resources/icon.png`); `mygo build` produces
  `saundo.exe` with ProductVersion 0.3.0 and the embedded icon (verified via
  PowerShell), and `saundo-setup-0.3.0-amd64.exe`, an NSIS installer.
- **Icon redesign.** The flat mark is a centered triangle with two concentric
  arcs. The 1024 px `resources/icon.png` uses the Signal palette, and
  `resources/icon-dark.png` the Pulse palette. The 32 px tray icon repeats the
  mark in black on a light taskbar and white on a dark one, re-drawn on
  `Tray.OnAppearanceChange`. It does not encode connection status; the tooltip
  and tray menu carry that. `TestTrayIconMonochrome` pins the 32 px size and
  the black or white ink. The exe's static icon is the light Signal tile.

## Build fix (2026-10-31)

- **Blank terminal beside the app.** `make build` omitted `-H windowsgui`, so
  `bin/saundo.exe` linked as a console-subsystem PE (subsystem 3) and
  Windows allocated an empty console window on every double-click. The build
  now links `-H windowsgui` and the binary reports subsystem 2 (verified by
  reading the PE optional header). `TestBuildTargetProducesGUIBinary` fails if
  the flag is removed from the Makefile.
- **Embedded icon path.** mygo resolves the default `resources/icon.png`
  against `cmd/saundo`, where no resources directory exists, so the exe
  carried Windows' generic application icon. The earlier PowerShell check
  printed `ICON OK` for that generic icon, which only proved a non-null icon
  was returned. `mygo.json` now names `../../resources/icon.png`; extracting
  the icon from `cmd/saundo/build/windows-amd64/saundo.exe` shows the
  new 32 px mark. `TestMyGoIconPathExists` fails when the path is missing or
  is not a square PNG.
- **Bin build carries the icon.** `make build` used a plain `go build`, so
  `bin/saundo.exe` had no Windows icon resource and no mygo manifest. It
  now runs `mygo build -platform windows/amd64` with `MYGO_LDFLAGS` carrying
  the version, then copies the result to `bin/saundo.exe`.
  `TestBuildTargetProducesGUIBinary` fails if the target stops using mygo.

## Bug fixes (2026-10-31, fourth pass)

- **Connected device display.** The engine's `selectedID` was initialized from
  `lastDeviceId` and `connect` never updated it. The snapshot's selected ID,
  the connected name, the UI 已连接 badge, the remembered volume, and the
  reconnect target therefore stayed on the old device. A live run at 13:59
  connected HomePod R: the sender logged `streaming to "HomePod R"` while the
  window showed `正在播放到 HomePod L` and the HomePod L row carried 已连接.
  `connect` now selects the device being connected, and `lastDeviceId` is
  persisted after the session first reaches streaming, so a failed attempt
  never becomes the next auto-connect target. Pinned by
  `TestConnectSelectsTheDeviceBeingConnected` and
  `TestFailedLaunchDoesNotReplaceLastDevice`. The same window then showed
  正在播放到 HomePod R and `settings.json` stored HomePod R's ID.
- **Sticky capture error.** WASAPI loopback sets `DATA_DISCONTINUITY` when the
  render stream pauses or the device is reconfigured, and the flag can arrive
  spuriously (miniaudio documents the same behavior). Capture reported it as
  an error, and the engine never cleared `captureError`, so 采集错误 stayed on
  screen until restart. On the pre-fix build a normal 400 ms tone produced
  20,160 frames and one discontinuity error, and the window kept showing
  `采集错误：capture: audio discontinuity on the capture stream`. Capture now
  ignores the flag, and a successful capture notification clears any stale
  error. Pinned by the new zero-error assertion in
  `TestLoopbackReceivesSamples` (it failed with one error before the fix) and
  by `TestCaptureErrorClearsWhenCaptureRecovers`. After the fix the same live
  tone left the status at `采集：PHL 27E2F7901 ... · 48000 Hz` with no error.
- `gofmt -l .` and `go vet ./...` printed nothing; `go test ./...` passed every
  package.

## Stereo groups (2026-10-31, fifth pass)

- **Discovery.** A real HomePod pair advertises `tsid=00000000-0000-4000-8000-000000000000`
  on both members, `gpn=HomePods`, and `igl=1` only on HomePod R. Discovery now
  groups by `tsid`, names the target from `gpn`, orders the leader first, hides
  the physical member rows, and marks the group offline unless both members are
  online. `gcgl` alone no longer marks a follower as leader. Pinned by
  `TestSnapshotCollapsesStereoPair`, `TestIncompleteStereoGroupIsOffline`, and
  `TestGCGLDoesNotMarkTheMemberAsLeader`.
- **Group sessions.** Each member gets its own AP2 session with
  `groupUUID=<tsid>`; SETPEERS carries both member addresses plus the sender.
  Pinned by `TestStereoGroupSessionCarriesGroupUUIDAndAllTimingPeers`. Capture
  fans out to one ring per member; pinned by the `ring.Fanout` tests and
  `TestGroupMemberRingsTakeOverCapture`. An incomplete group never opens a
  session (`TestOfflineStereoGroupDoesNotStart`).
- **Sender PTP master.** A pair expects the sender to supply the PTP timeline.
  `internal/airplay/ptp/master.go` binds UDP 319/320 once for both members,
  sends Announce/Sync/Follow_Up, answers Delay_Req and Pdelay_Req, and grants
  unicast subscriptions. Wire shape, grant validation, per-peer readiness, and
  the per-member clock view are pinned by the PTP package tests. A standalone
  HomePod still uses the follower path.
- **Live pair.** `TestLiveStereoGroup` (gated by `SAUNDO_STEREO_TEST=1`)
  found `HomePods` with HomePod R as leader, opened both member sessions, and
  reached streaming in the same run: PTP active and ready for both members,
  two member rings, volume 10. A four-second 440 Hz tone pushed through the
  capture fanout left the group streaming with zero dropped samples. The run
  took 8.93 seconds from test start to disconnect.
- **Real window.** The rebuilt app listed one `HomePods · 立体声组 · 2/2` row
  and hid the two member rows. Clicking 连接 produced
  `正在播放到 HomePods`, the 已连接 badge, and `ALAC · 44.1 kHz · 立体声 · PTP ·
  已同步`. A real system-audio tone played through the default WASAPI endpoint
  while the app was streaming; the status stayed `采集：Realtek Digital Output
  ... · 48000 Hz`, no capture error appeared, and 丢帧 stayed 0.
- `gofmt -l .` and `go vet ./...` printed nothing; `go test -count=1 ./...`
  passed every package, including the PTP bind tests once the app released
  UDP 319/320.

Not verified: user-confirmed audible output from both speakers, multi-room
groups larger than two, a member dropping or reconnecting mid-pair, and
software stereo groups built from independent receivers.

## Now Playing display (2026-10-31)

- **The toggle did nothing after startup.** `pollNowPlaying` read
  `a.cfg.ShowNowPlaying`, a settings copy loaded once when the app started. The
  switch only updated the engine, so enabling 推送正在播放 after launch never
  let the poller run. The poller now reads `snap.ShowNowPlaying` and the
  streaming state from the live engine snapshot.
- **The current track is visible.** The playback card now has a 正在播放 row.
  It updates every two seconds even with pushing off, shows
  `title · artist`, falls back to 未检测到媒体, and has the 推送 switch beside
  it. The duplicate switch in the settings card was removed. Pinned by
  `TestNowPlayingRowShowsTrack`.
- **Push behavior.** A changed track is sent only while the toggle is on and a
  session is streaming; a new stream resets the sent marker and resends the
  current track after reconnect. Pinned by
  `TestRefreshNowPlayingPushesOnlyWhenEnabled`.
- **Live SMTC read.** A muted Chrome window using the Media Session API created
  a real system media session. The rebuilt app displayed
  `SMTC 测试曲目 · 测试歌手`; when the browser switched to the user's other
  Chrome tab, the row followed it to that tab's title. Closing media returned
  the row to 未检测到媒体 on the next poll. The receiver-side push was not
  re-run live because connecting would have streamed the user's browser audio
  to the HomePods; the DMAP forwarding path remains covered by the sender
  tests.
- `gofmt -l .` and `go vet ./...` printed nothing; `go test -count=1 ./...`
  passed every package after the change.

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

## Live-edge start and pair lockstep (2026-10-31, sixth pass)

- **The defect.** Capture runs from app launch into a 4 s overwrite-oldest ring,
  but a session replayed that entire idle backlog from its oldest end. A stereo
  pair was worse: each member fixed its own `clockStart` and consumed its own
  ring from the oldest end at its own handshake instant, so one member stayed
  permanently ahead of the other. The pre-fix tree failed the repro at
  `internal/airplay/sender/latency_test.go` with
  `first sample = 1111, replaying the oldest pre-connect backlog`.
- **The fix.** `ring.Buffer.KeepNewest` and the atomic `ring.Fanout.KeepNewest`
  keep the newest window without touching the capture drop counter. One
  `sender.StartGate` per session owns the shared epoch and the single trim.
  A member arrives when its receiver accepts the stream (AP1 RECORD, AP2 stream
  SETUP); the last arrival seals the epoch. Playback begins from the engine's
  next `Tick` once the epoch is reached, so both pair members arm their pacers
  in the same Tick. Sync packets publish `latency + frames since epoch`, not the
  number of packets sent, which removes pacer-tick jitter from the pair offset.
  The trim keeps a 50 ms cushion (`DefaultStartCushion`).
- **Tests.** `TestStreamingStartsFromTheNewestCapture` now asserts the first
  decoded sample is the newest cushion boundary, that no stale marker survives,
  and that `Dropped()` stays zero. `TestPairStartsInLockstep` proves no member
  sends before every arrival, then byte-identical packets and identical first
  sync mappings. `TestStartGateSealsOnLastArrivalAndTrimsOnce`,
  `TestStartGateRateMismatchFailsClosed`, `TestStartGateZeroMemberClamp`,
  `TestStartGateZeroCushionTrimsEverything`,
  `TestStartGateNilTrimStillSuppliesTheEpoch`, and
  `TestStartGateArriveAfterSealIsNoOp` pin the gate rules.
  `TestKeepNewestKeepsTheNewestSamples`,
  `TestKeepNewestZeroNegativeAndOversized`,
  `TestKeepNewestWrappedBuffer`, and
  `TestFanoutKeepNewestTrimsEverySinkToOneWindow` pin the ring and fanout trim.
  The sender tests that preloaded `fillRing` now reset the ring after launch and
  push their payload instead of asserting the whole pre-start ring is replayed.
- **Checks run.** `gofmt -w` on every changed file; `go vet ./...` printed
  nothing; `go test ./...` passed every package.
- **Not verified.** Audible output and the 50 ms cushion under real WASAPI
  scheduling jitter are still pending real-device runs, as is measured
  left/right offset on a physical pair. The design adds about 50 ms of true
  end-to-end latency.

## Settings window split (2026-10-31)

- **Layout.** The tray flyout now carries playback and session controls only:
  device list, connect/disconnect, volume, Now Playing push, capture status,
  and the diagnostics log. Sender name, receiver password, auto-connect,
  reconnect, strict auth, latency, capture sources, and launch at login moved
  to a separate 480-wide `mygo.NewWindow` titled 设置 · saundo. The window
  is created hidden at startup, before the engine starts, so engine callbacks
  on other goroutines never race its field.
- **Open paths.** 设置 in the flyout header, or 设置… in the tray menu. The
  flyout hides when the settings window opens; closing the settings window
  hides it and the app stays in the tray.
- **Rendering pin.** `TestSettingsViewRendersConfiguration` renders every
  moved control, and `TestFlyoutKeepsPlaybackControlsOnly` fails if any of
  them returns to the flyout. Both views were rendered to PNG with
  `ui.NewTester` and inspected; the layout and the selected-device password
  hint are correct.
- **Live window.** The rebuilt `bin/saundo.exe` ran at 200% display
  scaling. A `PrintWindow` capture showed the flyout without the
  configuration cards and with 设置 in the header. Clicking 设置 hid the
  flyout and showed a `MyGoWindow` titled 设置 · saundo with all four
  cards and the 保存到 HomePods password hint. Clicking the title-bar close
  button hid the settings window while the process stayed alive; clicking
  设置 again reopened it.
- **Not separately exercised.** The tray menu 设置… entry, and a redraw of the
  open settings window driven by an engine snapshot change.
- `gofmt -l .` and `go vet ./...` printed nothing; `go test -count=1 ./...`
  passed every package.

## Silence suspend (2026-10-31)

- **What changed.** The engine now suspends the audio pacer after 2 s of
  digital silence and resumes on the first audible block. The capture fanout is
  wrapped by a silence meter that drops silent blocks while suspended, so the
  rings do not fill or count capture overflow. The sender keeps sync, feedback,
  volume polling, and the event channel running; resume re-anchors the RTP time
  to the wall clock, trims the ring to the 50 ms start cushion, rebuilds the
  resampler, and re-arms the pacer.
- **Protocol result.** `SETRATEANCHORTIME rate=0/1` returned RTSP 200 with
  plist `status: -12782` on both HomePods, with and without a `Session`
  header, and with the full rate-1 anchor (`rtpTime`, `firstAudibleRTPTime`,
  `networkTimeSecs`, `networkTimeFrac`, `networkTimeTimelineID`,
  `networkTimeFlags`). The method is for buffered streams; the realtime stream
  takes timing from the UDP sync channel.
- **Live runs.** A stereo pair (HomePod R/L) streamed a quiet 440 Hz tone. A
  60 s suspend kept both sessions in `streaming`, sent zero audio packets,
  kept PTP ready, and resumed at 44,100 frames/s after the re-anchor. A 10 s
  suspend with and without `SETRATEANCHORTIME` was audibly confirmed by the
  user: silence during the pause, clean tone on resume, no pops.
  `/playback-info` returned 455, so the receiver exposes no playback state on
  the AP2 realtime session.
- **Checks run.** `gofmt -l .` printed nothing; `go vet ./...` and
  `go test ./...` passed. The sender and engine gained unit tests for the
  pause/resume anchoring, idempotence, AP1 fallback, silence meter forwarding,
  and the engine state transitions.
- **Not verified.** AirPlay 1 receivers are deliberately excluded (the pause
  request returns false and they keep streaming silence); Apple TV realtime
  sessions are pending. macOS password pairing and the FairPlay finding are
  covered below. Pauses longer than 60 s were not soaked.

## Results from 2026-11-01 (Mac password and FairPlay finding)

- **MacBook Air, Mac14,2, Require Password.** The app asks for the AirPlay
  password on first connect and saves it; HAP pair-setup then used it: M1
  carried method 0 without Flags, M3/M4 completed, M5/M6
  stored credentials, and pair-verify encrypted the control channel. `GET /info`
  returned 200; the AP2 session SETUP returned 401 with a Digest challenge, and
  the RFC 2069 replay (username `AirPlay`) returned 200. The event channel
  opened, RECORD and SETPEERS returned 200, the PTP follower synchronized after
  27 exchanges, the stream SETUP returned a data port, and the sender reached
  `streaming` with 132,000 encrypted ALAC frames (3 s). Volume and feedback
  returned 200. Pinned by `TestLiveMacPassword` (`SAUNDO_MAC_TEST=1`,
  `SAUNDO_MAC_PASSWORD`, optional `SAUNDO_MAC_TONE=1`) and the unit tests
  `TestHAPPasswordPairsAndStreams`, `TestAP2DigestChallengesSessionSetup`, and
  `TestUsesPTPSelectsHomePodAndMac`.
- **Redmi TV, AppleTV3,2 (Xiaomi AirPlay), FairPlay required.** Discovery
  decoded the name to `REDMI 显示器 …`. The TV accepted the AP2 stream-only
  SETUP and RECORD with 200, but produced no sound with ALAC, PCM, encrypted,
  or plaintext audio. Its `/fp-setup` returns valid M2/M4, the iPhone Music app
  plays to it, and AP1 `SETUP` returns 400 while the AP2 session `SETUP` returns
  500. The audio path requires Apple FairPlay, which the project does not
  implement. The experimental open AP2 code was removed; discovery now maps
  `AppleTV3,x` to `AuthLegacyPin` and fails fast with a FairPlay message.
  Pinned by `TestLegacyAppleTVFailsFastForFairPlay` and
  `TestRedmiFairPlayReceiverAndNameDecoding`.
- **First-connect password dialog.** A receiver that needs an AirPlay password
  (`AuthHAPPassword` or `AuthPassword`) and has none stored no longer starts a
  session. The engine publishes `PasswordRequired`; the panel opens a masked
  password dialog, saves the value through `SetReceiverPassword`, and connects.
  Pinned by `TestMissingPasswordPromptsAndUsesIt` and
  `TestPasswordPromptRenders`.
- **Checks run.** `gofmt -l internal cmd` printed nothing; `go vet ./...` and
  `go test ./...` passed. The Mac live test was rerun; the Redmi FairPlay
  finding is recorded above.

## Adaptive flyout height (2026-11-01)

- **The defect.** The tray flyout was a fixed 420x500 DIP window while the
  device card already shrank with the device count. With fewer than three
  devices the leftover height collected in the spacer between the device card
  and the 播放 section: a 56 DIP band of empty panel for two devices, 112 for
  one.
- **The fix.** `deviceCardHeight` and `flyoutHeightFor` in
  `internal/app/flyout.go` share the device card's row geometry, and
  `App.fitFlyout` resizes the tray flyout when the device count changes: three
  or more devices 500 DIP, two devices 444, one device 388, none 470. The
  flyout is anchored to the tray icon by its bottom edge, so a resize grows or
  shrinks it upward and leaves the playback card in place. The spacer stays as
  the design's ~23 DIP leftover, absorbing font and DPI differences.
- **Tests.** `TestFlyoutHeightForDeviceRows` pins the four heights.
  `TestFlyoutKeepsPlaybackGap` renders each count at its target height and
  requires the gap from the device card to the 播放 head to stay within 1 DIP.
  `TestFlyoutTargetHeightFitsPlayback` checks the capture line is not clipped
  at the shorter heights. The empty and one-device previews now render at
  their target heights.
- **Checks run.** `gofmt -l .` and `go vet ./...` printed nothing;
  `go test ./...` passed every package. PNG renders at 500, 444, 388, and
  470 DIP confirmed the spacing and showed no clipped rows.
- **Live check (2026-11-01).** After the rebuild and restart, the user
  confirmed that the two-device flyout fits its content with no empty band,
  which runs `fitFlyout`'s `SetSize` path on the real tray flyout. Device
  churn while the panel is open, and the upward growth that comes with it,
  were not exercised.

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
| Discovery empty | mDNS blocked, wrong network profile, VPN, or AP/client isolation. |

## Evidence to capture before changing protocol code

When behavior differs from this document, capture:

1. The diagnostics log from the app (诊断日志).
2. A packet capture filtered to the receiver's address.
3. Whether the receiver answered each RTSP method, and with which status.
4. Whether the event channel received and answered requests.

Then update `docs/architecture.md` with the finding before changing the
protocol order or keys.
