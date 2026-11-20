//go:build preview

package app

// The state catalog: one row per UI state, shared by the CPU matrix, the wasm
// preview, and the platform render. The output stems stay exactly as the
// imperative matrix spelled them, so the catalog's render diffs one to one
// against the Phase 0 baseline.

import (
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"saundo/internal/airplay/sender"
	"saundo/internal/audio/analyze"
	"saundo/internal/nowplaying"
	"saundo/internal/settings"
)

// Surface is the window a scenario renders. A dialog renders inside its host
// view, which Host names.
type Surface uint8

const (
	SurfaceFlyout Surface = iota
	SurfaceSettings
	SurfaceDialog
	SurfaceTrayMenu
)

// Theme is the appearance a scenario renders. ThemeBoth writes Name as the
// light stem and Dark as the dark stem.
type Theme uint8

const (
	ThemeLight Theme = iota
	ThemeDark
	ThemeBoth
)

// Scenario is one UI state. ID is the stable join key across the matrix, the
// wasm preview, the platform render, and the site data. Name is the exact
// output stem for the light or only image; ThemeBoth also sets Dark to the
// exact dark stem. The stems are spelled out because two pairs omit the
// -light suffix, so a derived name would change the baseline.
type Scenario struct {
	ID       string
	Name     string
	Dark     string
	Surface  Surface
	Host     Surface
	Page     settingsPage
	Apply    func(*App)
	Size     func(a *App) (w, h int)
	Theme    Theme
	Scale    float32
	Scroll   float32
	Frames   int
	Move     func(*ui.Tester)
	SiteDemo bool
}

// Catalog is every state, grouped by surface.
func Catalog() []Scenario {
	all := make([]Scenario, 0, len(flyoutScenarios)+len(settingsScenarios)+len(dialogScenarios)+len(trayScenarios))
	all = append(all, flyoutScenarios...)
	all = append(all, settingsScenarios...)
	all = append(all, dialogScenarios...)
	all = append(all, trayScenarios...)
	return all
}

// PreviewScenarios is the catalog subset the wasm preview can show. The tray
// menu has its own panel, built from the same model as the native menu.
func PreviewScenarios() []Scenario {
	var out []Scenario
	for _, sc := range Catalog() {
		switch sc.Surface {
		case SurfaceFlyout, SurfaceSettings, SurfaceDialog, SurfaceTrayMenu:
			out = append(out, sc)
		}
	}
	return out
}

// view returns the one view a scenario renders. A dialog renders inside its
// host view.
func (s Scenario) view(a *App) func(*ui.Context) {
	switch s.Surface {
	case SurfaceSettings:
		return a.settingsView
	case SurfaceDialog:
		if s.Host == SurfaceSettings {
			return a.settingsView
		}
		return a.view
	case SurfaceTrayMenu:
		return a.previewMenuScene
	case SurfaceFlyout:
		return a.view
	}
	return nil
}

// usesSettingsPage reports whether Page selects a settings page for this
// scenario.
func (s Scenario) usesSettingsPage() bool {
	return s.Surface == SurfaceSettings || (s.Surface == SurfaceDialog && s.Host == SurfaceSettings)
}

// themeVariant is one image a still scenario writes.
type themeVariant struct {
	name string
	dark bool
}

// variants returns the images a still scenario writes: one per theme.
func (s Scenario) variants() []themeVariant {
	switch s.Theme {
	case ThemeDark:
		return []themeVariant{{s.Name, true}}
	case ThemeBoth:
		return []themeVariant{{s.Name, false}, {s.Dark, true}}
	}
	return []themeVariant{{s.Name, false}}
}

// anyWanted reports whether the preview filter wants any image of this
// scenario, so the driver skips the fixture work for the rest.
func (s Scenario) anyWanted(wanted func(string) bool) bool {
	if s.Frames > 0 {
		return wanted(s.Name)
	}
	for _, v := range s.variants() {
		if wanted(v.name) {
			return true
		}
	}
	return false
}

// flyoutSize is the tester size for a flyout whose height depends on the
// state.
func flyoutSize(a *App) (int, int) {
	return flyoutWidth, visibleFlyoutHeight(a)
}

// fixedSize returns a Size for a window whose dimensions do not depend on the
// state.
func fixedSize(w, h int) func(*App) (int, int) {
	return func(*App) (int, int) { return w, h }
}

// applyBase leaves the fresh fixture as it is: the catalog's default state.
func applyBase(*App) {}

// applyStreamingCard is the playing card with the sample track.
func applyStreamingCard(a *App) {
	applySampleCard(a, i18n.Locale())
	a.nowOK = true
	a.nowTrack = sampleTrack(i18n.Locale())
}

// applyUnknownSession is the card for an app with no artwork and no
// timeline, the third SMTC state the mockup covered.
func applyUnknownSession(a *App) {
	title, artist := "触屏版 MacBook Pro", "未知来源"
	if i18n.Locale() == "en" {
		title, artist = "Touchscreen MacBook Pro", "Unknown source"
	}
	a.nowSnap = nowplaying.Snapshot{
		Sessions: []nowplaying.Session{{
			AUMID: "Unknown.Player", Title: title, Artist: artist,
			Status:  nowplaying.StatusPaused,
			CanPlay: true, CanPrev: true, CanNext: true,
		}},
		Current: 0,
	}
	a.nowMediaAUMID = "Unknown.Player"
	a.levelFrame = func(time.Duration) analyze.Frame { return sampleLevel() }
}

// applyIdle is a disconnected receiver with no error.
func applyIdle(a *App) {
	a.snap.State = sender.StateIdle
	a.snap.ConnectedName = ""
	a.snap.PTPActive = false
}

// applyTakenOver is a receiver that another sender claimed.
func applyTakenOver(a *App) {
	a.snap.State = sender.StateIdle
	a.snap.TakenOver = true
	a.snap.ConnectedName = ""
}

// applyCustomLatency is a 700 ms custom latency, the value both the flyout and
// the audio page show.
func applyCustomLatency(a *App) {
	a.latencyMode = settings.LatencyCustom
	a.latencyCustom = 700
	a.latency = 700
	a.snap.LatencyMS = 700
}

// applyLatencyApplying is the moment after the standard preset was chosen and
// the receiver has not acknowledged it yet.
func applyLatencyApplying(a *App) {
	a.latencyMode = settings.LatencyStandard
	a.latency = 1500
	a.latencyApplying = true
}

// applyLampOff is the light switched off by the user.
func applyLampOff(a *App) {
	a.lampOff = true
	a.snap.LampOff = true
}

// applyLampCover is cover-driven lamp mode with the sample artwork.
func applyLampCover(a *App) {
	a.lampCover = true
	a.snap.LampCover = true
	applySampleCover(a, i18n.Locale())
}

// applyLiveCard is the settings page with a playing card and its artwork.
func applyLiveCard(a *App) {
	applySampleCard(a, i18n.Locale())
}
