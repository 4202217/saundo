# saundo

A pure-Go AirPlay 2 audio sender for Windows. It captures Windows system audio and streams it to HomePod, HomePod mini, Apple TV, and macOS AirPlay receivers.

Audio only. No video or screen mirroring. saundo is free software under
GPL-3.0-only.

## Status

- Architecture: [docs/architecture.md](docs/architecture.md)
- Verification and real-device checklist: [docs/verification.md](docs/verification.md)
- Implemented: HAP pairing (transient, on-screen PIN, and receiver password),
  AP2 Digest replay, encrypted RTSP control, AP2 session setup, event-channel
  keep-alive, NTP and PTP timing, uncompressed ALAC RTP streaming, retransmit,
  WASAPI loopback capture, mDNS discovery,
  native HomePod stereo pairs, system Now Playing display and metadata push,
  DPAPI credentials, JSON settings, and the mygo tray flyout.
- Verified in the automated suite: the crypto core, the full sender handshake and
  audio path against a fake receiver, discovery parsing, and WASAPI endpoint
  opening on Windows.
- Verified on real hardware on 2026-10-31: two HomePods (software 27.2) were
  discovered over mDNS, paired with HAP transient pairing, and streamed with
  ALAC over a PTP-timed session for 35 seconds without a drop. A four-second
  440 Hz tone was sent to one HomePod and its original volume was restored.
  The application was then driven through its real window: connect,
  disconnect, reconnect, volume, and hide-to-tray with tray-icon restore all
  worked, and the stream stayed up for more than six minutes. A later run
  played a real system WAV through WASAPI loopback to the HomePod while the app
  was streaming, and the user confirmed hearing the four-second tone on
  HomePod L. The same pair then appeared as one `HomePods` stereo-group target:
  both member sessions reached streaming through the sender-supplied PTP
  master, a four-second system tone caused zero drops, and the app window showed
  `正在播放到 HomePods` with PTP synchronized. The playback card also read a
  live Chrome Media Session and displayed its track. On 2026-11-01 a
  password-protected MacBook Air (Mac14,2) streamed through HAP password
  pairing, an RFC 2069 Digest replay on the AP2 session SETUP, PTP, and
  encrypted ALAC. A Redmi TV advertising `AppleTV3,2` was tested and requires
  Apple FairPlay audio, which this sender does not implement; discovery fails
  fast for that class. Apple TV PIN pairing is still pending; use
  [docs/verification.md](docs/verification.md).

## Engine references

- [airplay2-sender-cpp](https://github.com/akustikrausch/airplay2-sender-cpp)
- [pyatv](https://github.com/postlund/pyatv)
- [AirFlash](https://github.com/Ding-Kyoma/AirFlash): its Rust engine is a code reference, ported under the GPLv3 option.

## UI references

- [mygo-cloudmusic](https://github.com/egoist/mygo-cloudmusic): native UI structure and style.
- [Tuneblade](http://www.tuneblade.com/): layout reference for the device list, volume, and connection status.

## Build

Requires Go 1.27+.

```bash
go test ./...
```

## License

saundo is free software: you can redistribute it and/or modify it under the
terms of the GNU General Public License, version 3. See [LICENSE](LICENSE) for
the license text and [NOTICE](NOTICE) for third-party attributions.

## Agents

See [AGENTS.md](AGENTS.md) for the operating guide. All documentation is written in English.
