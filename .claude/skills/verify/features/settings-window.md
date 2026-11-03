# Settings window

A separate 480-wide `mygo.NewWindow` titled `设置 · saundo`, six pages behind
a 190 DIP sidebar; each page body scrolls in the gutter at the window edge.

## Sub-features

- 连接 page: 发送端名称 (the name receivers show on their next connection),
  断开后自动重连, 启动时自动连接上次设备, 严格验证接收器, receiver password
  for the selected device (stored per device, never displayed back).
- 接收器 page (receiver registry): every remembered or manually added row
  with hidden, auto-connect, and delay settings; manual-receiver add/update/
  remove dialogs.
- 音频 page: latency mode tiles (低延迟/标准/稳定/自定义), the custom slider
  (20–2000 ms, applies 500 ms after the last change), read-only sync rows.
- 采集 page: capture endpoint picker (跟随系统默认 or pinned), per-app
  capture (application list with actively-rendering marks).
- 更新 page: update states, channel (stable/beta), release-history list —
  see [updates.md](updates.md).
- 通用 page: 开机自动启动 (`HKCU\...\Run` value `saundo`), 托盘动画,
  language, about.

## How to get to it (user POV)

设置 in the flyout header, or 设置… in the tray menu. The flyout hides when
the settings window opens; closing the settings window hides it and the app
stays in the tray.

## Driving it

Headless UI pins the split and rendering:

```bash
go test -run 'TestSettingsViewRendersConfiguration|TestFlyoutKeepsPlaybackControlsOnly|TestSettingsPagesRender|TestSettingsAudioPageScrollsAtMinimumHeight' -v ./internal/app/
```

Live app: open via the flyout's 设置 (and once via the tray menu — the
设置… entry is a separate path), confirm the window class is `MyGoWindow`
with title `设置 · saundo`, flip one toggle (e.g. 托盘动画), close with the
title-bar button, and confirm the process stays alive and the window
reopens. Persisted effects are read back from `%APPDATA%\saundo\settings.json`
or the receiver registry (copy them into `$EV`, don't trust the UI echo).

Proven end state: the change survives a settings-window close/reopen and an
app restart, and the corresponding engine command fired (log line or state
change in the flyout).

## Gotchas

- The settings window is created hidden at startup, before the engine
  starts; a snapshot-driven redraw of the open window was not separately
  exercised (on the docs/verification.md pending list).
- A settings change during a live session may restart the session
  (latency presets, receiver delay) with the 300/500 ms debounce — one
  restart with the final value is correct; one per click is a bug.
- The window title is localized (`设置 · saundo`); match with `-TitleLike
  "saundo"` in capture-window.ps1, not the full title.
- The audio page's custom slider debounces; a screenshot taken mid-debounce
  shows the old value with the new selection.
- Passwords never render back — a page showing a stored password verbatim
  is a bug; only "a password exists" may be visible.
