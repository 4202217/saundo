//go:build preview

package app

import (
	"testing"

	"saundo/internal/receivers"
)

func TestPreviewReceiverAdd(t *testing.T) {
	a := Fixture(previewLocale)
	beforeReceivers := len(a.snap.Receivers)
	beforeDevices := len(a.snap.Devices)
	if !a.previewAction(ActionReceiverAdd, receivers.Spec{Name: "Bedroom", Type: receivers.TypeAirPlay2, Host: "192.168.1.40", Port: 7000}) {
		t.Fatal("receiver-add was not handled")
	}
	if len(a.snap.Receivers) != beforeReceivers+1 {
		t.Fatalf("receivers = %d, want %d", len(a.snap.Receivers), beforeReceivers+1)
	}
	if len(a.snap.Devices) != beforeDevices+1 {
		t.Fatalf("devices = %d, want %d", len(a.snap.Devices), beforeDevices+1)
	}
	v := a.snap.Receivers[beforeReceivers]
	if v.Name != "Bedroom" || v.Address != "192.168.1.40:7000" || !v.Online {
		t.Fatalf("added row = %+v", v)
	}
	if v.Kind != receivers.KindManual || v.Device.ID != v.Key {
		t.Fatalf("added row kind or device = %+v", v)
	}
	if v.AutoConnect || v.Entry.AutoConnect {
		t.Fatalf("added row auto-connect = %v / %v, want off", v.AutoConnect, v.Entry.AutoConnect)
	}
}

// previewDeviceIn reports whether the flyout's device list holds this ID.
func previewDeviceIn(a *App, id string) bool {
	for _, d := range a.snap.Devices {
		if d.ID == id {
			return true
		}
	}
	return false
}

// TestPreviewReceiverHiddenSyncsTheFlyout mirrors receivers.Visible: a hidden
// row leaves the device list unless it is the active session's receiver.
func TestPreviewReceiverHiddenSyncsTheFlyout(t *testing.T) {
	a := Fixture(previewLocale)
	const hidden = "AA:BB:CC:DD:EE:02"
	if !a.previewAction(ActionReceiverHidden, receiverBoolArg{key: hidden, on: true}) {
		t.Fatal("receiver-hidden was not handled")
	}
	if previewDeviceIn(a, hidden) {
		t.Fatal("a hidden non-active receiver stayed in the flyout")
	}
	if !a.previewAction(ActionReceiverHidden, receiverBoolArg{key: hidden, on: false}) {
		t.Fatal("receiver-hidden was not handled")
	}
	if !previewDeviceIn(a, hidden) {
		t.Fatal("a shown receiver is missing from the flyout")
	}

	active := a.snap.SelectedID
	if !a.previewAction(ActionReceiverHidden, receiverBoolArg{key: active, on: true}) {
		t.Fatal("receiver-hidden was not handled")
	}
	if !previewDeviceIn(a, active) {
		t.Fatal("hiding the active receiver dropped its row")
	}
}

// TestPreviewReceiverAutoKeepsOneTarget mirrors Registry.SingleAutoConnect.
func TestPreviewReceiverAutoKeepsOneTarget(t *testing.T) {
	a := Fixture(previewLocale)
	const target = "AA:BB:CC:DD:EE:02"
	if !a.previewAction(ActionReceiverAuto, receiverBoolArg{key: target, on: true}) {
		t.Fatal("receiver-auto was not handled")
	}
	var on []string
	for _, v := range a.snap.Receivers {
		if v.AutoConnect {
			on = append(on, v.Key)
		}
		if v.Entry.AutoConnect != v.AutoConnect {
			t.Fatalf("%s: entry auto-connect = %v, view = %v", v.Key, v.Entry.AutoConnect, v.AutoConnect)
		}
	}
	if len(on) != 1 || on[0] != target {
		t.Fatalf("auto-connect targets = %v, want only %s", on, target)
	}
}
