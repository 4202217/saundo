# Tray and flyout

## Sub-features

- Tray icon: black ink on a light taskbar, white on a dark one, re-drawn on
  appearance change; static face when 托盘动画 is off or Reduce Motion is on.
- Tray animation: 12 frames at 100 ms while connecting (three pulsing dots)
  or streaming (five bars riding a sine wave); eyes static.
- Left click toggles the flyout; click outside or Escape dismisses it; the
  app stays in the tray.
- Right click opens the menu: 显示面板, 设置…, 检查更新, 断开/连接上次设备,
  退出. The menu is installed only for the duration of the popup.
- Adaptive height: 500 DIP at three or more devices, 444 at two, 388 at one,
  470 when empty; resize grows the flyout upward from the tray anchor.
- Silent start: login launch and `--tray` hide the panel; the tooltip
  carries the status (`saundo · 正在播放到 <name>`).
- Single instance: a second launch hands its command line over and exits 0.

## How to get to it (user POV)

Everything lives behind the tray icon — possibly in the hidden-icons
overflow (chevron ^ in the taskbar corner).

## Driving it

Live app only — there is no headless harness for the tray:

```bash
powershell -NoProfile -ExecutionPolicy Bypass -File .claude/skills/verify/scripts/doctor.ps1   # process, windows
powershell -NoProfile -ExecutionPolicy Bypass -File .claude/skills/verify/scripts/capture-window.ps1 -Out /tmp/saundo-verify/flyout.png
```

Find the icon through UI Automation: the overflow flyout exposes a `saundo`
button; invoke it and watch the main window handle appear/vanish
(`MyGoFlyoutShadow`). A second launch must exit 0 and leave one process —
`echo $?` after `bin/saundo.exe` documents the handover. Animation frames
are pinned headless by `TestTrayAnimationFaces`; the toggle paths by
`TestTrayAnimationDefaultsOnAndRoundTrips`.

Proven end state: click toggles the panel (handle 0 ⇄ nonzero), Escape
dismisses it, the tooltip carries the live status, and exactly one
saundo.exe process exists throughout.

## Gotchas

- The tray icon does not encode connection status — judge by tooltip and
  tray menu, not by the icon's face.
- The flyout is frameless and layered: a plain screen grab can miss it;
  use PrintWindow (capture-window.ps1), and expect class
  `MyGoFlyoutShadow`.
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
