#!/usr/bin/env bash
# Print the bilingual release notes for a version.
set -u

version="${1:?usage: release-notes.sh <version>}"
cd "$(dirname "$0")/.." || exit 2

section() {
  awk -v v="$version" '$0 == "## " v || index($0, "## " v " ") == 1 {on=1; next} /^## / {on=0} on' "$1"
}

en="$(section CHANGELOG.md)"
zh="$(section CHANGELOG.zh-CN.md)"
if [ -z "$en" ]; then
  printf 'release-notes: no English section for %s\n' "$version" >&2
  exit 1
fi
if [ -z "$zh" ]; then
  printf 'release-notes: no Chinese section for %s\n' "$version" >&2
  exit 1
fi
printf '%s\n\n<!-- lang:zh -->\n\n%s\n' "$en" "$zh"
