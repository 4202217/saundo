---
name: session-lifecycle
description: Run the session lifecycle model whenever a change touches internal/engine/lifecycle.go or the session retry, wanted-set, takeover, or cancel wiring in internal/engine. The model exhaustively checks the machine's decisions and replays the session-lifecycle bugs from the repository's history.
---

# Session lifecycle model

The engine's session decisions — the reconnect ladder, the wanted set, the lost-member set, takeover, cancel, and the user-vs-automatic provenance — live in one machine, `internal/engine/lifecycle.go`. The model in `internal/engine/lifecycle_model_test.go` explores every event sequence the engine can drive that machine through, over its whole reachable state space, and asserts the machine's promises in every reachable state.

## When to run it

Run the check whenever a change touches:

- `internal/engine/lifecycle.go` (the machine itself), or
- any engine call site of `life.` — the callbacks in `session_lifecycle.go` and `cohort_leave.go`, the connect paths in `cohort.go`, `autoconnect.go`, and `receivers_cmd.go`, `planReconnect`, `armReconnect`, `attemptReconnect`, and `reconnectShouldRetry` in `engine.go`.

## How to run

```bash
go test ./internal/engine -run TestLifecycleModel -v
```

Two tests run:

- `TestLifecycleModelExploration` — applies every event to every reachable state and fails on the first broken promise, printing it with the event sequence that reaches it. It also asserts the exploration is not vacuous: every event fires, and the exhausted-ladder, takeover-with-wanted-set, and lost-set states are reachable.
- `TestLifecycleModelReplaysPastBugs` — five variants, each overriding the one transition a past fix corrected, must each fail with that fix's promise. If a variant passes, the model no longer guards that decision.

A full run takes about a second. `make check` runs it as part of `go test ./...`.

## What is verified, in plain words

- A retry that comes due while a session is live is pushed out; it never consumes its pending time or launches into the live session (ea1db36).
- A rejected password schedules nothing and consumes no ladder budget (31e1c7e).
- A member that left the session is never wanted again by an automatic path; only the user naming the key again un-loses it (d310b34, 7b36a83).
- A takeover stops automatic retries until the user acts or a launch succeeds (8330270).
- A user cancel stops automatic retries; a fresh attempt clears the stale attempt state, the counts, and the takeover and cancel flags.
- The ladder gives up only at its budget and never steps past it; a blocked gate changes nothing.

## Real bug or modeling error

The failure prints the broken promise and its shortest event trace. Read the trace against the engine's call sites:

- If the event sequence can occur in the engine — each event has a real caller for that state — the machine is wrong; fix `lifecycle.go`, not the model. Cite the trace in the commit body.
- If the sequence cannot occur — for example an event the engine feeds only in a state the machine reached another way — the event generation in `stateEvents` or the promise in `post` is too loose; narrow it. Do not weaken a promise to make a failure go away without checking the trace against the fix commits named above.

## Bounds and assumptions

- The wanted-set universe is three keys, `A`, `B`, and `C`; the ladder budget boundary at `maxReconnectAttempts` is exact, higher counts clamp in the visited key.
- The model sees the machine only. The transport, plan, prompts, and errors are outside it; the wiring of the events into the engine is pinned by the harness tests (`auth_failure_test.go`, `member_loss_test.go`, `reconnect_sync_test.go`, `engine_test.go`).
- `setWanted` events mirror what the engine feeds it: subsets of the wanted set, a user add appending a key, or a one-key start. `freshAttempt` events mirror a user-named key set. `dropLost` mirrors the `sessionLost` guard. If the engine gains a new way to call one of these, add the matching event shape here in the same change.

## Keeping it in step

Coverage record: `lifecycle.go` as a whole — every exported and internal method, every field — and the event wiring named under "When to run it". Sync commit: `2231f71`. When the covered code changes later, confirm the model still describes it, then move the sync commit forward in the same commit that changes the code. Use `/maintain-formal-verification-skill` after refactors that did not recheck it.
