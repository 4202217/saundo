# saundo feature map

The maintained verification source: one page per user-facing feature, each
answering — from the user's point of view — what it is, how to reach it, how
to drive it, and what end state proves it. Read `.claude/skills/verify/SKILL.md`
for the harnesses, the doctor, and the evidence rules.

A proof that drives one convenient entry point is incomplete when the page
lists others. A journey that fails for a missing precondition goes into that
page's Gotchas; never weaken the journey to make it pass.

| Feature | Surface | Primary harness |
|---|---|---|
| [Discovery and connect](discovery-and-connect.md) | Flyout device card | Live app + protocol tests |
| [Session controls](session-controls.md) | Flyout playback card | Headless UI + live app |
| [Tray and flyout](tray-and-flyout.md) | Tray icon, panel behavior | E2E UI + live app |
| [Settings window](settings-window.md) | `设置 · 声渡`, nine pages | Headless UI + E2E UI |
| [Ambient light](ambient-light.md) | Settings 氛围灯 page, flyout ambient card | Headless UI + live app |
| [Updates](updates.md) | Settings 更新 page, notes dialog | Headless UI + live app |

For the flyout, tray menu, and settings surfaces, the catalog in
`internal/app/ui_catalog*.go` and the preview are the evidence source.
`go run ./scripts/check-ui render -out <dir>` writes the CPU matrix in both
languages from the catalog rows.
`SAUNDO_PREVIEW_RENDER=<dir> go test -tags preview ./internal/app -run
TestPreviewRender -count=1` writes the platform renders from the same rows.
Name the scenario ID in the evidence. The catalog ID joins the matrix, the
platform render, the site projection, and the page you drove live.

Cross-cutting states that touch several features: silence suspend (the pacer
pauses after 5 s of digital silence), the idle standby that darkens the top
display after the configured silence threshold, the 3 s streaming-snapshot
hold during a latency restart, and Reduce Motion (Windows animation effects
off) — check the relevant page's Gotchas before concluding a bug.
