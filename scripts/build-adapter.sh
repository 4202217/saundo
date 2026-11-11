#!/usr/bin/env bash
#
# Build the vendored MediaRemote adapter and stage it where mygo ships it: a
# platform directory under cmd/saundo/resources, so a Windows checkout never
# needs the macOS-only framework. scripts/build.sh, CI, and the release job
# all call this one script.
set -euo pipefail

case "$(uname -s)" in
Darwin) ;;
*)
	echo "build-adapter: the MediaRemote adapter builds on macOS only" >&2
	exit 1
	;;
esac

root="$(cd "$(dirname "$0")/.." && pwd)"
build="$root/cmd/saundo/build/adapter-build"
out="$root/cmd/saundo/resources/darwin-arm64/adapter"

cmake -S "$root/native/mediaremote-adapter" -B "$build" -DCMAKE_BUILD_TYPE=Release
cmake --build "$build"

rm -rf "$out"
mkdir -p "$out/bin"
cp "$root/native/mediaremote-adapter/bin/mediaremote-adapter.pl" "$out/bin/"
cp -R "$build/MediaRemoteAdapter.framework" "$out/"

# The entitlement self-test needs its helper client. Ship it beside the
# loader; without it test.m exits 1 with "Test client path is missing".
client="$(find "$build" -type f -name MediaRemoteAdapterTestClient | head -n 1)"
if [ -z "$client" ]; then
	echo "build-adapter: MediaRemoteAdapterTestClient was not built" >&2
	exit 1
fi
cp "$client" "$out/bin/MediaRemoteAdapterTestClient"
chmod +x "$out/bin/MediaRemoteAdapterTestClient"
