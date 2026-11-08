<!-- reviewed-through: 2026-11-08 -->

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
- Local output mute in the live app: the engine session path passed
  (`TestLiveMuteLocal`); the settings-window switch itself has only headless
  coverage.
- A real default-device switch during playback.
- Multi-room groups larger than two, a member dropping or reconnecting
  mid-pair, and software stereo groups built from independent receivers.
- Audible output and the 50 ms cushion under real WASAPI scheduling jitter;
  a measured left/right offset on a physical pair.
- Silence pauses longer than 60 s.
- Device churn while the adaptive-height flyout is open, and its upward growth.
- The tray menu's 设置… entry, and a settings-window redraw driven by an
  engine snapshot change.
- Latency modes: listening at low, standard, stable, and custom; the restart
  gap; the 3 s streaming hold on a slow reconnect; custom values below 200 ms.
- Tray animation: toggling the setting while connecting or streaming, and
  changing Windows animation effects (Reduce Motion) while running.
- The settings 更新 page and notes dialog: available, downloading, ready, and
  failed states; the release-history list; install from the page.
- A live update from the previous release, and a clean Windows install of
  the current installer.
- The localized update history: the settings 更新 page reads
  `CHANGELOG.zh-CN.md` when the app language is Chinese, including the
  offered version's notes. Pinned by `TestNotesEntriesUseTheLocalizedChangelog`,
  `TestLanguageSwitchRefetchesHistory`, `TestNotesDialogRendersTheLocalizedEntry`,
  `TestFoundUpdatePrefetchesTheLocalizedNotes`, and
  `TestNotesDialogHidesTheEnglishNotesWhileLoading`.
- Receiver management live paths: manual add/edit/remove, hide/show filtering,
  the per-receiver delay restart, receiver-password save and clear, and a
  manual receiver with no mDNS advertisement. Pinned by the receivers, engine,
  and app tests and the `settings-receivers-*` / `dialog-receiver-*` previews.
- The directed discovery fallback in the live app: the resolver is proven
  against both HomePods (`TestLiveUnicastQuery`), but the app's first sighting
  of the re-merged group and the stale-group prune are pinned by headless
  engine tests only.
- Ambient light, artwork channel: the 2026-11-07 `AudioAccessory5,1` muted
  run shows three consecutive `SET_PARAMETER image/jpeg` covers (tide, moon,
  orchid) each repainted the top display with no `/artwork/command`. The
  POST trigger and `TCPArtwork` channel are removed. A black image lands on
  the default white because the firmware lifts black to a full channel. A
  true LED-off is the `updateMRPlaybackState` command (2026-11-07, see the
  playback-state section); the on-device SoundBoardServices XPC
  (`setLEDContents:`, `disable_leds`) remains a local-only path.
- Ambient light, off behavior: the manual 关灯 sends state `2` and turns the
  top display dark while the RTP stream stays up; 开灯 sends `1` and restores
  the current image. A media pause keeps the current light; there is no
  per-media playback-state change. Whether audible audio keeps playing (the
  probe ran muted), the behavior on other models, and the stereo pair are
  still unverified.
- Ambient light, no-media placeholder: a muted stereo pair `AudioAccessory5,1`
  27.3 probe on 2026-11-07 (`TestLivePlaceholderReturn`) sent a tide template,
  cleared the artwork slot, cycled state `2`→`1`, and sent a new item without
  artwork. The user observed blue and dark, with no white center. Clearing the
  sender's slot only stops future sends; the receiver keeps the last image.
  The white-center placeholder appears only on a fresh session with no
  artwork. The sender has no artwork-clear command.
- Ambient light, capability matrix: HomePod 2, HomePod 1, HomePod mini, and a
  receiver below software 27.2. `TopDisplayCapable` holds every unknown and
  the mini back (false) until these runs exist.
- Ambient light, stereo pair: each capable member shows the light, and an
  incapable member is skipped without breaking the pair.
- Ambient light, reconnect and preset change: the top display survives a
  reconnect, and a preset change replaces the light. `TestLampAppliedWhenStreamingStarts`
  and the sender artwork tests pin the re-push path, not the receiver side.
- Ambient light, takeover: another controller taking over the session, and
  whether the light needs clearing.

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
- Settings are stored in `%APPDATA%\saundo\settings.json`.

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
- **Latency UI.** The 音频 page's mode card offers 低延迟 200 ms, 标准 1.5 s,
  稳定 2 s, and a custom 20–2000 ms value; the header switch 托盘面板中显示
  shows or hides the flyout's four-segment row. The playback head's ALAC line
  carries the resolved value, e.g.
  `ALAC · 44.1 kHz · PTP · 已同步 · 延迟 700 ms`. Preset changes restart a
  live session; the custom slider applies 500 ms after the last change. The
  resolved `LatencyMS` still drives the stream SETUP's
  `latencyMin`/`latencyMax` and the sync-packet timeline on PTP and NTP
  sessions; pinned by `TestLatencyAppliesToStreamSetup`,
  `TestLatencyAndPasswordReachSession`, `TestApplyLatencyRestartsActiveSession`,
  `TestFlyoutLatencySegmentSelects`, `TestAudioModeTileSelects`, and
  `TestFlyoutHeightForDeviceRows`.
- **Password receivers.** The receiver-password field stores a per-device password
  that `SetAuth` passes to the RTSP digest path (pinned by
  `TestLatencyAndPasswordReachSession`).
- **Now Playing source.** A new `internal/nowplaying` package reads the
  Windows SMTC session (RoGetActivationFactory → RequestAsync →
  GetCurrentSession → GetMediaPropertiesAsync, all awaited by polling
  IAsyncInfo status, no event delegates). The app polls every two seconds
  when 推送正在播放 is on and forwards title/artist/album to the receiver;
  forwarding is pinned by `TestNowPlayingCommandForwards` and
  `TestNowPlayingRidesAsDMAP`. A real Chrome Media Session was later read live;
  see the Now Playing display section below.
- **App names and icons come from the shell.** SMTC exposes only the AUMID.
  MyGo Music (`MyGo Music.exe`, unpackaged) and Chrome
  (`Chrome.UserData.Profile1`) both failed WinRT
  `AppInfo.GetFromAppUserModelId` with 0x80070490, and the old hardcoded map
  left MyGo Music as 未知应用 (reported live on Windows 11 26300). The read
  now enumerates the Start menu's Applications folder once and matches an
  AUMID to an item by exact id, executable file name or display name, or the
  token before a profile suffix; the item supplies the localized name and a
  64×64 PNG icon. An app outside that folder falls back to its running
  executable and version resource, then the known-app map; without an icon
  the card shows the name alone, and a truly unknown app reads 未知应用.
  Live read after the change: `Chrome.UserData.Profile1` → Google Chrome,
  `MyGo Music.exe` → MyGo Music, packaged
  `AppleInc.AppleMusicWin_nzyj5cx40ttqa!App` → Apple Music, and
  `dev.zcode.app` → ZCode — the last two show that an arbitrary AUMID
  resolves when it is in the Applications folder; all four returned 2–5 KB
  PNG icons, saved under `.audit/previews/nowplaying-appname/icons/`
  (uncommitted). `TestMatchInstalledApp` pins the matching rules,
  `TestAppNameResolvesUnpackagedExecutable` the fallbacks, and
  `TestNowPlayingHeadShowsAppIconWhenAvailable` the icon's presence and
  absence.
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
  and hid the two member rows. Clicking 连接 produced the streaming footer
  (then `正在播放到 HomePods`, now `正在通过 HomePods 播放`), the 已连接 badge, and `ALAC · 44.1 kHz · 立体声 · PTP ·
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

- **The toggle did nothing after startup.** `pollNowPlaying` read a settings
  copy loaded once when the app started; the switch only updated the engine,
  so enabling the push after launch never let the poller run. The poller now
  reads the streaming state from the live engine snapshot. **2026-11-07:** the
  push became unconditional, so there is no switch left to go stale.
- **The current track is visible.** The playback card now has a 正在播放 row.
  It updates every two seconds, shows `title · artist`, and falls back to
  未检测到媒体. The 推送 switches it once carried were removed on 2026-11-07.
  Pinned by `TestNowPlayingCardShowsTrack`.
- **Push behavior.** A changed track is sent whenever a session is streaming;
  the old toggle was removed on 2026-11-07. A new stream resets the sent
  marker and resends the current track after reconnect. Pinned by
  `TestRefreshNowPlayingPushesWhileStreaming`.
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

## Now Playing media card (2026-11-06)

- **SMTC supplies the whole card.** `internal/nowplaying.Read` enumerates
  `GetSessions`, reads each session's source AUMID, media properties,
  playback status and control flags, timeline, and thumbnail, and keeps the
  manager's current session. Artwork comes through `IRandomAccessStream` →
  `IInputStream` → `IBuffer`/`IBufferByteAccess`; PowerShell cannot unwrap
  the same thumbnail (`Cannot convert System.__ComObject to
  IRandomAccessStreamWithContentType`). Live on Windows 11 26300: Chrome
  "Late Night Afrobeats Mix …" (paused, play only, seek available,
  53:41 / 58:39, 22,171-byte PNG) and Apple Music
  "Daimeiwakusei." (paused, play + next, 0:16 / 4:12, 210,734-byte
  JPEG). A later re-run at 2026-11-06 02:04 read Apple Music
  "Cinderella (feat. Ty Dolla $ign)" (paused, play/next/previous, 3:29 /
  8:00, 99,852-byte JPEG) cleanly. Re-run with
  `SAUNDO_SMTC=1 go test -run TestSMTCLiveSessions -v ./internal/nowplaying/`.
- **The card** shows the app name, the session-switch arrow, a
  fixed-52-DIP-tall poster whose width follows the cover's ratio (a 16:9
  thumbnail gets a wider poster, never a taller card), title/artist, a
  five-band meter, and the progress row. The transport controls sit at the
  row's left as 24-DIP circles in the session arrow's card style — card
  fill, muted 12-DIP glyph, hover, 4 DIP apart, no filled primary — and
  only enabled commands are drawn. Each calls `nowplaying.Control`
  (TryPlay/TryPause/TrySkipNext/TrySkipPrevious, plus
  TryChangePlaybackPosition for a later seek affordance); the tester pins
  the pause click and the AUMID in
  `TestNowPlayingCardControlsSendActions`. An unrecognized AUMID reads
  未知应用 / Unknown app. A session with no timeline keeps the same progress
  row as an empty track — no fill, thumb, times, or copy — so the card
  height does not change. With no session at all the card is not drawn: the
  playback card keeps the volume and latency rows, `fitFlyout`
  drops the card's 150 DIP, and the 62-DIP spacer that used to separate the
  device card from the playback card is gone. The tester pins the hidden card
  in `TestNowPlayingCardHidesWithoutMedia`, and the height cases in
  `TestFlyoutHeightForDeviceRows`. The commands were not sent to a live media
  app in this unit, because that would change the user's playback; the
  session lookup they use is the one the live SMTC test covers.
- **The first-artwork placeholder drew a light ring.** Before SMTC supplied
  the first thumbnail, the poster showed the note tile filled with the card
  color while the media card uses the pill; the tile's edge read as a white
  border (reported from the first Apple Music play on Windows 11 26300).
  Later plays used the cached artwork, which covers the tile. The artwork's
  tile now keeps the pill, so a corner sliver never shows as an edge; the
  placeholder tile moved to the raised card surface so it reads as a cover
  distinct from the card,
  and `TestNowPlayingPlaceholderStandsOutFromTheCard` pins that fill in both
  themes.
- **The meter.** Capture blocks feed `internal/audio/analyze.Meter` on the
  capture goroutine: stereo-energy level over 60 dB and five Goertzel bands,
  fast attack/slow release, a 20 s frame ring. The card reads the frame
  delayed by the selected latency, so the bars follow what the receiver is
  playing. `TestMeterLevelBandsAndDelay` pins level, bands, delayed read,
  and decay; `TestSilenceMeterFeedsTheLevelMeter` pins the engine tap. The
  card asks for 30 fps only with a real window and Reduce Motion off, so
  headless previews settle.
- **The meter twitched at low level.** Each band normalized its magnitude by
  its own running peak, and that peak decayed into the noise floor during
  quiet passages; every block then read `mag/peak` near 1, so dither and room
  tone filled the bars and moved them on tiny changes (reported against
  quiet playback). The meter now ignores band energy below -50 dBFS, floors
  the per-band gain 18 dB above the gate (a quiet passage compresses instead
  of pumping), and the envelope follower ignores changes under 0.02 of the
  bar range. `TestMeterGatesNoiseBelowTheFloor` and
  `TestMeterQuietToneKeepsBarsLow` read about 1.0 on the old meter and pass
  now; `TestFollowIgnoresSubBarChanges` pins the deadband. The change was
  judged with synthetic tones and noise, not with a listener's track; the
  live meter was not captured.
- **The cover field.** `coverField` center-crops the artwork, samples a
  23×24 grid, blurs it, blends toward its average, compresses HSL
  lightness, and lifts brightness/saturation per theme.
  `TestCoverFieldCompressesLightness` pins the size and compressed range.
  The native previews (`.audit/previews/nowplaying-card-v6/`, uncommitted)
  show the stream, compact no-media, and unknown/no-timeline states in both
  locales and themes; their artwork is a generated gradient, not a
  listener's cover.
- **Checks.** `mingw32-make check` passed: gofmt, go vet, `go tool mygo
  vet`, the catalog validation, every package's tests, and the verify map
  (`map ok`). `SAUNDO_PREVIEW_DIR=… go test ./internal/app -run
  TestRenderPreviews` passed in 20 s, and `mingw32-make build` rebuilt and
  relaunched `bin/saundo.exe`. With no media session the rebuilt flyout
  measured 420×456 live (the media card hidden and the old spacer gone) and
  was captured at `/tmp/saundo-verify/20261106-110235/flyout-compact.png`;
  the capture is DPI-clipped to the window's top-left, so the compact
  visual is `.audit/previews/nowplaying-card-v6/flyout-idle-zh.png` and the
  media state is `.audit/previews/nowplaying-card-v6/flyout-stream-light-zh.png`
  (606 DIP). Live HomePod audio, the meter against a real receiver, and a
  live press of the transport commands were not exercised in this unit.

- **The media card kept the first thumbnail.** `internal/nowplaying` cached
  each session's artwork by AUMID/title/album, and the app's decode key used
  the same fields. Chrome first exposed its logo as the thumbnail and the real
  video thumbnail arrived a poll later; both caches kept the logo while
  Windows' own media overlay showed the video. The package cache is gone (the
  thumbnail is read on every poll) and the app key now includes an FNV-1a
  hash of the bytes; `TestNowPlayingArtworkRefreshes` changes the artwork under
  the same title and fails without it. A fresh live read of the Chrome session
  returned the 14,243-byte video thumbnail.

## Now Playing settings (2026-11-06)

- **The 正在播放 page.** The settings window gains an eighth page between 采集
  and 更新. It has one card: the display master (`showNowPlayingPanel`, on)
  with the 系统媒体 badge, a preview of the real Now Playing card (the live session, or a sample Apple Music / 夜航 / 白鲸电台 card when nothing plays), and
  the rows 播放控制 (`playbackControls`), 封面配色
  (`coverColors`), and 音频电平 (`audioLevel`). Master off greys the preview
  and disables every row; there is no closed placeholder. New keys in
  `settings.json` default on; a missing key reads its default.
  **2026-11-08:** the card is 媒体卡片, its chip is 托盘面板中显示, and the
  switch only hides the flyout card; the settings preview and the option rows
  stay active.
  **2026-11-07:** the 在接收器上显示曲目信息 row was removed; the receiver
  push is unconditional and no longer depends on the card's display master.
- **The cover mode.** The page's 封面 row is a three-way segmented control
  in the latency switch's style: 无封面 (`none`), 默认封面 (`placeholder`),
  and 媒体封面 (`media`, default). The card omits the poster for none, shows
  the raised note tile for placeholder, and the media artwork for media; the main
  row keeps its 62-DIP height, so the flyout height is identical in all three.
  The mode persists as `coverMode` in `settings.json`; an invalid stored value
  reads as `media`. `TestNowPlayingCoverModes` pins the left title shift with
  no poster and the fixed card height; `TestNowPlayingCoverModePersists` and
  the settings test pin persistence and the invalid fallback.
- **The flyout follows the page.** `nowPlayingCard` returns nothing while the
  master is off, and `hasNowMedia` drops the card's 150 DIP from the flyout;
  the flyout shrinks exactly like the no-media state. The playback controls
  setting removes the transport; cover colors off draws the progress fill and
  meter in the default ink; audio level off removes the five bands. The
  receiver push is unconditional: the app sends the track and cover whenever
  a session streams.
  The settings preview uses the same card body in preview mode, dimmed at 45%
  opacity and read-only; it falls back to a fixed sample so no-media users still
  see the card. The sample strings are `settingsNowPlayingSampleTitle` /
  `settingsNowPlayingSampleArtist`.
- **The settings preview's meter stepped every two seconds.** The preview
  passed its read-only flag as the meter's animate flag, so the settings card
  never asked for paint frames; the bars sampled the live level only when the
  SMTC poll invalidated the window, once per 2 s. The card body now takes the
  meter's animation separately: the flyout always animates, and the preview
  animates only for a live session while the master is on. `meterAnimates`
  and `previewMeterAnimates` pin the decision. `meterAnimates` reads Reduce
  Motion through a func because `mygo.Theme.Preferences()` blocks without a
  running app; the eager read hung the table test. The smoothness itself is
  user-observed: no automated check counts a real window's frames.
- **The receiver push carries the cover.** The sender already emitted the
  thumbnail as an image `SET_PARAMETER`, but `Engine.SetNowPlaying` passed
  nil and the app's change key held only the text. The cover now rides
  through the engine command and both transports; the key includes an FNV-1a
  hash of the bytes, so a thumbnail that lands or changes after the text is
  sent again in the same session, and turning the switch back on resends the
  current track instead of waiting for the next.
  `TestNowPlayingCoverRidesAsImage`,
  `TestNowPlayingCommandForwards`, and
  `TestRefreshNowPlayingPushesArtworkChanges` pin the path. Live check
  2026-11-07: the MacBook Air (`Mac14,2`) AirPlay receiver showed the pushed
  SMTC cover in its Now Playing UI (user-confirmed). The HomePod pair has no
  display; its rows still receive the same metadata push.
- **Previews and tests.** `.audit/previews/nowplaying-settings/` (uncommitted)
  holds the live settings page, the master-off page, and the compact flyout
  without the card, zh-CN and English, light and dark. The page is covered by
  `TestSettingsPagesRender`; the master switch, persistence, disabled rows,
  and the flyout shrink by `TestNowPlayingSettingsPersist` and
  `TestNowPlayingPanelHiddenWhenOff`; the transport gate by
  `TestNowPlayingControlsSetting`.
- **Checks.** `mingw32-make check` passed (gofmt, go vet, `go tool mygo vet`,
  catalog validation, every package's tests, verify map `map ok`);
  `SAUNDO_PREVIEW_DIR=… TestRenderPreviews` passed; the previews above were
  inspected. Live app driving of the new page was not run in this unit.

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
  `TestFlyoutTargetHeightFitsPlayback` checks the playback card's last row,
  the latency segment, is not clipped at the shorter heights. The empty and
  one-device previews now render at
  their target heights.
- **Checks run.** `gofmt -l .` and `go vet ./...` printed nothing;
  `go test ./...` passed every package. PNG renders at 500, 444, 388, and
  470 DIP confirmed the spacing and showed no clipped rows.
- **Live check (2026-11-01).** After the rebuild and restart, the user
  confirmed that the two-device flyout fits its content with no empty band,
  which runs `fitFlyout`'s `SetSize` path on the real tray flyout. Device
  churn while the panel is open, and the upward growth that comes with it,
  were not exercised.

## Results from 2026-11-02 (new network, new Windows PC, old MacBook)

Audio streamed normally. Four issues surfaced:

- **Sender name.** The Chinese sender name "声渡" arrived on the old
  MacBook as mojibake. The AP2 session SETUP encoded every binary-plist
  string with the ASCII marker (0x5x), including UTF-8 bytes, so the
  receiver decoded them as Latin-1. Non-ASCII strings are now encoded as
  UTF-16BE (0x6x) and decoded fully; pinned by
  `TestBPListNonASCIIString`.
- **Update check.** The check returned 404 for
  `https://github.com/4202217/saundo/releases/latest/download/update-windows-amd64.json`.
  The 0.4.0 release was still a draft, and the repository had been made
  private, so no unauthenticated feed was served. The feed is fetched
  without credentials: the repository, or the `updates.url` host, must be
  public. After publishing 0.4.1 and restoring public visibility, the feed
  returned 200 with manifest 0.4.1. The 0.3.0 build predates the updater;
  the first update path is 0.4.0 to a later release.
- **Volume readout.** The slider's 0 to 100 value was formatted with the
  ICU `percent` style, which multiplied it by 100. The panel showed 0% to
  10000% and now shows 0% to 100%; pinned by the volume assertion in
  `TestViewRendersDevicesAndStatus`.
- **Idle drops.** Capture runs with no stream, and the undrained ring
  counts discarded samples. The flyout now shows the dropped figure only
  while streaming, and the engine reports zero while idle; pinned by
  `TestCaptureMetaHidesDropsWhenIdle`.

## Tray animation (2026-11-02)

- The tray animates `stream` and `connect` with 12 frames at 100 ms. The eyes stay static; only the mouth moves: the five stream bars ride a traveling sine wave, and the three connect dots pulse in brightness and size. The 托盘动画 switch in 通用 turns it off; Reduce Motion (Windows animation effects off) holds the static face even while the switch is on.
- The static stream icon is animation frame 0, and the static connect icon is three uniform 2.6 px dots. Switching the animation on or off therefore does not change the mouth size.
- Measured on the 4K 200% taskbar: `Tray.SetIcon` averaged 0.5 to 0.8 ms per frame. GDI objects stayed at 53 across 20-second runs, with no handle leak. Explorer CPU with the taskbar visible ranged from 1.6 percent of one core for the static icon to 4.8 percent for the 100 ms animation over 20-second samples; the figure is noisy and the app-side cost stayed under 1 percent. A 60 ms frame interval produced a 10 ms worst-case call, so 100 ms is the default.
- `TestTrayAnimationDefaultsOnAndRoundTrips`, `TestTrayAnimationSettingPersists`, `TestTrayAnimationCommandPersists`, `TestTrayAnimationFaces`, and `TestSettingsPagesRender` pin the default, the app and engine copies of the switch, the animated faces, and the settings row. Not verified: a live HomePod run of the final settings toggle; the timing above was measured in the prototype's demo mode, which uses the same animator and `SetIcon` path.

## Latency modes (2026-11-03)

- **Design.** 低延迟 is 200 ms and stays the default; 标准 is 1.5 s; 稳定 is
  2 s; 自定义 accepts 20–2000 ms. The flyout shows the four segments and the
  current value on the ALAC line, with no slider or value button of its own.
  The settings 音频 page owns the mode tiles and the custom slider.
- **References.** pyatv, owntone, shairport-sync, and UxPlay advertise 11025
  samples (250 ms) as the minimum; pyatv uses 66150 samples (1.5 s) for RAOP,
  shairport-sync's receiver default is 88200 (2 s), and RAOP-Player clamps to
  `RAOP_LATENCY_MIN` 11025. The 200 ms default is this project's HomePod
  SETUP value. A 100 ms preset was dropped because no target receiver has a
  record at that value; custom values below 200 ms are marked unverified.
- **Migration.** A stored 200, 1500, or 2000 ms selects the matching preset;
  any other stored value becomes custom with that value. A missing mode keeps
  the old latency as its custom value. `ShowLatencySwitch` defaults on.
- **Engine.** `SetLatency` keeps applying on the next session.
  `ApplyLatency` sets the value and restarts a live session so the receiver's
  stream SETUP carries it; `SetLatencySettings` keeps the app-only mode,
  custom value, and switch in the engine's settings copy so a later engine
  save cannot overwrite them. A preset click updates the selection, the ALAC
  line, and the engine's stored value at once, but the restart waits 300 ms
  for the last click; the custom slider waits 500 ms. Rapid low → standard →
  stable therefore sends one restart with the final mode. While that restart
  runs, the app holds the last streaming snapshot for up to 3 s, so the device
  row, footer, and tray keep their streaming face; a spinner
  follows the 播放 label so the line never shifts. Errors and user disconnects
  end the hold at once. The debounce is UI timing and is not pinned by a
  timing test; the selection and `ApplyLatency` restart paths are tested.
- **Tests.** `TestLatencyModeDefaults`, `TestLegacyLatencyMSChoosesMode`,
  `TestLatencyPresetNormalizesAndCustomRoundTrips`, `TestLatencyText`,
  `TestApplyLatencyRestartsActiveSession`, `TestLatencyApplyHoldsStreamingSnapshot`,
  `TestLatencySpinnerKeepsALACLineAnchored`, `TestLatencySettingsReachStore`,
  `TestFlyoutLatencySegmentSelects`, `TestAudioModeTileSelects`,
  `TestFlyoutLatencySwitchCanHide`, `TestFlyoutCustomLatencyOnALACLine`,
  `TestAudioCustomModeRendersSlider`, and `TestFlyoutHeightForDeviceRows`.
  The mygo previews `settings-audio-light`, `settings-audio-custom-light` and
  `-dark`, `flyout-custom-light` and `-dark`, `flyout-latency-applying`, and
  `flyout-switch-off` render
  the states in both languages.
- **Short windows.** Each settings page body scrolls inside the fixed title
  area, which ends in a divider. The overlay scrollbar runs in the 20 DIP
  gutter at the window edge, clear of the cards, and the audio page's
  read-only rows use the shared `settingRow` so their padding matches the
  other pages. At the 420 DIP minimum the audio page keeps the custom card
  and its slider whole and scrolls to the read-only rows; the compact and
  scrolled previews (`settings-audio-compact-light` and `-dark`,
  `settings-audio-compact-scrolled-light` and `-dark`) render those states,
  and `TestSettingsAudioPageScrollsAtMinimumHeight` scrolls the sync row into
  view.
- **Not verified.** Listening at each mode on a live HomePod, the reconnect
  gap when a preset restarts a session, and a measured dropout or
  end-to-end-latency sweep. The 3 s hold was not tuned on a slow HomePod
  reconnect. The mode values are reference-backed; the 200 ms default is the
  value the receiver has been used with.

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

## Ambient light, two-step cover (2026-11-06)

HomePod R (`AudioAccessory5,1`, software 27.2) with Sound Glow enabled,
muted to volume 0 so no test made sound.

- **Raw JPEG is rejected.** `POST /artwork/command` with `image/jpeg` returns
  `400 Bad Request` on both the plaintext HTTP server and the encrypted RTSP
  channel. The binary-plist body returns `200 OK`.
- **The POST alone does not replace the artwork.** A red cover sent by
  `SET_PARAMETER image/jpeg` stayed red after a blue binary-plist POST. The
  top display renders the current Now Playing cover, not the POST body.
- **The cover plus the POST works.** Blue `SET_PARAMETER image/jpeg`, then the
  binary-plist POST one second later, made the top display blue. The same
  sequence without the cover showed nothing.
- **Production path (superseded 2026-11-07).** `SetLampArtwork` sent a minimal
  DMAP item when the app had no track metadata, set the cover by
  `SET_PARAMETER image/jpeg`, then fired `POST /artwork/command` one second
  later. The cover-only run removed the POST; the current path is pinned by
  `TestSetLampArtworkSendsCover`, `TestSetLampArtworkWaitsForStreaming`,
  `TestSetLampArtworkDuplicateIsNoOp`, `TestSetLampArtworkDoesNotTouchDMAPText`, and
  `TestSetLampArtworkClearSendsNothing`.
- **Default reset, no LED-off (historical).** The first preset `origin` was
  a solid black JPEG. On HomePod 27.2 the firmware lifts black to a full
  channel for its color stops, so the top display landed on the default
  white, not an LED-off. Removed on 2026-11-07; off is now the playback
  state. The off switch sends the same image. A true LED-off has no
  AirPlay command; only the on-device SoundBoardServices XPC
  (`setLEDContents:`, `disable_leds`) drives the LEDs, and it is
  entitlement-gated.
- **Still unverified.** The default white through the `origin` preset on a
  real receiver, the 6,1 and first-generation HomePod, and a stereo pair's
  simultaneous repaint. A true LED-off has no AirPlay command.

## Local output mute (2026-11-06)

The connection mute calls `IAudioEndpointVolume.SetMute` on the render
endpoint. On some drivers that also silences WASAPI loopback capture, which
would silence the stream itself. A manual probe
(`go test -tags muteprobe ./internal/capture -run TestLoopbackSurvivesEndpointMute -v`)
played a 440 Hz tone through the default endpoint, captured the loopback, and
muted the endpoint between two measurements.

- **Probe result.** RMS stayed at about 0.17: 0.1679 before the mute, 0.1768
  while muted, and 0.1768 after the release in the latest run. The endpoint
  was already muted before the probe (`previousMute=true`), and the probe left
  it in that state. On this machine, Realtek Digital Output at 48 kHz, the
  loopback tap is upstream of the endpoint mute.
- **Pre-muted endpoints (2026-11-07).** The first version restored the
  endpoint's mute state from before the session. On a machine whose endpoint
  was already muted, every disconnect and quit restored silence: the
  `mute-guard.json` written at 16:13:55 read `previousMute=true` while the app
  was streaming, and each earlier restore had put the mute straight back. The
  connection mute is now temporary: `applyLocalMute` mutes, and
  `restoreLocalMute` always unmutes the endpoint it owns, whatever state it
  was found in. `TestMuteLocalReleasesAPreMutedEndpoint` starts with a muted
  endpoint and fails on the old code with "disconnect left the pre-session
  mute in place".
- **Driver dependence.** The tap point is driver-dependent; the settings note
  warns that muting can also silence capture. The probe is manual because it
  touches the user's audio device, not a CI test.
- **Crash guard.** The engine writes `mute-guard.json` before muting and
  removes it after releasing. `TestMuteGuardRecoversAfterCrash` covers the
  next-launch release, including a guard written by the first format with
  `previousMute`; `TestMuteGuardCorruptIsLeftAlone` keeps an unreadable guard
  instead of guessing which endpoint to unmute.
- **Pins.** `TestMuteLocalFollowsTheSession`,
  `TestMuteLocalOffRestoresAtOnce`, `TestMuteLocalOnWhileStreaming`,
  `TestMuteLocalFailureIsVisible`, `TestMuteLocalTargetsSelectedEndpoint`,
  `TestMuteLocalProcessCaptureUsesDefaultEndpoint`,
  `TestMuteLocalReleasesAPreMutedEndpoint`, `TestMuteLocalSettingPersists`,
  `TestConnectionPageMuteLocalStates`.
- **Live sessions.** `SAUNDO_MUTE_TEST=1 go test ./internal/engine -run TestLiveMuteLocal -v`
  discovered HomePod L at 192.168.31.46 on 2026-11-06, reached streaming,
  observed the local endpoint muted and `mute-guard.json` present, then
  disconnected and observed the endpoint released and the guard gone. The
  2026-11-07 run repeated the cycle with the stereo pair at 192.168.31.164 and
  added a second session that starts with the endpoint pre-muted: it stayed
  muted while streaming and was released on disconnect. The test mutes the
  receiver before audio can flow, drives the engine directly with temporary
  settings and credentials, and restores the receiver's remembered volume
  before disconnecting; it does not click the settings-window switch. That
  switch is pinned by the headless UI tests and the native previews.
- **Still owed.** A forced kill in the live app, and the settings-window
  toggle clicked in a real session; the engine guard and the UI wiring are
  covered separately.

## Stereo group name decoding (2026-11-06)

A HomePod mini stereo pair (`AudioAccessory5,1`, software 27) advertises the
Home app group name in the TXT `gpn` field. `hashicorp/mdns` reports TXT
strings in RFC 1035 presentation form, so a UTF-8 name arrives as `\DDD` byte
escapes: the pair named 卧室 advertised `gpn=\229\141\167\229\174\164`.
Discovery decoded those escapes for the instance name but not for `gpn`, so
the collapsed pair row and `receivers.json` stored the escaped text.

- **Finding.** A live browse printed the group as
  `name="\\229\\141\\167\\229\\174\\164"`, and `receivers.json` held the same
  escaped name under `stereo:7A6CE980-7265-5ABD-B3E9-B90563D5932F`.
- **Fix.** `parseEntry` runs `gpn` through `unescapeDNS`, the same path as
  the instance name; devices.md and protocol.md record the rule.
- **Pin.** `TestStereoGroupNameDecodesDNSEscapes` fails before the fix with
  `group name = "\\229\\141\\167\\229\\174\\164"` and passes after it.
- **Live app (2026-11-06).** The rebuilt 0.4.2 app discovered the pair as
  `卧室 · AudioAccessory5,1 · 立体声组 2/2`, showed the streaming footer
  (then `正在播放到 卧室`, now `正在通过 卧室 播放`), and
  rewrote the stored name to `卧室`. Flyout capture:
  `/tmp/saundo-verify/20261106-gpn-fix/flyout-after.png`.

## Directed discovery fallback (2026-11-06)

On this network the HomePod pair (HomePod L 192.168.31.46 and HomePod R
192.168.31.164, software 27.3) was online and accepted TCP 7000, but the
reported 8-second multicast browse saw only the MacBook Air and the Redmi. The
app therefore marked the pair offline, and a remembered group could not be
connected.

- **Multicast is intermittent.** Repeating
  `SAUNDO_MDNS_TEST=1 go test -run TestBrowseIntegration -v ./internal/discovery`
  with the directed pass disabled (the test now sets `unicastEvery = 0` so it
  measures multicast alone) produced three different snapshots in three runs:
  the complete 卧室 group; HomePod L and HomePod R as separate `_raop`-only
  devices (`ap2=false`, no tsid, so no group); and the MacBook Air and Redmi
  only. The loss is per reply, not a permanent block, which is why a single
  successful browse does not prove the next cycle will be complete.
- **Directed queries are reliable.** A directed PTR query to each address
  returned that HomePod's own `_airplay` and `_raop` instance every run, in
  about 10 to 30 ms. `_airplay` carried
  `tsid=7A6CE980-7265-5ABD-B3E9-B90563D5932F`, `gpn` 卧室, `deviceid`, and the
  feature bits. `_raop` carried `am`, `ft`, and `sf` and no `deviceid`, so its
  identity came from the `<MAC>@<name>` instance. Both members advertised
  `igl=1` in this capture. The `_airplay` reply is about 700 bytes; the DNS
  client read buffer must exceed the 512-byte default (the resolver uses
  8192).
- **Fallback.** `internal/discovery` keeps the multicast browse and adds a
  directed pass every fifth cycle (15 s at the 3 s interval). Candidates are
  the Windows ARP neighbor table (`GetIpNetTable`) plus the manual and
  last-known receiver hosts from `receivers.json`. The pass queries both
  service types, follows SRV/TXT/A when the responder omits them, synthesizes
  an `mdns.ServiceEntry`, and runs the existing `parseEntry`/merge path. A
  missing record skips that instance; it never becomes a device with an empty
  address.
- **Persistence and prune.** A discovered row stores its last host, port, and
  `lastSeen`; a stereo group stores the leader's endpoint. A remembered
  `stereo:<tsid>` unseen for 24 h is pruned with its auto-connect flag, because
  a split and re-merge mints a new tsid. A group without `lastSeen` is stamped
  on the first launch instead of pruned, so an upgrade still gets the full
  grace.
- **Pins.** `TestUnicastFallbackDiscoversStereoPair` (loopback DNS server,
  both service types, group 卧室), `TestUnicastSkipsInstanceWithoutAddress`,
  `TestUnicastRunsEveryFifthCycle`,
  `TestUnicastCandidatesMergeNeighborsAndSeeds`,
  `TestParseIpNetTableFiltersUnusableNeighbors`,
  `TestNeighborHostsReadsTheArpTable`,
  `TestDiscoveredReceiverPersistsAddressAndLastSeen`,
  `TestDiscoveredGroupPersistsLeaderAddress`,
  `TestLegacyGroupGetsGraceInsteadOfImmediatePrune`,
  `TestStaleGroupPrunedAfterGraceAndLosesAutoConnect`,
  `TestGroupOfflineWithinGraceIsKept`, and
  `TestPruneStaleGroupsRemovesOnlyExpiredUndiscoveredGroups`.
- **Live check.** `SAUNDO_UNICAST_TEST=192.168.31.46,192.168.31.164 go test
  -run TestLiveUnicastQuery -v ./internal/discovery` resolved both HomePods
  over both services in 20 ms; the log showed `tsid=7A6CE980-...` and
  `gpn="卧室"` for each member. With multicast disabled and only the directed
  pass running (`SAUNDO_UNICAST_BROWSE=1 go test -run TestLiveUnicastBrowse -v
  ./internal/discovery`), the service found the MacBook Air, the Redmi, and the
  卧室 group (2 members, online) in 1.0 s from the real ARP table.
## Ambient light, cover-only (2026-11-07)

HomePod R (`AudioAccessory5,1`, 27.2), muted, lamp app built with
`SAUNDO_LAMP_NO_TRIGGER=1`.

- **Cover-only repaint works.** Three consecutive `SET_PARAMETER image/jpeg`
  covers (tide, moon, orchid) each repainted the top display; no
  `/artwork/command` was sent. The startup ember cover was not the only one.
- **Removed.** `sendCover` now only sets the Now Playing cover. The
  `TCPArtwork` channel, `artworkTrigger` timer, binary-plist data URL body,
  keep-alive response parser, and 1 s delay are gone. Pinned by
  `TestSetLampArtworkSendsCover`, `TestSetLampArtworkWaitsForStreaming`,
  `TestSetLampArtworkDuplicateIsNoOp`, `TestSetLampArtworkDoesNotTouchDMAPText`, and
  `TestSetLampArtworkClearSendsNothing`.
- **One cover slot; off is a state (2026-11-07).** The sender stores the
  media cover and the ambient image separately. A template owns the artwork
  slot; cover priority hands it to the media cover; off sends the paused
  playback state and leaves the slot alone. A new media cover updates the Now
  Playing text without displacing a template. Pinned by
  `TestMediaCoverWinsWhenLampIsOff`, `TestLampCoverWinsWhileOn`,
  `TestLampCoverKeepsPlayingOnPause`, and `TestSetPlaybackStateSendsCommand`.
  The origin preset is gone; off no longer uses an image. Cover priority is a
  separate flag (`lamp.cover`); the six templates are the only presets.
- **Still unverified.** The receiver-side precedence between the light and
  the receiver track-info cover, the 6,1 and first-generation HomePod, and a
  stereo pair's simultaneous repaint.

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

## Ambient light, playback-state off (2026-11-07)

HomePod mini (`AudioAccessory5,1`, 27.2), muted. The sender connected, held a
silent RTP stream (empty ring, receiver volume 0), sent the tide palette
cover, then sent `updateMRPlaybackState` 2 → 1 → 2 → 1 through
`POST /command` with a binary plist body:
`{type: "updateMRPlaybackState", params: {mrPlaybackState: <n>}}`.

- **The display went blue → off → blue → off.** The user watched the top
  display through the four steps; the probe made no sound.
- **The stream stayed up.** `framesSent` rose from 1,502,688 at the first
  pause to 2,384,800 at the end, `State()` stayed `streaming`, and no close
  event arrived. Every `POST /command` returned `200 OK`.
- **Mechanism.** The AirPlayReceiver applies `mrPlaybackState` to the main
  session; MediaRemote's `isPlaying` changes; `NowPlayingCap` clears
  `currentLEDEffect`; `SoundBoardFramework.MediaRenderer` stops the Sound
  Glow renderer. The command is independent of the artwork slot, so no black
  or white image is needed for off. The ambient model uses it two ways: the
  manual 关灯 sends `2`, and 开灯 sends `1`; a media pause keeps the current
  light.
- **Pinned by.** `TestLivePlaybackState`, gated by
  `SAUNDO_PLAYBACK_STATE_TEST=1`. The test asserts the session stays up and
  logs each state change; the visual result is a user observation, not a
  test assertion.
- **Still unverified.** Audible audio during state `2` (the probe was muted),
  the command on the 6,1 and first-generation HomePod, and both members of a
  stereo pair.

## Ambient light, three-color template carriers (2026-11-08)

The six templates are no longer dark-base pictures with four blended colors.
The receiver clusters the artwork in Lab and drives the LEDs from three
colors, so a blended image collapsed to three near neighbours of one hue
family; dune's gold, blue, coral, and cream became three warm grays. Each
preset is now three Lab-separated colors with area weights (45/30/25), and
its 600×600 carrier is three flat color fields with a narrow feather. The
app preview paints the same three colors.

- **Pinned by.** `TestPresetCarriersRecoverTheirPalette` renders each
  carrier, decodes it, runs the same `CoverPreset` Lab k-means, and requires
  every designed color within ΔE 12. `TestPresetColorsAreDistinct` keeps
  each template's three colors at least ΔE 12 apart; the golden dune's
  closest pair is ΔE 14.5, and the old collapse's closest pair was ΔE 7.1.
  `TestPresetWeightsSumToOne` pins the carrier areas.
- **Live observation (2026-11-08).** The first all-gold dune (`#E0B000`,
  `#C47A1E`, `#F0C050`) read white on the top display; the pale gold was the
  white source. The final dune (`#C08A00` with zero blue, `#C47A1E`,
  `#7A3F14`) read as dark gold, and the user approved the six palettes by
  eye.
- **Not verified live.** The receiver's exact cluster count and sampling are
  not reversed.

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

## Flyout capture line removed (2026-11-08)

- **The change.** The playback card no longer draws the capture line
  (`采集 … · 48 kHz · 丢帧 0`, or its capture-error and not-running variants).
  The row cost 27.3 DIP, and the old 474-DIP base left the device card about
  5 DIP clipped; the base was 456 at that commit, so the window is 394 DIP at
  three device rows with no media card, 338 at two, 282 at one, and 365
  empty. The card padding change later moved the base to 459, making those
  397, 341, 285, and 368. The media card still adds 150, the latency row 44,
  and the ambient section 90.
- **Error bar.** The connection error bar used to ride on the space the
  capture line gave up in the error state. It now adds its own 48 DIP
  (`errorBarHeight`) through `flyoutHeightForState`, so the device card and
  the latency row stay unclipped. Minimum unclipped windows with the latency
  row on and no media were measured at 499/416/472/528 DIP for 0–3 devices;
  the error bar adds exactly 48 in every case.
- **Tests.** `TestFlyoutTargetHeightFitsPlayback` now holds the latency row
  as the playback card's last-row canary instead of the capture line;
  `TestErrorStateShowsRetry` asserts the error bar does not squeeze the
  device card or clip that row; `TestFlyoutHeightForDeviceRows` pins the new
  heights. `TestCaptureMetaHidesDropsWhenIdle` and `captureMeta` are gone.
- **Capture status.** The settings 采集状态 row still names the endpoint and
  rate (`formatRate` stays there); the flyout shows neither, and the verify
  map's capture-line assertions and the flyout mockup template are updated.
- **Checks run.** `mingw32-make check` passed: the gofmt gate printed
  nothing, `go tool mygo i18n check cmd/saundo` reported 331 messages in en
  and zh-CN, `go vet ./...` and `go tool mygo vet .` printed nothing,
  `go test ./...` passed every package, and `check-verify-map.sh` printed
  `map ok`. PNG renders at the stream, idle, error, and empty states showed
  no capture line and no clipped card. `mingw32-make build` rebuilt
  `bin/saundo.exe` and relaunched it; the live flyout was hidden while the
  user streamed, so the in-app visual check is left to the user.

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

## Graduated

Dated batches compressed under the rule at the top of this file: every claim
has a durable home — a named test, devices.md or protocol.md, the code that
owns the behavior, or a later dated entry. Each entry keeps the date, the
surviving conclusion, and the pin.

- 2026-10-31, first device run: HomePod 27.2 (`AudioAccessory5,1`) accepts
  the stream only with `timingProtocol: PTP`, `timingPeerInfo`/`timingPeerList`,
  and SETPEERS — an NTP-timed stream is silently ignored; the rule lives in
  protocol.md and devices.md. The PTP follower held roughly 1 ms offset
  (boot-relative clock; only the offset matters). A muted 35-second session
  and a 440 Hz tone streamed clean, and the user confirmed hearing the tone.
  The real-window run (autoConnect to streaming, 断开/连接, volume 200s, tray
  restore from the hidden-icons overflow) was re-proven in the tray flyout
  section below.
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

## UI copy and layout sweep (2026-11-08)

- **Copy gate.** `TestCatalogHasNoDeadKeys`, `TestCatalogBannedTerms`,
  `TestCatalogUniqueValues`, and `TestLatencyText` pin the rules in
  [ui-copy.md](ui-copy.md). The sweep deleted 16 dead keys and
  `receiverDelayUnit`, and moved 313 messages to the terms in that page.
- **Layout gate.** `SAUNDO_PREVIEW_DIR=... go test ./internal/app -run
  TestRenderPreviews` renders the state matrix in both languages. The flyout
  section heads share one left edge, the three settings card heads share the
  44-DIP title inset, the English compact segment reads `Cover` / `Light`,
  and the update page names a local build as `0.5.0 · 本地构建`.
- **Live app (2026-11-08).** The rebuilt 0.5.0 app was driven through the tray
  icon and the flyout gear. The tray tooltip read `声渡 · 正在通过 HomePods
  播放 · 0.5.0 可用`; the update page showed `0.5.0 可用` with `当前版本 0.5.0
  · 本地构建`. Captures:
  `.audit/ui-review-evidence-live/flyout.png` and
  `.audit/ui-review-evidence-live/settings-update-local.png`. The capture
  helper is DPI-virtualized at 720×500, so the right edge is cropped; the
  headless matrix is the full visual record.
- **Height.** The card padding change moved the flyout base from 456 to 459;
  `TestFlyoutHeightForDeviceRows` and `TestErrorStateShowsRetry` pin it.
- **Notes.** The connection, latency, and Sound Glow caveats sit under their
  cards as `infoNote`: an info glyph in a 16-DIP slot and 11.5 px muted copy
  that wraps instead of truncating. The lamp power hint stays inside the card
  as plain text. `声音光晕` is Apple's official Chinese name for Sound Glow.

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
