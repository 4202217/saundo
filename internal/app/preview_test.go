package app

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/audio/analyze"
	"saundo/internal/engine"
	"saundo/internal/lamp"
	"saundo/internal/nowplaying"
	"saundo/internal/settings"
)

// TestRenderPreviews writes the design's states for a visual check, in both
// languages. Run with SAUNDO_PREVIEW_DIR set; it is not part of the regular
// suite.
func TestRenderPreviews(t *testing.T) {
	dir := os.Getenv("SAUNDO_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set SAUNDO_PREVIEW_DIR to write preview images")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ locale, suffix string }{
		{"en", "en"},
		{"zh-CN", "zh"},
	} {
		// The English README uses the en images and the Chinese README the zh
		// ones, so each language gets its own set.
		testLocale = tc.locale
		renderPreviews(t, dir, tc.locale, tc.suffix)
	}
	testLocale = "zh-CN"
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

func renderPreviews(t *testing.T, dir, locale, suffix string) {
	t.Helper()
	// A local build stamps git describe, the widest version the About pill
	// has to hold.
	oldVersion := Version
	Version = "0.4.2-5-g1e8e94c-dirty"
	defer func() { Version = oldVersion }()
	save := func(name string, view func(*ui.Context), w, h int, dark bool) {
		tt := ui.NewTester(view, w, h)
		tt.SetScale(2)
		tt.SetLocale(locale)
		tt.SetDark(dark)
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		f, err := os.Create(filepath.Join(dir, name+"-"+suffix+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	// saveScrolled shows the page scrolled with the pointer over it, so the
	// scrollbar and the fixed title divider are visible.
	saveScrolled := func(name string, view func(*ui.Context), w, h int, dark bool, dy float32) {
		tt := ui.NewTester(view, w, h)
		tt.SetScale(2)
		tt.SetLocale(locale)
		tt.SetDark(dark)
		tt.Advance(400 * time.Millisecond)
		tt.Move(600, 300)
		tt.Scroll(600, 300, 0, dy)
		tt.Frame()
		f, err := os.Create(filepath.Join(dir, name+"-"+suffix+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	// frames writes the first frame and two more as the motion runs, to
	// check an enter animation instead of its end state.
	frames := func(name string, view func(*ui.Context), w, h int) {
		tt := ui.NewTester(view, w, h)
		tt.SetScale(2)
		tt.SetLocale(locale)
		for i, step := range []time.Duration{0, 70 * time.Millisecond, 250 * time.Millisecond} {
			if step > 0 {
				tt.Advance(step)
			}
			f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%s-%d.png", name, suffix, i)))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}
	// fixture fills the device names that come from the environment rather
	// than the catalog, so a preview reads naturally in its language.
	fixture := func() *App {
		a := previewApp()
		a.nameDraft = locales.AppName()
		a.snap.Name = locales.AppName()
		captureName := "扬声器"
		if locale == "en" {
			captureName = "Speakers"
		}
		a.captureNames[1] = captureName
		a.snap.CaptureName = captureName
		return a
	}

	stream := fixture()
	stream.nowOK = true
	stream.nowTrack = sampleTrack(locale)
	stream.nowSnap = sampleSnapshot(locale)
	stream.nowMediaAUMID = "AppleInc.AppleMusicWin_preview"
	cover := sampleCoverImage()
	stream.nowArt = ui.NewBitmap(cover)
	stream.nowIcon = ui.NewBitmap(sampleAppIcon())
	stream.nowFieldLight = coverField(cover, false)
	stream.nowFieldDark = coverField(cover, true)
	stream.levelFrame = func(time.Duration) analyze.Frame { return sampleLevel() }

	save("flyout-stream-light", stream.view, flyoutWidth, visibleFlyoutHeight(stream), false)
	save("flyout-stream-dark", stream.view, flyoutWidth, visibleFlyoutHeight(stream), true)

	// The unknown-app, no-timeline card: the third SMTC state the mockup
	// covered. It has no artwork and no duration, so the note tile and the
	// empty progress track stand in.
	unknownTitle, unknownArtist := "触屏版 MacBook Pro", "未知来源"
	if locale == "en" {
		unknownTitle, unknownArtist = "Touchscreen MacBook Pro", "Unknown source"
	}
	unknown := fixture()
	unknown.nowSnap = nowplaying.Snapshot{
		Sessions: []nowplaying.Session{{
			AUMID: "Unknown.Player", Title: unknownTitle, Artist: unknownArtist,
			Status:  nowplaying.StatusPaused,
			CanPlay: true, CanPrev: true, CanNext: true,
		}},
		Current: 0,
	}
	unknown.nowMediaAUMID = "Unknown.Player"
	unknown.levelFrame = func(time.Duration) analyze.Frame { return sampleLevel() }
	save("flyout-nowplaying-unknown-light", unknown.view, flyoutWidth, visibleFlyoutHeight(unknown), false)
	save("flyout-nowplaying-unknown-dark", unknown.view, flyoutWidth, visibleFlyoutHeight(unknown), true)

	idle := fixture()
	idle.snap.State = sender.StateIdle
	idle.snap.ConnectedName = ""
	idle.snap.PTPActive = false
	save("flyout-idle", idle.view, flyoutWidth, visibleFlyoutHeight(idle), false)

	connecting := fixture()
	connecting.snap.State = sender.StateHandshake
	connecting.snap.ConnectedName = ""
	save("flyout-connecting", connecting.view, flyoutWidth, visibleFlyoutHeight(connecting), false)

	failed := fixture()
	failed.snap.State = sender.StateIdle
	failed.snap.Error = "The receiver did not respond."
	if locale != "en" {
		failed.snap.Error = "接收器没有响应。"
	}
	failed.snap.ConnectedName = ""
	save("flyout-error", failed.view, flyoutWidth, visibleFlyoutHeight(failed), false)

	customFly := fixture()
	customFly.latencyMode = settings.LatencyCustom
	customFly.latencyCustom = 700
	customFly.latency = 700
	customFly.snap.LatencyMS = 700
	save("flyout-custom-light", customFly.view, flyoutWidth, visibleFlyoutHeight(customFly), false)
	save("flyout-custom-dark", customFly.view, flyoutWidth, visibleFlyoutHeight(customFly), true)

	offFly := fixture()
	offFly.showLatencySwitch = false
	save("flyout-switch-off", offFly.view, flyoutWidth, visibleFlyoutHeight(offFly), false)

	applying := fixture()
	applying.latencyMode = settings.LatencyStandard
	applying.latency = 1500
	applying.latencyApplying = true
	save("flyout-latency-applying", applying.view, flyoutWidth, visibleFlyoutHeight(applying), false)
	save("flyout-latency-applying-dark", applying.view, flyoutWidth, visibleFlyoutHeight(applying), true)

	empty := fixture()
	empty.snap.Devices = nil
	empty.snap.State = sender.StateIdle
	empty.snap.SelectedID = ""
	empty.selID = ""
	save("flyout-empty", empty.view, flyoutWidth, visibleFlyoutHeight(empty), false)

	stereo := fixture()
	stereo.snap.Devices = nil
	stereo.snap.SelectedID = ""
	stereo.selID = ""
	save("flyout-no-device", stereo.view, flyoutWidth, visibleFlyoutHeight(stereo), false)

	pageNames := map[settingsPage]string{
		pageGeneral:    "settings-general",
		pageConnection: "settings-connection",
		pageReceivers:  "settings-receivers",
		pageAudio:      "settings-audio",
		pageLamp:       "settings-lamp",
		pageCapture:    "settings-capture",
		pageNowPlaying: "settings-nowplaying",
		pageAbout:      "settings-about",
	}
	pages := []settingsPage{pageGeneral, pageConnection, pageReceivers, pageAudio, pageLamp, pageCapture, pageNowPlaying, pageAbout}
	for _, page := range pages {
		a := fixture()
		a.page = page
		if page == pageReceivers {
			a.applyReceiverFixtures()
		}
		save(pageNames[page]+"-light", a.settingsView, 720, 500, false)
	}
	// The connection page with the local-output mute on: the target state the
	// mockup settled, plus the waiting state a disconnected user sees.
	muteSettings := fixture()
	muteSettings.page = pageConnection
	muteSettings.cfg.MuteLocalOnConnect = true
	muteSettings.snap.MuteLocal = engine.MuteLocalActive
	save("settings-connection-mute-light", muteSettings.settingsView, 720, 500, false)
	save("settings-connection-mute-dark", muteSettings.settingsView, 720, 500, true)

	muteWaiting := fixture()
	muteWaiting.page = pageConnection
	muteWaiting.cfg.MuteLocalOnConnect = true
	muteWaiting.snap.MuteLocal = engine.MuteLocalWaiting
	muteWaiting.snap.State = sender.StateIdle
	muteWaiting.snap.ConnectedName = ""
	save("settings-connection-mute-waiting-light", muteWaiting.settingsView, 720, 500, false)

	// The Now Playing settings page with a live card, master on and off.
	nowSettings := fixture()
	nowSettings.page = pageNowPlaying
	nowSettings.nowSnap = sampleSnapshot(locale)
	nowSettings.nowMediaAUMID = "AppleInc.AppleMusicWin_preview"
	nowCover := sampleCoverImage()
	nowSettings.nowArt = ui.NewBitmap(nowCover)
	nowSettings.nowIcon = ui.NewBitmap(sampleAppIcon())
	nowSettings.nowFieldLight = coverField(nowCover, false)
	nowSettings.nowFieldDark = coverField(nowCover, true)
	nowSettings.levelFrame = func(time.Duration) analyze.Frame { return sampleLevel() }
	save("settings-nowplaying-live-light", nowSettings.settingsView, 720, 500, false)
	save("settings-nowplaying-live-dark", nowSettings.settingsView, 720, 500, true)
	nowSettings.cfg.ShowNowPlayingPanel = false
	save("settings-nowplaying-off-light", nowSettings.settingsView, 720, 500, false)
	nowSettings.cfg.ShowNowPlayingPanel = true
	nowSettings.cfg.CoverMode = settings.CoverPlaceholder
	save("settings-nowplaying-placeholder-light", nowSettings.settingsView, 720, 500, false)
	save("settings-nowplaying-placeholder-dark", nowSettings.settingsView, 720, 500, true)
	nowSettings.cfg.CoverMode = settings.CoverNone
	save("settings-nowplaying-no-cover-light", nowSettings.settingsView, 720, 500, false)
	nowSettings.cfg.CoverMode = settings.CoverMedia
	saveScrolled("settings-nowplaying-bottom-light", nowSettings.settingsView, 720, 500, false, 500)

	// The flyout cover modes with the sample card.
	coverFly := fixture()
	coverFly.nowSnap = sampleSnapshot(locale)
	coverFly.nowMediaAUMID = "AppleInc.AppleMusicWin_preview"
	coverCover := sampleCoverImage()
	coverFly.nowArt = ui.NewBitmap(coverCover)
	coverFly.nowIcon = ui.NewBitmap(sampleAppIcon())
	coverFly.nowFieldLight = coverField(coverCover, false)
	coverFly.nowFieldDark = coverField(coverCover, true)
	coverFly.levelFrame = func(time.Duration) analyze.Frame { return sampleLevel() }
	coverFly.cfg.CoverMode = settings.CoverNone
	save("flyout-cover-none-light", coverFly.view, flyoutWidth, visibleFlyoutHeight(coverFly), false)
	coverFly.cfg.CoverMode = settings.CoverPlaceholder
	save("flyout-cover-placeholder-light", coverFly.view, flyoutWidth, visibleFlyoutHeight(coverFly), false)
	save("flyout-cover-placeholder-dark", coverFly.view, flyoutWidth, visibleFlyoutHeight(coverFly), true)

	// The flyout with the media panel switched off: no card, compact panel.
	mediaOff := fixture()
	mediaOff.cfg.ShowNowPlayingPanel = false
	mediaOff.nowSnap = sampleSnapshot(locale)
	mediaOff.nowMediaAUMID = "AppleInc.AppleMusicWin_preview"
	save("flyout-media-off-light", mediaOff.view, flyoutWidth, visibleFlyoutHeight(mediaOff), false)

	darkSettings := fixture()
	darkSettings.page = pageConnection
	save("settings-connection-dark", darkSettings.settingsView, 720, 500, true)
	darkAbout := fixture()
	darkAbout.page = pageAbout
	save("settings-about-dark", darkAbout.settingsView, 720, 500, true)

	// The receivers page: an expanded row, and the hidden group open.
	receiversPage := fixture()
	receiversPage.page = pageReceivers
	receiversPage.applyReceiverFixtures()
	receiversPage.receiverOpen = map[string]bool{"m:ab12": true}
	receiversPage.receiverHiddenOpen = true
	save("settings-receivers-expanded-light", receiversPage.settingsView, 720, 500, false)
	save("settings-receivers-expanded-dark", receiversPage.settingsView, 720, 500, true)

	receiversHidden := fixture()
	receiversHidden.page = pageReceivers
	receiversHidden.applyReceiverFixtures()
	receiversHidden.receiverHiddenOpen = true
	save("settings-receivers-hidden-light", receiversHidden.settingsView, 720, 500, false)

	receiversEmpty := fixture()
	receiversEmpty.page = pageReceivers
	receiversEmpty.snap.Receivers = nil
	receiversEmpty.snap.Devices = nil
	receiversEmpty.snap.SelectedID = ""
	receiversEmpty.snap.ConnectedName = ""
	receiversEmpty.snap.State = sender.StateIdle
	receiversEmpty.selID = ""
	save("settings-receivers-empty-light", receiversEmpty.settingsView, 720, 500, false)
	save("settings-receivers-empty-dark", receiversEmpty.settingsView, 720, 500, true)

	receiversAllHidden := receiverFixture()
	receiversAllHidden.page = pageReceivers
	receiversAllHidden.snap.Receivers = receiversAllHidden.snap.Receivers[1:2]
	receiversAllHidden.snap.SelectedID = ""
	receiversAllHidden.snap.ConnectedName = ""
	receiversAllHidden.snap.State = sender.StateIdle
	receiversAllHidden.selID = ""
	save("settings-receivers-all-hidden-light", receiversAllHidden.settingsView, 720, 500, false)
	save("settings-receivers-all-hidden-dark", receiversAllHidden.settingsView, 720, 500, true)
	receiversPage.openAddReceiver()
	save("dialog-receiver-add", receiversPage.settingsView, 720, 500, false)
	receiversPage.closeReceiverDialog()
	if v, ok := receiversPage.receiverViewFor("m:ab12"); ok {
		receiversPage.beginEditReceiver(v)
		save("dialog-receiver-edit", receiversPage.settingsView, 720, 500, false)
		receiversPage.openRemoveReceiver(v)
		save("dialog-receiver-remove", receiversPage.settingsView, 720, 500, false)
		receiversPage.closeReceiverDialog()
		receiversPage.receiverDialog = receiverDialogState{kind: receiverDialogPassword, key: v.Key, entered: false}
		receiversPage.receiverDialogOpen = true
		save("dialog-receiver-password", receiversPage.settingsView, 720, 500, false)
	}

	customSettings := fixture()
	customSettings.page = pageAudio
	customSettings.latencyMode = settings.LatencyCustom
	customSettings.latencyCustom = 700
	customSettings.latency = 700
	save("settings-audio-custom-light", customSettings.settingsView, 720, 500, false)
	save("settings-audio-custom-dark", customSettings.settingsView, 720, 500, true)
	save("settings-audio-compact-light", customSettings.settingsView, 720, 420, false)
	save("settings-audio-compact-dark", customSettings.settingsView, 720, 420, true)
	saveScrolled("settings-audio-compact-scrolled-light", customSettings.settingsView, 720, 420, false, 80)
	saveScrolled("settings-audio-compact-scrolled-dark", customSettings.settingsView, 720, 420, true, 80)
	// The minimum window keeps the three presets as rows, so the names and
	// descriptions stay inside their cards.
	narrowAudio := fixture()
	narrowAudio.page = pageAudio
	save("settings-audio-640-light", narrowAudio.settingsView, 640, 500, false)
	save("settings-audio-640-dark", narrowAudio.settingsView, 640, 500, true)

	// The three feature pages at a wide window: the content column stops at
	// 720 DIP and centers instead of stretching the cards.
	wideAudio := fixture()
	wideAudio.page = pageAudio
	save("settings-audio-1440-light", wideAudio.settingsView, 1440, 900, false)
	wideLamp := fixture()
	wideLamp.page = pageLamp
	wideLamp.lampPreset = "moon"
	save("settings-lamp-1440-light", wideLamp.settingsView, 1440, 900, false)
	wideNow := fixture()
	wideNow.page = pageNowPlaying
	wideNow.nowSnap = sampleSnapshot(locale)
	wideNow.nowMediaAUMID = "AppleInc.AppleMusicWin_preview"
	save("settings-nowplaying-1440-light", wideNow.settingsView, 1440, 900, false)
	// A long capture device name: the status gets its own line under the
	// title instead of squeezing it into an ellipsis.
	longCapture := fixture()
	longCapture.page = pageCapture
	longCapture.snap.CaptureName = "Mi Monitor (NVIDIA High Definition Audio)"
	save("settings-capture-long-light", longCapture.settingsView, 720, 500, false)

	lampOff := fixture()
	lampOff.page = pageLamp
	lampOff.lampOff = true
	lampOff.snap.LampOff = true
	save("settings-lamp-off-light", lampOff.settingsView, 720, 500, false)
	save("settings-lamp-off-dark", lampOff.settingsView, 720, 500, true)

	lampCollapsed := fixture()
	save("flyout-lamp-collapsed-light", lampCollapsed.view, flyoutWidth,
		visibleFlyoutHeight(lampCollapsed), false)
	save("flyout-lamp-collapsed-dark", lampCollapsed.view, flyoutWidth,
		visibleFlyoutHeight(lampCollapsed), true)

	lampFly := fixture()
	lampFly.lampExpanded = true
	save("flyout-lamp-expanded-light", lampFly.view, flyoutWidth,
		visibleFlyoutHeight(lampFly), false)
	save("flyout-lamp-expanded-dark", lampFly.view, flyoutWidth,
		visibleFlyoutHeight(lampFly), true)
	lampFly.lampOff = true
	lampFly.snap.LampOff = true
	lampFly.lampExpanded = false
	save("flyout-lamp-off-light", lampFly.view, flyoutWidth,
		visibleFlyoutHeight(lampFly), false)

	coverImg := sampleCoverImage()
	coverChoice := fixture()
	coverChoice.page = pageLamp
	coverChoice.lampCover = true
	coverChoice.snap.LampCover = true
	coverChoice.nowArt = ui.NewBitmap(coverImg)
	coverChoice.nowFieldLight = coverField(coverImg, false)
	coverChoice.nowFieldDark = coverField(coverImg, true)
	coverChoice.nowLamp, coverChoice.nowLampOK = lamp.CoverPreset(coverImg)
	save("settings-lamp-cover-light", coverChoice.settingsView, 720, 500, false)
	save("settings-lamp-cover-dark", coverChoice.settingsView, 720, 500, true)
	coverLampFly := fixture()
	coverLampFly.lampCover = true
	coverLampFly.snap.LampCover = true
	coverLampFly.nowArt = ui.NewBitmap(coverImg)
	coverLampFly.nowFieldLight = coverField(coverImg, false)
	coverLampFly.nowFieldDark = coverField(coverImg, true)
	coverLampFly.nowLamp, coverLampFly.nowLampOK = lamp.CoverPreset(coverImg)
	coverLampFly.lampExpanded = true
	save("flyout-lamp-cover-light", coverLampFly.view, flyoutWidth,
		visibleFlyoutHeight(coverLampFly), false)
	save("flyout-lamp-cover-dark", coverLampFly.view, flyoutWidth,
		visibleFlyoutHeight(coverLampFly), true)

	noMedia := fixture()
	noMedia.page = pageLamp
	noMedia.lampCover = true
	noMedia.snap.LampCover = true
	save("settings-lamp-nomedia-light", noMedia.settingsView, 720, 500, false)
	save("settings-lamp-nomedia-dark", noMedia.settingsView, 720, 500, true)
	noMediaFly := fixture()
	noMediaFly.lampCover = true
	noMediaFly.snap.LampCover = true
	save("flyout-lamp-nomedia-light", noMediaFly.view, flyoutWidth,
		visibleFlyoutHeight(noMediaFly), false)

	pin := fixture()
	pin.snap.PinRequired = true
	pin.snap.PinDevice = "Apple TV"
	pin.pinOpen = true
	pin.pinCode = "3939"
	save("dialog-pin", pin.view, flyoutWidth, visibleFlyoutHeight(pin), false)

	password := fixture()
	password.snap.PasswordRequired = true
	password.snap.PasswordDevice = "MacBook"
	password.passwordOpen = true
	password.passwordPrompt = "hunter2"
	save("dialog-password", password.view, flyoutWidth, visibleFlyoutHeight(password), false)

	log := fixture()
	log.showLog = true
	log.snap.LogLines = []string{
		"2026-11-01 12:00:01 saundo 0.3.0",
		"2026-11-01 12:00:02 discovery: 3 devices",
		"2026-11-01 12:00:03 connect AA:BB:CC:DD:EE:01",
		"2026-11-01 12:00:03 pair-verify ok",
		"2026-11-01 12:00:04 ptp: follower ready",
		"2026-11-01 12:00:05 capture open: PHL 27E2F7901 (NVIDIA High Definition Audio) (48000 Hz)",
	}
	save("dialog-log", log.view, flyoutWidth, visibleFlyoutHeight(log), false)

	frames("probe-flyout-enter", fixture().view, flyoutWidth, visibleFlyoutHeight(fixture()))
	pinProbe := fixture()
	pinProbe.snap.PinRequired = true
	pinProbe.snap.PinDevice = "Apple TV"
	pinProbe.pinOpen = true
	pinProbe.pinCode = "3939"
	frames("probe-pin-enter", pinProbe.view, flyoutWidth, visibleFlyoutHeight(pinProbe))
	settingsProbe := fixture()
	settingsProbe.page = pageConnection
	frames("probe-settings-enter", settingsProbe.settingsView, 720, 500)

	mac := fixture()
	mac.snap.Devices = []device.Device{{
		ID: "AA:BB:CC:DD:EE:09", Name: "MacBook", Model: "Mac",
		Auth: device.AuthHAPPassword, AirPlay2: true, Online: true,
	}}
	mac.snap.SelectedID = ""
	mac.snap.State = sender.StateIdle
	mac.snap.ConnectedName = ""
	mac.selID = ""
	save("flyout-mac", mac.view, flyoutWidth, visibleFlyoutHeight(mac), false)

	hover := fixture()
	tt := ui.NewTester(hover.view, flyoutWidth, visibleFlyoutHeight(hover))
	tt.SetScale(2)
	tt.SetLocale(locale)
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	tt.Move(200, 99)
	tt.Frame()
	f, err := os.Create(filepath.Join(dir, "flyout-hover-"+suffix+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
	f.Close()

	upd := fixture()
	upd.cfg = settings.Default()
	upd.autoUpdate = true
	upd.upd = updateState{
		phase:   updateAvailable,
		version: "0.4.3",
		date:    time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC),
		notes: `### Added

- Per-app capture remembers the last application you streamed.

### Fixed

- The tray menu no longer stays checked after a device drops.
`,
	}
	upd.history = []releaseNote{
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
	upd.historyState = historyReady
	upd.page = pageUpdate
	save("settings-update-available", upd.settingsView, 720, 500, false)
	upd.notesOpen = true
	upd.notesRange = true
	upd.notesExpanded = map[string]bool{"0.4.2": true}
	save("settings-update-notes", upd.settingsView, 720, 500, false)
	upd.notesExpanded = map[string]bool{}
	upd.notesRange = false
	save("settings-update-notes-latest", upd.settingsView, 720, 500, false)
	upd.notesRange = true
	upd.notesOpen = false
	upd.upd.phase = updateDownloading
	upd.upd.got, upd.upd.total = 12_400_000, 20_100_000
	save("settings-update-downloading", upd.settingsView, 720, 500, false)
	upd.upd.phase = updateReady
	save("settings-update-ready", upd.settingsView, 720, 500, false)
	upd.upd.phase = updateAvailable
	save("flyout-update-chip", upd.view, flyoutWidth, visibleFlyoutHeight(upd), false)

	upd.upd = updateState{phase: updateChecking}
	save("settings-update-checking", upd.settingsView, 720, 500, false)
	upd.upd = updateState{phase: updateIdle, checked: true}
	save("settings-update-uptodate", upd.settingsView, 720, 500, false)
	upd.upd = updateState{phase: updateFailed, err: "network unreachable"}
	save("settings-update-failed", upd.settingsView, 720, 500, false)
}
