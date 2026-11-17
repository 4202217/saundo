# UI unification decision log

One row per phase. The plan is in [ui-unification.md](ui-unification.md).

| Date | Phase | Verdict | Evidence | Notes |
|---|---|---|---|---|
| 2026-11-17 | 0 | VERIFIED with a stable subset | `.audit/ui/baseline.json` holds 243 of 254 files | `ui.Tester` starts at `time.Now`, so 11 animated states differ by 7 to 318 pixels between runs. The stable subset is the baseline. The full matrix takes 28 to 31 seconds. |
| 2026-11-17 | 1 | VERIFIED | `check-ui render -out .audit/ui/phase1-verify` wrote 254 files in 31.6s. `check-ui diff` printed 243 files match. `mingw32-make check` passed, including windows/amd64 and darwin/arm64. `check-ui catalog` passed. `go tool nm` found no Catalog, Fixture, or sample symbols in the binary. | 85 scenarios produce 118 stems. The fixture lives in `internal/app/ui_fixture.go` untagged and the linker drops it. Five documented matrix commands in the skills and `docs/ui.md` were changed to `go run ./scripts/check-ui render` because the matrix now runs under the `preview` tag. |
| 2026-11-17 | 1 review | VERIFIED after fixes | The `phase1-review` workflow confirmed four defects and rejected two latent findings. | Fixed the `GOFLAGS` preview leak in `scripts/build.sh` and the Makefile, the hover target and the missing settle advance, and the stale output directory. Added `check-ui rebaseline`; it updated 2 of 243 baseline hashes for the hover fix. The two rejected findings are latent test hygiene, recorded in the workflow output, not defects. |
