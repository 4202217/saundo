package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
)

// TestDeviceHeadKeepsItsHeight pins the reserved action slot: the device head
// measures the same with and without a live session, so the 全部断开 button
// never resizes the panel when it appears.
func TestDeviceHeadKeepsItsHeight(t *testing.T) {
	with := previewApp()
	tt := ui.NewTester(with.view, flyoutWidth, visibleFlyoutHeight(with))
	tt.Frame()
	labelWith, _ := tt.Find(locales.FlyoutDevices())
	cardWith, _ := tt.Find(locales.DeviceListLabel())

	without := previewApp()
	without.snap.ActiveTargets = nil
	without.snap.Targets = nil
	without.snap.State = 0
	tt = ui.NewTester(without.view, flyoutWidth, visibleFlyoutHeight(without))
	tt.Frame()
	labelWithout, _ := tt.Find(locales.FlyoutDevices())
	cardWithout, _ := tt.Find(locales.DeviceListLabel())

	if with.disconnectAllShown() == without.disconnectAllShown() {
		t.Fatal("the fixture does not cover both head states")
	}
	if visibleFlyoutHeight(with) != visibleFlyoutHeight(without) {
		t.Fatalf("flyout height moves with the session: %d vs %d",
			visibleFlyoutHeight(with), visibleFlyoutHeight(without))
	}
	if labelWith.Y != labelWithout.Y || cardWith.Y != cardWithout.Y {
		t.Fatalf("device head moves with the session: label %+v vs %+v, card %+v vs %+v",
			labelWith, labelWithout, cardWith, cardWithout)
	}
}
