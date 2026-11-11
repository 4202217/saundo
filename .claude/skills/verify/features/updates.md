# Updates

Both platforms use the same updater. Each check reads the platform's signed
manifest from the latest published release, `update-windows-amd64.json` or
`update-darwin-arm64.json`, 10 s after launch and then every 24 h. A build
that has no feed or cannot write where it is installed reports the disabled
state: the version row says "Updates aren't available in this build", removes
the chip, and makes no request. The disabled flag is in memory only, so a
fresh launch shows the chip again.

## Sub-features

- Update check against the public feed
  (the platform's manifest on the releases URL, fetched without
  credentials, 10 s after launch then every 24 h — `update.go:356-380`,
  timer at `:362`, ticker at `:370`); every non-manual check returns early
  when the automatic-check preference is off (`update.go:112-115`). States:
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
- Install downloads, verifies the signature, and replaces the app. On
  Windows it swaps the files in place (the running exe is renamed aside, no
  installer process runs). On macOS it stages the new bundle and a helper
  swaps it after the app quits, so the Dock keeps one tile. A separate
  立即重启 relaunches into the new version.
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

Live: the feed must be public. A 404 means the release is a draft or the
repository is private, not that the check is broken. Probe the platform's
manifest read-only:

```bash
curl -sL -o /dev/null -w '%{http_code}' \
  https://github.com/4202217/saundo/releases/latest/download/update-windows-amd64.json
curl -sL -o /dev/null -w '%{http_code}' \
  https://github.com/4202217/saundo/releases/latest/download/update-darwin-arm64.json
```

Open the 更新 page, click the check, and capture the state transition; the
page and the log agree on available/downloading/ready/failed. The disabled
path writes no log line (`update.go:116-125` returns before `finishCheck`).

macOS: a published darwin manifest finds an update and installs it in place.
The running app keeps its version until it quits; the helper then swaps the
bundles and reopens the app when 立即重启 asked for it. A 404 for
`update-darwin-arm64.json` means the release predates the mac feed or its mac
build did not upload, not that the check is broken. The first live update
measures whether TCC audio capture and Keychain access survive the new
ad-hoc cdhash; record it in docs/verification.md.

Local: `make test-mac-update` (`scripts/test-mac-update.sh`) builds 0.6.1
with the `updateharness` source and 0.6.2 with a throwaway key in a
temporary tree. `cmd/saundo/harness_update.go` reads the signed manifest and
archive from a directory, so the update path needs no server or feed request.
It drives
check, install, quit, and the bundle swap, then checks the version,
signature, and quarantine attribute. It does not cover the in-app restart
button or the published GitHub feed.

A local `make build` (`mingw32-make build` on Windows) stamps `git
describe`, such as `0.4.2-2-gab1c020-dirty`. A commit past a release sorts
below it, so the public feed still offers that release and the notes dialog
lists it with the history since the build. Installing an update replaces
the app with that release; rebuild to return to a development stamp.

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
- Install does not restart the app by itself. On Windows the swap happens in
  place and the session continues until you click 立即重启. On macOS the swap
  happens when the app quits, so 立即重启 quits and reopens it.
- On macOS an ad-hoc update changes the app's cdhash, so the first launch
  after an update can ask again for system-audio capture and Keychain
  access. Developer ID signing and notarization remove this. The live check
  is in docs/verification.md.
