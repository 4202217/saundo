# Security policy

## Reporting a vulnerability

Do not open a public issue for a security problem. Use GitHub's private
vulnerability reporting:

https://github.com/4202217/saundo/security/advisories/new

Include the version, the Windows version, the receiver, and the steps to
reproduce. Please do not include receiver passwords, pairing keys, or other
users' data.

## Scope

Security-sensitive parts of saundo:

- AirPlay pairing and session credentials, stored with the OS secret store:
  Windows DPAPI, and the macOS login-Keychain master key with AES-256-GCM.
  A legacy receiver password that cannot be moved into that store stays in
  the legacy `settings.json` map, is retried on the next start, and is
  reported by the startup log's kept-password count;
- the encrypted RTSP control and event channels;
- the update signing key, the update manifest, and the installer signature;
- the vendored MediaRemote adapter used by Now Playing on macOS;
- the GitHub release workflow and its secrets.

## Supported versions

Only the latest published release receives fixes. Update from the app's
update page or from the releases page.
