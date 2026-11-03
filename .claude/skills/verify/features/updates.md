# Updates

## Sub-features

- Update check against the public feed
  (`update-windows-amd64.json` on the releases URL); states: up to date,
  available, downloading, ready, failed.
- 更新 page in the settings window: channel (stable/beta), automatic check
  and download toggles, skipped version, last check time.
- Release-notes dialog: the history list reads `CHANGELOG.md` in English and
  `CHANGELOG.zh-CN.md` when the app language starts with `zh`.
- Install from the page (runs the downloaded installer).
- Update chip in the flyout shares state with the settings page.

## How to get to it (user POV)

Settings → 更新 page. The chip in the flyout header opens the same state.

## Driving it

Headless UI renders the states:

```bash
go test -run 'TestSettingsPagesRender|TestUpdateStatesRender' -v ./internal/app/
```

Live app: the feed must be public — a 404 here means the release is a draft
or the repository is private, not that the check is broken. Open the 更新
page, click the check, and capture the state transition; the log and the
page agree on available/downloading/ready/failed.

Proven end state: the page shows the manifest version from the feed, the
notes dialog opens with the release history in the app's language, and a
failed check renders the failed state instead of hanging.

## Gotchas

- The feed is fetched without credentials; a private repository or draft
  release serves nothing (observed 2026-11-02: 404 until the release was
  published and visibility restored).
- The first updater shipped in 0.4.0 — older builds have no update path.
- The update history is localized: switch the app language and confirm the
  dialog reads the Chinese changelog; the file must stay in sync with the
  English one.
- An install attempt closes the app — treat "install from the page" as a
  fresh-launch journey (stop conditions in SKILL.md apply) and verify the
  new version via the doctor afterwards.
