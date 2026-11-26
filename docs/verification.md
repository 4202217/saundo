<!-- reviewed-through: 2026-11-25 -->

# Verification

This document is the acceptance path for saundo. The automated suite covers
the crypto core and the full sender handshake against an in-process fake
receiver. Real-device behavior is verified with the steps below.

Dated results append below and are never rewritten. They are compressed,
never deleted, once every claim in a dated section has a durable home: a
named test, devices.md or protocol.md records it, the code that owns
the behavior documents it, or a later dated entry supersedes it. The release
audit moves whatever qualifies to the Graduated section at the end of this
file — one entry per batch: date, surviving conclusion, pin. Each release's
audit graduates that release's own dated batch; it never defers the batch
it just produced. When the file passes roughly 800 lines, compress at the
next opportunity without waiting for a release.

## Currently pending

Live verification still owed, consolidated from the dated notes below.
Historical "not verified" lines are frozen as of their date; this list is the
live state:

- Apple TV on-screen PIN pairing and stored-credential reconnect — the
  remaining definition-of-done gap.
- Apple TV realtime sessions: an Apple TV AP2 realtime session has not been
  run.
- A 30-minute soak at the 200 ms production preset with a stable audio
  device and no other app takeover. The 100 ms soak is withdrawn: the
  2026-11-10 presentation-latency-floor entry in Graduated shows the HomePod
  mini pair breaks up below roughly 200 ms whatever the sender.
- A live long-idle resume on a HomePod pair: leave the session silent for
  more than 30 minutes, then play; the first audible block must rebuild the
  cohort and the pair must play. The 2026-11-24 rebuild has only headless
  coverage so far.
- A resume after a 30-minute-plus suspension on a HomePod pair (the rebuild
  path): the 2026-11-25 18 m 10 s suspension was clean with the resume flush,
  but the threshold itself and the rebuild behind it are still untested live.
  An AP2 receiver outside the HomePod family (the MacBook Air) is also
  unverified under the suspension and the flush.
- A live bandwidth re-measure of the duplicate policy: two HomePods
  standalone and the same two as a combo should settle near 360 KB/s total
  on a clean link after one closed session. The 2026-11-25 change has
  headless coverage only.
- Local output mute: the connection page showed the switch on with
  本机已静音；断开或退出时恢复 during the 2026-11-11 live pass, and the
  engine session path passed (`TestLiveMuteLocal`); toggling the switch
  itself has only headless coverage.
- Local output mute across a live latency switch: a 2026-11-13 live HomePods
  run logged the mute once and then four in-place stream rebuilds with no
  restore between them, while earlier lines in the same log show a restore
  after each session end. The log names no control per rebuild, so audible
  confirmation stays the user's. The engine path is pinned by
  `TestLatencyRestartKeepsLocalMute`, `TestLatencyRestartKeepsProcessMute`,
  and `TestReceiverDelayRestartKeepsLocalMute`.
- Local output mute across an added target, and its restore when the rebuild
  fails: no live run yet. The engine path is pinned by
  `TestAddTargetKeepsLocalMute`, `TestAddTargetKeepsProcessMute`,
  `TestAddTargetRestoresMuteWhenTheNewSessionFails`, and
  `TestAddTargetRestoresMuteWhenTheLaunchFails`; the row action is pinned
  by `TestRowActionKeepsOtherReceiversConnectable`. A live run should add a
  receiver while streaming, check the log shows no restore until the
  disconnect, then add an unreachable receiver and confirm the endpoint comes
  back.
- A multi-device session on real hardware: one capture stream on 2..N targets
  with one start epoch, a two-pair session (four PTP master peers), a live add
  whose gap is audible on the playing rooms, and a rejected add that leaves
  them playing. The engine behavior is pinned by
  `TestConnectWhileStreamingAddsTarget`, `TestRejectedAddLeavesCohortLive`,
  `TestCohortGateWaitsForEveryMember`, and `TestNewMasterAcceptsManyPeers`;
  the 2026-11-17 live session covers two standalone HomePods and a MacBook
  Air, and the two-pair, four-peer run plus a measured room offset remain
  owed.
- Multi-device acceptance checklist (2026-11-18). Two members stream and each
  volume slider moves only its own room; mute and unmute stay per device. A
  third target joins mid-playback and the original pair resumes in sync.
  Disconnecting one member pauses the others only for the rebuild and they
  resume; the gap is accepted for now. A PIN or receiver password prompt on
  one member leaves the other playing. Sleep, wake, lock, and a network change
  recover the session. The session identity is checked in the footer, the tray
  tooltip, the connection card, and the sidebar, which read the same member
  set. Still owed: three-member runs, a left/right assignment judged by ear,
  different per-device delays judged by ear, and a 30-minute three-member soak
  for drift, drops, CPU, and memory.
- Stereo combo acceptance checklist (2026-11-20). Live-verify: left and
  right judged by ear through a combo; swap and unlink while streaming; a
  member lost mid-stream from a combo (the combo target leaves the session;
  survivor-only playback is not implemented); a launch from auto-connect with a flagged combo;
  and a combo over a mini plus a full HomePod. The engine paths are pinned by
  the combo tests listed in the 2026-11-20 entry.
- Powering off a member mid-session (2026-11-18 live pass, after the member
  isolation fix). HomePod R was powered off while R and L streamed. The
  survivor kept playing its receiver buffer until the engine saw the lost
  member's control connection close at 20:42:23.700, logged `cohort: HomePod R
  left the session; continuing with 1 target(s)`, stopped the survivor at
  20:42:23.701, and resumed streaming at 20:42:25.069 (a 1.37 s rebuild gap,
  the accepted cost). R disappeared from discovery and returned on power-on.
  The delay before the gap is the survivor's buffer plus TCP close detection,
  not extra session work.
- Auto-connect after a member returns (2026-11-18, deferred by the user;
  premise updated 2026-11-21). A receiver that is powered off and comes back
  mid-session is not connected again: 启动时自动连接 is a launch-time
  multi-select choice — the launch connects every flagged receiver —
  evaluated when the app starts, and nothing watches a member reappearing
  while another target streams. The user asked to leave it for now. The
  eventual shape is either a per-receiver auto-recover or a launch choice
  that restores the last wanted set.
- Per-device volume and channel on real hardware: two standalone HomePods set
  left and right, a MacBook Air set stereo, different volumes on each, and the
  result confirmed by ear, from the flyout rows and the settings receiver rows.
  The settings row keeps the volume slider and the channel segment; the
  flyout's live device rows carry each target's volume slider, a single
  receiver's row adds the icon-only channel button, a stereo target draws
  none, and the playback card keeps the latency switch. The
  per-device delay row was removed from settings at the user's request, and the
  engine keeps the per-device delay data and its `UpdateReceiver` delay path
  for a later UI. The split, live change, and registration paths are pinned by
  `TestChannelMix`, `TestLiveSetChannelChangesTheNextPacket`,
  `TestBuildPlanTargetVolumeAndChannel`, `TestSetTargetVolumeAppliesOnlyItsTarget`,
  `TestInitialVolumeAdoptedPerTarget`,
  `TestReceiverChannelPatchSavesAndAppliesLive`,
  `TestFlyoutDeviceRowCarriesItsOwnVolume`, and
  `TestFlyoutChannelButtonNamesItsState`; no live per-device volume or channel
  run exists yet.
- A two-computer takeover with both apps auto-reconnecting: the second
  should show the takeover state, the first should keep streaming, and
  neither should re-enter the reconnect loop. The single-machine two-session
  probe and the engine test cover the classification.
- The diagnostics data in a live session: the SETUP `wiFiMetrics` object and
  its band values, and a stereo pair's worst/sum aggregation. The parsing
  and aggregation are pinned by the sender and engine tests; no live device
  has reported `wiFiMetrics` yet, and no UI consumes the model.
- A multi-homed receiver in a multi-device session (2026-11-21, after the
  socket-ownership routing fix). A receiver that answers from two
  interfaces, pooled with a second receiver: its timing replies and 0x55
  retransmit requests must reach its own sender. The engine paths are
  pinned by `TestCohortAttributesAUDPReplyToTheMemberThatOwnsTheSocket` and
  `TestAutoConnectDropsASilentFlaggedReceiverAndLaunchesTheRest`; no
  live multi-homed run exists yet.
- A real default-device switch during playback.
- Multi-room groups larger than two, and a member dropping or reconnecting
  mid-pair. Software stereo groups are consolidated into the stereo combo
  acceptance checklist above.
- A live macOS in-app update: the mac updater now follows the ad-hoc darwin
  manifest. The first release with that manifest is the baseline, and the
  first live update from it measures whether TCC audio capture and Keychain
  access survive the new cdhash. The local harness (`make test-mac-update`)
  covers the download, stage, swap, signature, and quarantine checks; the
  published feed and the permission prompts remain owed.
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
- The macOS App Nap hold: `TestSessionActive` and
  `TestPowerHoldTransitions` cover the state machine, but no live Mac run
  has shown the process stays unsuppressed while the flyout is hidden.

- The 0.6.4 verification-skill live pass: the six feature pages have source
  coverage and a green map, but they were not driven live because the release
  instance held the single-instance lock (the doctor passed read-only). Drive
  them on the e2e build at the next session.

## Automated checks

Run `mingw32-make check` from the repository root before declaring a build
good; it chains the gofmt gate, `go vet`, the architecture and size gates,
and `go test ./...`. Without mingw32-make:

```bash
go test ./...
go vet ./...
go run ./scripts/check-arch
go run ./scripts/check-size
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

- 发送端名称 changes the name the receiver shows on its next connection. A new profile defaults to the app language's brand, `Saundo` in English and `声渡` in Chinese; a stored name is kept.
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

Compressed 2026-11-14, mid-cycle, per the 800-line rule above: every dated
section from 2026-10-31 through 2026-11-13 moved here, one entry per batch.
The 0.6.1 audit's compression took the file from 2050 to 937 lines; this
one took it from 1627.

- 2026-10-31, tray flyout: The control panel is a `mygo.NewFlyout` anchored
  to the tray icon, frameless and shadowed at 480x720 DIPs, its root
  painting `Theme.Background`; left click toggles, outside or Escape
  dismisses, right click opens the menu, and login or `--tray` launches
  stay hidden. The menu installs only for the right-click popup's
  duration, so the primary click reaches `OnClick`. Both toggle directions
  verified on the live `MyGoFlyoutShadow` window. Pins:
  `internal/app/flyout.go`, ui.md.
- 2026-11-04, receiver migration and launch auto-connect: `Run()` migrates
  the legacy `autoConnect` and `lastDeviceId` from `settings.json` onto the
  stereo pair's `stereo:<tsid>` registry entry, clears the legacy flag,
  and writes `receivers.json`; the registry then drives the next launch
  with `autoConnect=false`, and a discovered target waits for presence
  before the engine opens both member sessions. Live on the HomePods
  pair. The receiver-management paths not driven live are the
  `Currently pending` entry. Pins: the `internal/receivers`,
  `internal/engine`, and `internal/app` tests and the
  `settings-receivers-*` / `dialog-receiver-*` previews.
- 2026-11-04, release 0.4.2: the draft assets passed `sha256sum -c` with a
  signed manifest (run 41758834319), and the published feed returned 200
  for the manifest and the installer. The installer is unsigned, so
  SmartScreen warns. The maintainer's smoke test is the release request.
- 2026-11-05, ambient light phase 1: `internal/lamp` renders the six
  presets as deterministic 600x600 sRGB JPEGs (quality 0.88), lazily
  cached; `settings.Lamp` stores the enabled flag and preset, an old file
  loads off + dune, and an unknown preset normalizes to dune.
  `device.TopDisplayCapable` requires the HomePod family at software 27.2
  or later and fails closed on an unknown model or version. The engine
  renders off the sender's tick path and reapplies when a session starts
  streaming; the sender deduplicates identical bytes and fans out per
  capable member. Live: the 5,1 pair displayed the lagoon and dawn lights
  once Sound Glow was enabled; the `/artwork/command` channel that worked
  then was superseded by `SET_PARAMETER` on 2026-11-06. Pins: the
  `internal/lamp`, `internal/device`, `internal/engine`, and app lamp
  tests; keystone `TestPresetsMatchCatalog` and `TestTopDisplayCapable`.
- 2026-11-07, release 0.5.0 defaults: local-output mute and ambient light
  default on; the receiver track-info push is unconditional and has no
  switch. Pins: `TestDefaultsEnableLocalMute`,
  `TestLampDefaultsOnAndCover`, `TestLampOldFileLoadsDefaults`. The draft
  runs 42264388577, 42314253794 (the mygo 0.8.0 rebuild), and 42339224999
  (the defaults commit `d1f6e76`, the CI check being the release gate
  after a local `json.SyntaxError` failure) passed checksum and signature
  checks; the release published and `releases/latest` served the 0.5.0
  stable feed.
- 2026-11-08, ambient preview and session following: the Now Playing card
  follows the SMTC current session on every read, and the switch arrow
  sets a pin that survives polls until another session becomes current
  (`TestNowPlayingFollowsTheCurrentSession`); the lamp preview follows the
  session the engine pushes, not the card's selection
  (`TestLampPreviewFollowsThePushedSessionNotTheCard`). The
  two-playing-sessions live drive stays in `Currently pending`.
- 2026-11-08, connection recovery and discovery names: `cmdDisconnect` and
  `connect` clear `lastError` when an attempt ends, cancel clears it
  before the idle snapshot, and `actionFor` keeps other online rows
  connectable in the error phase; `parseEntry` derives the service from
  the instance suffix, fixing the `._airplay._tcp` leak into display
  names. Pins: `TestUserDisconnectClearsConnectionError`,
  `TestConnectClearsPreviousError`,
  `TestRowActionKeepsOtherReceiversConnectable`,
  `TestParseEntryUsesTheAdvertisedService`.
- 2026-11-08, the macOS port: Core Audio capture aggregates a process tap
  with the default output, and the output device must be the aggregate's
  sub-device and clock master (a tap-only aggregate refuses with
  `kAudioHardwareIllegalOperationError`); MediaRemote returns empty
  payloads to third-party processes on 27.3, so Now Playing reads through
  the vendored adapter loaded inside `/usr/bin/perl` (the perl loader
  rejects relative framework paths and takes the function and its
  parameter as separate argv entries), one session only, no enumeration
  symbol, and send 0/1/4/5 and seek control the source app; icons render
  via `NSWorkspace iconForFile:` and an ImageIO thumbnail;
  LaunchServices resolves the localized app name and
  `NSFileManager displayNameAtPath:` replaces the missing
  `NSBundle localizedName`; the Keychain master key with AES-256-GCM
  round-trips, a rebuilt ad-hoc binary prompts once per item while a
  stable signing identity ends the prompts, and headless runs take
  `SAUNDO_CREDS_MASTER_KEY`; quarantined ad-hoc builds need Open Anyway
  once per cdhash. Discovery, connect, and audio to a HomePod mini were
  verified live on 2026-11-07. Pins: platforms.md, implementation.md.
- 2026-11-09, Windows side of the darwin port: the full gate green on
  Windows for the merged tree; live shell identity through
  `TestSMTCLiveSessions`; the flyout window is `MyGoFlyoutShadow|Saundo`
  and settings `MyGoWindow|Settings · Saundo`; `capture-window.ps1` is
  DPI-unaware on a 1.75x display, so evidence captures must be DPI-aware.
  The verification map was audited from source and corrected in its own
  commit. The 0.6.0-beta.1 release mechanics are in the 0.6.0 entry below.
- 2026-11-09, release 0.6.0 with 0.6.0-beta.1: the 196-frame UI review
  fixed two P0 truncations and four P1 terminology issues in `0789520`;
  the verification map was corrected in `b3e4eaa`; the beta draft (run
  42506672553) and the stable draft (run 42615271600) passed checksum and
  signature checks; the stable release published and the feed serves the
  signed 0.6.0 manifest.
- 2026-11-11, 0.6.1 UI mockup templates: both templates matched the
  shipped interface, including the idle-standby state and the height rule
  (459 base, 150 media, 44 latency, 90 ambient, 48 error, 53 expanded);
  superseded by the 0.6.2 and 0.6.3 template entries.
- 2026-11-11, 0.6.1 release audit: the docs audit corrected five live
  docs (devices.md, protocol.md, architecture.md, implementation.md,
  ui.md) and compressed this log from 2050 to 937 lines; the 242-image UI
  review fixed one P0 (the English standby subtitle truncated at 640 DIP)
  and one P2 (terminal periods, now in the punctuation gate); the live
  pass drove the built app through flyout, settings, health card, log
  dialog, standby row, and a disconnect/reconnect cycle
  (`.audit/release-0.6.1/live/`); harness fixes: the doctor matches the
  settings window by class, the capture script is DPI-aware, and the tray
  page matches by class and prefix. Draft run 42854757255 verified and
  the release published; the feed serves the signed 0.6.1 manifest.
- 2026-11-11, macOS in-app updates enabled: the mac build enters the
  updater path and requests `update-darwin-arm64.json`, expected 404
  until the first darwin feed; `make test-mac-update`
  (`scripts/test-mac-update.sh`) drives 0.6.1 through available, install,
  quit, and the bundle swap with signature and quarantine checks in a
  temporary tree that uses its own bundle ID and HOME, so it measures no
  TCC or Keychain prompts. The live update, the restart button, and
  cdhash-survival of TCC and Keychain stay in `Currently pending`.
- 2026-11-11, macOS audio IO realtime safety: the IOProc takes no lock
  and allocates nothing (one atomic stream pointer, scratch sized at
  open, atomic stats); the silence meter feeds a 100 ms level ring and
  the analyzer runs off the audio thread; `internal/audio/ring` is a
  lock-free SPSC ring. Pins: `TestIOProcPushDoesNotAllocate`,
  `TestIOProcOversizedBufferIsDropped`, `TestIOProcPushAndStatsRace`,
  `TestSilenceMeterPushDoesNotAllocate`, `TestLevelPumpFeedsTheMeter`,
  `TestPushPopDoNotAllocate`, `TestOverwritePushDoesNotAllocate`,
  `TestFanoutPushDoesNotAllocate`; the ring, capture, and engine
  packages pass `-race`. The `TestMuteGuardRecoversAfterCrash`
  test-only race noted here was fixed on 2026-11-14 (`acd63dc`).
- 2026-11-11, macOS output mute scope: the mute failed with OSStatus
  2003332927 ('who?') because `ScopeOutput` was `'out '` instead of
  `'outp'`; fixed, `DeviceHasOutput` sees output streams,
  `TestCoreAudioOutputScope` fails on the old value, the muteprobe-tagged
  `TestDarwinEndpointMute` passed on real hardware, and the user
  confirmed a HomePod connection mutes the local output.
- 2026-11-12, 0.6.2 release audit: the docs audit corrected the custom
  latency range (20 to 2000 ms, clamped to 20 to 4000) in protocol.md,
  the macOS update location rule in platforms.md, the mygo version date
  in architecture.md, the engine mute and level workers in
  architecture.md and implementation.md, and the whole-second
  LatencyText rule in ui-copy.md; the 250-image UI review fixed one P1
  (设备 misnamed a receiver in the three new engine errors, `cf8882d`,
  and the banned-terms gate now carries five past strings) and one P2;
  the live pass evidence is `.audit/release-0.6.2/live/`; draft run
  43060976433 verified and the release published, both manifests
  serving 0.6.2. The mockup templates gained the credential-recovery
  state and its +48 height rule.
- 2026-11-12, macOS per-app capture: a process tap mixes named processes
  (`initStereoMixdownOfProcesses:`), the HAL lists clients through
  `kAudioHardwarePropertyProcessObjectList` carrying `ppid`/`pbid`;
  helpers fold into the app whose bundle ID prefixes theirs, two live
  processes of one app are one row, a tap uses the default output as
  clock master and ignores `EndpointID`, and the engine mutes the
  default output for per-app sessions. The PID-0 zero-root fallback was
  a real bug caught live. The picker carries NSWorkspace icons. Live: an
  `afplay` tone delivered 32,256 frames from that process alone, and the
  user picked Chrome through the icon picker. Known gaps: the app list
  is built once at launch, a tap fixes its process list at creation, and
  an app with no HAL process object fails the open and retries with the
  backoff. Pins: `TestBuildAppListGroupsHelpers`,
  `TestBuildAppListSkipsSelf`, `TestAppFamilyFollowsBundlePrefix`,
  `TestOwnerApp`, `TestBuildAppListMergesDuplicateBundle`,
  `TestAudioApplicationsReadsLiveLists`, `TestMixdownTapDescriptionBuilds`,
  `TestAppIconLive`, `TestAppCapturePickerChoosesMultipleApps`.
- 2026-11-12, per-app local mute: a per-app tap fixes its local-output
  behavior at creation, so the capture loop rebuilds it `CATapMuted`
  while a session streams and unmuted on disconnect, setting off, or
  selection change (setting a live tap's description fails
  `kAudioDevicePermissionsError` `'!hog'`); Windows keeps the
  whole-output mute and the UI note says so. The operator confirmed the
  split live: Music on the receiver, other apps on the Mac. Pins:
  `TestMuteLocalProcessCaptureMutesTheSelectedApp`,
  `TestMuteLocalProcessFailureIsVisible`,
  `TestMuteLocalProcessOffRestores`,
  `TestMuteLocalProcessCaptureUsesDefaultEndpoint`,
  `TestMixdownTapMuteBehavior`, `TestPerAppCaptureLive`.
- 2026-11-12, multi-app capture and bundle-ID persistence: the engine
  stores `captureAppIds` and `captureAppNames`, the darwin leaf unions
  the bundle families into one mixdown tap and skips apps that are not
  running, `pid:<n>` covers a process without a bundle, and a saved
  `captureProcessPid` migrates to the bundle ID once the app list
  loads. The status row reads "N 个应用", "N/M 个应用" with a warning
  dot, or 等待所选应用; a real capture failure stays red with its detail
  in the tooltip. Pins: `TestAppsTapIDs`,
  `TestCaptureAppsCommandPersistsAndStartsTheSelection`,
  `TestAppCapturePickerChoosesMultipleApps`,
  `TestMigrateLegacyAppSelection`,
  `TestMuteLocalAppSelectionMutesTheApps`,
  `TestCaptureStatusCountsMultipleApps`,
  `TestCaptureStatusShowsPartialSelection`,
  `TestCaptureStatusWaitsForTheApp`, `TestCaptureNoAppProcessWaits`.
- 2026-11-12, Windows per-app capture withdrawn: Windows offers system
  capture only. The process loopback copies the app's audio without
  stopping local playback, muting the app's session also silences the
  capture (RMS 0.1735 before, 0.0000 after), and the endpoint mute
  silences every app; the split needs a virtual audio driver, outside a
  pure-Go no-driver app. The process-loopback leaf stays in the tree,
  unexposed. Pins: `TestAppCaptureNotOffered`,
  `TestUnsupportedPlatformDropsAppSelection`,
  `TestCapturePageHidesPerAppWhenUnsupported`.
- 2026-11-12, Windows loopback carries endpoint volume: on Realtek
  Digital Output the captured PCM carries the endpoint volume (RMS
  0.0706 at step 50 down to 0.0001 at 0; the 0%/100% ratio is 0.001) and
  the endpoint mute does not reach the tap (0.1679 before, 0.1768
  muted); a quiet PC attenuates the receiver by the same amount, and
  saundo leaves the endpoint volume alone. The macOS process tap is
  expected to sit before the output volume; unmeasured on a Mac.
- 2026-11-13, Windows endpoint-volume takeover removed: the connection
  mute no longer changes the render endpoint's volume; it still mutes
  the endpoint while a session streams and restores it afterwards.
  Removed after user feedback: the takeover owned a system-wide setting
  for the session, re-applied it every second, overrode the Windows
  volume keys, and needed a crash guard. An old guard's volume field is
  ignored, and the next launch releases the mute.
- 2026-11-13, AP2 first-connect volume fail-closed: only a finite
  numeric `initialVolume` in [-144, 0] counts as known (`plistRealOK`,
  `volumeDBInRange`); an unknown value sends no `SET_PARAMETER volume`
  so the receiver keeps its own level, and below -144 clamps to -144.
  The C++ reference force-pushes 0 dB; saundo deliberately does not.
  Pins: `TestFirstConnectUnknownVolumeSendsNoRequest` and
  `TestVolumePollKeepsTheStoredVolumeOnUnknownField` fail without the
  fix; the remaining `TestFirstConnect*` and `TestVolumePoll*` guards
  pin the invariants.
- 2026-11-13, tray menu: one row per visible receiver, each drawing its
  `deviceIconKey` family glyph as a menu template in the text color; the
  in-session receiver keeps its family glyph in the palette accent
  (#4f46e5 light, #a5b4fc dark). A bullet, a check mark, and a
  connection-glyph swap were each tried and reverted for the connected
  mark; color means connected, not chosen. There is no 断开连接 row (the
  flyout's active row offers it); the mute row draws bell/bellOff and
  toggles `MuteLocalOnConnect`; the lamp row reads 关闭氛围灯 or
  开启氛围灯 with lamp/lampOff glyphs bound to `Snapshot.LampOff`; a
  连接上次接收器 fallback, disabled without a remembered receiver,
  precedes 显示托盘面板, 设置…, 检查更新…, and 退出声渡; pause has no row
  because the engine has no public pause command. Offline rows stay
  clickable through the remembered endpoint. The cached menu rebuilds
  when `trayMenuModel.key()` changes: locale, session phase, target,
  mute, lamp, connected, and dark. Live runs on 2026-11-13 confirmed the
  shape while streaming to HomePods. Environment finding: the session's
  local-output mute explains `TestLoopbackReceivesSamples` failing on
  this machine (it reproduces at 37352a4), so the failure is
  environmental, not a regression. Superseded 2026-11-14: the in-session
  row's click now ends the session, so the row itself carries the
  disconnect (the tray-toggle entry above). Pins: `TestTrayMenuShapeSelected`,
  `TestTrayMenuHasNoDisconnectRow`, `TestTrayMenuLampRow`,
  `TestTrayMenuMuteIcon`, `TestTrayMenuDeviceIconFollowsSession`,
  `TestTrayMenuKey`, `TestTrayMenuDeviceSubtitle`,
  `TestTrayMenuFallbackWithNoDevices`,
  `TestTrayMenuFallbackForHiddenTarget`.
- 2026-11-13, ambient-light copy names its object: the shared
  `lampTurnOff`/`lampTurnOn` keys feed the tray row and the flyout power
  button's accessible label (关闭氛围灯 / 开启氛围灯, Turn off / Turn on
  ambient light); the 已关灯 chip and 自动关闭氛围灯 setting keep their
  own copy. Pins: `TestTrayMenuLampRow` and the catalog gates. Superseded
  2026-11-16: the tray lamp row is the ambient submenu; pins moved to
  `TestTrayMenuLampSubmenu` and `TestTrayMenuLampOffDisablesChoices`.
- 2026-11-15, a stereo pair follower never takes mid-session artwork from
  its own session: HomePod L (the group row's host, the leader) followed
  every template change while HomePod R (`AudioAccessory5,1`, software
  27.3) stayed on its session-start image, although every push reached
  both members over their own RTSP sessions with identical bytes and
  both answered `200 OK` each time. Three live experiments on one
  session pinned the rule. (1) Five artwork-only template switches at
  19:56:02–13 (ember, dune, moss, tide, orchid): L followed all, R
  ignored all. (2) Manual off/on at 20:04:07–13 sent real
  `updateMRPlaybackState` 2 then 1 to both members; both displays
  darkened and relit, and R relit with its old image even though the
  resume re-pushed the artwork after state 1. (3) At 20:10:35–49 an
  artwork-only push, then off, then on with new template bytes
  (tide, 8847 B) riding the state-1 resume: R still showed the old
  image. Conclusion: once a follower's session is running its display
  is governed by the pair's internal sync, not its own session; only
  the session-start first light reaches it. Two fix iterations. First,
  a control-plane split: `groupTransport` routed metadata, artwork, and
  playback state to the leader only (the member matching the group
  row's host, first member as fallback). The routing shipped and the
  lamp pushes visibly went to L only (20:52:55 and 20:55:49 sessions),
  but R then stayed dark even at cold start — the pair's internal sync
  never delivers the leader's artwork on its own, so leader-only
  routing regressed the cold start and was reverted. That dead end
  surfaced the root cause: 361c78f (2026-11-10) had removed the
  SETPEERS announcement from PTP sessions as a timing no-op ("the
  receiver can take about 0.4 s to answer it"), which unseeded the
  member sync that carries Now Playing between the tops; the lamp
  shipped 2026-11-14, after that removal, so the follower never
  followed mid-session artwork. Fix as shipped: SETPEERS restored on a
  pair member's PTP session (gated on non-empty timing peers, keeping
  the standalone shortcut; the reply gates the stream SETUP on clock
  readiness and the round trip hides inside the PTP wait), and the
  control plane reverted to per-member fan-out. Same day, interleave
  observations with an iPhone
  pushing to the pair: transient pair-setup failed with TLV error 2
  (`Pairing PIN was not accepted`) across five backoff attempts
  20:38–20:50 after the previous app was killed mid-stream, then
  paired at 20:52:53 after an app restart; 3.7 s later the iPhone
  retook the pair and saundo logged `session taken over: receiver
  closed the session` (protocol.md §11 behavior). Whether the PIN
  refusals were the dead-session aftermath or the iPhone's active
  session is not resolved; both healed without device intervention.
  Live verification of the follower following the SETPEERS seed: the
  21:12:11 session answered `SETPEERS reply (200)` on both members and
  the user confirmed color switches then followed on both tops. The
  session also began from freshly power-cycled HomePods, so the
  attribution between the restored announcement and the clean device
  state is not isolated; the user keeps observing. Pins:
  `TestStereoGroupSessionCarriesGroupUUIDAndPTPTimeline`,
  `TestStandalonePTPSessionSkipsSetPeers`,
  `TestGroupArtworkFansOutToBothCapableMembers`,
  `TestGroupArtworkSkipsIncapableMember`.
- 2026-11-18 to 2026-11-21, covers, combos, and the observation rule:
  Apple Music's idle SMTC session publishes a thumbnail that is a white
  frame at alpha 63 over a transparent field; `usableArtwork` requires
  one pixel at alpha >= 0.5, the session reads as no cover, and the card
  draws its own placeholder (pins: `TestUsableArtworkRejectsAppleMusicIdlePlaceholder`
  and the other `TestUsableArtwork*` tests; fixture
  `internal/nowplaying/testdata/apple-music-idle.png`; the placeholder
  rule lives in ui.md). Two independent receivers link into one software
  stereo combo: a `combo:<hex>` registry row holds the members in
  playback order, the plan gives each member one input side with no
  group UUID and no SETPEERS peers, both members join the shared PTP
  follower set, link moves the members' launch flags onto the combo and
  unlink hands them back, and swap reorders and hot-applies the sides
  (pins: `TestComboPlanPlaysLeftAndRight`, `TestSwapComboSidesFlipsMembers`,
  `TestLinkComboMovesTheLaunchFlag`, `TestUnlinkComboReturnsTheLaunchFlag`,
  `TestComboLinkEndsAMembersAutoConnect`, `TestAutoTargetsSkipsLinkedMembers`,
  `TestComboVolumeReportConvergesTheMembers`, `TestLinkComboClearsAPendingPin`,
  `TestDelayRebuildRefusesAnOfflineMember`, `TestReceiverComboRowRendersTheChannelMap`,
  `TestFlyoutPairChannelButtonIsFixed`, `TestComboPickRowsExcludeTheOtherSide`,
  `TestStereoIconFollowsMembers`; live acceptance stays in `Currently
  pending`). An official stereo pair invalidates a manual combo over the
  same members: the combo reads 不可用, connect refuses with
  `ErrComboStale`, auto-connect skips it, and `Visible` drops it unless
  it is the live target (pins:
  `TestMergeInvalidatesAComboWhoseMemberJoinedAnOfficialPair`,
  `TestValidateTargetRefusesAStaleCombo`, `TestInvalidComboRowIsMarked`,
  `TestVisibleFiltersHiddenAndKeepsActive`). Grouping reads this
  session's advertisements only — the registry caches preferences and
  endpoints and never asserts a relationship — so a dissolved pair's
  combo heals within one discovery cycle and a stale remembered entry is
  cache hygiene (pin: `TestMergeLetsAStaleRememberedPairGo`;
  architecture.md §5). The discovery merge takes the tsid from the
  latest `_airplay` record and keeps `igl` monotonic, so a dissolved
  pair de-synthesizes within one cycle (pins:
  `TestMergeDropsThePairTsidWhenTheAdvertisementStopsCarryingIt`,
  `TestGroupFieldsParsedAndMerged`).
- 2026-11-14 to 11-16, the 0.6.3 release and the tray menu: the 250-image
  preview review fixed three P0s — the English latency segment truncated to
  "Low late…" → Low (`TestFlyoutLatencyLowFitsItsSegment`), the capture page
  copy, and the tray mute row's Unmute label
  (`TestTrayMenuMuteLabelFollowsThePreference`); a manufactured Windows
  per-app-mute-caveat test was deleted. Release run 43468834016's first
  `check-macos` failure was a test-only concurrent-map flake
  (`TestMuteGuardRecoversAfterCrash` racing `recoverMuteGuard`), fixed
  post-tag in `acd63dc`; all ten checksums passed; 0.6.3 published
  2026-11-13T17:23Z with both feeds serving signed manifests; the live
  0.6.2→0.6.3 update stays owed in `Currently pending`. The 2026-11-14
  tray row-toggle decision (`TestTrayRowActionTogglesTheSession`) was
  superseded 2026-11-16 by the quick-actions menu: flat rows connect or
  switch, the session action row carries 取消连接/断开连接/重试连接, the
  media group, the ambient submenu with previews and the 上次/✔ rules, the
  stateful update row, 更多接收器…, and the 重新搜索 empty state; the live
  right-click on both platforms stays under `Currently pending`. Pins:
  `TestTrayMenuShape`, `TestTrayMenuSessionRow`,
  `TestTrayMenuMediaRowsFollowSession`, `TestTrayMenuLampSubmenu`,
  `TestTrayMenuLampOffDisablesChoices`, `TestTrayMenuUpdateRow`,
  `TestTrayMenuMoreReceivers`, `TestTrayMenuEmptyState`,
  `TestMenuPreviewPNG`, `TestCoverPreviewColors`.
- 2026-11-15 to 11-16, stereo ambient first light: registry members persist
  `osVersion`, `CachedDevice` restores it per member, `offlineDevice` reuses
  the cached device, and sender log lines carry the member name (verified
  live 2026-11-15 on HomePod L/R software 27.3: both tops lit on a cold
  start). The one-dark-top race is a follower that does not assume playing:
  a pair member now sends an explicit state `1` at stream start before the
  first cover, and re-sends the first cover 2 s later (`coverRetry`,
  one-shot per session, standalone skipped). Live runs confirmed the order
  on both members and the user confirmed R lights on the first connect
  without tapping. The 2026-11-15 SETPEERS attribution did not survive: on
  a PTP session SETPEERS is a receiver no-op and cannot seed display sync;
  protocol.md §11 records the mechanism. Pins:
  `TestCachedDeviceGroupKeepsTopDisplayCapability`,
  `TestRememberedStereoPairKeepsTopDisplayCapability`,
  `TestSenderLogTagsTheReceiver`, `TestFirstCoverResendsOnceOnAPair`,
  `TestFirstCoverRetrySendsTheCurrentCover`,
  `TestFirstCoverRetrySkipsAStandalone`,
  `TestPairSessionSendsPlayingStateBeforeTheFirstCover`,
  `TestStandaloneSessionStillAssumesPlaying`.
- 2026-11-17, the 0.6.4 release run: the range review classified
  `v0.6.3..HEAD` (41 commits); the UI review's 254-frame matrix and the
  skill audit (changed) ran clean; the critical-path subset review landed
  after the tag as `12c6fdf` and `844e475` (the high-contrast notes-theme
  cache and the refuted SETPEERS comments; a history exception: `6735e3d`
  mixes the OSVersion cache fix with reply-label diagnostics, un-splittable
  after the tag; kept: the 2 s select duplication in `mute.go`/`silence.go`,
  the cover-retry diagnostics line, `Features.Persisted` as the store's
  shape; kept gap: RTSP replies log the method alone), the first tag
  `04fdd96` was re-cut as `117508a` on top of the fixes, and workflow
  44022602002 published 0.6.4 at 2026-11-17T05:21:56Z with every asset
  checksum verified and both feeds serving signed manifests. The mockup
  templates refreshed with 40 renders under `.audit/release-0.6.4/mockups/`.
  A live update from 0.6.3 and the 0.6.4 pre-release install remain owed.
- 2026-11-17, the multi-device engine: one `sessionPlan` and one cohort
  transport replace the single-receiver session and the stereo-group
  transport — one `StartGate` epoch and one capture trim per session, at
  most one PTP role per session. An 18-defect review shipped 12 fixes, each
  with a failing-before test (mixed AP1/AP2 pause, removing a live target,
  capture-rate change between arrivals, same-host routing, retry pruning,
  Now Playing replay, the error-bar scope). One `ptp.Followers` owns UDP
  319/320 and demultiplexes by source IPv4, so unpaired standalones share
  the follower set, and a stale remembered pair is skipped at launch. The
  live three-receiver session (two standalone HomePods + MacBook Air)
  streamed at 19:17 with the user confirming sync by ear at 200 ms — the
  sync rides the per-device latency settings, not the follower set alone.
  Per-target volume and channel mix per 352-frame block before ALAC; the
  channel stores in receivers.json, a pair is forced stereo, `VolumeByDevice`
  is per target, an unset target adopts `initialVolume`, and a knob report
  updates only its target. Pins:
  `TestFollowersDemuxBySourceIP`, `TestClockPlanRoles`,
  `TestClockPlanRejectsConflicts`, `TestAddSecondStandalonePTPReceiver`,
  `TestAutoConnectSkipsAStaleStereoPair`,
  `TestAutoConnectKeepsALiveStereoPair`; the live gaps (offset measurement,
  drift soak, the two-pair master run, a mid-session member drop, per-device
  volume and channel by ear) live in `Currently pending`.
- 2026-11-17 to 11-20, the session surfaces: the settings receiver row
  drops the per-device delay and volume at the user's request, then keeps
  the volume beside the channel segment (the engine keeps the delay data and
  `UpdateReceiver` for a later UI). The flyout device row is 66 DIP with a
  per-target mixer — slider, mute speaker (`mutedVolume`, session state
  only), and the channel button cycling stereo → left → right with the
  faded-side icon — and the playback card is not drawn without media or the
  latency row. The mixer exists only while a target streams; the trailing
  action column is one fixed 92 DIP; 断开连接 sends `DisconnectTarget` and
  rebuilds with the rest holding the mute; the head's 全部断开 keeps a
  reserved 26-DIP slot so the panel never resizes (base 438). The engine
  owns the join: `sessionPlan.joinKey` and `TargetControl.State` publish
  member/joining/reopening/failed per active key, one verb per state, a
  member's mixer survives a rebuild, and the dead-transport arm publishes
  after `closeSession`. `sessionMembers` reads the member set for the tray
  (every member accented and disabled), the connection card, the sidebar,
  and the slowest-member latency. A dead member leaves the cohort
  (`LostTarget`) and the survivors keep playing; the engine publishes a
  `SessionNotice` and the 44-DIP bar family (loss, join, error, credential)
  shares one geometry, +52 DIP while shown. A refused password prompts
  instead of burning the ladder, and the live pass confirmed it twice
  (2026-11-18 21:57/21:58). The connection page states the session
  (已连接设备 with notable counters, 本次连接 health grid) and the
  stability verdict reads the trailing 60 seconds
  (`internal/engine/stability.go`). Pins:
  `TestChannelIconFadesTheOtherSide`, `TestFlyoutVolumeButtonNamesItsAction`,
  `TestDisconnectTargetLeavesOthers`, `TestFlyoutConnectingRowHidesMixer`,
  `TestFlyoutDisconnectAllButtonFollowsSession`,
  `TestConnectingRowActions`, `TestDisconnectJoiningTargetBeforeStreaming`,
  `TestConnectPublishesJoiningThenMember`,
  `TestFailedAttemptPublishesRetryOrReopening`,
  `TestRowActionFollowsTargetState`, `TestDeviceHeadKeepsItsHeight`,
  `TestSessionMembersFollowActiveTargets`,
  `TestSessionLatencyUsesSlowestMember`, `TestTrayMenuMarksEveryMember`,
  `TestMemberDeathKeepsOthersPlaying`, `TestMemberDeathPublishesANotice`,
  `TestSessionNoticeAddsItsHeight`,
  `TestRejectedPasswordPromptsInsteadOfRetrying`,
  `TestTargetControlsCarryMeasuredHealth`, `TestConnectionPageDeviceList`,
  `TestConnectionPageSessionCard`,
  `TestDiagnosticsUnderrunsAgeOutOfTheWindow`; the live gaps (the notice in
  a live session, the reworked page on real receivers) live in
  `Currently pending`.
- 2026-11-23, the silent keepalive removes the pause crackle: the
  receiver's realtime path is a pull jitter buffer with PLC, and a stream
  suspended for more than a moment leaves the renderer unrecovered —
  measured on `AudioAccessory5,1` 27.3 with a software combo of two
  standalone HomePods: a 275 ms pause resumed clean, pauses of 3.5 s
  through 2 h resumed with persistent static until a reconnect, with 丢样 0
  and 0.0% retransmit. The sender now never suspends the stream: a fully
  starved packet carries the compact ALAC silence frame (protocol.md §6),
  the idle phase and the standby clock read `silence.LastLoud`, and the
  欠载 counter commits a short fill only when the source returns inside the
  window. Pins: `TestCompactSilenceDecodesToZeroFrames`,
  `TestStarvedPacketsCarryTheCompactSilenceFrame`,
  `TestSilentSourceKeepsTheStreamAndOnlyThePhaseIdles`,
  `TestLaunchIntoSilenceKeepsTheStream`,
  `TestDarkAnchorTracksSoundNotTheStream`, and
  `TestStatsCountUnderrunsAndBuffer`. The receiver-side decode is covered
  by the maintainer's 0.6.6 test (below); the zero run's `maxRun` caveat
  lives in protocol.md §6, and the device behavior in devices.md.

## 2026-11-21 — the launch deadline and the multi-homed cohort route

The 0.6.5 subset review's two Major risks are fixed. `maybeAutoConnect`
held the whole flagged launch set forever when one flagged row had no
address; the wait now runs on a 15-second deadline, past it the silent rows
drop with a line and the rest launch, and the dropped row keeps its flag
for the next app start (`3f88f23`; pin
`TestAutoConnectDropsASilentFlaggedReceiverAndLaunchesTheRest`). A
multi-homed receiver's UDP replies, arriving from an interface the plan did
not dial, dropped in a multi-member cohort; the per-member forward
goroutine now stamps every event with the member that owns the receiving
socket, so the reply reaches its own sender and the timing answer returns
over the dialed interface to the requesting port (`465fdc2`; pin
`TestCohortAttributesAUDPReplyToTheMemberThatOwnsTheSocket`). Owed live: a
receiver reachable on two interfaces in a two-receiver session, its timing
replies and retransmit requests confirmed on the streaming member — see
the multi-homed entry in Currently pending.

## 2026-11-21 — the verification skill's 0.6.5 maintenance run

The skill maintenance audited all six feature pages from source and drove
the live surface. Outcome: **changed** — the proven corrections landed as
5827621: session-controls still described the retired global volume row
and missed the session notice bar, the TargetState action verbs, and the
no-tint rule; tray-and-flyout measured three rows at 500 instead of 433
and read the tray accent as one device; ambient-light never said combo
members take the lamp sync and that a combo's missing group UUID skips the
first-light retry; discovery-and-connect missed the combo dialogs, the
multi-select launch and its 15 s drop deadline, and the socket-ownership
routing; updates cited five moved ranges and omitted the 本地构建 label.
Live pass (evidence `/tmp/saundo-verify/20261121-171029`): the doctor on
the running 0.6.4 instance found a live two-member HomePod L + R cohort
streaming (RTSP GET/FEEDBACK 200s flowing), the log excerpt and 286-frame
preview matrix are in the evidence dir, the headless UI suites passed, and
the passive mDNS browse passed. Unreachable this run, with the concrete
prerequisite: the receiver handshake drives (`SAUNDO_DEVICE_TEST=1`) —
the receivers are members of the maintainer's live session, and the e2e
UI harness shares the single-instance lock with the running app. Findings
recorded, not fixed here: no combo lamp test exists, and a stale comment
at `receivers_page.go:13-16` still names the per-receiver delay row.

## 2026-11-21 — the 0.6.5 candidate build and the maintainer's test

The candidate build (`go tool mygo build -platform windows/amd64
-skip-installer`, `MYGO_LDFLAGS=-X saundo/internal/app.Version=0.6.5`,
signed with `MYGO_UPDATER_PRIVATE_KEY`) produced
`cmd/saundo/build/windows-amd64/update-windows-amd64.json`: version 0.6.5,
the English 0.6.5 changelog section as its notes, six signatures, and
download URLs under `releases/download/v0.6.5/`; the update archive, three
deltas (0.6.2/0.6.3/0.6.4 → 0.6.5), and `SHA256SUMS` sit beside it. The
signing key's actual location is the Claude desktop container path
(`AppData/Local/Packages/Claude_…/LocalCache/Roaming/mygo/update-keys/`)
— where `mygo keygen` wrote it under that environment's APPDATA; the
documented `%APPDATA%\mygo\update-keys` path does not exist on this
machine, so `docs/releasing.md` still names the wrong default for this
setup. The maintainer announced a trial of the candidate on 2026-11-21
(tray-exit of 0.6.4, then the candidate binary from the install path) and
no defect came back; the release request of 2026-11-22 (正式发布) stands
as the maintainer's confirmation that the 0.6.5 build was tested on their
own PC, and it authorizes publishing. The rebuilt candidate at tip
fc03422 differs from the trialed one by one visual commit (9484271, the
latency segments' centering).

## 2026-11-21 — 0.6.5 commit range review

The release range v0.6.4..HEAD holds 49 commits, oldest 01729d5 (docs:
record the 0.6.4 draft and publish), tip fc03422 (site: the stereo stage
plays as a live pair). The two merge commits f17912b and bca6bd4 (PR #53)
carry commits listed below. Fifteen are internal; thirty-two are
user-visible and map to the changelog lines of the 0.6.5 section.

Internal, no changelog line: 01729d5 (the 0.6.4 release record), 1401d30
(multi-device docs), a8a2888 (settings schema 2 — an older build treats a
v2 file as read-only instead of dropping the persisted lastTargets field),
347cb3c (the underrun window's comment names its unit), 8039820 (the size
gate counts only code lines), 78b711f and 8423af0 (site-only, not app
behavior), 7a53163 (test pins for the combo edit path), 0f3ce44 (the
preview matrix gains a scrolled receivers state), 6eb59d4 (the ui-mockup
templates refreshed to the 0.6.5 interface), 5827621 (the verify feature
map refreshed to the 0.6.5 interface), e37400f (the release materials
staged), fa40ad0 (a device finding: a re-created pair member stalls stream
SETUP until a power cycle, recorded in devices.md and its dated entry),
42e5353 and fc03422 (site: the landing page's session model and stereo
stage).

Post-review fixes, user-visible: 3f88f23 (the launch deadline drops a
flagged receiver that never resolves an address — Fixed, the auto-connect
grace line) and 465fdc2 (cohort events route by the socket they arrived on
— Fixed, the multi-homed replies line). Both are the release subset
review's two Major findings, fixed before the release commit.

User-visible, by 0.6.5 changelog line:

- Added, multi-device playback: d664fa3 (cohort transport, per-target
  volume and channel, per-target flyout rows), 4269409 (primary-target
  concept retired, LastTargets), 2842972 (member set read by tray,
  connection page, sidebar, latency line), 7b36a83 (member death keeps the
  survivors playing), 9608099 (session notice bar), fe3242e (connection
  page states devices and health), 49623ea (surfaces drop the primary
  chip, sidebar counts members).
- Added, software stereo combos: 3d6d106 (combo registry entry, plan, hot
  side swap), 462a198 (create dialog, section, combo rows), af68096 (an
  official pair makes a combo Unavailable), 706c3ad (in-place edit, Seen
  separates advertisement from reachability).
- Added, model detail switch: e2ea676 (the switch and the
  parenthetical-hiding rule), b7f816e (the default flips to off).
- Changed, auto-connect multi-select: 3e36d80 (every flagged receiver
  connects as one cohort), 38ae9ea (a link moves the members' flags onto
  the combo).
- Changed, trailing-60-second stability verdict: 4f197e7.
- Changed, receiver row state: 20ec64c (title chips, second line with
  address, channel, model, version), 58eeef8 (rows name only what the
  title does not; model identifiers leave settings).
- Changed, per-section create actions on the receivers page: 2bc2baa.
- Changed, flyout head and rows: 4a9c7b0 (the head shows only the latency
  value), bc6939c (rows drop the selection tint).
- Fixed, phantom underruns: 9475c85.
- Fixed, the rejected-password prompt: 31e1c7e.
- Fixed, a dissolved pair de-synthesizes in one cycle: 7f99017.
- Fixed, Apple Music's idle thumbnail: f031863.
- Fixed, combo and audit corrections: 7fa04d5 (picker exclusion, tag
  placement, stereo channel row removed), 21bb7a8 (swap-side round trip,
  registry schema 2, pair member not linkable, offline add refused,
  takeover as member loss, combo delay and volume inheritance),
  de1fd19 (create entry waits for two eligible receivers, refused saves
  report on the page, notice resize, offline combo reads 离线, tray combo
  glyph), 19b9add (chips and mixer marks sit level with the text).
- Changed, the flyout's latency segments center their icon-and-label
  pairs: 9484271.

Code review: subset ran (15 commits) -> no P0. The two axes (Standards and
Spec) each reported zero P0 and zero documented-standard breaches. The Spec
axis recorded two Major behavioral risks, both fixed before the release
commit: the launch's indefinite wait for a silent flagged row, fixed in
3f88f23, and the cohort's dialed-IP-only event routing, fixed in 465fdc2.
The Standards axis left judgement calls only: the repeated auth-reject pair
in the sender, the duplicated password-prompt blocks in plan.go, and a
stale comment at cohort.go:619.

The 0.6.5 UI review ran the same day. The preview matrix (286 renders plus
the probe frames) rendered into `.audit/release-0.6.5/ui-review/` in zh-CN
and English, light and dark, and the copy gates passed
(`TestCatalogKeyParity`, `TestCatalogHasNoDeadKeys`,
`TestCatalogBannedTerms`, `TestCatalogUniqueValues`, `TestLatencyText`).
The reviewed states — the receivers page and its combo rows, the create and
unlink dialogs, the expanded row, the flyout's combo, notice, joining, and
latency-head states, and the connection page's session cards — match
docs/ui.md and docs/ui-copy.md; no P0 and no terminology finding. One
coverage gap: no preview showed the model-detail card, fixed by the scrolled
receivers state (0f3ce44).

## 2026-11-22 — 0.6.5 draft and publish

Workflow 44927966383 succeeded on the re-cut release commit (`cb183b4`):
`check`, `check-macos`, `draft`, `build`, and `build-macos` all green. The
draft carried fifteen assets — the Windows installer, both update
archives, both signed manifests, `install.ps1`, `install.sh`, the dmg,
`SHA256SUMS`, and the 0.6.2/0.6.3/0.6.4 deltas for both platforms — and
`sha256sum -c SHA256SUMS` passed every asset. Both manifests carry version
0.6.5, the English notes (including the latency-segment line), and six
signatures. The bilingual release notes were set from
`scripts/release-notes.sh`, and the release was published 2026-11-22 at
https://github.com/4202217/Saundo/releases/tag/v0.6.5. The public feed
serves `update-windows-amd64.json` and `update-darwin-arm64.json` at
version 0.6.5 with signatures; the installer and dmg return 200. The
repository answers at its renamed casing `4202217/Saundo`; the
`4202217/saundo` URLs redirect. Not run: a live update from 0.6.4 — it
needs an installed 0.6.4 build and stays in `Currently pending`.

## 2026-11-22 — 0.6.5 re-cut

The first `v0.6.5` tag (`f5a9f6b`) failed the workflow's `check` job and
was deleted locally and remotely before any release existed (draft and
build skipped). The failure was `TestGroupHeadsShareOneBaseline` on the
runner: the two section labels measured 123.7 and 123.6 — a 0.1 DIP
sub-DIP rounding difference that never appears on the developer's machine,
where the full gate ran green on the same commit. The test compared the
rendered label's Y with exact float equality; it now allows half a DIP,
which still pins the pin's intent (a stretched head row pushes the label
by whole DIPs). The release commit is re-cut on top of the fix; the
manifest and the changelog are unchanged.

## 2026-11-21 — a re-created pair member stalled stream SETUP until a power cycle

The maintainer reported connects slower than before. The log disagrees
with a sender regression and agrees with a wedged receiver: from
13:33:37, every connect to the 卧室 official pair waited 5.4–6.5 s for
HomePod L's (192.168.31.46) final stream SETUP reply — 13:33:38.706 →
44.967, 13:36:33.888 → 39.969, 13:49:48.401 → 54.379, 15:08:32.030 →
38.038, 15:37:33.129 → 39.225, 19:29:41.184 → 47.407, 19:30:08.497 →
14.253, 21:27:30.209 → 36.703 — for a total of ~8 s against ~1 s that
morning. Every earlier stage on the same control connection stayed
sub-100 ms: pairing, session SETUP, RECORD, SETPEERS, and the PTP wait all
answered promptly, and HomePod R answered its own stream SETUP in 43–129
ms inside the same stalled sessions. Combo and standalone connects on the
same minutes stayed fast (13:35:06, the combo, 0.03 s), so the network
path was sound. The slow reply and the member's event-channel
`POST /command` flush arrived in the same millisecond (21:27:36.703), the
signature of a receiver-side pipeline releasing both connections at once,
not of a lost segment.

The onset rules the sender out: binary 0.6.4-38-g7f99017 connected the
same pair in 0.03 s at 13:32:38 and stalled at 13:33:37, the plain 0.6.4
candidate stalled at 13:49, and a later dev build stalled again — the
stall survives every build while sitting on one receiver at one stage.
The onset also sits minutes after the pair was dissolved and re-created
in the Home app during the de-synthesize test (five `stereo:` UUIDs
accumulated in the registry that afternoon; the re-created pair is
`stereo:31932A59…`). The wire sequence is unchanged: the 08:33 fast
session and the slow ones run the same stages with the same 1188-byte
session SETUP.

A power cycle of HomePod L cleared it: 21:45:28.196 → 29.827 (1.63 s,
L's stream SETUP 178 ms) and 21:49:26.930 → 28.669 (1.74 s, 219 ms), both
the same 卧室 pair path with SETPEERS. Finding: an `AudioAccessory5,1`
27.3 pair member can wedge its stream-SETUP handling after Home-app
re-pairing, holding each connect to a fixed multi-second stall until the
speaker power cycles; the sender treats it as receiver state and carries
no workaround. Evidence: the maintainer's `saundo.log` and `saundo.log.1`
for 2026-11-21; the profile note lives in devices.md's HomePod stereo
pair section. No code change, so nothing is pinned.

## 2026-11-23 — 0.6.6 commit range review

The release range v0.6.5..HEAD holds 33 commits, oldest d1666d2 (site: the
hero flyout replica matches the 0.6.5 interface), tip 819226f (capture: the
darwin stop guard fits the file's line budget). The two merge commits
adfca01 (PR #55) and 65bd739 (PR #54) carry commits listed below. Twelve
are user-visible and map to the changelog lines of the 0.6.6 section;
nineteen are internal.

Internal, no changelog line: d1666d2, 6f0a21d, 31d2760, 63b43eb, and
1d44b3a (site-only, not app behavior), 6624dc5 and 9058256 (the 0.6.5
release record and the README previews), 524729a (mygo v0.10.0; the Go API
is unchanged, the single-instance and popup-fitting behavior rides library
code), 6c56c16 (the lifecycle refactor, behavior pinned by existing
tests), c4e5e79 and afa25e2 (the lifecycle model and its test sweep),
9096f42 (the architecture gate), d1ad62d (comment restatement plus the
disclosed SRP Wipe reshape), 750b32a and 9124959 (the capture restart race
and its macOS half; no reproducible user symptom), 819226f (the macOS guard
folded into the buffer loop to keep the file's line budget), a93eeb2 (a
malformed bplist key fails closed), 7fa18d5 (the non-string-key test
fixture), and f350dc9 (two stale comments).

User-visible, by 0.6.6 changelog line:

- Fixed, pauses no crackle, and the underrun rebase: 145a2dc.
- Fixed, full-scale clicks and the ring reset replay: d6e2d4d.
- Fixed, no password material to an unproven receiver, the CSeq reply
  matching, the feedback keep-alive, the event decrypt failure, the
  takeover classification, and the TCP data path: f33a553 and c3483ee.
- Fixed, PTP by transport, the drop-accounting rebase, and the per-app mute
  release: 30a5edc.
- Fixed, the credentials store on a locked Keychain and the password
  projection: b3ebca8 and 4cf7a47.
- Fixed, a lost member leaves the wanted set: d310b34.
- Fixed, the password as typed and the metadata sample: 8a0eb6e.
- Fixed, the stability label, the hidden row's click target, and the
  flyout device head gap: 4e4b433, 4362574, and 7bfb291.

Code review: subset ran (12 commits) -> fixed in 4cf7a47, 9124959
(line-budget follow-up 819226f), 7fa18d5, and f350dc9. Both axes returned
no P0. The Standards axis's hard
findings were the ALAC rule in AGENTS.md still demanding uncompressed
frames only (the rule now names the compact silence frame; fixed in the
release commit), `creds.HasPassword` discarding the last known map
(4cf7a47), and the capture race's missing test, which 750b32a documents as
needing live WASAPI timing — 9124959 extends the same guard to the macOS
tap, which had the gap in full. The Spec axis's confirmable findings were
that macOS half, a bplist fixture whose offsets pointed at the null marker
instead of the integer key it names (7fa18d5), two comments that
contradicted the code (f350dc9), and the changelog's bandwidth claim,
now scoped to a fully starved packet. Left as judgement calls: the
now-callerless transport `SetPaused`/`Pushing` surface, `pendingReq`
entries appended outside the `sendRequest` cap, a 4 MB slot overflow
killing the slot for the session, and `lastGood` being process-local.

## 2026-11-23 — the 0.6.6 UI review and mockup refresh

The range changes no catalog string (no `cmd/saundo/locales` diff), and
the copy gates pass in this release's acceptance gate (`TestCatalogKeyParity`,
`TestCatalogHasNoDeadKeys`, `TestCatalogBannedTerms`,
`TestCatalogUniqueValues`, `TestLatencyText`). The preview matrix rendered
290 files into `.audit/release-0.6.6/ui-review/` in zh-CN and English,
light and dark. The changed surfaces were inspected: the connection card's
stability label with its 60-second note at 720 and 640 DIP (English no
longer truncates either text), the hidden-receivers trigger (the pill's
90-DIP hit box is pinned by `TestHiddenGroupTriggerMatchesPill`; a static
render does not show a hit box), and the flyout device head (9.3 DIP to
its card, matching the ambient head). No P0, no terminology finding.

The ui-mockup templates were compared with the shipped surface
(`theme.go`, `ui_kit.go`, `flyout.go`, `settings_window.go`,
`receivers_page.go`). They were refreshed to the 0.6.5 interface in
6eb59d4, and 0.6.6 changes no token, widget size, or shell geometry they
describe: already current, no template change. The release matrix rendered
44 PNGs into `.audit/release-0.6.6/mockups/` — settings target, normal, and
edge; flyout idle, streaming, empty, error, media-off, and the cover,
template, and expanded lamp states; zh-CN and English, light and dark — and
every render was inspected for clipping, overflow, and wrapped copy.

## 2026-11-23 — the verification skill's 0.6.6 maintenance run

Outcome: **changed** — 9d5d971. The source audit was targeted at the
range's user-facing surfaces (the silent keepalive and the three geometry
fixes); the previous full six-page wave ran 2026-11-21. It found two
drifts: `features/README.md` still listed the retired silence suspend as a
cross-cutting state, and `ambient-light.md`'s standby recipe waited for the
engine's retired idle and resume log lines. Both were corrected, and
`verify-map` passes.

Live pass: attached read-only to the maintainer's running 0.6.5 instance
(doctor pid 25572, streaming to HomePod L and HomePod R, FEEDBACK and GET
replies flowing; evidence in `/tmp/saundo-verify/20261123-224510`). The
passive mDNS browse found HomePod L, HomePod R, MacBook Air, and the
Apple TV-class display row. Unreachable this run, with the concrete
prerequisite: the fresh-instance drives (the keepalive's pause and resume,
the flyout and settings geometry) need the single-instance lock, which the
running app owns; receiver handshake probes (`SAUNDO_DEVICE_TEST=1`) would
interrupt the maintainer's live session. The maintainer's own test of this
build covers the pause and resume (next section).

## 2026-11-23 — the 0.6.6 candidate build and the maintainer's test

The candidate build (`go tool mygo build -platform windows/amd64`,
`MYGO_LDFLAGS=-X saundo/internal/app.Version=0.6.6`, signed with
`MYGO_UPDATER_PRIVATE_KEY`) produced
`cmd/saundo/build/windows-amd64/update-windows-amd64.json`: version 0.6.6,
the English 0.6.6 changelog section as its notes, one signature, and
download URLs under `releases/download/v0.6.6/`. The update archive and
three deltas (0.6.3, 0.6.4, 0.6.5 → 0.6.6) sit beside it with their
signatures, plus `install.ps1` and `SHA256SUMS`. The signing key's actual
location on this machine is the Claude desktop container path
(`AppData/Local/Packages/Claude_pzs8sxrjxfjjc/LocalCache/Roaming/mygo/update-keys/`),
not the documented `%APPDATA%\mygo\update-keys`.

The release request of 2026-11-23 (0.6.6 ships mainly the urgent pause
crackle fix) stands as the maintainer's confirmation that this build was
tested on their own PC. It covers the pause and resume the silent-keepalive
batch left owed, including the HomePod's decode of the compact silence
frame (silence, no noise, no reconnect), which is why that batch graduated
above. The tag and the GitHub publish are held at the maintainer's
request: the release is prepared locally, no `v0.6.6` tag exists, and
nothing was pushed or published.

## 2026-11-24 — the 0.6.6 draft and publish

The maintainer authorized the release on 2026-11-24; the release commit
was written the day before while the tag was held, so its body still
records the hold. Before tagging, the maintainer's machine was running the
0.6.6 build from this tree (doctor pid 35456, started 08:36), and the
final gate reran green on `63de5f4`.

Workflow 45299615476 succeeded on `63de5f4`: `check`, `check-macos`,
`draft`, `build`, and `build-macos` all green. The tag `v0.6.6` points at
`build: release 0.6.6`, pushed to main with the commit. The draft carried
fifteen assets — the Windows installer, both update archives, both signed
manifests, `install.ps1`, `install.sh`, the dmg, `SHA256SUMS`, and the
0.6.3/0.6.4/0.6.5 deltas for both platforms — and
`sha256sum -c SHA256SUMS` passed every listed asset. Both manifests carry
version 0.6.6, the English 0.6.6 notes, and a main signature with signed
deltas. The bilingual release notes were set from
`scripts/release-notes.sh`, and the release was published 2026-11-24 at
https://github.com/4202217/saundo/releases/tag/v0.6.6. The public feed
serves `update-windows-amd64.json` and `update-darwin-arm64.json` at
version 0.6.6 with signatures; the installer and dmg return 200. Not run:
a live update from 0.6.5 to 0.6.6 — it needs the installed previous build
and stays in `Currently pending`.

## 2026-11-24 — a long silent stretch left a 27.3 stereo pair silent until a new session

The maintainer reported that after a long pause the HomePod pair stayed
silent while the app showed a live session. The failure was live at 22:45:
the source had been audible until 22:43:45 and the phase was dark, but the
report's state had followed a much longer idle earlier in the evening
(19:47–21:18, 91 minutes of source silence, with the stream up).

Measurements during the failure, in order:

- A loopback capture probe opened the same default render endpoint saundo
  uses and read the source audio (peak 28704) while the receiver stayed
  silent, so capture and encoding were not the fault.
- Both members answered every `GET` and `FEEDBACK` with `200 OK`; the
  session, PTP, and the event channel stayed up.
- A volume nudge did nothing. A state 2→1 cycle did nothing. A manual
  reconnect restored both members immediately, and stayed working.
- A manual 关灯 (state 2) for one minute while music kept playing, then
  开灯 (state 1), did not disturb the audio. `updateMRPlaybackState` is not
  the trigger.
- A two-minute pause did not reproduce it; the maintainer reported that two
  minutes is not a long pause, and no shorter stable repro exists.
- The compact silence frame is not the suspect: the 0.6.6 pause/resume run
  decoded it as silence with no noise and no reconnect.

Conclusion: a HomePod pair at software 27.3 stops rendering the realtime
stream after a long silent stretch, regardless of the continuing RTP
keepalive. State 1, volume, and artwork do not revive it; only a new session
does. The display and the standby setting are unrelated to the audio.

Fix: the engine rebuilds the cohort on the first audible block when the
captured source was silent for at least `longIdleRebuildAfter` (ten
minutes); a shorter pause keeps the live stream and its seamless resume. The
rebuild reuses `rebuildSession`, holds the local mute, and re-applies the
light through the new session's launch. Pins:
`TestLongIdleRebuildsTheSessionOnResume`,
`TestShortIdleKeepsTheLiveStream`,
`TestLongIdleRebuildSkipsAnUnaddressablePlan`,
`TestLongIdleRebuildSkipsANonStreamingTransport`.

Not run: a live long-idle resume with the rebuild (needs a pause longer than
ten minutes and a listen) and the 30-minute default standby soak. Both stay
in `Currently pending`.

## 2026-11-25 — a combo streamed twice the audio rate of two standalone HomePods

The maintainer measured about 360 KB/s with HomePod L and R connected
standalone and about 720 KB/s with the same two speakers combined into a
stereo combo. An uncompressed ALAC packet is 352 stereo frames, about
1.4 KB, and the pacer sends one every 8 ms: one copy per member is about
181 KB/s, so 360 KB/s is the two-member single-copy rate and 720 KB/s is the
same with one extra copy per member.

The combo target is a synthesized registry key. It had no
`lastSentBlocks`/`lastRetransmits`, because `rememberSessionStats` ran only
on a receiver-initiated close or a takeover; a user disconnect, an app quit,
and every rebuild skipped it. `DuplicatesFor` therefore read a zero entry
and returned the first-session extra copy for both members forever. The
plan also took the count from the synthesized target entry rather than from
the members' own entries, so the standalone histories never helped.

Fix: `closeSession` records the session's ratio before the transport goes
away (it covers user disconnect, quit, and every rebuild), reading the live
transport instead of the last publish; each pair or combo member's own
counters are written into its own entry as well as the folded target entry;
and each member's copy count is sized from its own entry, falling back to
the target entry when the member has none. A clean combo now sends one copy
per member (~360 KB/s total); only a member that actually retransmitted pays
the extra copy. Pins: `TestUserDisconnectRemembersTheSessionStats`,
`TestComboCloseRemembersTheSessionStats`,
`TestComboPlanSizesCopiesPerMember`,
`TestOnClosedRemembersRetransmitRatio`,
`TestRememberSessionStatsPerTarget`.

Not run: a live bandwidth re-measure after the fix (the running test build
predates it; the 360 KB/s expectation is arithmetic from the packet layout
and the pinned duplicate choice).

## 2026-11-25 — the long-idle rebuild threshold moves to one hour

The maintainer set `longIdleRebuildAfter` to one hour after the ten-minute
value landed. The only confirmed failing idle is the 91-minute stretch of
2026-11-24, so an hour stays inside that evidence while keeping ordinary
pauses on the live stream; the opposite risk is a receiver that gives up
sooner than an hour — the 25-minute gap that evening is not confirmed either
way — which still needs a manual reconnect. The 30-second working value used
to exercise the rebuild path is replaced by the production value in the same
change. A live long-idle run at one hour stays in `Currently pending`.

## 2026-11-25 — the threshold settles at 30 minutes after a 59-minute crackle

After the one-hour value landed, a pause of roughly 59 measured minutes
(eight minutes short of the wall-clock impression) resumed with PLC static
on the old stream. The rebuild did not run: the log had no `long idle` line
and no `session stopped`, so the gap was just under the threshold and the
engine took the silent short-gap branch. The receiver was already bad at
that point, which puts its limit below an hour. `longIdleRebuildAfter` is
now 30 minutes. A diagnostic line records every resume after at least five
minutes of source silence with the measured gap and the threshold, so the
next miss shows the number instead of the wall-clock estimate. Thirty
minutes remains a middle ground: the 5-minute resume of 2026-11-10 was
clean, the 59-minute one was not, and the exact limit is still unmeasured.

## 2026-11-25 — the idle suspension replaces the compact keepalive

A working-tree experiment suspended the RTP pacer after five seconds of
source silence and resumed on the first audible block. On the
`AudioAccessory5,1` 27.3 pair, pauses of 48 s, 55 s, and 3 m 25 s resumed
clean with the source test tone and the display active. This contradicts the
2026-11-23 entry, which measured persistent static after a suspension of
3.5 s or more; the static did not reproduce with this build and receiver, so
the compact keepalive is no longer the idle policy.

The idle path now: suspends the pacer and pauses the silence meter after 5 s
of source silence; resumes with a re-anchored RTP timeline on the first
sound; keeps the continuous silence stream for a receiver that cannot
suspend (AirPlay 1); suspends a session launched into silence immediately;
and rebuilds the cohort on the first sound after 30 minutes of suspension
(the 2026-11-24 long-idle wedge). The compact frame still fills a packet
that starves while the pacer is running, such as a capture hiccup. Pins:
`TestSilentSourceSuspendsTheStreamAndThePhaseIdles`,
`TestLaunchIntoSilenceSuspendsTheStream`,
`TestStandbyDarkensAtThresholdAndResumesOnSound`,
`TestDarkAnchorTracksSoundNotTheStream`,
`TestLongIdleRebuildsTheSessionOnResume` and its guards.

Not run: a 15–20 minute suspended run, an AP2 receiver outside the HomePod
family under the suspension (the MacBook Air is on the network), and the
live 30-minute long-idle rebuild. All stay in `Currently pending`.

## 2026-11-25 — the resume FLUSH clears the stop-send crackle

The stop-send design suspends the pacer on silence and re-anchors the RTP
time on resume. A 43-minute session with 14 pause/resume cycles developed
persistent static, volume-correlated, with `0.0%` retransmits and no
underrun; the static appeared after one resume and disappeared at another, so
a receiver-side jitter buffer or PLC state was being disturbed by the
timestamp jump with a continuous sequence. The sender now sends `FLUSH` with
the next `seq` and `rtptime` before the first resumed packet.

On the `AudioAccessory5,1` 27.3 pair both members answered `200 OK` on every
flush. Suspensions of 7 m 3 s, 6 m 35 s, and 18 m 10 s resumed clean, with
the display state 1 and cover on return; the maintainer heard no crackle in
the run. The 30-minute rebuild remains for the long-idle wedge. Pins:
`TestPauseStopsAudioKeepsKeepAliveAndResumesAnchored` (the request and its
RTP-Info), the fake receiver's `FLUSH` reply, and the existing suspension
tests. Not run: a resume after a 30-minute-plus suspension, and an AP2 Mac
receiver under the flush; both stay in `Currently pending`.

## 2026-11-26 — the Apple Music progress clock no longer bounces at the start

On macOS 27.3 the Now Playing card's clocks flipped between `0:00` and
`-0:01` in the first seconds of an Apple Music track. Live captures of the
vendored adapter (`stream --no-diff --micros --debounce=200`) show why:
`elapsedTimeMicros` is the position at `timestampEpochMicros` and stays frozen
between MediaRemote notifications (one 12.36 s playback window left both values
unchanged, then advanced both by 12.361 s together). The app polls every two
seconds, and the old card extrapolated from the poll's arrival time, so each
fresh poll rewound the displayed position to the stale elapsed value. A second
capture at the start of playback caught `playing=true` arriving first with the
previous paused session's stamp and `playbackRate=0`, then the real `rate=1`
payload about 0.25 s later — the same stale stamp problem at the worst moment.

The card now advances from the payload's playback timestamp (`PositionAt`,
set from `timestampEpochMicros` on macOS; Windows leaves it zero because SMTC
has no sampling instant) and only when the adapter reports a positive
`playbackRate` (carried through as `Advancing`); extrapolation is capped at
30 s, so a stale stamp cannot jump the clock forward.
The artwork and transport controls are unchanged. Pins:
`TestProgressPositionAnchorsToPayloadTimestamp`,
`TestProgressPositionSurvivesStalePolls`,
`TestProgressPositionBoundsStaleStamp`,
`TestProgressPositionHoldsWhenRateIsZero`,
`TestParseStreamLineMapsTheSession` (the rate=1 fixture marks `Advancing`),
`TestProgressPositionAcceptsSeekAndNewerStamp`, and
`TestProgressRowDoesNotFlipRemaining` (the old logic rendered `0:03` / `-0:00`
where the fixed one holds `0:02` / `-0:01`).

On 2026-11-26 the maintainer ran the fixed build on macOS 27.3 with Apple
Music and confirmed the card no longer bounces at the start of a track. The
live visual check is done; no receiver was involved.
