<!-- reviewed-through: 2026-11-01 -->

# UI design language

Scope: the tray flyout, the settings window, the PIN and password dialogs, the tray icon, and their motion. This page is the design contract for anyone adding or changing UI: which tokens and widgets to use, how things move, what the states look like, and how a change is verified without a human.

The values live in code, not here. `theme.go` owns the palette, `ui_kit.go` owns the shared widgets and motion primitives, `flyout.go` owns the geometry — the tests named below pin them. This page carries the rules and the inventory; when code and this page disagree, code wins and this page is updated in the same commit.

Sources: `mygo-cloudmusic` for native UI structure and style; Tuneblade for the device-list, volume, and connection-status layout (reference only, never code); magpie for the flyout and settings-window look (reference its look, never its code). The codebase-orientation side of the UI — which files, which windows — is in [implementation.md](implementation.md).

## Tokens and widgets

- Every color comes from the `palette` struct in `theme.go`, which maps the platform theme onto the design's light and dark palettes (`bg`, `panel`, `card`, `pill`, `line`, `fg`, `muted`, `accent`, `sel`, `green`, `red` families). Never hardcode a color; never branch on dark mode outside `theme.go`.
- Every repeated control comes from `ui_kit.go`: `sectionHead`, `iconButton`, `textButton` (a row action without a handler is a status, not a button — it takes no focus), `chipButton`, `primaryButton`, `quietButton`, `chip` (neutral/OK/accent), `toggle`, `slider`, `settingRow`, `rowLine`, `fieldInput`, `fieldSelect`, `staticText`. Do not rebuild these inline.
- One `phase` — idle, connecting, streaming, error — derives from the engine's finer-grained sender states (`phaseOf`) and drives the whole interface's color (`phaseColor`) and face (`face`). New UI speaks in phases, not in sender states.

## Motion

Rules:

- Durations use four tiers only: 120 ms micro feedback, 140 ms selection/switch, 160 ms page/content, 200 ms expand/collapse. Nothing over 240 ms except loading indicators.
- Default curve `EaseOut`; symmetric in/out pairs use `EaseInOut`; never `Bouncy`; never the default `Smooth`/`Snappy` (500 ms — too slow for this design) without an explicit short `Duration`.
- `ContentTransition`'s 350 ms default is overridden to 160 ms everywhere.
- Reduce Motion (the system setting) degrades automatically: poses land directly while fades stay, `Transition` lands directly while colors stay smooth, `Timeline` jumps to the end. `Loop` keeps running but is used only for loading and searching, never decoration.
- Animation keys are stable identities — the device ID for a device row, the section ID for a settings page, `"enter"`, `"sel"` — never an index.
- No business state changes inside animation callbacks; observe with `Settled`/`Arrived`/`Transitioning` instead.
- The first frame shows the final state. Only an explicit entrance or a user action starts an animation; `enterPose` and `dialogPose` run once per show through an entered flag.

Motion tokens:

| Token | API | Value | Reduce Motion |
|---|---|---|---|
| micro | `AnimateColor` / `AnimatePose` | 120 ms, EaseOut | colors keep; movement lands |
| select | `AnimateColor` | 140 ms, EaseOut | colors keep |
| content | `ContentTransition` | 160 ms, EaseOut | fade only |
| page | `Transition` | 160 ms, EaseOut | fade only |
| expand | `Transition` | 200 ms, EaseOut | fade only |
| enter | `AnimatePose` / `Transition` | 180 ms, EaseOut | fade only |
| switch / slider / toast | mygo built-ins | 140 / 150 / 200 ms | built-ins comply |
| pulse (loading) | `Loop` | 1.2 s, EaseInOut | keeps running |

Interaction inventory (what moves, and how):

| Interaction | Animation |
|---|---|
| Flyout and settings content entrance | rise 6 points + fade, 180 ms, once per show (`enterPose`) |
| Dialog panel | scale from 98% + fade, 150 ms (`dialogPose`) |
| Device row hover | background color, 120 ms |
| Device row selection | background `#ECEBFB`, dark `#33345A`, 140 ms |
| Device add/remove/order | `Transition` enter/exit collapse + fade, 200 ms, keyed by device ID |
| Row-tail action/status swap | text color 120 ms, content 160 ms |
| Now Playing change | content crossfade, 160 ms |
| Diagnostics log expand/collapse | collapse + fade, 200 ms |
| Settings page switch | slide 6 px + fade, 160 ms, keyed by section |
| Sidebar active pill | color 140 ms; a static pill, no sliding indicator |
| Input focus ring | color 120 ms; the keyboard focus ring never fades in |
| Empty/searching state | pulse loop 1.2 s on the search face, opacity 0.55–1 |
| Tray menu, window show/hide | none — native Windows and mygo own them |

Implementation notes: smoothing the selection color needs a custom row column, because `ui.List`'s built-in selection snaps. Stable keys are what let `Transition` recognize a row across reorder. `Collapse` folds height inside a column. `AnimatePose` and `ContentTransition` complete in paint-only frames on a steady flyout, so the resident window does not rebuild its view every frame.

## Windows and geometry

- The flyout is a frameless, shadowed 420-DIP-wide panel anchored to the tray icon by its bottom edge. Its height adapts to the device count so the gap between the device card and the playback card stays constant: base 500, three device rows at 56 each, a 140 empty state (`flyoutHeightFor` in `flyout.go`); a resize grows upward and leaves the playback card in place.
- The settings window is 720×500 (min 640×420) with the system title bar hidden; a custom bar draws under the system window buttons (`settingsTitleBar`), and a 190-wide sidebar switches four pages: 通用, 连接, 音频, 采集.
- The settings window paints one surface (`bg`) behind the title bar, the sidebar, and the content; `line2` borders separate them. The update window belongs to mygo's updater plugin: it keeps the OS title bar and the plugin's own Apple-style accent, outside this palette.
- Both windows read the same App state; an engine snapshot update redraws both.
- The flyout root paints the panel surface explicitly, because a flyout is transparent by default.

## State faces

- One smile mark carries the product. The tray icon and the flyout footer show the face whose mouth names the phase: `smile` at rest, `stream` (spectrum bars) while streaming, `connect` (three dots) while connecting or reconnecting, `error` (X eyes) on failure. The empty device card pulses the search face while discovery runs.
- The 32 px tray icon is monochrome — black on a light taskbar, white on a dark one — and redraws on `Tray.OnAppearanceChange`. It does not encode connection status; the tooltip and tray menu carry that.
- Device rows pick their icon by receiver model (`TestDeviceIconFollowsModel`).

## Verifying a UI change

- Render, don't guess: `ui.NewTester` drives a view headlessly. The preview hooks write PNGs — `SAUNDO_PREVIEW` (flyout), `SAUNDO_PREVIEW_SETTINGS` (settings window), and `SAUNDO_PREVIEW_DIR` (the state matrix: stream, idle, connecting, failed, empty, stereo).
- Pinning tests assert content and geometry: `TestViewRendersDevicesAndStatus`, `TestSettingsPagesRender`, `TestFlyoutKeepsPlaybackControlsOnly`, `TestStereoGroupRowRenders`, `TestNowPlayingRowShowsTrack`, `TestErrorStateShowsRetry`, `TestEmptyStateOffersSearch`, `TestFlyoutHeightForDeviceRows`, `TestFlyoutKeepsPlaybackGap`, `TestFlyoutTargetHeightFitsPlayback`, `TestPhaseOf`, `TestTrayIconFollowsPhase`.
- Keyboard and assistive access are part of the contract: `TestSettingsNavKeyboardChoosesPage`, `TestToggleTogglesWithKeyboard`, `TestSliderHasNameAndKeys` fail if a control loses its label or its keys.
- Idle budgets are pinned too: `TestFlyoutIdleRequestsNoFrames` and `TestSettingsIdleRequestsNoFrames` fail if an idle window starts repainting.
- After a motion change, run the acceptance checklist: durations within the four tiers; no `Bouncy` and no 500 ms defaults; Reduce Motion lands poses directly while fades and colors survive; stable keys, so a reorder does not replay entrances; first frame shows the final state; tray menu and window show/hide stay native. Then check the tray icon on light and dark taskbars, and toggle Windows' animation-effects setting for Reduce Motion.
