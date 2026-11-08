#!/usr/bin/env bash
# Build saundo, replace the running instance, and start the new one.
#
# A running saundo.exe locks bin/saundo.exe on Windows, so the build stops
# the app before the copy and waits for the handle to go, then launches the
# new binary. On macOS the same cycle goes through the .app bundle in
# bin/saundo.app, and the old instance is asked to quit so the receiver
# keeps its advertisements. `mingw32-make build` runs this script, and so
# does `make build` on macOS; `--fast` skips the NSIS installer for the
# development loop.
set -u

fast=0
case "${1:-}" in
	--fast) fast=1 ;;
	"") ;;
	*) echo "usage: $0 [--fast]" >&2; exit 2 ;;
esac

cd "$(dirname "$0")/.."

# A local build carries git describe, so a commit past a release sorts below
# that release and the updater still offers it. Set VERSION to stamp another
# version; the release workflow stamps the real one.
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
VERSION="${VERSION#v}"
GO="${GO:-go}"

case "$(uname -s)" in
Darwin)
	# The Now Playing adapter: an Objective-C framework the app's resources
	# carry, loaded at runtime through /usr/bin/perl (see docs/platforms.md).
	set -e
	cmake -S native/mediaremote-adapter -B cmd/saundo/build/adapter-build -DCMAKE_BUILD_TYPE=Release
	cmake --build cmd/saundo/build/adapter-build
	mkdir -p cmd/saundo/build/adapter/bin
	cp native/mediaremote-adapter/bin/mediaremote-adapter.pl cmd/saundo/build/adapter/bin/
	cp -R cmd/saundo/build/adapter-build/MediaRemoteAdapter.framework cmd/saundo/build/adapter/
	(
		cd cmd/saundo
		CGO_ENABLED=0 MYGO_LDFLAGS="-X saundo/internal/app.Version=$VERSION" \
			"$GO" tool mygo build -platform darwin/arm64 -skip-build-command
	)
	set +e

	# Ask the running app to quit so HomePod keeps its advertisement; force
	# only if it will not go.
	osascript -e 'tell application "saundo" to quit' >/dev/null 2>&1
	sleep 1
	pkill -x saundo >/dev/null 2>&1

	src="cmd/saundo/build/darwin-arm64/saundo.app"
	if [ ! -d "$src" ]; then
		echo "build: $src is missing; mygo build produced no app bundle" >&2
		exit 1
	fi
	mkdir -p bin
	rm -rf bin/saundo.app
	cp -R "$src" bin/saundo.app

	open bin/saundo.app
	;;
*)
	set -e
	build_args=(-platform windows/amd64 -skip-build-command)
	if [ "$fast" -eq 1 ]; then
		build_args+=(-skip-installer)
	fi
	(
		cd cmd/saundo
		CGO_ENABLED=0 MYGO_LDFLAGS="-X saundo/internal/app.Version=$VERSION" \
			"$GO" tool mygo build "${build_args[@]}"
	)
	set +e

	taskkill //F //IM saundo.exe >/dev/null 2>&1

	copied=0
	for _ in $(seq 1 10); do
		if cp cmd/saundo/build/windows-amd64/saundo.exe bin/saundo.exe 2>/dev/null; then
			copied=1
			break
		fi
		sleep 1
	done
	if [ "$copied" -ne 1 ]; then
		echo "build: bin/saundo.exe is still locked; close saundo and run again" >&2
		exit 1
	fi

	if [ "$fast" -ne 1 ]; then
		cp cmd/saundo/build/windows-amd64/saundo-setup-*.exe bin/ 2>/dev/null
	fi

	cd bin
	cmd //c start "" saundo.exe
	;;
esac
