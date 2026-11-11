#!/usr/bin/env bash
# Drive a full in-app update on macOS without touching a release.
#
# The script copies this tree to a temporary directory, builds an old and a
# new app there, signs the new update with a throwaway Ed25519 key, and
# gives the old app a local directory that holds the signed darwin manifest
# and its archive. The old build carries a test-only source (-tags
# updateharness) that reads those files instead of the network; the updater
# still checks the archive against the public key baked into the app. The
# script launches the old app with its own bundle identifier and HOME,
# waits for it to download and stage the update, quits it so the staged
# bundle replaces the old one, then checks the new version, its signature,
# and the quarantine attribute.
#
# It never touches the installed app, its settings, its credentials, or the
# published update feed. The update path reads from the local directory; the
# app's other features, such as release notes and discovery, can still use the
# network. macOS can ask for system audio capture and local network access for
# the test identifier. The update needs neither, so allow or deny them as they
# appear.
#
# Usage: bash scripts/test-mac-update.sh
#        BUILD_ONLY=1 bash scripts/test-mac-update.sh   stop after the builds
#        KEEP=1 bash scripts/test-mac-update.sh        keep the temporary tree
#
# Not covered: the in-app 立即重启 button. The helper's relaunch path is
# covered by mygo's TestUpdaterWaitsForTheAppToExit and by the live
# prerelease check in docs/verification.md.
set -euo pipefail

if [ "$(uname -s)" != "Darwin" ]; then
	echo "test-mac-update: macOS only" >&2
	exit 2
fi
for tool in go python3 rsync ditto codesign xattr; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "test-mac-update: $tool is required" >&2
		exit 2
	fi
done

root="$(cd "$(dirname "$0")/.." && pwd)"
go="${GO:-go}"
old_version="${OLD_VERSION:-0.6.1}"
new_version="${NEW_VERSION:-0.6.2}"
work="$(mktemp -d "${TMPDIR:-/tmp}/saundo-update-test.XXXXXX")"
repo="$work/repo"
feed="$work/feed"
home="$work/home"
apps="$work/apps"
keys="$work/keys"
settings_dir="$home/Library/Application Support/saundo"
log="$settings_dir/saundo.log"
app_pid=""

cleanup() {
	if [ -n "$app_pid" ] && kill -0 "$app_pid" 2>/dev/null; then
		kill -TERM "$app_pid" 2>/dev/null || true
		wait "$app_pid" 2>/dev/null || true
	fi
	# A staged update's helper waits for the app and holds the bundle paths.
	pkill -f "$apps/" 2>/dev/null || true
	if [ "${KEEP:-0}" = "1" ]; then
		echo "test-mac-update: kept $work"
	else
		rm -rf "$work"
	fi
}
trap cleanup EXIT INT TERM

say() { printf 'test-mac-update: %s\n' "$*"; }
dump() {
	echo "--- app log ---" >&2
	tail -40 "$log" 2>/dev/null || true
	echo "--- app stdout ---" >&2
	tail -20 "$work/app.out" 2>/dev/null || true
}
fail() {
	echo "FAIL: $*" >&2
	dump
	exit 1
}

mkdir -p "$feed" "$home" "$apps" "$keys" "$settings_dir"
say "copying the tree to $repo"
rsync -a --exclude='.git' --exclude='bin' --exclude='cmd/saundo/build' --exclude='.zcode' "$root/" "$repo/"

if [ ! -d "$repo/cmd/saundo/resources/darwin-arm64/adapter" ]; then
	say "building the MediaRemote adapter"
	(cd "$repo" && bash scripts/build-adapter.sh) >"$work/adapter.log" 2>&1 || {
		tail -20 "$work/adapter.log" >&2
		fail "the adapter build failed"
	}
fi

say "making a throwaway update key"
(cd "$repo/cmd/saundo" && "$go" tool mygo keygen -o "$keys") >/dev/null
pub="$(tr -d '\n' < "$keys/mygo-update.pub")"
priv="$keys/mygo-update.key"

# mygo build requires an HTTPS updates.url even when the test source serves
# the files. example.invalid never resolves, and the source replaces it.
feed_url="https://example.invalid/saundo"

config="$repo/cmd/saundo/mygo.json"
python3 - "$config" "$old_version" "$pub" "$feed_url" <<'PY'
import json, sys
path, version, pub, url = sys.argv[1:5]
with open(path) as f:
    cfg = json.load(f)
cfg["version"] = version
cfg["identifier"] = "com.saundo.update.test"
updates = cfg.setdefault("updates", {})
updates["publicKey"] = pub
updates["url"] = url
updates.pop("github", None)
updates["deltas"] = 0
with open(path, "w") as f:
    json.dump(cfg, f, indent=2, ensure_ascii=False)
    f.write("\n")
PY
cat >> "$repo/CHANGELOG.md" <<EOF

## $new_version - 2026-11-11

Local update harness test.
EOF

say "building $old_version with the harness source"
(cd "$repo/cmd/saundo" && GOFLAGS="-tags=updateharness" \
	MYGO_LDFLAGS="-X saundo/internal/app.Version=$old_version" \
	"$go" tool mygo build -platform darwin/arm64 -skip-build-command -skip-dmg -sign -) >"$work/old-build.log" 2>&1 || {
	tail -20 "$work/old-build.log" >&2
	fail "the $old_version build failed"
}
old_app="$repo/cmd/saundo/build/darwin-arm64/saundo.app"
[ -d "$old_app" ] || fail "the $old_version build produced no app bundle"
ditto "$old_app" "$apps/saundo.app"
old_ver="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$apps/saundo.app/Contents/Info.plist")"
[ "$old_ver" = "$old_version" ] || fail "the old bundle is $old_ver, want $old_version"

python3 - "$config" "$new_version" <<'PY'
import json, sys
path, version = sys.argv[1:3]
with open(path) as f:
    cfg = json.load(f)
cfg["version"] = version
with open(path, "w") as f:
    json.dump(cfg, f, indent=2, ensure_ascii=False)
    f.write("\n")
PY

say "building $new_version with the test key"
(cd "$repo/cmd/saundo" && MYGO_UPDATER_PRIVATE_KEY="$(cat "$priv")" \
	MYGO_LDFLAGS="-X saundo/internal/app.Version=$new_version" \
	"$go" tool mygo build -platform darwin/arm64 -skip-build-command -skip-dmg -sign -) >"$work/new-build.log" 2>&1 || {
	tail -20 "$work/new-build.log" >&2
	fail "the $new_version build failed"
}
new_dir="$repo/cmd/saundo/build/darwin-arm64"
manifest="$new_dir/update-darwin-arm64.json"
archive="$new_dir/saundo-$new_version-darwin-arm64.tar.gz"
[ -f "$manifest" ] || fail "the new build wrote no darwin manifest"
[ -f "$archive" ] || fail "the new build wrote no darwin archive"
cp "$manifest" "$archive" "$feed/"
[ -f "$new_dir/install.sh" ] && cp "$new_dir/install.sh" "$feed/"
python3 - "$manifest" "$new_version" "$feed_url" <<'PY'
import json, sys
path, version, feed = sys.argv[1:4]
with open(path) as f:
    m = json.load(f)
assert m["version"] == version, m
assert m["signature"], m
assert m["url"].startswith(feed + "/"), m["url"]
print("test-mac-update: manifest %s, archive %d bytes" % (m["version"], m["size"]))
PY

if [ "${BUILD_ONLY:-0}" = "1" ]; then
	say "build-only: manifest and archive are in $feed"
	exit 0
fi

cat > "$settings_dir/settings.json" <<'JSON'
{
  "autoUpdate": true,
  "autoDownload": true
}
JSON

say "launching $old_version from $apps/saundo.app"
HOME="$home" \
SAUNDO_UPDATE_HARNESS="$feed" \
SAUNDO_CREDS_MASTER_KEY="$(printf '0123456789abcdef%.0s' 1 2 3 4)" \
"$apps/saundo.app/Contents/MacOS/saundo" >"$work/app.out" 2>&1 &
app_pid=$!
for _ in $(seq 1 100); do
	if [ -f "$log" ] && grep -q "saundo $old_version" "$log"; then
		break
	fi
	kill -0 "$app_pid" 2>/dev/null || fail "the app exited during launch"
	sleep 0.1
done
grep -q "saundo $old_version" "$log" || fail "the app did not log its version"

say "waiting for $new_version to install"
ready=0
for _ in $(seq 1 900); do
	if grep -q "update: $new_version installed, restart to apply" "$log"; then
		ready=1
		break
	fi
	if grep -q "update: check failed" "$log"; then
		break
	fi
	kill -0 "$app_pid" 2>/dev/null || break
	sleep 0.1
done
[ "$ready" = 1 ] || fail "the app did not stage $new_version"

say "quitting the app so the staged bundle replaces the old one"
kill -TERM "$app_pid" 2>/dev/null || true
for _ in $(seq 1 100); do
	kill -0 "$app_pid" 2>/dev/null || break
	sleep 0.1
done
kill -KILL "$app_pid" 2>/dev/null || true
wait "$app_pid" 2>/dev/null || true
app_pid=""

say "waiting for the bundle swap"
swapped=0
for _ in $(seq 1 300); do
	if [ -f "$apps/saundo.app/Contents/Info.plist" ]; then
		v="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$apps/saundo.app/Contents/Info.plist" 2>/dev/null || true)"
		if [ "$v" = "$new_version" ]; then
			swapped=1
			break
		fi
	fi
	sleep 0.1
done
if [ "$swapped" != 1 ]; then
	ls -la "$apps" >&2
	fail "the bundle was not replaced"
fi

say "verifying the updated bundle"
codesign --verify --deep --strict "$apps/saundo.app" || fail "the updated bundle does not verify"
if xattr -l "$apps/saundo.app" 2>/dev/null | grep -q 'com.apple.quarantine'; then
	fail "the updated bundle carries com.apple.quarantine"
fi
if ls -a "$apps" | grep -q '^\.saundo.app.update-'; then
	fail "the update helper left its work directory behind"
fi

say "PASS: $old_version -> $new_version in place, signature valid, no quarantine"
