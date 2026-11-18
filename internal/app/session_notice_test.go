package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/sender"
	"saundo/internal/engine"
)

// lostNotice is the engine's member-lost event for the fixture.
func lostNotice() engine.SessionNotice {
	return engine.SessionNotice{
		Kind: engine.NoticeMemberLost, DeviceID: "AA:BB:CC:DD:EE:02",
		Name: "Apple TV", Remaining: 1,
	}
}

// TestSessionNoticeShowsLostMember pins the notice bar: it names the member
// that left, says the others continue, and offers retry and dismiss.
func TestSessionNoticeShowsLostMember(t *testing.T) {
	a := previewApp()
	a.snap.Notice = lostNotice()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	want := locales.FlyoutMemberLostNotice("Apple TV")
	for _, text := range []string{want, locales.ActionRetry(), locales.FlyoutNoticeDismiss()} {
		if !tt.HasText(text) {
			t.Fatalf("notice is missing %q; texts: %q", text, tt.Texts())
		}
	}
	if err := tt.Click(locales.FlyoutNoticeDismiss()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText(want) {
		t.Fatalf("the dismissed notice is still drawn; texts: %q", tt.Texts())
	}
}

// TestSessionNoticeJoinCopy pins the join variant: it warns that the other
// devices pause briefly, and it closes for this attempt.
func TestSessionNoticeJoinCopy(t *testing.T) {
	a := multiSessionApp()
	a.snap.State = sender.StateHandshake
	setTargetState(a, "AA:BB:CC:DD:EE:02", engine.TargetJoining)
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	want := locales.FlyoutJoinNotice("Apple TV")
	if !tt.HasText(want) {
		t.Fatalf("join notice is missing %q; texts: %q", want, tt.Texts())
	}
	if err := tt.Click(locales.FlyoutNoticeDismiss()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText(want) {
		t.Fatalf("the closed join notice is still drawn; texts: %q", tt.Texts())
	}
}

// TestSessionNoticeAddsItsHeight pins the window growth: the bar and its body
// gap appear while a notice shows and leave when it is dismissed.
func TestSessionNoticeAddsItsHeight(t *testing.T) {
	a := previewApp()
	plain := visibleFlyoutHeight(a)
	a.snap.Notice = lostNotice()
	if got, want := visibleFlyoutHeight(a), plain+sessionNoticeHeight; got != want {
		t.Fatalf("notice height = %d, want %d", got, want)
	}
}
