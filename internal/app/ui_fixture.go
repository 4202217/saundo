package app

// One shared UI fixture: the base App, the receiver rows, and the sample
// media the preview matrix, the wasm preview, and the untagged tests render.
// It carries no build tag so the untagged tests keep working. The unexported
// entries are unreachable from a production build, so the linker drops them.

import (
	"image"
	"image/color"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/audio/analyze"
	"saundo/internal/engine"
	"saundo/internal/lamp"
	"saundo/internal/nowplaying"
	"saundo/internal/receivers"
	"saundo/internal/settings"
)

// visibleFlyoutHeight is the height a tester should use for this App's
// current devices, latency row, and media card.
func visibleFlyoutHeight(a *App) int {
	return flyoutHeightForState(len(a.snap.Devices), a.showLatencySwitch, a.hasNowMedia(), a.lampCover, a.lampExpanded, a.errorBarShown(), a.snap.CredentialReset)
}

// previewApp is the base fixture the untagged tests and the preview matrix
// build on.
func previewApp() *App {
	a := &App{
		cfg:               settings.Default(),
		volume:            65,
		captureNames:      []string{locales.CaptureFollowSystem(), "Speakers"},
		captureIDs:        []string{"", "{endpoint}"},
		captureLabels:     []string{locales.CaptureFollowSystem(), "Speakers"},
		captureChoice:     locales.CaptureFollowSystem(),
		appNames:          []string{locales.CaptureAllSystem(), "Spotify", "Music"},
		appPIDs:           []uint32{0, 1234, 5678},
		appIDs:            []string{"", "com.spotify.client", "com.apple.Music"},
		appNameByID:       map[string]string{"com.spotify.client": "Spotify", "com.apple.Music": "Music"},
		nameDraft:         "声渡",
		latency:           200,
		latencyMode:       "low",
		latencyCustom:     700,
		showLatencySwitch: true,
		lampOff:           false,
		lampPreset:        "dune",
		standbyEnabled:    true,
		standbyMinutes:    30,
		setThemeSource:    mygo.Theme.SetSource,
	}
	devices := []device.Device{
		{ID: "AA:BB:CC:DD:EE:01", Name: "HomePods", Host: "192.168.1.21", Port: 7000, Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true, Online: true},
		{ID: "AA:BB:CC:DD:EE:02", Name: "Apple TV", Host: "192.168.1.22", Port: 7000, Model: "AppleTV6,2", Auth: device.AuthHAPPIN, AirPlay2: true, Online: true},
		{ID: "AA:BB:CC:DD:EE:03", Name: "HomePod mini", Host: "192.168.1.23", Port: 7000, Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true},
	}
	a.selID = devices[0].ID
	a.snap = engine.Snapshot{
		Devices: devices,
		// The receivers settings page reads this list, while the flyout reads
		// Devices, so the fixture fills both from the same rows.
		Receivers: []receivers.View{
			{Key: devices[0].ID, Name: devices[0].Name, Kind: receivers.KindDiscovered, Model: devices[0].Model, Online: true, AutoConnect: true,
				Device: devices[0],
				Entry:  receivers.Entry{Key: devices[0].ID, Kind: receivers.KindDiscovered, Name: devices[0].Name, Host: devices[0].Host, Port: devices[0].Port, Model: devices[0].Model, AirPlay2: true, AutoConnect: true}},
			{Key: devices[1].ID, Name: devices[1].Name, Kind: receivers.KindDiscovered, Model: devices[1].Model, Online: true, DelayMS: 120,
				Device: devices[1],
				Entry:  receivers.Entry{Key: devices[1].ID, Kind: receivers.KindDiscovered, Name: devices[1].Name, Host: devices[1].Host, Port: devices[1].Port, Model: devices[1].Model, AirPlay2: true, DelayMS: 120}},
			{Key: devices[2].ID, Name: devices[2].Name, Kind: receivers.KindDiscovered, Model: devices[2].Model,
				Device: devices[2],
				Entry:  receivers.Entry{Key: devices[2].ID, Kind: receivers.KindDiscovered, Name: devices[2].Name, Host: devices[2].Host, Port: devices[2].Port, Model: devices[2].Model, AirPlay2: true}},
		},
		SelectedID:        devices[0].ID,
		State:             sender.StateStreaming,
		ConnectedName:     devices[0].Name,
		Volume:            65,
		CaptureName:       "Speakers",
		CaptureRate:       48000,
		CaptureRunning:    true,
		Name:              "声渡",
		ReceiverLatencyMS: 70,
		DroppedSamples:    0,
		PTPActive:         true,
		PTPReady:          true,
		Reconnect:         true,
		LampOff:           false,
		LampPreset:        "dune",
		StandbyEnabled:    true,
		StandbyMinutes:    30,
	}
	a.reconnect = true
	return a
}

// Fixture returns the base fixture with the locale's app name and capture
// name, so a rendered state reads naturally in its language. The preview
// matrix and the wasm preview use it; the untagged tests keep previewApp.
func Fixture(locale string) *App {
	a := previewApp()
	a.nameDraft = locales.In(locale).AppName()
	a.snap.Name = a.nameDraft
	captureName := "扬声器"
	if locale == "en" {
		captureName = "Speakers"
	}
	a.captureNames[1] = captureName
	a.captureLabels[1] = captureName
	a.snap.CaptureName = captureName
	return a
}

// applyReceiverRows swaps the base fixture's device list for the receivers
// page's sample rows: one visible remembered HomePod, one hidden HomePod
// mini, one online manual AirPort Express, and one stereo group.
func applyReceiverRows(a *App) {
	a.snap.SelectedID = "AA:00"
	a.snap.ConnectedName = "客厅 HomePod"
	a.selID = "AA:00"
	a.snap.Devices = []device.Device{
		{ID: "AA:00", Name: "客厅 HomePod", Host: "192.168.1.20", Port: 7000, Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true, Online: true},
	}
	a.snap.Receivers = []receivers.View{
		{
			Key: "AA:00", Name: "客厅 HomePod", Kind: receivers.KindDiscovered,
			Model: "AudioAccessory5,1", Online: true, AutoConnect: true, DelayMS: 0,
			Device: a.snap.Devices[0],
			Entry:  receivers.Entry{Key: "AA:00", Kind: receivers.KindDiscovered, Name: "客厅 HomePod", AutoConnect: true},
		},
		{
			Key: "AA:01", Name: "卧室 HomePod mini", Kind: receivers.KindDiscovered,
			Model: "AudioAccessory5,1", Hidden: true, DelayMS: 120,
			Device: device.Device{ID: "AA:01", Name: "卧室 HomePod mini", Model: "AudioAccessory5,1"},
			Entry:  receivers.Entry{Key: "AA:01", Kind: receivers.KindDiscovered, Name: "卧室 HomePod mini", Hidden: true, DelayMS: 120},
		},
		{
			Key: "m:ab12", Name: "书房 AirPort Express", Kind: receivers.KindManual,
			Type: receivers.TypeAirPortExpress, Address: "192.168.1.32:5000",
			Model: "AirPort", Online: true, PasswordSet: true,
			Device: device.Device{ID: "m:ab12", Name: "书房 AirPort Express", Model: "AirPort", Auth: device.AuthSetup, Online: true},
			Entry:  receivers.Entry{Key: "m:ab12", Kind: receivers.KindManual, Type: receivers.TypeAirPortExpress, Name: "书房 AirPort Express", Host: "192.168.1.32", Port: 5000},
		},
		{
			Key: "stereo:1", Name: "客厅立体声", Kind: receivers.KindDiscovered,
			Model: "AudioAccessory5,1", Online: true, Stereo: true,
			Device: device.Device{ID: "stereo:1", Name: "客厅立体声", Model: "AudioAccessory5,1", StereoID: "1",
				Members: []device.Device{{ID: "AA:10", Name: "L"}, {ID: "AA:11", Name: "R"}}, Online: true},
			Entry: receivers.Entry{Key: "stereo:1", Kind: receivers.KindDiscovered, Name: "客厅立体声"},
		},
	}
}

// applyLocalReceiver is the receivers page with one receiver: this host's own
// AirPlay endpoint, already forced into the hidden group.
func applyLocalReceiver(a *App) {
	a.page = pageReceivers
	a.receiverHiddenOpen = true
	const key = "02:8F:B8:25:3A:B2"
	a.snap.Receivers = []receivers.View{{
		Key: key, Name: "MacBook Air", Kind: receivers.KindDiscovered,
		Model: "Mac14,2", Online: true, Hidden: true, Local: true,
		Device: device.Device{ID: key, Name: "MacBook Air", Host: "192.168.31.200", Port: 7000,
			Model: "Mac14,2", Online: true, Local: true},
		Entry: receivers.Entry{Key: key, Kind: receivers.KindDiscovered, Name: "MacBook Air", Hidden: true},
	}}
	a.snap.Devices = nil
}

// sampleTrack is a fictional track, so no real listening shows in a preview.
func sampleTrack(locale string) nowplaying.Track {
	if locale == "en" {
		return nowplaying.Track{Title: "Night Ferry", Artist: "Signal Coast", Album: "Open Water"}
	}
	return nowplaying.Track{Title: "夜航", Artist: "白鲸电台", Album: "远洋信号"}
}

// sampleCoverImage is a generated gradient, so a preview shows the cover path
// without embedding a real listener's artwork.
func sampleCoverImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(60 + x), G: uint8(90 + y/2), B: uint8(150 + x/3), A: 255})
		}
	}
	return img
}

// sampleAppIcon is a generated rounded icon, so the preview shows the app
// icon path without borrowing a real app's mark.
func sampleAppIcon() image.Image {
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

// sampleLevel is a fixed meter frame: previews must not depend on the
// machine's audio or on the capture engine.
func sampleLevel() analyze.Frame {
	return analyze.Frame{Bands: [analyze.BandCount]float32{0.25, 0.6, 0.85, 0.55, 0.35}}
}

// sampleSnapshot is the card fixture: one playing session and a second one
// behind the switch arrow. The artwork stays empty, so the cover falls back to
// the note tile instead of showing a real listener's media.
func sampleSnapshot(locale string) nowplaying.Snapshot {
	tr := sampleTrack(locale)
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

// applySampleCard gives a the playing card fixture: the sample snapshot, its
// artwork, and the meter frame. The lamp carries the same cover's glow but
// the mode stays whatever the scenario chose.
func applySampleCard(a *App, locale string) {
	a.nowSnap = sampleSnapshot(locale)
	a.nowMediaAUMID = "AppleInc.AppleMusicWin_preview"
	cover := sampleCoverImage()
	a.nowArt = ui.NewBitmap(cover)
	a.nowIcon = ui.NewBitmap(sampleAppIcon())
	a.nowFieldLight = coverField(cover, false)
	a.nowFieldDark = coverField(cover, true)
	a.nowLamp, a.nowLampOK = lamp.CoverPreset(cover)
	a.levelFrame = func(time.Duration) analyze.Frame { return sampleLevel() }
}

// applySampleCover gives a the sample artwork and the lamp preset made from
// it, without a playing session.
func applySampleCover(a *App) {
	cover := sampleCoverImage()
	a.nowArt = ui.NewBitmap(cover)
	a.nowFieldLight = coverField(cover, false)
	a.nowFieldDark = coverField(cover, true)
	a.nowLamp, a.nowLampOK = lamp.CoverPreset(cover)
}
