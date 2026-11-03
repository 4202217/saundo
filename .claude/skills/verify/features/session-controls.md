# Session controls

The flyout's playback area: volume, latency segments, Now Playing row,
capture status, and the diagnostics log card.

## Sub-features

- Volume slider: sends `SET_PARAMETER volume` while streaming; 0 mutes
  (the `-144` dBFS sentinel); a remembered device reconnects at its stored
  volume; a first connection adopts the receiver's `initialVolume`. While
  streaming, the sender polls `GET /info` every second — a knob moved on the
  receiver itself mirrors back into the slider.
- Latency: flyout segments 低延迟 200 ms (default) / 标准 1.5 s / 稳定 2 s /
  自定义 20–2000 ms; the value shows on the ALAC line
  (`延迟 700 ms`). Preset clicks debounce the session restart 300 ms,
  the custom slider 500 ms; the effective value is base + the receiver's
  DSP/DAC delay, clamped 20–4000 ms. The 面板快捷切换 header switch hides the
  segment row.
- Now Playing row: `title · artist`, falls back to 未检测到媒体, refreshes
  every 2 s from Windows SMTC; the 推送 switch forwards changed tracks as
  DMAP only while streaming.
- Capture line: `采集：<endpoint> · <rate> Hz`; 采集设备 follows the Windows
  default or pins a specific endpoint (a queued async restart supersedes the
  in-flight one); 丢帧 shows only while streaming (zero while idle).
- Silence suspend: after 2 s of digital silence the pacer suspends (zero
  audio packets, sync/feedback/volume keep running); the first audible block
  resumes with a re-anchored RTP timeline and a 50 ms cushion trim.
- Diagnostics card: rolling log; 复制日志 puts the visible log on the
  clipboard.

## How to get to it (user POV)

Open the flyout from the tray icon. All controls are on the playback card.

## Driving it

Headless UI drives the real view functions:

```bash
go test -run 'TestViewRendersDevicesAndStatus|TestFlyoutLatencySegmentSelects|TestAudioModeTileSelects|TestNowPlayingRowShowsTrack|TestCaptureMetaHidesDropsWhenIdle' -v ./internal/app/
SAUNDO_PREVIEW_DIR="$EV/frames" go test -run TestRenderPreviews ./internal/app/
```

Live app: connect (see discovery-and-connect), then move the slider and
confirm `SET_PARAMETER volume` answered 200 in the log; switch latency and
confirm one `streaming to` restart with the new `延迟` value on the ALAC
line; play a real 440 Hz WAV through the default device (Windows `Add-Type`
with `System.Media.SoundPlayer`, or any player) and confirm the capture line
and 丢帧 stay at 0.

Proven end state: the ALAC line carries the selected latency, the slider
matches the receiver's own volume, and the capture line names the endpoint
that is actually rendering.

## Gotchas

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
- `SETRATEANCHORTIME rate=0` returns status `-12782` from HomePods even on
  success paths — the realtime stream takes timing from the sync channel.
