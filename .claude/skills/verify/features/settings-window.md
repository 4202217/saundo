# Settings window

A separate `mygo.NewWindow` (`settings_window.go:94`): a 720×500 page
area (the window frame adds a few DIPs; min 640×420), hidden title bar,
created hidden at startup before the engine starts. The title, cards, and
notes share one content column capped at 720 DIP and centered on a wider
window; the scroll gutter stays at the window edge. Title is localized: `设置 · 声渡` under zh-CN,
`Settings · Saundo` under en — match windows by class `MyGoWindow` + pid;
`-TitleLike` is case-sensitive and the old `"saundo"` matches neither
locale.
Nine pages behind a 190 DIP sidebar, in nav order 通用, 连接, 接收器, 音频,
氛围灯, 采集, 正在播放, 更新, 关于; each page body scrolls in the gutter at the window edge.

## Sub-features

- 通用 page: 发送端名称 (the name receivers show on their next connection),
  登录时自动启动 (a `HKCU\...\Run` value named `com.saundo.app` in the
  packaged build; a macOS login item on darwin), 托盘动画, language, and
  外观 (跟随系统/浅色/深色; persists as `theme` and repaints both windows at
  once). On macOS the page also carries the 托盘面板气泡样式 / Tray panel as
  popover row (`flyoutStyleSetting`); it is hidden on Windows.
- 连接 page: a status card for the current device, 断开后自动重连,
  严格验证接收器, and 连接后静音本机输出 (on by default). The mute row's
  subtitle follows the engine state (`MuteLocal`): off, waiting for a
  connection, muted now, or failed; the driver caveat under the card shows
  only while the switch is on. Launch auto-connect is not here — it is per receiver on the 接收器 page.
- 接收器 page (receiver registry, `receivers_page.go`): every remembered or
  manually added row with 启动时自动连接 (per receiver), hidden, and a
  0–2000 ms delay slider (500 ms debounced restart); per-receiver password
  row (masked, saved/cleared through the OS secret store: DPAPI on Windows,
  Keychain-backed AES on macOS); add/edit/password/
  remove dialogs (`receivers_dialogs.go`). The flyout and this list show
  manual receivers and online discovered receivers; an offline discovered
  receiver is hidden and returns with its saved settings. The active
  session's row stays reachable even when it just went offline. The 已启用
  head renders only when the list has rows. An empty page is plain prose:
  one discovery line when nothing is found, or an all-hidden title plus the
  recovery hint when every row is hidden; the add row is the only plus.
- 音频 page: latency mode tiles (低延迟/标准/稳定/自定义), the custom slider
  (20–2000 ms, applies 500 ms after the last change), the 托盘面板中显示
  header switch that shows or hides the flyout's latency segment row, and
  read-only format/sync rows. Below 714 DIP the three presets stack as
  full-width rows in the custom tile's shape; at every width they take the
  height their wrapped description needs, so no text leaves its card. The
  latency caveat sits under the card as an infoNote.
- 氛围灯 page: the top-display card with the HomePod 专属 tag and the power
  action, a 72-DIP preview of the current preset with its localized name, a
  颜色来源 row (封面 / 模板) whose segment sits at its right, a 模板 block
  whose six fixed 68-DIP tiles (暖烬, 沙丘, 苔庭, 潮汐, 兰夜, 月白) wrap on a
  narrow page, its 六组预设灯光颜色 description, and the Sound Glow reminder. Off dims the preview to
  35% and disables the grid. The page and the flyout's ambient card read the
  same global choice; see [ambient-light.md](ambient-light.md).
- 采集 page: capture endpoint picker (跟随系统默认 or pinned) and per-app
  capture (application list built from window titles + audio sessions). Both
  pickers are 220 DIP wide, for device and app names. The status keeps the
  row's right-hand column, capped at 55% and ellipsized so a long device name
  cannot squeeze the title; the full name is in its tooltip.
- 正在播放 page: one card. The header is the 媒体卡片 title with the
  托盘面板中显示 chip and the display switch (`showNowPlayingPanel`, on);
  below it a preview of the real media card (the live session, or a sample
  card when nothing plays); a live session's meter animates at 30 fps, and the
  sample's stays still. Then a 封面 row whose three-way segmented control
  sits at its right (无 / 默认 / 媒体, default media; persists as
  `coverMode`), then 播放控制 (`playbackControls`), 封面配色
  (`coverColors`), and 音频电平 (`audioLevel`). The receiver track info is always pushed; there
  is no row and no setting. The display switch only hides the flyout card; the
  preview and every row stay active. New
  `settings.json` keys default on; missing keys read their defaults; changes
  save immediately and resize the flyout.
- 更新 page: update states, channel (stable/beta), release-history list —
  see [updates.md](updates.md).
- 关于 page: the identity card shows the app's smile mark and the version
  line. Below it are the project links and the license/notices rows, and the
  version line never shows the git describe suffix.

## How to get to it (user POV)

设置 (gear) in the flyout header, or 设置… in the tray menu. The flyout
hides when the settings window opens; closing the settings window hides it
and the app stays in the tray.

## Driving it

Headless UI pins the split and rendering:

```bash
go test -run 'TestSettingsPagesRender|TestSettingsColumnCapsAndCenters|TestAudioPageKeepsLatencyTextInsideItsCards|TestLampTemplateTilesKeepTheirSize|TestNowPlayingCoverOptionsSitInTheRow|TestCaptureStatusRowKeepsItsTitle|TestConnectionPageMuteLocalStates|TestMuteLocalSettingPersists|TestFlyoutKeepsPlaybackControlsOnly|TestSettingsAudioPageScrollsAtMinimumHeight|TestReceiverPageRendersGroupsAndRows|TestReceiverPageEmptyListShowsDiscoveryHint|TestReceiverPageAllHiddenShowsProse|TestNowPlayingCoverModes|TestNowPlayingCoverModePersists|TestNowPlayingSettingsPersist|TestNowPlayingPanelHiddenWhenOff|TestNowPlayingControlsSetting' -v ./internal/app/
```

Settings defaults:

```bash
go test -run 'TestDefaultsEnableLocalMute|TestLampDefaultsOnAndCover|TestLampOldFileLoadsDefaults' -v ./internal/settings/
```

Live engine path for the mute (mutes the receiver first, drives the engine
directly, restores the local endpoint and the receiver's remembered volume):

```bash
SAUNDO_MUTE_TEST=1 go test -run TestLiveMuteLocal -v ./internal/engine/
```

Live app: open via the flyout's gear (and once via the tray menu — the
设置… entry is a separate path), confirm the window class is `MyGoWindow`,
the sidebar order is 通用/连接/接收器/音频/氛围灯/采集/正在播放/更新/关于, and the
window is ~720 DIP wide. Open 正在播放, turn 托盘面板中显示 off: the flyout
(reopened) has no media card and is shorter, while the preview stays live
and every option row stays active; the value persists in settings.json after
a restart. Flip one 接收器 toggle, close via the title-bar button
(process must stay alive, window reopens), restart the app and read the
value back from `receivers.json` on disk — never trust the UI echo. On 连接,
turn on 连接后静音本机输出 while streaming: the PC speakers mute, the
HomePod keeps playing, and the subtitle reads 本机已静音; disconnecting or
quitting restores the speakers, even when they were muted before the session.
A forced kill while muted leaves `mute-guard.json` under `%APPDATA%\saundo\`
on Windows (`~/Library/Application Support/saundo/` on macOS), and the next
launch releases the endpoint and removes the file.

Proven end state: the change survives a settings-window close/reopen and an
app restart, and the corresponding engine command fired (log line or state
change in the flyout).

## Gotchas

- A snapshot-driven redraw of the open settings window was not separately
  exercised (on the docs/verification.md pending list).
- A settings change during a live session may restart the session
  (latency presets, receiver delay) with the 300/500 ms debounce — one
  restart with the final value is correct; one per click is a bug.
- The window title follows the app locale (`设置 · 声渡` / `Settings ·
  Saundo`); capture-window.ps1 with `-TitleLike "saundo"` matches neither
  locale — use `-Class MyGoWindow` plus `-ProcId`.
- The audio page's custom slider debounces; a screenshot taken mid-debounce
  shows the old value with the new selection.
- The media card display switch only hides the flyout card; the settings
  preview and the option rows stay active. It is not the same as no media:
  the flyout card is hidden either way and the flyout shrinks the same amount,
  but the settings preview shows the live or sample card.
- The receiver track info is always pushed while streaming; the display
  switch does not gate it.
- Passwords never render back — dots only (`receivers_page.go:489-492`). A
  page showing a stored password verbatim is a bug.
