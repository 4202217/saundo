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
	# It lands in the darwin/arm64 platform resource directory, so no other
	# platform's build needs it. A committed signing identity would name one
	# developer's certificate; pass one for a stable local identity, else
	# sign ad hoc.
	set -e
	bash scripts/build-adapter.sh
	sign="${SAUNDO_MAC_SIGN_IDENTITY:--}"
	(
		cd cmd/saundo
		CGO_ENABLED=0 MYGO_LDFLAGS="-X saundo/internal/app.Version=$VERSION" \
			"$GO" tool mygo build -platform darwin/arm64 -skip-build-command -sign "$sign"
	)
	set +e

	src="cmd/saundo/build/darwin-arm64/saundo.app"
	if [ ! -d "$src" ]; then
		echo "build: $src is missing; mygo build produced no app bundle" >&2
		exit 1
	fi
	mkdir -p bin
	# Stage the new bundle before touching the running install, so a failed
	# copy leaves the previous app in place.
	rm -rf bin/.saundo.app.new
	if ! cp -R "$src" bin/.saundo.app.new; then
		echo "build: could not stage the new app bundle in bin/" >&2
		exit 1
	fi

	# Ask the running app to quit so HomePod keeps its advertisement; force
	# only if it will not go.
	osascript -e 'tell application "saundo" to quit' >/dev/null 2>&1
	sleep 1
	pkill -x saundo >/dev/null 2>&1

	rm -rf bin/.saundo.app.old
	if [ -d bin/saundo.app ] && ! mv bin/saundo.app bin/.saundo.app.old; then
		echo "build: could not move the old app bundle aside" >&2
		exit 1
	fi
	if ! mv bin/.saundo.app.new bin/saundo.app; then
		echo "build: could not install the new app bundle" >&2
		if [ -d bin/.saundo.app.old ]; then mv bin/.saundo.app.old bin/saundo.app; fi
		exit 1
	fi

	if ! open bin/saundo.app; then
		echo "build: the new app bundle did not open; restoring the previous one" >&2
		rm -rf bin/saundo.app
		if [ -d bin/.saundo.app.old ]; then
			mv bin/.saundo.app.old bin/saundo.app
			open bin/saundo.app >/dev/null 2>&1
		fi
		exit 1
	fi
	rm -rf bin/.saundo.app.old
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
		if ! cp cmd/saundo/build/windows-amd64/saundo-setup-*.exe bin/; then
			echo "build: could not copy the installer to bin/" >&2
			exit 1
		fi
	fi

	cd bin
	cmd //c start "" saundo.exe
	;;
esac
