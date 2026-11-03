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
| [Tray and flyout](tray-and-flyout.md) | Tray icon, panel behavior | Live app |
| [Settings window](settings-window.md) | `设置 · saundo`, six pages | Headless UI + live app |
| [Updates](updates.md) | Settings 更新 page, notes dialog | Headless UI + live app |

Cross-cutting states that touch several features: silence suspend (the pacer
pauses after 2 s of digital silence), the 3 s streaming-snapshot hold during
a latency restart, and Reduce Motion (Windows animation effects off) — check
the relevant page's Gotchas before concluding a bug.
