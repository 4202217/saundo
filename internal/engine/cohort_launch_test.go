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

// TestPairSiblingStreamingDoesNotCountAsASurvivor pins the pair rule inside a
// mixed cohort: a streaming sibling belongs to the same target, so it is not
// an independent survivor. With no other target streaming, the failure is the
// set's and the whole cohort retries instead of the pair being dropped while
// the single is still in its handshake.
func TestPairSiblingStreamingDoesNotCountAsASurvivor(t *testing.T) {
	c := mixedPairCohort(t)
	c.memberStateChanged(1, sender.StateStreaming)

	c.memberLaunched(0, false, "receiver refused")

	if got := c.LostTarget(); got != "" {
		t.Fatalf("lost target = %q, want a whole-set retry", got)
	}
	if got := c.Failure().TargetKey; got != "pair" {
		t.Fatalf("failure target = %q, want the pair for attribution", got)
	}
}

// TestPairFailureDropsThePairAsOneTarget pins the other half of the same rule:
// when an independent target does stream, the failed pair is dropped as one
// target. It must be the pair key, never a single member.
func TestPairFailureDropsThePairAsOneTarget(t *testing.T) {
	c := mixedPairCohort(t)
	c.memberStateChanged(1, sender.StateStreaming)
	c.memberStateChanged(2, sender.StateStreaming)

	c.memberLaunched(0, false, "receiver refused")

	if got := c.LostTarget(); got != "pair" {
		t.Fatalf("lost target = %q, want the whole pair dropped as one target", got)
	}
}

// mixedPairCohort builds a pair plus an independent single: members 0 and 1
// are the pair, member 2 is the single.
func mixedPairCohort(t *testing.T) *cohortTransport {
	t.Helper()
	c := buildTestCohort(3)
	c.plan.targets = []plannedTarget{
		{key: "pair", first: 0, count: 2},
		{key: "s", first: 2, count: 1},
	}
	c.members[0].targetKey = "pair"
	c.members[1].targetKey = "pair"
	c.members[2].targetKey = "s"
	return c
}

// TestAMemberCloseKeepsTheFirstLostTarget pins the delivery order: when one
// member closes and another member's launch fails before the engine reads the
// dead signal, the close is the first loss and must not be overwritten. The
// engine would otherwise drop the target that failed and rebuild around the
// receiver that just closed.
func TestAMemberCloseKeepsTheFirstLostTarget(t *testing.T) {
	c := twoTargetCohort(t)
	c.memberStateChanged(1, sender.StateStreaming)
	c.memberClosed(1)
	c.memberLaunched(0, false, "receiver refused")

	if got := c.LostTarget(); got != "b" {
		t.Fatalf("lost target = %q, want the member that closed first", got)
	}
}

// TestALaunchFailureKeepsTheFirstLossInEitherOrder pins the mirror order: a
// launch failure recorded first owns the loss, and a later close does not
// replace it.
func TestALaunchFailureKeepsTheFirstLossInEitherOrder(t *testing.T) {
	c := twoTargetCohort(t)
	c.memberStateChanged(1, sender.StateStreaming)
	c.memberLaunched(0, false, "receiver refused")
	c.memberClosed(1)

	if got := c.LostTarget(); got != "a" {
		t.Fatalf("lost target = %q, want the target whose launch failed", got)
	}
}
