#!/usr/bin/env bash
# Reject the preview tag in GOFLAGS. mygo copies GOFLAGS tags into its own
# build tags, so a leaked export would compile the preview fixture and the
# fake handlers into the shipping binary. Local build scripts and the release
# workflow call this before any mygo build.
set -u

case " ${GOFLAGS:-} " in
*preview*)
	echo "build: GOFLAGS must not carry the preview tag" >&2
	exit 2
	;;
esac
