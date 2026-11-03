---
name: verify
description: Drive the real saundo app and prove behavior with evidence — launch or attach, run the doctor, exercise a feature, capture proof. Use when a change needs live verification beyond `make check`, when filling a `Currently pending` item in docs/verification.md, or when a bug report needs reproduction.
---

# Verify saundo

`make check` proves the logic. This skill proves the app: launch it (or attach
to the running instance), exercise a feature the way a user would, and leave
evidence behind. The per-feature journeys live in [features/](features/);
docs/verification.md holds the acceptance history and the `Currently pending`
list this skill exists to fill.

## The three harnesses

Pick by what the proof needs. Cheapest that proves the claim wins.

1. **Headless UI** (no window, no device): `go test` renders the real view
   functions through `ui.NewTester` with a fixture snapshot. Clicks
   (`tt.Click("连接")`), text assertions (`tt.HasText`), and PNG frames. Use
   for UI behavior, layout, locale, and motion.
2. **Protocol** (no device): `go test ./internal/airplay/sender/` drives the
   full AP2 handshake against the in-process fake receiver. Use for pairing,
   crypto, RTSP, timing, and stream behavior.
3. **Live app** (real exe, optionally real receiver): the running instance
   observed through its log, windows, and tray; env-gated device tests for
   real hardware. Use when the claim is about WASAPI, mDNS, PTP, the tray, or
   end-to-end behavior. Audible output is always confirmed by the user, never
   by a test.

## Stop conditions

Stop and report when any of these is true:

- `make check` fails — fix or report before driving anything live.
- A saundo instance is running and the proof needs a fresh launch: the
  single-instance lock means you cannot run two, and the running one may be
  the user's streaming session. Ask before stopping it.
- The journey would stream the user's real desktop audio to a real receiver
  (tone tests, capture tests) without the user having agreed to sound in the
  room.
- The proof would delete or overwrite `%APPDATA%\saundo\credentials.dat` or
  `settings.json` — never do either.

## 1. Gate

```bash
make check
```

`gofmt -l .` + `mygo i18n check` + `go vet` + `mygo vet` + `go test ./...`.
If make is missing, run the four commands from AGENTS.md. A live run on a
red gate proves nothing about the change.

## 2. Launch

Two ways to get a drivable instance. Run the doctor (step 3) first — it tells
you which situation you are in.

**Attach** (default): an instance is already running — drive it. Read-only
observation of the user's own instance is always safe. Do not stop a
process you did not start.

**Fresh launch** (needs no running instance):

```bash
make build    # builds via mygo, stops the running app, starts bin/saundo.exe
# or, with console logs:
go run ./cmd/saundo
# or, hidden in the tray:
bin/saundo.exe --tray
```

`make build` kills saundo.exe by image name — that is its job, but it means
the user's instance dies with it. Say so before running it.

Readiness — all three, in order:

1. `%APPDATA%\saundo\saundo.log` gains a `saundo <version>` line (the first
   line every run writes).
2. The tray icon appears (tooltip `saundo` or `saundo · <status>`).
3. A normal launch shows the flyout (window class `MyGoFlyoutShadow`);
   login launch and `--tray` stay hidden — usable means the tray answers,
   not that a window is visible.

A clean-slate run redirects the whole data dir (settings, credentials, log)
via the environment; DPAPI still decrypts because it is user-keyed:

```bash
mkdir -p /tmp/saundo-clean
APPDATA='C:\tmp\saundo-clean' cmd //c start "" "C:\...\bin\saundo.exe"
```

Isolation rules: one instance per session — the single-instance lock is
keyed by app name, not by data dir, so a redirected second launch still
hands over and exits 0. First run shows the Windows Firewall prompt
(mDNS 5353, PTP UDP 319/320); unsolicited inbound must be allowed on the
private network or PTP times out. A fresh dir has no receivers: discovery
takes about ten seconds on a LAN with one.

Teardown: stop only what you started, by PID (step 6).

## 3. Doctor

One read-only check that answers "is this instance worth driving?":

```bash
powershell -NoProfile -ExecutionPolicy Bypass -File .claude/skills/verify/scripts/doctor.ps1
```

Checks process up, binary path and version, data-dir files (names and sizes
only — never prints `credentials.dat`), the last 12 log lines with a
`fatal:` scan, and which windows the PID owns. If anything looks off later
in the run, run the doctor again before concluding anything.

Known trap it reports: `saundo.log` can be missing on disk while the app
runs — a deleted file leaves the app writing to a dangling handle. The live
log is then the flyout's 诊断日志 card (复制日志 copies it to the clipboard).

## 4. Drive

**Headless UI.** The real view code runs in-process:

```bash
mkdir -p "$EV/frames"
SAUNDO_PREVIEW_DIR="$EV/frames" go test -run TestRenderPreviews ./internal/app/
go test -run 'TestViewRendersDevicesAndStatus|TestFlyoutLatencySegmentSelects' -v ./internal/app/
```

`previewApp()` in `internal/app/app_test.go` is the fixture snapshot. A new
journey is a new Go test in the owning package: build the fixture, render
with `ui.NewTester(a.view, w, h)`, click with stable handles — the visible
locale string (`连接`, `断开`, `设置`) is the selector; never coordinates —
assert with `tt.HasText`, and `tt.Frame()` + `png.Encode(tt.Image())` for
evidence. Advance animations with `tt.Advance(d)`.

**Protocol.** The fake receiver decrypts everything and byte-compares audio:

```bash
go test -v ./internal/airplay/sender/
```

**Live app.** Observe, then act, then observe again:

- Observe: the doctor; `tail -f %APPDATA%\saundo\saundo.log`; capture the
  real window (the flyout is layered — PrintWindow, not a screen grab):
  ```bash
  powershell -NoProfile -ExecutionPolicy Bypass -File .claude/skills/verify/scripts/capture-window.ps1 -Out /tmp/saundo-verify/flyout.png
  # settings window: -Class MyGoWindow -TitleLike "saundo"
  ```
- Act: real user paths only. Click the tray icon or the flyout's own
  buttons; the tray icon lives in the taskbar or the hidden-icons overflow,
  reachable through UI Automation (`saundo` button). Drive the engine's
  real surface (`Engine.Connect`, `SetVolume`, …) only from a Go test, and
  say in the evidence that the path bypassed the UI.
- Judge by log lines with known meanings: `control channel now encrypted`,
  `streaming to "<name>"`, `SET_PARAMETER volume` answered 200, `session
  stopped`, `fatal:`. Failure meanings are tabled in
  docs/verification.md — match the symptom before changing code.

**Live receiver** (needs hardware on the LAN, mutes the receiver first):

```bash
SAUNDO_MDNS_TEST=1   go test -run TestBrowseIntegration -v ./internal/discovery/
SAUNDO_DEVICE_TEST=1 go test -run TestLiveHomePodHandshake -v ./internal/airplay/sender/
SAUNDO_STEREO_TEST=1 go test -run TestLiveStereoGroup -v ./internal/airplay/sender/
SAUNDO_MAC_TEST=1 SAUNDO_MAC_PASSWORD=... go test -run TestLiveMacPassword -v ./internal/airplay/sender/
```

`SAUNDO_DEVICE_TONE=1` sends an audible 440 Hz tone — only with the user's
OK. The PTP capture test runs inside an active session
(`SAUNDO_PTP_PEER=<ip>`); see docs/verification.md.

## 5. Evidence

One directory per run, outside the repo, named in the report:

```bash
EV="/tmp/saundo-verify/$(date +%Y%m%d-%H%M%S)"; mkdir -p "$EV"
```

Capture, into `$EV`:

- **The action and the resulting state**, not just the final screen: the PNG
  before and after a click, the log excerpt spanning the action (copy
  `saundo.log`, or paste 复制日志 output into `$EV/log.txt`).
- Side effects where they land: `settings.json` / `receivers.json` copies
  (plain config — fine to copy), the exit code of a second-launch handover,
  `go test -v` output. Never copy `credentials.dat` — DPAPI-protected
  secrets are not evidence.
- Headless frames: the PNG the test wrote; name them by state
  (`flyout-before.png`, `flyout-after-click.png`).
- Live window captures via capture-window.ps1.

Proof standards:

- Exercise the real user path. A test that pokes engine methods directly is
  labeled as such in the evidence.
- A silent-desktop WASAPI run proves format negotiation, not audio content.
  Audible output is recorded as "user confirmed".
- When a test mode skips something (muted receiver, silence stream), say
  what was actually observed, not what the flag promises.
- One claim per journey; a claim the map lists under another entry point is
  not covered by this run.

## 6. Cleanup

- Kill only PIDs this run started: `powershell -c "Stop-Process -Id <pid>"`.
  Never `taskkill //IM saundo.exe` from verification — that is build.sh's
  move and it takes the user's instance too.
- A redirected-APPDATA instance: stop it, then delete its scratch dir (it
  holds the run's DPAPI blobs — still scratch). Copy evidence out first.
- Never touch `%APPDATA%\saundo\*` — the user's real credentials, settings,
  and registry live there. If an experiment had to modify it (latency
  migration, pairing), back the file up into `$EV` first and restore after.
- Evidence survives cleanup: it lives in `$EV`, not in scratch. After
  cleanup, `ls "$EV"` and confirm every artifact the report will cite.

## 7. Feature map

Before declaring a feature verified, read its page in
[features/](features/) — it lists the sub-features, the user path, the
harness commands, and the gotchas that already cost someone time. Keep the
map honest: a journey that fails for a missing precondition goes into its
Gotchas; never weaken the journey to make it pass.

## Keeping this skill current

When the app changes shape (new window class, renamed page, moved control),
update features/ and this file in the same change. `/maintain-verification-skill`
is the maintenance loop for exactly that.
