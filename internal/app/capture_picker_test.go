package app

import (
	"testing"

	"saundo/cmd/saundo/locales"
)

// capturePickerApp gives the picker two endpoints with the same name and one
// whose name equals the follow-system sentinel.
func capturePickerApp() *App {
	a := previewApp()
	a.captureNames = []string{locales.CaptureFollowSystem(), "Speakers", "Speakers", locales.CaptureFollowSystem()}
	a.captureIDs = []string{"", "id-1", "id-2", "id-3"}
	a.rebuildCaptureLabels()
	return a
}

// TestCaptureLabelsKeepEveryEndpointSelectable pins the identity fix: the
// select's labels are unique, and each label resolves to its own endpoint ID
// and real name.
func TestCaptureLabelsKeepEveryEndpointSelectable(t *testing.T) {
	a := capturePickerApp()
	labels := a.captureLabels
	if len(labels) != len(a.captureNames) {
		t.Fatalf("labels = %q, want one per endpoint", labels)
	}
	if labels[0] != locales.CaptureFollowSystem() {
		t.Fatalf("sentinel label = %q", labels[0])
	}
	if labels[1] == labels[2] {
		t.Fatalf("duplicate endpoint names share the label %q", labels[1])
	}
	if labels[3] == labels[0] {
		t.Fatalf("an endpoint named like the sentinel is unselectable: %q", labels[3])
	}
	for _, tc := range []struct {
		label string
		id    string
		name  string
	}{
		{labels[2], "id-2", "Speakers"},
		{labels[3], "id-3", locales.CaptureFollowSystem()},
	} {
		id, name, ok := a.captureEndpointFor(tc.label)
		if !ok || id != tc.id || name != tc.name {
			t.Fatalf("captureEndpointFor(%q) = %q, %q, %v; want %q, %q", tc.label, id, name, ok, tc.id, tc.name)
		}
	}
}

// TestSnapshotSelectsTheEndpointByIDAndClearsStaleLabel pins the sync path:
// the engine's endpoint ID selects its own row, and a follow-system snapshot
// drops a stale label instead of leaving the old device on screen.
func TestSnapshotSelectsTheEndpointByIDAndClearsStaleLabel(t *testing.T) {
	a := capturePickerApp()
	a.snap.CaptureEndpoint = "id-2"
	a.syncCaptureChoice()
	if a.captureChoice != a.captureLabels[2] {
		t.Fatalf("selected label = %q, want %q", a.captureChoice, a.captureLabels[2])
	}
	a.captureChoice = "stale"
	a.snap.CaptureEndpoint = ""
	a.applySnapshot(a.snap)
	if a.captureChoice != locales.CaptureFollowSystem() {
		t.Fatalf("follow-system snapshot kept the stale label %q", a.captureChoice)
	}
}
