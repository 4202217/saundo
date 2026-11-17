//go:build preview

package app

import (
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/settings"
)

var flyoutScenarios = []Scenario{
	{ID: "flyout.stream", Name: "flyout-stream-light", Dark: "flyout-stream-dark", Surface: SurfaceFlyout, Size: flyoutSize, Theme: ThemeBoth, Apply: applyStreamingCard, SiteDemo: true},
	{ID: "flyout.stream.largeText", Name: "flyout-stream-large-text", Surface: SurfaceFlyout, Size: flyoutSize, Scale: 1.25, Apply: applyStreamingCard},
	{ID: "flyout.nowPlaying.unknown", Name: "flyout-nowplaying-unknown-light", Dark: "flyout-nowplaying-unknown-dark", Surface: SurfaceFlyout, Size: flyoutSize, Theme: ThemeBoth, Apply: applyUnknownSession, SiteDemo: true},
	{ID: "flyout.idle", Name: "flyout-idle", Surface: SurfaceFlyout, Size: flyoutSize, Apply: applyIdle, SiteDemo: true},
	{ID: "flyout.connecting", Name: "flyout-connecting", Surface: SurfaceFlyout, Size: flyoutSize, SiteDemo: true, Apply: func(a *App) {
		a.snap.State = sender.StateHandshake
		a.snap.ConnectedName = ""
	}},
	{ID: "flyout.error", Name: "flyout-error", Surface: SurfaceFlyout, Size: flyoutSize, SiteDemo: true, Apply: func(a *App) {
		a.snap.State = sender.StateIdle
		a.snap.Error = "接收器没有响应。"
		if i18n.Locale() == "en" {
			a.snap.Error = "The receiver did not respond."
		}
		a.snap.ConnectedName = ""
	}},
	{ID: "flyout.credential", Name: "flyout-credential-light", Dark: "flyout-credential-dark", Surface: SurfaceFlyout, Size: flyoutSize, Theme: ThemeBoth, Apply: func(a *App) {
		a.snap.CredentialReset = true
	}},
	{ID: "flyout.takenOver", Name: "flyout-taken-over-light", Dark: "flyout-taken-over-dark", Surface: SurfaceFlyout, Size: flyoutSize, Theme: ThemeBoth, Apply: applyTakenOver, SiteDemo: true},
	{ID: "flyout.customLatency", Name: "flyout-custom-light", Dark: "flyout-custom-dark", Surface: SurfaceFlyout, Size: flyoutSize, Theme: ThemeBoth, Apply: applyCustomLatency, SiteDemo: true},
	{ID: "flyout.latencySwitchOff", Name: "flyout-switch-off", Surface: SurfaceFlyout, Size: flyoutSize, Apply: func(a *App) {
		a.showLatencySwitch = false
	}},
	{ID: "flyout.latencyApplying", Name: "flyout-latency-applying", Dark: "flyout-latency-applying-dark", Surface: SurfaceFlyout, Size: flyoutSize, Theme: ThemeBoth, Apply: applyLatencyApplying, SiteDemo: true},
	{ID: "flyout.empty", Name: "flyout-empty", Surface: SurfaceFlyout, Size: flyoutSize, SiteDemo: true, Apply: func(a *App) {
		a.snap.Devices = nil
		a.snap.State = sender.StateIdle
		a.snap.SelectedID = ""
		a.selID = ""
	}},
	{ID: "flyout.noDevice", Name: "flyout-no-device", Surface: SurfaceFlyout, Size: flyoutSize, Apply: func(a *App) {
		a.snap.Devices = nil
		a.snap.SelectedID = ""
		a.selID = ""
	}},
	{ID: "flyout.coverNone", Name: "flyout-cover-none-light", Surface: SurfaceFlyout, Size: flyoutSize, SiteDemo: true, Apply: func(a *App) {
		applySampleCard(a, i18n.Locale())
		a.cfg.CoverMode = settings.CoverNone
	}},
	{ID: "flyout.coverPlaceholder", Name: "flyout-cover-placeholder-light", Dark: "flyout-cover-placeholder-dark", Surface: SurfaceFlyout, Size: flyoutSize, Theme: ThemeBoth, SiteDemo: true, Apply: func(a *App) {
		applySampleCard(a, i18n.Locale())
		a.cfg.CoverMode = settings.CoverPlaceholder
	}},
	{ID: "flyout.mediaOff", Name: "flyout-media-off-light", Surface: SurfaceFlyout, Size: flyoutSize, SiteDemo: true, Apply: func(a *App) {
		a.cfg.ShowNowPlayingPanel = false
		a.nowSnap = sampleSnapshot(i18n.Locale())
		if len(a.nowSnap.Sessions) > 0 {
			a.nowMediaAUMID = a.nowSnap.Sessions[0].AUMID
		}
	}},
	{ID: "flyout.lamp.standby", Name: "flyout-lamp-standby-light", Dark: "flyout-lamp-standby-dark", Surface: SurfaceFlyout, Size: flyoutSize, Theme: ThemeBoth, SiteDemo: true, Apply: func(a *App) {
		a.snap.Standby = true
	}},
	{ID: "flyout.lamp.collapsed", Name: "flyout-lamp-collapsed-light", Dark: "flyout-lamp-collapsed-dark", Surface: SurfaceFlyout, Size: flyoutSize, Theme: ThemeBoth, Apply: applyBase, SiteDemo: true},
	{ID: "flyout.lamp.expanded", Name: "flyout-lamp-expanded-light", Dark: "flyout-lamp-expanded-dark", Surface: SurfaceFlyout, Size: flyoutSize, Theme: ThemeBoth, SiteDemo: true, Apply: func(a *App) {
		a.lampCover = false
		a.snap.LampCover = false
		a.lampExpanded = true
	}},
	{ID: "flyout.lamp.off", Name: "flyout-lamp-off-light", Surface: SurfaceFlyout, Size: flyoutSize, Apply: applyLampOff, SiteDemo: true},
	{ID: "flyout.lamp.cover", Name: "flyout-lamp-cover-light", Dark: "flyout-lamp-cover-dark", Surface: SurfaceFlyout, Size: flyoutSize, Theme: ThemeBoth, SiteDemo: true, Apply: func(a *App) {
		applyLampCover(a)
		a.lampExpanded = true
	}},
	{ID: "flyout.lamp.noMedia", Name: "flyout-lamp-nomedia-light", Surface: SurfaceFlyout, Size: flyoutSize, Apply: func(a *App) {
		a.lampCover = true
		a.snap.LampCover = true
	}},
	{ID: "flyout.mac", Name: "flyout-mac", Surface: SurfaceFlyout, Size: flyoutSize, Apply: func(a *App) {
		a.snap.Devices = []device.Device{{
			ID: "AA:BB:CC:DD:EE:09", Name: "MacBook", Model: "Mac14,2",
			Auth: device.AuthHAPPassword, AirPlay2: true, Online: true,
		}}
		a.snap.SelectedID = ""
		a.snap.State = sender.StateIdle
		a.snap.ConnectedName = ""
		a.selID = ""
	}},
	{ID: "flyout.hover", Name: "flyout-hover", Surface: SurfaceFlyout, Size: flyoutSize, Apply: applyBase, Move: func(tt *ui.Tester) {
		tt.Move(200, 166)
	}},
	{ID: "flyout.update.chip", Name: "flyout-update-chip", Surface: SurfaceFlyout, Size: flyoutSize, Apply: applyUpdateChip},
	{ID: "probe.flyout.enter", Name: "probe-flyout-enter", Surface: SurfaceFlyout, Size: flyoutSize, Frames: 3, Apply: applyBase},
}
