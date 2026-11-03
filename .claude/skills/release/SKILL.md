---
name: release
description: Cut and publish a saundo release. Use when the user asks to release, tag, publish, or prepare release notes for a new version.
---

# Release saundo

Read `AGENTS.md` and `docs/releasing.md` before you start. `docs/releasing.md` is the human checklist. This skill adds the agent gates and the exact commands.

## Stop conditions

Stop and report when any of these is true:

- The working tree is dirty.
- The docs audit has an open item.
- `cmd/saundo/mygo.json` and the tag disagree.
- `make check` fails.
- The local build or its update manifest is wrong.
- The repository is private.
- `MYGO_UPDATER_PRIVATE_KEY` is missing.
- The release workflow fails.
- The draft is missing an expected asset.
- The clean Windows test fails.
- The public update feed does not serve the new manifest after publish.

Do not tag, push, or publish around a failed check. Do not rotate `MYGO_UPDATER_PRIVATE_KEY`. Do not make the repository private.

## 1. Confirm the release

Ask the user for the version and the commit range. Use `v<version>` as the tag. The version in `cmd/saundo/mygo.json` must match the tag. A beta uses `0.4.2-beta.1`. A stable release uses `0.4.2`.

## 2. Docs audit

Run the `Docs audit` section in `docs/releasing.md`. Fix drift in the release commit.

- `README.md` and `README.zh-CN.md`: feature list, screenshots, install, and update surface. The update surface is the settings 更新 page and notes dialog. There is no standalone update window.
- `docs/ui.md`, `docs/implementation.md`, `docs/protocol.md`, `docs/devices.md`, and `docs/architecture.md`: the code and behavior they describe.
- `docs/verification.md`: the `Currently pending` list and dated evidence. Keep every unverified feature in the list.
- `CHANGELOG.md`: the `## <version> - <date>` section.
- Links and screenshots. Bump the `reviewed-through` dates on the docs you touched.

Commit the docs audit with the version and changelog under `build: release <version>`. End the commit body with the `AGENTS.md` fields. The `Docs:` field names the files.

## 3. Preflight

After the release commit, run:

```bash
bash scripts/release-check.sh
```

The script checks the clean tree, version, tag, changelog, docs files, links and screenshots, public repository, update secret, release workflow, and current update feed. Stop on any failure.

## 4. Gate and local build

Run the acceptance gate:

```bash
make check
```

If make is absent, run the four commands in `AGENTS.md`.

Build the release candidate:

```bash
cd cmd/saundo
VERSION="$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' mygo.json | head -n 1)"
MYGO_LDFLAGS="-X saundo/internal/app.Version=$VERSION" \
  MYGO_UPDATER_PRIVATE_KEY="$(cat "$APPDATA/mygo/update-keys/mygo-update.key")" \
  go tool mygo build -platform windows/amd64
```

Check `build/windows-amd64/update-windows-amd64.json`. It must carry the version, the changelog notes, and a signature. Install the installer on the Windows test machine and smoke test it.

## 5. Tag and push

Ask the user to confirm the release commit and the docs audit. Then:

```bash
VERSION=...
git tag -a "v$VERSION" -m "saundo $VERSION"
git branch -f main HEAD
git push origin main
git push origin "v$VERSION"
```

The tag must match `cmd/saundo/mygo.json`. The tag push runs `.github/workflows/release.yml`.

## 6. Watch the workflow

```bash
gh run list --workflow release.yml --limit 1 --json databaseId,status
gh run watch <id> --exit-status
```

The workflow runs `check`, `draft`, and `build`. The build uploads the installer, the update archive, the signed manifest, `install.ps1`, and `SHA256SUMS` to the draft. Stop if any job fails. Read the failed log before changing anything.

## 7. Verify the draft

```bash
gh release view "v$VERSION" --json isDraft,assets
```

Install the draft installer on a clean Windows 10 or 11 machine. Check:

- The installer runs, installs per user, and appears in Settings > Apps.
- The app discovers a receiver, connects, and streams.
- The settings 更新 page and notes dialog render. The flyout update chip appears when a newer release exists.
- The previous release updates to this one through the settings 更新 page.
- Settings, credentials, and the language choice survive the update.
- Uninstalling leaves the expected data, or the docs say what it keeps.
- The installer is unsigned unless signing is configured. SmartScreen warns. Check the installer SHA256 against the release `SHA256SUMS`.

Record the result in `docs/verification.md` before publishing. If a check fails, fix it and ship a new version. Do not publish the broken draft.

## 8. Release notes and publish

The workflow's generated notes are sparse. Set the draft body from the changelog section:

```bash
VERSION=...
bash scripts/release-notes.sh "$VERSION" > ".audit/release-notes-$VERSION.md"
gh release edit "v$VERSION" --notes-file ".audit/release-notes-$VERSION.md"
```

Publish after the user confirms the draft verification:

```bash
gh release edit "v$VERSION" --draft=false
```

## 9. Verify the public feed and the update

```bash
curl -fsSL "https://github.com/4202217/saundo/releases/latest/download/update-windows-amd64.json"
```

The manifest must be the new version and carry a signature. The installer download must return 200. On a test machine with the previous release installed, check for updates. It must find the new version and install it. The new build's own check must say it is up to date.

The repository must stay public. A private repository returns 404 for the feed even after publish. If the repository is private, stop and use a public `updates.url` host instead.

## 10. Beta releases

Use a version such as `0.4.2-beta.1`. Set `mygo.json` and `CHANGELOG.md` to the exact same string. Tag `v0.4.2-beta.1` and publish with `--prerelease`. Apps with Beta updates see it. Stable apps skip it. The first stable release after the beta must use a higher stable version.

## 11. Rollback, signing, GPL

For rollback, signing, and GPL compliance, follow `docs/releasing.md`. Do not rotate the update key. Keep the previous installers available. The source tag is the GPL source for the release.

## Report

State which checks ran, what they printed, and which did not run. Name the release URL, the draft state, the feed result, and the update test. If you stopped, name the failing gate and the file or command that proves it.
