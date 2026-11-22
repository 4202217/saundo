package app

import (
	"image"
	"image/color"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/audio/analyze"
	"saundo/internal/nowplaying"
)

// nowPlayingPreview is the settings page's card. It shows the live session,
// or a sample card when nothing plays, so the preview never disappears. The
// display switch only hides the flyout card; the preview stays live.
func (a *App) nowPlayingPreview(c *ui.Context, p palette) {
	s, ok := a.nowSession()
	var art *ui.Bitmap
	var light, dark *image.RGBA
	level := func() analyze.Frame { return analyze.Frame{} }
	if ok {
		art, light, dark = a.nowArt, a.nowFieldLight, a.nowFieldDark
		delay := time.Duration(a.latencyMS()) * time.Millisecond
		level = func() analyze.Frame {
			if a.levelFrame != nil {
				return a.levelFrame(delay)
			}
			return analyze.Frame{}
		}
	} else {
		s, art, light, dark, level = a.sampleCard()
	}
	field := light
	if p.dark {
		field = dark
	}
	a.nowPlayingCardBody(c, p, s, true, previewMeterAnimates(ok), art, field, level, true)
}

// sampleCard returns the settings preview's sample session, made once per
// language. The view rebuilds every frame, so the bitmap and the two color
// fields must outlive it.
func (a *App) sampleCard() (nowplaying.Session, *ui.Bitmap, *image.RGBA, *image.RGBA, func() analyze.Frame) {
	locale := i18n.Locale()
	if !a.sampleReady || a.sampleLocale != locale {
		a.sampleSession, a.sampleArt, a.sampleFieldLight, a.sampleFieldDark, a.sampleLevel = previewSession()
		a.sampleLocale, a.sampleReady = locale, true
	}
	return a.sampleSession, a.sampleArt, a.sampleFieldLight, a.sampleFieldDark, a.sampleLevel
}

// previewSession is the settings page's sample card when nothing plays.
func previewSession() (nowplaying.Session, *ui.Bitmap, *image.RGBA, *image.RGBA, func() analyze.Frame) {
	sample := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			t := float64(x+y) / 126
			sample.SetRGBA(x, y, color.RGBA{
				R: uint8(135 + (47-135)*t),
				G: uint8(121 + (142-121)*t),
				B: uint8(221 + (154-221)*t),
				A: 255,
			})
		}
	}
	s := nowplaying.Session{
		AUMID: "settings.preview", AppName: locales.SettingsNowPlayingSampleApp(),
		Title:  locales.SettingsNowPlayingSampleTitle(),
		Artist: locales.SettingsNowPlayingSampleArtist(),
		Status: nowplaying.StatusPlaying, Playing: true,
		CanPlay: true, CanPause: true, CanPrev: true, CanNext: true,
		Position: 128 * time.Second, Duration: 244 * time.Second,
	}
	frame := analyze.Frame{Bands: [analyze.BandCount]float32{0.35, 0.6, 0.85, 0.55, 0.4}}
	return s, ui.NewBitmap(sample), coverField(sample, false), coverField(sample, true), func() analyze.Frame { return frame }
}
