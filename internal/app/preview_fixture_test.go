//go:build preview

package app

import "testing"

// previewScenario returns the catalog state with this ID.
func previewScenario(t *testing.T, id string) Scenario {
	t.Helper()
	for _, sc := range PreviewScenarios() {
		if sc.ID == id {
			return sc
		}
	}
	t.Fatalf("no preview scenario %q", id)
	return Scenario{}
}

// TestFixtureCarriesMedia pins the shared fixture through the flyout.stream
// state: the scenario must fill the Now Playing card and the lamp cover data
// it shares, not just the fields it happens to set.
func TestFixtureCarriesMedia(t *testing.T) {
	a := Fixture(previewLocale)
	previewScenario(t, "flyout.stream").Apply(a)
	s, ok := a.nowSession()
	if !ok || s.Title == "" {
		t.Fatal("the streaming state has no media session")
	}
	if !a.nowOK || a.nowTrack.Title != s.Title {
		t.Fatalf("nowOK = %v, track = %q", a.nowOK, a.nowTrack.Title)
	}
	if a.nowArt == nil || !a.nowLampOK || a.nowFieldLight == nil || a.levelFrame == nil {
		t.Fatal("the media fixture is incomplete")
	}
}

// TestFixtureLocalizesCapture pins both capture arrays: the option's label
// and its name follow the fixture's locale.
func TestFixtureLocalizesCapture(t *testing.T) {
	zh := Fixture("zh-CN")
	if zh.captureNames[1] != "扬声器" || zh.captureLabels[1] != "扬声器" {
		t.Fatalf("zh capture = %q / %q, want 扬声器", zh.captureNames[1], zh.captureLabels[1])
	}
	en := Fixture("en")
	if en.captureNames[1] != "Speakers" || en.captureLabels[1] != "Speakers" {
		t.Fatalf("en capture = %q / %q, want Speakers", en.captureNames[1], en.captureLabels[1])
	}
}

// TestFixtureAppliesTheme pins the appearance hook the settings select calls.
func TestFixtureAppliesTheme(t *testing.T) {
	if Fixture(previewLocale).setThemeSource == nil {
		t.Fatal("the fixture has no theme source")
	}
}

// TestFixtureFillsReceivers pins the base rows: the receivers page and the
// flyout read the same three receivers, and one row carries auto-connect.
func TestFixtureFillsReceivers(t *testing.T) {
	a := Fixture(previewLocale)
	if len(a.snap.Devices) != 3 || len(a.snap.Receivers) != 3 {
		t.Fatalf("devices = %d, receivers = %d, want 3 and 3", len(a.snap.Devices), len(a.snap.Receivers))
	}
	auto := 0
	for _, v := range a.snap.Receivers {
		if v.AutoConnect {
			auto++
		}
	}
	if auto != 1 {
		t.Fatalf("auto-connect rows = %d, want 1", auto)
	}
}
