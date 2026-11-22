package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestHiddenGroupTriggerMatchesPill pins the hidden group's click target to
// the size of its hover pill: clicking to the right of the pill, on the
// otherwise flat part of the row, must not toggle the group.
func TestHiddenGroupTriggerMatchesPill(t *testing.T) {
	newPage := func() (*App, *ui.Tester, ui.Rect) {
		a := receiverFixture()
		a.page = pageReceivers
		tt := ui.NewTester(a.settingsView, 720, 560)
		tt.Frame()
		r, ok := tt.Find("已隐藏")
		if !ok {
			t.Fatal("the hidden group head is missing")
		}
		return a, tt, r
	}

	a, tt, r := newPage()
	// The pill is the chevron, label, and count, not the full 490-DIP row.
	if r.W > 180 {
		t.Fatalf("the trigger spans %.1f DIP, wider than the pill", r.W)
	}
	// Inside the pill expands; to its right, on the flat row, does not.
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	if !a.receiverHiddenOpen {
		t.Fatal("clicking the pill did not expand the hidden group")
	}

	a2, tt2, r2 := newPage()
	tt2.ClickAt(r2.X+r2.W+40, r2.Y+r2.H/2)
	if a2.receiverHiddenOpen {
		t.Fatal("clicking the flat row beside the pill expanded the hidden group")
	}
}
