<!-- reviewed-through: 2026-11-06 -->

# UI design language

Scope: the tray flyout, the settings window, the PIN and password dialogs, the tray icon, and their motion. This page is the design contract for anyone adding or changing UI: which tokens and widgets to use, how things move, what the states look like, and how a change is verified without a human.

The values live in code, not here. `theme.go` owns the palette, `ui_kit.go` owns the shared widgets and motion primitives, `flyout.go` owns the geometry — the tests named below pin them. This page carries the rules and the inventory; when code and this page disagree, code wins and this page is updated in the same commit.

Sources: `mygo-cloudmusic` for native UI structure and style; Tuneblade for the device-list, volume, and connection-status layout (reference only, never code); magpie for the flyout and settings-window look (reference its look, never its code). The codebase-orientation side of the UI — which files, which windows — is in [implementation.md](implementation.md).

## Tokens and widgets

- Every color comes from the `palette` struct in `theme.go`, which maps the platform theme onto the design's light and dark palettes (`bg`, `panel`, `card`, `pill`, `line`, `fg`, `muted`, `accent`, `sel`, `green`, `red` families). The platform theme is Windows' appearance unless 通用 → 外观 pins 浅色 or 深色; that choice is saved as `theme` in `settings.json` and applied through `mygo.Theme.SetSource` before the first window paints. Never hardcode a color; never branch on dark mode outside `theme.go`.
- Every repeated control comes from `ui_kit.go`: `sectionHead`, `iconButton`, `textButton` (a row action without a handler is a status, not a button — it takes no focus), `chipButton`, `primaryButton`, `quietButton`, `chip` (neutral/OK/accent), `toggle`, `slider`, `settingRow` (and `settingRowColors` for the greyed hidden group), `rowLine`, `fieldInput`, `fieldSelect`, `staticText`. Do not rebuild these inline.
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
| Now Playing card | updates in place; only the meter animates, 30 fps while the session plays |
| Diagnostics log expand/collapse | collapse + fade, 200 ms |
| Settings page switch | slide 6 px + fade, 160 ms, keyed by section |
| Sidebar active pill | color 140 ms; a static pill, no sliding indicator |
| Input focus ring | color 120 ms; the keyboard focus ring never fades in |
| Empty/searching state | pulse loop 1.2 s on the search face, opacity 0.55–1 |
| Tray icon | 12 frames at 100 ms while connecting or streaming; the static face otherwise |
| Tray menu, window show/hide | none — native Windows and mygo own them |

Implementation notes: smoothing the selection color needs a custom row column, because `ui.List`'s built-in selection snaps. Stable keys are what let `Transition` recognize a row across reorder. `Collapse` folds height inside a column. `AnimatePose` and `ContentTransition` complete in paint-only frames on a steady flyout, so the resident window does not rebuild its view every frame.

## Windows and geometry

- The flyout is a frameless, shadowed 420-DIP-wide panel anchored to the tray icon by its bottom edge. Its height adapts to the device count and the media state: base 474, three device rows at 56 each, a 141 empty state (`flyoutHeightFor` in `flyout.go`), and no flexible spacer, so the playback card follows the device card directly. The Now Playing media card adds its own 150 DIP while a session exists and is absent otherwise, so the panel shrinks when playback stops; the latency switch adds 44 DIP. Three device rows with a media session and the latency row make 606, and without a session 456.
- The Now Playing media card names the selected SMTC session in its head with the session-switch arrow, then lays out a 52-DIP-tall poster on the left whose width follows the cover's aspect ratio — a 16:9 thumbnail gets a wider poster, never a taller card — the title and artist beside it, and a five-bar meter at the title row's far right. The progress row runs full width: enabled transport controls first, as 24-DIP circular buttons in the session arrow's card style (same fill, muted 12-DIP glyph, hover; 4 DIP apart; the play/pause identified by its icon), then elapsed, the cover-field fill and thumb, and remaining. A session without a duration keeps the same row as an empty track, with no fill, thumb, times, or copy. With no session at all the card is not drawn: the playback card keeps the volume, latency, and capture lines, and `fitFlyout` shrinks the window by the card's height. Progress fill and meter sample a 23×24 blurred, lightness-compressed field of the cover, so the card follows the art's colors on both themes. An unknown app reads 未知应用. Until the first artwork arrives, the note tile uses the media card's own background, so it leaves no light edge around the poster. There are no app icons: the app name is the app's only identity. The 正在播放 settings page controls the card: the display master, the playback controls, the receiver track info, the cover colors, and the audio level. Master off hides the card (and shrinks the flyout) and dims the settings preview; the receiver track info is effective only while the master is on. The playback head's ALAC line carries the active receiver's effective latency: the global base plus that row's DSP/DAC delay, clamped to 20–4000 ms, e.g. `ALAC · 44.1 kHz · PTP · 已同步 · 延迟 700 ms`. An idle engine shows the base. While a latency change restarts the session, the flyout holds the last streaming snapshot for up to 3 s, so the device row, footer, capture line, and tray keep their streaming face; a spinner follows the 播放 label, so the right-aligned ALAC line never shifts. Preset clicks coalesce for 300 ms and custom edits for 500 ms before that restart, so rapid switching sends one restart with the final mode; the hold starts at the restart, not at the first click. An error, the timeout, or a user disconnect returns the real connecting state.
- The settings window is 720×500 (min 640×420) with the system title bar hidden; a custom bar draws under the system window buttons (`settingsTitleBar`), and a 190-wide sidebar switches eight pages: 通用, 连接, 接收器, 音频, 采集, 正在播放, 更新, 关于. The 正在播放 page is one card: the display master with the 系统媒体 badge, a preview of the real Now Playing card — the live session when present, otherwise a sample card so the preview never disappears (greyed and read-only when off), and rows for 播放控制, 接收器显示曲目信息, 封面配色, and 音频电平; the rows are disabled while the master is off. Its copy uses 正在播放 / 面板 / 播放控制 / 音频电平 / 封面配色 / 接收器显示曲目信息. The 音频 page's latency mode card has three preset tiles (低延迟 200 ms, 标准 1.5 s, 稳定 2 s) and a custom tile whose slider expands only while custom is selected; its header toggle 面板快捷切换 shows or hides the flyout's four-segment row. Each page body scrolls inside the fixed title area when the window is shorter than its content, so the 420 DIP minimum still reaches the custom slider and the read-only rows. The title block ends in a divider; the overlay scrollbar runs in the 20 DIP gutter at the window edge, clear of the cards, and the audio page's read-only rows use the shared `settingRow`.
- The 接收器 page is the receiver-management surface: a merged list card (remembered, manual, and stereo groups in one card) under the 已启用 group head with its count pill — the head and pill render only when the list has rows — a dashed 添加接收器 row with an accent plus tile and a one-line hint below the card, and a collapsible 已隐藏 group (same card background, greying the row's name, glyph, switch, and expanded settings) above a divider. An empty page is plain prose where the list would be, with no border or icon: no receivers at all shows one muted discovery line (网络上的 AirPlay 接收器会自动出现在这里。), every receiver hidden shows 所有接收器都已隐藏 at 12.5 px over the existing recovery hint, and the add row follows in both cases, then the hidden group. A receiver row expands in place: the head carries the glyph with the online dot, the name line with 手动添加 / 自动连接 / 已隐藏 chips and the model-or-address subtitle, an 编辑 side button on expanded manual rows, the row-tail enable switch (on = shown in the flyout, off = moved to the hidden group), and a rotating chevron. The edit button overlays the gap before the tail, so every row's switch keeps one x. The expanded body is full width, no inset panel: 启动时自动连接 (one target at a time; toggling one clears the others), the DSP/DAC 延迟 row (a 0–2000 ms slider with step 10, the 0 ms / 2000 ms scale beneath, and the value on the subtitle line), and the 接收器密码 row (a masked •••••• pill when saved, a key-icon 更改/设置 text button). Three dialogs open over the window with the shared `dialogScrim`: add/edit (name + type, host, port with its default following the type, password authentication select with a conditional password field, and a red 移除接收器 in the edit footer), password (a masked 新密码, the local-only hint, 清除已保存密码), and remove (red head, two hint lines, red 移除 commit). Every dialog keeps a reserved error row so its footer never jumps, and the footer buttons are the shared 34-DIP pills.
- The 关于 page's identity card shows the app's smile mark and the version line. The card is followed by the project links and the license card, both 50-DIP rows that open with the external-link glyph.
- The settings window paints one surface (`bg`) behind the title bar, the sidebar, and the content; `line2` borders separate them.
- There is no standalone update window. The settings 更新 page carries the whole update surface: the S1 card for available, downloading, ready, and failed states, the version row for checking and its result, and a sidebar dot on 更新 while an update waits. "更新内容" opens a read-only dialog over the settings window (`update_dialog.go`) with the shared `dialogScrim` behind it. It lists every version released since this build from the repository's localized changelog, `CHANGELOG.md` or `CHANGELOG.zh-CN.md`, chosen from the app language (`i18n.Locale`), over raw.githubusercontent (`update_history.go`): the offered version opens in full and the older ones follow as folded rows with their date and a count. The fetch starts when the update is found, so the dialog usually opens with the localized entry in place. Until it lands, a non-English dialog shows the loading row instead of flashing the manifest's English notes; the fetched entry replaces them, and the manifest stays the fallback when the fetch fails. A range field switches between that list and the latest version only. Notes render with `ui/markdown.View` under a dialog-sized theme, and version numbers use tabular figures. A found update also shows as the flyout chip and the tray tooltip; no window opens on its own.
- The settings 更新 page repeats the same state in an S1 card above its rows, and the flyout footer carries an update chip that opens that page. All three read one update state; installing from any of them shows the same progress, with `mygo.Updater` doing the signed download and install.
- All windows read the same App state; an engine snapshot update redraws them.
- The flyout root paints the panel surface explicitly, because a flyout is transparent by default.

## State faces

- One smile mark carries the product. The tray icon and the flyout footer show the face whose mouth names the phase: `idle-open` (open eyes, straight line) while disconnected, `smile` while the session is connected but silent, `stream` (spectrum bars) while streaming, `connect` (three dots) while connecting or reconnecting, `error` (X eyes) on failure. A connected stream with no audible audio shows `smile`: after two seconds of digital silence the engine suspends RTP and reports `Snapshot.AudioIdle`, so the face names playback, not the session. The empty device card pulses the search face while discovery runs.
- The tray animates the `stream` and `connect` faces at 12 frames per 100 ms while the 托盘动画 setting is on. Turning the setting off, or Windows animation effects (Reduce Motion), holds the static face. The static stream icon is animation frame 0, and the static connect icon is three uniform 2.6 px dots, so toggling does not change the mouth size.
- The 32 px tray icon is monochrome — black on a light taskbar, white on a dark one — and redraws on `Tray.OnAppearanceChange`. The tooltip and tray menu carry the finer session state.
- Device rows pick their icon by receiver model (`TestDeviceIconFollowsModel`).

## Verifying a UI change

- Render, don't guess: `ui.NewTester` drives a view headlessly. The preview hooks write PNGs — `SAUNDO_PREVIEW` (flyout), `SAUNDO_PREVIEW_SETTINGS` (settings window), and `SAUNDO_PREVIEW_DIR` (the state matrix: stream, now playing, idle, connecting, failed, empty, stereo).
- Pinning tests assert content and geometry: `TestViewRendersDevicesAndStatus`, `TestSettingsPagesRender`, `TestFlyoutKeepsPlaybackControlsOnly`, `TestStereoGroupRowRenders`, `TestNowPlayingCardShowsTrack`, `TestNowPlayingCardHidesWithoutMedia`, `TestNowPlayingSettingsPersist`, `TestNowPlayingPanelHiddenWhenOff`, `TestNowPlayingControlsSetting`, `TestNowPlayingPlaceholderBlendsWithTheCard`, `TestNowPlayingCardControlsSendActions`, `TestCoverFieldCompressesLightness`, `TestErrorStateShowsRetry`, `TestEmptyStateOffersSearch`, `TestFlyoutHeightForDeviceRows`, `TestFlyoutKeepsPlaybackGap`, `TestFlyoutTargetHeightFitsPlayback`, `TestPhaseOf`, `TestTrayIconFollowsPhase`, `TestFlyoutLatencySegmentSelects`, `TestAudioModeTileSelects`, `TestFlyoutLatencySwitchCanHide`, `TestFlyoutCustomLatencyOnALACLine`, `TestAudioCustomModeRendersSlider`, `TestApplyLatencyRestartsActiveSession`, `TestLatencyApplyHoldsStreamingSnapshot`, `TestLatencySpinnerKeepsALACLineAnchored`, `TestSettingsAudioPageScrollsAtMinimumHeight`, `TestLatencyModeDefaults`, `TestReceiverPageRendersGroupsAndRows`, `TestReceiverPageScrollsAtMinimumHeight`, `TestReceiverPageEmptyListShowsDiscoveryHint`, `TestReceiverPageAllHiddenShowsProse`, `TestReceiverExpandedRowShowsConfig`, `TestReceiverAddDialogRenders`, `TestReceiverAddDialogValidatesInPlace`, `TestReceiverEditDialogFillsAndSaves`, `TestReceiverRemoveDialogRenders`, `TestReceiverPasswordRowOpensItsDialog`, `TestReceiverEditButtonOpensEditWithoutCollapsing`, `TestReceiverDelayDraftFeedsTheSliderAndALACLine`, `TestFlyoutALACLineShowsActiveReceiverDelay`, `TestFlyoutFiltersHiddenReceivers`.
- Keyboard and assistive access are part of the contract: `TestSettingsNavKeyboardChoosesPage`, `TestToggleTogglesWithKeyboard`, `TestSliderHasNameAndKeys` fail if a control loses its label or its keys.
- Idle budgets are pinned too: `TestFlyoutIdleRequestsNoFrames` and `TestSettingsIdleRequestsNoFrames` fail if an idle window starts repainting.
- After a motion change, run the acceptance checklist: durations within the four tiers; no `Bouncy` and no 500 ms defaults; Reduce Motion lands poses directly while fades and colors survive; stable keys, so a reorder does not replay entrances; first frame shows the final state; tray menu and window show/hide stay native. Then check the tray icon on light and dark taskbars, and toggle Windows' animation-effects setting for Reduce Motion.
