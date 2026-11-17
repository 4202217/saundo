//go:build preview

package app

import (
	"image"
	"image/color"
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/internal/audio/analyze"
	"saundo/internal/lamp"
	"saundo/internal/nowplaying"
)

// previewMedia fills the playback card with a fictional session, so the
// preview shows the media path without a real listener's data.
func previewMedia(a *App) {
	cover := previewCoverImage()
	a.nowOK = true
	a.nowTrack = previewTrack(previewLocale)
	a.nowSnap = previewSnapshot(previewLocale)
	a.nowMediaAUMID = "AppleInc.AppleMusicWin_preview"
	a.nowArt = ui.NewBitmap(cover)
	a.nowLamp, a.nowLampOK = lamp.CoverPreset(cover)
	a.nowIcon = ui.NewBitmap(previewAppIcon())
	a.nowFieldLight = coverField(cover, false)
	a.nowFieldDark = coverField(cover, true)
	a.levelFrame = func(time.Duration) analyze.Frame { return previewLevel() }
}

func previewTrack(locale string) nowplaying.Track {
	if locale == "en" {
		return nowplaying.Track{Title: "Night Ferry", Artist: "Signal Coast", Album: "Open Water"}
	}
	return nowplaying.Track{Title: "夜航", Artist: "白鲸电台", Album: "远洋信号"}
}

func previewSnapshot(locale string) nowplaying.Snapshot {
	tr := previewTrack(locale)
	return nowplaying.Snapshot{
		Sessions: []nowplaying.Session{
			{
				AUMID: "AppleInc.AppleMusicWin_preview", AppName: "Apple Music",
				Title: tr.Title, Artist: tr.Artist, Album: tr.Album,
				Status: nowplaying.StatusPlaying, Playing: true,
				CanPlay: true, CanPause: true, CanNext: true, CanPrev: true,
				Position: 128 * time.Second, Duration: 244 * time.Second,
			},
			{
				AUMID: "Chrome.UserData.preview", AppName: "Chrome",
				Title: "Open Water Sessions", Artist: "Signal Coast",
				Status:  nowplaying.StatusPaused,
				CanPlay: true, CanSeek: true,
				Position: 416 * time.Second, Duration: 899 * time.Second,
			},
		},
		Current: 0,
	}
}

func previewCoverImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(60 + x), G: uint8(90 + y/2), B: uint8(150 + x/3), A: 255})
		}
	}
	return img
}

func previewAppIcon() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			dx, dy := float64(x)-15.5, float64(y)-15.5
			if dx*dx+dy*dy > 15.5*15.5 {
				continue
			}
			t := float64(x+y) / 62
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(91 + (54-91)*t),
				G: uint8(108 + (197-108)*t),
				B: uint8(255 + (176-255)*t),
				A: 255,
			})
		}
	}
	return img
}

func previewLevel() analyze.Frame {
	return analyze.Frame{Bands: [analyze.BandCount]float32{0.25, 0.6, 0.85, 0.55, 0.35}}
}
