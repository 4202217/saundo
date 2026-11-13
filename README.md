<div align="center">

<a href="https://github.com/4202217/saundo"><img src="resources/icon.png" width="120" alt="The saundo icon: a smiling face on a rounded dark tile"></a>

# saundo · 声渡

### Stream all your Mac's or PC's audio to AirPlay 2 receivers — from the tray, in pure Go.

**声渡 (Shēngdù) — "ferry the sound": 声, sound; 渡, to ferry across.**<br>
**saundo** is サウンド, the Japanese katakana rendering of "sound".<br>
HomePod, HomePod mini, Mac, and Apple TV: everything the Mac or PC plays, one tray click away.

[![Listed on Awesome MyGo](https://awesomemygo.dev/badge.svg)](https://awesomemygo.dev/apps/saundo) 

**[Download](https://github.com/4202217/saundo/releases/latest)** &nbsp;·&nbsp; **[Issues](https://github.com/4202217/saundo/issues)** &nbsp;·&nbsp; **[Docs](docs/verification.md)** &nbsp;·&nbsp; **English** · [简体中文](README.zh-CN.md)

<br>

<p align="center">
  <img src="docs/screenshots/flyout-stream-light-en.png" width="290" alt="The Saundo tray panel: receiver list, media card, latency, volume, and ambient light">
  &nbsp;&nbsp;&nbsp;
  <img src="docs/screenshots/settings-connection-light-en.png" width="470" alt="The settings window: connection page">
</p>

</div>

<br>

## Is saundo for you?

saundo captures whatever the system is playing — WASAPI loopback on Windows, a Core Audio process tap on macOS — and streams it as uncompressed ALAC over AirPlay 2 to HomePod, HomePod mini, Mac receivers, and Apple TV. One tray icon, one binary, no WebView:

- You play audio on your Mac or PC and want it on a HomePod on the same network.
- You want **all** system audio — a browser, a game, a player — not one app's stream.
- You prefer a native tray app over a web view.

Not for you if you need video or screen mirroring (this is audio only), or if your receiver requires Apple FairPlay — some TVs do. saundo detects that class of receiver and fails fast instead of connecting silently.

Runs on Windows 10 and 11 and on macOS 14.4+ (Apple Silicon); the interface ships in English and Simplified Chinese and follows the system language by default. macOS builds ship as a dmg with an ad-hoc signed app: drag the app to Applications, then pass the first launch through System Settings → Privacy & Security → Open Anyway. See [docs/platforms.md](docs/platforms.md).

## Quick start

**1 · Install**. Grab the installer from **[Releases](https://github.com/4202217/saundo/releases/latest)**, or build from source:

```bash
mingw32-make check  # gofmt gate, go vet, mygo UI vet, go test — the acceptance gate
mingw32-make build  # builds bin/saundo.exe and an NSIS installer (bin/saundo-setup-*.exe) through mygo, then restarts the running app
```

<sub>The installer is not code-signed yet, so SmartScreen warns. Building from source needs Go 1.27+ (the toolchain [mygo](https://github.com/egoist/mygo) requires) and `mingw32-make` from MinGW-w64; then run `bin/saundo-setup-*.exe`. On macOS, `make check` and `make build` run the same gate and produce `bin/saundo.app`; cmake builds the Now Playing adapter.</sub>

**2 · Connect**. The app lives in the tray: a left click opens the panel, a right click opens the menu. Pick a receiver and connect; one that requires a password asks once and remembers it.

**3 · Play**. Play anything on the Mac or PC. The sound follows, and the volume slider drives the receiver's own volume.

<sub>Windows Firewall prompts on first launch — allow it on private networks: discovery (mDNS) and PTP timing both need inbound UDP. On macOS, allow the app under Privacy & Security → Local Network for the same reason.</sub>

## What it does

- **System audio capture** — everything the Mac or PC plays: WASAPI loopback on Windows, a Core Audio process tap on macOS, or, on macOS, one or more selected apps.
- **AirPlay 2, properly timed** — uncompressed ALAC at 44.1 kHz, PTP-locked to the receiver's clock, encrypted control and event channels, retransmit on request. HomePod stereo pairs play in lockstep.
- **Receivers, managed** — find them with mDNS or add an IP; keep them across power-offs; hide rows; choose one auto-connect target and set a per-receiver delay and password. Switch receivers from the panel or its right-click menu without stopping playback.
- **One tray surface** — the panel carries the media card, latency, volume, and ambient light; the right-click menu carries receivers, local mute, the lamp, settings, updates, and quit. Animated faces follow the session.
- **Ambient light** — a HomePod's top display follows the media cover or one of six three-color templates, and turns off after a chosen idle period.
- **Mute the local output** — silences this computer while streaming and restores it on disconnect. With per-app capture on macOS, only the selected apps go quiet.
- **Latency modes** — 200 ms, 1.5 s, 2 s, or a custom 20–2000 ms; switch while playing.
- **Sessions that recover** — auto-reconnect with backoff, silence suspend, takeover handling, and receiver volume following.
- **Updates, languages, pairing** — the settings update page installs signed updates on both platforms; English and Simplified Chinese; HomePod, Apple TV PIN, and receiver-password pairing with OS-secured credentials.

## Receiver support

| Receiver | Status |
|---|---|
| HomePod / HomePod mini (software 27) | Verified — PTP-timed sessions |
| HomePod stereo pair | Verified — the sender acts as the PTP master |
| Mac, AirPlay Receiver (incl. Require Password) | Verified — password pairing |
| Apple TV, on-screen PIN | Implemented; not yet verified on hardware |
| Receivers that require Apple FairPlay (some TVs) | Not supported — detected at discovery, fails fast |

How each family advertises, pairs, and behaves: [docs/devices.md](docs/devices.md). The failure-mode table (connected-but-silent, pairing 403/470, firewall) and the live-device checklist: [docs/verification.md](docs/verification.md).

## How it fits together

The tray panel talks to a UI-agnostic engine, which drives a sans-i/o sender state machine over encrypted RTSP. WASAPI loopback (Windows) or the Core Audio process tap (macOS) feeds a resampler and a ring buffer; a PTP follower (or master, for stereo pairs) keeps every stream on the receiver's clock. Design decisions and risks: [docs/architecture.md](docs/architecture.md). The wire-level protocol: [docs/protocol.md](docs/protocol.md). The code map and platform seams: [docs/implementation.md](docs/implementation.md) and [docs/platforms.md](docs/platforms.md).

## Documentation

- [docs/architecture.md](docs/architecture.md) — design decisions, technology choices, risks
- [docs/protocol.md](docs/protocol.md) — the wire-level protocol: pairing, control channel, AP2 session, RTP, timing
- [docs/devices.md](docs/devices.md) — per-receiver profiles: how each family advertises, pairs, and behaves
- [docs/ui.md](docs/ui.md) — the UI design language: tokens, widgets, motion, windows
- [docs/implementation.md](docs/implementation.md) — the code map: runtime model, packages, flows, build
- [docs/platforms.md](docs/platforms.md) — the Windows/macOS seams, feature parity, packaging
- [docs/verification.md](docs/verification.md) — automated checks, live-test recipes, the evidence log
- [docs/releasing.md](docs/releasing.md) — the release checklist, signing keys, and rollback
- [SECURITY.md](SECURITY.md) — vulnerability reporting
- [AGENTS.md](AGENTS.md) — operating guide for coding agents

## References

- [airplay2-sender-cpp](https://github.com/akustikrausch/airplay2-sender-cpp) — primary blueprint (Apache-2.0)
- [pyatv](https://github.com/postlund/pyatv) — RAOP transport semantics (MIT)
- [AirFlash](https://github.com/Ding-Kyoma/AirFlash) — WASAPI and PTP code reference, ported under the GPLv3 option
- [magpie](https://github.com/yetone/magpie) — tray panel and settings design reference

## License

saundo is free software under GPL-3.0-only. See [LICENSE](LICENSE) for the license text and [NOTICE](NOTICE) for third-party attributions.
