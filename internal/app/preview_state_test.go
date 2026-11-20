//go:build preview

package app

import "testing"

// TestPreviewScenariosApplyEveryState is the browser-free evidence for the
// state selector: every scenario applies without a panic, sets its settings
// page, and asks for its host window.
func TestPreviewScenariosApplyEveryState(t *testing.T) {
	scenarios := PreviewScenarios()
	if len(scenarios) == 0 {
		t.Fatal("no preview scenarios")
	}
	surfaces := map[Surface]bool{}
	for _, sc := range scenarios {
		surfaces[sc.Surface] = true
	}
	for _, want := range []Surface{SurfaceFlyout, SurfaceSettings, SurfaceDialog, SurfaceTrayMenu} {
		if !surfaces[want] {
			t.Fatalf("the wasm preview has no surface %d state", want)
		}
	}

	old := previewShowHost
	t.Cleanup(func() { previewShowHost = old })
	var hosts []Surface
	previewShowHost = func(_ *App, host Surface) { hosts = append(hosts, host) }

	for i, sc := range scenarios {
		a := Fixture("en")
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s: previewStateAt panicked: %v", sc.ID, r)
				}
			}()
			previewStateAt(a, i)
		}()
		if sc.usesSettingsPage() && a.page != sc.Page {
			t.Fatalf("%s: page = %d, want %d", sc.ID, a.page, sc.Page)
		}
		want := SurfaceFlyout
		switch {
		case sc.Surface == SurfaceSettings || (sc.Surface == SurfaceDialog && sc.Host == SurfaceSettings):
			want = SurfaceSettings
		case sc.Surface == SurfaceTrayMenu:
			want = SurfaceTrayMenu
		}
		if len(hosts) != i+1 {
			t.Fatalf("%s: host was not shown", sc.ID)
		}
		if hosts[i] != want {
			t.Fatalf("%s: host = %d, want %d", sc.ID, hosts[i], want)
		}
	}
	if previewStateCount() != len(scenarios) {
		t.Fatalf("previewStateCount = %d, want %d", previewStateCount(), len(scenarios))
	}
}

// previewScenarioIndex is the selector position of a catalog state.
func previewScenarioIndex(t *testing.T, id string) int {
	t.Helper()
	for i, sc := range PreviewScenarios() {
		if sc.ID == id {
			return i
		}
	}
	t.Fatalf("no preview scenario %q", id)
	return -1
}

// TestPreviewStatesDoNotAccumulate drives the one long-lived App the wasm
// preview keeps: a state must read the fresh fixture, not the state selected
// before it.
func TestPreviewStatesDoNotAccumulate(t *testing.T) {
	old := previewShowHost
	t.Cleanup(func() { previewShowHost = old })
	previewShowHost = func(*App, Surface) {}

	stream := previewScenarioIndex(t, "flyout.stream")
	a := Fixture(previewLocale)
	for _, id := range []string{"flyout.idle", "flyout.error", "flyout.empty"} {
		previewStateAt(a, previewScenarioIndex(t, id))
		previewStateAt(a, stream)
		want := Fixture(previewLocale)
		if a.snap.State != want.snap.State || a.snap.ConnectedName != want.snap.ConnectedName || a.snap.PTPActive != want.snap.PTPActive {
			t.Fatalf("after %s: state = %v, name = %q, ptp = %v", id, a.snap.State, a.snap.ConnectedName, a.snap.PTPActive)
		}
		if a.snap.Error != "" || a.snap.TakenOver {
			t.Fatalf("after %s: error = %q, takenOver = %v", id, a.snap.Error, a.snap.TakenOver)
		}
		if len(a.snap.Devices) != len(want.snap.Devices) {
			t.Fatalf("after %s: devices = %d, want %d", id, len(a.snap.Devices), len(want.snap.Devices))
		}
		if !a.nowOK || a.nowTrack.Title == "" {
			t.Fatalf("after %s: the media card is empty", id)
		}
	}
}

// TestSettingsStateSurvivesOnShow proves the scenario applies after its host
// window opens: the settings window's OnShow hook clears the log, so the
// dialog state must set it again.
func TestSettingsStateSurvivesOnShow(t *testing.T) {
	old := previewShowHost
	t.Cleanup(func() { previewShowHost = old })
	previewShowHost = func(a *App, host Surface) {
		if host == SurfaceSettings {
			a.settingsEntered = false
			a.showLog = false
		}
	}

	a := Fixture(previewLocale)
	previewStateAt(a, previewScenarioIndex(t, "dialog.pin"))
	previewStateAt(a, previewScenarioIndex(t, "dialog.log"))
	if !a.showLog {
		t.Fatal("the host's OnShow hook cleared the log scenario")
	}
	if a.page != pageAbout {
		t.Fatalf("page = %d, want pageAbout", a.page)
	}
}
