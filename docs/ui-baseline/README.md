# UI pixel baseline

This directory is the committed visual baseline for the native UI. It holds
`manifest.json` and the 24 PNGs it describes: 12 canonical state stems, each
rendered in English and Simplified Chinese by the CPU matrix
(`go test -tags preview ./internal/app -run TestRenderPreviews`). The matrix
uses the software rasterizer, and the manifest records it as
`ui.Tester/raster`.

The baseline is per host. Windows is the reference host and the only host
that diffs it: the Windows `check` jobs in `.github/workflows/ci.yml` and
`.github/workflows/release.yml` render the 12 stems and run `check-ui diff`.
The macOS jobs build and test every change but do not compare pixels. Add a
macOS baseline only when a macOS contributor needs one, as a separate
directory and manifest; never mix hosts in one baseline.

## Canonical states

| Stem | Surface | State |
| --- | --- | --- |
| `flyout-stream-light` | Flyout | Playing, sample track, light |
| `flyout-stream-dark` | Flyout | Playing, sample track, dark |
| `flyout-connecting` | Flyout | Handshake in progress |
| `flyout-error` | Flyout | Receiver did not respond |
| `flyout-empty` | Flyout | No receivers |
| `settings-general-light` | Settings | General page |
| `settings-connection-dark` | Settings | Connection page, dark |
| `settings-receivers-expanded-light` | Settings | Receivers page, manual row and hidden group open |
| `settings-audio-custom-light` | Settings | Audio page, 700 ms custom latency |
| `settings-lamp-cover-light` | Settings | Lamp page, cover mode |
| `settings-nowplaying-live-light` | Settings | Now Playing page, live card |
| `dialog-receiver-add` | Dialog | Add receiver, over the receivers page |

Twelve stems times two locales is 24 PNGs. The stems are stable in the Phase 0
matrix; the 11 animated files are never canonical.

## Regenerate

Two full renders prove the matrix is stable, the stable manifest keeps only
the files that match, and `subset` copies the canonical set here.

```bash
go run ./scripts/check-ui render -out .audit/ui/regen-a
go run ./scripts/check-ui render -out .audit/ui/regen-b
go run ./scripts/check-ui baseline \
  -a .audit/ui/regen-a/manifest.json \
  -b .audit/ui/regen-b/manifest.json \
  -out .audit/ui/regen-stable.json
go run ./scripts/check-ui subset \
  -baseline .audit/ui/regen-stable.json \
  -src .audit/ui/regen-a \
  -states "flyout-stream-light,flyout-stream-dark,flyout-connecting,flyout-error,flyout-empty,settings-general-light,settings-connection-dark,settings-receivers-expanded-light,settings-audio-custom-light,settings-lamp-cover-light,settings-nowplaying-live-light,dialog-receiver-add" \
  -out docs/ui-baseline
```

`subset` refuses a stem that matches no file, refuses a PNG whose bytes do not
match the stable manifest, and replaces this directory's PNGs and manifest so
a removed state cannot survive. It never invents a file. Once a stable
manifest is on disk, the regenerate is two commands: `check-ui render` writes
the states, and `check-ui subset` selects them into this directory.

## Update an intentional change

Membership is fixed. A state joins or leaves only through an explicit change
to this directory and to the state list in the CI render command, reviewed
with the code that changes it.

For a visual change under review, render the full canonical set and update the
existing file set:

```bash
go run ./scripts/check-ui render -out .audit/ui/update \
  -only "flyout-stream-light,flyout-stream-dark,flyout-connecting,flyout-error,flyout-empty,settings-general-light,settings-connection-dark,settings-receivers-expanded-light,settings-audio-custom-light,settings-lamp-cover-light,settings-nowplaying-live-light,dialog-receiver-add"
go run ./scripts/check-ui rebaseline \
  -baseline docs/ui-baseline/manifest.json \
  -against .audit/ui/update/manifest.json
```

`rebaseline` keeps the baseline file set, refuses a file that is missing from
the new render, verifies every new PNG against the new manifest, updates the
changed hashes, and copies the PNGs beside the output manifest. Review the
PNG diff and the manifest diff, then commit both. Update the baseline only
when the visual change is the one under review.

## The diff

```bash
go run ./scripts/check-ui diff \
  -baseline docs/ui-baseline/manifest.json \
  -against .audit/ui/ci/manifest.json
```

The diff is exact. It verifies that each manifest describes the PNGs beside
it before comparing the two manifests, and it prints the canonical count on a
pass. A changed hash or size fails with the file name. A missing manifest, a
missing PNG, a renderer mismatch, or a PNG whose bytes disagree with its
manifest is inconclusive and exits 2; it is never a pass. `go run` reports a
child's non-zero status as `exit status 2` and itself exits 1, so build the
command when the raw exit code matters.
