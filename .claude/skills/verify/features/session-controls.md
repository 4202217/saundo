# Session controls

The flyout's playback area: volume, latency segments, the Now Playing media
card, capture status, and the diagnostics log card.

## Sub-features

- Volume slider: sends `SET_PARAMETER volume` while streaming; 0 mutes
  (the `-144` dBFS sentinel); a remembered device reconnects at its stored
  volume; a first connection adopts the receiver's `initialVolume`. While
  streaming, the sender polls `GET /info` every second — a knob moved on the
  receiver itself mirrors back into the slider.
- Latency: flyout segments 低延迟 200 ms (default) / 标准 1.5 s / 稳定 2 s /
  自定义 20–2000 ms; the value shows on the ALAC line
  (`延迟 700 ms`). Preset clicks debounce the session restart 300 ms,
  the custom slider 500 ms (constants `latency.go:20-21`); the effective
  value is base + the receiver's DSP/DAC delay, clamped 20–4000 ms
  (`receivers.go:44-45,155-167`). The 面板快捷切换 switch that hides the
  segment row lives in the settings window's 音频 page latency-card header,
  not in the flyout.
- Now Playing media card: app name (未知应用 when SMTC names no app) with
  the session-switch arrow; a fixed-52-DIP-tall poster whose width follows
  the cover's aspect ratio (a 16:9 thumbnail gets a wider poster; the card
  height does not change); title and artist; a five-band meter delayed by
  the selected presentation latency; and a full-width progress row. Enabled
  transport controls (prev / play-pause / next) sit at the row's left as
  24-DIP circles in the session arrow's card style — card fill, muted
  12-DIP glyph, hover, 4 DIP apart, no filled primary — and call
  `nowplaying.Control`. The row then shows elapsed, cover-field fill, thumb,
  and remaining. A session with no timeline keeps the same row as an empty
  track: no fill, thumb, times, or copy. Refreshed every 2 s from Windows
  SMTC (`nowplaying.Read`); only controls the session reports enabled are
  drawn. The cover falls back to a note tile when the thumbnail is absent;
  the tile uses the card's own fill, so the first-artwork placeholder has no
  light edge.
  With no session at all the card is not drawn: the playback card keeps the
  volume, latency, and capture lines, and `fitFlyout` shrinks the flyout by
  the card's 150 DIP. The 正在播放 settings page gates the card: the display
  master, the playback controls, the receiver track info, the cover colors,
  and the audio level; master off hides the card and shrinks the flyout, and
  the receiver track info is effective only while the master is on. The
  settings preview shows the live card, or a sample when nothing plays. The 推送 switch is gone; changed tracks still forward
  as DMAP plus the cover image when the engine's `showNowPlaying` setting is on.
- Capture line: `采集 {endpoint}` with the rate formatted as kHz
  (`48 kHz`); 采集设备 follows the Windows default or pins a specific
  endpoint (a queued async restart supersedes the in-flight one); 丢帧
  shows only while streaming (zero while idle).
- Silence suspend: after 2 s of digital silence the pacer suspends (zero
  audio packets, sync/feedback/volume keep running); the first audible block
  resumes with a re-anchored RTP timeline and a 50 ms cushion trim
  (`silence.go:31-46`, resume `stream.go:76-98`).
- Diagnostics card: the 诊断日志 card opens a log dialog whose 复制 button
  puts the visible log on the clipboard (`flyout.go:449-481`).

## How to get to it (user POV)

Open the flyout from the tray icon. All controls are on the playback card.

## Driving it

Headless UI drives the real view functions:

```bash
go test -run 'TestViewRendersDevicesAndStatus|TestFlyoutLatencySegmentSelects|TestAudioModeTileSelects|TestNowPlayingCardShowsTrack|TestNowPlayingCardHidesWithoutMedia|TestNowPlayingPlaceholderStandsOutFromTheCard|TestNowPlayingCardControlsSendActions|TestNowPlayingArtworkRefreshes|TestNowPlayingSettingsPersist|TestNowPlayingPanelHiddenWhenOff|TestNowPlayingControlsSetting|TestNowPlayingCoverModes|TestNowPlayingCoverModePersists|TestCoverFieldCompressesLightness|TestCaptureMetaHidesDropsWhenIdle' -v ./internal/app/
go test -run 'TestMeterLevelBandsAndDelay|TestMeterGatesNoiseBelowTheFloor|TestMeterQuietToneKeepsBarsLow|TestFollowIgnoresSubBarChanges' -v ./internal/audio/analyze/
go test -run 'TestSilenceMeterFeedsTheLevelMeter' -v ./internal/engine/
SAUNDO_SMTC=1 go test -run TestSMTCLiveSessions -v ./internal/nowplaying/
SAUNDO_PREVIEW_DIR="$EV/frames" go test -run TestRenderPreviews ./internal/app/
```

Live app: connect (see discovery-and-connect), then move the slider and
confirm `SET_PARAMETER volume` answered 200 in the log; switch latency and
confirm one `streaming to` restart with the new `延迟` value on the ALAC
line; play a real 440 Hz WAV through the default device (Windows `Add-Type`
with `System.Media.SoundPlayer`, or any player) and confirm the capture line
and 丢帧 stay at 0. Start a media session in Chrome or Apple Music and
confirm the card picks up its app name, title, artist, cover, progress, and
available controls within two polls; use the transport controls in the
progress row, pressing play/pause only when changing the user's playback is
acceptable. While audio plays, confirm the five bars move with the beat and
settle to five dots when the session pauses. Switch between a square cover
and a 16:9 Chrome thumbnail and confirm the poster gets wider while the card
height stays fixed. A session with no timeline (some browser video tabs)
must show an empty progress track, not a status line or a fake fill. In the
设置 → 正在播放 page, turn the master off: the preview greys, the rows disable,
and the flyout card disappears and shortens; turn it on again and the card
returns. Close
every player and confirm the media card disappears entirely and the flyout
shrinks; the volume, latency, and capture lines remain, with no empty card,
status line, or reserved blank under the device card.

Proven end state: the ALAC line carries the selected latency, the slider
matches the receiver's own volume, and the capture line names the endpoint
that is actually rendering.

## Gotchas

- The progress-row transport controls change the user's real media playback;
  never press them in an automated check without asking.
- A no-timeline session's empty progress track is the design, not a failed
  read; do not expect a status line or a fill. With no session at all the
  card is absent and the flyout shrinks — do not look for a no-media card.
- The note tile on the first play is the artwork placeholder, not a broken
  image; it shares the card's fill and should show no edge.
- A player can expose a placeholder thumbnail first (Chrome's logo) and the
  real cover a poll later; the card must refresh when the thumbnail bytes
  change under the same session title.
- The meter reads the captured desktop, not the media session, so another
  app's sound moves it. It is delayed by the selected presentation latency
  (`Engine.LevelFrame`); a bar that trails the beat by the latency is the
  design, not a bug. It only animates with a real flyout window; a headless
  tester shows one static frame.
- SMTC app names are a fixed map; an unlisted AUMID shows 未知应用 while the
  session's title and art still render. Do not treat that as a read failure.
- A silent desktop proves nothing about audio content — WASAPI on a silent
  desktop still negotiates formats. Audible output is user-confirmed.
- Preset changes restart a live session; the app holds the last streaming
  snapshot up to 3 s so rows keep their streaming face with a spinner. A
  snapshot showing streaming during that window is the hold, not a second
  session.
- Latency custom values below 200 ms are reference-marked unverified; the
  SETUP clamps to 20–4000 ms only.
- The receiver's volume poll can move the slider under you while streaming —
  that is the two-way sync, not a fight between user input and the engine.
- Capture restarts are async: a quick endpoint switch queues and supersedes;
  two restarts in the log do not mean two capture streams.
- The sender never sends `SETRATEANCHORTIME` — the realtime stream takes
  its timeline from the UDP sync channel (protocol.md). If you exercise the
  method by hand, a HomePod answers plist `status: -12782` even with a
  well-formed anchor; the method is for buffered streams, and the reply is
  not an auth or formatting failure.
