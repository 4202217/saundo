# Releasing saundo

The release workflow builds a signed update archive and a Windows installer,
uploads them to a draft GitHub release, and leaves the release unpublished
until a person has checked it. Installed apps update from the published
release.

## Prerequisites

- Go 1.27+ and the `mygo` tool pinned in `go.mod`.
- GitHub CLI access to `4202217/saundo`.
- The update signing key. `mygo keygen` wrote it to
  `%APPDATA%\mygo\update-keys\mygo-update.key`; the public half is in
  `cmd/saundo/mygo.json` under `updates.publicKey`.
- A Windows code signing certificate, once signing is configured. The
  workflow currently builds unsigned installers.

Keep the update signing key in a password manager and in the repository
secret `MYGO_UPDATER_PRIVATE_KEY`. Anyone who has it can ship code to
installed apps. Losing it strands them on the current version.

```bash
gh secret set MYGO_UPDATER_PRIVATE_KEY < "C:\Users\<you>\AppData\Roaming\mygo\update-keys\mygo-update.key"
```

## Version and changelog

The version is `cmd/saundo/mygo.json` `version`. The release notes are the
`## <version>` section of `CHANGELOG.md`; `mygo build` embeds that section
in the update manifest and the update window shows it.

1. Bump `version` in `cmd/saundo/mygo.json`.
2. Add the matching `## <version> - <date>` section to `CHANGELOG.md`.
3. Commit both changes.
4. Tag the commit `v<version>` and push the tag. The workflow checks that
   the tag and the configuration version agree before it builds anything.

## Local gate

Run the acceptance gate before tagging:

```bash
make check
```

Or run the commands directly:

```bash
go tool mygo i18n check cmd/saundo
gofmt -l .
go vet ./...
go tool mygo vet .
go test ./...
```

Build the app locally:

```bash
cd cmd/saundo
MYGO_LDFLAGS="-X saundo/internal/app.Version=$(jq -r .version mygo.json)" \
  go tool mygo build -platform windows/amd64 -skip-installer
```

Build the signed update files without uploading them:

```bash
cd cmd/saundo
MYGO_UPDATER_PRIVATE_KEY="$(cat "$APPDATA/mygo/update-keys/mygo-update.key")" \
  go tool mygo build -platform windows/amd64 -skip-installer
```

Check `build/windows-amd64/update-windows-amd64.json`: it must carry the
expected version, the changelog notes, and a signature.

## Publish

Pushing a `v*` tag runs `.github/workflows/release.yml`:

1. `check` runs the catalogs, vet, and tests.
2. `draft` checks the tag against `mygo.json` and creates a draft release.
3. `build` stamps the app version, builds the installer and signed update
   files, and uploads them to the draft.

Review the draft, then install it on a clean Windows 10 or 11 machine and
check:

- the installer runs, installs per user, and appears in Settings > Apps;
- the app discovers a receiver, connects, and streams;
- the previous release updates to the new one through the update window;
- settings, credentials, and the language choice survive the update;
- uninstalling leaves no surprise data, or documents what it keeps;
- SmartScreen accepts the installer once code signing is configured.

Publish the draft when the checks pass:

```bash
gh release edit v0.4.0 --draft=false
```

Installed apps see the release after it is published. The update manifests
are uploaded last, so an app never sees a manifest whose files are missing.

## Beta releases

Tag a beta as `v0.5.0-beta.1` and publish its GitHub release as a
prerelease. Apps with **Beta updates** on call `SetPrereleases(true)` and
see it; stable apps skip it. The `CHANGELOG.md` section must use the same
version string, `## 0.5.0-beta.1`.

## Signing

Windows code signing is not configured yet. Add one of these to
`cmd/saundo/mygo.json`:

```json
"windows": {
  "certificate": "build/code-signing.pfx",
  "timestampUrl": "http://timestamp.digicert.com"
}
```

or use `windows.signCommand` for Azure Trusted Signing or a hardware token.
The password comes from `MYGO_WINDOWS_CERTIFICATE_PASSWORD`. In CI, write
the decoded certificate to the configured path before the build and set the
password secret. Signing covers the executable, the installer, and the
uninstaller.

## Rollback

Keep the installers of earlier releases available. If a release is broken:

- publish a fixed patch with a higher version; installed apps update to it;
- if a version crashes before its update check, point users at the previous
  installer in the release notes;
- never delete or rotate the update signing key to fix a release. A new key
  cannot authenticate updates for apps built with the old one.

## GPL compliance

The repository is GPL-3.0-only. A binary release must carry the license
texts and point at the corresponding source:

- include `LICENSE`, `NOTICE`, and `LICENSES/` in the installer or beside
  the release assets;
- link the release notes to the source tag;
- keep the third-party notices for the runtime dependencies in the binary
  distribution.
