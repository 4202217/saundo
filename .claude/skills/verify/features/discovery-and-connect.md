# Discovery and connect

## Sub-features

- mDNS browse on the LAN; `_raop`/`_airplay` advertisements merge into one
  device row; the name line carries a password or auth chip (密码锁 +
  需要密码, 钥匙 + 密码已保存, 感叹号 + 密码错误, key + PIN 配对, alert +
  需要认证 / 暂不支持), and the second line reads 声道 · model · version
  for a single (model · version for a stereo group), with the marketing name
  for a known HomePod, Apple TV, or Mac identifier, without the raw
  identifier or the address. A combo row keeps the 立体声组合 tag and lists its members.
- Directed fallback every fifth cycle (15 s at the 3 s interval): a unicast
  DNS-SD query on UDP 5353 to the Windows ARP neighbors and the remembered
  receiver hosts, for both `_airplay._tcp` and `_raop._tcp`, feeding the same
  parse and merge path. It finds HomePods that miss multicast. Windows only:
  macOS has no ARP neighbor table (`neighbors_other.go`), so the directed
  pass probes remembered hosts only.
- A discovered row stores its last host, port, `lastSeen`, and the OS
  version, and each stereo member keeps its own (the offline projection is
  `CachedDevice`); that cache lets a remembered pair auto-connect and light
  both tops before mDNS answers (`TestRememberedStereoPairKeepsTopDisplayCapability`).
  An inactive
  `stereo:<tsid>` unseen for 24 h is pruned with its auto-connect flag, while
  a group without `lastSeen` gets the full grace. The active/reconnect target
  is force-added and is not pruned while the engine owns it.
- Offline marking after `max(3×interval + timeout, 5×interval + timeout)`,
  which is about 17 s at the default 3 s interval, and pruning after 90 s.
  Neither
  surface lists an inactive offline remembered row: `Manageable`
  (`receivers/merge.go:187-195`) drops it, and its saved settings return when
  it is seen again. The active session's row stays while the engine owns it:
  settings shows `离线`, the flyout shows the disconnect/retry action. The
  offline action is reachable only in the auto-reconnect-off, device-closed
  state; the UI fixture covers it, and it was not seen live.
- Stereo-group collapse (`tsid`): one row named `HomePods` with the subtitle
  `<model> · 立体声组 2/2`, leader first, member rows hidden, group offline
  unless both members are online.
- A manual combo with a member inside a native pair is marked invalid
  (`receivers.Target.Invalid`): settings reads 不可用 with 成员已加入立体声组 and
  keeps only the layout plus 解除组合, the flyout and tray omit it, auto-connect
  skips it, and `validateTarget` refuses it with `ErrComboStale`. `DropLinked`
  exempts an invalid combo, so an online standalone member returns as its own
  row.
- A software stereo combo is created, edited in place, and unlinked from the
  receivers page's 立体声组合 section (`receivers_dialogs.go:136-140,610,755`).
  Linking moves the members' launch flags onto the combo and unlinking hands
  them back; swapping sides hot-applies the new channels without a rebuild
  (`TestLinkComboMovesTheLaunchFlag`,
  `TestSwapComboSidesFlipsMembers`).
- The launch auto-connect flags are a set: toggling one receiver never
  clears the others, and every flagged row connects as one cohort in
  registry order. A flagged row with no address yet holds the launch for
  discovery up to `autoPendingWait` (15 s), then drops with a log line and
  the rest launch; the flag stays on the dropped row for the next app start
  (`TestAutoConnectDropsASilentFlaggedReceiverAndLaunchesTheRest`).
- Cohort events route by the socket they arrived on: the per-member forward
  goroutine stamps every event with its member's dialed host, so a
  multi-homed receiver's UDP replies reach its own sender in a multi-member
  session instead of being dropped as an unknown peer
  (`TestCohortAttributesAUDPReplyToTheMemberThatOwnsTheSocket`).
- Local receiver: an advertisement whose address belongs to this host is
  marked Local (`discovery.go:291,328-330`, `unicast.go:82`), forced into the
  hidden
  group with its auto-connect flag cleared (`merge.go:82-88`), drawn with
  the 本机 chip and no switch or expander (`receivers_page.go:217-230,335-336`),
  and never listed in the flyout. The engine refuses to connect it
  (`plan.go:447-454`), the launch auto-connect skips it
  (`engine.go:1880-1886`), and the sender stops a session whose socket local
  and peer addresses match (`sender.go:860-864`). A sender and its own
  receiver would share one audio path and loop.
- Connect paths by auth kind: HAP transient (fixed PIN `3939`, stops at M4),
  on-screen PIN (配对码 dialog, Apple TV), receiver password (masked dialog on
  first connect, stored per device), legacy FairPlay fail-fast
  (`AppleTV3,x` → FairPlay message, never a silent session).
- Stored-credential reconnect: pair-setup skipped, pair-verify only.
- Disconnect sends TEARDOWN; auto-reconnect backs off 1s/2s/5s/10s/15s, five
  failed connect attempts, then 自动重连 {count} 次后停止 (`engine.go:248-251`,
  budget at `:218-219`, `planReconnect` at `:1241-1255`, give-up message at
  `:1243-1244`). A key absent from the list is retried every 5 s without
  consuming the budget; a receiver that merely goes offline keeps its cached
  endpoint and each attempt counts. A user connect resets the budget
  (`connectResolved`, `cohort.go:626-629`), so 重试, 重新连接, and the tray's
  重试连接 start a fresh 1/2/5/10/15 s cycle after a give-up
  (`TestUserRetryAfterGiveUpResumesReconnect`).
- Takeover is terminal: a clean control close during streaming stops the
  reconnect (`engine.go:1511-1529`; `sender.go:906`), the status reads
  已被其他设备接管, and the active row offers 重新连接
  (`flyout.go:281-283`; `flyout_devices.go:462-470`). A close carrying an error stays
  retryable.
- A failed attempt clears its error on the next connect or disconnect
  (`cohort.go:629`, `engine.go:1033`), and other online rows stay connectable
  in the
  error phase (`flyout.go:244-286`; `TestRowActionKeepsOtherReceiversConnectable`).

## How to get to it (user POV)

Launch the app; the flyout lists receivers on the LAN (about ten seconds).
Click a row, click 连接. On a PIN receiver the 配对码 dialog opens; on a
password receiver a masked prompt opens on first connect. The tray menu's
receiver rows are the second entry point: a click connects when idle, or adds
that receiver to the playing cohort while streaming (an already-active
target is a no-op), and its session row carries 取消连接 /
断开连接 / 重试连接; see [tray-and-flyout.md](tray-and-flyout.md).

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
the log shows `reply "RTSP/1.0 200 OK" for POST /feedback` every 2 s on AP2 (AP1
uses one probe then 25 s). The 已连接 chip is the settings connection page and
sidebar, not a flyout row.

## Gotchas

- A HomePod on software 27 and a macOS receiver silently ignore an
  NTP-timed stream: without PTP the stream SETUP is accepted and nothing
  plays. The session must show PTP and 已同步.
- A Mac with Require Password answers the AP2 session SETUP with 401 first;
  the log line `digest challenge, re-sending SETUP with auth` is success,
  not failure. A saved password the receiver refuses stops the retry ladder:
  the password dialog reopens, `ErrPasswordRejected` shows under the card, and
  adding that receiver to a live cohort leaves the other targets playing.
  Pinned by `TestHAPPasswordWrongPasswordFailsWithoutPrompt`,
  `TestRejectedPasswordPromptsInsteadOfRetrying`, and
  `TestRejectedPasswordOnTheOnlyTargetDoesNotRetry`.
- Pairing 470 means the receiver wants a PIN it did not show; the app falls
  back from transient to on-screen PIN once.
- Multicast is intermittent on some LANs; one successful browse does not
  prove the next cycle. A receiver that is online but missing from the flyout
  should be checked with the directed pass before calling it offline.
- A failed launch must not become the auto-connect target: the registry's
  auto-connect flag is seeded once from `lastDeviceId` by migration, and
  `lastDeviceId` is persisted only after streaming is first reached.
- Takeover (已被其他设备接管) needs a second AirPlay sender to start to the
  same receiver mid-stream; the app alone cannot produce it. The switch step
  needs two simultaneously online receivers, because one receiver cannot
  exercise the other rows' actions.
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
