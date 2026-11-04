# Releasing saundo

The release workflow builds a signed update archive and a Windows installer, uploads them to a draft GitHub release, and leaves the release unpublished until the draft checks pass and publishing is approved. Installed apps update from the published release.

Two human inputs remain. The maintainer's pre-release test of the build on their own PC is the smoke test. The publish approval can come up front in the release request, otherwise it is asked once after the draft checks pass.

The agent runbook is [.claude/skills/release/SKILL.md](../.claude/skills/release/SKILL.md). The mechanical preflight is [scripts/release-check.sh](../scripts/release-check.sh). Keep this page, the skill, and the script in step.

## Prerequisites

- Go 1.27+ and the `mygo` tool pinned in `go.mod`.
- GitHub CLI access to `4202217/saundo`.
- A public repository, or a public `updates.url` host. The update check is unauthenticated. A private repository returns 404 for `/releases/latest/download/update-<target>.json` even after the release is published.
- The update signing key. `mygo keygen` wrote it to `%APPDATA%\mygo\update-keys\mygo-update.key`; the public half is in `cmd/saundo/mygo.json` under `updates.publicKey`.
- The repository secret `MYGO_UPDATER_PRIVATE_KEY`. Anyone who has the key can ship code to installed apps. Losing it strands them.
- A Windows code signing certificate only if the release uses signing. The current workflow builds unsigned installers.

```bash
gh secret set MYGO_UPDATER_PRIVATE_KEY < "C:\Users\<you>\AppData\Roaming\mygo\update-keys\mygo-update.key"
```

## Docs audit

Audit the docs against the tree before the release commit. Fix drift in the same commit.

- `README.md` and `README.zh-CN.md`: the feature list, screenshots, install, and update surface. The update surface is the settings Updates page and its notes dialog. There is no standalone update window.
- `docs/ui.md`, `docs/implementation.md`, `docs/protocol.md`, `docs/devices.md`, and `docs/architecture.md`: the code and behavior they describe.
- `docs/verification.md`: the `Currently pending` list and the dated evidence. Keep every unverified feature in the list.
- `CHANGELOG.md`: the English canonical. It must be ASCII and use English UI descriptions. Add the `## <version> - <date>` section.
- `CHANGELOG.zh-CN.md`: the Chinese counterpart with the same version headings. The app's update history reads it when `App.Locale` starts with `zh`.
- Links and screenshots. Bump the `reviewed-through` dates on the docs you touched.

## Version and changelog

The version is `cmd/saundo/mygo.json` `version`. The release notes are the `## <version> - <date>` section of `CHANGELOG.md`. `mygo build` embeds that section in the update manifest, and the settings Updates page shows the release history from the localized changelog.

```bash
VERSION="$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' cmd/saundo/mygo.json | head -n 1)"
```

1. Bump `version` in `cmd/saundo/mygo.json`.
2. Add the matching section to `CHANGELOG.md` and `CHANGELOG.zh-CN.md`.
3. Build the candidate, check its update manifest, and record the maintainer's pre-release test in `docs/verification.md`.
4. Commit the version, both changelogs, the docs audit, and the maintainer's test record as `build: release <version>`.
5. Run the preflight, then tag that release commit. Do not add a commit between the release commit and the tag.

## Local gate

Run the acceptance gate before the release commit:

```bash
make check
```

If make is absent, run the commands in `AGENTS.md`.

Build the app locally:

```bash
VERSION="$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' cmd/saundo/mygo.json | head -n 1)"
cd cmd/saundo
MYGO_LDFLAGS="-X saundo/internal/app.Version=$VERSION" \
  go tool mygo build -platform windows/amd64 -skip-installer
```

Build the signed update files without uploading them:

```bash
cd cmd/saundo
VERSION="$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' mygo.json | head -n 1)"
MYGO_LDFLAGS="-X saundo/internal/app.Version=$VERSION" \
  MYGO_UPDATER_PRIVATE_KEY="$(cat "$APPDATA/mygo/update-keys/mygo-update.key")" \
  go tool mygo build -platform windows/amd64 -skip-installer
```

Check `build/windows-amd64/update-windows-amd64.json`. It must carry the expected version, the English changelog notes, and a signature. The maintainer's pre-release test on their own PC is the human smoke test; record it in `docs/verification.md`. The release does not ask for a second install or a clean-machine run.

After the release commit, run the preflight:

```bash
bash scripts/release-check.sh
```

The script checks the clean tree, the release commit subject, the version, the tag, the English and Chinese changelogs, the docs files, links and screenshots, the public repository, the update secret, the release workflow, and the current update feed. Stop on a failure.

## Tag and push

Pushing a `v*` tag runs `.github/workflows/release.yml`. The tag points to the release commit.

1. `check` runs the catalogs, vet, and tests.
2. `draft` checks the tag against `mygo.json` and creates a draft release.
3. `build` stamps the app version, builds the installer and signed update files, and uploads them to the draft.

Check the draft without installing it:

- The assets are present: the installer, the update archive, the signed manifest, `install.ps1`, and `SHA256SUMS`.
- `sha256sum -c SHA256SUMS` passes.
- The manifest carries the expected version, the English changelog notes, and a signature.
- The installer is unsigned unless signing is configured. SmartScreen warns.

The maintainer's pre-release test covers install and run behavior. A clean Windows install is optional and runs only when the maintainer asks for it.

The draft is invisible to the updater. The previous release updates to this one after publish. Record the draft verification in `docs/verification.md` in a commit on main after the tag. Never move the tag to that commit.

Set the draft body from the bilingual release notes before publishing. The workflow's generated notes are sparse.

```bash
VERSION="$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' cmd/saundo/mygo.json | head -n 1)"
mkdir -p .audit
bash scripts/release-notes.sh "$VERSION" > ".audit/release-notes-$VERSION.md"
gh release edit "v$VERSION" --notes-file ".audit/release-notes-$VERSION.md"
```

The release body carries the English section and the Chinese section below `<!-- lang:zh -->`. The app's update history reads the localized changelog. The signed manifest notes come from the English `CHANGELOG.md`.

Publish the draft when the checks pass and the release request authorized publishing; otherwise ask once:

```bash
gh release edit "v$VERSION" --draft=false
```

After publishing, verify the public feed:

```bash
curl -fsSL "https://github.com/4202217/saundo/releases/latest/download/update-windows-amd64.json"
```

The manifest must be the new version and carry a signature. The installer download must return 200. A live update from the previous release needs an installed build; record it in `docs/verification.md` when the maintainer next runs one, and do not block on it. Installed apps see the release only after it is published. The update manifests are uploaded last, so an app never sees a manifest whose files are missing.

## First release

The first updater-enabled release is the baseline. A build from before the updater cannot update. The 0.3.0 build predates the updater; the first update path runs from 0.4.0 to a later published version.

## Beta releases

Tag a beta as `v0.5.0-beta.1` and publish its GitHub release as a prerelease. Apps with **Beta updates** on call `SetPrereleases(true)` and see it. Stable apps skip it. `CHANGELOG.md` and `CHANGELOG.zh-CN.md` must use the same version heading. The first stable release after the beta must use a higher stable version.

## Signing

Windows code signing is optional. The current release is unsigned, and SmartScreen warns. To sign, add one of these to `cmd/saundo/mygo.json`:

```json
"windows": {
  "certificate": "build/code-signing.pfx",
  "timestampUrl": "http://timestamp.digicert.com"
}
```

Or use `windows.signCommand` for Azure Trusted Signing or a hardware token. The password comes from `MYGO_WINDOWS_CERTIFICATE_PASSWORD`. In CI, write the decoded certificate to the configured path before the build and set the password secret. Signing covers the executable, the installer, and the uninstaller.

## Rollback

Keep the installers of earlier releases available. If a release is broken:

- Publish a fixed patch with a higher version. Installed apps update to it.
- If a version crashes before its update check, point users at the previous installer in the release notes.
- Never delete or rotate the update signing key to fix a release. A new key cannot authenticate updates for apps built with the old one.

## GPL compliance

The repository is GPL-3.0-only. A binary release must carry the license texts and point at the corresponding source:

- Include `LICENSE`, `NOTICE`, and `LICENSES/` in the installer or beside the release assets.
- Link the release notes to the source tag.
- Keep the third-party notices for the runtime dependencies in the binary distribution.
