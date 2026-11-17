---
name: ui-mockup
description: This skill should be used when the user asks to "mock up a UI change", "show me design variants", "decide this with mockups", or when a saundo page, dialog, empty state, or interaction has more than one defensible shape. It picks one of four instruments (HTML sketch, native preview, platform render, live verify), builds it, compares the render, and graduates the approved state through the catalog into production.
---

# Decide a saundo UI change

Use rendered evidence to settle the shape before production code. The mockup is throwaway evidence. The commit is the durable record.

The UI has one fixture and one state catalog. Every instrument reads them. A state keeps one ID from the sketch overlay through the catalog, the wasm preview, the CPU matrix, the platform render, the site projection, and the verify map.

- Fixture: `internal/app/ui_fixture.go`.
- Catalog: `internal/app/ui_catalog*.go`.
- Landing page projection: `site/assets/demo-data.js`.
- Canonical pixels: `docs/ui-baseline/`.

## The four instruments

| Instrument | Answers | Use it for | Command |
|---|---|---|---|
| Sketch (HTML) | What should it look like before Go exists? | An open shape, variants, native menu and window chrome, file dialogs, shortcuts | `check-ui sketch`, headless Chrome |
| Native preview (wasm) | How does the real view behave when clicked? | Flyout, settings, dialogs, the tray menu panel | `go tool mygo dev -web` |
| Platform render | What pixels does the user see on Windows or macOS? | The final visual judgement | `SAUNDO_PREVIEW_RENDER` test, `check-ui render`, `check-ui diff` |
| Verify (live app) | Does the shipped app do it? | Tray, native chrome, discovery, audio, persistence | `.claude/skills/verify/` |

The browser is not pixel truth. It uses browser fonts and WebGL 2. The platform renderer is the visual truth. The live app is the only authority for the tray, system windows, and audio.

The sketch and the site are hand-built second implementations. The token gate pins their palette, checked metrics, flyout height rule, and window sizes. The wording gate pins the flyout copy the site carries. Nothing pins shape.

## When to use

- A new page, dialog, empty state, or interaction with more than one defensible shape.
- A visual bug whose fix could be a restyle or a removal.
- The user asks for mockups, variants, or a design comparison.
- A decision that ships on the flyout, settings, a dialog, or the tray menu panel.

Skip it for a one-line copy change, a spacing fix with an obvious target, or a behavior change with no visual decision.

The release refreshes both templates and regenerates the baseline. A later sketch starts from that baseline, so the release needs no manual comparison of three artifacts.

## Choose the instrument

- **Native preview.** The decision will ship and the surface runs in the browser target: the flyout, the settings window, a dialog, or the tray menu panel. It runs the real views with the catalog. The human clicks the wasm build. The agent inspects `render.Still` PNGs from the same catalog.
- **HTML sketch.** The shape is still open, two or three layouts are defensible, or the question is about a surface no instrument draws in-product: the native tray menu chrome, window chrome, file dialogs, global shortcuts. It is the cheapest way to compare directions.
- **Platform render.** The shape is chosen and the question is what the shipping renderer draws. Render the catalog states through `render.Still`, not the browser.
- **Verify.** The claim is about the shipped app. Use the feature page under `.claude/skills/verify/features/`.

For a shipping design, use both the sketch and the native preview. The sketch chooses a direction. The preview and the platform render prove the chosen one.

## The graduation path

A design decision moves through four stages, and the ID moves with it.

1. **Sketch overlay.** Start from the approved baseline. Add a delta that holds only the new states, copy, and layout. It may invent data the engine cannot reach yet. A state that never ships is deleted here.
2. **Catalog.** The approved delta becomes a `Scenario` in `internal/app/ui_catalog*.go`. The invented data either enters the fixture or is deleted. The `ID` is the join key and never changes when a file name or page title changes.
3. **Preview and tests.** The scenario renders in wasm, through the CPU matrix, and through the platform renderer. A behavior test pins the transition. The scenario joins the canonical pixel set only through an explicit baseline change.
4. **Verify.** The feature page names the scenario ID and the live check.

A shipped state without a catalog entry is a gate failure.

## Sketch baseline and overlay

Start a round with the command. It copies the current approved template into the topic folder, creates a working copy, and writes the delta document.

```bash
go run ./scripts/check-ui sketch -topic <topic> -surface settings
go run ./scripts/check-ui sketch -topic <topic> -surface flyout -force
# After a release refreshed the templates, replace the baseline and keep the work:
go run ./scripts/check-ui sketch -topic <topic> -surface settings -force
```

The first run creates the topic. The second surface uses `-force` for the same topic; it writes its own baseline and overlay and leaves the first surface's files alone.

The folder `.audit/mockups/<topic>/` then holds:

- `baseline-settings.html` or `baseline-flyout.html`: an exact copy of the current template. Do not edit it.
- `mockup.html` (settings) or `flyout.html` (flyout): the working copy. Edit only the marked blocks.
- `overlay-settings.md` or `overlay-flyout.md`: the design delta. Fill in the proposed scenario IDs, the target, normal, and edge states, the copy in both languages, the layout changes, the rejected alternatives, and the render paths.

The names carry the surface, so one topic can hold both a settings and a flyout baseline. The release uses one topic for both.

The real delta is the diff between the two files:

```bash
git diff --no-index .audit/mockups/<topic>/baseline-settings.html .audit/mockups/<topic>/mockup.html
```

The overlay never carries tokens or shell geometry. The token gate owns those. With `-force`, the baseline is replaced and the working copy and overlay survive, so a release refresh does not cost a design round.

Render a state with headless Chrome. The settings template selects `?view=target|normal|edge&lang=zh|en&dark=0|1&zoom=1|1.25|1.5`. The flyout template selects `?state=idle|streaming|standby|empty|error|credential&lang=zh|en&dark=0|1&latency=0|1&media=0|1&lamp=cover|template|expanded&zoom=1|1.25|1.5`.

```bash
chrome --headless=new --hide-scrollbars --window-size=1400,900 \
  --screenshot=.audit/mockups/<topic>/r1-<state>-<lang>-<theme>.png \
  "file:///<absolute-repo-path>/.audit/mockups/<topic>/mockup.html?view=target&lang=zh&dark=0"
```

Use an absolute `file://` URL. On Windows, `cygpath -m "$(pwd)"` gives the path Chrome accepts.

## Native preview: a new feature

Use this when a feature adds UI and logic to the flyout, settings window, a dialog, or the tray menu panel, and the decision will ship.

1. Add the state first. Put the new fields in the production state (`engine.Snapshot` or the `App` fields) before the view. The preview and the product read one shape.
2. Add catalog entries in `internal/app/ui_catalog*.go`: the default, the target, and the edge that shares the layout. `PreviewScenarios()` picks up every flyout, settings, dialog, and tray menu entry for the wasm selector. The catalog is the one mock data set. The wasm selector, the CPU matrix, and the platform render read it.
3. Write the view in the real file (`flyout.go`, `settings_window.go`, or the owning page). It is product code.
4. Route a new action through `act` with an `Action` constant. Put the real engine, store, or OS call in an `actX` helper and register the fake transition in `previewHandlers`. The seam fails closed. In preview, an action with no handler or an argument of the wrong type panics and never runs the real closure. Write the transition from `docs/protocol.md` or `docs/devices.md`, so the fake state is one the engine can reach.
5. Human: run `go tool mygo dev -web` and click the flow. Agent: render the catalog through the platform renderer and inspect the PNGs for clipping, overflow, wrapping, and state truth.
6. Freeze the catalog and the transition table. Implement the real engine adapter, keep the views, and replay the transitions in tests.

A new settings page follows the same steps. Add the `settingsPage` and its nav entry. The window renders the page. Wrap an action in `act` only when it reaches the engine or the OS. A nil-safe setter updates `a.cfg` directly.

## Native preview: change an existing feature or its UI

- **Visual change.** Edit the real view. Render the same catalog states before and after. The agent compares the PNGs. Leave the action registry and the seam alone.
- **Behavior change.** Change the real action and the matching `previewHandlers` transition together. Click it in wasm, render the affected states, and update the transition test.
- **New state.** Add one catalog entry. The wasm selector and both render drivers read it. One entry is the whole mock.
- **Regression.** Render every catalog state before the change as a baseline, and after it for comparison. The image diff is the visual regression. The transition table is the behavior regression.

## Native preview commands

```bash
# Human: a hot-reloading browser build.
(cd cmd/saundo/preview && GOFLAGS=-tags=preview go tool mygo dev -web)

# Agent: one PNG per still scenario from the same catalog, through the
# platform renderer. The driver skips motion, hover, and scrolled states.
SAUNDO_PREVIEW_RENDER="$(pwd)/.audit/preview/<topic>/render" \
  go test -tags preview ./internal/app -run TestPreviewRender -count=1

# Agent: the full CPU matrix, including motion, hover, and scrolled states,
# in both languages.
go run ./scripts/check-ui render -out .audit/preview/<topic>/matrix

# The gates.
go run ./scripts/check-ui tokens
go run ./scripts/check-ui words
go run ./scripts/check-ui catalog
go run ./scripts/check-ui wasm
go test -tags preview ./internal/app -count=1
```

The platform command writes one appearance per scenario. A `ThemeBoth` scenario writes its light and dark stems. The CPU matrix writes the same stems with `-en` and `-zh` suffixes.

## Native preview rules

- The platform render is the visual truth. The browser is the interaction surface. Present the running build and the render PNGs together.
- Refine the same view and catalog in place. Implement the version the user has seen.
- Final-pose renders need the enter animation settled. The platform driver sets the flyout, settings, dialog, and log entered flags. A new view with an enter pose needs its flag set there, or the PNG is the invisible start of the animation.
- The page sets the size. A browser window ignores size options, and a zero-size viewport produces a zero-height canvas. The preview page sizes the flyout to 420 wide and its state height, settings to 720x500, and the tray menu panel to 260 wide and its row height. Size the browser window to the surface, or use the page's `setPreviewSize` hook.
- The browser has no tray. The `tray.menu` scenario draws the preview panel from `trayModel` and `trayMenuItems`, the same model the native menu uses. The panel is a mock. Its clicks route through the typed actions. The shipped native menu is not touched by any of this.
- Keep the preview command under `cmd/saundo/preview`, so the app package stays a leaf.
- Keep action helpers in their own file, so a budgeted file such as `flyout.go` does not grow.
- The fixture and the preview handlers are throwaway. The views, the state shape, and the transition table are not.

## Measured geometry

These are the shipped values. The flyout template's height rule and the token gate pin them.

- Settings: 720x500 page area, minimum 640x420, a 190-wide sidebar with nine pages, a 32-DIP title bar, and a 14/20 page inset.
- Flyout: 420 wide. Its height follows `flyoutHeightForState`: base 439 for three device rows, 62 less because the media card replaces the old Now Playing row, 90 for the permanent ambient section, +150 while a media session exists, +44 for the latency row, +48 for the connection error bar, +48 for the credential-recovery notice, and +53 for the expanded ambient grid. Three device rows are 56 each. The empty state is 141. The device card and the playback card touch through the body's 8-DIP gap.
- Tray menu panel: 260 wide. Each row is 30 DIP, 40 with a subtitle. A separator is 13 DIP, a 1-DIP line with 6 DIP above and below. The panel adds 12 DIP of padding.

## Sketch rules

- Constrain the HTML to what mygo can express, so the approved render translates one-to-one. Card to a `ui.Column` with the palette border and radius. Row to `ui.ButtonBase` or `ui.Row`. Switch and slider to `toggle` and `slider`. Add row to a dashed `ButtonBase`. Empty note and group head to `ui.Column` or `ui.Row` with `ui.Text`. Hidden group to `CollapsibleBase`. Chips to the shared chip helper.
- Edit only the marked blocks. In the settings template they are `NAV`, `PAGE`, `STATES`, `COPY`, the three view functions, and the notes. In the flyout template they are `COPY`, `DEVICES`, `STATES`, the state functions, and the notes. Keep the tokens, shell, toolbar, and helpers.
- Show a visible switcher for state, language, and theme. Render every state in zh-CN and English, light and dark.
- Write the renders to `.audit/mockups/<topic>/` as `r<N>-<state>-<lang>-<theme>.png`. Self-contained, no network.
- Build three variants only when the user explicitly asks for variants, a model bakeoff, or collision thinking (碰撞). Run one per model from the pstack `arena runners` line, each with its own output path. Require each to render and inspect its own screenshot.
- A new state starts in the overlay. Do not add it to the production view or the catalog until the design is approved.

## Judge the render

Read the render and view its screenshots. Score:

1. One action, one meaning. A glyph appears only where clicking performs that action.
2. Truthful copy. The state says what is true and what to do next. Do not invent discovery status the engine does not expose.
3. Visual continuity. Same tokens, type scale, insets, and geometry as the surface it joins.
4. State coverage. Target, normal, and edge states, in zh-CN and English, light and dark.
5. Native feasibility. Every element maps to a mygo primitive and an existing icon.
6. No layout jump. The header, list slot, and add action keep their order when content changes.

Check state semantics in the render, not the report. Confirm hidden rows show off switches, disabled actions are not drawn, and English copy stays on one line.

## Present, pick, and iterate

For one mockup, present the link, its stance, and its tradeoffs. For three, present each variant by link with its stance, tradeoffs, and a recommendation. Name a base and any graft from another variant.

The user answers in one of four ways:

- **Approve.** Freeze that render as the approved design and go to the graduation path.
- **Refine.** Edit the same working copy in place, re-render, inspect, and present again. Keep the path stable so the link still works. Append every accepted change to the overlay.
- **Graft.** Fold the requested part of another variant into the chosen file by hand, re-render, and show the result before implementing. Do not edit the losing variant.
- **Reject or redirect.** Do not average the variants. Rewrite the overlay with the new constraint and run another round. A changed direction is a new round, not a silent edit of the old one.

Implement only the render the user has seen and approved after the last edit.

## Implement the pick

Create an isolated worktree from `HEAD` when the main checkout may have other work:

```bash
git worktree add --detach ../saundo-<topic> HEAD
```

Give one owner the page, locale catalogs, generated accessors, tests, docs, and verify map. Do not split that unit across writers.

Delete the control, glyph, or key the design replaces. Do not restyle it into a second variant. Regenerate the catalog after editing JSON:

```bash
go tool mygo generate cmd/saundo
```

Add a `ui.Tester` test that asserts the visible text for each new state. Update any older pin the design intentionally changes. Add the `Scenario` to the catalog so both renderers show both languages and themes. Update `docs/ui.md` and `.claude/skills/verify/features/<page>.md` when the page shape or journey changes.

## Verify and hand off

```bash
mingw32-make check
go run ./scripts/check-ui render -out .audit/ui/<topic>
go run ./scripts/check-ui diff -baseline docs/ui-baseline/manifest.json -against .audit/ui/<topic>/manifest.json
mingw32-make build
```

The canonical diff compares the 24 committed PNGs. Files outside the canonical set are named and not compared. The full matrix is for eyeballing. If the approved change touched a canonical state, run `go run ./scripts/check-ui rebaseline -baseline docs/ui-baseline/manifest.json -against .audit/ui/<topic>/manifest.json` only when the visual change is the one under review. Never edit the baseline manifest or its PNGs by hand.

Rebuild only when it will not interrupt an active stream. Leave the change uncommitted unless the user asks to commit. When they do, use the `AGENTS.md` fields and name the tests that pin the new state.

## Site fidelity

The landing page is a second product implementation, not a mock. Its automated gates are `check-ui tokens`, `check-ui words`, and `TestReplicaDataIsCurrent`. The remaining pass is visual:

- Serve the repository and open `/site/`: `python -m http.server 8000`, then `http://localhost:8000/site/`.
- Inspect 1400x900 and 320 px wide, in zh-CN and English, light and dark.
- Confirm the anatomy: header, receiver rows, playback card, latency segments and volume, ambient light, status line.
- Confirm no clipped or wrapped copy and no console errors.
- Record the screenshots under `.audit/release-<version>/site/`.

There is no stored reference PNG pair to compare. The page may change its story, such as how many receiver rows it shows, but not its anatomy.

The page and the sketch keep separate HTML copies. The token gate keeps their copied numbers in sync, and the wording gate keeps the page's flyout copy in sync. Extracting the site's flyout into a component shared with the sketch is deferred. The trigger is a drift the gates cannot express: the same structural rule changes in both copies for one design decision, twice, or a component edit that has to be made in both copies each round. Until then, the gate is cheaper than the component.

## Stop conditions

- `make check` is red. Run it before spending on mockups.
- The token gate is red. Fix the template or the app first.
- The app is streaming and a rebuild would interrupt it. Ask before stopping it.
- Manufacturing the state needs `%APPDATA%\saundo\*` changed. Never mutate the user's registry, credentials, or settings.
- A variant's render contradicts its report. Fix the variant or drop it.
- The platform render and wasm disagree. Fix the shared view or state.
- A preview handler adds a transition the engine cannot reach.
- A proposed state has no catalog row and no plan to add one.

## Report

Name the instruments that ran. Name whether the round was one mockup or three variants, the baseline and overlay paths, the states rendered, the render paths, the pick and graft, the checks that ran and printed what, and the checks not run. For the native preview, name what stayed fake and what remains unwired. State where the evidence lives (`.audit/mockups/<topic>/` or `.audit/preview/<topic>/`, never committed) and whether the implementation is committed.
