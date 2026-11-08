---
name: lamp-palette
description: Add or refine a HomePod ambient-light template palette. Use when choosing the three colors for a template, adding a new template, or debugging a palette that reads white, muddy, or monochrome on the top display.
---

# HomePod ambient-light palettes

The top display never shows the template JPEG. `NowPlayingCap` clusters the
artwork in Lab, turns the result into three `MediaColors`, and
`SoundGlowRenderer` drives the LED ring from those colors plus the device's
own audio envelope. The JPEG is a palette carrier. Design the three colors,
not the picture.

## What the receiver does

- The artwork is clustered in Lab; the three colors become the noise color
  stops, and the primary drives the breathing blob.
- Each stop is renormalized so at least one channel is full, then the
  brightness curve maps 0..0.7 to 0..0.2 and adds 0.015. Absolute lightness
  in the carrier does not survive; hue and chroma do.
- Pale and near-white colors wash to white. A pure gold with a zero blue
  channel survives; a pale gold does not. The 2026-11-08 dune iteration:
  `#E0B000`/`#C47A1E`/`#F0C050` read white; `#C08A00`/`#C47A1E`/`#7A3F14`
  read dark gold.
- The layout is invisible. The carrier is three flat color fields with a
  narrow feather; the app preview paints the extracted three-color glow.

## Design rules

- Three colors and area weights that sum to 1. The default is 45/30/25.
- Keep every pair at least ΔE 12 apart; aim for 25+ unless the theme is
  intentionally monochrome. Dune is a golden family; its closest pair is
  14.5.
- Avoid pale, near-white, and near-black colors. Keep at least one channel
  low; zero blue is the strongest guard for gold.
- One primary (the largest area) plus two supporting colors. The device's
  exact primary-selection rule is not reversed; make all three work.

## Check a palette

```bash
go run ./scripts/lamp-palette -preset dune
go run ./scripts/lamp-palette -c '#E0B000' -c '#C47A1E' -c '#7A3F14' -w 45,30,25
```

The tool builds the production carrier, runs the local `lamp.CoverPreset`
k-means, and prints the recovered colors, pairwise ΔE, the scale-by-max
device stops, and the dimmed stops. It exits 1 when a color is not recovered
within ΔE 12 or a pair collapses below ΔE 12. The stops are an
approximation; the top display decides.

## Add a template

1. Pick three colors and weights; run the tool until it passes.
2. Add the preset to `internal/lamp/presets.go`: `ID`, `Colors`, `Weights`,
   `Base: scaleColor(Colors[0], 0.20)`, and a fresh `Seed`.
3. Add the localized name to `cmd/saundo/locales/en.json` and
   `zh-CN.json` (the `lampPreset*` keys) and a case to `lampPresetName` in
   `internal/app/lamp.go`. The catalog is generated; `mingw32-make check`
   runs the i18n step.
4. Update `docs/ui.md` (the template names), `docs/verification.md` (the
   pinned tests and the device observation), and the verify feature map's
   template list in `.claude/skills/verify/features/ambient-light.md`.
5. Run `go test ./internal/lamp/`, then `mingw32-make check` and
   `mingw32-make build`.
6. Render and inspect the native previews:
   `SAUNDO_PREVIEW_DIR=<absolute dir> go test ./internal/app -run TestRenderPreviews`,
   then open `settings-lamp-light-zh.png`.
7. Eyeball the real HomePod. The top display is the final judge. Record the
   result in `docs/verification.md` and `docs/devices.md`.

## Tests that pin the workflow

- `TestPresetCarriersRecoverTheirPalette`: each carrier's k-means recovers
  every designed color within ΔE 12.
- `TestPresetColorsAreDistinct`: every pair is at least ΔE 12.
- `TestPresetWeightsSumToOne`: the carrier areas are exact.

## Gotchas

- The local k-means is a model, not the device. The receiver's exact cluster
  count and sampling are not reversed.
- The device's color-stop normalization is not fully reversed; the tool's
  scale-by-max stops are an approximation. The top display decides.
- A monochrome theme still needs three Lab-separated colors or the receiver
  may merge them; the golden dune is the worked example.
- The app preview paints the palette, not the live LED frames; do not tune
  the preview to match the device's animation.
