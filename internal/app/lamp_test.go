package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/engine"
	"saundo/internal/settings"
)

func TestFlyoutLampCardAlwaysVisible(t *testing.T) {
	a := previewApp()
	a.snap.Devices = nil
	a.snap.SelectedID = ""
	a.snap.State = 0
	a.selID = ""
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(0, a.showLatencySwitch))
	for _, want := range []string{"氛围灯", "HomePod 专属", "沙丘"} {
		if !tt.HasText(want) {
			t.Fatalf("flyout is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestFlyoutLampSwitchToggles(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(len(a.snap.Devices), a.showLatencySwitch))
	tt.Frame()
	if err := tt.Click(locales.LampToggle()); err != nil {
		t.Fatal(err)
	}
	if a.lampEnabled || a.snap.LampEnabled || a.cfg.Lamp.Enabled {
		t.Fatal("the switch did not turn the lamp off")
	}
}

func TestFlyoutLampExpandsAndChoosesPreset(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightForState(len(a.snap.Devices), a.showLatencySwitch, false))
	tt.Frame()
	if _, ok := tt.Find("苔庭"); ok {
		t.Fatal("the preset grid is visible while collapsed")
	}
	if err := tt.Click("展开配色"); err != nil {
		t.Fatal(err)
	}
	if !a.lampExpanded {
		t.Fatal("the arrow did not expand the card")
	}
	tt = ui.NewTester(a.view, flyoutWidth, flyoutHeightForState(len(a.snap.Devices), a.showLatencySwitch, true))
	tt.Frame()
	for _, want := range []string{"暖烬", "沙丘", "苔庭", "潮汐", "兰夜", "月白"} {
		if _, ok := tt.Find(want); !ok {
			t.Fatalf("expanded grid is missing %q", want)
		}
	}
	if err := tt.Click("苔庭"); err != nil {
		t.Fatal(err)
	}
	if a.lampPreset != "moss" || a.snap.LampPreset != "moss" {
		t.Fatalf("preset = %q/%q, want moss", a.lampPreset, a.snap.LampPreset)
	}
}

func TestFlyoutLampOffDisablesGrid(t *testing.T) {
	a := previewApp()
	a.lampEnabled = false
	a.snap.LampEnabled = false
	a.lampExpanded = true
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightForState(len(a.snap.Devices), a.showLatencySwitch, true))
	tt.Frame()
	if err := tt.Click("苔庭"); err != nil {
		t.Fatal(err)
	}
	if a.lampPreset != "dune" {
		t.Fatalf("preset changed while off: %q", a.lampPreset)
	}
}

func TestSettingsLampPageOffShowsHintAndDisablesPresets(t *testing.T) {
	a := previewApp()
	a.page = pageLamp
	a.lampEnabled = false
	a.snap.LampEnabled = false
	tt := ui.NewTester(a.settingsView, 720, 500)
	if !tt.HasText("关闭后不发送灯色") {
		t.Fatalf("off hint is missing; texts: %q", tt.Texts())
	}
	tt.Frame()
	if err := tt.Click("潮汐"); err != nil {
		t.Fatal(err)
	}
	if a.lampPreset != "dune" {
		t.Fatalf("preset changed while off: %q", a.lampPreset)
	}
}

func TestSettingsLampPageChoosesPreset(t *testing.T) {
	a := previewApp()
	a.page = pageLamp
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	if err := tt.Click("潮汐"); err != nil {
		t.Fatal(err)
	}
	if a.lampPreset != "tide" || a.snap.LampPreset != "tide" {
		t.Fatalf("preset = %q/%q, want tide", a.lampPreset, a.snap.LampPreset)
	}
}

func TestLampSurfacesInEnglish(t *testing.T) {
	withLocale(t, "en")
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(len(a.snap.Devices), a.showLatencySwitch))
	for _, want := range []string{"Ambient light", "HomePod only", "Dune"} {
		if !tt.HasText(want) {
			t.Fatalf("English flyout is missing %q; texts: %q", want, tt.Texts())
		}
	}
	a.page = pageLamp
	tt = ui.NewTester(a.settingsView, 720, 500)
	for _, want := range []string{"Ambient light", "Enable ambient light", "HomePod only", "HomePod top display", "Dune"} {
		if !tt.HasText(want) {
			t.Fatalf("English lamp page is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

// TestAppSaveKeepsEngineLamp pins the single-writer rule: an app-only save
// must carry the engine's current lamp choice, not the value loaded at
// startup.
func TestAppSaveKeepsEngineLamp(t *testing.T) {
	a := previewApp()
	a.sets = settings.NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	a.apply(engine.Snapshot{LampEnabled: true, LampPreset: "orchid"})
	a.cfg.LaunchAtStart = true
	a.saveSettings()
	got := a.sets.Load()
	if !got.Lamp.Enabled || got.Lamp.Preset != "orchid" {
		t.Fatalf("app save wrote lamp %+v, want on + orchid", got.Lamp)
	}
}
