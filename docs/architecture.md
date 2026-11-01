# saundo Architecture and Technology Selection

## 0. Goals and scope

Build a Windows desktop application that streams Windows system audio over AirPlay 2 in real time to HomePod, HomePod mini, Apple TV, and macOS AirPlay receivers.

Constraints:

- Pure Go, single process, single binary.
- AirPlay audio only. No AirPlay video, no FairPlay.
- Distributed under GPL-3.0-only. Apache-2.0, MIT, and GPL reference code may be read and ported, including the AirFlash Rust engine.
- Correctness and verifiability over speed.
- mygo native UI. WASAPI loopback for capture.

Not in v1:

- Screen mirroring and video.
- Multi-room sync and HomePod stereo pairs.
- Application-level audio capture.
- AirPlay 1 RAOP. AP1 is a later compatibility layer.

## 1. Decision

| Layer | Choice | License | Notes |
|---|---|---|---|
| Language | Go 1.27+ | BSD-3-Clause | Pure Go, `CGO_ENABLED=0` |
| UI | mygo native UI | MIT | D3D11 rendering, tray and installer |
| Primary reference | airplay2-sender-cpp | Apache-2.0 | About 7k lines, sans-i/o, verified against Apple TV, HomePod, and macOS in its shipping product |
| Secondary reference | pyatv | MIT | RAOP transport semantics, HAP pairing, test vectors |
| Code reference | AirFlash Rust engine | GPLv3+ | WASAPI capture, PTP, HomePod OS 27, HAP/RTP/session; ported under the GPLv3 option |
| Crypto | Go standard library + x/crypto | BSD-3-Clause | SHA-512, HMAC, HKDF, ChaCha20-Poly1305, X25519, Ed25519 |
| SRP | In-house | Standard library | SRP-6a 3072-bit, SHA-512, client side |
| TLV8 | In-house | Standard library | HAP pairing, 255-byte fragmentation |
| bplist | In-house | Standard library | Only the types used by AirPlay SETUP |
| Audio codec | In-house uncompressed ALAC | Standard library | No full ALAC encoder needed |
| Discovery | hashicorp/mdns | MIT | Actively maintained; Windows DNS-SD fallback |
| Capture | In-house WASAPI binding | Standard library | Reference AirFlash `wasapi.rs` and mygo's WASAPI output |
| Timing | NTP + PTP follower | Standard library | NTP for Apple TV and macOS; PTP follower for HomePod software 27, which keeps its own grandmaster |
| Credentials | Windows DPAPI | OS API | Through syscall |

Rejected:

- No AirFlash Rust engine as a runtime sidecar. Pure Go rewrite, but its Rust code may be ported as an implementation reference.
- No C#/WPF, no Electron/Tauri.
- No stale Go libraries such as go-wca or zeroconf on the critical path.
- No pyatv PCM path. Modern receivers require ALAC.

## 2. Reference split

### airplay2-sender-cpp, primary blueprint

Apache-2.0, last commit 2026-09-05. It is the closest implementation to the goal. About 7k lines, roughly 3k core and 1.9k tests. It is a sans-i/o state machine that owns no socket, timer, or thread; it talks to the host through the six-method `RaopIo` interface. This maps well to Go.

Port map:

| C++ | Content | Go target |
|---|---|---|
| `src/airplay_crypto.cpp` | SRP-6a, TLV8, bplist, ChaCha20-Poly1305, HKDF, X25519, Ed25519, digest auth | `internal/airplay/crypto` |
| `src/raop_sender.cpp` | Pairing, encrypted control channel, event channel, AP2 SETUP, ALAC, pacing, retransmit | `internal/airplay/sender` |
| `src/raop_io.h` | Six-method I/O interface | `internal/airplay/io` |
| `src/raop_loop.cpp` | poll/WSAPoll host | `internal/airplay/host`, rewritten with Go net |
| `src/ring_buffer.h` | Single-producer single-consumer ring buffer | `internal/audio/ring` |
| `src/raop_creds.h` | Credential JSON | `internal/airplay/creds` |
| `test/core_tests.cpp` | In-process fake receiver covering both handshakes and encrypted audio | Go tests in `internal/airplay/sender` |

It reports verification of:

- Apple TV 4K PIN pairing and the encrypted control channel.
- HomePod and macOS transient pairing.
- Realtime streams must use ALAC; receivers ignore `ct` and `audioFormat`.

The standalone build's real-hardware pass is still listed on its roadmap. The same sender code ships in a real product.

### pyatv, secondary blueprint

MIT, last commit 2026-06-19. `raop/protocols/airplayv2.py` is the AirPlay 2 audio sender reference, including HAP pair-verify, encrypted RTSP, the `shk` audio key, ChaCha20 with an 8-byte nonce, feedback, and the NTP timing server. The C++ implementation also credits pyatv as the RAOP transport source. Use pyatv to cross-check fields and flow.

Note: pyatv uses PCM (`ct=1`); the C++ blueprint uses ALAC (`ct=2`). Modern receivers require ALAC.

### AirFlash, code reference

GPLv3+ (dual-licensed with a commercial option), last commit 2026-10-29. It uses PTP and PCM/ALAC and supports HomePod OS 27, stereo pairs, EQ, and WASAPI application capture. saundo uses the GPLv3 option, so the combined work is distributed under GPL-3.0-only. The AirFlash Rust engine is a code reference; port `wasapi.rs`, `clock.rs`, `session.rs`, `transport.rs`, `auth.rs`, `rtsp.rs`, `rtp.rs`, `credentials.rs`, `buffered.rs`, `profile.rs`, `receiver_capabilities.rs`, `main.rs`, and related modules as needed. Record the source in the file header and in NOTICE. The Go implementation still follows its own package structure.

### Unused references

- goplay2, Apache-2.0, unmaintained since 2023. It is a receiver with a PulseAudio/portaudio stack; useful only for the receiver perspective.
- UxPlay, GPLv3, receiver, not relevant to a sender.
- airplay2-rs, GPLv3, alpha, useful as a behavior reference; the primary code references remain C++ and AirFlash.

## 3. System architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     mygo native UI / Tray                    │
│  device list  pairing PIN  volume  settings window  diag     │
└──────────────────────────┬──────────────────────────────────┘
                           │ method calls / events
┌──────────────────────────▼──────────────────────────────────┐
│                     Session Controller                      │
│  Idle → Connecting → Pairing → Handshake → Streaming        │
└───────┬───────────────┬───────────────────┬─────────────────┘
        │               │                   │
┌───────▼──────┐ ┌──────▼───────┐ ┌─────────▼───────────────┐
│ Discovery    │ │ Pairing      │ │ Sender                  │
│ mDNS browse  │ │ HAP SRP      │ │ RTSP control            │
│ TXT /info    │ │ pair-verify  │ │ AP2 SETUP / event       │
└──────────────┘ └──────────────┘ │ ALAC RTP / pacing       │
                                  └─────────┬───────────────┘
                                            │
┌───────────────┐   ┌──────────────┐  ┌─────▼──────────────┐
│ Capture       │──▶│ Resample     │─▶│ Ring Buffer        │
│ WASAPI        │   │ 48k → 44.1k  │  │ 352 frames/packet  │
│ loopback      │   │ s16le stereo │  └─────┬──────────────┘
└───────────────┘   └──────────────┘        │
                                    ┌────────▼────────┐
                                    │ AirPlay receiver│
                                    │ HomePod / ATV   │
                                    └─────────────────┘

Cross-cutting: Settings (JSON), Credentials (DPAPI), Diagnostics (logs and metrics)
```

There are three concurrency roles:

- The mygo UI main thread.
- The sender event-loop goroutine, which owns all protocol sockets and timers.
- The capture goroutine, which owns WASAPI capture and ring-buffer writes.

They interact through bounded queues and command channels. No mutable state is shared.

## 4. Modules and directories

```
saundo/
├── cmd/saundo/main.go
├── internal/
│   ├── app/                  mygo windows, tray, views, pairing dialog
│   ├── discovery/            mDNS, TXT parsing, device aggregation
│   ├── capture/              WASAPI loopback, device enumeration, default-device following
│   ├── audio/
│   │   ├── resample/         48k → 44.1k, linear or polyphase
│   │   └── ring/             SPSC ring buffer
│   ├── airplay/
│   │   ├── crypto/           SHA-512, HKDF, ChaCha20, X25519, Ed25519, SRP, TLV8, bplist
│   │   ├── alac/             uncompressed ALAC frame encoder
│   │   ├── creds/            credential JSON and DPAPI
│   │   ├── device/           receiver description and auth enum
│   │   ├── io/               TCP/UDP interfaces and Go net host
│   │   ├── ptp/              PTP follower for HomePod software 27
│   │   └── sender/           state machine, RTSP, pairing, event channel, ALAC, pacing
│   ├── engine/               session controller, discovery/capture/host wiring
│   ├── settings/             JSON configuration
│   └── diagnostics/          rolling log and counters
├── resources/                icons, tray assets, NOTICE
├── docs/architecture.md
└── go.mod
```

## 5. Protocol flow

The order below follows the C++ blueprint. A wrong order usually produces "connected but silent" or a drop after 30 seconds.

### 5.1 Discovery

Browse mDNS for `_raop._tcp` and `_airplay._tcp`. Parse `deviceid`, `features`, `sf`, `pw`, `am`, and `model` from TXT records. Use `sf` and `features` to choose authentication:

- `pw=true`: password authentication.
- `am=AirPort*`: auth-setup.
- HomeKit bit: Apple TV uses on-screen PIN; HomePod and macOS use transient.
- No auth bit: plain RTSP.

The C++ blueprint has no mDNS and only accepts an IP address. The Go side adds discovery before calling the sender.

### 5.2 Pairing

Two paths.

**HAP transient, HomePod and macOS.**

1. `POST /pair-setup`, TLV8: Method=0, State=1, Flags=0x10.
2. Receive the M2 salt and B.
3. `POST /pair-setup`, State=3, PublicKey=A, Proof=M1, fixed PIN `3939`.
4. Receive the M4 server proof, verify it, and stop at M4.
5. `sharedSecret = SRP K = SHA-512(S)`, 64 bytes.

**HAP PIN, Apple TV.**

1. `POST /pair-pin-start` with `X-Apple-HKP: 3` so the Apple TV displays a 4-digit PIN.
2. M1 through M4; the user enters the PIN.
3. M5 and M6 exchange long-term keys. M5's inner TLV8 is encrypted with an HKDF-derived session key using nonce `PS-Msg05`; M6 decrypts with `PS-Msg06`.
4. `pair-verify` M1 through M3, X25519 plus Ed25519 signatures, nonces `PV-Msg02` and `PV-Msg03`.
5. `sharedSecret = X25519 ECDH`, 32 bytes.

Long-term credentials:

```json
{"atvId":"<hex>","clientId":"<uuid>","ltpk":"<hex>","ltsk":"<hex>"}
```

Encrypt with DPAPI and index by device `deviceid`.

**Audio key.** Both `shk` and the audio ChaCha20 key are the first 32 bytes of `sharedSecret`, with no HKDF. The transient K is 64 bytes and must be truncated to 32. Control and event keys still use HKDF over the full `sharedSecret`.

### 5.3 Encrypted control channel

After `pair-verify`, switch the control channel to ChaCha20-Poly1305 immediately. Apple TV rejects plaintext SETUP.

Each frame:

```
[2-byte little-endian length][ciphertext][16-byte tag]
```

- Chunks up to 1024 bytes.
- AAD is the 2-byte length prefix.
- Nonce is 4 zero bytes plus an 8-byte little-endian counter.
- Separate counters for send and receive.
- Control keys: HKDF-SHA512("Control-Salt", "Control-Write-Encryption-Key" / "Control-Read-Encryption-Key", sharedSecret, 32).

### 5.4 AP2 session

1. `GET /info` on the encrypted control channel.
2. `SETUP rtsp://host/sessionId` with a binary plist body: `deviceID`, `sessionUUID`, `timingPort`, `timingProtocol: NTP`, `isMultiSelectAirPlay`, `groupContainsGroupLeader`, `macAddress`, `model`, `name`, `osBuildVersion`, `osName`, `osVersion`, `senderSupportsRelay`, `sourceVersion`, `statsCollectionEnabled`.
3. The response contains `eventPort`.
4. Connect the event channel TCP; the receiver initiates the reverse connection.
5. Send `RECORD` with an empty body and standard RTSP headers. It goes after session SETUP and before stream SETUP.
6. `SETUP` stream with a binary plist body:

| Field | Value |
|---|---|
| `audioFormat` | `0x40000`, ALAC/44100/16/2 |
| `audioMode` | `default` |
| `controlPort` | Local control UDP port |
| `ct` | `2`, ALAC |
| `isMedia` | `true` |
| `latencyMax` | `88200` for macOS; the configured latency for HomePod |
| `latencyMin` | `11025` for macOS; the configured latency for HomePod |
| `shk` | HomePod: first 32 bytes of `sharedSecret`; macOS: Events-Write key; open: 32 random bytes |
| `spf` | `352` |
| `sr` | `44100` |
| `type` | `0x60` |
| `supportsDynamicStreamID` | `false` |
| `streamConnectionID` | Numeric session id |

The response provides `dataPort` and `controlPort`. Do not send another RECORD; start streaming.

A macOS receiver uses the same table with its `shk`/latency values and the PTP timing path below.

### 5.5 Event channel and keep-alive

The event channel is a reverse encrypted TCP connection. The receiver pushes events such as `POST /command` and `updateInfo`. Decrypt each event and reply with a minimal 200 OK, or Apple TV tears down the session after about 25 to 30 seconds.

The response must be:

```
RTSP/1.0 200 OK\r\n
Server: AirTunes/220.68\r\n
CSeq: <n>\r\n
\r\n
```

Do not add `Content-Length`, `Audio-Latency`, or any other header. One extra header corrupts the receiver timeline, which looks like a healthy connection with no audio.

Event keys: HKDF-SHA512("Events-Salt", "Events-Write-Encryption-Key" / "Events-Read-Encryption-Key", sharedSecret, 32). Because the connection is reversed, swap the receive and send keys.

Also send `POST /feedback RTSP/1.0` with `CSeq`, `User-Agent`, `DACP-ID`,
`Active-Remote`, `Client-Instance`, and `Content-Length: 0`. The AirPlay 2
interval is 2 seconds, matching pyatv's `FEEDBACK_INTERVAL` and the C++
blueprint's AP2 path. The AirPlay 1 probe uses one request and then a 25
second repeating timer if the receiver answers 200.

Feedback is not the keep-alive; the event channel is. Implement both.

### 5.6 Audio

The realtime stream sends uncompressed ALAC frames, MSB-first:

```
3b  channel tag = 1 (stereo CPE)
4b  0
12b 0
1b  hasSize = 0 (use the cookie default 352)
2b  wastedBytes = 0
1b  isNotCompressed = 1
352 × {L16, R16}, 16-bit samples, MSB-first
3b  END tag = 7
pad to a byte boundary
```

RTP header, 12 bytes:

```
0x80, 0xE0 (marker on the first packet, then 0x60), seq (be16), timestamp (be32), ssrc (be32)
```

`ssrc` is the session id. The payload is the ALAC frame. For AP2 encryption:

- AAD is RTP header bytes 4 through 11.
- Nonce is an 8-byte little-endian counter.
- Append the last 8 bytes of the nonce after the ciphertext.

Pacing uses a token bucket targeting 44100 frames per second, about one tick per 8 ms. Push silence when the ring buffer is empty to keep the receiver timeline alive. Keep a recent packet backlog for `0x55` retransmit requests.

A session starts at the live edge. Capture runs from launch, so the ring can
hold seconds of pre-connect audio. When a receiver accepts the stream, the
sender marks itself arrived at the session's `StartGate`; playback starts from
`Tick` only after every member has arrived and the gate's shared epoch is
reached. The first successful `Begin` performs the one trim that keeps the
newest 50 ms cushion (`DefaultStartCushion`): the session ring for a single
receiver, or every member ring through the capture fanout in one call under the
fanout's push lock for a stereo pair. The trim is policy, not capture overflow,
so it does not increment the dropped-sample counter.

Sync timestamps are anchored to that shared epoch. The published RTP value is
`latency + frames since epoch`, not the number of packets actually sent, so a
pacer tick that fires a few microseconds late does not become a pair offset.
Audio packet timestamps remain `latency + framesSent`.

On an NTP session, a 20-byte sync packet (type `0xD4`) carries the NTP wall
time every second. On a PTP session, a 28-byte sync packet (type `0xD7`) carries
the receiver's clock identity and a PTP timestamp every 100 ms:

```
0x80 or 0x90 (marker on the first), 0xD7, be16 6,
be32 rtpTimestamp, be64 ptpNetworkTime, be32 rtpTimestamp - latency,
be64 ptpClockID
```

The PTP clock is boot-relative, not Unix epoch; the follower maps local Unix
nanoseconds through a measured offset. Session SETUP carries `timingProtocol:
PTP`, `timingPeerInfo`, and `timingPeerList` with the sender's clock ID. After
RECORD and before the stream SETUP, the sender sends SETPEERS and waits for the
follower to become ready. Stream SETUP advertises `latencyMin = latencyMax =
8820` (200 ms).

### 5.7 Capture

WASAPI shared-mode loopback. Default output device, 48 kHz float32 stereo. Convert to s16le, resample linearly to 44.1 kHz, and write to the ring buffer.

Capture and sending use separate goroutines. The ring is bounded; when full, drop the oldest frames and increment a counter. The capture goroutine never touches the network.

At a session start the ring keeps only its newest 50 ms edge. The start gate
discards the rest of the idle backlog before the first audio packet (see 5.6).
The discarded samples do not count as capture overflow.

### 5.8 State machine

```
Idle
  → Connecting         TCP connect
  → Pairing            pair-setup / pair-verify / PIN
  → Handshake          GET /info → session SETUP → event → RECORD → stream SETUP
  → Streaming          RTP + ALAC + feedback
  → Idle               TEARDOWN / error / user stop
```

The sender event-loop goroutine drives the state machine. The UI sends start, stop, set volume, and submit PIN through a command channel. The sender emits launched, closed, pinRequired, credentialsObtained, and error events.

## 6. Key design decisions

**Port the sans-i/o structure.** Define the six `RaopIo` methods as Go interfaces: TCP connect/send/close and UDP bind/send/close. The sender does not hold sockets directly; the host pushes data back in. Tests can run the full handshake with a fake host and no network. The Go net host uses goroutines and channels.

**Single process, no sidecar.** Protocol, capture, and UI live in one process. One less IPC layer and simpler debugging.

**ALAC first, no PCM.** Modern receivers require ALAC. Implement only
uncompressed ALAC frames, not the compression algorithm. The encoder is about
40 lines.

**NTP by default, PTP for HomePod and macOS.** The C++ blueprint uses NTP and
was verified against HomePod and Apple TV, but a standalone HomePod on software
27 keeps its own PTP grandmaster and ignores an NTP-timed stream. A macOS
AirPlay receiver behaves the same way: it advertises feature bit 41, accepts an
NTP session SETUP, then silently ignores the stream SETUP. The Go port therefore
runs a PTP follower (ported from AirFlash's `clock.rs`) when the receiver
advertises feature bit 41 and its model starts with `AudioAccessory` or `Mac`.
The sender talks to it through a narrow `PTPClock` interface, so NTP stays the
path for Apple TV.

**No old Go libraries.** SRP, TLV8, bplist, ALAC, and WASAPI are implemented in-house or ported from C++/Rust. Only discovery uses hashicorp/mdns, with Windows DNS-SD as a fallback. This avoids dependence on unmaintained go-wca and zeroconf.

**DPAPI for credentials.** Long-term keys never touch disk in plaintext. Logs never contain PINs, long-term keys, or session keys.

**Minimal 200 OK.** Event-channel response headers are strictly minimal. This is a failure mode the C++ blueprint already hit.

## 7. Milestones

Status at the end of the implementation session:

- M0: complete, with unit tests for RFC vectors, SRP round trip, AEAD, and the
  bplist codec.
- M1: complete; the fake receiver drives transient and PIN pairing, encrypted
  control, AP2 setup, event-channel responses, ALAC byte comparison,
  retransmit, timing, feedback, and failure paths.
- M2: Go net host and mDNS discovery implemented. Verified against a real
  HomePod on software 27.2 with PTP, including a 35-second session with no
  drop. Apple TV and macOS hardware remain untested.
- M3: WASAPI loopback, resampling, ring buffer, mygo tray flyout, tray menu,
  PIN dialog, device list, volume, capture endpoint selection, settings, and
  diagnostics implemented. The app launches on Windows; real-device playback is
  pending.
- M4: reconnect (1s/2s/5s/10s/15s backoff, capped at five attempts), per-device
  volume with receiver-volume adoption on first connect, device online status
  and AirPlay group detection, log rotation, single-instance lock, and an
  autostart toggle are implemented. Now Playing reads the Windows SMTC
  session, shows the track in the playback card, and pushes changed metadata
  while the toggle is on. AP1 compatibility, packaging, and installer are
  pending.

### M0 Crypto and wire-format core

- SHA-512, HMAC, HKDF.
- ChaCha20-Poly1305 with an 8-byte nonce.
- X25519, Ed25519.
- SRP-6a 3072/SHA-512 client.
- TLV8 codec.
- bplist codec covering dict/array/string/data/int/bool/real.
- Digest auth.
- Unit tests using C++ and pyatv test vectors.

Acceptance: all vectors pass; `go test ./internal/airplay/crypto/...` is green.

### M1 Sender state machine and fake receiver

- `RaopIo` interface and fake host.
- Pairing: transient and PIN paths.
- Encrypted control channel.
- AP2 session SETUP, event channel, RECORD, stream SETUP.
- Uncompressed ALAC encoder.
- RTP packetization and encryption.
- Event-channel decryption and minimal 200 OK.
- Fake-receiver tests covering handshake, audio, retransmit, and error paths.

Acceptance: the fake receiver decrypts and byte-compares ALAC audio; all tests pass without network access.

### M2 Go net host and real device

- Go TCP/UDP host.
- NTP timing server.
- mDNS discovery and TXT authentication mapping.
- Connection to a real Apple TV, HomePod, or macOS.
- Volume, TEARDOWN, and error recovery.

Acceptance: 30 minutes of continuous playback with no pops and no drop at 30 seconds.

### M3 Capture and UI

- WASAPI loopback.
- Resampling and ring buffer.
- mygo tray flyout, tray menu, device list, PIN dialog, and settings.
- Default-device following and endpoint selection.

Acceptance: the normal flow works; discovery, pairing, and playback take no more than three steps. The UI does not block, and engine errors are visible.

### M4 Polish

- Reconnect, silence standby, diagnostics log.
- AP1 compatibility layer.
- PTP if a real device requires it.
- Now Playing metadata.
- Installer and icons.

## 8. Risks

**Event channel and keep-alive.** This is the most likely cause of "connected but silent." Response headers must be minimal, and every event must be decrypted and answered with 200. Cover it with the C++ blueprint and fake-receiver tests.

**Pairing path differences.** Apple TV uses PIN plus pair-verify; HomePod and macOS use transient pairing. The audio key lengths differ and must be truncated to 32 bytes. Use test vectors for both paths.

**ALAC frame format.** Receivers require ALAC; PCM produces silence. Follow the C++ bit layout and byte-compare with the fake receiver.

**WASAPI capture.** A pure-Go COM binding needs careful lifetime handling and default-device switching. Reference AirFlash's `wasapi.rs` and mygo's WASAPI output.

**mDNS and firewall.** Windows Firewall, multiple NICs, and VPNs all affect discovery. Start with hashicorp/mdns and switch to the Windows DNS-SD API if needed.

**HomePod OS differences.** Confirmed on real hardware on 2026-10-31: HomePod
software 27.2 accepts an NTP session SETUP and RECORD but silently ignores the
stream SETUP, so the sender timed out. With PTP negotiation, SETPEERS, and a
ready PTP follower it streams. The app selects PTP automatically for
`AudioAccessory` receivers that advertise feature bit 41.

**Toolchain.** Go 1.27.0 is installed. TLV8 and uncompressed ALAC are implemented; the rest of the crypto core remains.

**License.** saundo is distributed under GPL-3.0-only. The C++ blueprint is Apache-2.0 and pyatv is MIT; keep NOTICE and copyright headers when porting. AirFlash is dual-licensed under GPLv3+ or a commercial license; this project uses the GPLv3 option, which requires the combined work to be distributed under GPLv3.

## 9. Dependencies and licenses

| Dependency | License | Last commit | Usage |
|---|---|---|---|
| Go standard library | BSD-3-Clause | With Go | Foundation |
| golang.org/x/crypto | BSD-3-Clause | Active | ChaCha20-Poly1305, HKDF, Ed25519, X25519 |
| github.com/egoist/mygo | MIT | 2026-10 | UI, tray, packaging |
| github.com/hashicorp/mdns | MIT | 2026-10 | mDNS discovery |
| airplay2-sender-cpp | Apache-2.0 | 2026-09 | Primary reference; keep NOTICE when porting |
| pyatv | MIT | 2026-06 | Secondary reference; keep copyright notices when porting |
| AirFlash | GPLv3+ (or commercial) | 2026-10 | Code reference; ported under the GPLv3 option |

Do not introduce go-wca, zeroconf, or howett.net/plist. Implement or port the corresponding capability instead.

## 10. Reference links

- airplay2-sender-cpp: https://github.com/akustikrausch/airplay2-sender-cpp
- pyatv: https://github.com/postlund/pyatv
- AirFlash: https://github.com/Ding-Kyoma/AirFlash
- mygo: https://github.com/egoist/mygo
- mygo-cloudmusic: https://github.com/egoist/mygo-cloudmusic
- AirFlash PTP reference: https://github.com/owntone/libairptp
- hashicorp/mdns: https://github.com/hashicorp/mdns
- goplay2: https://github.com/openairplay/goplay2
- UxPlay: https://github.com/FDH2/UxPlay

## 11. Implementation notes and deviations

These were decided while porting the reference. Each is either a protocol
detail the summary above left ambiguous or a deliberate extension.

- **AP2 feedback every 2 seconds.** The C++ sender sets a 2 second timer on the
  AP2 path even though its comment mentions 25 seconds; pyatv's
  `FEEDBACK_INTERVAL` is 2.0. The 25 second interval applies to the AirPlay 1
  probe.
- **Event keys on transient pairing.** The C++ derives `Events-*` keys only
  after pair-verify. Transient pairing stops at M4, so the Go port also derives
  the event keys from the SRP shared secret there. That lets the app decrypt and
  answer event-channel requests if a HomePod or Mac opens the channel. It is a
  superset of the reference behavior.
- **Event-channel response.** The exact reply is `RTSP/1.0 200 OK`,
  `Server: AirTunes/550.10`, the request's `CSeq` when present, one blank line.
  No `Content-Length`, no `Audio-Latency`, no other header.
- **Timing reply layout.** The 32-byte reply carries the request's transmit time
  at bytes 8..16 as the reference time, and the current NTP time at 16..24 and
  24..32.
- **AP2 selection.** The engine enables the AP2 path for every HAP pairing
  (`AuthHAPTransient`, `AuthHAPPIN`, or `AuthHAPPassword`). A `_raop`-only
  receiver without HAP stays on the plaintext AirPlay 1 path. An Apple TV
  3-style receiver that needs FairPlay is rejected before a session starts.
- **Concurrency.** The Go host delivers every socket event on one channel. The
  engine owns the only goroutine that calls sender methods, so no sender state
  is shared. Capture runs in its own COM-initialized goroutine and only touches
  the ring.
- **Audio key sources.** A HomePod session clamps the shared secret to 32 bytes
  for `shk` and the ChaCha key. A password-paired Mac uses the Events-Write key
  as `shk` (the pyatv profile). Control and event keys use the full secret
  through HKDF.
- **FairPlay receivers (Apple TV 3, Xiaomi/Redmi TV).** Confirmed on the LAN on
  2026-11-01: a Redmi TV advertises `AppleTV3,2` over `_raop` and `_airplay`,
  `et=0,3,5`, and a working `/fp-setup`; the iPhone Music app plays to it, but a
  third-party audio sender does not. AP1 `SETUP` returns 400, the AP2 session
  `SETUP` returns 500, and the stream-only AP2 `SETUP` accepts but produces no
  sound with ALAC, PCM, encrypted, or plaintext audio. The audio path requires
  Apple FairPlay, which this project does not implement. Discovery maps
  `AppleTV3,x` to `AuthLegacyPin` and the sender fails fast with a FairPlay
  message instead of starting a silent session.
- **Password-protected macOS receivers.** "AirPlay to Mac" with "Require
  Password" advertises the HAP feature bits, no `pw` TXT, and `sf` bit 7 set
  (a Mac14,2 shows `sf=0x284`), so it pairs with HAP pair-setup keyed by the
  stored receiver password (`AuthHAPPassword`): M1 without Flags, M3 with the
  password as the SRP secret, then M5/M6 and pair-verify, all before any AP2
  traffic. The app asks for the password in a dialog on first connect and
  saves it for later sessions. Transient pairing (`Flags=0x10`) is rejected by this
  receiver with TLV error 3. The AP2 session SETUP then returns 401 with
  `WWW-Authenticate: Digest realm="airplay"`; the sender answers with the RFC
  2069 digest (username `AirPlay`, no qop) and replays the request. The Mac
  also advertises PTP feature bit 41 and silently ignores an NTP-timed stream,
  so it runs the same PTP follower as HomePod. Its stream SETUP uses
  `latencyMin 11025`, `latencyMax 88200`, a 66150-sample scheduling latency,
  ALAC (`audioFormat 0x40000`, `ct 2`), and the Events-Write key as `shk`.
- **PTP for HomePod software 27 and macOS.** The receiver is the grandmaster.
  The same follower serves a Mac; its PTP clock is likewise boot-relative. The follower
  binds UDP 319 and 320, subscribes to Announce/Sync/Follow_Up/Delay_Resp,
  answers peer-delay requests, and estimates the offset with Sync/Follow_Up
  pairs plus Delay_Resp. It treats the receiver's timeline as black-box
  nanoseconds: the real HomePod clock is boot-relative, so the offset is large
  but stable. Readiness requires three exchanges over at least 250 ms and a
  fresh offset. If UDP 319/320 are blocked, the session fails before the stream
  SETUP with a firewall hint. The handshake deadline extends to 30 seconds
  while waiting for the clock: after a 30-minute session the receiver needed
  more than the 10-second handshake timeout to re-establish PTP (measured
  2026-10-31).
- **Stereo pairs.** A configured HomePod pair advertises the same `tsid` on
  both members; `igl=1` marks the leader, and `gcgl=1` only says the group
  contains its leader. Discovery collapses members into one logical target with
  ID `stereo:<tsid>`, names it from `gpn`, keeps the leader first, and hides the
  physical rows. The group is online only when both members are online.
- **Stereo sessions.** A pair is addressed as two AP2 sessions, one per member.
  Each session SETUP carries `groupUUID = tsid`; SETPEERS lists both member
  addresses plus the sender. Capture fans out into one ring per member, and the
  group reports streaming only when both sessions launched.
- **Live-edge start and pair lockstep.** A session's `StartGate` owns one start
  epoch and one trim. Each member arrives when its receiver accepts the stream
  (AP1 RECORD, AP2 stream SETUP) and waits; the last arrival seals the epoch.
  The first `Begin` from the engine Tick trims the session's ring once (for a
  pair, every ring on the capture fanout, atomically under its push lock), and
  every member begins from the same epoch in that Tick. For a stereo pair both
  member rings therefore keep the same content window, and the sync mapping is
  anchored to the shared epoch rather than to each member's send count.
- **Stereo PTP role.** A standalone HomePod OS 27 is the grandmaster, so the
  sender follows it. A stereo pair instead expects the sender to supply the
  timeline: one PTP master binds UDP 319/320 for both members, sends
  Announce/Sync/Follow_Up, answers Delay_Req/Pdelay_Req, and grants unicast
  subscriptions. The master's per-member readiness gates each session's stream
  SETUP.
- **Session selection.** `connect(d)` makes the device being connected the
  engine's selected device. The snapshot, connected name, UI badge, remembered
  volume, and reconnect target all follow that one value. `lastDeviceId` is
  persisted only after the session reaches streaming, so a failed attempt does
  not become the next auto-connect target.
- **Capture discontinuities.** Loopback sets `DATA_DISCONTINUITY` when the
  render stream pauses, the device is reconfigured, or spuriously. It is a
  normal gap, not a capture failure, so capture ignores the flag. A genuine
  capture error is cleared by the next successful stream notification; the UI
  does not keep a stale error after recovery.
- **Silence suspend.** A silence meter in front of the capture fanout records
  the last block with a sample above ±1. After two seconds of digital silence
  the engine calls `transport.SetPaused(true)`: the sender disarms its audio
  pacer and sends no RTP, while the sync, feedback, volume-poll, and event
  timers keep running, so the receiver underruns silently instead of losing the
  session. The first audible block is forwarded to the rings and the engine
  resumes from its 25 ms poll: the sender re-anchors `framesSent` to the
  elapsed wall clock, trims the ring to the 50 ms start cushion, rebuilds the
  resampler, and re-arms the pacer. The realtime stream takes its timeline from
  the UDP sync channel, not from `SETRATEANCHORTIME`; a HomePod answers that
  method on realtime streams with plist `status: -12782` and ignores it. The
  suspend is AP2-only; an AirPlay 1 session keeps the continuous silence
  stream. Verified on a HomePod stereo pair with a 60 s pause and an audible
  resume.
- **Tray flyout and settings window.** The app has no ordinary main window.
  `mygo.NewFlyout` owns the daily panel as a frameless, shadowed 420x500
  flyout anchored to the tray icon; a left click toggles it, outside clicks
  dismiss it, and a right click opens the tray menu. The flyout holds the
  device list with per-row connect, disconnect, cancel, and retry
  actions, the volume slider, the Now Playing row with its push
  toggle, and a diagnostics viewer. The app's display name is 声渡; its
  module, binary, and storage directory are `saundo`. The first run after
  the rename copies `settings.json` and `credentials.dat` from the old
  `%APPDATA%\airgoplay` directory (`internal/app/migrate.go`), so the last
  device and saved passwords survive. Configuration lives in a separate 720x500
  window with a 190-wide sidebar (设置 in the flyout header, or 设置… in the
  tray menu) whose four pages are 通用, 连接, 音频, and 采集. The window hides
  the system title bar (`TitleBarStyle: TitleBarHidden`) and draws its own
  32-DIP bar under the system window buttons, so the bar takes the window's
  surface color instead of the desktop accent. Both windows read
  the same App state, and an engine snapshot update redraws both. Normal
  launch shows the flyout, while login launch stays in the tray.
- **State faces.** One smile mark carries the product. The app icon and the
  flyout header use the plain smile; the tray icon and the flyout footer show
  the same face with a mouth that names the state: spectrum bars for
  streaming, closed eyes and a line for idle, three dots for connecting, X
  eyes for failure, and an O for searching. The tray PNGs live in
  `internal/app/assets` and switch with the engine state and the taskbar
  appearance; the footer draws the same faces as SVG in the state's color.
- **Design tokens.** `internal/app/theme.go` maps the platform theme onto the
  design's light and dark palettes and sets it for each frame, so stock
  widgets and the custom rows share one set of surfaces, lines, and status
  colors. The window's content rises six points as it fades in once per
  show, a dialog's panel scales from 98% as it fades in, and rows, sidebar
  items, and settings pages transition their colors and content; the motion
  respects Reduce Motion. The panel root paints the flyout's surface color,
  because a flyout is transparent by default.
- **Now Playing.** The app polls the Windows SMTC session every two seconds and
  shows the current title/artist in the playback card even when pushing is
  off. Push mode is read from the live engine snapshot, not the settings copy
  loaded at startup; a track is sent only while a session is streaming, and a
  new stream resets the sent marker so reconnect restores the metadata.
- **Credentials.** Stored as one DPAPI-protected JSON map in
  `%APPDATA%\saundo\credentials.dat`, keyed by the receiver's `deviceid`.

## 12. Open questions

- Are the target devices Apple TV, HomePod, macOS, or all of them?
- Is an AirPlay 1 compatibility layer required?
- mygo native UI is the current choice; is WebView2 ever needed?
- Is the first version limited to the system audio endpoint, with application capture in M4?
- Which real devices are available for testing?
