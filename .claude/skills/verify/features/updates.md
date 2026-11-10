# Updates

Windows today: the macOS build compiles the updater off (`platform_darwin.go`
sets `updatesSupported=false`; the compile gate is `!darwin`) and never starts
the launch or 24 h checks. The 更新 page still opens in its normal state, with
a 检查更新 chip; the first click refuses (no network call) and flips the
version row to "Updates aren't available in this build", removes the chip, and
writes no log line. The flag is in memory only, so a fresh launch shows the
chip again. The toggles and channel selector stay editable. The full surface
returns when a signed, notarized mac feed exists.

## Sub-features

- Update check against the public feed
  (`update-windows-amd64.json` on the releases URL, fetched without
  credentials, 10 s after launch then every 24 h — `update.go:359-384`,
  timer at `:365`, ticker at `:373`); every non-manual check returns early
  when the automatic-check preference is off (`update.go:115-118`). States:
  checking, up to date, available, downloading (with progress), ready,
  failed.
- 更新 page in the settings window: channel (stable/beta), automatic check
  and download toggles, a check-now action, the primary action button
  (Install / Restart now / Retry by state), a Cancel link button, and the
  S1 card's What's New, Skip this version, and Remind me later links beside
  the byte and percent progress row (`settings_window.go:458-477`).
- Release-notes dialog: the offered version's notes plus, when fetchable,
  the history of every version since the running build — from `CHANGELOG.md`
  in English and `CHANGELOG.zh-CN.md` when the app language starts with
  `zh` (`update_history.go`, pinned by `TestChangelogURLLocale`). The
  fetched entry replaces the signed manifest's English notes for the offered
  version (`TestNotesEntriesUseTheLocalizedChangelog`), and a language
  switch refetches the history (`TestLanguageSwitchRefetchesHistory`). The
  head's range select switches between the history since this build and the
  latest version only, and the older entries render as folded rows with a
  history link (`update_dialog.go:36-45,115-132`). The fetch starts when the
  update is found
  (`TestFoundUpdatePrefetchesTheLocalizedNotes`), and a non-English dialog
  shows the loading row rather than flashing English
  (`TestNotesDialogHidesTheEnglishNotesWhileLoading`).
- Install downloads, verifies the signature, and swaps the files in place
  (the running exe is renamed aside — no installer process runs, the app
  keeps running). A separate 立即重启 relaunches into the new version.
- Update chip in the flyout **footer** (`flyout.go:671-693`, chip at
  `:677-691`) shares state with the settings page; clicking it opens the
  更新 page. A waiting update also draws a dot on the 更新 sidebar item, and
  the tray tooltip and menu carry the check action and the available version
  (`settings_window.go:203-206`; `app.go:883,902-908`; docs/ui.md).

## How to get to it (user POV)

Settings → 更新 page. The footer chip in the flyout opens the same state.

## Driving it

Headless UI renders the states:

```bash
go test -run 'TestSettingsPagesRender|TestCheckFindsAnUpdate|TestInstallReachesReady|TestChangelogURLLocale' -v ./internal/app/
```

Live: the feed must be public — a 404 means the release is a draft or the
repository is private, not that the check is broken. Probe it read-only:

```bash
curl -sL -o /dev/null -w '%{http_code}' \
  https://github.com/4202217/saundo/releases/latest/download/update-windows-amd64.json
```

Open the 更新 page, click the check, and capture the state transition; on the
Windows check path the page and the log agree on available/downloading/ready/
failed. The macOS refusal path writes no log line (`update.go:119-126` returns
before `finishCheck`).

macOS, updater compiled off: attach to the running instance (the settings
window is a normal window; do not rebuild while the app is streaming). The
page opens with the 检查更新 chip. Click it once: the chip becomes the
disabled line, no request is made, `settings.json` `lastCheck` stays empty,
and the log gains no `update:` line. A fresh launch is needed to see the chip
again.

A local `mingw32-make build` stamps `git describe`, such as
`0.4.2-2-gab1c020-dirty`. A commit past a release sorts below it, so the
public feed still offers that release and the notes dialog lists it with
the history since the build. Installing an update replaces the binary with
that release; rebuild to return to a development stamp.

Headless coverage: the page shows the manifest version from the feed, the
notes dialog opens with the release history in the app's language, and a
failed check renders the failed state instead of hanging. The live page
states, the release-history list, and install from the page are still owed;
docs/verification.md keeps them in Currently pending.

## Gotchas

- A private repository or draft release serves nothing (observed
  2026-11-02: 404 until the release was published and visibility restored).
- The release-history file is fetched from the repository's default branch:
  a commit that exists only locally (unpushed) 404s for every user until
  pushed — observed 2026-11-04 with `CHANGELOG.zh-CN.md` (25 unpushed
  commits at the time).
- The first updater shipped in 0.4.0 — older builds have no update path.
- The update history is localized: switch the app language and confirm the
  dialog reads the Chinese changelog.
- Install does not close or restart the app by itself — the swap happens in
  place and the session continues until you click 立即重启.
- On macOS the updater is compiled off until a signed, notarized feed exists.
  The page opens in the normal state with the 检查更新 chip; the first click
  flips it to the disabled line, makes no request, and writes no log line.
  The disabled flag is in memory only, so a fresh launch shows the chip
  again. The toggles and channel selector stay editable.
