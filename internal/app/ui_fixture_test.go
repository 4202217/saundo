package app

// The demo fixture is the source the landing page and the preview matrix both
// read, so its opening state has to be the app's own default, not a second
// opinion kept beside settings.Default.

import (
	"testing"

	"saundo/internal/settings"
)

func TestFixtureLampFollowsSettingsDefault(t *testing.T) {
	cfg := settings.Default()
	a := previewApp()
	if a.lampOff != cfg.Lamp.Off || a.lampCover != cfg.Lamp.Cover || a.lampPreset != cfg.Lamp.Preset {
		t.Fatalf("fixture lamp = off %v, cover %v, preset %q; settings.Default = off %v, cover %v, preset %q",
			a.lampOff, a.lampCover, a.lampPreset, cfg.Lamp.Off, cfg.Lamp.Cover, cfg.Lamp.Preset)
	}
	if a.snap.LampOff != cfg.Lamp.Off || a.snap.LampCover != cfg.Lamp.Cover || a.snap.LampPreset != cfg.Lamp.Preset {
		t.Fatalf("fixture snapshot lamp = off %v, cover %v, preset %q; settings.Default = off %v, cover %v, preset %q",
			a.snap.LampOff, a.snap.LampCover, a.snap.LampPreset, cfg.Lamp.Off, cfg.Lamp.Cover, cfg.Lamp.Preset)
	}
}

func TestFixtureHomePodsIsAStereoPair(t *testing.T) {
	a := previewApp()
	if len(a.snap.Devices) == 0 {
		t.Fatal("the fixture has no devices")
	}
	d := a.snap.Devices[0]
	if d.Name != "HomePods" {
		t.Fatalf("the first receiver is %q, want HomePods", d.Name)
	}
	if !d.IsStereoGroup() || len(d.Members) != 2 {
		t.Fatalf("%q is not a two-member stereo group: stereo ID %q, %d members", d.Name, d.StereoID, len(d.Members))
	}
	if len(a.snap.Receivers) == 0 || !a.snap.Receivers[0].Stereo {
		t.Fatal("the receivers page does not carry the same stereo group")
	}
}
