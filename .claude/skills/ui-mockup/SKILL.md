---
name: ui-mockup
description: This skill should be used when the user asks to "mock up a UI change", "show me design variants", "decide this with mockups", or when a saundo page, dialog, empty state, or interaction has more than one defensible shape. It builds one rendered mockup in the current session by default, fans out to competing variants only when the user asks to collide ideas or compare designs, then implements the chosen design through the native UI gate.
---

# Decide a saundo UI change with mockups

Use rendered mockups to settle the shape before writing production code. The mockup is throwaway evidence; the commit is the durable record.

## HTML first, Go after

The mockup is a throwaway HTML sketch. The product is Go with mygo widgets. The HTML exists to settle layout, copy, and states quickly. Never ship it, embed it, or turn it into a WebView. The toolbar, window shadow, backdrop blur, and zoom around the 720x500 window are mockup scaffolding. The page inside the window is the part that becomes Go.

Constrain the HTML to what mygo can express so the approved render translates one-to-one. Card to a `ui.Column` with the palette border and radius. Row to `ui.ButtonBase` or `ui.Row`. Switch and slider to `toggle` and `slider`. Add row to a dashed `ButtonBase`. Empty note and group head to `ui.Column` or `ui.Row` with `ui.Text`. Hidden group to `CollapsibleBase`. Chips to the shared chip helper.

## When to use

- A new page, dialog, empty state, or interaction with more than one defensible shape.
- A visual bug whose fix could be a restyle or a removal.
- The user asks for mockups, variants, or a design comparison.

Skip it for a one-line copy change, a spacing fix with an obvious target, or a behavior change with no visual decision.

The release refreshes both templates even when no design question is open: the release skill requires them to describe the shipped interface before the release commit.

## Stop conditions

- The gate is red. Run `mingw32-make check` before spending on mockups.
- The app is streaming and a rebuild would interrupt it. Ask before stopping it.
- Manufacturing the state needs `%APPDATA%\saundo\*` changed. Never mutate the user's registry, credentials, or settings.
- A variant's render contradicts its report. Fix the variant or drop it.

## 1. Frame the decision

State the state or interaction to decide and the smallest change that could satisfy it. Quote the user's complaint. Read the current page (`internal/app/<page>.go`), the design contract (`docs/ui.md`), and the locale files. Capture the current settings window when the app is running. Use `.claude/skills/verify/scripts/capture-window.ps1` with `-Class MyGoWindow -ProcId <pid>`.

If the evidence rules out the change the user asked for, show it and stop.

## 2. Write the brief

Write one brief. Use it for the current session's mockup, and reuse it verbatim for every variant when fanning out. Carry:

- The goal and the quoted complaint.
- The surface and size. Settings is 720x500, minimum 640x420, with a 190-wide sidebar of nine pages. The flyout is 420 wide and its height follows `flyoutHeightFor`: 439 for three device rows, 62 less because the media card replaces the old Now Playing row, 90 for the permanent ambient section, +150 while a media session exists, +44 for the latency row, +48 for the connection error bar, and +53 for the expanded ambient grid (`flyoutHeightForState`).
- The token and geometry source: `internal/app/theme.go`, and `.audit/mockup-final.html` when it is the visual baseline.
- The locale source: `cmd/saundo/locales/en.json` and `cmd/saundo/locales/zh-CN.json`. New copy needs a key and both languages.
- Native constraints: mygo primitives only (`Column`, `Row`, `Box`, `Text`, `ButtonBase`, `Icon`, `toggle`, `slider`, `CollapsibleBase`, `SegmentedBase`), plus borders, radius, padding, opacity, and the palette. No images, web fonts, or gradients outside the ambient preview's color glow. Inline SVG only for icons in `internal/app/icons.go`.
- The states to render: the target, the normal state, and the edge that shares the layout. Render every state in zh-CN and English, light and dark.
- A visible switcher in the mockup for state, language, and theme.
- The output folder: `.audit/mockups/<topic>/`. The single mockup is `mockup.html`; variants are `mockup-<model>.html`. Keep the Chrome renders beside them as `r<N>-<state>-<lang>-<theme>.png`. Self-contained, no network.

When fanning out, only the model varies.

## 3. Build the mockup

Default to one variant in the current session. Choose the surface template:

```bash
mkdir -p .audit/mockups/<topic>
# Settings window:
cp .claude/skills/ui-mockup/assets/mockup-template.html .audit/mockups/<topic>/mockup.html
# Flyout:
cp .claude/skills/ui-mockup/assets/flyout-template.html .audit/mockups/<topic>/flyout.html
```

Edit only the marked blocks. In the settings template they are `NAV`, `PAGE`, `STATES`, `COPY`, the three view functions, and the notes. In the flyout template they are `COPY`, `DEVICES`, `STATES`, and the notes. Keep the tokens, shell, toolbar, and helpers. Render with headless Chrome, inspect the screenshot, and fix clipping, overflow, and wrapped copy. Use the template's URL parameters to screenshot a specific state, and its media and ambient toggles for the two optional height contributions.

Build three variants only when the user explicitly asks for variants, a model bakeoff, or collision thinking (碰撞). For three, run one per model from the pstack `arena runners` line, default opus, sonnet, and fable, each with its own output path. Use `pstack:poteto-agent` when available and run them in parallel. Require each to render and inspect its own screenshot.

For one mockup, report its stance, the states it renders, the locale keys and both translations, the icon mapping, anything mygo cannot express, the screenshot path, and the rejected alternatives. For three, require the same from each variant.

## 4. Judge the render

Read the mockup and view its screenshots. For three, read every variant. Score:

1. One action, one meaning. A glyph appears only where clicking performs that action.
2. Truthful copy. The state says what is true and what to do next. Do not invent discovery status the engine does not expose.
3. Visual continuity. Same tokens, type scale, insets, and geometry as the page it joins.
4. State coverage. Target, normal, and edge states; zh-CN and English; light and dark.
5. Native feasibility. Every element maps to a mygo primitive and an existing icon.
6. No layout jump. The header, list slot, and add action keep their order when content changes.

Check state semantics in the render, not the report. Confirm hidden rows show off switches, disabled actions are not drawn, and English copy stays on one line.

## 5. Present, pick, and iterate

For one mockup, present the link, its stance, and its tradeoffs. For three, present each variant by link with its stance, tradeoffs, and a recommendation. Name a base and any graft from another variant. Do not merge variants into a direction the user cannot see.

The user answers in one of four ways:

- **Approve.** Freeze that render as the approved design and go to step 6.
- **Refine.** Edit the same mockup file in place, re-render, inspect, and present again. Keep the path stable so the link still works. Update the brief with every accepted change.
- **Graft.** Fold the requested part of another variant into the chosen file by hand, re-render, and show the result before implementing. Do not edit the losing variant.
- **Reject or redirect.** Do not average the variants. Rewrite the brief with the new constraint and run another round. A changed direction is a new round, not a silent edit of the old one.

Write each round's screenshots as `.audit/mockups/<topic>/r<N>-<state>-<lang>-<theme>.png`. Implement only the render the user has seen and approved after the last edit. Do not implement a mockup the user has not seen.

After implementation, a further design change follows the same rule. Edit the working tree, re-render the native preview, inspect it, and re-run the gate. If the change already landed in a commit, make a new commit; do not amend a landed commit unless the user asks.

## 6. Implement the pick

Create an isolated worktree from HEAD when the main checkout may have other work:

```bash
git worktree add --detach ../airgoplay-<topic> HEAD
```

Give one owner the page, locale catalogs, generated accessors, tests, docs, and verify map. Do not split that unit across writers.

Delete the control, glyph, or key the design replaces. Do not restyle it into a second variant. Regenerate the catalog after editing JSON:

```bash
go tool mygo generate cmd/saundo
```

Add a `ui.Tester` test that asserts the visible text for each new state. Update any older pin the design intentionally changes. Add page-level states to `TestRenderPreviews` in `internal/app/preview_test.go` so the native renderer shows both locales and themes. Update `docs/ui.md` and `.claude/skills/verify/features/<page>.md` when the page shape or journey changes.

## 7. Verify and hand off

From the main checkout:

```bash
mingw32-make check
SAUNDO_PREVIEW_DIR="$(pwd)/.audit/previews/<topic>" go test ./internal/app -run TestRenderPreviews -count=1
mingw32-make build
```

Inspect the native PNGs for the chosen state, the normal state, both locales, and both themes. Rebuild only when it will not interrupt an active stream. Leave the change uncommitted unless the user asks to commit. When they do, use the `AGENTS.md` fields and name the tests that pin the new state.

## Gotchas

- Keep one folder per decision under `.audit/mockups/<topic>/`. Never drop mockups at the repository root or in the `.audit` root.
- Do not rebuild the window chrome. Copy `assets/mockup-template.html` or `assets/flyout-template.html` and edit the marked blocks; a rebuilt shell drifts from the real window.
- The flyout height is not fixed. Keep the template's `flyoutHeight` rule so the device card and the playback card keep the same gap; do not stretch one without the other.
- A dashed container beside the dashed add row reads as a second disabled row. Prefer plain prose for an informational empty state.
- A zero count pill names a group with nothing in it. Render the group head only with rows, or suppress the pill.
- The add row is the only add affordance. A decorative plus will be clicked.
- The English copy is longer. Measure it at the narrow width and keep it on one line.
- Check `internal/app/icons.go` before using a glyph. Delete an unused icon only after a full `grep`.
- `TestRenderPreviews` resolves `SAUNDO_PREVIEW_DIR` against the test's working directory. Pass an absolute path.
- A worktree from the remote default branch may lack the current branch's commits. Create it from `HEAD`.
- Copying a worktree file over the main checkout can erase another session's uncommitted edit. Check `git status` first and copy only untouched files.

## Report

Name whether the run was one mockup or three variants, the surface template, the render evidence, the pick and graft, the checks that ran and printed what, and the checks not run. State where the mockups live (`.audit/mockups/<topic>/`, never committed) and whether the implementation is committed.
