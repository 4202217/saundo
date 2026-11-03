#!/usr/bin/env bash
# Read-only preflight for a saundo release. Run after the release commit.
set -u

cd "$(dirname "$0")/.." || exit 2

REPO="4202217/saundo"
errors=0

fail() { printf 'FAIL %s\n' "$*"; errors=$((errors + 1)); }
warn() { printf 'WARN %s\n' "$*"; }
ok() { printf 'ok   %s\n' "$*"; }

version="$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' cmd/saundo/mygo.json | head -n 1)"
if [ -z "$version" ]; then
  fail "cannot read version from cmd/saundo/mygo.json"
  exit 1
fi
tag="v$version"
printf 'release preflight for %s (%s)\n\n' "$version" "$tag"

if [ -n "$(git status --porcelain)" ]; then
  fail "working tree is dirty; commit the release first"
else
  ok "working tree clean"
fi

if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  fail "tag $tag already exists; bump the version"
else
  ok "tag $tag is not created yet"
fi

if git merge-base --is-ancestor main HEAD; then
  ok "main is an ancestor of HEAD"
else
  fail "main is not an ancestor of HEAD; fast-forward main before tagging"
fi

if grep -Fq "## $version" CHANGELOG.md; then
  ok "changelog has ## $version"
else
  fail "CHANGELOG.md has no ## $version section"
fi
if awk -v v="$version" '$0 == "## " v || index($0, "## " v " ") == 1 {on=1; next} /^## / {on=0} on' CHANGELOG.md | grep -q '^-'; then
  ok "changelog section has entries"
else
  fail "changelog section for $version is empty"
fi

if LC_ALL=C grep -q '[^ -~]' CHANGELOG.md; then
  fail "CHANGELOG.md contains non-ASCII text; keep the English canonical changelog ASCII"
else
  ok "CHANGELOG.md is ASCII"
fi
if [ -f CHANGELOG.zh-CN.md ]; then
  en_versions="$(grep '^## ' CHANGELOG.md | sed 's/[[:space:]]*-[[:space:]]*[0-9-]*$//' | sort)"
  zh_versions="$(grep '^## ' CHANGELOG.zh-CN.md | sed 's/[[:space:]]*-[[:space:]]*[0-9-]*$//' | sort)"
  if [ "$en_versions" = "$zh_versions" ]; then
    ok "English and Chinese changelog versions match"
  else
    fail "CHANGELOG.zh-CN.md version headings differ from CHANGELOG.md"
  fi
else
  fail "missing CHANGELOG.zh-CN.md"
fi

subject="$(git log -1 --format=%s)"
if [ "$subject" = "build: release $version" ]; then
  ok "HEAD is the release commit"
else
  fail "HEAD is not build: release $version; tag the release commit, not a later verification commit"
fi

required=(
  AGENTS.md README.md README.zh-CN.md CHANGELOG.md CHANGELOG.zh-CN.md LICENSE NOTICE
  LICENSES/Apache-2.0.txt LICENSES/MIT-pyatv.txt
  docs/architecture.md docs/protocol.md docs/devices.md docs/implementation.md docs/ui.md docs/verification.md docs/releasing.md
  docs/screenshots/flyout-stream-light-en.png docs/screenshots/flyout-stream-light-zh.png
  docs/screenshots/settings-connection-light-en.png docs/screenshots/settings-connection-light-zh.png
  cmd/saundo/mygo.json cmd/saundo/locales/en.json cmd/saundo/locales/zh-CN.json
  internal/app/update.go internal/app/update_dialog.go internal/app/update_history.go
  .github/workflows/release.yml scripts/release-check.sh scripts/release-notes.sh
  .claude/skills/release/SKILL.md
)
for f in "${required[@]}"; do
  [ -f "$f" ] || fail "missing $f"
done

grep -qi 'latency' README.md || fail "README.md lacks latency modes"
grep -qi 'tray' README.md || fail "README.md lacks tray animation"
grep -qi 'update page\|update surface' README.md || fail "README.md lacks the update surface"
grep -q '延迟' README.zh-CN.md || fail "README.zh-CN.md lacks latency modes"
grep -q '托盘' README.zh-CN.md || fail "README.zh-CN.md lacks tray animation"
grep -q '更新页' README.zh-CN.md || fail "README.zh-CN.md lacks the update page"
grep -Fq 'update window' README.md && fail "README.md says update window; use update page"
grep -Fq '更新窗口' README.zh-CN.md && fail "README.zh-CN.md says 更新窗口; use 更新页"
grep -q 'Docs audit' docs/releasing.md || fail "docs/releasing.md lacks a docs audit"
grep -q 'Latency modes' docs/verification.md || fail "docs/verification.md lacks latency modes"
grep -q 'Tray animation' docs/verification.md || fail "docs/verification.md lacks tray animation"
grep -q '更新 page' docs/verification.md || fail "docs/verification.md lacks the update page"
grep -q '1500' docs/protocol.md || fail "docs/protocol.md lacks the latency mode mapping"
grep -q '2000' docs/protocol.md || fail "docs/protocol.md lacks the latency mode mapping"
grep -Fq 'There is no standalone update window' docs/ui.md || fail "docs/ui.md must state there is no standalone update window"

grep -q 'reviewed-through' docs/ui.md && ok "docs/ui.md has a reviewed-through date" || warn "docs/ui.md has no reviewed-through date"
grep -q 'reviewed-through' docs/implementation.md && ok "docs/implementation.md has a reviewed-through date" || warn "docs/implementation.md has no reviewed-through date"
grep -q 'reviewed-through' docs/verification.md && ok "docs/verification.md has a reviewed-through date" || warn "docs/verification.md has no reviewed-through date"

grep -q '"github"[[:space:]]*:[[:space:]]*"4202217/saundo"' cmd/saundo/mygo.json || fail "mygo.json has no updates.github for 4202217/saundo"
grep -q '"publicKey"[[:space:]]*:' cmd/saundo/mygo.json || fail "mygo.json has no updates.publicKey"
grep -q '"changelog"[[:space:]]*:[[:space:]]*"../../CHANGELOG.md"' cmd/saundo/mygo.json || fail "mygo.json does not use the English CHANGELOG.md"
grep -Fq 'tags: ["v*"]' .github/workflows/release.yml || fail "release.yml is not tag-triggered"
grep -q 'MYGO_UPDATER_PRIVATE_KEY' .github/workflows/release.yml || fail "release.yml does not read MYGO_UPDATER_PRIVATE_KEY"

if command -v gh >/dev/null 2>&1; then
  visibility="$(gh repo view "$REPO" --json visibility --jq .visibility 2>/dev/null || true)"
  if [ "$visibility" = "PUBLIC" ]; then
    ok "repository is public"
  else
    fail "repository is not public ($visibility)"
  fi
  if gh secret list --repo "$REPO" 2>/dev/null | grep -q '^MYGO_UPDATER_PRIVATE_KEY'; then
    ok "update signing secret is set"
  else
    fail "MYGO_UPDATER_PRIVATE_KEY is not set"
  fi
else
  fail "gh is not installed"
fi

code="$(curl -sSL -o /dev/null -w '%{http_code}' "https://github.com/$REPO/releases/latest/download/update-windows-amd64.json" || true)"
if [ "$code" = "200" ]; then
  ok "current update feed returns 200"
else
  fail "current update feed returns $code; the chain is broken before this release"
fi

echo
if [ "$errors" -ne 0 ]; then
  printf '%d preflight check(s) failed\n' "$errors"
  exit 1
fi
printf 'preflight passed for %s\n' "$version"
