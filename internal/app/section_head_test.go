package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/engine"
)

// TestGroupHeadsShareOneBaseline pins the settings pages' section labels to
// one baseline: 已连接设备 heads a card with no action while 单独接收器 heads
// one with the 添加 action, and an action that stretched the head row would
// push that label further from the page divider above it. The two labels
// render in two tester apps, and a runner's font metrics round sub-DIP
// differently from the developer's, so the pin allows half a DIP; a real
// baseline break moves the label by whole DIPs.
func TestGroupHeadsShareOneBaseline(t *testing.T) {
	conn := multiSessionApp()
	conn.page = pageConnection
	tt := ui.NewTester(conn.settingsView, 720, 500)
	connHead, ok := tt.Find(locales.SettingsConnectionDevices())
	if !ok {
		t.Fatalf("the 已连接设备 head is missing; texts: %q", tt.Texts())
	}

	recv := receiverFixture()
	recv.page = pageReceivers
	tt = ui.NewTester(recv.settingsView, 720, 500)
	recvHead, ok := tt.Find(locales.ReceiverGroupEnabled())
	if !ok {
		t.Fatalf("the 单独接收器 head is missing; texts: %q", tt.Texts())
	}

	if d := connHead.Y - recvHead.Y; d > 0.5 || d < -0.5 {
		t.Fatalf("section labels sit on different baselines: 已连接设备 at %.1f, 单独接收器 at %.1f",
			connHead.Y, recvHead.Y)
	}
}

// TestDeviceHeadKeepsItsHeight pins the reserved action slot: the device head
// measures the same with and without a live session, so the 全部断开 button
// never resizes the panel when it appears.
func TestDeviceHeadKeepsItsHeight(t *testing.T) {
	// Two targets, so the head carries its 全部断开 action; one target would
	// not show it, and the fixture would not cover both head states.
	with := previewApp()
	with.snap.ActiveTargets = []string{"AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02"}
	with.snap.Targets = append(with.snap.Targets, engine.TargetControl{
		ID: "AA:BB:CC:DD:EE:02", Name: "Apple TV", Volume: 50, Channel: "stereo", Stereo: false, State: engine.TargetMember,
	})
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
