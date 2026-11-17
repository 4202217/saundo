//go:build preview

package app

import (
	"time"

	"github.com/egoist/mygo/i18n"

	"saundo/internal/airplay/sender"
	"saundo/internal/engine"
	"saundo/internal/settings"
)

var (
	sizeSettings = fixedSize(settingsContentWidth, settingsContentHeight)
	sizeCompact  = fixedSize(720, 420)
	sizeMinimum  = fixedSize(640, 420)
	sizeAudioMin = fixedSize(640, 500)
	sizeWide     = fixedSize(1440, 900)
)

var settingsScenarios = []Scenario{
	{ID: "settings.general", Name: "settings-general-light", Surface: SurfaceSettings, Page: pageGeneral, Size: sizeSettings, Apply: applyBase},
	{ID: "settings.connection", Name: "settings-connection-light", Dark: "settings-connection-dark", Surface: SurfaceSettings, Page: pageConnection, Size: sizeSettings, Theme: ThemeBoth, Apply: applyBase},
	{ID: "settings.receivers", Name: "settings-receivers-light", Surface: SurfaceSettings, Page: pageReceivers, Size: sizeSettings, Apply: applyReceiverRows},
	{ID: "settings.audio", Name: "settings-audio-light", Surface: SurfaceSettings, Page: pageAudio, Size: sizeSettings, Apply: applyBase},
	{ID: "settings.lamp", Name: "settings-lamp-light", Surface: SurfaceSettings, Page: pageLamp, Size: sizeSettings, Apply: applyBase},
	{ID: "settings.capture", Name: "settings-capture-light", Surface: SurfaceSettings, Page: pageCapture, Size: sizeSettings, Apply: applyBase},
	{ID: "settings.nowPlaying", Name: "settings-nowplaying-light", Surface: SurfaceSettings, Page: pageNowPlaying, Size: sizeSettings, Apply: applyBase},
	{ID: "settings.about", Name: "settings-about-light", Dark: "settings-about-dark", Surface: SurfaceSettings, Page: pageAbout, Size: sizeSettings, Theme: ThemeBoth, Apply: applyBase},
	{ID: "settings.connection.largeText", Name: "settings-connection-large-text", Surface: SurfaceSettings, Page: pageConnection, Size: sizeSettings, Scale: 1.25, Apply: applyBase},
	{ID: "settings.connection.mute", Name: "settings-connection-mute-light", Dark: "settings-connection-mute-dark", Surface: SurfaceSettings, Page: pageConnection, Size: sizeSettings, Theme: ThemeBoth, Apply: applyMuteActive},
	{ID: "settings.connection.muteWaiting", Name: "settings-connection-mute-waiting-light", Surface: SurfaceSettings, Page: pageConnection, Size: sizeSettings, Apply: applyMuteWaiting},
	{ID: "settings.connection.minimum", Name: "settings-connection-640-light", Dark: "settings-connection-640-dark", Surface: SurfaceSettings, Page: pageConnection, Size: sizeMinimum, Theme: ThemeBoth, Apply: applyBase},
	{ID: "settings.general.minimumBottom", Name: "settings-general-640-bottom-light", Dark: "settings-general-640-bottom-dark", Surface: SurfaceSettings, Page: pageGeneral, Size: sizeMinimum, Theme: ThemeBoth, Scroll: 400, Apply: applyBase},
	{ID: "settings.connection.unknown", Name: "settings-connection-unknown-light", Dark: "settings-connection-unknown-dark", Surface: SurfaceSettings, Page: pageConnection, Size: sizeSettings, Theme: ThemeBoth, Apply: func(a *App) {
		a.snap.ReceiverLatencyMS = 0
	}},
	{ID: "settings.connection.unstable", Name: "settings-connection-unstable-light", Dark: "settings-connection-unstable-dark", Surface: SurfaceSettings, Page: pageConnection, Size: sizeSettings, Theme: ThemeBoth, Apply: func(a *App) {
		a.snap.DroppedSamples = 3
	}},
	{ID: "settings.nowPlaying.live", Name: "settings-nowplaying-live-light", Dark: "settings-nowplaying-live-dark", Surface: SurfaceSettings, Page: pageNowPlaying, Size: sizeSettings, Theme: ThemeBoth, Apply: applyLiveCard},
	{ID: "settings.nowPlaying.off", Name: "settings-nowplaying-off-light", Surface: SurfaceSettings, Page: pageNowPlaying, Size: sizeSettings, Apply: applyNowPlayingOff},
	{ID: "settings.nowPlaying.placeholder", Name: "settings-nowplaying-placeholder-light", Dark: "settings-nowplaying-placeholder-dark", Surface: SurfaceSettings, Page: pageNowPlaying, Size: sizeSettings, Theme: ThemeBoth, Apply: applyNowPlayingPlaceholder},
	{ID: "settings.nowPlaying.noCover", Name: "settings-nowplaying-no-cover-light", Surface: SurfaceSettings, Page: pageNowPlaying, Size: sizeSettings, Apply: applyNowPlayingNoCover},
	{ID: "settings.nowPlaying.bottom", Name: "settings-nowplaying-bottom-light", Surface: SurfaceSettings, Page: pageNowPlaying, Size: sizeSettings, Scroll: 500, Apply: applyNowPlayingBottom},
	{ID: "settings.connection.takenOver", Name: "settings-connection-taken-over-light", Dark: "settings-connection-taken-over-dark", Surface: SurfaceSettings, Page: pageConnection, Size: sizeSettings, Theme: ThemeBoth, Apply: applyTakenOver},
	{ID: "settings.connection.takenOverMinimum", Name: "settings-connection-taken-over-640-light", Surface: SurfaceSettings, Page: pageConnection, Size: sizeMinimum, Apply: applyTakenOver},
	{ID: "settings.receivers.expanded", Name: "settings-receivers-expanded-light", Dark: "settings-receivers-expanded-dark", Surface: SurfaceSettings, Page: pageReceivers, Size: sizeSettings, Theme: ThemeBoth, Apply: applyReceiversExpanded},
	{ID: "settings.receivers.hidden", Name: "settings-receivers-hidden-light", Surface: SurfaceSettings, Page: pageReceivers, Size: sizeSettings, Apply: applyReceiversHidden},
	{ID: "settings.receivers.local", Name: "settings-receivers-local-light", Dark: "settings-receivers-local-dark", Surface: SurfaceSettings, Page: pageReceivers, Size: sizeSettings, Theme: ThemeBoth, Apply: applyReceiversLocal},
	{ID: "settings.receivers.empty", Name: "settings-receivers-empty-light", Dark: "settings-receivers-empty-dark", Surface: SurfaceSettings, Page: pageReceivers, Size: sizeSettings, Theme: ThemeBoth, Apply: applyReceiversEmpty},
	{ID: "settings.receivers.allHidden", Name: "settings-receivers-all-hidden-light", Dark: "settings-receivers-all-hidden-dark", Surface: SurfaceSettings, Page: pageReceivers, Size: sizeSettings, Theme: ThemeBoth, Apply: applyReceiversAllHidden},
	{ID: "settings.audio.custom", Name: "settings-audio-custom-light", Dark: "settings-audio-custom-dark", Surface: SurfaceSettings, Page: pageAudio, Size: sizeSettings, Theme: ThemeBoth, Apply: applyCustomLatency},
	{ID: "settings.audio.compact", Name: "settings-audio-compact-light", Dark: "settings-audio-compact-dark", Surface: SurfaceSettings, Page: pageAudio, Size: sizeCompact, Theme: ThemeBoth, Apply: applyCustomLatency},
	{ID: "settings.audio.compactScrolled", Name: "settings-audio-compact-scrolled-light", Dark: "settings-audio-compact-scrolled-dark", Surface: SurfaceSettings, Page: pageAudio, Size: sizeCompact, Theme: ThemeBoth, Scroll: 80, Apply: applyCustomLatency},
	{ID: "settings.audio.minimum", Name: "settings-audio-640-light", Dark: "settings-audio-640-dark", Surface: SurfaceSettings, Page: pageAudio, Size: sizeAudioMin, Theme: ThemeBoth, Apply: applyBase},
	{ID: "settings.audio.wide", Name: "settings-audio-1440-light", Surface: SurfaceSettings, Page: pageAudio, Size: sizeWide, Apply: applyBase},
	{ID: "settings.lamp.wide", Name: "settings-lamp-1440-light", Surface: SurfaceSettings, Page: pageLamp, Size: sizeWide, Apply: func(a *App) {
		a.lampPreset = "moon"
	}},
	{ID: "settings.nowPlaying.wide", Name: "settings-nowplaying-1440-light", Surface: SurfaceSettings, Page: pageNowPlaying, Size: sizeWide, Apply: applyWideNowPlaying},
	{ID: "settings.capture.long", Name: "settings-capture-long-light", Surface: SurfaceSettings, Page: pageCapture, Size: sizeSettings, Apply: func(a *App) {
		a.snap.CaptureName = "Mi Monitor (NVIDIA High Definition Audio)"
	}},
	{ID: "settings.lamp.off", Name: "settings-lamp-off-light", Dark: "settings-lamp-off-dark", Surface: SurfaceSettings, Page: pageLamp, Size: sizeSettings, Theme: ThemeBoth, Apply: applyLampOff},
	{ID: "settings.lamp.standbyOff", Name: "settings-lamp-standby-off-light", Dark: "settings-lamp-standby-off-dark", Surface: SurfaceSettings, Page: pageLamp, Size: sizeSettings, Theme: ThemeBoth, Apply: func(a *App) {
		a.standbyEnabled = false
	}},
	{ID: "settings.lamp.minimumBottom", Name: "settings-lamp-640-bottom-light", Dark: "settings-lamp-640-bottom-dark", Surface: SurfaceSettings, Page: pageLamp, Size: sizeMinimum, Theme: ThemeBoth, Scroll: 400, Apply: applyBase},
	{ID: "settings.lamp.minimumOff", Name: "settings-lamp-640-off-light", Dark: "settings-lamp-640-off-dark", Surface: SurfaceSettings, Page: pageLamp, Size: sizeMinimum, Theme: ThemeBoth, Scroll: 400, Apply: func(a *App) {
		a.standbyEnabled = false
	}},
	{ID: "settings.lamp.cover", Name: "settings-lamp-cover-light", Dark: "settings-lamp-cover-dark", Surface: SurfaceSettings, Page: pageLamp, Size: sizeSettings, Theme: ThemeBoth, Apply: applyLampCover},
	{ID: "settings.lamp.noMedia", Name: "settings-lamp-nomedia-light", Dark: "settings-lamp-nomedia-dark", Surface: SurfaceSettings, Page: pageLamp, Size: sizeSettings, Theme: ThemeBoth, Apply: applyLampNoMedia},
	{ID: "settings.update.available", Name: "settings-update-available", Surface: SurfaceSettings, Page: pageUpdate, Size: sizeSettings, Apply: applyUpdateAvailable},
	{ID: "settings.update.notes", Name: "settings-update-notes", Surface: SurfaceSettings, Page: pageUpdate, Size: sizeSettings, Apply: applyUpdateNotes},
	{ID: "settings.update.notesLatest", Name: "settings-update-notes-latest", Surface: SurfaceSettings, Page: pageUpdate, Size: sizeSettings, Apply: applyUpdateNotesLatest},
	{ID: "settings.update.downloading", Name: "settings-update-downloading", Surface: SurfaceSettings, Page: pageUpdate, Size: sizeSettings, Apply: applyUpdateDownloading},
	{ID: "settings.update.ready", Name: "settings-update-ready", Surface: SurfaceSettings, Page: pageUpdate, Size: sizeSettings, Apply: applyUpdateReady},
	{ID: "settings.update.checking", Name: "settings-update-checking", Surface: SurfaceSettings, Page: pageUpdate, Size: sizeSettings, Apply: applyUpdateChecking},
	{ID: "settings.update.upToDate", Name: "settings-update-uptodate", Surface: SurfaceSettings, Page: pageUpdate, Size: sizeSettings, Apply: applyUpdateUpToDate},
	{ID: "settings.update.failed", Name: "settings-update-failed", Surface: SurfaceSettings, Page: pageUpdate, Size: sizeSettings, Apply: applyUpdateFailed},
	{ID: "probe.settings.enter", Name: "probe-settings-enter", Surface: SurfaceSettings, Page: pageConnection, Size: sizeSettings, Frames: 3, Apply: applyBase},
}

// applyMuteActive is the connection card with local-output mute on.
func applyMuteActive(a *App) {
	a.cfg.MuteLocalOnConnect = true
	a.snap.MuteLocal = engine.MuteLocalActive
}

// applyMuteWaiting is the connection card before a receiver is connected.
func applyMuteWaiting(a *App) {
	a.cfg.MuteLocalOnConnect = true
	a.snap.MuteLocal = engine.MuteLocalWaiting
	a.snap.State = sender.StateIdle
	a.snap.ConnectedName = ""
}

func applyNowPlayingOff(a *App) {
	applyLiveCard(a)
	a.cfg.ShowNowPlayingPanel = false
}

func applyNowPlayingPlaceholder(a *App) {
	applyLiveCard(a)
	a.cfg.CoverMode = settings.CoverPlaceholder
}

func applyNowPlayingNoCover(a *App) {
	applyLiveCard(a)
	a.cfg.CoverMode = settings.CoverNone
}

func applyNowPlayingBottom(a *App) {
	applyLiveCard(a)
	a.cfg.CoverMode = settings.CoverMedia
}

// applyWideNowPlaying is the Now Playing page at the wide window, without the
// artwork probes the live card carries.
func applyWideNowPlaying(a *App) {
	a.nowSnap = sampleSnapshot(i18n.Locale())
	a.nowMediaAUMID = "AppleInc.AppleMusicWin_preview"
}

func applyReceiversExpanded(a *App) {
	applyReceiverRows(a)
	a.receiverOpen = map[string]bool{"m:ab12": true}
	a.receiverHiddenOpen = true
}

func applyReceiversHidden(a *App) {
	applyReceiverRows(a)
	a.receiverHiddenOpen = true
}

func applyReceiversLocal(a *App) {
	applyLocalReceiver(a)
	a.snap.SelectedID = ""
	a.snap.ConnectedName = ""
	a.snap.State = sender.StateIdle
	a.selID = ""
}

func applyReceiversEmpty(a *App) {
	a.snap.Receivers = nil
	a.snap.Devices = nil
	a.snap.SelectedID = ""
	a.snap.ConnectedName = ""
	a.snap.State = sender.StateIdle
	a.selID = ""
}

func applyReceiversAllHidden(a *App) {
	applyReceiverRows(a)
	a.snap.Receivers = a.snap.Receivers[1:2]
	a.snap.SelectedID = ""
	a.snap.ConnectedName = ""
	a.snap.State = sender.StateIdle
	a.selID = ""
}

func applyLampNoMedia(a *App) {
	a.lampCover = true
	a.snap.LampCover = true
}

// applyUpdateBase is the update page's available state with the release
// history loaded.
func applyUpdateBase(a *App) {
	a.cfg = settings.Default()
	a.autoUpdate = true
	a.page = pageUpdate
	a.upd = updateState{
		phase:   updateAvailable,
		version: "0.4.3",
		date:    time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC),
		notes: `### Added

- Per-app capture remembers the last application you streamed.

### Fixed

- The tray menu no longer stays checked after a device drops.
`,
	}
	a.history = []releaseNote{
		{Version: "0.4.2", Date: time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC),
			Notes: `### Changed

- The update dialog now lists every version since your last update.
`},
		{Version: "0.4.1", Date: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
			Notes: `### Fixed

- The volume readout now shows 0% to 100%.
- The settings window uses one background.
`},
	}
	a.historyState = historyReady
}

func applyUpdateAvailable(a *App) {
	applyUpdateBase(a)
}

func applyUpdateNotes(a *App) {
	applyUpdateBase(a)
	a.notesOpen = true
	a.notesRange = true
	a.notesExpanded = map[string]bool{"0.4.2": true}
}

func applyUpdateNotesLatest(a *App) {
	applyUpdateBase(a)
	a.notesOpen = true
	a.notesExpanded = map[string]bool{}
}

func applyUpdateDownloading(a *App) {
	applyUpdateBase(a)
	a.notesRange = true
	a.notesExpanded = map[string]bool{}
	a.upd.phase = updateDownloading
	a.upd.got, a.upd.total = 12_400_000, 20_100_000
}

func applyUpdateReady(a *App) {
	applyUpdateBase(a)
	a.notesRange = true
	a.notesExpanded = map[string]bool{}
	a.upd.phase = updateReady
	a.upd.got, a.upd.total = 12_400_000, 20_100_000
}

// applyUpdateChip is the flyout chip after the ready state returned to
// available.
func applyUpdateChip(a *App) {
	applyUpdateBase(a)
	a.notesRange = true
	a.notesExpanded = map[string]bool{}
	a.upd.got, a.upd.total = 12_400_000, 20_100_000
}

func applyUpdateChecking(a *App) {
	applyUpdateBase(a)
	a.notesRange = true
	a.notesExpanded = map[string]bool{}
	a.upd = updateState{phase: updateChecking}
}

func applyUpdateUpToDate(a *App) {
	applyUpdateBase(a)
	a.notesRange = true
	a.notesExpanded = map[string]bool{}
	a.upd = updateState{phase: updateIdle, checked: true}
}

func applyUpdateFailed(a *App) {
	applyUpdateBase(a)
	a.notesRange = true
	a.notesExpanded = map[string]bool{}
	a.upd = updateState{phase: updateFailed, err: "network unreachable"}
}
