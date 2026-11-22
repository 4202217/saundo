package engine

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
)

// The lifecycle model exhaustively explores every event sequence the engine
// can drive the session machine through, over the machine's whole reachable
// state space, and asserts the decisions the past fixes landed on hold in
// every reachable state. A change to lifecycle.go that reintroduces one of
// those bugs fails here without any new test being written.
//
// Scope: the machine in lifecycle.go and its event surface only. The engine's
// transport, plan, prompts, and errors sit outside the model; the engine's
// wiring of the events is pinned by the harness tests (auth_failure_test,
// member_loss_test, reconnect_sync_test). Counts clamp in the visited key —
// the ladder budget boundary at maxReconnectAttempts stays exact — so the
// exploration closes on a finite space.

// modelKeys bounds the wanted set the model reasons about.
var modelKeys = []string{"A", "B", "C"}

// modelNow is the fixed clock every model event runs at.
var modelNow = time.Unix(0, 0)

// lifeState is the hashable shape of a machine state.
type lifeState struct {
	wanted  string
	lost    string
	armed   bool // reconnectWanted
	taken   bool
	cancel  bool // userDisconnect
	auto    bool // connectAuto
	num     int
	count   int
	pending bool // reconnectAt non-zero
	sinceTo bool // explorer flag: a takeover happened and no user attempt or launch followed
}

func clampInt(v, at int) int {
	if v > at {
		return at
	}
	return v
}

func (l *lifecycle) state(sinceTo bool) lifeState {
	w := make([]string, len(l.wantedKeys))
	copy(w, l.wantedKeys)
	lostKeys := make([]string, 0, len(l.lost))
	for k := range l.lost {
		lostKeys = append(lostKeys, k)
	}
	sort.Strings(lostKeys)
	return lifeState{
		wanted:  strings.Join(w, ","),
		lost:    strings.Join(lostKeys, ","),
		armed:   l.reconnectWanted,
		taken:   l.takenOver,
		cancel:  l.userDisconnect,
		auto:    l.connectAuto,
		num:     clampInt(l.reconnectNum, maxReconnectAttempts),
		count:   clampInt(l.reconnectCount, 3),
		pending: !l.reconnectAt.IsZero(),
		sinceTo: sinceTo,
	}
}

func (l *lifecycle) clone() lifecycle {
	out := *l
	out.wantedKeys = make([]string, len(l.wantedKeys))
	copy(out.wantedKeys, l.wantedKeys)
	if l.lost != nil {
		out.lost = make(map[string]bool, len(l.lost))
		for k, v := range l.lost {
			out.lost[k] = v
		}
	}
	return out
}

// modelMachine is the event surface the engine drives on the machine. The
// real adapter wraps lifecycle unchanged; each bug variant overrides the one
// transition its past fix corrected.
type modelMachine interface {
	freshAttempt(keys ...string)
	userRebuild()
	userCancel()
	clearWanted()
	setWanted(keys []string)
	dropLost(key string) []string
	launchSucceeded()
	passwordRejected()
	preempted()
	memberLostRebuilt()
	attemptStarted()
	retryDue(active bool, now time.Time) retryDecision
	armReconnect(now time.Time)
	mayAutoRetry(stopping, enabled, hasSelection bool) bool
	live() *lifecycle
	// with returns an adapter of the same variant wrapping l, so every
	// explored state keeps the variant's behavior.
	with(l *lifecycle) modelMachine
}

type realMachine struct {
	*lifecycle
}

func newRealMachine() realMachine {
	l := lifecycle{}
	return realMachine{&l}
}

func (m realMachine) with(l *lifecycle) modelMachine { return realMachine{l} }

// armReconnect mirrors the engine's armReconnect: the retry gate and a
// not-already-pending retry, then the ladder step.
func (m realMachine) armReconnect(now time.Time) {
	if m.mayAutoRetry(false, true, true) && m.reconnectAt.IsZero() {
		m.planRetry(now)
	}
}

func (m realMachine) live() *lifecycle { return m.lifecycle }

// modelEvent is one event the engine can drive. post names the promise the
// event makes about the machine's observable state; it runs on every machine,
// so a variant that breaks the promise fails the exploration.
type modelEvent struct {
	name  string
	apply func(m modelMachine, want modelWant)
	post  func(before, after *lifecycle, want modelWant) string
}

// modelWant carries the event's parameters and the explorer's takeover flag.
type modelWant struct {
	key     string
	sinceTo bool
}

// orderedSubsets enumerates every ordered subset of keys, empty included.
func orderedSubsets(keys []string) [][]string {
	if len(keys) == 0 {
		return [][]string{nil}
	}
	rest := orderedSubsets(keys[1:])
	out := make([][]string, 0, 2*len(rest))
	for _, s := range rest {
		out = append(out, s)
		with := make([]string, 0, len(s)+1)
		with = append(with, keys[0])
		with = append(with, s...)
		out = append(out, with)
	}
	return out
}

func modelEvents() []modelEvent {
	events := []modelEvent{
		{
			name:  "userRebuild",
			apply: func(m modelMachine, _ modelWant) { m.userRebuild() },
			post: func(before, after *lifecycle, _ modelWant) string {
				if after.userDisconnect || after.connectAuto {
					return "a user rebuild left the cancel or automatic state set"
				}
				if after.reconnectCount != before.reconnectCount {
					return "a user rebuild touched the retry count"
				}
				return ""
			},
		},
		{
			name:  "userCancel",
			apply: func(m modelMachine, _ modelWant) { m.userCancel() },
			post: func(_, after *lifecycle, _ modelWant) string {
				if !after.userDisconnect || after.reconnectCount != 0 {
					return "a user cancel did not record itself"
				}
				return ""
			},
		},
		{
			name:  "clearWanted",
			apply: func(m modelMachine, _ modelWant) { m.clearWanted() },
			post: func(_, after *lifecycle, _ modelWant) string {
				if len(after.wantedKeys) != 0 {
					return "clearWanted kept keys"
				}
				return ""
			},
		},
		{
			name:  "launchSucceeded",
			apply: func(m modelMachine, _ modelWant) { m.launchSucceeded() },
			post: func(_, after *lifecycle, _ modelWant) string {
				if !after.reconnectWanted || !after.reconnectAt.IsZero() || after.reconnectNum != 0 ||
					after.takenOver || after.connectAuto {
					return "a successful launch did not restart the session's retry state"
				}
				return ""
			},
		},
		{
			name:  "passwordRejected",
			apply: func(m modelMachine, _ modelWant) { m.passwordRejected() },
			post: func(before, after *lifecycle, _ modelWant) string {
				if !after.reconnectAt.IsZero() {
					return "a rejected password scheduled a retry"
				}
				if after.reconnectNum != before.reconnectNum {
					return "a rejected password consumed ladder budget"
				}
				return ""
			},
		},
		{
			name:  "preempted",
			apply: func(m modelMachine, _ modelWant) { m.preempted() },
			post: func(_, after *lifecycle, _ modelWant) string {
				if !after.takenOver || after.reconnectWanted || !after.reconnectAt.IsZero() {
					return "a takeover did not stop automatic retries"
				}
				return ""
			},
		},
		{
			name:  "memberLostRebuilt",
			apply: func(m modelMachine, _ modelWant) { m.memberLostRebuilt() },
			post: func(_, after *lifecycle, _ modelWant) string {
				if !after.reconnectAt.IsZero() || after.reconnectNum != 0 || !after.connectAuto {
					return "a survivor rebuild did not reset the retry it must not fire"
				}
				return ""
			},
		},
		{
			name:  "attemptStarted",
			apply: func(m modelMachine, _ modelWant) { m.attemptStarted() },
			post: func(before, after *lifecycle, _ modelWant) string {
				if after.reconnectCount != before.reconnectCount+1 {
					return "an automatic attempt did not count itself once"
				}
				return ""
			},
		},
		{
			name:  "retryDueActive",
			apply: func(m modelMachine, _ modelWant) { m.retryDue(true, modelNow) },
			post: func(_, after *lifecycle, _ modelWant) string {
				if after.reconnectAt.IsZero() {
					return "a retry due on a live session did not push out"
				}
				return ""
			},
		},
		{
			name:  "retryDueIdle",
			apply: func(m modelMachine, _ modelWant) { m.retryDue(false, modelNow) },
			post: func(_, after *lifecycle, _ modelWant) string {
				if !after.reconnectAt.IsZero() {
					return "a due retry did not consume its pending time"
				}
				return ""
			},
		},
		{
			name:  "armReconnect",
			apply: func(m modelMachine, _ modelWant) { m.armReconnect(modelNow) },
			post: func(before, after *lifecycle, want modelWant) string {
				scheduled := after.reconnectNum == before.reconnectNum+1
				if want.sinceTo && scheduled {
					return "an automatic retry was scheduled after a takeover"
				}
				eligible := before.mayAutoRetry(false, true, true) && before.reconnectAt.IsZero()
				if !eligible {
					if !after.equal(before) {
						return "a blocked gate still changed the ladder"
					}
					return ""
				}
				if scheduled {
					if after.reconnectNum > maxReconnectAttempts {
						return "the ladder stepped past its budget"
					}
					if !after.reconnectWanted || after.reconnectAt.IsZero() {
						return "a scheduled retry is not armed"
					}
					return ""
				}
				if after.reconnectNum < maxReconnectAttempts {
					return "the ladder gave up below its budget"
				}
				if after.reconnectWanted || !after.reconnectAt.IsZero() {
					return "a given-up ladder is still armed"
				}
				return ""
			},
		},
	}
	return events
}

// stateEvents are the events whose parameters depend on the state the engine
// can feed them. setWanted never names a lost key on its own: the automatic
// rebuild paths feed it subsets of the wanted set, and a user add names a
// key through freshAttempt first, which un-loses it.
func stateEvents(wanted []string, lost map[string]bool) []modelEvent {
	var out []modelEvent
	// A user connect names one key; the auto-connect launch names any
	// non-empty set of flagged keys.
	for mask := 1; mask < 1<<len(modelKeys); mask++ {
		ks := make([]string, 0, len(modelKeys))
		for i, k := range modelKeys {
			if mask&(1<<i) != 0 {
				ks = append(ks, k)
			}
		}
		out = append(out, modelEvent{
			name:  "freshAttempt[" + strings.Join(ks, ",") + "]",
			apply: func(m modelMachine, _ modelWant) { m.freshAttempt(ks...) },
			post: func(_, after *lifecycle, _ modelWant) string {
				if after.userDisconnect || after.takenOver || after.reconnectCount != 0 ||
					after.reconnectNum != 0 || after.connectAuto {
					return "a fresh attempt did not clear the stale attempt state"
				}
				for _, k := range ks {
					if after.lost[k] {
						return "the user's own key stayed lost"
					}
				}
				return ""
			},
		})
	}
	// Rebuilds and prunes feed order-preserving subsets of the wanted set;
	// a user add appends a discovered key that is not lost; a start opens a
	// one-key set.
	sets := orderedSubsets(wanted) // includes the empty set; skip it here
	for _, s := range sets {
		if len(s) == 0 {
			continue
		}
		k := s
		out = append(out, setWantedEvent(k))
	}
	for _, k := range modelKeys {
		if lost[k] || hadKey(wanted, k) {
			continue
		}
		out = append(out, setWantedEvent(append(append([]string(nil), wanted...), k)))
		out = append(out, setWantedEvent([]string{k}))
	}
	// A member loss drops a key the session holds, and only when others
	// remain — the engine's sessionLost guard.
	if len(wanted) > 1 {
		for _, key := range wanted {
			k := key
			out = append(out, modelEvent{
				name:  "dropLost[" + k + "]",
				apply: func(m modelMachine, _ modelWant) { m.dropLost(k) },
				post: func(before, after *lifecycle, _ modelWant) string {
					if after.hadKey(k) {
						return "the lost key stayed wanted"
					}
					if !before.hadKey(k) {
						return "dropLost dropped a key the session did not hold"
					}
					return ""
				},
			})
		}
	}
	return out
}

func setWantedEvent(keys []string) modelEvent {
	k := keys
	return modelEvent{
		name:  "setWanted[" + strings.Join(k, ",") + "]",
		apply: func(m modelMachine, _ modelWant) { m.setWanted(append([]string(nil), k...)) },
		post: func(_, after *lifecycle, _ modelWant) string {
			if strings.Join(after.wantedKeys, ",") != strings.Join(k, ",") {
				return "setWanted did not set the requested set"
			}
			return ""
		},
	}
}

func hadKey(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

func (l *lifecycle) hadKey(key string) bool {
	return hadKey(l.wantedKeys, key)
}

// equal compares the machine's observable state. The explorer uses it to
// require that a blocked event changes nothing.
func (l *lifecycle) equal(o *lifecycle) bool {
	return l.state(false) == o.state(false)
}

// modelStats records that the exploration is not vacuous.
type modelStats struct {
	fired          map[string]int
	sawGiveUp      bool
	sawTakenWanted bool
	sawLost        bool
}

// modelViolation is one broken promise, with the event sequence that reaches it.
type modelViolation struct {
	kind  string
	trace string
}

// exploreLifecycle runs every event on every reachable state. It returns the
// broken promises with their shortest traces.
func exploreLifecycle(start modelMachine) ([]modelViolation, modelStats) {
	stats := modelStats{fired: map[string]int{}}
	root := start.live().clone()
	rootKey := root.state(false)
	type node struct {
		m       lifecycle
		sinceTo bool
	}
	parent := map[lifeState]lifeState{rootKey: {}}
	parentEvent := map[lifeState]string{rootKey: ""}
	seen := map[lifeState]bool{rootKey: true}
	var violations []modelViolation

	queue := []node{{m: root}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		curKey := cur.m.state(cur.sinceTo)
		events := append(modelEvents(), stateEvents(cur.m.wantedKeys, cur.m.lost)...)
		for _, ev := range events {
			before := cur.m.clone()
			after := cur.m.clone()
			ev.apply(start.with(&after), modelWant{sinceTo: cur.sinceTo})
			stats.fired[ev.name]++
			want := modelWant{sinceTo: cur.sinceTo}
			kind := ev.post(&before, &after, want)
			if kind != "" {
				kind = ev.name + ": " + kind
			}
			if kind == "" {
				kind = checkLostNotWanted(&after)
			}
			if kind == "" {
				kind = checkGate(&after)
			}
			if kind == "" && after.reconnectNum >= maxReconnectAttempts && !after.reconnectWanted {
				stats.sawGiveUp = true
			}
			if kind == "" && after.takenOver && len(after.wantedKeys) > 0 {
				stats.sawTakenWanted = true
			}
			if kind == "" && len(after.lost) > 0 {
				stats.sawLost = true
			}
			sinceTo := (cur.sinceTo || ev.name == "preempted") &&
				!strings.HasPrefix(ev.name, "freshAttempt") &&
				ev.name != "launchSucceeded" && ev.name != "userCancel"
			afterState := after.state(sinceTo)
			if kind != "" {
				violations = append(violations, modelViolation{
					kind:  kind,
					trace: traceTo(parent, parentEvent, rootKey, curKey) + " → " + ev.name,
				})
				continue
			}
			if !seen[afterState] {
				seen[afterState] = true
				parent[afterState] = curKey
				parentEvent[afterState] = ev.name
				queue = append(queue, node{m: after, sinceTo: sinceTo})
			}
		}
	}
	return violations, stats
}

// traceTo reconstructs the event sequence that reaches curKey.
func traceTo(parent map[lifeState]lifeState, parentEvent map[lifeState]string, root, cur lifeState) string {
	var parts []string
	for cur != root {
		parts = append([]string{parentEvent[cur]}, parts...)
		next, ok := parent[cur]
		if !ok || next == cur {
			break
		}
		cur = next
	}
	return strings.Join(parts, " → ")
}

func checkLostNotWanted(l *lifecycle) string {
	for _, k := range l.wantedKeys {
		if l.lost[k] {
			return fmt.Sprintf("the lost key %s is wanted again without user intent", k)
		}
	}
	return ""
}

func checkGate(l *lifecycle) string {
	if l.takenOver && l.mayAutoRetry(false, true, true) {
		return "a taken-over receiver may still be retried automatically"
	}
	if l.userDisconnect && l.mayAutoRetry(true, false, false) {
		return "a user-cancelled session may still be retried automatically"
	}
	return ""
}

// TestLifecycleModelExploration asserts every promise on the real machine and
// that the exploration is not vacuous: every event fires, and the states the
// past bugs lived in are reachable.
func TestLifecycleModelExploration(t *testing.T) {
	violations, stats := exploreLifecycle(newRealMachine())
	if len(violations) > 0 {
		var b strings.Builder
		for _, v := range violations {
			fmt.Fprintf(&b, "%s\n  trace: %s\n", v.kind, v.trace)
		}
		t.Fatalf("the lifecycle machine broke %d promises:\n%s", len(violations), b.String())
	}
	for _, ev := range modelEvents() {
		if stats.fired[ev.name] == 0 {
			t.Fatalf("the model never fired %s; its promise is vacuous", ev.name)
		}
	}
	for _, prefix := range []string{"freshAttempt[", "setWanted[", "dropLost["} {
		fired := false
		for name, n := range stats.fired {
			if strings.HasPrefix(name, prefix) && n > 0 {
				fired = true
			}
		}
		if !fired {
			t.Fatalf("the model never fired a %s event; its promise is vacuous", prefix)
		}
	}
	if !stats.sawGiveUp {
		t.Fatal("the model never reached the exhausted ladder; give-up is unverified")
	}
	if !stats.sawTakenWanted {
		t.Fatal("the model never reached a takeover with a wanted set")
	}
	if !stats.sawLost {
		t.Fatal("the model never reached a non-empty lost set")
	}
}

// The variants below replay the session-lifecycle bugs from the history.
// Each overrides the one transition its fix corrected and must fail the
// exploration with the predicted promise. If a variant stops failing, the
// model no longer guards that decision.
//
//	replayStaleRetry     ea1db36: the stale retry killed the live session
//	replayPasswordLadder 31e1c7e: the rejected password burned the ladder
//	replayLostKeyKept    d310b34: the failed rebuild kept the lost key
//	replayFullSetRebuilt 7b36a83: the automatic retry rebuilt the dead member
//	replayTakeoverRetry  8330270: the takeover scheduled a reconnect

type replayStaleRetry struct{ realMachine }

func (m replayStaleRetry) with(l *lifecycle) modelMachine {
	return replayStaleRetry{realMachine{l}}
}

func (m replayStaleRetry) retryDue(active bool, now time.Time) retryDecision {
	return m.lifecycle.retryDue(false, now)
}

type replayPasswordLadder struct{ realMachine }

func (m replayPasswordLadder) with(l *lifecycle) modelMachine {
	return replayPasswordLadder{realMachine{l}}
}

func (m replayPasswordLadder) passwordRejected() {
	m.lifecycle.passwordRejected()
	m.planRetry(modelNow)
}

type replayLostKeyKept struct{ realMachine }

func (m replayLostKeyKept) with(l *lifecycle) modelMachine {
	return replayLostKeyKept{realMachine{l}}
}

func (m replayLostKeyKept) dropLost(key string) []string {
	if m.lost == nil {
		m.lost = make(map[string]bool)
	}
	m.lost[key] = true
	out := make([]string, len(m.wantedKeys))
	copy(out, m.wantedKeys)
	return out
}

type replayFullSetRebuilt struct{ realMachine }

func (m replayFullSetRebuilt) with(l *lifecycle) modelMachine {
	return replayFullSetRebuilt{realMachine{l}}
}

func (m replayFullSetRebuilt) memberLostRebuilt() {
	m.lifecycle.memberLostRebuilt()
	m.setWanted(append([]string(nil), modelKeys...))
}

type replayTakeoverRetry struct{ realMachine }

func (m replayTakeoverRetry) with(l *lifecycle) modelMachine {
	return replayTakeoverRetry{realMachine{l}}
}

func (m replayTakeoverRetry) preempted() {
	m.lifecycle.cancelRetry()
}

func TestLifecycleModelReplaysPastBugs(t *testing.T) {
	cases := []struct {
		name     string
		machine  modelMachine
		contains string
	}{
		{"ea1db36 stale retry kills the live session", replayStaleRetry{newRealMachine()}, "retryDueActive"},
		{"31e1c7e rejected password burns the ladder", replayPasswordLadder{newRealMachine()}, "passwordRejected"},
		{"d310b34 failed rebuild keeps the lost key", replayLostKeyKept{newRealMachine()}, "the lost key"},
		{"7b36a83 retry rebuilds the full wanted set", replayFullSetRebuilt{newRealMachine()}, "the lost key"},
		{"8330270 takeover schedules a reconnect", replayTakeoverRetry{newRealMachine()}, "takeover"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			violations, _ := exploreLifecycle(tc.machine)
			if len(violations) == 0 {
				t.Fatal("the bug variant passed the model; the model no longer replays this bug")
			}
			for _, v := range violations {
				if !strings.Contains(v.kind, tc.contains) {
					t.Fatalf("variant failed with an unexpected promise: %s\n  trace: %s", v.kind, v.trace)
				}
			}
		})
	}
}
