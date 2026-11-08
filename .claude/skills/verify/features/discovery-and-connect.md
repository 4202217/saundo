# Discovery and connect

## Sub-features

- mDNS browse on the LAN; `_raop`/`_airplay` advertisements merge into one
  device row; the subtitle shows model and pairing path.
- Directed fallback every fifth cycle (15 s at the 3 s interval): a unicast
  DNS-SD query on UDP 5353 to the Windows ARP neighbors and the remembered
  receiver hosts, for both `_airplay._tcp` and `_raop._tcp`, feeding the same
  parse and merge path. It finds HomePods that miss multicast.
- A discovered row stores its last host, port, and `lastSeen`; a
  `stereo:<tsid>` unseen for 24 h is pruned with its auto-connect flag, while
  a group without `lastSeen` gets the full grace.
- Offline marking after about three missed query cycles
  (`staleAfter = 3×interval + timeout`) and pruning after 90 s; a row shows
  离线.
- Stereo-group collapse (`tsid`): one `HomePods · 立体声组 · 2/2` row, leader
  first, member rows hidden, group offline unless both members are online.
- Connect paths by auth kind: HAP transient (fixed PIN `3939`, stops at M4),
  on-screen PIN (配对码 dialog, Apple TV), receiver password (masked dialog on
  first connect, stored per device), legacy FairPlay fail-fast
  (`AppleTV3,x` → FairPlay message, never a silent session).
- Stored-credential reconnect: pair-setup skipped, pair-verify only.
- Disconnect sends TEARDOWN; auto-reconnect backs off 1s/2s/5s/10s/15s, five
  attempts, then 自动重连 5 次后停止 (`engine.go:221,236-239`).

## How to get to it (user POV)

Launch the app; the flyout lists receivers on the LAN (about ten seconds).
Click a row, click 连接. On a PIN receiver the 配对码 dialog opens; on a
password receiver a masked prompt opens on first connect.

## Driving it

Protocol path, no device — the fake receiver decrypts control, event, and
audio channels and byte-compares the stream:

```bash
go test -v ./internal/airplay/sender/
```

Live, with hardware on the LAN (mutes the receiver, streams silence):

```bash
SAUNDO_MDNS_TEST=1   go test -run TestBrowseIntegration -v ./internal/discovery/
SAUNDO_UNICAST_TEST=<homepod-ip> go test -run TestLiveUnicastQuery -v ./internal/discovery/
SAUNDO_UNICAST_BROWSE=1 go test -run TestLiveUnicastBrowse -v ./internal/discovery/
SAUNDO_DEVICE_TEST=1 go test -run TestLiveHomePodHandshake -v ./internal/airplay/sender/
```

Live app: run the doctor, connect from the real flyout, and judge by log
lines — `pair-setup M1`…M4, `control channel now encrypted`, `session SETUP`,
`event channel open`, `streaming to "<name>"`. Capture
`$EV/flyout-before.png` and `-after.png` with capture-window.ps1. Disconnect
by 断开 and confirm `session stopped` plus the row returning to idle.

Proven end state: the flyout shows 正在通过 `<name>` 播放 with the
`ALAC · 44.1 kHz · 立体声 · PTP · 已同步` line (PTP on HomePod/Mac), the row
carries 已连接, and the log shows `reply "RTSP/1.0 200 OK" for FEEDBACK`
every 2 s.

## Gotchas

- A HomePod on software 27 and a macOS receiver silently ignore an
  NTP-timed stream: without PTP the stream SETUP is accepted and nothing
  plays. The session must show PTP and 已同步.
- A Mac with Require Password answers the AP2 session SETUP with 401 first;
  the log line `digest challenge, re-sending SETUP with auth` is success,
  not failure.
- Pairing 470 means the receiver wants a PIN it did not show; the app falls
  back from transient to on-screen PIN once.
- Multicast is intermittent on some LANs; one successful browse does not
  prove the next cycle. A receiver that is online but missing from the flyout
  should be checked with the directed pass before calling it offline.
- A failed launch must not become the auto-connect target: `lastDeviceId`
  is persisted only after streaming is first reached.
- Windows Firewall must allow inbound UDP on the private network, or PTP
  and timing requests go unanswered (UI reports a PTP timeout).
- FairPlay receivers (Xiaomi/Redmi `AppleTV3,2`) accept SETUP and produce
  no sound — the app must fail fast with the FairPlay message; that is the
  correct behavior, not a bug.
