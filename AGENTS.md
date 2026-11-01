# AGENTS.md

Operating guide for coding agents working in this repository. Read this file before changing code. All project documentation is written in English.

## Project

saundo is a Windows-only AirPlay 2 audio sender written in pure Go. It captures Windows system audio and streams it to HomePod, HomePod mini, Apple TV, and macOS AirPlay receivers.

Scope:

- Audio only. No video, screen mirroring, FairPlay, or AirPlay 1 in v1. A plaintext AP1 fallback exists for `_raop`-only receivers, but AP1 is not a target.
- Pure Go, single process, single binary. `CGO_ENABLED=0`.
- Native UI with mygo. No WebView.
- Distributed under GPL-3.0-only. See `LICENSE` and `NOTICE`.

Current status:

- Go 1.27.3 (the toolchain mygo requires). Module: `saundo`.
- Implemented: crypto core, HAP transient, PIN, and password pairing, AP2
  password Digest replay, encrypted RTSP, AP2
  session, event channel, NTP and PTP timing, uncompressed ALAC RTP, WASAPI
  loopback capture, mDNS discovery, native HomePod stereo pairs, system Now
  Playing display and metadata push, DPAPI credentials, settings, diagnostics,
  and the mygo tray flyout.
- Verified against real HomePods on software 27.2: pairing, PTP, a 35-second
  session with no drop, the full tray flyout and menu flow, audible playback
  of system audio confirmed by the user, and a two-member stereo pair streaming
  through the sender-supplied PTP master. A password-protected MacBook Air
  (Mac14,2) was verified on 2026-11-01: HAP password pairing, Digest replay on
  the AP2 session SETUP, PTP, and encrypted ALAC streaming. A Redmi TV
  advertising `AppleTV3,2` was tested and found to require Apple FairPlay audio;
  it is not supported, and discovery fails fast for that class. Apple TV live
  tests are pending. See `docs/verification.md`.

## Read first

- `docs/architecture.md`: current design, protocol flow, milestones, risks.
- `README.md`: project overview and references.

If the architecture document conflicts with the reference implementation or real-device behavior, follow the C++ reference and the device, then update the document.

## Commands

Make targets (run from Git Bash; `mingw32-make` works too):

```bash
make check    # gofmt gate + go vet + go test — the milestone gate
make test     # go test ./...
make race     # go test -race ./... (needs a C toolchain for the race detector)
make vet      # go vet ./...
make fmt      # gofmt -w .
make build    # CGO_ENABLED=0 build into bin/saundo.exe, GUI subsystem, version stamped from git describe; scripts/build.sh then stops the running app and starts the new build
```

Without make, run the commands directly:

```bash
go test ./...
go vet ./...
gofmt -l .
```

If `go` is not on PATH, use `C:\Program Files\Go\bin\go.exe`, add that directory to PATH, or pass it to make: `make GO="C:\Program Files\Go\bin\go.exe" check`.

Run `make check` (or all three commands) before declaring a milestone complete. A compiling binary is not a completed feature.

## Architecture rules

- Keep the engine UI-agnostic. The UI talks to an `Engine` interface and channels. No IPC and no sidecar.
- Keep the sender sans-i/o, following `airplay2-sender-cpp`: `RaopIo` has six methods (TCP connect/send/close, UDP bind/send/close). The sender owns no sockets, timers, or threads; the host pushes data, connection events, and ticks into it.
- One sender event-loop goroutine owns all protocol sockets and timers. Capture runs in its own goroutine. The UI runs on mygo's main thread. Do not share mutable state; use bounded queues and command channels.
- ALAC is mandatory on the realtime stream. Send uncompressed ALAC frames, never PCM. Use `internal/airplay/alac.EncodeUncompressed`.
- Use NTP for receivers that accept it. A HomePod on software 27 and a macOS
  AirPlay receiver keep their own PTP grandmaster and silently ignore an
  NTP-timed stream: when feature bit 41 is set and the model starts with
  `AudioAccessory` or `Mac`, run `internal/airplay/ptp.Follower`, advertise PTP
  in the session SETUP, send SETPEERS, wait for the clock to be ready, and use
  the 28-byte PTP sync packet. Keep timing behind the sender's `PTPClock`
  interface.
- Do not use stale critical-path libraries such as `go-wca` or `grandcat/zeroconf`. Use `hashicorp/mdns` for discovery and write the WASAPI binding yourself.
- Store credentials with DPAPI. Never log PINs, long-term keys, or session keys.

## Protocol invariants

These are the failure modes that produce "connected but silent" or a drop after 25 to 30 seconds.

1. Encrypt the RTSP control channel immediately after pair-verify. Frame: `[2-byte little-endian length][ciphertext][16-byte tag]`, chunks up to 1024 bytes, AAD = length prefix, nonce = 4 zero bytes plus an 8-byte little-endian counter, separate send and receive counters.
2. AP2 order: `GET /info` then `SETUP rtsp://host/sessionId` (session plist, `timingProtocol: NTP` or `PTP`), read `eventPort`, connect the event channel, send `RECORD`, send the stream `SETUP` (ALAC plist), then stream. A password-protected Mac answers the session SETUP with 401 plus `WWW-Authenticate: Digest realm="airplay"`; replay it with RFC 2069 (username `AirPlay`, no qop).
3. The event channel is a reverse encrypted TCP connection. Decrypt receiver `POST /command` events and reply with a minimal `RTSP/1.0 200 OK`. The response may contain only `Server` and `CSeq`. Do not add `Content-Length`, `Audio-Latency`, or any other header.
4. Send `POST /feedback RTSP/1.0` with `CSeq`, `User-Agent`, `DACP-ID`, `Active-Remote`, `Client-Instance`, and `Content-Length: 0`. AirPlay 2 sends it every 2 seconds (pyatv `FEEDBACK_INTERVAL`, verified against HomePod 27); the AirPlay 1 probe repeats every 25 seconds when the receiver answers 200. Feedback is not the keep-alive; the event channel is.
5. Audio key sources: HomePod uses the first 32 bytes of `sharedSecret` with no HKDF (transient pairing produces a 64-byte SRP K; truncate it); a password-paired Mac uses the Events-Write key as `shk` (pyatv's macOS profile). Control and event keys use HKDF over the full shared secret.
6. Stream SETUP: `audioFormat 0x40000`, `ct 2`, `type 0x60`, `spf 352`, `sr 44100`, `shk` = 32-byte audio key, `streamConnectionID` = numeric session id. A macOS receiver uses `latencyMin 11025`, `latencyMax 88200`, and a 66150-sample scheduling latency; HomePod uses the configured latency.
7. RTP header is 12 bytes. The first packet uses marker `0xE0`; later packets use `0x60`. AAD is RTP header bytes 4 through 11. Append the 8-byte nonce after the ciphertext. Pace at 44100 frames per second with a token bucket; while a stream is active, push silence when the ring is empty; keep a backlog for `0x55` retransmit requests. After 2 s of digital silence, suspend the pacer and send no RTP, keeping sync, feedback, volume polling, and the event channel alive; resume by re-anchoring the RTP time to the wall clock and restarting from the 50 ms cushion. `SETRATEANCHORTIME` is ignored on HomePod realtime streams (plist `status` -12782). Do not suspend AirPlay 1 sessions; they keep the continuous silence stream.
8. Pairing paths: Apple TV uses `POST /pair-pin-start` with `X-Apple-HKP: 3`, then M1 through M6 and pair-verify. HomePod uses transient pairing: M1 with `Flags=0x10`, fixed PIN `3939`, stop at M4. A Mac with Require Password uses HAP pair-setup method 0 with the receiver password as the SRP secret: no Flags, no `/pair-pin-start`, then M5/M6 and pair-verify; its AP2 session SETUP may answer 401 Digest, replayed with username `AirPlay`. The app prompts for that password on first connect with a masked dialog, saves it, and reconnects; the settings page can change it. Persist `{"atvId","clientId","ltpk","ltsk"}` with DPAPI.
9. HomePod software 27 and macOS receivers need PTP. Session SETUP carries `timingProtocol: PTP`, `timingPeerInfo`, and `timingPeerList`; after RECORD, send SETPEERS and activate the follower. The stream SETUP waits until the clock is ready. Sync packets are 28 bytes: `0x80/0x90, 0xD7, be16 6, be32 rtp, be64 ptpTime, be32 rtp-latency, be64 clockID`, every 100 ms. HomePod uses `latencyMin` and `latencyMax` 8820; a Mac uses 11025 and 88200 with a 66150-sample scheduling latency. Bind UDP 319 and 320; the receiver is the grandmaster and its PTP clock is boot-relative.
10. A HomePod stereo pair is not a single leader session. Group members share `tsid`; `igl=1` marks the leader. Open one AP2 session per member, send the same `groupUUID` (`tsid`) in each session SETUP, put every member address plus the sender in SETPEERS, and run the sender as PTP grandmaster for the pair. Standalone HomePod OS 27 keeps the follower role. Capture fans out to one ring per member; the group launches only after both members stream.

## References

Clone them outside the repository and do not commit them:

```bash
git clone --depth 1 https://github.com/akustikrausch/airplay2-sender-cpp
git clone --depth 1 https://github.com/postlund/pyatv
git clone --depth 1 https://github.com/Ding-Kyoma/AirFlash
```

- `airplay2-sender-cpp` (Apache-2.0) is the primary blueprint. Read its README recipe, `src/airplay_crypto.*`, `src/raop_sender.*`, `src/raop_io.h`, `src/raop_loop.cpp`, `src/ring_buffer.h`, `src/raop_creds.h`, and `test/core_tests.cpp`.
- `pyatv` (MIT) is the secondary reference for RAOP transport semantics, HAP pairing, and test vectors.
- `AirFlash` (GPLv3+, dual-licensed) is a code reference for WASAPI capture, PTP, HomePod OS 27 behavior, HAP, RTP, and session details. This project uses the GPLv3 option, so direct porting is allowed. Record the source in the file header and in `NOTICE`.
- UI references: `mygo-cloudmusic` for native UI structure and style, and Tuneblade for device-list, volume, and connection-status layout. Tuneblade is closed-source; reference its UI only, never its code.

## Code standards

Error handling:

- A failed read, parse, or decode means "unknown" — never "empty", "zero", or "absent". Where a previous value exists, keep it.
- Gate decisions fail closed: unknown receiver features, unknown auth state, or unreadable credentials never silently take the permissive path. For example, a failed features parse must not route an `AudioAccessory` receiver to NTP.

Tests:

- Every bug fix ships a test that fails without the fix. A test that already fails on the current tree is a bug; fix it at its cause.
- Test fixtures live in the owning package's `testdata/` directory: captured device logs, packet dumps, bplist blobs, saved transcripts. Tests read fixtures from `testdata/`, never from the network or from outside the repository. The go tool ignores `testdata/` when scanning packages.

Reporting:

- State which checks ran, what they printed, and which did not run. An unrun check is listed as not run.

## Testing and workflow

- Work one milestone at a time: M0 crypto core, M1 sender plus fake receiver, M2 Go net host and real device, M3 capture and UI, M4 polish.
- Build the fake receiver from `test/core_tests.cpp` before testing real hardware. It should drive the full handshake and byte-compare decrypted ALAC audio without network access.
- After each milestone run `go test ./...`, `go vet ./...`, and `gofmt -l .`. Report what is done and what is not.
- When real-device behavior differs, capture logs and packets first, then change code, and record the finding in `docs/architecture.md`.
- Ask the user only for genuine product decisions. Proceed on reversible engineering work.
- Do not commit unless the user explicitly asks.
- Keep all documentation in English.

## Definition of done for v1

A Windows user can discover a HomePod, Apple TV, or Mac, pair when required, stream system audio over AirPlay 2 with ALAC, adjust volume, disconnect and reconnect, and use the tray flyout and menu. No video, no FairPlay, no AirPlay 1.
