package engine

import (
	"testing"

	"saundo/internal/airplay/sender"
)

func twoTargetCohort(t *testing.T) *cohortTransport {
	t.Helper()
	c := buildTestCohort(2)
	c.plan.targets = []plannedTarget{
		{key: "a", first: 0, count: 1},
		{key: "b", first: 1, count: 1},
	}
	c.members[0].targetKey = "a"
	c.members[1].targetKey = "b"
	return c
}

// TestLaunchFailureKeepsStreamingTargets pins the multi-target policy: when
// one independent target fails its launch after another already streams, the
// session drops the failed target and keeps the survivor instead of tearing
// the whole cohort down.
func TestLaunchFailureKeepsStreamingTargets(t *testing.T) {
	c := twoTargetCohort(t)
	c.memberStateChanged(1, sender.StateStreaming)

	c.memberLaunched(0, false, "receiver refused")

	if got := c.LostTarget(); got != "a" {
		t.Fatalf("lost target = %q, want the failed target", got)
	}
	if got := c.Failure().TargetKey; got != "a" {
		t.Fatalf("failure target = %q, want the failed target", got)
	}
	if c.Failure().Auth {
		t.Fatal("a plain launch failure was marked as an auth failure")
	}
}

// TestPairLaunchFailureDoesNotSplitThePair pins the stereo rule: a pair is one
// target, so one failed member retries the pair whole; it must not try to keep
// half a pair.
func TestPairLaunchFailureDoesNotSplitThePair(t *testing.T) {
	c := buildTestCohort(2)
	c.plan.targets = []plannedTarget{{key: "pair", first: 0, count: 2}}
	c.members[0].targetKey = "pair"
	c.members[1].targetKey = "pair"
	c.memberStateChanged(1, sender.StateStreaming)

	c.memberLaunched(0, false, "receiver refused")

	if got := c.LostTarget(); got != "" {
		t.Fatalf("lost target = %q, want a whole-pair retry", got)
	}
}

// TestLaunchFailureWithoutASurvivorRetriesTheSet pins the other side of the
// policy: no target has streamed yet, so the failure is the set's, not one
// member's, and the whole cohort retries.
func TestLaunchFailureWithoutASurvivorRetriesTheSet(t *testing.T) {
	c := twoTargetCohort(t)
	c.memberLaunched(0, false, "receiver refused")

	if got := c.LostTarget(); got != "" {
		t.Fatalf("lost target = %q, want a whole-set retry", got)
	}
	if got := c.Failure().TargetKey; got != "a" {
		t.Fatalf("failure target = %q, want the failed target for attribution", got)
	}
}
