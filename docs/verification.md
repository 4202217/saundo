<!-- reviewed-through: 2026-11-17 -->

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
- Local output mute across a live latency switch: a 2026-11-13 live HomePods
  run logged the mute once and then four in-place stream rebuilds with no
  restore between them, while earlier lines in the same log show a restore
  after each session end. The log names no control per rebuild, so audible
  confirmation stays the user's. The engine path is pinned by
  `TestLatencyRestartKeepsLocalMute`, `TestLatencyRestartKeepsProcessMute`,
  and `TestReceiverDelayRestartKeepsLocalMute`.
- Local output mute across a receiver switch, and its restore when the switch
  fails: no live run yet. The engine path is pinned by
  `TestDeviceSwitchKeepsLocalMute`, `TestDeviceSwitchKeepsProcessMute`,
  `TestDeviceSwitchRestoresMuteWhenTheNewSessionFails`, and
  `TestDeviceSwitchRestoresMuteWhenTheLaunchFails`; the row action is pinned
  by `TestRowActionKeepsOtherReceiversConnectable`. A live run should switch
  between two receivers while streaming, check the log shows no restore
  until the disconnect, then switch to an unreachable receiver and confirm
  the endpoint comes back.
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

## 2026-11-14 — release 0.6.3 UI review

The 250-image preview matrix (`.audit/release-0.6.3/previews`, zh-CN and
English, light and dark) was rendered, fixed, and re-rendered. P0 fixes: the
English flyout latency segment truncated to "Low late…", now the single word
Low (`TestFlyoutLatencyLowFitsItsSegment` fails on the old label); the
capture page said "a single app" where macOS ships multi-select, now
"System audio, or one or more apps."; the tray mute row now reads Unmute
local output while the preference is on
(`TestTrayMenuMuteLabelFollowsThePreference`). English per-app copy is
plural-neutral. The Windows per-app mute caveat and its Windows-only test
were removed: the engine drops an unsupported app selection at load, so the
state could not occur and the test manufactured it. Both mockup templates
already described the shipped interface: the flyout height rule is 439
base, -62 for the removed Now Playing row, +90 ambient, +150 media, +44
latency, +48 error, +48 credential, +53 expanded, with latency labels Low,
Standard, Stable, Custom; 44 renders in `.audit/release-0.6.3/mockups/`.
Two independent reviewers on different model families checked the findings
and the fixes. The verification skill corrected all six feature pages from
source, in their own commit before the release commit. The release
candidate was built with the updater key and launched against the HomePods
pair; the live flyout capture is
`.audit/release-0.6.3/live/flyout-0.6.3.png`.

## 2026-11-14 — 0.6.3 release

The release workflow (run 43468834016) first failed `check-macos` on a
test-only `fatal error: concurrent map writes` in
`TestMuteGuardRecoversAfterCrash`: the harness's mute worker raced the
test's direct `recoverMuteGuard` call. The job passed on re-run,
`build-macos` passed, and the flake fix landed as `acd63dc` on main after
the tag (`2f14bc5`). The draft carried `install.ps1`, `install.sh`, the
Windows installer, both update archives, four deltas, both signed
manifests, the arm64 dmg, and `SHA256SUMS`; `sha256sum -c` passed all ten
checksummed files. Published 2026-11-13T17:23Z with bilingual notes; both
feed manifests serve 0.6.3 with signatures. A live update from 0.6.2 is
still owed and stays on the pending list.

## 2026-11-14 — tray receiver rows toggle their session

- **Decision.** A tray receiver row now toggles the session it carries. The
  row whose ID is `Snapshot.SelectedID` while the phase is streaming
  disconnects; while connecting it cancels the attempt, matching the flyout's
  active-row action; a failed row still retries. Every other row connects or
  switches, as before. The decision is `trayRowAction`, which reuses the
  flyout's `rowActionKind` vocabulary, and `trayDeviceClick` dispatches it.
  No separate 断开连接 row is added.
- **Not run live.** The click path needs a real receiver and a live session,
  which would stream this Mac's desktop audio to the HomePods pair; the last
  live tray run (2026-11-13) stands for the menu shape.
- **Pins.** `TestTrayRowActionTogglesTheSession`.
- **Superseded 2026-11-16.** Receiver rows connect or switch only; the
  session action row carries the cancel, disconnect, and retry actions.

## 2026-11-16 — tray menu around quick actions

- **Decision.** This supersedes the 2026-11-13 flat menu and the 2026-11-14
  row-toggle decision. The menu keeps one flat row per visible receiver, up to
  six, and every row connects or switches; the in-session row is disabled and the
  session action row below the list carries 取消连接, 断开连接, or 重试连接.
  The media group follows when the system media session has controls and
  `PlaybackControls` is on: 上一首, 暂停 or 播放, and 下一首; the menu shows no
track title.
  The ambient light is one submenu row: the power
  action, the 封面 choice, and the six template choices, each with a round
  color preview. The ambient choices are disabled while the light is off; the
  power row turns the light on, then a choice can be picked, matching the
  flyout and settings. The stored choice carries 上次 while off; the current
  choice is disabled while lit and its label ends in a single-color ✔. The update row is stateful (下载更新, 正在下载, 重启并安装);
  更多接收器… opens the receivers page past six; with no visible receiver,
  重新搜索 with 暂无接收器 sits first. No row uses a check column; the current ambient choice is
  disabled and greyed while lit.
  While the light is on, the parent row's icon is the current preview; off uses
  the lamp-off glyph.
- **Evidence.** The mockup at `.audit/mockups/tray-menu/mockup.html` established the layout;
  the final live menu drops the track title and uses the current preview as
  the ambient parent icon. The menu is cached and rebuilt on every right
  click so media, lamp, and update rows are current.
- **Not run live.** The native menu visuals still need a live right-click on
  Windows and macOS; the unit tests pin the model, item order, labels, the current
  suffix, and preview PNGs.
- **Pins.** `TestTrayMenuShape`, `TestTrayMenuSessionRow`,
  `TestTrayMenuMediaRowsFollowSession`, `TestTrayMenuMediaControlSendsAction`,
  `TestTrayMenuLampSubmenu`, `TestTrayMenuLampOffDisablesChoices`,
  `TestTrayMenuUpdateRow`, `TestTrayMenuMoreReceivers`,
  `TestTrayMenuEmptyState`, `TestTrayMenuLampPresetOrder`,
  `TestTrayMenuDeviceSubtitle`, `TestTrayMenuKey`, `TestMenuPreviewPNG`, and
  `TestCoverPreviewColors`.

## 2026-11-15 — cold-start stereo ambient light

A cold start auto-connects a remembered stereo pair from the receiver
registry before mDNS answers. The cached `Member` entries did not persist
`OSVersion`, and `offlineDevice` dropped the member list entirely, so the
`TopDisplayCapable` gate failed closed and the sender skipped both members
(`ambient light: ... does not support top-display artwork; skipped`). The
media cover path was not gated, so a pair could still light one top from the
cover while the template never reached either. Whether the other top stayed
dark because the cover missed it or because its renderer was paused was not
isolated; a later reconnect uses the discovered record with `osVersion` and
restores the template path.

`receivers.Member` and `receivers.Entry` now persist `osVersion`;
`CachedDevice` restores it per member and falls back to the entry's value;
`offlineDevice` reuses the cached device so the registry projection and the
cold-start connect describe the same pair. Sender log lines now carry the
member name, and each RTSP reply names its method, content type, and byte
count, so the two members are distinguishable. `SetPlaybackState` logs when
a state is assumed rather than sent.

Verified live on 2026-11-15 with HomePod L/R (`AudioAccessory5,1`, software
27.3). Before the fix both members were skipped at launch. After it, a cold
start logged `cover: 11108 bytes, lamp=true` for each member, each replied
`200 OK` to `SET_PARAMETER image/jpeg (11108 B)`, and both tops lit. The
state-1 launch assumption is unchanged; the intermittent single-dark
occurrence on the media-cover path was not reproduced after the fix and
remains open. Pins: `TestCachedDeviceGroupKeepsTopDisplayCapability`,
`TestCachedDeviceGroupFallsBackToTheEntryOSVersion`,
`TestMergeKeepsCachedGroupMembers`,
`TestRememberedStereoPairKeepsTopDisplayCapability`,
`TestSenderLogTagsTheReceiver`, `TestPendingRequestLabelNamesTheArtwork`.

## 2026-11-16 — stereo first light is re-sent after the pair sync

A first connect to the stereo pair occasionally left one HomePod's top
display on its white placeholder until a manual disconnect and reconnect.
The 18:44:25 first light was delivered to both members and both answered
`200 OK` to `SET_PARAMETER image/jpeg (11108 B)`, so the RTSP layer did not
drop it. The follower's top follows the pair's HomeKit/tightSync
media-system sync, not its own session's artwork stream; the session's
first light is the one image it takes from its own session, and it is sent
at the earliest instant the session streams. When the sync has not yet
established the Now Playing item, the image is acknowledged without
reaching the LED renderer, and artwork-only pushes later in the same
session do not recover it (2026-11-15 experiments below). A new session
does, which is why the reconnect worked.

The 2026-11-15 SETPEERS attribution does not survive later evidence: a
PTP session makes SETPEERS a no-op on the receiver, so it cannot seed the
display sync. The 2026-11-15 live success came from the freshly
power-cycled pair, as the commit body already noted.

Fix as shipped: a pair member re-sends the session's first effective cover
2 s after the first cover send (`internal/airplay/sender` `coverRetry`),
which gives the follower's media-system sync time to attach the item. The
retry is one-shot per session, follows a template change made inside the
window, and is skipped on a standalone receiver. Pins:
`TestFirstCoverResendsOnceOnAPair`,
`TestFirstCoverRetrySendsTheCurrentCover`,
`TestFirstCoverRetrySkipsAStandalone`,
`TestFirstCoverRetryStopsWithTheSession`.

Live sender run 2026-11-16 20:03 on the pair: the first light at
20:03:25.403 and the retry at 20:03:27.403 each answered `200 OK` on both
members; the real track metadata arrived at 20:03:26.936, inside the
window, the same race as the 18:44 failure. The LED observation is the
user's and stays open. The 2 s delay is the first attempt; if a follower
still stays white, the next step is to gate the first light on each
member's first event-channel `updateInfo` instead of a fixed delay.

Follow-up 2026-11-16 20:14: the retry did not change the symptom because
the follower was dark, not missing the retry. On a cold start R stayed
off while L showed the template; tapping R's glass woke it with the
template colors, then it turned white, and later template changes did not
follow. The sender assumed playing and sent no state `1` on either
member (`playback state: 1 unchanged, not sent`). A pair follower does
not assume playing, so a pair member now sends an explicit state `1` at
stream start, before the first cover (`sender.Start`; `beginPlayback`
sends the state before the metadata and cover). A standalone receiver
keeps the image-only first light. Pins:
`TestPairSessionSendsPlayingStateBeforeTheFirstCover`,
`TestPairSessionKeepsAPreStartPause`,
`TestStandaloneSessionStillAssumesPlaying`. Live sender run 2026-11-16
20:30 on the pair: `playback state: 1` precedes the cover on both
members, both answered `200 OK` to the state command and the image, and
the 2 s retry followed. The user confirmed R lights on the first connect
without tapping the glass.

## 2026-11-17 — 0.6.4 UI mockup templates

The two ui-mockup templates were compared with the shipped interface and
refreshed. The flyout template's media card head now carries the 18-DIP app
icon and the 20-DIP session switch on the 24-DIP top-aligned row, the pill
pads 10 DIP on top, and the main row keeps its 2-DIP top margin; the height
notes name the 62-DIP Now Playing row and the 48-DIP error bar. The settings
template's General sample carries all five shipped rows, including tray
animation and appearance, in both languages. The palette and the height rule
already matched `theme.go` and `flyoutHeightFor`. Renders: 40 states under
`.audit/release-0.6.4/mockups/` (settings target/normal/edge and flyout
idle/streaming/empty/error in zh-CN and English, light and dark, plus the
media-off and ambient template/expanded states); every render was inspected
for clipping, overflow, and wrapped copy, and none was found.

## 2026-11-17 — 0.6.4 commit range review

Range `v0.6.3..HEAD`: 41 commits, oldest `0672dd1`, tip `ecaf44e`.
Classified from the diff, not the subject alone. A user-visible commit names
the `CHANGELOG.md` 0.6.4 line that describes it. A merge commit has no
content of its own, so the commits it brings in are classified here.

- `ecaf44e` app: media card head at an 18-DIP icon and a 20-DIP switch —
  user-visible; "The media card's head places its app icon, name, and
  session switch on one top-aligned line..."
- `3c0d7cc` merge PR #50 — merge; its commits are below.
- `a16d5fc` docs: mygo guides in the code map — internal.
- `75b47bf` docs: release range review step — internal.
- `4c12210` app: top-align the media card head — user-visible; the same
  media-card-head line as `ecaf44e`.
- `8a0f6bc` merge PR #49 `app/tray-menu` — merge; its commits are below.
- `8f5079a` merge PR #48 `fix/ambient-ring` — merge; its commits are below.
- `bd42208` app: tray menu around quick actions — user-visible; "Media
  controls in the tray menu..." and "The tray menu is rebuilt around the
  session..."
- `42ab568` landing: no vignette on the light pod — internal (site only).
- `cc89459` docs: outside comparison sources dropped — internal (docs and
  site copy).
- `b62c4ac` docs: settings e2e driving block — internal.
- `215261b` merge PR #47 — merge.
- `b4429b6` docs: e2e wording platform-neutral — internal.
- `bd7a68d` merge PR #46 — merge.
- `108a482` docs: e2e UI driving on macOS — internal.
- `d4ffc31` sender: stereo follower before its first light — user-visible;
  "A cold start to a stereo pair lights both top displays..."
- `34d9dca` verify: mygo automation plugin — internal.
- `e0eaf3c` build: mygo v0.9.1 — internal (the documented render diff is
  subpixel; the bump is a prerequisite for the render plugin).
- `7337398` merge: mygo audit app fixes — merge.
- `03a98a0` merge: landing glow lattice — merge.
- `6c1c580` app: desktop text scale and mygo identities — user-visible;
  "The interface follows the desktop text scale..."
- `a26a8e4` build: `time.Sleep` ban in sans-I/O — internal.
- `a9df430` build: dependency allowlist — internal.
- `aad9751` build: channel gate across files — internal.
- `2099d6a` docs: skills point at the Makefile — internal.
- `b142fb6` build: release runbook in step with the gates — internal.
- `d73ea15` build: architecture rules as gates — internal (test-only
  refactors in engine and receivers).
- `bee1aaa` landing: glow noise lattice floor — internal (site only).
- `47d4103` sender: SETPEERS seeds the pair member sync — user-visible;
  "Ambient light changes made during playback reach the second HomePod's
  top display again..."
- `d285983` build: rename landing to site — internal (structure).
- `f4d9ae6` build: landing deploy to Pages — internal (site only).
- `91b39eb` landing: modernize page, glow renderer — internal (site only).
- `de0f660` landing: bilingual landing page — internal (site only).
- `57aa077` engine: a user connect restarts the reconnect budget —
  user-visible; "Retry after the automatic reconnect attempts are used up
  starts a fresh backoff..."
- `6735e3d` receivers: stereo member OS version for the cold-start light —
  user-visible; the same cold-start line as `d4ffc31`.
- `28bce7f` docs: tray-toggle feature in the verified docs — internal.
- `a1ae9d4` docs: verification log compressed — internal.
- `271b576` docs: every doc checked against 0.6.3 — internal.
- `e059453` app: tray receiver rows toggle on click — user-visible in
  principle, superseded inside this range by `bd42208`; the shipped
  behavior is the "The tray menu is rebuilt around the session..." line.
- `6737a5b` docs: 0.6.3 draft and publish recorded — internal.
- `0672dd1` engine: mute worker stopped in the guard tests — internal
  (test).

Code review: subset ran (7 commits) -> fixed in `12c6fdf` and `844e475`;
the result entry is below.

## 2026-11-17 — release 0.6.4 UI review

The copy inventory (`cmd/saundo/locales/en.json`, `zh-CN.json`) and the
rendered preview matrix (`.audit/release-0.6.4/previews/`, 254 PNGs across
both locales, light and dark, every settings page, the dialogs, the flyout
states, and 1.25x text) were reviewed against `docs/ui-copy.md` and
`docs/ui.md` before the changelogs. The new tray strings follow the
terminology table (接收器, 托盘面板, 取消连接 / 断开连接 / 重试连接,
关闭氛围灯 / 开启氛围灯, 上次 / Last used, single-color ✔); the catalog
gates are green and no string truncates. The media card head was checked at
both locales and themes and at 1.25x text with no clipping. Outcome: clean —
no P0 or terminology finding, so no product fix commit.

## 2026-11-17 — 0.6.4 verification skill audit

All six feature pages were read against source (one reader per page) and
`go run ./scripts/verify-map` resolves every name and path. Outcome:
changed — the proven corrections landed as their own commit, `verify: the
feature map matches the 0.6.4 tree`: the tray media rows (no track title),
row order past six receivers, the ambient submenu, the reconnect budget reset
on a user connect, the tray as a second connect entry point, text-scale
window sizing and the page-switch scroll, the update surface's removed tray
check action, and the citations that moved with the recent app changes.
The live pass did not run: the release instance (pid 6532) is up and the e2e
build shares the single-instance lock, so stopping the user's instance needs
consent. The doctor passed read-only. Coverage gap: live driving of the six
pages (tracked under Currently pending). No product defect was found.

## 2026-11-17 — 0.6.4 release

- Version `0.6.4`; the release commit is `build: release 0.6.4` and the tag
  `v0.6.4` is created locally. At the maintainer's request this release is
  not pushed: no workflow run, draft, publish, or public-feed check happened
  in this run, so the draft and feed steps remain for the push.
- Docs audit: `architecture.md`, `implementation.md`, `platforms.md`,
  `protocol.md`, `devices.md`, `ui.md`, and `ui-copy.md` were reviewed
  against the tree and corrected where the code and the documents disagreed:
  the runtime roles and socket owner, the AP1 raw-PCM scope, the PTP
  fail-closed gate, SETPEERS on a pair member's PTP session, the event
  channel's direction, the AP2 request headers, the flyout heights and tray
  ambient row, and the check summaries in `AGENTS.md` and `platforms.md`.
  Reviewed-through dates bumped to 2026-11-17.
- Gate: `mingw32-make check` green — gofmt, 364 catalog messages, `go vet`
  under the default, `updateharness`, and `e2e` tags, `mygo vet`, check-arch
  "architecture seams hold", check-size "120 files within their line budget",
  all package tests, verify map "map ok", and check-platforms on
  windows/amd64 and darwin/arm64.
- Candidate: `go tool mygo build -platform windows/amd64` produced
  `saundo-setup-0.6.4-amd64.exe`, `saundo-0.6.4-windows-amd64.tar.gz`, deltas
  from 0.6.1, 0.6.2, and 0.6.3, and `update-windows-amd64.json` carrying
  version 0.6.4, the English 0.6.4 notes, and an 88-character signature. The
  candidate was not installed: the running instance holds the
  single-instance lock.
- Maintainer's pre-release test: the release request (2026-11-17) authorized
  the release and carried no separate hands-on statement for this build, so
  none is recorded. A pre-push install and smoke run remains owed.

## 2026-11-17 — 0.6.4 critical-path code review (post-tag)

The critical-path code review rule landed after `v0.6.4` was tagged. The
0.6.4 range touches `internal/airplay/sender`, `internal/engine`, and
`internal/app/update*.go`, so the subset review is owed before the local tag
is pushed. Code review: not run -> the rule did not exist when the release
commit was cut. The next release's range review carries the line.

## 2026-11-17 — 0.6.4 critical-path code review result

`/code-review` ran on `v0.6.3...v0.6.4`, restricted to `internal/airplay`,
`internal/engine`, `internal/capture`, `internal/app/update*.go`, and
`internal/app/migrate.go`: 7 commits, 25 files. Code review: subset ran
(7 commits) -> fixed in `12c6fdf` and `844e475`.

Standards findings. Hard: the sender comment and two test comments still
taught the refuted SETPEERS display-sync mechanism that `d4ffc31` corrected
in the docs (fixed in `12c6fdf`); commit `6735e3d` mixes the OSVersion cache
fix with reply-label diagnostics, which cannot be split after the tag and is
recorded as a history exception. Judgement: the update notes dialog cached
its Markdown theme on the appearance only, so high contrast left stale
hairlines, muted copy, and headings (fixed in `844e475`); the 2 s
stop-with-timeout select is duplicated in `mute.go` and `silence.go` (kept:
two call sites); `onCoverRetry` logs `cover: first-light retry` even when
`sendCover` skipped an empty image (kept: the retry is armed only after a
cover and the line is diagnostics); `Features.Persisted` travels as a pair
into the receiver cache (kept: that pair is the store's shape).

Spec findings. The high-contrast notes theme (c1) and the SETPEERS comments
(c2) are the fixes above. Partial: `6735e3d` said every RTSP reply carries
the method, content type, and byte count, but only `sendRequest` labels the
request, so AP2 and pairing replies log the method alone (kept as a
diagnostics gap; extend the label at the next attribution run). Scope:
the notes-theme cache was not named in a commit body and now carries
`844e475`.

`mingw32-make check` is green after the fixes (`check-size` 120 files,
`app.go` baseline lowered to 1259). The candidate build was not rebuilt:
it would no longer match `v0.6.4`. The tag stays on `04fdd96`; the two
fixes are after it and nothing is pushed.

## 2026-11-17 — 0.6.4 re-cut

The first `v0.6.4` tag (`04fdd96`) was pushed and its release workflow was
canceled before any release was created; the tag was deleted locally and
remotely, and the release commit was re-cut on top of the critical-path
fixes (`12c6fdf`, `844e475`). The version, both changelogs, and both READMEs
are unchanged; the tag now includes the fixes, and the range review carries
the `Code review:` line the new preflight requires. The candidate built from
the first cut is stale; CI builds the published assets from the new tag.

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
