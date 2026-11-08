#!/usr/bin/env bash
# Stable entry point for the verification map check. The checker itself is the
# Go command in scripts/verify-map: it reads the tree once and answers every
# assertion from memory, where the old per-span recursive greps spent about a
# minute on Windows. Wired into `mingw32-make check`; safe standalone.
#
# Check that the verification skill's feature map only asserts facts that
# still exist in the source: every TestXxx, SAUNDO_* gate, quoted locale
# string, .go path with a line citation, and Go symbol named in
# .claude/skills/verify/ must be findable in the tree.
set -u
cd "$(dirname "$0")/.." || exit 2
exec "${GO:-go}" run ./scripts/verify-map "$@"
