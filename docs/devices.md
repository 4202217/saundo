<!-- reviewed-through: 2026-11-09 -->

# Receiver profiles

One section per receiver family: how it advertises, which pairing path it takes, which keys, timing, and latency it needs, and the quirks verified on hardware. The wire mechanics are in [protocol.md](protocol.md); verification evidence in [verification.md](verification.md). Update this page in the same commit as a device finding.

## HomePod / HomePod mini (software 27.x)

- Advertises `AudioAccessory5,1` (and siblings) with the HAP feature bits; feature bit 41 set.
- Pairs with HAP transient pairing by default: `Flags=0x10`, fixed PIN `3939`, stop at M4 (protocol.md §2.1). A HomePod can gate AirPlay with a password; it then uses HAP password pairing (protocol.md §2.3) but keeps the HomePod `shk` and latency profile. The family comes from `AudioAccessory`, not from the pairing method.
- `shk` and the audio ChaCha20 key: first 32 bytes of the 64-byte SRP K, no HKDF (protocol.md §2.5).
- **PTP required.** It accepts an NTP session SETUP and RECORD, then silently ignores the stream SETUP. The sender runs the PTP follower against the HomePod's grandmaster.
- Latency: `latencyMin` = `latencyMax` = 8820 (200 ms) unless the user configured another value. The stream SETUP response reports `arrivalToRenderLatencyMs`, the receiver's own output latency; a stereo pair reported 69 ms live on 2026-11-09. It is not the sender's presentation latency and is not added to the configured latency. saundo records it for the settings connection card and diagnostics.
- `SETRATEANCHORTIME` returns plist `status: -12782` and is ignored on realtime streams.
- Event-channel keys derive from the SRP shared secret; feedback every 2 s.
- **Session takeover.** A second sender's session displaces the active one. The displaced sender's control connection closes cleanly with no error and no takeover command; the winner keeps streaming. A HomePod does not send `hijackID` to the loser. The sender treats a clean close during streaming as terminal and does not reconnect. Observed on a stereo pair member `AudioAccessory5,1` 27.2 on 2026-11-09 (protocol.md §11).
- **Directed discovery.** Multicast is intermittent on this network: the
  2026-11-06 failure report described an 8-second browse that saw only the
  MacBook Air and the Redmi, and three repeated 2-second browse runs during
  verification alternated between the complete 卧室 group, the two HomePods as
  separate `_raop`-only rows (the `_airplay` replies were lost, so no `tsid`),
  and the MacBook/REDMI only. Both HomePods answered a directed DNS-SD query
  on UDP 5353 (`_airplay._tcp` and `_raop._tcp` PTR, then SRV, TXT, and A)
  every time, in about 10 to 30 ms, and both accepted TCP 7000. Discovery
  keeps the multicast browse and adds a directed pass every fifth cycle over
  the Windows ARP neighbor table and the remembered receiver addresses; the
  directed reply carries the same TXT fields, including the stereo `tsid` and
  `gpn`.
- Verified on software 27.2 (2026-10-31): pairing, PTP, a 35-second muted session, a six-minute app run, a 30-minute soak at exactly 44100 × 1800 frames with zero drops, an audible tone with volume restore — and a cold start that needed more than 10 s to re-establish PTP after a long session (the handshake deadline is now 30 s).
- Ambient light / Sound Glow (2026-11-07): the top display renders the current Now Playing cover once Sound Glow is enabled in the HomePod settings; without it the top stays white. A new 600×600 `SET_PARAMETER image/jpeg` cover on the encrypted control channel repaints it. `POST /artwork/command` is not needed and was removed after three consecutive cover changes repainted an `AudioAccessory5,1` at 27.2. The ambient light has a permanent card, a power action, and a 封面/模板 priority segment. Cover priority (the default) hands the slot to the media cover; template priority shows one of six templates. The manual 关灯 sends the `updateMRPlaybackState` command (`POST /command`, binary plist, `mrPlaybackState: 2`), which clears the Now Playing renderer while the RTP stream keeps running; state `1` restores the current image. Observed on a muted `AudioAccessory5,1` 27.2 on 2026-11-07 (blue → off → blue → off, session streaming throughout). A media pause keeps the current light; only the manual 关灯 sends state `2`, and 开灯 sends `1`. With cover priority on and no media cover, the sender sends no image and the receiver keeps its last light. A stereo pair `AudioAccessory5,1` 27.3 probe on 2026-11-07 confirmed that clearing the slot, a state 2→1 cycle, and a new item without artwork keep the last image; the white-center placeholder appears only on a fresh session. The sender keeps one artwork slot: a template owns it or cover mode hands it to the media cover; the playback state is independent. The on-device `SoundBoardServices` XPC (`setLEDContents:`, `disable_leds`) remains a local-only path with entitlement. `TopDisplayCapable` accepts the HomePod family at 27.2 or later. Template palettes (2026-11-08): the top display renormalizes the three MediaColors and dims the result; a pure gold `#E0B000` and the pale `#F0C050` read white, while `#C08A00` with zero blue, `#C47A1E`, and `#7A3F14` read dark gold.

## HomePod stereo pair

- Both members advertise the same `tsid`; `gpn` carries the group name (`HomePods`). A UTF-8 group name arrives in RFC 1035 `\DDD` byte escapes, the same presentation form as the instance name, and must be decoded before display. `igl=1` marks the leader; `gcgl` alone does not. The 2026-10-31 pair marked only HomePod R, and the re-merged 2026-11-06 pair marked both members.
- Discovery collapses the pair into one target `stereo:<tsid>`, hides the member rows, and reports online only when both members are online.
- A directed query to a member returns only that member's own instance, so
  rebuilding the pair needs both addresses in the candidate list. The ARP
  neighbor table usually holds both after the sender's earlier TCP
  connections; the remembered group row stores the leader's host and port as
  its next-probe seed.
- When both members carry `igl=1`, the collapse sorts leaders first and then
  by name, so it still produces one stable leader-first group.
- One AP2 session per member with `groupUUID = tsid`; SETPEERS carries both members plus the sender; **the sender is the PTP grandmaster** (protocol.md §7–8) — the opposite of the standalone HomePod's follower role.
- Capture fans out to one ring per member; both sessions must reach streaming before the group launches.
- Verified (2026-10-31): both member sessions streamed through the sender-supplied master, a system tone caused zero drops, the flyout showed the streaming footer (then `正在播放到 HomePods`, now `正在通过 HomePods 播放`), and a 60 s silence suspend kept both sessions up and resumed audibly.
- Ambient light on the pair (2026-11-07): `updateMRPlaybackState` reaches both members and state `2` darkens both. A state `1` on the follower can leave its renderer white; a new session restores it. The sender assumes playing on a fresh session (no redundant state `1`) and resends the artwork after a resume's state `1`; after a disconnect/reconnect and a resume, both members were normal. If a follower is already stuck white, reconnect.

## macOS AirPlay receiver (Require Password)

Tested on a MacBook Air, `Mac14,2`, 2026-11-01.

- Advertises the HAP feature bits with **no `pw` TXT** and `sf` bit 7 set (`sf=0x284` on the Mac14,2). The `Mac` model selects the Mac key and latency profile; the password is only the pairing method.
- Transient pairing (`Flags=0x10`) is rejected with TLV error 3. It pairs with HAP pair-setup method 0 keyed by the receiver password: M1 without Flags, M3 with the password as the SRP secret, M5/M6, then pair-verify (protocol.md §2.3).
- The app prompts for the password on first connect with a masked dialog, saves it per receiver in DPAPI, and reconnects; the receiver's row on the settings 接收器 page can change or clear it. A receiver that needs a password and has none stored never starts a session.
- The AP2 session SETUP answers 401 with a Digest challenge; the sender replays with RFC 2069, username `AirPlay` (protocol.md §4).
- `shk` is the Events-Write key (protocol.md §2.5).
- **PTP required** (feature bit 41); the same follower serves it, its clock likewise boot-relative.
- Latency: `latencyMin 11025`, `latencyMax 88200`, plus a 66150-sample scheduling latency.
- **Now Playing metadata.** While streaming, the sender's DMAP `mlit` and the follow-up image `SET_PARAMETER` reach its Now Playing UI. The pushed SMTC cover was confirmed on screen on 2026-11-07 (MacBook Air, `Mac14,2`).

## Apple TV (on-screen PIN)

- `POST /pair-pin-start` with `X-Apple-HKP: 3` makes the TV display a 4-digit code; the app opens its 配对码 dialog.
- M1–M6 with the PIN, then pair-verify; a reconnect skips the dialog and runs pair-verify from stored credentials.
- NTP timing; no PTP requirement known.
- **Live verification pending** — the remaining definition-of-done gap. The pairing and control-channel path is covered by the fake receiver and by the C++ blueprint's shipped product.

## FairPlay TVs (`AppleTV3,x`) — unsupported

Tested on a Redmi TV, 2026-11-01.

- Advertises `AppleTV3,2` over `_raop` and `_airplay`, `et=0,3,5`, and a working `/fp-setup`; the iPhone Music app plays to it. The escaped mDNS name decodes to UTF-8 (`REDMI 显示器 …`).
- A third-party sender cannot play: AP1 `SETUP` returns 400, the AP2 session `SETUP` returns 500, and the stream-only AP2 `SETUP` accepts but produces no sound with ALAC, PCM, encrypted, or plaintext audio. The audio path requires Apple FairPlay.
- Discovery maps `AppleTV3,x` to `AuthLegacyPin` and the sender fails fast with a FairPlay message instead of starting a silent session.

## AirPlay 1 (`_raop`-only) — fallback, not a target

- A receiver advertising only `_raop` without HAP bits stays on the plaintext AirPlay 1 path: RECORD starts the stream, the 25 s probe keeps it alive, and the ring-empty state pushes continuous silence (no suspend).
- AP1 sessions are deliberately excluded from the silence-suspend behavior (protocol.md §6).

## Manual receivers (no mDNS)

- Added on the settings 接收器 page with a name, one of three types, and a
  bare IP or hostname. The port defaults to 5000 for AirPort Express and
  AirPlay 1 and 7000 for AirPlay 2. The row keys by `m:<uuid>`; editing
  keeps the key and updates the fields.
- There is no advertisement, so there are no feature bits and no `model`.
  A manual row therefore always runs NTP. A receiver that requires PTP
  (HomePod software 27, macOS) must be discovered; the finalized add dialog
  has no PTP control.
- The declared type chooses the pairing path: AirPort Express uses
  auth-setup, AirPlay 2 uses HAP transient pairing or HAP password pairing
  when a password is stored, and AirPlay 1 uses the plaintext path or
  digest password authentication.
- A manual AirPlay 2 row with a password has no model, so it uses the
  generic HAP key and latency profile, which is the HomePod rule. A Mac with
  Require Password must be discovered so its `Mac` model selects the Mac
  profile.
- The password is stored in DPAPI under the `password:m:<uuid>` namespace,
  never in `settings.json` or the log. An unreadable stored password is
  treated as absent: the engine prompts again instead of attempting the
  permissive path.
- A manual row is always attemptable (`Online` true), even when the
  receiver is powered off; a failed dial surfaces as a session error and
  falls into the reconnect policy.

## AirPort (auth-setup) — untested path

- `am=AirPort*` in the TXT record maps to the auth-setup path (`AuthSetup`). No AirPort hardware has been tested; treat the path as scaffolding until one is.
