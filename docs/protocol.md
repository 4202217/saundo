<!-- reviewed-through: 2026-11-12 -->

# AirPlay 2 wire protocol

This page is the wire-level specification: discovery and auth selection, pairing, the encrypted control channel, the AP2 session, the event channel, RTP streaming, and timing. It merges the original protocol flow with the implementation notes recorded while porting and testing against real devices; where the two used to disagree, what follows is the behavior that works.

Per-receiver profiles — which family requires which path, and its verified quirks — live in [devices.md](devices.md). Design rationale and technology choices are in [architecture.md](architecture.md). Verification evidence is in [verification.md](verification.md).

When real-device behavior contradicts this page, capture logs and packets first (see [verification.md](verification.md)), change the code, and update this page in the same commit.

## 1. Discovery and auth selection

Browse mDNS for `_raop._tcp` and `_airplay._tcp`. Parse `deviceid`, `features`, `sf`, `pw`, `am`, and `model` from the TXT records, and merge the two services' advertisements into one device. Choose authentication:

- `pw=true`: password authentication (`AuthPassword`).
- `am=AirPort*`: auth-setup (`AuthSetup`, untested path).
- HAP feature bits: Apple TV uses on-screen PIN; HomePod uses transient by default; a receiver with `sf` bit 7 (Require Password) uses HAP password pairing. The model, not the pairing method, selects the family profile (§2.5): a password-gated HomePod is not a Mac.
- No auth bit: plain RTSP.
- `AppleTV3,x` model strings map to `AuthLegacyPin` and fail fast with a FairPlay message (see [devices.md](devices.md)).

The engine enables the AP2 path for every HAP pairing (`AuthHAPTransient`, `AuthHAPPIN`, `AuthHAPPassword`). A `_raop`-only receiver without HAP stays on the plaintext AirPlay 1 path.

### 1.1 Manual receivers

A receiver typed into the settings 接收器 page has no advertisement, so the engine synthesizes its description from the declared type. AirPort Express uses auth-setup; AirPlay 2 uses HAP transient pairing, or HAP password pairing when a password is stored; AirPlay 1 uses the plaintext path, or digest password authentication when a password is stored. The control port defaults to 5000 for AirPort Express and AirPlay 1 and 7000 for AirPlay 2. A manual AirPlay 2 row without a password attempts transient pairing; a receiver that requires a password rejects it, so the password must be stored on the row.

Without TXT records there are no feature bits, including the PTP bit. A manual session therefore always runs NTP. A receiver that requires PTP (HomePod software 27, macOS) must be discovered; the add dialog has no PTP override. The password is stored in the OS secret store (DPAPI on Windows, the Keychain-backed store on macOS) under the `password:<key>` namespace, consumed locally by HAP SRP or the AP1 Digest computation, and never written to the diagnostic log.

## 2. Pairing

### 2.1 HAP transient — HomePod

1. `POST /pair-setup`, TLV8: Method=0, State=1, Flags=0x10.
2. Receive the M2 salt and B.
3. `POST /pair-setup`, State=3, PublicKey=A, Proof=M1, fixed PIN `3939`.
4. Verify the M4 server proof and stop at M4.
5. `sharedSecret = SRP K = SHA-512(S)`, 64 bytes.

### 2.2 HAP PIN — Apple TV

1. `POST /pair-pin-start` with `X-Apple-HKP: 3`; the TV displays a 4-digit PIN.
2. M1 through M4; the user enters the PIN.
3. M5 and M6 exchange long-term keys. M5's inner TLV8 is encrypted with an HKDF-derived session key (nonce `PS-Msg05`); M6 decrypts with `PS-Msg06`.
4. `pair-verify` M1 through M3: X25519 plus Ed25519 signatures, nonces `PV-Msg02` and `PV-Msg03`.
5. `sharedSecret = X25519 ECDH`, 32 bytes.

### 2.3 HAP password — receiver with Require Password

1. `POST /pair-setup`, Method=0, State=1, **no Flags**, no `/pair-pin-start`.
2. M3 carries the receiver password as the SRP secret; M4 verifies.
3. M5/M6 exchange long-term keys, then pair-verify as usual — all before any AP2 traffic.
4. Transient pairing (`Flags=0x10`) is rejected by this receiver with TLV error 3.

The tested case is a Mac. A HomePod can gate AirPlay the same way; it runs the same HAP password flow but keeps the HomePod key and latency profile (§2.5).

### 2.4 Long-term credentials

```json
{"atvId":"<hex>","clientId":"<uuid>","ltpk":"<hex>","ltsk":"<hex>"}
```

Encrypted with the OS secret store (DPAPI on Windows, the Keychain-backed store on macOS) and indexed by the receiver's `deviceid`. Stored credentials skip pair-setup on reconnect; the session runs pair-verify directly.

### 2.5 Audio key derivation

Two rules, one per receiver family. The pairing method does not select the rule; a HomePod that paired with a password still uses the first rule:

- HomePod: both `shk` and the audio ChaCha20 key are the **first 32 bytes of `sharedSecret`, with no HKDF**. The transient K is 64 bytes and must be truncated.
- Password-paired Mac: `shk` is the **Events-Write key** (HKDF over the full shared secret).

Control and event keys always use HKDF-SHA512 over the full `sharedSecret` (§3, §5).

## 3. Encrypted control channel

After `pair-verify`, switch the control channel to ChaCha20-Poly1305 immediately; Apple TV rejects plaintext SETUP. Each frame:

```
[2-byte little-endian length][ciphertext][16-byte tag]
```

- Chunks up to 1024 bytes.
- AAD is the 2-byte length prefix.
- Nonce is 4 zero bytes plus an 8-byte little-endian counter.
- Separate counters for send and receive.
- Keys: HKDF-SHA512("Control-Salt", "Control-Write-Encryption-Key" / "Control-Read-Encryption-Key", sharedSecret, 32).

## 4. AP2 session

Order (a wrong order usually produces "connected but silent" or a drop after 30 seconds):

1. `GET /info` on the encrypted control channel.
2. `SETUP rtsp://host/sessionId` with a binary plist body: `deviceID`, `sessionUUID`, `timingPort`, `timingProtocol` (`NTP` or `PTP`), `isMultiSelectAirPlay`, `groupContainsGroupLeader`, `macAddress`, `model`, `name`, `osBuildVersion`, `osName`, `osVersion`, `senderSupportsRelay`, `sourceVersion`, `statsCollectionEnabled`.
3. The response contains `eventPort`.
4. Connect the event channel (§5); the receiver initiates the reverse connection.
5. `RECORD` with an empty body and standard RTSP headers; it goes after session SETUP and before stream SETUP.
6. On a PTP session, SETPEERS is a no-op; activate the PTP follower as soon as RECORD is answered and wait for the clock to be ready (§7) before the stream SETUP.
7. `SETUP` stream with the ALAC plist (table below).
8. The response provides `dataPort` and `controlPort`. It may also carry `arrivalToRenderLatencyMs`, the receiver's own arrival-to-render latency in milliseconds. The value is diagnostic; it is not the sender's presentation latency and is not added to it. Do not send another RECORD; start streaming.

**Wi-Fi metrics.** The response's first stream object may also carry `wiFiMetrics` (capital F) with `rssi` (integer dBm), `ssid` (string), and `wifiBand` (`1` = 2.4 GHz, `2` = 5 GHz, `3` = 6 GHz). It is a device-side snapshot that can be up to 30 minutes old, not a live signal reading; a session that starts within 30 seconds, or a receiver with metrics collection off, omits it. saundo parses the object during the stream SETUP and records it in the engine snapshot; an absent object is unknown, and the app never polls `/metrics` or `/info` for it. The band enumeration is the receiver's; a value outside 1–3 reads unknown. The live capture that confirms this object is pending (see [verification.md](verification.md)).

**Digest challenge.** A password-protected Mac answers the session SETUP with 401 and `WWW-Authenticate: Digest realm="airplay"`. Replay the identical request with the RFC 2069 digest — username `AirPlay`, no qop — adding `Authorization`. The replay returns 200.

Stream SETUP fields:

| Field | Value |
|---|---|
| `audioFormat` | `0x40000`, ALAC/44100/16/2 |
| `audioMode` | `default` |
| `controlPort` | Local control UDP port |
| `ct` | `2`, ALAC |
| `isMedia` | `true` |
| `latencyMax` | `88200` macOS; `8820` HomePod; the configured latency otherwise |
| `latencyMin` | `11025` macOS; `8820` HomePod; the configured latency otherwise |
| `shk` | 32-byte audio key (§2.5) |
| `spf` | `352` |
| `sr` | `44100` |
| `type` | `0x60` |
| `supportsDynamicStreamID` | `false` |
| `streamConnectionID` | Numeric session id |

The latency modes resolve to a millisecond value before SETUP. Low is 200 ms, standard is 1500 ms, stable is 2000 ms, and custom is 20 to 2000 ms. The base plus a receiver's DSP/DAC delay clamps to 20 to 4000 ms, and the sender sends that value unchanged. A live change restarts the session so the new latency reaches the stream SETUP and the sender re-anchors the timeline.

A macOS receiver adds a 66150-sample scheduling latency (see [devices.md](devices.md)).

## 5. Event channel and keep-alive

The event channel is a reverse encrypted TCP connection: the receiver connects back and pushes events such as `POST /command` and `updateInfo`. Decrypt each event and reply; an unanswered channel tears the session down after about 25–30 seconds.

The reply must be exactly:

```
RTSP/1.0 200 OK\r\n
Server: AirTunes/550.10\r\n
CSeq: <n>\r\n
\r\n
```

The request's `CSeq` only when present. Do not add `Content-Length`, `Audio-Latency`, or any other header — one extra header corrupts the receiver timeline, which looks like a healthy connection with no audio.

Event keys: HKDF-SHA512("Events-Salt", "Events-Write-Encryption-Key" / "Events-Read-Encryption-Key", sharedSecret, 32). Because the connection is reversed, swap the receive and send keys. Transient pairing stops at M4, before the reference derives event keys, so the port derives them from the SRP shared secret there — a superset of the reference behavior that lets the app answer the channel if a HomePod or Mac opens it.

Also send `POST /feedback RTSP/1.0` with `CSeq`, `User-Agent`, `DACP-ID`, `Active-Remote`, `Client-Instance`, and `Content-Length: 0`:

- AirPlay 2: every **2 seconds** (pyatv `FEEDBACK_INTERVAL`, verified against HomePod 27).
- AirPlay 1: one probe, then every 25 seconds while the receiver answers 200.

Feedback is not the keep-alive; the event channel is. Implement both. The AP2 `/feedback` response body is a 55-byte bplist containing `{"streams": []}` on HomePod 27.2; it carries no queue, late, or underrun telemetry. Use the sender's own counters and the stream SETUP's `arrivalToRenderLatencyMs` for diagnostics.

## 6. RTP streaming

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

RTP header, 12 bytes: `0x80, 0xE0` on the first packet (marker set), `0x60` afterwards, seq (be16), timestamp (be32), `ssrc` (be32, the session id). AP2 encryption: AAD is RTP header bytes 4–11 (seq, timestamp, ssrc); the nonce is an 8-byte little-endian counter whose last 8 bytes are appended after the ciphertext.

Pacing uses a token bucket at 44100 frames per second, about one tick per 8 ms. While a stream is active, push silence when the ring is empty. Keep a recent packet backlog for `0x55` retransmit requests; a retransmit is answered with the original packet.

At the start epoch the sender sends two prefill packets. Each audio packet carries extra copies chosen from the previous session's retransmit ratio: the first session and any lossy session send an extra copy, a clean session sends none, and a session above roughly 3% loss sends two extras. A receiver deduplicates by RTP sequence number, so extra copies cost bandwidth, not duplicated audio.

**Silence suspend.** After 5 s of digital silence the engine suspends the audio pacer and the sender sends no RTP, while sync, feedback, volume polling, and the event channel keep running — the receiver underruns silently instead of losing the session. On the first audible block the sender re-anchors `framesSent` to the elapsed wall clock, trims the ring to the 50 ms start cushion, rebuilds the resampler, and re-arms the pacer. `SETRATEANCHORTIME` is not part of this: a HomePod answers it on realtime streams with plist `status: -12782` and ignores it — the method is for buffered streams, and the realtime stream takes its timeline from the UDP sync channel. The suspend is AP2-only; an AirPlay 1 session keeps the continuous silence stream.

## 7. Timing

### NTP

Used for receivers that accept it. A 20-byte sync packet (type `0xD4`) carries the NTP wall time every second. The 32-byte reply to a timing request carries the request's transmit time at bytes 8–16 as the reference time, and the current NTP time at 16–24 and 24–32.

### PTP — follower

HomePod software 27 and macOS receivers keep their own PTP grandmaster and silently ignore an NTP-timed stream: they accept an NTP session SETUP and RECORD, then never answer the stream SETUP. When the receiver advertises feature bit 41 (models `AudioAccessory*`, `Mac*`), the session runs PTP:

- Session SETUP carries `timingProtocol: PTP`, `timingPeerInfo`, and `timingPeerList` with the sender's clock ID.
- After RECORD, activate the follower without a SETPEERS round trip; the stream SETUP waits until the clock is ready.
- The follower binds UDP 319 and 320, subscribes to Announce/Sync/Follow_Up/Delay_Resp, answers peer-delay requests, and estimates the offset from Sync/Follow_Up pairs plus Delay_Resp. Readiness requires three exchanges over at least 250 ms with a fresh offset. If the ports are firewall-blocked, the session fails before the stream SETUP with a firewall hint.
- The receiver's PTP clock is boot-relative, not Unix epoch; only the measured offset matters.
- The handshake deadline extends to 30 seconds while waiting for the clock: after a 30-minute session the receiver needed more than 10 seconds to re-establish PTP.

Sync packets are 28 bytes, every 100 ms:

```
0x80 or 0x90 (marker on the first), 0xD7, be16 6,
be32 rtpTimestamp, be64 ptpNetworkTime, be32 rtpTimestamp - latency,
be64 ptpClockID
```

### PTP — master (stereo pairs)

A stereo pair expects the sender to supply the timeline: one PTP master binds UDP 319/320 for both members, sends Announce/Sync/Follow_Up, answers Delay_Req/Pdelay_Req, and grants unicast subscriptions. The master's per-member readiness gates each session's stream SETUP. A standalone HomePod keeps the follower role.

## 8. Stereo session topology

A HomePod stereo pair is not one session:

- Group members share `tsid`; `igl=1` marks the leader (`gcgl` only says the group contains its leader). Discovery collapses the members into one logical target, `stereo:<tsid>`, named from `gpn` (decode its RFC 1035 `\DDD` escapes first), leader first; the group is online only when both members are online.
- Open one AP2 session **per member**; each session SETUP carries `groupUUID = tsid`.
- SETPEERS is skipped on every PTP session (single and pair); each member's follower activates from its RECORD reply.
- Capture fans out into one ring per member (`ring.Fanout`); the group reports streaming only when every member's session launched.

## 9. Session lifecycle and start gate

```
Idle
  → Connecting         TCP connect
  → Pairing            pair-setup / pair-verify / PIN / password
  → Handshake          GET /info → session SETUP → event channel → RECORD → PTP activate and wait → stream SETUP
  → Streaming          RTP + ALAC + feedback + sync
  → Idle               TEARDOWN / error / user stop
```

A session starts at the live edge: capture runs from launch, so the ring holds seconds of pre-connect audio, and replaying it from the oldest end is wrong. One `StartGate` per session owns the start:

- A member arrives when its receiver accepts the stream (AP1 RECORD, AP2 stream SETUP) and waits; the last arrival seals the shared epoch.
- The first `Begin` from the engine's Tick performs the one trim that keeps the newest 50 ms cushion (`DefaultStartCushion`) — the session ring for a single receiver, or every member ring through the fanout in one call under its push lock for a pair. The trim is policy, not capture overflow, so it does not increment the dropped-sample counter.
- Every member begins from the same epoch in that Tick, so a pair's rings keep the same content window and the members arm their pacers in lockstep.
- Sync timestamps are anchored to that shared epoch: the published RTP value is `latency + frames since epoch`, not the number of packets actually sent, so a late pacer tick does not become a pair offset. Audio packet timestamps remain `latency + framesSent`.

## 10. Top-display artwork (Sound Glow)

HomePod software 27.2 renders the current Now Playing cover on its top display once the user enables Sound Glow in the HomePod settings. Without that setting the display stays blank even though every request succeeds.

The top display renders the artwork already attached to the Now Playing item. One `SET_PARAMETER` with `Content-Type: image/jpeg` and the raw JPEG body on the encrypted control channel, with the same `Session` and `RTP-Info` headers as track metadata, makes the image the Now Playing cover and repaints the top display:

```
SET_PARAMETER rtsp://<receiver>/<session> RTSP/1.0
Session: <session>
RTP-Info: seq=<n>;rtptime=<ts>
Content-Type: image/jpeg
Content-Length: <n>

<raw JPEG bytes>
```

- When the sender has no track metadata, it first sends a minimal DMAP item (`mlit` containing `minm` = `saundo`) so the cover has a Now Playing item to attach to.
- The image is a 600×600 sRGB JPEG at quality ~0.9; the receiver downscales and softens it for the top display.
- The ambient light has a permanent card, a power action, and a 封面/模板 priority segment. Cover priority (the default) hands the artwork slot to the media cover; template priority chooses one of six rendered carriers, pushed on choice change and on stream start; each carrier is three flat color fields, and the receiver's k-means reduces it to the three colors that drive the light. The manual 关灯 sends the paused playback state, and the top display goes dark; no image is involved. A media pause keeps the current light; a new cover (a track change) updates it. With cover priority on and no media cover, the sender sends no image and the receiver keeps its last light; a fresh session with no artwork shows the device's white-center placeholder. In the flyout, cover priority hides the chevron and the template list; template priority shows the chevron and expands the list only on click. The sender has no artwork-clear command.
- A true LED-off is a playback-state command, not an image. `POST /command` on the encrypted control channel with a binary plist body sets MediaRemote's playback state:

  ```
  POST /command RTSP/1.0
  Session: <session>
  Content-Type: application/x-apple-binary-plist
  Content-Length: <n>

  {type: "updateMRPlaybackState", params: {mrPlaybackState: 2}}
  ```

  `mrPlaybackState` is `1` playing and `2` paused. The receiver's AirPlay command processor applies it to the main session; `NowPlayingCap` observes the MediaRemote `isPlaying` change and clears the LED effect, so `SoundBoardFramework.MediaRenderer` stops the Sound Glow and the top display goes dark. The command does not stop the RTP stream.

  **Idle standby** uses the same command. After five seconds without an audible block the engine suspends the pacer; once the configured standby threshold (1, 5, 10, or 30 minutes, default 30) has passed since the last RTP push, the sender sends state `2` and the top display goes dark while the session stays up. The first audible block sends state `1`, resends the artwork, and resumes the pacer. The clock counts from the last RTP packet, not from the silence meter, so a manual light action or a late packet does not lengthen or shorten the threshold. Measured on a muted HomePod mini `AudioAccessory5,1` 27.2 on 2026-11-07: a tide palette cover lit the display, state 2 turned it off, state 1 lit it again, state 2 turned it off again, with `framesSent` rising 1.50M → 2.38M and no session close. On a stereo pair the state reaches both members; a state `1` on the follower can leave its renderer white, and only a new session restores it, so a sender should avoid a redundant state `1` and resend the artwork after a resume. The on-device `com.apple.soundboardservices.server` XPC (`setLEDContents:`, `disable_leds`) remains a local-only path, but it is no longer the only way to turn the LEDs off.
- A song's own cover competes for the artwork slot: a template owns it; cover priority hands it to the receiver track-info cover. The playback state is independent of that slot: with state 2 the display is dark whatever image is attached, and state 1 restores the current image. Clearing the sender's slot only stops future sends; the receiver keeps the last image.
- The setting cannot be discovered over the protocol; `TopDisplayCapable` accepts the HomePod family at 27.2 or later.

Measured on a HomePod mini `AudioAccessory5,1` 27.2. On 2026-11-06, with the receiver muted, raw JPEG `/artwork/command` POSTs returned `400 Bad Request`; a binary-plist POST returned `200 OK` but did not replace the cover. On 2026-11-07, three consecutive `SET_PARAMETER image/jpeg` covers (tide, moon, orchid) each repainted the top display with no `/artwork/command`; the POST trigger and the separate `TCPArtwork` channel were removed. Later on 2026-11-07, the `updateMRPlaybackState` command turned the display off and back on while the stream stayed up (see above). A stereo pair `AudioAccessory5,1` 27.3 probe on 2026-11-07 confirmed that clearing the artwork slot, a state 2→1 cycle, and a new item without artwork all keep the last image; the white-center placeholder appears only on a fresh session with no artwork.

## 11. Session takeover

A receiver serves one active sender per session. When a second sender opens a session to the same receiver, the receiver gives it the device and closes the displaced sender's control connection. The displaced sender does not receive a takeover command, a `hijackID`, or a specific error. On a HomePod `AudioAccessory5,1` 27.2 stereo pair member, the loser received a normal `POST /command` with `type: updateInfo`, then a clean control-channel close with an empty error, while the winner reached streaming.

The sender treats a clean control close during streaming as terminal. It stops RTP, timers, and sockets, reports `Preempted`, and does not reconnect. A close carrying an error stays a retryable connection failure, and a clean close during pairing or handshake stays a start failure. The engine records the takeover separately from a generic error so the interface can show the state and offer a manual reconnect instead of fighting the new sender.

Observed on 2026-11-09 with two sender sessions against the same HomePod R, `192.168.31.164:7000`, model `AudioAccessory5,1`, group `HomePods`. The probe is `TestLiveTakeoverProbe`, gated by `SAUNDO_TAKEOVER_TEST=1`. The automated classification is pinned by `TestCleanControlCloseDuringStreamingPreempts` and `TestControlCloseWithErrorIsNotPreemption`; the engine's no-reconnect path by `TestTakeoverStopsReconnect`.
