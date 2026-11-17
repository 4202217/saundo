# One data source, four instruments

This plan unifies saundo's UI mockup, preview, and verification data. The maintainer and the agent working on a UI change are the users. The rule is one fixture and one state catalog feeding four declared render paths, with gates that fail on drift. Phases 0 to 5 each land as one pull request with its own evidence.

Status. Plan only. No production code changed. The plan lives on `ui/unification`, cut from `main` at `a04ce1b`.

## How to read this

Each phase is a unit of work. A phase names the files it touches, the evidence that proves it, and the reason it comes in this order. Phase 1 is the one-way door and stops for review.

This plan follows `figure-it-out`, not the autopilot template. The UI loop is a human design process on a two-platform Go app. A ten-lane swarm and a perf ratio per pull request would add ceremony without adding evidence. The rigor is in the baseline, the gates, and the checkpoints.

The program enforces one rule. A state has one ID. That ID appears in the sketch overlay, the wasm preview, the CPU matrix, the platform render, the site data when marked, and the verify feature map.

Execution starts only on the operator's explicit go. This document is the plan, not a record of execution.

## Definition of done

Done is a predicate. Each line is a check, not an intention.

- Adding one `Scenario` to the catalog makes the state appear in the wasm preview, the CPU matrix, and the platform render. When the scenario carries `SiteDemo`, the state also appears in the landing page data. The same ID appears in the verify feature map.
- Changing one token in `internal/app/theme.go` fails `check-ui tokens` until both HTML templates and the site CSS agree.
- Changing one scenario name fails `check-ui catalog` until the baseline manifest and the verify map agree.
- A seeded one-pixel change to a canonical state fails `check-ui diff`. An unchanged tree passes.
- A release runs `check-ui all`, regenerates the matrix and the site data, and needs no manual comparison of three artifacts.

## Scope

In scope:

- The base fixture, the sample data, and the state catalog in `internal/app`.
- The CPU matrix, the platform render, the wasm preview, and the landing page data projection.
- The gate scripts, the evidence manifest, and the CI wiring.
- The `ui-mockup`, `verify`, `ui-review`, and `release` skills where they describe the workflow.
- The docs that state the workflow, mainly `docs/ui.md`, `docs/verification.md`, and `docs/releasing.md`.

Out of scope:

- New UI features or a visual redesign.
- Protocol, engine, capture, or Now Playing behavior.
- A browser pixel baseline.
- A general screenshot-diff framework with tolerance sliders.
- Replacing mygo or the native UI.
- Merging the `worktree-render-demo` branch.

## Current state

The workflow has four data sets and four render paths, and none of them share an ID.

| Area | What exists | Evidence | Limit |
|---|---|---|---|
| Fixture | `previewApp`, `receiverFixture`, `localReceiverFixture`, and `PreviewApp` in the native-preview branch | `internal/app/app_test.go:55`, `internal/app/receivers_page_test.go:27`, `internal/app/receivers_page_test.go:88`, `internal/app/preview_fixture.go` in `app/native-preview` | Four copies of the same sample devices and settings |
| Matrix | 118 named state stems, two locales, 18 motion frames, rendered by `save`, `saveScaled`, `saveScrolled`, and `frames` calls | `internal/app/preview_test.go:29`, `:147`, `:269`. The file is 799 lines | Env-gated, not in `make check`, no baseline comparison |
| Platform render | `TestPreviewRender` renders three flyout states through `render.Still` | `internal/app/preview_render_test.go` in `app/native-preview` | Asserts nothing about the image. The catalog is flyout-only |
| Wasm preview | Real views and widgets in the browser, a fake reducer, console hooks | `cmd/saundo/preview`, `internal/app/preview_*` in `app/native-preview` | The preview tag is not in `make check`. Unknown actions fall through to the real closure |
| Site data | Hardcoded `FLY_DEVS`, `FLY_MODES`, `MEDIA`, `fly`, `lamp`, `AMB_TPLS`, `lat` | `site/index.html:1745`, `:1755`, `:2220`, `:2337`, `:2341`, `:2358`, `:2359` | No generator on `main`. The render-demo branch's generator is not merged |
| Tokens | Two palette literals in Go, copied into both HTML templates and the site CSS | `internal/app/theme.go:33`, `:60`. `.claude/skills/ui-mockup/assets/*.html` | `check-replica` exists only on `worktree-render-demo`. No gate runs on `main` |
| Gates | `make check` runs format, catalogs, vet, mygo vet, arch, size, tests, verify map, platforms | `Makefile` `check` target | No preview compile, no matrix, no mockup check, no pixel check |
| Evidence | `.audit/` holds the matrix and mockups | `.gitignore` excludes `.audit/`. `docs/verification.md:306` cites a 0.6.3 directory that is absent | Cited evidence does not survive the checkout |

The two research branches hold complementary parts.

`app/native-preview` has the driver shape. It runs the real views in wasm, routes clicks through the `act` seam, and renders platform PNGs with `render.Still`. It has five commits, 32 files, and about 1,434 added lines. It also has two fixtures, a three-state catalog, an unguarded reducer fallthrough, and no gate.

`worktree-render-demo` has the data discipline. It generates `site/assets/demo-data.js` from the app fixture, pins it with `TestReplicaDataIsCurrent`, compares 15 tokens and 11 metrics with `scripts/check-replica`, and checks the flyout's visible wording against the page. It has no pixel comparison, its committed comparison pair is stale, and it sits on 218 commits of unmerged work.

## Target architecture

### One fixture

One fresh `*App` per scenario. The base fixture carries the union of the sample data that the three current fixtures and the site need:

- receivers across discovered, manual, hidden, offline, stereo, and local states
- the Now Playing sample track, cover, icon, and meter frame
- volume, latency, lamp preset, standby, capture names, and both locales
- the site-only fields the current literals carry, including the lamp preset ID and the template palettes

The fixture lives in one place. Named variants become catalog entries, not separate fixture functions. `receiverFixture` and `localReceiverFixture` stop being fixtures and become `Apply` functions.

The fixture should be untagged only if the shipped binary does not keep it. Phase 1 checks this with `go tool nm`. If the symbol survives, the fixture moves behind the `preview` tag and the untagged tests get a minimal shared constructor, or the team accepts the small binary cost. The check decides, not a preference.

### One catalog

One table of scenarios. The type sketch is the contract for Phase 1.

```go
type Surface uint8

const (
	SurfaceFlyout Surface = iota
	SurfaceSettings
	SurfaceDialog
	SurfaceTrayMenu
)

type Theme uint8

const (
	ThemeLight Theme = iota
	ThemeDark
	ThemeBoth
)

type Scenario struct {
	ID       string   // stable across instruments, e.g. "flyout.stream"
	Name     string   // output stem, e.g. "flyout-stream-light"
	Dark     string   // exact dark stem when it is not Name + "-dark"
	Surface  Surface
	Host     Surface  // owning view for a dialog
	Page     settingsPage
	Apply    func(*App)
	Size     func(*App) (w, h int)
	Theme    Theme
	Scale    float32  // 1, or 1.25 for the large-text probes
	Scroll   float32
	Frames   int      // 0 for a still, 3 for a motion probe
	Move     func(*ui.Tester) // the hover state that settles and moves the pointer
	SiteDemo bool
}
```

Phase 1 refined the sketch in three places. `Dark` names the exact dark stem because some light stems do not end in `-light`. `Host` selects the owning view for a dialog, since dialogs render inside either the flyout or the settings window. `Move` reproduces `flyout-hover`, the one state that moves the pointer and renders a second frame. The driver still reads the same fields, and the output stems are unchanged.

Rules for the table:

- `ID` is the join key. It never changes when a file name or a page title changes.
- `Apply` runs on a fresh `Fixture()`. It never depends on the previous scenario.
- `Name` keeps today's output stem during the migration, so Phase 0 and Phase 1 can compare hashes one to one.
- `Surface` selects `a.view` or `a.settingsView`. A dialog still renders inside its owning view. `Apply` sets the dialog flags.
- `Theme`, `Scale`, `Scroll`, and `Frames` are driver options, not separate state copies.

The catalog splits by surface when it approaches the 800-line cap. Suggested files are `ui_catalog_flyout.go`, `ui_catalog_settings.go`, and `ui_catalog_dialogs.go`. Test files are exempt from the cap, but the catalog is not a test file because the wasm preview imports it.

### Four render paths

The four paths stay separate because they answer four different questions. Unifying them into one renderer would make at least one answer false.

| Claim | Authority | Where it runs |
|---|---|---|
| Deterministic layout and copy for a named state | Software rasterizer through `ui.NewTester` | Local and CI |
| The pixels a user sees on Windows or macOS | `render.Still` on D3D11 or Metal | Local, or a GPU runner |
| Click, scroll, transition, and animation behavior | Real views in wasm in a browser | Local. A compile check runs in CI |
| Tray, native window chrome, discovery, audio | The shipped app under the verify skill | A real machine |

A fifth consumer is the landing page. It is not a mock and not a test double. It is a second product implementation, and it consumes a projection of the fixture. Its claim is that the page matches the app closely enough for its demo.

### Graduation path

A design decision moves through four states, and the ID moves with it.

1. **Sketch overlay.** The HTML sketch starts from the current approved baseline and adds a delta file. The delta holds the new states, copy, and layout. It is allowed to invent data the engine cannot reach yet.
2. **Catalog.** The approved delta becomes a `Scenario`. The invented data either enters the fixture or is deleted.
3. **Preview and tests.** The scenario renders in wasm and through both PNG drivers. A behavior test pins the transition. The render baseline pins the pixels when the scenario joins the canonical set.
4. **Verify.** The feature page in `.claude/skills/verify/features/` names the scenario ID and the live check.

A scenario that never ships is deleted at the overlay stage. A scenario that ships without a catalog entry is a gate failure.

### Gates and evidence

One command, `go run ./scripts/check-ui`, with these subcommands:

- `catalog` validates unique IDs, known surfaces, positive sizes, a registered view, a handled action for every typed action, and an existing fixture for every `SiteDemo` scenario.
- `render` writes the CPU matrix and a `manifest.json`.
- `truth` writes the canonical subset through `render.Still` and records the renderer from `render.Progress`.
- `diff` compares two manifests and their PNG hashes. A missing baseline is inconclusive, never a pass.
- `rebaseline` updates the hashes of the existing baseline file set and refuses a missing file. It never adds or removes a state.
- `tokens` compares the Go palette and geometry literals with both HTML templates and the site CSS.
- `words` renders the flyout and settings, collects visible text including `aria-label` and `alt`, and requires the site and the sketch to carry it.
- `wasm` builds `GOOS=js GOARCH=wasm` with the `preview` tag.
- `all` runs the checks that belong in the milestone gate.

The manifest records `commit`, `dirty`, `renderer`, `scenario ID`, `name`, `surface`, `locale`, `theme`, `scale`, `width`, `height`, and `sha256`. The baseline is a committed manifest plus the canonical PNGs. The full matrix stays in `.audit/` and is not committed.

## Repository policy

The `preview` build tag marks harness code. It covers the catalog, the wasm command, the fake reducer, and any preview-only fixture. The tag never reaches a production build. `scripts/build.sh` and the `build` target reject a `GOFLAGS` value that contains `preview`. `check-ui wasm` passes the tag explicitly.

The production app must not import the preview command. `internal/app` stays a leaf. The preview command lives under `cmd/saundo/preview` and imports the app package, which the architecture gate already allows.

The renderer is pinned per use. The matrix and the CI baseline use the software rasterizer. The platform truth render uses `render.Auto` and records the name that `render.Progress` reports. A manifest without a renderer is a setup error.

The baseline lives in `docs/ui-baseline/`. It holds a manifest and a canonical subset of about twelve PNGs, one per surface and state family. The first candidate set is flyout streaming light and dark, flyout connecting, flyout error, flyout empty, settings general, settings connection dark, settings receivers expanded, settings audio custom, settings lamp cover, settings now playing live, and the receiver add dialog. Phase 4 may adjust the list to balance coverage against review load. The membership is fixed. `check-ui rebaseline` updates hashes for an intentional change, and a state joins or leaves only through an explicit baseline change reviewed with the code.

## Work sequence

### Phase 0. Capture the baseline

Goal. Prove the observation method before changing the thing observed.

Changes.

- Add `scripts/check-ui` with `render` and `diff` in their simplest form. `render` wraps the existing `TestRenderPreviews` without changing the matrix. `diff` compares two manifests and hashes.
- Write a manifest for the current matrix on this host.
- Run the render twice and compare.
- Measure the full matrix runtime.

Files. `scripts/check-ui/main.go`, `scripts/check-ui/main_test.go`, `Makefile` (a non-gating `ui-baseline` target), this plan.

Acceptance. Two runs produce identical hashes for the stable subset, and every unstable file is named with its pixel difference. The baseline manifest lists the stable files. The full manifest lists 118 state stems across two locales and 18 motion frames. The runtime is recorded. `make check` stays green.

Result. The matrix wrote 254 files in 28 to 31 seconds. 243 files are stable. The 11 unstable files differ by 7 to 318 pixels, all from animation phase. `ui.Tester` starts its clock at `time.Now`, so `Advance(400ms)` lands at a different phase on each run. The unstable names are four `flyout-latency-applying` files, two `settings-update-checking` files, `flyout-stream-large-text-en`, `settings-connection-large-text-en`, `settings-audio-light-en`, `settings-connection-light-en`, and `settings-receivers-light-en`. The baseline is `.audit/ui/baseline.json`. The full matrix still renders for human review.

Evidence. Two full manifests and the stable manifest. The unstable list and the pixel counts. The runtime log.

Risk. The tester's text system may use host fonts. If the two runs differ beyond the known animation phase, the diff moves to a smaller canonical subset and records the host in the manifest. The matrix still renders for human review, but it is not the baseline.

### Phase 1. One fixture and one catalog

Goal. Replace the imperative matrix with the catalog without changing a pixel.

Changes.

- Move the base fixture to one shared function.
- Define `Scenario` and the catalog.
- Migrate the 113 `save` calls, surface by surface, into catalog entries.
- Make the matrix driver read the catalog. A fresh fixture per scenario replaces the shared mutable locals.
- Delete the fixture duplicates that the catalog now covers.
- Keep the current output stems so hashes compare one to one.
- Add `check-ui catalog`.

Files. `internal/app/ui_fixture.go`, `internal/app/ui_catalog.go`, `internal/app/ui_catalog_flyout.go`, `internal/app/ui_catalog_settings.go`, `internal/app/ui_catalog_dialogs.go`, `internal/app/preview_test.go`, `internal/app/app_test.go`, `internal/app/receivers_page_test.go`, `scripts/check-ui/main.go`, `scripts/check-size/baseline.txt` only if a grandfathered file changes size.

Acceptance. Every Phase 0 hash matches for the same state and locale, except the two `flyout-hover` images whose Move was corrected after review. `check-ui catalog` passes. `make check` is green. No state disappears. The manifest maps every old stem to its scenario ID.

Result. The matrix wrote 254 files. 243 stable files match the Phase 0 baseline after `check-ui rebaseline` updated the two hover hashes. `mingw32-make check` passed, including windows/amd64 and darwin/arm64. The production binary keeps no catalog, fixture, or sample symbol. The five documented matrix commands now use `check-ui render`.

Evidence. A manifest diff with only the two intended hover changes. The catalog validation output. The full gate output.

Risk. A scenario may depend on mutation left by an earlier scenario. A fresh fixture per scenario exposes that. Fix the fixture, not the order.

Stop. Review this phase before Phase 2. The catalog shape and the fixture location are the one-way doors.

### Phase 2. Merge the native preview and fix the seams

Goal. Make the wasm and platform drivers read the catalog, and make preview actions fail closed.

Changes.

- Merge `app/native-preview`, or port its five commits. Resolve the skill text against the catalog model.
- Replace `PreviewStates` and `PreviewApp` with the catalog and the fixture.
- Replace string action names with typed `Action` constants and a handler registry. In preview mode, an unhandled action panics. A test enumerates every action and asserts a handler exists.
- Remove the `i18n.SetLocaleFunc` call from `init`. The driver sets the locale per state, and the test entry point sets the test locale.
- Add settings and receiver scenarios to the catalog. Keep the tray menu panel outside the catalog until its view takes locale and theme from the app, not from package globals. Add the size hook the browser page needs, or document the viewport sizing.
- Make `TestPreviewRender` a catalog driver for the flyout, then for the other surfaces.
- Add `go vet -tags preview ./...`, `go test -tags preview ./internal/app`, and `check-ui wasm` to `make check` and CI.
- Add the `GOFLAGS` guard to the build paths.

Files. `cmd/saundo/preview/*`, `internal/app/preview_*`, `internal/app/act.go`, `internal/app/*_actions.go`, `internal/app/ui_catalog*.go`, `Makefile`, `.github/workflows/ci.yml`, `.claude/skills/ui-mockup/SKILL.md`.

Acceptance. The wasm build succeeds. The preview tests pass. A seeded unknown action panics in preview and fails the test. The flyout scenarios render through both PNG drivers. The settings window renders the catalog's page, and the tray menu panel still opens.

Result. The catalog is the only preview state source. All 18 action call sites use typed actions, and an unhandled action panics instead of running the real closure. The wasm builds at 26,417,305 bytes. The platform driver wrote 108 non-blank PNGs from 85 preview scenarios. The CPU matrix still matches 243 stable files. The preview gates run in `make check`, both CI jobs, and both release check jobs, and the release build rejects a `GOFLAGS` preview tag. The review found 23 defects, and all are fixed with regression tests. The tray menu panel still opens and its clicks use the typed actions, but it has no catalog scenario: its view reads package globals and `mygo.App.Locale()`/`Theme.IsDark()`, so a catalog entry would render in the host locale and theme. That entry moves to Phase 5.

Evidence. The wasm build log, the 108 platform PNGs with their non-blank assertion, the 243-file matrix diff, and the full gate output.

Risk. The branch's fixture and reducer assumptions conflict with the catalog. Port the behavior, not the file layout. The wasm sizing hook may need a custom page.

### Phase 3. Site projection and consistency gates

Goal. Generate the landing page data from the fixture and gate the replica.

Changes.

- Port `TestExportReplicaData`, `TestReplicaDataIsCurrent`, `TestReplicaSharesTheFlyoutsWording`, and the token and metric checks from `worktree-render-demo`. Record the source commit.
- Add `SiteDemo` to the catalog and generate `site/assets/demo-data.js` from the fixture and the marked scenarios.
- Replace the site's hardcoded `FLY_DEVS`, `FLY_MODES`, `MEDIA`, `lamp`, `lat`, and `AMB_TPLS` with the generated data. Keep the old literals behind a fallback until the generated page passes.
- Fix the wording check to read `aria-label` and `alt`, and make the gap list fail when it goes stale.
- Rename the pair evidence or regenerate it. The stale pair is not evidence.
- Add `check-ui tokens` and `check-ui words` to `make check` and CI.

Files. `internal/app/replica_data_test.go`, `internal/app/replica_texts_test.go`, `scripts/check-ui/main.go`, `site/assets/demo-data.js`, `site/index.html`, `docs/reference/README.md` if the reference procedure is ported.

Acceptance. A fixture change without regeneration fails the staleness test with the regeneration command. A seeded token drift fails. A seeded wording drift fails. The landing page renders the same demo in a browser.

Result. `site/assets/demo-data.js` is generated from `Fixture("zh-CN")` and the `flyout.stream` scenario. It carries the receivers, per-player sessions with both languages, volume, latency modes, the lamp preset and per-track cover lamps, the six ambient templates, and the 17 `SiteDemo` IDs. The page loads it before its own script, keeps its demo logic, and falls back to a static demo if the file is missing. `check-ui tokens` passes 15 tokens in two appearances, 11 metrics, and 420 px. `check-ui words` passes 97 messages, 92 on the page, 5 declared gaps. The page keeps two receiver rows, and the dataset carries three. The review found 20 defects, all fixed, including the mini glyph, the English track text, the per-player transport, the 320 px fit, the cover lamp, and the README claims. The matrix still matches 243 stable files. The full gate passes. Screenshots are in `.audit/ui/phase3-fixes/screenshots/`.

Evidence. The generated dataset, the gate outputs, the browser screenshots at 1400x900 and 320 px in both languages, and the full gate.

Risk. The page keeps a static demo fallback for a missing dataset; the dataset is still the single source. The site-fidelity skill and the four-instrument skill rewrite are Phase 5.

### Phase 4. Baselines and CI

Goal. Make an unintended visual change fail the build.

Changes.

- Add the canonical subset and its committed manifest and PNGs.
- Add `check-ui diff` to the Windows CI job. macOS runs the builds and tests, not the pixel diff, unless a macOS baseline is added later.
- Record the renderer through `render.Progress`.
- Add the baseline update command and the review rule. `check-ui rebaseline` updates the existing file set and never adds or removes a state. Update the baseline only when the visual change is the one under review.
- Cite the manifest in `docs/verification.md`.

Files. `scripts/check-ui/main.go`, `docs/ui-baseline/manifest.json`, `docs/ui-baseline/*.png`, `.github/workflows/ci.yml`, `docs/verification.md`.

Acceptance. A seeded one-pixel change fails. An unchanged tree passes. A missing baseline is inconclusive. A renderer mismatch is reported, not hidden.

Result. `docs/ui-baseline/` holds the 12 canonical state stems and 24 PNGs, one per locale, plus the manifest. `check-ui subset` cuts the subset from the full stable manifest and refuses a missing stem or a mismatched PNG. `check-ui diff` verifies each manifest against the PNGs beside it, refuses a missing manifest or a missing renderer, and names a changed file. The Windows CI and release check jobs render the canonical states and run the diff; the macOS jobs skip the pixel diff because the baseline is per host. `TestPreviewRender` records `Progress.Renderer` per image and at the top level; the local run reported `Direct3D 11 (NVIDIA GeForce RTX 5070)`. The canonical render matched 24 files. A one-pixel change to a baseline PNG, a missing manifest, and a renderer mismatch each failed closed; the unchanged tree passed.

Evidence. The red run and the green run, with the manifest and the hash.

Risk. Cross-host font differences. The baseline is per host. Windows is the reference. Add macOS only when a macOS contributor needs it.

### Phase 5. Sketch baseline and graduation path

Goal. Make the HTML sketch a projection of the same source, with an overlay for the design delta.

Changes.

- Gate both HTML templates' tokens and metrics with `check-ui tokens`.
- Add a sketch baseline step. The preferred form generates the baseline from the shared data and the current approved layout. The fallback is to copy the refreshed templates and gate the copies.
- Extract the flyout component from the site page only if the gate shows repeated drift. The fallback is gate-only. This is a scope decision, not an assumption.
- Define the overlay file under `.audit/mockups/<topic>/`. It adds the speculative states and copy and references the baseline data.
- Update the `ui-mockup` skill to the four-instrument model and the graduation path.
- Update the verify feature map and the release skill. The release regenerates the baseline and the site data instead of comparing three artifacts.

Files. `.claude/skills/ui-mockup/SKILL.md`, `.claude/skills/ui-mockup/assets/*.html`, `site/index.html` if extracting, `.claude/skills/verify/features/*.md`, `.claude/skills/release/SKILL.md`, `docs/releasing.md`, `docs/ui.md`.

Acceptance. A new state starts in an overlay, graduates to the catalog, and appears in all four paths. A release dry run regenerates the matrix and the site data and needs no manual three-way comparison.

Evidence. The overlay-to-catalog diff, the release dry-run log, and the updated skill text.

Risk. The component extraction breaks the site's demo. The fallback is gate-only, and the extraction becomes a separate decision.

## Verification

Each phase is verified on the artifact, not on the report.

- Unit. The phase's tests and `check-ui` subcommands. A gate change ships with a seeded failure that must go red.
- Live. Phase 0 inspects the manifest. Phase 1 diffs hashes. Phase 2 clicks the wasm surfaces and inspects the platform PNGs. Phase 3 loads the landing page in a browser. Phase 4 seeds a pixel change and runs CI. Phase 5 runs the release dry run.
- Perf. Phase 0 measured 28 to 31 seconds for the full matrix. The budget is 37 seconds. The canonical subset budget is 60 seconds.
- Review. Phases 1 and 5 stop for the operator. Phases 2, 3, and 4 report and continue on the operator's go.

## Decisions for the operator

1. Does the landing page keep a live interactive flyout? If yes, Phase 3 gates the replica. If it is decoration, replace it with app-rendered media and drop the site half of Phase 3. The engineering path is the same either way until that point.
2. Does the baseline live in git as a manifest plus twelve PNGs, or as a manifest only with the PNGs kept as CI artifacts? The repository already commits `docs/screenshots/`, so committing the baseline is consistent. The alternative keeps binary files out of history.
3. Does Phase 5 extract shared HTML components, or gate the current templates and defer extraction? The gate-only path is cheaper and proven by `check-replica`. The extraction path gives the sketch and the site one component source.

## Risks

| Risk | Phase | Mitigation |
|---|---|---|
| The catalog migration changes pixels | 1 | Baseline first, fresh fixture per scenario, hash parity, stop for review |
| The preview tag reaches a production build | 2 | `GOFLAGS` guard, explicit tags, `go tool nm` check, CI vet |
| An unhandled preview action runs a real OS call | 2 | Typed actions, fail-closed panic, registry test |
| The wasm viewport stretches the settings window | 2 | Size hook or custom dev page. Click through before acceptance. |
| The site loader breaks the demo | 3 | Fallback to the hardcoded data until the gate passes |
| The fixture ships in the production binary | 1 | `go tool nm` check. Move it behind the tag if it survives. |
| Cross-host font differences break the baseline | 4 | CPU renderer, per-host baseline, Windows as the reference |
| The branch merge creates conflicts | 2 | Port the five native-preview commits. Port render-demo files. Never merge the branch. |
| The 800-line cap forces an artificial split | 1 | Split the catalog by surface. Test files are exempt. |
| The operator review load grows | 4 | Canonical subset of about twelve states. The full matrix stays for eyeballing. |

## Rollback

Each phase is one pull request and reverts on its own. Phase 1 is the one-way door. The Phase 0 manifest and the kept file stems make a revert to the imperative matrix mechanical.

The native-preview merge reverts independently of the catalog. The site generator keeps the hardcoded data until its gate is green, so a failed Phase 3 reverts to the current page.

## Decision trail

The plan lives at `docs/plans/ui-unification.md`. During execution, append one dated row per phase to `docs/plans/ui-unification-log.md` with the phase, the verdict, the evidence path, and any deviation. Commit the log with the phase. The commit body follows the AGENTS.md fields, `Checks`, `Pinned-by`, `Finding`, and `Docs`.

## Appendix A. Evidence map

- Matrix entry point and helpers. `internal/app/preview_test.go:29`, `:147`, `:155`, `:176`, `:198`, `:269`. The file is 799 lines.
- Base fixture. `internal/app/app_test.go:36` for `TestMain`, `:55` for `previewApp`.
- Receiver fixtures. `internal/app/receivers_page_test.go:27` for `receiverFixture`, `:88` for `localReceiverFixture`.
- Palette. `internal/app/theme.go:33` and `:60`.
- Flyout geometry. `internal/app/flyout.go:25` for `flyoutWidth`, `:26` for `flyoutHeight`, `:54` for `flyoutHeightFor`, `:74` for `flyoutHeightForState`.
- Site data. `site/index.html:1745` `AMB_TPLS`, `:1755` `lamp`, `:2220` `lat`, `:2337` `FLY_DEVS`, `:2341` `FLY_MODES`, `:2358` `MEDIA`, `:2359` `fly`.
- Mockup skill and templates. `.claude/skills/ui-mockup/SKILL.md` is 147 lines. `assets/flyout-template.html` is 824 lines. `assets/mockup-template.html` is 713 lines.
- Gates. `Makefile` `check` target. `.github/workflows/ci.yml` runs `go test -short`, `mygo vet`, `verify-map`, and the platform builds, but no preview, matrix, mockup, or pixel check.
- Evidence gap. `docs/verification.md:306` cites `.audit/release-0.6.3/previews`, which is absent. `.audit/` is gitignored.
- Native preview branch. `app/native-preview` has five commits from `main`, 32 files, about 1,434 added lines. Its catalog is three flyout states. Its `act` seam has 18 call sites. Its unhandled action falls through to the real closure.
- Render demo branch. `worktree-render-demo` has 218 commits from merge base `307b214`. Its reusable parts are `internal/app/replica_data_test.go`, `replica_texts_test.go`, `replica_journey_test.go`, `scripts/check-replica/main.go`, and the site's `demo-data.js` loader. Its committed comparison pair is stale and is not evidence.

## Appendix B. Alternatives rejected

**One renderer for everything.** Rejected. The browser cannot produce the platform pixels, the platform renderer cannot produce the browser interaction, and the live app is the only authority for the tray, system windows, and audio.

**One tool with four modes.** Rejected as a replacement for four instruments. A single command entry point is fine. A single artifact with a single claim is not. The sketch must work before Go exists, and the verify skill must run the shipped app.

**Merge `worktree-render-demo` wholesale.** Rejected. The branch sits on 218 unmerged commits and carries a second flyout implementation. Port its generator and gates, and leave the branch as research evidence.

**Keep three systems and share only the fixture.** Rejected. The gates and the evidence stay divergent, and the release still needs a manual three-way comparison.

**Generate all UI from HTML.** Rejected. The product is mygo widgets in Go. Code generation would add a translation layer that the current workflow avoids.

**Use the browser as pixel truth.** Rejected. The site's fonts and WebGL 2 differ from the platform renderer by design.

## Appendix C. Open spikes

1. Do `ui.NewTester` and `render.Still` with `render.CPU` produce identical pixels for the same view and state? Phase 0. Pass when the hash is equal for one state. If not, the CPU baseline uses one driver and the other stays for breadth.
2. Does the untagged fixture survive in the shipped binary? Phase 1. Pass when `go tool nm` shows no fixture symbol, or the team accepts the cost.
3. Can the site's hardcoded data be replaced without breaking the demo? Phase 3. Pass when the page renders the same states in a browser.
4. Does the wasm page need a size hook for the settings window and the tray menu? Phase 2. Pass when both surfaces open at their intended size.
5. Does `mygo dev -web` rebuild reliably on `internal/app` changes? Phase 2. Pass when an edit to `flyout.go` reloads the page.
6. Can a test enumerate every typed action and prove the reducer handles it? Phase 2. Pass when a seeded missing handler fails.
7. Is the software render identical on Windows and macOS? Phase 4. Pass when the manifest matches, or the baseline stays per host.

## Appendix D. Source references

- `docs/architecture.md`, `docs/implementation.md`, `docs/ui.md`, `docs/ui-copy.md`, `docs/verification.md`, `docs/releasing.md`.
- `.claude/skills/ui-mockup/SKILL.md`, `.claude/skills/verify/SKILL.md`, `.claude/skills/ui-review/SKILL.md`, `.claude/skills/release/SKILL.md`.
- `AGENTS.md`, especially the architecture rules, the code standards, and the commit fields.
- The two research branches as evidence, not as merge targets.
