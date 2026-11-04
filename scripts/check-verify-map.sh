#!/usr/bin/env bash
# Check that the verification skill's feature map only asserts facts that
# still exist in the source. Every TestXxx, SAUNDO_* gate, quoted locale
# string, .go path with line citation, and Go symbol named in
# .claude/skills/verify/ must be findable in the tree, so docs drift turns
# into a red gate instead of a wrong live verification. Wired into
# `mingw32-make check`; safe to run standalone.
#
# Classification of a backticked span:
#   CJK text          -> each CJK run must exist in a locale catalog or .go
#   bare identifier   -> must appear as a word in Go source; all-uppercase
#                        wire vocabulary may live in protocol/devices docs
#   bare <file>.go(:n)-> file must exist, line must land inside it
#   anything else     -> commands, flags, placeholders: prose, skipped
set -u
cd "$(dirname "$0")/.." || exit 2

errors=0
fail() { printf 'FAIL %s\n' "$*"; errors=$((errors + 1)); }

MAPDIR=.claude/skills/verify
if [ ! -d "$MAPDIR" ]; then
	echo "no $MAPDIR; nothing to check"
	exit 0
fi

# Spans that name things outside this tree on purpose: framework window
# classes (pinned live by the doctor) and external tools.
allowlist='^(MyGoFlyoutShadow|MyGoWindow|gofmt|taskkill|curl|powershell|mygo)$'

# check_go_file <basename> <optional :line spec> — the file must exist in
# the tree and the cited line must land inside it.
check_go_file() {
	name=$1 linespec=$2
	path=$(git ls-files -- '*.go' | grep "/${name}\$" | head -1)
	if [ -z "$path" ]; then
		fail "map: $name does not exist in the tree"
		return
	fi
	if [ -n "$linespec" ]; then
		n=${linespec#:}; n=${n%%-*}
		total=$(wc -l < "$path")
		[ "$n" -le "$total" ] 2>/dev/null \
			|| fail "map: $path has $total lines; citation says $n"
	fi
}

files=("$MAPDIR/SKILL.md" "$MAPDIR"/features/*.md)
spans_tmp=$(mktemp)
trap 'rm -f "$spans_tmp"' EXIT

for f in "${files[@]}"; do
	[ -f "$f" ] || continue
	base=${f#"$MAPDIR"/}

	# 1. Test names, anywhere in prose or recipes (also pipe-separated lists).
	for name in $(grep -oE 'Test[A-Z][A-Za-z0-9_]*' "$f" | sort -u); do
		grep -rqE "func ${name}\(" --include='*_test.go' cmd internal \
			|| fail "$base: test $name does not exist in any _test.go"
	done

	# 2. Env-var gates.
	for ev in $(grep -oE 'SAUNDO_[A-Z0-9_]+' "$f" | sort -u); do
		grep -rqF "$ev" --include='*.go' cmd internal \
			|| fail "$base: env gate $ev not found in source"
	done

	# 3. Directory-prefixed .go paths with optional line citations.
	for p in $(grep -oE '(cmd|internal|scripts)/[A-Za-z0-9_./-]+\.go(:[0-9]+(-[0-9]+)?)?' "$f" | sort -u); do
		path=${p%%:*}
		if [ ! -f "$path" ]; then
			fail "$base: path $path does not exist"
			continue
		fi
		line=${p#"$path"}
		if [ -n "$line" ]; then
			n=${line#:}; n=${n%%-*}
			total=$(wc -l < "$path")
			[ "$n" -le "$total" ] 2>/dev/null \
				|| fail "$base: $path has $total lines; citation says $n"
		fi
	done

	grep -oE '`[^`]+`' "$f" | sed 's/^`//; s/`$//' >> "$spans_tmp"
done

for span in $(LC_ALL=C sort -u "$spans_tmp"); do
	grep -qE "$allowlist" <<< "$span" && continue

	# Bare .go citation, with or without a directory prefix.
	if grep -qE '^([A-Za-z0-9_./-]+/)?[A-Za-z0-9_]+\.go(:[0-9]+(-[0-9]+)?)?$' <<< "$span"; then
		name=$(basename "${span%%:*}")
		check_go_file "$name" "${span#*[a-z0-9].go}"
		continue
	fi

	# CJK text: check each CJK run as a UI string.
	if grep -qP '[\x{4e00}-\x{9fff}]' <<< "$span" 2>/dev/null; then
		for run in $(grep -oP '[\x{4e00}-\x{9fff}]{2,}' <<< "$span"); do
			grep -rqF "$run" cmd/saundo/locales 2>/dev/null \
				|| grep -rqF "$run" --include='*.go' cmd internal 2>/dev/null \
				|| fail "map: string $run not in locales or source"
		done
		continue
	fi

	# Symbol-shaped spans: identifiers that name real Go things.
	if grep -qE '^[A-Za-z_][A-Za-z0-9_]{3,}$' <<< "$span"; then
		case "$span" in Test*|SAUNDO_*) continue ;; esac
		if grep -rqE "\<${span}\>" --include='*.go' cmd internal 2>/dev/null; then
			continue
		fi
		if grep -qE '^[A-Z0-9_]+$' <<< "$span" \
			&& grep -rqF "$span" docs/protocol.md docs/devices.md 2>/dev/null; then
			continue
		fi
		fail "map: symbol $span not found in source or docs"
	fi
	# Everything else (commands, flags, placeholders, log lines) is prose.
done

if [ "$errors" -gt 0 ]; then
	printf '%d stale map assertion(s)\n' "$errors"
	exit 1
fi
echo "map ok: every test name, env gate, path, string, and symbol resolves"
