# Tray and flyout

## Sub-features

- Tray icon: black ink on a light taskbar, white on a dark one, re-drawn on
  appearance change; static face when 托盘动画 is off or Reduce Motion is on.
- Tray animation: 12 frames at 100 ms (`tray_anim.go:11-12`) while
  connecting (three pulsing dots) or streaming (five bars riding a sine
  wave); eyes static.
- Left click toggles the flyout; click outside or Escape dismisses it; the
  app stays in the tray.
- Right click opens the menu: 显示托盘面板, 设置…, 检查更新…, 断开连接 or
  连接上次接收器, 退出声渡. The menu is installed only for the duration of
  the popup.
- Adaptive height: `flyoutHeightFor`/`flyoutHeightForState` derive the
  height from the device rows (56 each, 141 empty state), the latency row
  (44), the Now Playing card (150 while a session exists and
  `showNowPlayingPanel` is on), and the ambient section (90 collapsed, plus
  53 when its preset grid expands). The base constant is 459, but the
  function subtracts the old 62-DIP media row and adds the 90-DIP lamp
  section, so three device rows with no media measure 487. There is no
  flexible spacer; the playback card follows the device card. The resize
  grows the flyout upward from the tray anchor (`fitFlyout`,
  `app.go:769-779`), pinned by `TestFlyoutHeightForDeviceRows`,
  `TestFlyoutKeepsPlaybackGap`, and `TestFlyoutTargetHeightFitsPlayback`.
- Ambient light card: a permanent 44-DIP row under the playback card, with
  a 32-DIP preview, the preset name, and a rotating chevron.
  Expanding shows a 10-per-row grid of 30-DIP swatches, up to four rows,
  scrolling past that. The card has no on/off switch; the power action dims
  the top display, and the settings page owns the light choice. See
  [ambient-light.md](ambient-light.md).
- Faces: idle-open while disconnected, smile while connected but silent,
  stream while streaming, connect while connecting, error on failure; the
  flyout footer shares them (`faceOf`).
- Silent start: login launch and `--tray` hide the panel; the tooltip is
  the localized brand plus the status while connecting or streaming
  (`声渡 · 正在通过 <name> 播放` under zh-CN, `Saundo · Streaming to <name>`
  under en).
- Single instance: a second launch hands its command line over and exits 0.

## How to get to it (user POV)

Everything lives behind the tray icon — possibly in the hidden-icons
overflow (chevron ^ in the taskbar corner).

## Driving it

Live app only — there is no headless harness for the tray:

```bash
powershell -NoProfile -ExecutionPolicy RemoteSigned -File .claude/skills/verify/scripts/doctor.ps1   # process, windows
powershell -NoProfile -ExecutionPolicy RemoteSigned -File .claude/skills/verify/scripts/capture-window.ps1 -Out /tmp/saundo-verify/flyout.png
```

Find the icon through UI Automation: the overflow flyout names the button
by its first tooltip, which is the localized brand (`声渡` under zh-CN,
`Saundo` under en), not `saundo`. Invoke it and watch the main window handle
appear/vanish (`MyGoFlyoutShadow`). A second launch must exit 0 and leave
one process —
checking `$?` after launching `bin/saundo.exe` documents the handover. Animation frames
are pinned headless by `TestTrayAnimationFaces`; the app toggle by
`TestTrayAnimationSettingPersists`, the engine command by
`TestTrayAnimationCommandUpdatesSnapshotOnly`, and the store default by
`TestTrayAnimationDefaultsOnAndRoundTrips`.

The commands and the `MyGoFlyoutShadow`/`saundo.exe` paths above are
Windows-only. On macOS use the menu-bar icon, `screencapture -x`, and the
log at `~/Library/Application Support/saundo/saundo.log`.

Proven end state: click toggles the panel (handle 0 ⇄ nonzero), Escape
dismisses it, the tooltip carries the live status, and exactly one
saundo.exe process exists throughout. The recorded live run covered an
outside click and a UIA invoke, not an Escape press.

## Gotchas

- The tray icon face follows the phase (`faceOf`), so it does encode the
  session state; the tooltip is still the authoritative text.
- The flyout is frameless and transparent: a WS_POPUP/WS_EX_TOOLWINDOW
  window drawn through DirectComposition, not WS_EX_LAYERED. A plain
  screen grab can miss it; use PrintWindow (capture-window.ps1), and expect
  class `MyGoFlyoutShadow`. The panel is 420 DIP wide (`flyout.go:26`) — a
  capture reporting ~420 px (or a 1.25× multiple of it under DPI
  virtualization) is correct; 480 was an old design value. On macOS the
  rounded flyout is a nonactivating panel, so an open settings window does
  not come forward (`flyout_window.go`).
- On Windows `showWindow` calls `mygo.App.Focus()` through `focus_other.go`;
  on macOS it is a no-op (`focus_darwin.go`). The Windows tray focus was not
  exercised live, so the proven end state does not cover it.
- A flyout hidden at capture time is a normal state (app in tray), not a
  failure — the doctor lists window absence as OK.
- `SetIcon` costs 0.5–0.8 ms per frame at 100 ms; do not "fix" explorer CPU
  noise sampled over seconds — the app-side cost stays under 1%.
- The tray menu exists only while its popup is open; enumerating menus
  outside the right-click window finds nothing.
- Reduce Motion (Windows animation effects off) holds the static face even
  with the switch on — that is required behavior, not a broken toggle.
- Device churn while the panel is open (rows appearing, height growing
  upward) is on the docs/verification.md pending list; a run that covers it
  should say so explicitly.
