#!/usr/bin/env bash
# Build saundo, replace the running instance, and start the new one.
#
# A running saundo.exe locks bin/saundo.exe on Windows, so the build stops
# the app before the copy and waits for the handle to go, then launches the
# new binary. `mingw32-make build` runs this script.
set -u

cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
VERSION="${VERSION#v}"
GO="${GO:-go}"

set -e
(
	cd cmd/saundo
	CGO_ENABLED=0 MYGO_LDFLAGS="-X saundo/internal/app.Version=$VERSION" \
		"$GO" tool mygo build -platform windows/amd64 -skip-build-command
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

cp cmd/saundo/build/windows-amd64/saundo-setup-*.exe bin/ 2>/dev/null

cd bin
cmd //c start "" saundo.exe
