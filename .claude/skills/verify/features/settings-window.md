# Settings window

A separate `mygo.NewWindow` (`settings_window.go:82-108`): 720 DIP wide
(min 640×420), hidden title bar, created hidden at startup before the
engine starts. Title is localized: `设置 · 声渡` under zh-CN,
`Settings · saundo` under en — match windows by class `MyGoWindow` + pid,
or `-TitleLike` the active locale's brand string, never a hardcoded title.
Seven pages behind a 190 DIP sidebar, in nav order 通用, 连接, 接收器, 音频,
采集, 更新, 关于; each page body scrolls in the gutter at the window edge.

## Sub-features

- 通用 page: 发送端名称 (the name receivers show on their next connection),
  开机自动启动 (`HKCU\...\Run` value `saundo`), 托盘动画, language, and
  外观 (跟随系统/浅色/深色; persists as `theme` and repaints both windows at once).
- 连接 page: a status card for the current device, 断开后自动重连, and
  严格验证接收器. Launch auto-connect is not here — it is per receiver on
  the 接收器 page.
- 接收器 page (receiver registry, `receivers_page.go`): every remembered or
  manually added row with 启动时自动连接 (per receiver), hidden, and a
  0–2000 ms delay slider (500 ms debounced restart); per-receiver password
  row (masked, saved/cleared through the DPAPI store); add/edit/password/
  remove dialogs (`receivers_dialogs.go`). The 已启用 head renders only when
  the list has rows. An empty page is plain prose: one discovery line when
  nothing is found, or an all-hidden title plus the recovery hint when every
  row is hidden; the add row is the only plus.
- 音频 page: latency mode tiles (低延迟/标准/稳定/自定义), the custom slider
  (20–2000 ms, applies 500 ms after the last change), the 面板快捷切换
  header switch that shows or hides the flyout's latency segment row, and
  read-only format/sync rows.
- 采集 page: capture endpoint picker (跟随系统默认 or pinned) and per-app
  capture (application list built from window titles + audio sessions).
- 更新 page: update states, channel (stable/beta), release-history list —
  see [updates.md](updates.md).
- 关于 page: the identity card shows the app's smile mark, the product
  tagline, and the version line. Below it are the project links and the
  license/notices rows, and the version line never shows the git describe
  suffix.

## How to get to it (user POV)

设置 (gear) in the flyout header, or 设置… in the tray menu. The flyout
hides when the settings window opens; closing the settings window hides it
and the app stays in the tray.

## Driving it

Headless UI pins the split and rendering:

```bash
go test -run 'TestSettingsPagesRender|TestFlyoutKeepsPlaybackControlsOnly|TestSettingsAudioPageScrollsAtMinimumHeight|TestReceiverPageRendersGroupsAndRows|TestReceiverPageEmptyListShowsDiscoveryHint|TestReceiverPageAllHiddenShowsProse' -v ./internal/app/
```

Live app: open via the flyout's gear (and once via the tray menu — the
设置… entry is a separate path), confirm the window class is `MyGoWindow`,
the sidebar order is 通用/连接/接收器/音频/采集/更新, and the window is
~720 DIP wide. Flip one 接收器 toggle, close via the title-bar button
(process must stay alive, window reopens), restart the app and read the
value back from `receivers.json` on disk — never trust the UI echo.

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
- Passwords never render back — dots only (`receivers_page.go:528-531`). A
  page showing a stored password verbatim is a bug.
