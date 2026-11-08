package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/engine"
	"saundo/internal/settings"
)

// TestFlyoutLampCardStaysWhenOff pins the permanent card and the power
// action: the card never disappears; the power glyph turns the light off and
// on.
func TestFlyoutLampCardStaysWhenOff(t *testing.T) {
	a := previewApp()
	a.snap.Devices = nil
	a.snap.SelectedID = ""
	a.snap.State = 0
	a.selID = ""
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(0, a.showLatencySwitch, false))
	for _, want := range []string{"氛围灯", "沙丘"} {
		if !tt.HasText(want) {
			t.Fatalf("flyout is missing %q; texts: %q", want, tt.Texts())
		}
	}

	if err := tt.Click(locales.LampTurnOff()); err != nil {
		t.Fatal(err)
	}
	if !a.lampOff || !a.snap.LampOff || !a.cfg.Lamp.Off {
		t.Fatal("the power action did not turn the light off")
	}
	tt = ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(0, a.showLatencySwitch, false))
	for _, want := range []string{"氛围灯", "沙丘", "已关灯"} {
		if !tt.HasText(want) {
			t.Fatalf("flyout lost %q when the light turned off; texts: %q", want, tt.Texts())
		}
	}
	if err := tt.Click(locales.LampTurnOn()); err != nil {
		t.Fatal(err)
	}
	if a.lampOff || a.snap.LampOff || a.cfg.Lamp.Off {
		t.Fatal("the power action did not turn the light back on")
	}
}

func TestSettingsLampPowerTurnsTheLightOff(t *testing.T) {
	a := previewApp()
	a.page = pageLamp
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	if err := tt.Click(locales.LampTurnOff()); err != nil {
		t.Fatal(err)
	}
	if !a.lampOff || !a.snap.LampOff || !a.cfg.Lamp.Off {
		t.Fatal("the power action did not turn the light off")
	}
	fly := ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(len(a.snap.Devices), a.showLatencySwitch, false))
	fly.Frame()
	if !fly.HasText("氛围灯") {
		t.Fatal("the flyout lost the ambient card after the light turned off")
	}
}

func TestFlyoutLampExpandsAndChoosesPreset(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
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
	tt = ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	for _, want := range []string{"暖烬", "沙丘", "苔庭", "潮汐", "兰夜", "月白"} {
		if _, ok := tt.Find(want); !ok {
			t.Fatalf("expanded grid is missing %q", want)
		}
	}
	if err := tt.Click("暖烬"); err != nil {
		t.Fatal(err)
	}
	if a.lampPreset != "ember" || a.snap.LampPreset != "ember" {
		t.Fatalf("preset = %q/%q, want ember", a.lampPreset, a.snap.LampPreset)
	}
	if !a.lampExpanded {
		t.Fatal("picking a template collapsed the list")
	}
}

// TestFlyoutLampOffRequiresTurningOn pins the off rule: the grid is hidden,
// the segment and templates are disabled, and only 开灯 restores the light.
func TestFlyoutLampOffRequiresTurningOn(t *testing.T) {
	a := previewApp()
	a.lampOff = true
	a.snap.LampOff = true
	a.lampCover = false
	a.snap.LampCover = false
	a.lampPreset = "dune"
	a.lampExpanded = false

	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	for _, want := range []string{"氛围灯", "沙丘", "已关灯"} {
		if !tt.HasText(want) {
			t.Fatalf("flyout is missing %q while off; texts: %q", want, tt.Texts())
		}
	}
	if _, ok := tt.Find("暖烬"); ok {
		t.Fatal("the template grid is visible while the light is off")
	}
	a.setLampCover(true)
	a.chooseLampPreset("ember")
	if !a.lampOff || a.lampCover || a.lampPreset != "dune" {
		t.Fatalf("off state changed: off=%v cover=%v preset=%q", a.lampOff, a.lampCover, a.lampPreset)
	}
	if err := tt.Click(locales.LampTurnOn()); err != nil {
		t.Fatal(err)
	}
	if a.lampOff {
		t.Fatal("turning on did not restore the light")
	}
	tt = ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if err := tt.Click("展开配色"); err != nil {
		t.Fatal(err)
	}
	tt = ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if err := tt.Click("暖烬"); err != nil {
		t.Fatal(err)
	}
	if a.lampOff || a.lampPreset != "ember" {
		t.Fatalf("preset after turning on = %q/%v, want ember on", a.lampPreset, a.lampOff)
	}
}

// TestLampOffIgnoresPriorityAndPreset pins the guards behind the disabled
// controls: neither the segment nor a template changes the off state.
func TestLampOffIgnoresPriorityAndPreset(t *testing.T) {
	a := previewApp()
	a.lampOff = true
	a.snap.LampOff = true
	a.lampCover = true
	a.lampPreset = "dune"
	a.setLampCover(false)
	a.chooseLampPreset("ember")
	if !a.lampOff || !a.lampCover || a.lampPreset != "dune" {
		t.Fatalf("off state changed: off=%v cover=%v preset=%q", a.lampOff, a.lampCover, a.lampPreset)
	}
}

// TestSettingsLampPrioritySegmentTurnsCoverOn pins the settings segment.
func TestSettingsLampPrioritySegmentTurnsCoverOn(t *testing.T) {
	a := previewApp()
	a.page = pageLamp
	a.cfg.Lamp = settings.LampSettings{Preset: "dune"}
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	if err := tt.Click(locales.LampSourceCover()); err != nil {
		t.Fatal(err)
	}
	if !a.lampCover || !a.snap.LampCover || !a.cfg.Lamp.Cover {
		t.Fatal("the priority segment did not choose the cover")
	}
	if a.lampOff {
		t.Fatal("the priority segment left the light off")
	}
}

// TestSettingsLampPrioritySegmentTurnsCoverOff pins the other side.
func TestSettingsLampPrioritySegmentTurnsCoverOff(t *testing.T) {
	a := previewApp()
	a.page = pageLamp
	a.lampCover = true
	a.snap.LampCover = true
	a.cfg.Lamp = settings.LampSettings{Cover: true, Preset: "dune"}
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	if err := tt.Click(locales.LampSourceTemplate()); err != nil {
		t.Fatal(err)
	}
	if a.lampCover || a.snap.LampCover || a.cfg.Lamp.Cover {
		t.Fatal("the priority segment did not choose the template")
	}
	if a.lampExpanded {
		t.Fatal("choosing the template expanded the list by default")
	}
	if a.lampPreset != "dune" {
		t.Fatalf("the template changed to %q", a.lampPreset)
	}
}

// TestFlyoutLampCoverHidesTheGrid pins the flyout: cover priority hides the
// chevron and the template list; choosing the template shows them.
func TestFlyoutLampCoverHidesTheGrid(t *testing.T) {
	a := previewApp()
	a.lampCover = true
	a.snap.LampCover = true
	a.cfg.Lamp = settings.LampSettings{Cover: true, Preset: "dune"}
	a.lampExpanded = true
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	for _, hidden := range []string{"暖烬", "展开配色", "收起配色"} {
		if _, ok := tt.Find(hidden); ok {
			t.Fatalf("cover priority still shows %q", hidden)
		}
	}
	if err := tt.Click(locales.LampPriorityTemplateShort()); err != nil {
		t.Fatal(err)
	}
	if a.lampCover || a.lampExpanded {
		t.Fatal("choosing the template did not switch priority and stay collapsed")
	}
	tt = ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if _, ok := tt.Find("暖烬"); ok {
		t.Fatal("the template list expanded by default")
	}
	if err := tt.Click("展开配色"); err != nil {
		t.Fatal(err)
	}
	tt = ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if _, ok := tt.Find("暖烬"); !ok {
		t.Fatalf("expanding did not show the template list; texts: %q", tt.Texts())
	}
	if err := tt.Click("暖烬"); err != nil {
		t.Fatal(err)
	}
	if a.lampCover || a.lampPreset != "ember" {
		t.Fatalf("picking a template = cover:%v/%q, want template ember", a.lampCover, a.lampPreset)
	}
	if !a.lampExpanded {
		t.Fatal("picking a template collapsed the list")
	}
}

// TestFlyoutCloseCollapsesLampList pins the hide behavior: the template
// list folds when the flyout hides, so the next open starts collapsed.
func TestFlyoutCloseCollapsesLampList(t *testing.T) {
	a := previewApp()
	a.lampExpanded = true
	a.collapseLampList()
	if a.lampExpanded {
		t.Fatal("closing the flyout left the template list expanded")
	}
}

// TestSettingsLampPickingPresetWhileCoverOnTurnsCoverOff pins direct
// manipulation on the settings page, where the templates stay visible.
func TestSettingsLampPickingPresetWhileCoverOnTurnsCoverOff(t *testing.T) {
	a := previewApp()
	a.page = pageLamp
	a.lampCover = true
	a.snap.LampCover = true
	a.cfg.Lamp = settings.LampSettings{Cover: true, Preset: "dune"}
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	if err := tt.Click("暖烬"); err != nil {
		t.Fatal(err)
	}
	if a.lampCover || a.snap.LampCover || a.cfg.Lamp.Cover {
		t.Fatal("picking a template did not turn cover off")
	}
	if a.lampPreset != "ember" {
		t.Fatalf("preset = %q, want ember", a.lampPreset)
	}
}

func TestSettingsLampPageOffRequiresTurningOn(t *testing.T) {
	a := previewApp()
	a.page = pageLamp
	a.lampOff = true
	a.snap.LampOff = true
	a.lampCover = true
	a.snap.LampCover = true
	a.lampPreset = "dune"
	tt := ui.NewTester(a.settingsView, 720, 500)
	if !tt.HasText(locales.SettingsLampTemplatesDesc()) {
		t.Fatalf("template description is missing; texts: %q", tt.Texts())
	}
	a.chooseLampPreset("ember")
	if !a.lampOff || a.lampPreset != "dune" {
		t.Fatalf("picking a template while off changed the light: off=%v preset=%q", a.lampOff, a.lampPreset)
	}
	tt.Frame()
	if err := tt.Click(locales.LampTurnOn()); err != nil {
		t.Fatal(err)
	}
	if a.lampOff || a.snap.LampOff {
		t.Fatal("turning on did not restore the light")
	}
	tt = ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	if err := tt.Click("暖烬"); err != nil {
		t.Fatal(err)
	}
	if a.lampOff || a.lampPreset != "ember" {
		t.Fatalf("preset after turning on = %q/%v, want ember on", a.lampPreset, a.lampOff)
	}
}

func TestSettingsLampPageChoosesPreset(t *testing.T) {
	a := previewApp()
	a.page = pageLamp
	a.lampExpanded = true
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	if err := tt.Click("暖烬"); err != nil {
		t.Fatal(err)
	}
	if a.lampPreset != "ember" || a.snap.LampPreset != "ember" {
		t.Fatalf("preset = %q/%q, want ember", a.lampPreset, a.snap.LampPreset)
	}
	if !a.lampExpanded {
		t.Fatal("picking a template collapsed the list")
	}
}

func TestLampSurfacesInEnglish(t *testing.T) {
	withLocale(t, "en")
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	for _, want := range []string{"Ambient light", "Cover", "Light", "Dune"} {
		if !tt.HasText(want) {
			t.Fatalf("English flyout is missing %q; texts: %q", want, tt.Texts())
		}
	}
	a.page = pageLamp
	tt = ui.NewTester(a.settingsView, 720, 500)
	for _, want := range []string{"Ambient light", "HomePod only", "Color source", "Cover", "Template", "Dune"} {
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
	a.apply(engine.Snapshot{LampOff: false, LampPreset: "ember"})
	a.cfg.LaunchAtStart = true
	a.saveSettings()
	got := a.sets.Load()
	if got.Lamp.Off || got.Lamp.Preset != "ember" {
		t.Fatalf("app save wrote lamp %+v, want light on + ember", got.Lamp)
	}
}
