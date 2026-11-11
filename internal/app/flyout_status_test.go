package app

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/sender"
)

func TestFlyoutStatusIsLiveRegion(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	want := locales.StatusStreamingTo("HomePods")
	info, ok := tt.AccessByRole(ui.RoleStatus, "")
	if !ok {
		t.Fatalf("flyout has no RoleStatus; access: %+v", tt.AccessAll())
	}
	if info.Name != want {
		t.Fatalf("status name = %q, want %q", info.Name, want)
	}
}

func TestFlyoutStatusAnnouncesChange(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	tt.UseScreenReader()
	a.snap.State = sender.StateConnecting
	tt.Frame()
	want := locales.StatusConnecting()
	if got := tt.Announcements(); !slices.Contains(got, want) {
		t.Fatalf("announcements = %q, want %q", got, want)
	}
}
