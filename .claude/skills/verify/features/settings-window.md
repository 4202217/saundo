# Settings window

A separate `mygo.NewWindow` (`settings_window.go:82-108`): 720 DIP wide
(min 640×420), hidden title bar, created hidden at startup before the
engine starts. Title is localized: `设置 · 声渡` under zh-CN,
`Settings · saundo` under en — match windows by class `MyGoWindow` + pid,
or `-TitleLike` the active locale's brand string, never a hardcoded title.
Eight pages behind a 190 DIP sidebar, in nav order 通用, 连接, 接收器, 音频,
采集, 正在播放, 更新, 关于; each page body scrolls in the gutter at the window edge.

## Sub-features

- 通用 page: 发送端名称 (the name receivers show on their next connection),
  开机自动启动 (`HKCU\...\Run` value `saundo`), 托盘动画, language, and
  外观 (跟随系统/浅色/深色; persists as `theme` and repaints both windows at once).
- 连接 page: a status card for the current device, 断开后自动重连,
  严格验证接收器, and 连接后静音本机输出. The mute row's subtitle follows
  the engine state (`MuteLocal`): off, waiting for a connection, muted now,
  or failed; the driver caveat under the card shows only while the switch is
  on. Launch auto-connect is not here — it is per receiver on the 接收器 page.
- 接收器 page (receiver registry, `receivers_page.go`): every remembered or
  manually added row with 启动时自动连接 (per receiver), hidden, and a
  0–2000 ms delay slider (500 ms debounced restart); per-receiver password
  row (masked, saved/cleared through the DPAPI store); add/edit/password/
  remove dialogs (`receivers_dialogs.go`). The flyout and this list show
  manual receivers and online discovered receivers; an offline discovered
  receiver is hidden and returns with its saved settings. The active
  session's row stays reachable even when it just went offline. The 已启用
  head renders only when the list has rows. An empty page is plain prose:
  one discovery line when nothing is found, or an all-hidden title plus the
  recovery hint when every row is hidden; the add row is the only plus.
- 音频 page: latency mode tiles (低延迟/标准/稳定/自定义), the custom slider
  (20–2000 ms, applies 500 ms after the last change), the 面板快捷切换
  header switch that shows or hides the flyout's latency segment row, and
  read-only format/sync rows.
- 采集 page: capture endpoint picker (跟随系统默认 or pinned) and per-app
  capture (application list built from window titles + audio sessions).
- 正在播放 page: one card. The header is the display master 显示正在播放
  (`showNowPlayingPanel`, on) with the 系统媒体 badge; below it a preview of the
  real Now Playing card (the live session, or a sample card when nothing
  plays); a live session's meter animates at 30 fps while the master is on,
  and the sample's stays still. Then come the rows 封面 (a three-way
  latency-style segmented control: 无封面 / 默认封面 / 媒体封面, default
  media; persists as `coverMode`), 播放控制 (`playbackControls`),
  封面配色 (`coverColors`), 音频电平 (`audioLevel`), and last
  在接收器上显示曲目信息 (`showNowPlaying`, the existing engine setting,
  off); the receiver push is last because it leaves the panel untouched.
  Master off greys the
  preview at 45% and disables every row; the dependent values stay. The
  receiver row is effective only while the master is on. New `settings.json`
  keys default on; missing keys read their defaults; changes save immediately
  and resize the flyout.
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
go test -run 'TestSettingsPagesRender|TestConnectionPageMuteLocalStates|TestMuteLocalSettingPersists|TestFlyoutKeepsPlaybackControlsOnly|TestSettingsAudioPageScrollsAtMinimumHeight|TestReceiverPageRendersGroupsAndRows|TestReceiverPageEmptyListShowsDiscoveryHint|TestReceiverPageAllHiddenShowsProse|TestNowPlayingCoverModes|TestNowPlayingCoverModePersists|TestNowPlayingSettingsPersist|TestNowPlayingPanelHiddenWhenOff|TestNowPlayingControlsSetting' -v ./internal/app/
```

Live engine path for the mute (mutes the receiver first, drives the engine
directly, restores the local endpoint and the receiver's remembered volume):

```bash
SAUNDO_MUTE_TEST=1 go test -run TestLiveMuteLocal -v ./internal/engine/
```

Live app: open via the flyout's gear (and once via the tray menu — the
设置… entry is a separate path), confirm the window class is `MyGoWindow`,
the sidebar order is 通用/连接/接收器/音频/采集/正在播放/更新/关于, and the
window is ~720 DIP wide. Open 正在播放, flip the master off: the preview greys,
the rows disable, and the flyout (reopened) has no media card and is shorter;
the value persists in settings.json after a restart. Flip one 接收器 toggle, close via the title-bar button
(process must stay alive, window reopens), restart the app and read the
value back from `receivers.json` on disk — never trust the UI echo. On 连接,
turn on 连接后静音本机输出 while streaming: the PC speakers mute, the
HomePod keeps playing, and the subtitle reads 本机已静音; disconnecting or
quitting restores the speakers. A forced kill while muted leaves
`mute-guard.json` under `%APPDATA%\saundo\`, and the next launch restores the
endpoint and removes the file.

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
  saundo`); capture-window.ps1 with `-TitleLike "saundo"` only matches the
  en locale — prefer `-Class MyGoWindow` plus `-ProcId`.
- The audio page's custom slider debounces; a screenshot taken mid-debounce
  shows the old value with the new selection.
- The 正在播放 master off is not the same as no media: the media card is
  hidden either way and the flyout shrinks the same amount, but the settings
  page still shows the greyed preview and the retained option values.
- The receiver track info only reaches the receiver while both the master and
  the receiver row are on.
- Passwords never render back — dots only (`receivers_page.go:528-531`). A
  page showing a stored password verbatim is a bug.
