package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/sender"
	"saundo/internal/engine"
	"saundo/internal/receivers"
)

// multiSessionApp is a streaming session with two members, the case the
// single-device surfaces used to miss.
func multiSessionApp() *App {
	a := previewApp()
	const second = "AA:BB:CC:DD:EE:02"
	a.snap.ActiveTargets = []string{"AA:BB:CC:DD:EE:01", second}
	a.snap.Targets = append(a.snap.Targets, engine.TargetControl{
		ID: second, Name: "Apple TV", Volume: 50, Channel: receivers.ChannelStereo,
		State: engine.TargetMember,
	})
	a.snap.State = sender.StateStreaming
	a.snap.ConnectedName = "HomePods + Apple TV"
	a.selID = "AA:BB:CC:DD:EE:01"
	return a
}

// TestSessionMembersFollowActiveTargets pins the read layer: active targets in
// session order, each with its device row and published controls.
func TestSessionMembersFollowActiveTargets(t *testing.T) {
	a := multiSessionApp()
	members := a.sessionMembers()
	if len(members) != 2 {
		t.Fatalf("members = %d, want two", len(members))
	}
	if members[0].Device.ID != "AA:BB:CC:DD:EE:01" || members[1].Device.ID != "AA:BB:CC:DD:EE:02" {
		t.Fatalf("member order = %q, %q", members[0].Device.ID, members[1].Device.ID)
	}
	for _, m := range members {
		if !m.HasTarget || m.Target.State != engine.TargetMember {
			t.Fatalf("member %s controls = %+v", m.Device.ID, m.Target)
		}
	}
	a.snap.ActiveTargets = nil
	if got := a.sessionMembers(); len(got) != 0 {
		t.Fatalf("idle session still lists %d members", len(got))
	}
}

// TestSessionNameJoinsWhileOpening pins the identity before the engine names
// the cohort: the member names are joined, and the engine's name wins later.
func TestSessionNameJoinsWhileOpening(t *testing.T) {
	a := multiSessionApp()
	a.snap.ConnectedName = ""
	if got, want := a.sessionName(), "HomePods + Apple TV"; got != want {
		t.Fatalf("opening session name = %q, want %q", got, want)
	}
	if primary, ok := a.sessionPrimary(); !ok || primary.Device.ID != "AA:BB:CC:DD:EE:01" {
		t.Fatalf("primary = %+v", primary)
	}
}

// TestSessionLatencyUsesSlowestMember pins the ALAC line's value: each member
// carries its own delay, so the session shows the slowest one.
func TestSessionLatencyUsesSlowestMember(t *testing.T) {
	a := multiSessionApp()
	a.snap.Receivers = []receivers.View{
		{Key: "AA:BB:CC:DD:EE:01", Name: "HomePods", DelayMS: 0, Online: true},
		{Key: "AA:BB:CC:DD:EE:02", Name: "Apple TV", DelayMS: 500, Online: true},
	}
	if got, want := a.sessionLatencyMS(), receivers.EffectiveLatencyMS(a.latencyMS(), 500); got != want {
		t.Fatalf("session latency = %d, want the slowest member %d", got, want)
	}
	a.snap.ActiveTargets = nil
	if got := a.sessionLatencyMS(); got != a.latencyMS() {
		t.Fatalf("idle latency = %d, want the base %d", got, a.latencyMS())
	}
}

// TestTrayMenuMarksEveryMember pins the tray: every session member carries the
// streaming subtitle and the accent glyph, not just the primary.
func TestTrayMenuMarksEveryMember(t *testing.T) {
	a := multiSessionApp()
	a.menuIcons = loadMenuIcons()
	items := a.trayMenuItems(a.trayModel("zh-CN", false))
	for i := 0; i < 2; i++ {
		if !items[i].Disabled {
			t.Fatalf("member row %d still offers a connect", i)
		}
		if got, want := items[i].Subtitle, locales.StatusStreaming(); got != want {
			t.Fatalf("member row %d subtitle = %q, want %q", i, got, want)
		}
	}
	m := a.trayModel("zh-CN", false)
	if got := m.deviceIcon(m.devices[0]); got != "devon:homepod-mini" {
		t.Fatalf("member 0 icon = %q", got)
	}
	if got := m.deviceIcon(m.devices[1]); got != "devon:appletv" {
		t.Fatalf("member 1 icon = %q", got)
	}
	if got := m.deviceIcon(m.devices[2]); got != "dev:homepod-mini" {
		t.Fatalf("idle row icon = %q", got)
	}
}

// TestConnectionPageSpeaksForTheSession pins the connection card and sidebar:
// a multi-member session shows the joined name and a member count.
func TestConnectionPageSpeaksForTheSession(t *testing.T) {
	a := multiSessionApp()
	a.page = pageConnection
	a.snap.ReceiverLatencyMS = 70
	tt := ui.NewTester(a.settingsView, 720, 500)
	for _, want := range []string{
		locales.SettingsConnectionDevices(), "2", "HomePods", "Apple TV",
	} {
		if !tt.HasText(want) {
			t.Fatalf("connection page is missing %q; texts: %q", want, tt.Texts())
		}
	}
	name, _, _ := a.sidebarIdentity(paletteFor(false))
	if name != "HomePods + Apple TV" {
		t.Fatalf("sidebar identity = %q, want the session name", name)
	}
}
