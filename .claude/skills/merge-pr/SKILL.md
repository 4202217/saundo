---
name: merge-pr
description: Merge a saundo pull request with its gates intact. Use when asked to merge, land, or close out a PR, and before any merge that touches the sender, engine, capture, update, or credentials paths.
---

# Merge a pull request

Read `AGENTS.md` first: the commit body is the durable record, and a merge is
an outward-facing action. Merge only when the user asked for it, and never
around a red check.

## Stop conditions

- CI is red or still running.
- A commit body lacks the `AGENTS.md` fields (`Checks:`, `Pinned-by:`,
  `Finding:`, `Docs:`), or a bug fix names no failing test.
- The critical-path review below has an unresolved P0 or Standards finding.
- The working tree is dirty, or the PR's base is not `main`.
- The PR changes behavior a user can see and carries no evidence in
  `docs/verification.md` or the verify skill.

## 1. Read the PR

```bash
gh pr view <n> --json number,title,body,files,commits,baseRefName,mergeable,mergeStateStatus,statusCheckRollup
gh pr view <n> --json commits --jq '.commits[] | .messageHeadline'
```

Check that every commit body ends with the four fields, that the branch is
up to date with `main`, and that the diff matches the stated goal. A PR that
mixes unrelated changes is split before it merges.

## 2. Gate

```bash
gh pr checks <n> --watch
```

The required checks are `check`, `check-macos`, and `platforms`. A red check
is read, never bypassed. When CI is unavailable, or the change touches a
platform seam (see `.claude/skills/multiplatform/SKILL.md`), run the gate on
the PR head in a throwaway worktree:

```bash
git fetch origin pull/<n>/head:pr-<n>
git worktree add --detach ../saundo-pr-<n> pr-<n>
cd ../saundo-pr-<n> && mingw32-make check
```

Remove the worktree and branch after the merge.

## 3. Critical-path code review

The release process reads these paths diff by diff and runs `/code-review`
on the subset. A merge does the same before landing:

```bash
git fetch origin pull/<n>/head:pr-<n>
git diff --name-only "origin/main...pr-<n>" -- internal/airplay internal/engine internal/capture 'internal/app/update*.go' internal/app/migrate.go
```

When the list is non-empty, run `/code-review` on `origin/main...pr-<n>`
(the same critical-path set as `.claude/skills/release/SKILL.md`). Fix every
P0 and every unresolved Standards finding in the PR branch, push, and rerun
the checks and the local gate. Record the outcome as a PR comment:

```
Code review: subset ran (<N> commits) -> no P0
Code review: subset ran (<N> commits) -> fixed in <commit>
```

A PR with no critical-path file records `Code review: not required` and
proceeds. Do not merge with a review finding open; `scripts/release-check.sh`
enforces the same rule over the release range.

## 4. Merge

The repository keeps its merge commits, so a PR lands with `--merge`, never
squash or rebase; the commit bodies are the record the next agent reads.

```bash
gh pr merge <n> --merge
```

Delete the remote branch only when the user asks or the repository's branch
list is already tidy; the merge commit keeps the history either way.

## 5. After the merge

```bash
git checkout main
git pull --ff-only
git log --oneline -n 5
```

Confirm the merge commit is on `main`, the tree is clean, and the branch (if
kept) is gone from the PR list. Do not tag: releases come from the `release`
skill, whose range review classifies the merged commits.

If the PR is user-visible, note it for the next release's range review; the
changelogs are written at release time, not in the PR.

## Report

Name the PR, the checks and what they printed, the critical-path review
outcome (subset, commit count, findings), the merge commit, and anything left
open. State what did not run.
