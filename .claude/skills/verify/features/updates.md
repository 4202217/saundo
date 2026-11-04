# Updates

## Sub-features

- Update check against the public feed
  (`update-windows-amd64.json` on the releases URL, fetched without
  credentials, 10 s after launch then every 24 h — `update.go:335`);
  states: checking, up to date, available, downloading (with progress),
  ready, failed.
- 更新 page in the settings window: channel (stable/beta), automatic check
  and download toggles, a check-now action, and the primary action button
  (Install / Restart now / Retry / Cancel by state).
- Release-notes dialog: the offered version's notes plus, when fetchable,
  the history of every version since the running build — from `CHANGELOG.md`
  in English and `CHANGELOG.zh-CN.md` when the app language starts with
  `zh` (`update_history.go`, pinned by `TestChangelogURLLocale`).
- Install downloads, verifies the signature, and swaps the files in place
  (the running exe is renamed aside — no installer process runs, the app
  keeps running). A separate 立即重启 relaunches into the new version.
- Update chip in the flyout **footer** (`flyout.go:332-352`) shares state
  with the settings page; clicking it opens the 更新 page.

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

Open the 更新 page, click the check, and capture the state transition; the
page and the log agree on available/downloading/ready/failed.

Proven end state: the page shows the manifest version from the feed, the
notes dialog opens with the release history in the app's language, and a
failed check renders the failed state instead of hanging.

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
