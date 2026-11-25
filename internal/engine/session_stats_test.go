package engine

import (
	"testing"

	"saundo/internal/airplay/sender"
	"saundo/internal/receivers"
)

// TestUserDisconnectRemembersTheSessionStats pins the close path that used to
// skip the write: a user disconnect must store the session's counters, so the
// next connect's DuplicatesFor picks one copy instead of the first-session
// extra copy.
func TestUserDisconnectRemembersTheSessionStats(t *testing.T) {
	h := newHarness(t)
	h.addDeviceOnline()
	h.connect()
	h.launch()
	h.fake.stats = []sender.Stats{{SentBlocks: 1200, Retransmits: 0}}

	h.e.handleCommand(command{kind: cmdDisconnect})

	entry, ok := h.e.reg.Get(testDeviceID)
	if !ok {
		t.Fatal("the receiver entry disappeared")
	}
	if entry.LastSentBlocks != 1200 || entry.LastRetransmits != 0 {
		t.Fatalf("stored stats = %d/%d, want 1200/0", entry.LastSentBlocks, entry.LastRetransmits)
	}
	if got := receivers.DuplicatesFor(entry); got != 0 {
		t.Fatalf("next duplicates = %d, want none for a clean session", got)
	}
}

// TestComboCloseRemembersTheSessionStats pins the synthesized target: a
// combo's folded health lands in the combo entry, so its next session does
// not read a zero entry and pay the first-session extra copy forever.
func TestComboCloseRemembersTheSessionStats(t *testing.T) {
	h, key := linkCombo(t)
	h.e.handleCommand(command{kind: cmdConnect, deviceID: key})
	h.launch()
	h.fake.stats = []sender.Stats{
		{SentBlocks: 600, Retransmits: 0},
		{SentBlocks: 700, Retransmits: 0},
	}

	h.e.handleCommand(command{kind: cmdDisconnect})

	entry, ok := h.e.reg.Get(key)
	if !ok {
		t.Fatal("the combo entry disappeared")
	}
	if entry.LastSentBlocks != 1300 || entry.LastRetransmits != 0 {
		t.Fatalf("stored stats = %d/%d, want 1300/0", entry.LastSentBlocks, entry.LastRetransmits)
	}
	if got := receivers.DuplicatesFor(entry); got != 0 {
		t.Fatalf("next duplicates = %d, want none for a clean session", got)
	}
	left, ok := h.e.reg.Get(testDeviceID)
	if !ok || left.LastSentBlocks != 600 || left.LastRetransmits != 0 {
		t.Fatalf("left member stats = %+v ok=%v", left, ok)
	}
	right, ok := h.e.reg.Get(comboSecondDevice().ID)
	if !ok || right.LastSentBlocks != 700 || right.LastRetransmits != 0 {
		t.Fatalf("right member stats = %+v ok=%v", right, ok)
	}
}

// TestComboPlanSizesCopiesPerMember pins the duplicate choice: a combo's
// members use their own loss history, so a clean member sends one copy while
// only a lossy member pays the extra copy.
func TestComboPlanSizesCopiesPerMember(t *testing.T) {
	h, key := linkCombo(t)
	clean, _ := h.e.reg.Get(testDeviceID)
	clean.LastSentBlocks, clean.LastRetransmits = 1000, 0
	h.e.reg.Put(clean)
	lossy, _ := h.e.reg.Get(comboSecondDevice().ID)
	lossy.LastSentBlocks, lossy.LastRetransmits = 1000, 50
	h.e.reg.Put(lossy)

	plan, err := h.e.buildPlan([]string{key})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.members) != 2 {
		t.Fatalf("members = %d, want 2", len(plan.members))
	}
	if got := plan.members[0].duplicates; got != 0 {
		t.Fatalf("clean member duplicates = %d, want 0", got)
	}
	if got := plan.members[1].duplicates; got != 2 {
		t.Fatalf("lossy member duplicates = %d, want 2", got)
	}
}
