# Discovery and connect

## Sub-features

- mDNS browse on the LAN; `_raop`/`_airplay` advertisements merge into one
  device row; the subtitle shows the model plus an auth label for PIN,
  password, unsupported, or auth-required receivers. Transient and open
  receivers show `model · AirPlay 2` or `AirPlay 1`.
- Directed fallback every fifth cycle (15 s at the 3 s interval): a unicast
  DNS-SD query on UDP 5353 to the Windows ARP neighbors and the remembered
  receiver hosts, for both `_airplay._tcp` and `_raop._tcp`, feeding the same
  parse and merge path. It finds HomePods that miss multicast. Windows only:
  macOS has no ARP neighbor table (`neighbors_other.go`), so the directed
  pass probes remembered hosts only.
- A discovered row stores its last host, port, and `lastSeen`; an inactive
  `stereo:<tsid>` unseen for 24 h is pruned with its auto-connect flag, while
  a group without `lastSeen` gets the full grace. The active/reconnect target
  is force-added and is not pruned while the engine owns it.
- Offline marking after about three missed query cycles
  (`staleAfter = 3×interval + timeout`) and pruning after 90 s. Neither
  surface lists an inactive offline remembered row: `Manageable`
  (`receivers/merge.go:159-167`) drops it, and its saved settings return when
  it is seen again. The active session's row stays while the engine owns it:
  settings shows `离线`, the flyout shows the disconnect/retry action. The
  offline action is reachable only in the auto-reconnect-off, device-closed
  state; the UI fixture covers it, and it was not seen live.
- Stereo-group collapse (`tsid`): one row named `HomePods` with the subtitle
  `<model> · 立体声组 2/2`, leader first, member rows hidden, group offline
  unless both members are online.
- Local receiver: an advertisement whose address belongs to this host is
  marked Local (`discovery.go:235`, `unicast.go:82`), forced into the hidden
  group with its auto-connect flag cleared (`merge.go:82-88`), drawn with
  the 本机 chip and no switch or expander (`receivers_page.go:217-230,335-336`),
  and never listed in the flyout. The engine refuses to connect it
  (`engine.go:1204-1208`), the launch auto-connect skips it
  (`engine.go:1762-1767`), and the sender stops a session whose socket local
  and peer addresses match (`sender.go:809-815`). A sender and its own
  receiver would share one audio path and loop.
- Connect paths by auth kind: HAP transient (fixed PIN `3939`, stops at M4),
  on-screen PIN (配对码 dialog, Apple TV), receiver password (masked dialog on
  first connect, stored per device), legacy FairPlay fail-fast
  (`AppleTV3,x` → FairPlay message, never a silent session).
- Stored-credential reconnect: pair-setup skipped, pair-verify only.
- Disconnect sends TEARDOWN; auto-reconnect backs off 1s/2s/5s/10s/15s, five
  failed connect attempts, then 自动重连 5 次后停止 (`engine.go:288-292`,
  budget at `:259-260`, give-up message at `:1324`). A receiver that is not
  discovered is retried every 5 s without consuming the budget, so a
  receiver that vanishes mid-session does not reach the give-up message.
- Takeover is terminal: a clean control close during streaming stops the
  reconnect (`engine.go:1372-1374,1539-1554`), the status reads
  已被其他设备接管, and the active row offers 重新连接
  (`flyout.go:272-276`; `app.go:990-992`). A close carrying an error stays
  retryable.
- A failed attempt clears its error on the next connect or disconnect
  (`engine.go:1216`, `:1020`), and other online rows stay connectable in the
  error phase (`flyout.go:244-286`; `TestRowActionKeepsOtherReceiversConnectable`).

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
lines — `reply "RTSP/1.0 200 OK" for POST`, `control channel now encrypted`,
`session SETUP ok`, `event channel open`, `streaming to "<name>"`. Capture
`$EV/flyout-before.png` and `-after.png` with capture-window.ps1. Disconnect
by 断开连接 and confirm `session stopped` plus the row returning to 连接.

Proven end state: the flyout shows 正在通过 `<name>` 播放 with the
`ALAC · 44.1 kHz · PTP · 已同步 · 延迟 N ms` line (PTP on HomePod/Mac), and
the log shows `reply "RTSP/1.0 200 OK" for FEEDBACK` every 2 s on AP2 (AP1
uses one probe then 25 s). The 已连接 chip is the settings connection page and
sidebar, not a flyout row.

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
- A failed launch must not become the auto-connect target: the registry's
  auto-connect flag is seeded once from `lastDeviceId` by migration, and
  `lastDeviceId` is persisted only after streaming is first reached.
- Windows Firewall must allow inbound UDP on the private network, or PTP
  and timing requests go unanswered (UI reports a PTP timeout). On macOS the
  equivalent is Local Network permission: mDNS, RTSP, and PTP are dropped
  until the user allows the app.
- The live recipe's doctor and capture scripts and `%APPDATA%` log are
  Windows-only. On macOS the log is
  `~/Library/Application Support/saundo/saundo.log`, window proof is
  `screencapture -x`, and `SAUNDO_UNICAST_BROWSE` cannot exercise an ARP table
  (none exists).
- FairPlay receivers (Xiaomi/Redmi `AppleTV3,2`) accept SETUP and produce
  no sound — the app must fail fast with the FairPlay message; that is the
  correct behavior, not a bug.
