<!-- reviewed-through: 2026-11-03 -->

# UI design language

Scope: the tray flyout, the settings window, the PIN and password dialogs, the tray icon, and their motion. This page is the design contract for anyone adding or changing UI: which tokens and widgets to use, how things move, what the states look like, and how a change is verified without a human.

The values live in code, not here. `theme.go` owns the palette, `ui_kit.go` owns the shared widgets and motion primitives, `flyout.go` owns the geometry — the tests named below pin them. This page carries the rules and the inventory; when code and this page disagree, code wins and this page is updated in the same commit.

Sources: `mygo-cloudmusic` for native UI structure and style; Tuneblade for the device-list, volume, and connection-status layout (reference only, never code); magpie for the flyout and settings-window look (reference its look, never its code). The codebase-orientation side of the UI — which files, which windows — is in [implementation.md](implementation.md).

## Tokens and widgets

- Every color comes from the `palette` struct in `theme.go`, which maps the platform theme onto the design's light and dark palettes (`bg`, `panel`, `card`, `pill`, `line`, `fg`, `muted`, `accent`, `sel`, `green`, `red` families). Never hardcode a color; never branch on dark mode outside `theme.go`.
- Every repeated control comes from `ui_kit.go`: `sectionHead`, `iconButton`, `textButton` (a row action without a handler is a status, not a button — it takes no focus), `chipButton`, `primaryButton`, `quietButton`, `chip` (neutral/OK/accent), `toggle`, `slider`, `settingRow`, `rowLine`, `fieldInput`, `fieldSelect`, `staticText`. Do not rebuild these inline.
- One `phase` — idle, connecting, streaming, error — derives from the engine's finer-grained sender states (`phaseOf`) and drives the whole interface's color (`phaseColor`). `face` adds one distinction inside streaming: a stream with no audible audio shows the resting face. New UI speaks in phases, not in sender states.

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
| Latency preset segment or tile | selected color 140 ms; the click updates the ALAC line at once, the restart waits 300 ms for the last click, and the hold keeps the streaming faces for up to 3 s while a spinner follows the 播放 label |
| Custom latency slider | built-in 150 ms; the restart waits 500 ms after the last change |
| Latency panel switch | switch 140 ms; the flyout height changes by 44 DIP |
| Row-tail action/status swap | text color 120 ms, content 160 ms |
| Now Playing change | content crossfade, 160 ms |
| Diagnostics log expand/collapse | collapse + fade, 200 ms |
| Settings page switch | slide 6 px + fade, 160 ms, keyed by section |
| Sidebar active pill | color 140 ms; a static pill, no sliding indicator |
| Input focus ring | color 120 ms; the keyboard focus ring never fades in |
| Empty/searching state | pulse loop 1.2 s on the search face, opacity 0.55–1 |
| Tray icon | 12 frames at 100 ms while connecting or streaming; the static face otherwise |
| Tray menu, window show/hide | none — native Windows and mygo own them |

Implementation notes: smoothing the selection color needs a custom row column, because `ui.List`'s built-in selection snaps. Stable keys are what let `Transition` recognize a row across reorder. `Collapse` folds height inside a column. `AnimatePose` and `ContentTransition` complete in paint-only frames on a steady flyout, so the resident window does not rebuild its view every frame.

## Windows and geometry

- The flyout is a frameless, shadowed 420-DIP-wide panel anchored to the tray icon by its bottom edge. Its height adapts to the device count so the gap between the device card and the playback card stays constant: base 500, three device rows at 56 each, a 140 empty state (`flyoutHeightFor` in `flyout.go`); a resize grows upward and leaves the playback card in place. With the latency switch on (`ShowLatencySwitch`), the four-segment mode row adds 44 DIP, so three device rows make 544. The playback head's ALAC line carries the current latency, e.g. `ALAC · 44.1 kHz · PTP · 已同步 · 延迟 700 ms`. While a latency change restarts the session, the flyout holds the last streaming snapshot for up to 3 s, so the device row, footer, capture line, and tray keep their streaming face; a spinner follows the 播放 label, so the right-aligned ALAC line never shifts. Preset clicks coalesce for 300 ms and custom edits for 500 ms before that restart, so rapid switching sends one restart with the final mode; the hold starts at the restart, not at the first click. An error, the timeout, or a user disconnect returns the real connecting state.
- The settings window is 720×500 (min 640×420) with the system title bar hidden; a custom bar draws under the system window buttons (`settingsTitleBar`), and a 190-wide sidebar switches five pages: 通用, 连接, 音频, 采集, 更新. The 音频 page's latency mode card has three preset tiles (低延迟 200 ms, 标准 1.5 s, 稳定 2 s) and a custom tile whose slider expands only while custom is selected; its header toggle 面板快捷切换 shows or hides the flyout's four-segment row. Each page body scrolls inside the fixed title area when the window is shorter than its content, so the 420 DIP minimum still reaches the custom slider and the read-only rows. The title block ends in a divider; the overlay scrollbar runs in the 20 DIP gutter at the window edge, clear of the cards, and the audio page's read-only rows use the shared `settingRow`.
- The settings window paints one surface (`bg`) behind the title bar, the sidebar, and the content; `line2` borders separate them.
- There is no standalone update window. The settings 更新 page carries the whole update surface: the S1 card for available, downloading, ready, and failed states, the version row for checking and its result, and a sidebar dot on 更新 while an update waits. "更新内容" opens a read-only dialog over the settings window (`update_dialog.go`) with the shared `dialogScrim` behind it. It lists every version released since this build: the offered one from the signed manifest, then older ones read from the repository's localized changelog, `CHANGELOG.md` or `CHANGELOG.zh-CN.md`, chosen from `App.Locale`, over raw.githubusercontent (`update_history.go`) as folded rows with their date and a count; a range field switches between that list and the latest version only. Notes render with `ui/markdown.View` under a dialog-sized theme, and version numbers use tabular figures. A found update also shows as the flyout chip and the tray tooltip; no window opens on its own.
- The settings 更新 page repeats the same state in an S1 card above its rows, and the flyout footer carries an update chip that opens that page. All three read one update state; installing from any of them shows the same progress, with `mygo.Updater` doing the signed download and install.
- All windows read the same App state; an engine snapshot update redraws them.
- The flyout root paints the panel surface explicitly, because a flyout is transparent by default.

## State faces

- One smile mark carries the product. The tray icon and the flyout footer show the face whose mouth names the phase: `smile` at rest, `stream` (spectrum bars) while streaming, `connect` (three dots) while connecting or reconnecting, `error` (X eyes) on failure. A connected stream with no audible audio also shows `smile`: after two seconds of digital silence the engine suspends RTP and reports `Snapshot.AudioIdle`, so the face names playback, not the session. The empty device card pulses the search face while discovery runs.
- The tray animates the `stream` and `connect` faces at 12 frames per 100 ms while the 托盘动画 setting is on. Turning the setting off, or Windows animation effects (Reduce Motion), holds the static face. The static stream icon is animation frame 0, and the static connect icon is three uniform 2.6 px dots, so toggling does not change the mouth size.
- The 32 px tray icon is monochrome — black on a light taskbar, white on a dark one — and redraws on `Tray.OnAppearanceChange`. The tooltip and tray menu carry the finer session state.
- Device rows pick their icon by receiver model (`TestDeviceIconFollowsModel`).

## Verifying a UI change

- Render, don't guess: `ui.NewTester` drives a view headlessly. The preview hooks write PNGs — `SAUNDO_PREVIEW` (flyout), `SAUNDO_PREVIEW_SETTINGS` (settings window), and `SAUNDO_PREVIEW_DIR` (the state matrix: stream, idle, connecting, failed, empty, stereo).
- Pinning tests assert content and geometry: `TestViewRendersDevicesAndStatus`, `TestSettingsPagesRender`, `TestFlyoutKeepsPlaybackControlsOnly`, `TestStereoGroupRowRenders`, `TestNowPlayingRowShowsTrack`, `TestErrorStateShowsRetry`, `TestEmptyStateOffersSearch`, `TestFlyoutHeightForDeviceRows`, `TestFlyoutKeepsPlaybackGap`, `TestFlyoutTargetHeightFitsPlayback`, `TestPhaseOf`, `TestTrayIconFollowsPhase`, `TestFlyoutLatencySegmentSelects`, `TestAudioModeTileSelects`, `TestFlyoutLatencySwitchCanHide`, `TestFlyoutCustomLatencyOnALACLine`, `TestAudioCustomModeRendersSlider`, `TestApplyLatencyRestartsActiveSession`, `TestLatencyApplyHoldsStreamingSnapshot`, `TestLatencySpinnerKeepsALACLineAnchored`, `TestSettingsAudioPageScrollsAtMinimumHeight`, `TestLatencyModeDefaults`.
- Keyboard and assistive access are part of the contract: `TestSettingsNavKeyboardChoosesPage`, `TestToggleTogglesWithKeyboard`, `TestSliderHasNameAndKeys` fail if a control loses its label or its keys.
- Idle budgets are pinned too: `TestFlyoutIdleRequestsNoFrames` and `TestSettingsIdleRequestsNoFrames` fail if an idle window starts repainting.
- After a motion change, run the acceptance checklist: durations within the four tiers; no `Bouncy` and no 500 ms defaults; Reduce Motion lands poses directly while fades and colors survive; stable keys, so a reorder does not replay entrances; first frame shows the final state; tray menu and window show/hide stay native. Then check the tray icon on light and dark taskbars, and toggle Windows' animation-effects setting for Reduce Motion.
