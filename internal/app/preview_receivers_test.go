//go:build preview

package app

import (
	"testing"

	"saundo/internal/receivers"
)

func TestPreviewReceiverAdd(t *testing.T) {
	a := PreviewApp()
	before := len(a.snap.Receivers)
	if !previewReduce(a, "receiver-add", receivers.Spec{Name: "Bedroom", Type: receivers.TypeAirPlay2, Host: "192.168.1.40", Port: 7000}) {
		t.Fatal("receiver-add was not handled")
	}
	if len(a.snap.Receivers) != before+1 {
		t.Fatalf("receivers = %d, want %d", len(a.snap.Receivers), before+1)
	}
	if len(a.snap.Devices) != before+1 {
		t.Fatalf("devices = %d, want %d", len(a.snap.Devices), before+1)
	}
	v := a.snap.Receivers[before]
	if v.Name != "Bedroom" || v.Address != "192.168.1.40:7000" || !v.Online {
		t.Fatalf("added row = %+v", v)
	}
	if v.Kind != receivers.KindManual || v.Device.ID != v.Key {
		t.Fatalf("added row kind or device = %+v", v)
	}
}
