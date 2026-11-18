# Tray and flyout

## Sub-features

- Tray icon: black ink on a light taskbar, white on a dark one, re-drawn on
  appearance change; static face when 托盘动画 is off or Reduce Motion is on.
- Tray animation: 12 frames at 100 ms (`tray_anim.go:11-12`) while
  connecting (three pulsing dots) or streaming (five bars riding a sine
  wave); eyes static.
- Left click toggles the flyout; click outside or Escape dismisses it; the
  app stays in the tray.
- Right click opens the menu: one row per visible receiver, up to six, each
  with its receiver-family glyph in one icon column. The in-session receiver
  draws that glyph in the accent color; an unreachable row keeps the 离线
  subtitle and stays clickable. Clicking a row connects when idle, or adds
  that receiver to the playing session and rebuilds the cohort while
  streaming; an already-active target is a silent no-op. Only the primary
  active row is marked and disabled this round. While connecting every row is
  disabled. The row under the list is the session action: 取消连接, 断开连接,
  or 重试连接, and it is absent while idle; with more than six receivers the
  更多接收器… row sits between the sixth receiver row and the session action,
  and 重试连接 is disabled when no receiver is selected. The playback group
  follows when the system media session has controls: previous, 暂停 or 播放,
  and next, with no track title or subtitle. Each control row appears only
  when the session has that capability, so the group is not always three rows.
  It is absent when playback controls are off or no controllable session
  exists. Then the ambient-light submenu:
  关闭氛围灯 or 开启氛围灯, the 封面 choice, and the six template choices, each
  with a round color preview. The choices are disabled while the light is off; the
  power row turns the light on, then a choice can be picked, matching the
  flyout and settings. No row uses a check column; the current ambient choice is disabled while lit and
  its label ends in a single-color ✔; every choice is disabled while off, where the
  stored choice carries 上次.
  While the light is on, the parent row's icon is the current preview; off
  uses the lamp-off glyph. Then 显示托盘面板, 设置…, a stateful update row (下载更新, 正在下载,
  重启并安装) only while an update waits (disabled while downloading), and 退出声渡. When more than six
  receivers exist, 更多接收器… opens the receivers page; when none is visible,
  重新搜索 with 暂无接收器 sits first. The menu is installed only for the
  duration of the popup and is rebuilt on every right click. Pinned by
  `TestTrayMenuShape`, `TestTrayMenuSessionRow`,
  `TestTrayMenuMediaRowsFollowSession`, `TestTrayMenuMediaControlSendsAction`,
  `TestTrayMenuLampSubmenu`, `TestTrayMenuLampOffDisablesChoices`,
  `TestTrayMenuUpdateRow`, `TestTrayMenuMoreReceivers`, and
  `TestTrayMenuEmptyState`.
- Device rows: every row is 66 DIP high whether or not it is a live target.
  A live row carries a 26-DIP channel icon button (a 16-DIP glyph) before the
  name and its own volume slider and percent; the button and the 14-DIP
  speaker share one lead, the name and the rail start at one x, and the
  connect action owns a centered trailing column fixed at one width whatever
  its label says. The speaker is a button: one click mutes the target (volume
  to zero, muted speaker shown), the next restores the level it muted from.
  The channel button cycles stereo → left → right; a stereo pair is a
  disabled stereo. A member keeps its mixer while the cohort re-establishes,
  so adding a device never takes the first device's controls away. A joining
  or reopening row is plain: its second line reads 正在连接… while joining and
  正在重连… while an automatic retry reopens it, and otherwise a plain row
  keeps its name, model, and action on the live row's 26/2/20 rhythm. The row's action is a table on the engine's `TargetState`: the
  joining row offers 取消, a member and a reopening target offer 断开连接, a
  failed target offers 重试, and a device with no role falls back to the
  phase. A rebuild names no joiner, so it offers no 取消. The row's 断开连接
  removes only that target (`DisconnectTarget` rebuilds the cohort with the
  rest; the last target ends the session), while the device head's 全部断开
  ends the whole session. The head reserves the action's 10-DIP height in every state, so the panel does not move when the button appears; pinned by `TestDeviceHeadKeepsItsHeight`.
  Pinned by
  `TestFlyoutDeviceRowCarriesItsOwnVolume`,
  `TestFlyoutVolumeButtonNamesItsAction`,
  `TestFlyoutChannelButtonNamesItsState`,
  `TestFlyoutPairChannelButtonIsFixed`, `TestNextChannelCycles`,
  `TestFlyoutRowsShareOneHeight`, `TestChannelIconFadesTheOtherSide`,
  `TestFlyoutJoiningRowHidesMixer`, `TestRebuildRowsOfferNoCancel`,
  `TestRowActionFollowsTargetState`,
  `TestFlyoutDisconnectAllButtonFollowsSession`,
  `TestConnectPublishesJoiningThenMember`, `TestAddPublishesMemberAndJoining`,
  `TestJoinAndRebuildPublishTheSameAudioFields`,
  `TestDisconnectJoiningTargetBeforeStreaming`,
  and `TestDisconnectTargetLeavesOthers`.
- Adaptive height: `flyoutHeightFor`/`flyoutHeightForState` derive the
  height from the device rows (66 each, 141 empty state), the latency row
  (44, while 托盘面板中显示 is on), the Now Playing card (150 while a session exists and
  `showNowPlayingPanel` is on), the ambient section (90 collapsed, plus
  53 when its preset grid expands), and the playback section's head,
  borders, and gap (33) when neither media nor the latency row is drawn. The
  base constant is 428, but the function subtracts the old 62-DIP media row
  and adds the 90-DIP lamp section, so three device rows with no media
  measure 500. The empty playback section is pinned by
  `TestFlyoutPlaybackSectionHidesWhenEmpty` and
  `TestFlyoutLatencySwitchCanHide`. There is no
  flexible spacer; the playback card follows the device card. Both windows
  carry the desktop text scale as a content zoom, so the flyout and settings
  sizes are multiplied by the scale when sized and shown
  (`window_scale.go`); a 125% scale makes every captured window 1.25x wider.
  The resize grows the flyout upward from the tray anchor (`fitFlyout`,
  `window_scale.go:15-31`), pinned by `TestFlyoutHeightForDeviceRows`,
  `TestFlyoutKeepsPlaybackGap`, and `TestFlyoutTargetHeightFitsPlayback`.
- Ambient light card: a permanent 44-DIP row under the playback card, with
  a 32-DIP preview, the source name (or the preset name), a 待机 chip while
  the standby holds the display dark, a compact 封面/模板 segment, and a
  rotating chevron only in template priority. Expanding shows a 10-per-row
  grid of 30-DIP swatches, up to four rows, scrolling past that. The card
  has no on/off switch; the power action turns the light on and off. See
  [ambient-light.md](ambient-light.md).
- Update chip: the footer's trailing 22-DIP chip opens the settings 更新
  page. The tray tooltip appends the available version while an update
  waits, and `Update ready` with no version once it is downloaded.
- Credential recovery: when the engine reports
  `Snapshot.CredentialReset`, a text-only 48-DIP bar with the connection
  error bar's inset and height sits under the device card. Its chevron
  opens a 380-DIP dialog with the reason, the re-pair step, the
  receiver-reset fallback, and 不再提示 / 重新配对. A successful pairing or
  the dismiss action clears the notice. Pinned by
  `TestCredentialResetNoticeAndDialog` and
  `TestCredentialResetNoticeAndRecovery`.
- Faces: idle-open while disconnected, smile while connected but silent,
  stream while streaming, connect while connecting, error on failure; the
  flyout footer shares them (`faceOf`).
- Silent start: login launch and `--tray` hide the panel; the tooltip is
  the localized brand plus the status while connecting or streaming
  (`声渡 · 正在通过 <name> 播放` under zh-CN, `Saundo · Streaming to <name>`
  under en), plus the update suffix while one is available or ready.
- Single instance: a second launch hands its command line over and exits 0.

## How to get to it (user POV)

Everything lives behind the tray icon (Windows; possibly in the hidden-icons
overflow, chevron ^ in the taskbar corner) or the menu-bar icon (macOS).

## Driving it

The tray icon and its menu are outside the automation protocol; on Windows
drive them through UI Automation and the doctor, on macOS through the
menu-bar icon. The flyout and the settings window it opens are drivable
through the e2e harness's element tree on both platforms.

```bash
powershell -NoProfile -ExecutionPolicy RemoteSigned -File .claude/skills/verify/scripts/doctor.ps1   # process, windows
mingw32-make e2e-build
bin/mygo-automation.exe open --socket "$(cygpath -w /tmp/saundo-e2e.sock)"
bin/mygo-automation.exe windows
bin/mygo-automation.exe snapshot -i
bin/mygo-automation.exe press @e4 --settle   # 设置 raises the settings window
bin/mygo-automation.exe screenshot /tmp/saundo-verify/settings.png --window 2
```

On macOS the same commands drop the `.exe` suffix and cygpath, and the
build starts under a redirected `HOME`:

```bash
make e2e-build
go build -o bin/mygo-automation github.com/egoist/mygo/plugins/automation/cmd/mygo-automation
mkdir -p /tmp/saundo-e2e-home
HOME=/tmp/saundo-e2e-home MYGO_AUTOMATION=/tmp/saundo-e2e.sock ./bin/saundo-e2e >/tmp/saundo-e2e.log 2>&1 &
bin/mygo-automation open --socket /tmp/saundo-e2e.sock
bin/mygo-automation press @e4 --settle   # 设置 raises the settings window
```

Start the e2e build with a redirected data dir (`APPDATA` on Windows, `HOME`
on macOS) and the automation socket variable, as SKILL.md step 2 shows. The
flyout starts hidden when its tray icon is in the overflow; hidden windows
still answer tree, act, and capture, but the PNG is the frame the window
would draw. The settings window gives a visible capture.

For the tray itself, find the icon through UI Automation: the tray button's class is
`SystemTray.NormalButton` and its accessible name is the localized brand
followed by the live tooltip (`声渡 声渡 · 正在通过 HomePods 播放` under
zh-CN, `Saundo ...` under en), so match the class and a name prefix, not an
exact `saundo`. Invoke it and watch the main window handle appear/vanish
(`MyGoFlyoutShadow`). A second launch must exit 0 and leave
one process —
checking `$?` after launching `bin/saundo.exe` documents the handover. Which
faces animate is pinned headless by `TestTrayAnimationFaces` (it does not pin
the 12 frames or their assets); the app toggle by
`TestTrayAnimationSettingPersists`, the engine command by
`TestTrayAnimationCommandUpdatesSnapshotOnly`, and the store default by
`TestTrayAnimationDefaultsOnAndRoundTrips`.

The tray-specific commands and the `MyGoFlyoutShadow`/`saundo.exe` paths
above are Windows-only. On macOS the e2e commands work as written above; the
tray icon itself is the menu bar, outside the automation protocol, so
capture it with `screencapture -x` and read the log at
`~/Library/Application Support/saundo/saundo.log`.

Proven end state on Windows: click toggles the panel (handle 0 ⇄ nonzero),
Escape dismisses it, the tooltip carries the live status, and exactly one
saundo.exe process exists throughout. The recorded live run covered an
outside click and a UIA invoke, not an Escape press. The e2e path covers the
panel's controls and the settings window; the tray click path stays UIA. On
macOS the e2e path is proven (2026-11-16): both windows listed, both trees
read, 设置 and the settings 连接 sidebar button pressed, and both windows
captured; the menu-bar click path is outside the protocol and unproven.

## Gotchas

- The tray icon face follows the phase (`faceOf`), so it does encode the
  session state; the tooltip is still the authoritative text.
- The flyout is frameless and transparent: a WS_POPUP/WS_EX_TOOLWINDOW
  window drawn through DirectComposition, not WS_EX_LAYERED. A plain
  screen grab can miss it; use PrintWindow (capture-window.ps1), and expect
  class `MyGoFlyoutShadow`. The panel is 420 DIP wide (`flyout.go:25`); the
  script is DPI-aware, so it captures the full window at the monitor scale
  (1050 px at 250% with 100% text scale; a higher text scale multiplies the
  width), not a cropped DIP-sized bitmap. 480 was an old design
  value. On macOS the
  rounded flyout is a nonactivating panel, so an open settings window does
  not come forward (`focus_darwin.go:5-8`).
- On Windows `showWindow` calls `mygo.App.Focus()` through `focus_other.go`;
  on macOS it is a no-op (`focus_darwin.go`). The Windows tray focus was not
  exercised live, so the proven end state does not cover it.
- A flyout hidden at capture time is a normal state (app in tray), not a
  failure — the doctor lists window absence as OK.
- `SetIcon` costs 0.5–0.8 ms per frame at 100 ms; do not "fix" explorer CPU
  noise sampled over seconds — the app-side cost stays under 1%.
- The tray menu exists only while its popup is open; enumerating menus
  outside the right-click window finds nothing.
- Credential recovery is unreachable from the UI: stop the app, corrupt
  `%APPDATA%\saundo\credentials.dat` (it is quarantined as
  `credentials.dat.corrupt-<UTC>`), and relaunch.
- The empty menu's 重新搜索 row appears when no receiver is visible; it calls
  the engine refresh. The old 连接上次接收器 fallback is gone. The cached menu
  is rebuilt on every right click, so media, lamp, and update rows are current
  even when no snapshot arrived.
- Reduce Motion (Windows animation effects off) holds the static face even
  with the switch on — that is required behavior, not a broken toggle.
- Device churn while the panel is open (rows appearing, height growing
  upward) is on the docs/verification.md pending list; a run that covers it
  should say so explicitly.
- The e2e build shares the single-instance lock with the release app. Stop
  the release instance first, and do not expect a second launch to be
  drivable.
- A hidden flyout still answers `act`, so a press can change state while the
  screenshot shows a built frame. Raise the settings window when the proof
  needs an on-screen draw.
- On Windows, the first press of the flyout's gear with `--settle` can time
  out because the hidden flyout keeps requesting frames. Press it again
  without `--settle`; the window then reports `visible:true` in
  `windows --json`.
  The macOS run settled immediately and printed `(no changes)`.
- On macOS the e2e binary is a plain executable, not an `.app` bundle, and
  still starts a full UI app with the flyout and settings windows; do not
  wrap it in a bundle just to drive it. The binaries are `bin/saundo-e2e`
  and `bin/mygo-automation`, with no `.exe` suffix.
