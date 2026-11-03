#!/usr/bin/env bash
# Print the changelog section for a version.
set -u

version="${1:?usage: release-notes.sh <version>}"
cd "$(dirname "$0")/.." || exit 2

out="$(awk -v v="$version" '$0 == "## " v || index($0, "## " v " ") == 1 {on=1; next} /^## / {on=0} on' CHANGELOG.md)"
if [ -z "$out" ]; then
  printf 'release-notes: no section for %s\n' "$version" >&2
  exit 1
fi
printf '%s\n' "$out"
