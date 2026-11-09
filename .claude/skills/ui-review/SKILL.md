---
name: ui-review
description: Review saundo's UI copy and layout for consistency before a release, or when asked to audit UI copy, card headers, notes, or visual style. It inventories the catalogs, renders the preview matrix, lists findings by severity, fixes them in phases, and gates the result on the copy lints and the preview matrix.
---

# Review the UI

Read `docs/ui-copy.md` and `docs/ui.md` before you start. They are the rules.
This skill is the audit, the fix loop, and the release gate.

## When to use

- A release is being prepared.
- The user asks for a copy or visual consistency review.
- A card, header, note, dialog, or control changes shape.
- A device finding adds or changes UI copy.

Skip the full audit for a one-line copy change with an obvious key. The catalog
gates still apply.

## Stop conditions

- `mingw32-make check` is red. Fix or report before reviewing.
- The app is streaming and a rebuild would interrupt it. Ask first.
- A change would edit `%APPDATA%\saundo\*`. Never do that.
- The evidence contradicts the requested change. Show it and stop.

## 1. Inventory

- The copy lives in `cmd/saundo/locales/en.json` and `zh-CN.json`. The
  generated `catalog_gen.go` is never edited by hand; after a catalog change
  run `go tool mygo generate cmd/saundo`.
- Render the matrix in both languages:
  `SAUNDO_PREVIEW_DIR=/tmp/saundo-ui-review go test ./internal/app -run TestRenderPreviews -count=1`
  Add `SAUNDO_PREVIEW_ONLY=settings-audio,settings-lamp` to render only the
  pages a change touched.
- Read the owning files: `ui_kit.go` for shared widgets, `flyout.go`,
  `settings_window.go`, `receivers_page.go`, `lamp.go`, the dialogs, and the
  tests that pin the strings.
- The live app: `.claude/skills/verify/SKILL.md`. The capture is
  DPI-virtualized and can crop the right edge; the headless matrix is the full
  visual record.

## 2. Write the findings

Write the list before changing code. One row per issue: file:line, the current
copy or metric, the defect, and the proposed fix. Order by severity.

- P0: a fact is wrong, a control's behavior contradicts its copy, a string
  truncates, or a test passes for the wrong reason.
- P1: one concept has two names, or one state has two phrasings.
- P2: tone, redundancy, punctuation, units, dead keys, or a visual token
  outside the ladder.

Check the copy for colloquialisms, duplicated explanations, protocol or
implementation terms, unofficial product names, and English/Chinese drift.
Check the layout for row heights, action sizes, label recipes, chips, card
headers, card radii, dialog heads, type sizes, units, and status treatments.

## 3. Fix in phases

1. Subtract. Delete dead keys and duplicate variants first. Run
   `TestCatalogHasNoDeadKeys`.
2. Gate. Add or extend the catalog tests with the fix so the tree stays green:
   `TestCatalogHasNoDeadKeys`, `TestCatalogBannedTerms`,
   `TestCatalogUniqueValues`, `TestLatencyText`. Prove each gate catches a real
   past string.
3. Copy. Rewrite both catalogs together. Update every pinned test and every
   doc that quotes an old string. Regenerate the catalog.
4. Shape. Move repeated controls into `ui_kit.go`; keep the action ladder, row
   metrics, card header, type scale, and the note placement in the docs. Delete
   the older inline variants.
5. Docs. Update `docs/ui-copy.md`, `docs/ui.md`, and the dated evidence in
   `docs/verification.md`; bump the `reviewed-through` dates.
6. Verify. `mingw32-make check`, the preview matrix, then `mingw32-make build`.
   Drive the changed pages in the live app when the change is visual.

## 4. Independent review

Model routing changes; inspect it before spawning (the router's tier fields,
e.g. `magpie claude-desktop`). If every model alias resolves to the same
family, no independent reviewer exists — skip this step and say so in the
report.

Otherwise keep a decision trail in `.audit/<task>.tsv`. Spawn a fresh reviewer
on a different model family with the report, the trail, and the diff. Fix every
confirmed flag, then have a second fresh reviewer check the fixes. Report the
gate as closed only when an independent reviewer is impossible for another
reason.

## 5. Release gate

The `release` skill runs this before the changelogs. Block the release on a red
copy lint, an unresolved P0, or a terminology split. Land the fixes as their
own commit before `build: release <version>`; the changelogs describe the
user-visible result.

## The rules that bite

- One entity, one name: 接收器, 托盘面板, 播放, 采集, 媒体卡片, 氛围灯,
  顶屏. The page and nav say 正在播放; the card says 媒体卡片. The full table
  is in `docs/ui-copy.md`.
- A display switch controls presence in the tray panel only. The media card and
  the latency row use `托盘面板中显示` + switch; hiding either leaves the
  preview and the option rows active, and the feature keeps working. The
  ambient card is permanent and carries a power action (关灯/开灯); off darkens
  the top display and keeps the card.
- Notes: a caveat about something outside the card sits under the card as
  `infoNote` with the info glyph in a 16-DIP slot, 11.5 px muted, wrapping. A
  description or interaction hint stays inside the card as plain text. The
  caveat names the control or setting it concerns.
- Header: a 22-DIP tile, a 13/600 title, an optional tag chip, and the trailing
  control in a 44-DIP row.
- Ladder: 26 row action, 28 field commit, 30 card action, 34 dialog. Rows are
  56 DIP. Type is 8.5/10.5/11.5/12.5/13/15/18. Cards are 10 in the flyout and
  12 in settings; dialogs 14. Chips are 22 status, 20 inline, 18 count.
- State wording has one long form and one chip form, e.g. 正在连接… / 连接中,
  正在通过 {name} 播放 / 正在播放. Latency uses `settings.LatencyText`: under
  1000 ms prints `700 ms`, otherwise `1.5 s`.
- The update surface names the release the build is, marks a local build as
  本地构建, and never shows the git describe suffix.

## Gotchas

- `ui.NewTester.HasText` is a substring match. A test can pass on a neighbouring
  row. Use the `locales` accessor or assert the absence of the old string.
- `catalog_gen.go` is generated. Edit the JSON, then run
  `go tool mygo generate cmd/saundo`.
- The live capture is DPI-virtualized; the headless matrix is the full visual.
- `mingw32-make` needs `$HOME/mingw64/bin` on PATH. The wrapper at
  `$HOME/bin/mingw32-make` covers a shell that started before the PATH change.
- The preview matrix covers zh-CN and en; check both, light and dark.
