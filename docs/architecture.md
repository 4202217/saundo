<!-- reviewed-through: 2026-11-04 -->

# saundo Architecture and Technology Selection

This page carries the stable design layer: goals, technology decisions, the system shape, design rationale, and risks. The wire-level protocol lives in [protocol.md](protocol.md), receiver profiles in [devices.md](devices.md), the code map in [implementation.md](implementation.md), the UI design language in [ui.md](ui.md), and verification evidence in [verification.md](verification.md).

## 0. Goals and scope

Build a Windows desktop application that streams Windows system audio over AirPlay 2 in real time to HomePod, HomePod mini, Apple TV, and macOS AirPlay receivers.

Constraints:

- Pure Go, single process, single binary.
- AirPlay audio only. No AirPlay video, no FairPlay.
- Distributed under GPL-3.0-only. Apache-2.0, MIT, and GPL reference code may be read and ported, including the AirFlash Rust engine.
- Correctness and verifiability over speed.
- mygo native UI. WASAPI loopback for capture.

Not targets:

- Screen mirroring and video.
- FairPlay audio. Apple TV 3-class receivers fail fast at discovery; see [devices.md](devices.md).

Multi-room stereo pairs, application-level capture, and an AirPlay 1 fallback are implemented; they are documented where they belong, not here.

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

Clone the reference repositories outside this repository and do not commit them:

```bash
git clone --depth 1 https://github.com/akustikrausch/airplay2-sender-cpp
git clone --depth 1 https://github.com/postlund/pyatv
git clone --depth 1 https://github.com/Ding-Kyoma/AirFlash
```

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

Cross-cutting: Settings (JSON), Receiver registry (JSON), Credentials (DPAPI), Diagnostics (logs and metrics)
```

There are three concurrency roles:

- The mygo UI main thread.
- The sender event-loop goroutine, which owns all protocol sockets and timers.
- The capture goroutine, which owns WASAPI capture and ring-buffer writes.

They interact through bounded queues and command channels. No mutable state is shared.

## 4. Protocol

The wire-level protocol — discovery and auth selection, pairing, the encrypted control channel, the AP2 session, the event channel, RTP streaming, timing, and the stereo session topology — lives in [protocol.md](protocol.md), with per-receiver profiles in [devices.md](devices.md). This page used to carry a summary of it alongside implementation notes; the two copies drifted, so the wire detail now has exactly one home.

## 5. Key design decisions

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

**Connect selects the device being connected.** The snapshot's selected ID, the connected name, the UI badge, the remembered volume, and the reconnect target all follow the device the user connected to, and `lastDeviceId` persists only after the session reaches streaming — a failed attempt never becomes the next auto-connect target.

**Capture discontinuities are not errors.** Loopback sets `DATA_DISCONTINUITY` when the render stream pauses, the device is reconfigured, or spuriously; capture ignores the flag, and a successful capture notification clears any stale error instead of leaving one on screen.

## 6. Risks

**Event channel and keep-alive.** This is the most likely cause of "connected but silent." Response headers must be minimal, and every event must be decrypted and answered with 200. Cover it with the C++ blueprint and fake-receiver tests.

**Pairing path differences.** Apple TV uses PIN plus pair-verify; HomePod uses transient pairing; a password-protected Mac uses HAP password pairing and a Digest replay on the session SETUP. The audio key source differs per family. Use test vectors for each path; the profiles are in [devices.md](devices.md).

**ALAC frame format.** Receivers require ALAC; PCM produces silence. Follow the C++ bit layout and byte-compare with the fake receiver.

**WASAPI capture.** A pure-Go COM binding needs careful lifetime handling and default-device switching. Reference AirFlash's `wasapi.rs` and mygo's WASAPI output.

**mDNS and firewall.** Windows Firewall, multiple NICs, and VPNs all affect discovery. Start with hashicorp/mdns and switch to the Windows DNS-SD API if needed.

**HomePod OS differences.** Confirmed on real hardware on 2026-10-31: HomePod
software 27.2 accepts an NTP session SETUP and RECORD but silently ignores the
stream SETUP, so the sender timed out. With PTP negotiation, SETPEERS, and a
ready PTP follower it streams. The app selects PTP automatically for
`AudioAccessory` and `Mac` receivers that advertise feature bit 41.

**License.** saundo is distributed under GPL-3.0-only. The C++ blueprint is Apache-2.0 and pyatv is MIT; keep NOTICE and copyright headers when porting. AirFlash is dual-licensed under GPLv3+ or a commercial license; this project uses the GPLv3 option, which requires the combined work to be distributed under GPLv3.

## 7. Dependencies and licenses

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

## 8. Reference links

- airplay2-sender-cpp: https://github.com/akustikrausch/airplay2-sender-cpp
- pyatv: https://github.com/postlund/pyatv
- AirFlash: https://github.com/Ding-Kyoma/AirFlash
- mygo: https://github.com/egoist/mygo
- mygo-cloudmusic: https://github.com/egoist/mygo-cloudmusic
- AirFlash PTP reference: https://github.com/owntone/libairptp
- hashicorp/mdns: https://github.com/hashicorp/mdns
- goplay2: https://github.com/openairplay/goplay2
- UxPlay: https://github.com/FDH2/UxPlay

