# Site reference

`site/index.html` draws the tray flyout by hand, in HTML, CSS, and a small
script that plays a demo. The page is a second implementation of the same
interface. It is not a mock and it is not a test double, so the values have one
source, and gates fail when the copy drifts.

The source of the site projection was the `worktree-render-demo` research
branch at `f3f6bf8`. Its data types and gates were ported onto the shared
fixture and catalog; the branch was not merged.

## The dataset

`site/assets/demo-data.js` is generated, not edited. It carries:

- the fixture's receivers, with model, auth kind, online state, selected row,
  and stereo group;
- the sessions the card can switch between, each with its own track list,
  duration, and position;
- the volume, the latency tier, its three presets, and the custom value;
- the opening flyout state: phase, standby, applying, pairing, and PTP;
- the lamp's priority, template ID, and — when a cover exists — the cover
  lamp's base color and seeded blobs;
- the six ambient templates, with both catalog names, their three colors, and
  their seed;
- the IDs of the catalog scenarios marked `SiteDemo`.

The page reads those values and keeps its own layout, type, and demo logic. A
receiver's name is never translated; the page localizes labels, not data.

## Regenerate

From the repository root:

```bash
SAUNDO_DEMO_OUT=../../site/assets/demo-data.js \
  go test -tags preview -run TestExportReplicaData ./internal/app/
```

The environment path is read from `internal/app`, the test's working
directory. The generated file is committed with the change that produces it.

## The gates

`mingw32-make check` runs three checks, split by what each needs:

- `go run ./scripts/check-ui tokens` compares the numbers. For the page, it
  checks the 15 `--fl-*` palette tokens against `internal/app/theme.go` in
  both appearances, the 11 DIP metrics in `flyout.go` and `ui_kit.go` against
  the CSS declarations they produced, and `flyoutWidth` against the page's
  `.fly-wrap` width. For the two sketch templates it checks 44 palette tokens
  in both appearances, 31 metrics, the flyout height rule, and the drawn
  window sizes against `theme.go`, `flyout.go`, `ui_kit.go`,
  `settings_window.go`, `lamp.go`, and `settings_window_create.go`. The page
  lays the panel out at 420 px and scales it, so there is one layout to keep
  faithful.
- `go run ./scripts/check-ui words` renders the real flyout in zh-CN and in
  English, takes every catalog message that view shows, and requires the page
  to carry it. It reads visible text, script strings, and `aria-label`, `alt`,
  and `title` values. Messages the page does not carry yet sit in
  `replicaTextGaps`; the test fails when one is fixed but still declared, and
  when a declared gap is no longer rendered.
- `TestReplicaDataIsCurrent` fails when the committed dataset and the fixture
  part ways, and prints the regenerate command.

Neither gate can judge shape: a missing affordance, a fill in the wrong color,
or a control that grew heavier than the real one stays with visual review.

## Visual pass

The automated gates cannot judge shape, so a release inspects the page once.

1. Serve the repository root with `python -m http.server 8000`, then open
   `http://localhost:8000/site/`.
2. Inspect 1400x900 and 320 px wide, in zh-CN and English, light and dark.
3. Confirm the anatomy: header, receiver rows, playback card, latency
   segments and volume, ambient light, status line.
4. Confirm no clipped or wrapped copy and no console errors.
5. Save the screenshots to `.audit/release-<version>/site/` and name the
   directory in `docs/verification.md`.

There is no committed reference PNG pair to compare against. The page may
change its story, but its anatomy has to stay the real one.

The page and the sketch templates keep separate HTML copies. The token gate
keeps their copied numbers in sync, and the wording gate keeps the page's
flyout copy in sync. A shared component is deferred until a structural rule
has to change in both copies for one design decision twice, which is the
point where the gate stops paying for itself.

## What the page may change

The replica may change the story it tells — how many receivers it has rows for,
which track is showing, which latency tier — but the anatomy has to stay the
real one: header, receivers, playback, latency segments and volume, ambient
light, status line. The dataset supplies the values; the page supplies the
arrangement.
