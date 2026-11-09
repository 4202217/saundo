---
name: release
description: Cut and publish a saundo release. Use when the user asks to release, tag, publish, or prepare release notes for a new version.
---

# Release saundo

Read `AGENTS.md` and `docs/releasing.md` before you start. `docs/releasing.md` is the human checklist. This skill adds the agent gates and the exact commands.

## Human gates

Two, and no more. Treat the release request as the maintainer's confirmation that they tested this build on their own PC; record that test and do not ask for another. Publish approval arrives in the release request when it says to publish once the checks pass; without it, ask once, after the draft checks pass. Run everything else without asking.

## Stop conditions

Stop and report when any of these is true:

- The working tree is dirty.
- The docs audit has an open item.
- The verification skill audit reports a product defect.
- The UI review skill reports an unresolved P0 or terminology finding.
- `cmd/saundo/mygo.json` and the tag disagree.
- `mingw32-make check` fails.
- The local build or its update manifest is wrong.
- The repository is private.
- `MYGO_UPDATER_PRIVATE_KEY` is missing.
- The release workflow fails.
- A draft check fails.
- The public update feed does not serve the new manifest after publish.

Do not tag, push, or publish around a failed check. Do not rotate `MYGO_UPDATER_PRIVATE_KEY`. Do not make the repository private.

## 1. Version and range

Use the version from the release request as `v<version>`. Without one, pick the next semver from the unreleased changelog (features minor, fixes patch), state it in your first line, and proceed. The range is every commit since the last release tag. The version in `cmd/saundo/mygo.json` must match the tag. A beta uses `0.4.2-beta.1`. A stable release uses `0.4.2`.

## 2. Docs audit, UI review, mockup templates, and changelogs

Run the `Docs audit` section in `docs/releasing.md`. Fix drift in the release commit.

- `docs/ui.md`, `docs/implementation.md`, `docs/protocol.md`, `docs/devices.md`, and `docs/architecture.md`: the code and behavior they describe.
- `docs/verification.md`: the `Currently pending` list and dated evidence. Keep every unverified feature in the list. Compress fully graduated dated batches into the file's Graduated section, per the rule in its header.
- `CHANGELOG.md`: the English canonical. Add the `## <version> - <date>` section. Keep it ASCII. Describe the UI in English. Do not use Chinese UI labels.
- `CHANGELOG.zh-CN.md`: the Chinese counterpart. It needs the same version headings and sections. The app's update history reads it when `App.Locale` starts with `zh`.
- Links and screenshots. Bump the `reviewed-through` dates on the docs you touched.

Run the UI review skill (`/ui-review`) before you write the changelogs. It inventories the copy, renders the preview matrix, and lists copy and layout findings by severity. Resolve every P0 and every terminology finding. Land the fixes as their own commit before the release commit; the changelogs describe the user-visible result. A red copy lint or an unresolved P0 stops the release. Record the preview matrix path and the UI review outcome in `docs/verification.md`.

Refresh the UI mockup templates after the UI review settles the interface and before the changelogs. The two assets under `.claude/skills/ui-mockup/assets/` and the skill's height rule must describe the interface this release ships, not the previous one. Compare them with the shipped palette, widgets, and geometry in `internal/app/theme.go`, `ui_kit.go`, `flyout.go`, `settings_window.go`, and `receivers_page.go`; update tokens, widget sizes, shell geometry, sample content, and both locale strings. Render the settings target, normal, and edge states and the flyout's idle, streaming, empty, and error states in zh-CN and English, light and dark, plus the flyout's media card and ambient states, into `.audit/release-<version>/mockups/`. Inspect every screenshot for clipping, overflow, and wrapped copy, and record the render directory in `docs/verification.md`. Land the template update as its own commit before the release commit, like the UI review fixes. Follow the render recipe in `.claude/skills/ui-mockup/SKILL.md`.

Do not commit the docs, README, or changelog changes yet. The release commit comes after the maintainer's test is recorded.

## 3. Update the READMEs

Update `README.md` and `README.zh-CN.md` in the release commit. Both are user-facing and must say the same thing.

- Add or revise the feature list for the release's user-visible changes. The Chinese file uses the Chinese UI labels.
- Keep the install section and the update surface current. The update surface is the settings Updates page and its notes dialog. There is no standalone update window.
- Check that every screenshot and link the two files reference resolves.

## 4. Maintain the verification skill

Run `/pstack:maintain-verification-skill` before the gate. It audits `.claude/skills/verify/` from source and drives every feature once.

- `clean`: continue.
- `changed`: land the proven corrections on the release branch as their own commit before the release commit. If it opened a PR, merge or cherry-pick it first. Do not fold these changes into `build: release <version>`.
- `blocked`: record the coverage gap in `docs/verification.md` and continue when the gap is coverage. A proven product defect stops the release.

## 5. Gate, local build, and the maintainer's test

Run the acceptance gate:

```bash
mingw32-make check
```

If mingw32-make is absent, run the four commands in `AGENTS.md`.

Build the release candidate:

```bash
cd cmd/saundo
VERSION="$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' mygo.json | head -n 1)"
MYGO_LDFLAGS="-X saundo/internal/app.Version=$VERSION" \
  MYGO_UPDATER_PRIVATE_KEY="$(cat "$APPDATA/mygo/update-keys/mygo-update.key")" \
  go tool mygo build -platform windows/amd64
```

Check `build/windows-amd64/update-windows-amd64.json`. It must carry the version, the English changelog notes, and a signature. The release request confirms the maintainer tested this build on their own PC. Record that pre-release test in `docs/verification.md`, with the date and version, from what the request says. Do not ask for a second install or a clean-machine run.

## 6. Release commit and preflight

Commit the version, both changelogs, both READMEs, the docs audit, and the maintainer's test record under `build: release <version>`. End the commit body with the `AGENTS.md` fields. The `Docs:` field names the files.

Then run:

```bash
bash scripts/release-check.sh
```

The script checks the clean tree, the release commit subject, the version, the tag, the English changelog, the Chinese changelog, the docs files, links and screenshots, the public repository, the update secret, the release workflow, and the current update feed. Stop on any failure.

## 7. Tag the release commit

The tag points to the release commit. Do not add a commit between the release commit and the tag. The tag creates the draft only; publishing is the public step. Tag and push:

```bash
VERSION=...
git tag -a "v$VERSION" -m "saundo $VERSION"
git branch -f main HEAD
git push origin main
git push origin "v$VERSION"
```

The tag commit subject is `build: release <version>`. The preflight enforces it. The tag push runs `.github/workflows/release.yml`.

## 8. Watch the workflow

```bash
gh run list --workflow release.yml --limit 1 --json databaseId,status
gh run watch <id> --exit-status
```

The workflow runs `check`, `draft`, and `build`. The build uploads the installer, the update archive, the signed manifest, `install.ps1`, and `SHA256SUMS` to the draft. Stop if any job fails. Read the failed log before changing anything.

## 9. Verify the draft

```bash
gh release view "v$VERSION" --json isDraft,assets
mkdir -p ".audit/draft-$VERSION"
gh release download "v$VERSION" --dir ".audit/draft-$VERSION"
(cd ".audit/draft-$VERSION" && sha256sum -c SHA256SUMS)
```

Check the draft without installing it. The assets are present: the installer, the update archive, the signed manifest, `install.ps1`, and `SHA256SUMS`. The checksums pass. The manifest carries the version, the English notes, and a signature. The installer is unsigned unless signing is configured. The maintainer's pre-release test covers install and run behavior; install the draft on a clean machine only when the user asks.

The draft is invisible to the updater. The previous release updates to this one after publish. Record the draft verification in `docs/verification.md` in a commit on main after the tag. Never move the tag to that commit.

## 10. Release notes and publish

The workflow's generated notes are sparse. Set the draft body from the bilingual release notes. The script prints the English section, the `<!-- lang:zh -->` marker, and the Chinese section:

```bash
VERSION=...
mkdir -p .audit
bash scripts/release-notes.sh "$VERSION" > ".audit/release-notes-$VERSION.md"
gh release edit "v$VERSION" --notes-file ".audit/release-notes-$VERSION.md"
```

The release body carries both languages. The app's update history reads the localized changelog. The signed manifest notes come from the English `CHANGELOG.md`.

Publish when the release request authorized it; otherwise ask once, with the draft checks in hand:

```bash
gh release edit "v$VERSION" --draft=false
```

## 11. Verify the public feed

```bash
curl -fsSL "https://github.com/4202217/saundo/releases/latest/download/update-windows-amd64.json"
```

The manifest must be the new version and carry a signature. The installer download must return 200. A live update from the previous release needs an installed build; record it in `docs/verification.md` when the maintainer next runs one, and do not block on it.

The repository must stay public. A private repository returns 404 for the feed even after publish. If the repository is private, stop and use a public `updates.url` host instead.

## 12. Beta releases

Use a version such as `0.4.2-beta.1`. Set `mygo.json`, `CHANGELOG.md`, and `CHANGELOG.zh-CN.md` to the exact same version headings. Tag `v0.4.2-beta.1` and publish with `--prerelease`. Apps with Beta updates see it. Stable apps skip it. The first stable release after the beta must use a higher stable version.

## 13. Rollback, signing, GPL

For rollback, signing, and GPL compliance, follow `docs/releasing.md`. Do not rotate the update key. Keep the previous installers available. The source tag is the GPL source for the release.

## Report

State which checks ran, what they printed, and which did not run. Name the release URL, the draft state, the feed result, the update test, the verification skill outcome, the UI review outcome (clean, changed, blocked), the preview matrix path, and the mockup template refresh (changed, already current, or the recorded gap) with its render directory. If you stopped, name the failing gate and the file or command that proves it.
