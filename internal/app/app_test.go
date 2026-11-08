package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/engine"
	"saundo/internal/nowplaying"
	"saundo/internal/settings"
)

// testLocale makes the generated accessors deterministic; the UI follows
// i18n.Locale, which TestMain wires to this variable.
var testLocale = "zh-CN"

func TestMain(m *testing.M) {
	mygo.Use(native.Plugin)
	i18n.SetLocaleFunc(func() string { return testLocale })
	os.Exit(m.Run())
}

// visibleFlyoutHeight is the height a tester should use for this App's
// current devices, latency row, and media card.
func visibleFlyoutHeight(a *App) int {
	return flyoutHeightForState(len(a.snap.Devices), a.showLatencySwitch, a.hasNowMedia(), a.lampCover, a.lampExpanded, a.errorBarShown())
}

func withLocale(t *testing.T, tag string) {
	t.Helper()
	old := testLocale
	testLocale = tag
	t.Cleanup(func() { testLocale = old })
}

func previewApp() *App {
	a := &App{
		cfg:               settings.Default(),
		volume:            65,
		captureNames:      []string{locales.CaptureFollowSystem(), "Speakers"},
		captureIDs:        []string{"", "{endpoint}"},
		captureChoice:     locales.CaptureFollowSystem(),
		appNames:          []string{locales.CaptureAllSystem(), "Spotify"},
		appIDs:            []string{"0", "1234"},
		appChoice:         locales.CaptureAllSystem(),
		nameDraft:         "声渡",
		latency:           200,
		latencyMode:       "low",
		latencyCustom:     700,
		showLatencySwitch: true,
		lampOff:           false,
		lampPreset:        "dune",
	}
	a.selID = "AA:BB:CC:DD:EE:01"
	a.snap = engine.Snapshot{
		Devices: []device.Device{
			{ID: "AA:BB:CC:DD:EE:01", Name: "HomePods", Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true, Online: true},
			{ID: "AA:BB:CC:DD:EE:02", Name: "Apple TV", Model: "AppleTV6,2", Auth: device.AuthHAPPIN, AirPlay2: true, Online: true},
			{ID: "AA:BB:CC:DD:EE:03", Name: "HomePod mini", Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true},
		},
		SelectedID:     "AA:BB:CC:DD:EE:01",
		State:          sender.StateStreaming,
		ConnectedName:  "HomePods",
		Volume:         65,
		CaptureName:    "Speakers",
		CaptureRate:    48000,
		CaptureRunning: true,
		Name:           "声渡",
		DroppedSamples: 0,
		PTPActive:      true,
		PTPReady:       true,
		Reconnect:      true,
		LampOff:        false,
		LampPreset:     "dune",
	}
	a.reconnect = true
	return a
}

func TestViewRendersDevicesAndStatus(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	for _, want := range []string{
		"声渡", "AIRPLAY 2", "接收器", "HomePods", "Apple TV", "HomePod mini",
		"连接", "断开连接", "离线", "音量", "65%", "正在通过 HomePods 播放",
		"ALAC · 44.1 kHz · PTP · 已同步", "延迟 200 ms", "低延迟", "标准", "稳定", "自定义", "诊断日志",
		"氛围灯", "沙丘",
	} {
		if !tt.HasText(want) {
			t.Fatalf("view is missing %q; texts: %q", want, tt.Texts())
		}
	}
	if tt.HasText("6,500%") {
		t.Fatalf("volume was scaled by 100: %q", tt.Texts())
	}
	if path := os.Getenv("SAUNDO_PREVIEW"); path != "" {
		writePreview(t, tt, path)
	}
}

func TestEnglishLocaleRenders(t *testing.T) {
	withLocale(t, "en")
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	for _, want := range []string{
		"saundo", "AIRPLAY 2", "Receivers", "3 receivers", "65%", "Streaming to HomePods",
		"Diagnostics",
	} {
		if !tt.HasText(want) {
			t.Fatalf("English view is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestPseudoLocaleRenders(t *testing.T) {
	withLocale(t, "en-XA")
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	if len(tt.Texts()) == 0 {
		t.Fatal("pseudo-locale view rendered no text")
	}
}

func TestLanguageLabel(t *testing.T) {
	a := previewApp()
	for tag, want := range map[string]string{
		"":      locales.LanguageSystem(),
		"en":    locales.LanguageEnglish(),
		"zh-CN": locales.LanguageChinese(),
	} {
		a.cfg.Language = tag
		if got := a.languageLabel(); got != want {
			t.Errorf("languageLabel(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestLocaleChangeRelabelsSentinels(t *testing.T) {
	a := previewApp()
	withLocale(t, "en")
	a.onLocaleChanged()
	if got := a.captureNames[0]; got != "Follow system default" {
		t.Fatalf("capture sentinel = %q", got)
	}
	if got := a.appNames[0]; got != "Entire system" {
		t.Fatalf("app sentinel = %q", got)
	}
	if got := a.captureChoice; got != "Follow system default" {
		t.Fatalf("capture choice = %q", got)
	}
}

func TestPasswordPromptRenders(t *testing.T) {
	a := previewApp()
	a.snap.PasswordRequired = true
	a.snap.PasswordDevice = "MacBook"
	a.snap.PasswordDeviceID = "AA:BB:CC:DD:EE:02"
	a.passwordOpen = true
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	for _, want := range []string{
		"输入接收器密码", "输入 MacBook 的 AirPlay 密码。",
		"密码保存在本机，只用于这台接收器。", "连接", "取消",
	} {
		if !tt.HasText(want) {
			t.Fatalf("password prompt is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestSettingsPagesRender(t *testing.T) {
	cases := []struct {
		page settingsPage
		want []string
	}{
		{pageGeneral, []string{"通用", "发送端身份、启动与外观。", "发送端名称", "保存", "登录时自动启动", "托盘动画", "语言", "外观"}},
		{pageConnection, []string{"连接", "重连与验证策略。", "HomePods", "已连接", "断开后自动重连", "严格验证接收器", "连接后静音本机输出", "避免声音在本机与接收器上重复播放；断开或退出时恢复。"}},
		{pageReceivers, []string{"接收器", "管理接收器、自动连接、显示与每台接收器的延迟补偿。", "网络上的 AirPlay 接收器会自动出现在这里。", "添加接收器", "手动填写 IP 或主机名"}},
		{pageAudio, []string{"音频", "在延迟与稳定性之间权衡", "延迟模式", "托盘面板中显示", "低延迟", "标准", "稳定", "自定义", "200 ms", "1.5 s", "2 s", "700 ms", "音频格式", "ALAC · 44.1 kHz · 立体声", "同步方式", "PTP · 已同步"}},
		{pageLamp, []string{"氛围灯", "所有 HomePod 顶屏显示同一盏灯。", "HomePod 专属", "颜色来源", "跟随媒体封面，或使用固定模板。", "封面", "模板", "六组预设灯光颜色，点击切换。", "暖烬", "沙丘", "苔庭", "潮汐", "兰夜", "月白"}},
		{pageCapture, []string{"采集", "系统声音", "按应用采集", "采集状态", "Speakers · 48 kHz"}},
		{pageNowPlaying, []string{"正在播放", "托盘面板中的媒体卡片、封面配色与音频电平。", "媒体卡片", "托盘面板中显示", "封面", "无", "默认", "媒体", "播放控制", "封面配色", "音频电平", "Apple Music", "夜航"}},
		{pageUpdate, []string{"更新", "版本、自动检查与安装。", "当前版本", "检查更新", "自动检查更新", "自动下载并安装", "更新通道", "稳定版", "测试版"}},
		{pageAbout, []string{"关于", "版本、项目链接与许可。", "声渡 · saundo", "开发版", "项目主页", "版本发布", "问题反馈", "许可证", "第三方声明"}},
	}
	for _, tc := range cases {
		a := previewApp()
		a.page = tc.page
		tt := ui.NewTester(a.settingsView, 720, 500)
		for _, want := range tc.want {
			if !tt.HasText(want) {
				t.Fatalf("settings page %d is missing %q; texts: %q", tc.page, want, tt.Texts())
			}
		}
		if path := os.Getenv("SAUNDO_PREVIEW_SETTINGS"); path != "" {
			writePreview(t, tt, path)
		}
	}
}

func TestConnectionPageMuteLocalStates(t *testing.T) {
	a := previewApp()
	a.page = pageConnection
	a.cfg.MuteLocalOnConnect = true
	a.snap.MuteLocal = engine.MuteLocalWaiting

	tt := ui.NewTester(a.settingsView, 720, 500)
	for _, want := range []string{
		locales.SettingsMuteLocal(),
		locales.SettingsMuteLocalDescWaiting(),
		locales.SettingsMuteLocalNote(),
	} {
		if !tt.HasText(want) {
			t.Fatalf("waiting mute state is missing %q; texts: %q", want, tt.Texts())
		}
	}

	a.snap.MuteLocal = engine.MuteLocalActive
	tt = ui.NewTester(a.settingsView, 720, 500)
	if !tt.HasText(locales.SettingsMuteLocalDescOn()) {
		t.Fatalf("active mute state is missing its subtitle; texts: %q", tt.Texts())
	}

	a.cfg.MuteLocalOnConnect = false
	a.snap.MuteLocal = engine.MuteLocalOff
	tt = ui.NewTester(a.settingsView, 720, 500)
	if tt.HasText(locales.SettingsMuteLocalNote()) {
		t.Fatalf("the driver note stayed visible with the mute off; texts: %q", tt.Texts())
	}
}

func TestMuteLocalSettingPersists(t *testing.T) {
	sets := settings.NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	cfg := settings.Default()
	if err := sets.Save(cfg); err != nil {
		t.Fatal(err)
	}
	a := &App{sets: sets, cfg: cfg}

	a.setMuteLocal(true)
	if !sets.Load().MuteLocalOnConnect {
		t.Fatal("turning the mute on did not persist")
	}
	a.setMuteLocal(false)
	if sets.Load().MuteLocalOnConnect {
		t.Fatal("turning the mute off did not persist")
	}
}

func TestAboutVersionText(t *testing.T) {
	stable := locales.SettingsChannelStable()
	beta := locales.SettingsChannelBeta()
	cases := []struct{ version, channel, want string }{
		{"0.4.2", stable, "版本 0.4.2 · 稳定版"},
		{"v0.4.2", beta, "版本 0.4.2 · 测试版"},
		{"0.4.2-5-g1e8e94c", stable, "版本 0.4.2 · 本地构建"},
		{"0.4.2-5-g1e8e94c-dirty", stable, "版本 0.4.2 · 本地构建"},
		{"0.5.0-beta.1", beta, "版本 0.5.0-beta.1 · 测试版"},
		{"dev", stable, "开发版"},
		{"", stable, "开发版"},
	}
	for _, tc := range cases {
		if got := aboutVersionText(tc.version, tc.channel); got != tc.want {
			t.Errorf("aboutVersionText(%q, %q) = %q, want %q", tc.version, tc.channel, got, tc.want)
		}
	}
}

func TestAboutPageShowsCleanVersion(t *testing.T) {
	old := Version
	Version = "0.4.2-5-g1e8e94c-dirty"
	t.Cleanup(func() { Version = old })
	a := previewApp()
	a.page = pageAbout
	tt := ui.NewTester(a.settingsView, 720, 500)
	if !tt.HasText("版本 0.4.2 · 本地构建") {
		t.Fatalf("about page is missing the clean version line; texts: %q", tt.Texts())
	}
	if tt.HasText("0.4.2-5-g1e8e94c-dirty") {
		t.Fatalf("about page still shows the git describe stamp; texts: %q", tt.Texts())
	}
}

func TestAboutPageOpensProjectLinks(t *testing.T) {
	a := previewApp()
	a.page = pageAbout
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	for _, label := range []string{"项目主页", "版本发布", "问题反馈", "许可证", "第三方声明"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	want := []string{aboutRepoURL, aboutReleasesURL, aboutIssuesURL, aboutLicenseURL, aboutNoticesURL}
	if got := tt.OpenedURLs(); !slices.Equal(got, want) {
		t.Fatalf("opened %q, want %q", got, want)
	}
}

func TestFlyoutKeepsPlaybackControlsOnly(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	for _, moved := range []string{
		locales.SettingsLaunchAtStart(), locales.ReceiverAutoToggle(),
		locales.SettingsReconnect(), locales.SettingsStrictAuth(),
		locales.ReceiverPasswordTitle(), locales.SettingsSystemOutput(),
	} {
		if tt.HasText(moved) {
			t.Fatalf("flyout still shows configuration %q; texts: %q", moved, tt.Texts())
		}
	}
}

func TestStereoGroupRowRenders(t *testing.T) {
	a := previewApp()
	a.snap.Devices = []device.Device{{
		ID: device.StereoGroupID("tsid"), Name: "HomePods", Model: "AudioAccessory5,1",
		StereoID: "tsid", Online: true,
		Members: []device.Device{
			{ID: "AA:BB:CC:DD:EE:01", Name: "HomePod L", Online: true},
			{ID: "AA:BB:CC:DD:EE:02", Name: "HomePod R", Online: true},
		},
	}}
	a.snap.SelectedID = ""
	a.snap.State = sender.StateIdle
	a.snap.ConnectedName = ""
	a.selID = device.StereoGroupID("tsid")
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	for _, want := range []string{"HomePods", "立体声组 2/2"} {
		if !tt.HasText(want) {
			t.Fatalf("group view is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestNowPlayingCardShowsTrack(t *testing.T) {
	a := previewApp()
	a.nowOK = true
	a.nowTrack = nowplaying.Track{Title: "夜曲", Artist: "周杰伦", Album: "十一月的萧邦"}
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	for _, want := range []string{"夜曲", "周杰伦", "未知应用"} {
		if !tt.HasText(want) {
			t.Fatalf("now-playing card is missing %q; texts: %q", want, tt.Texts())
		}
	}
	if tt.HasText("已暂停 · 无进度信息") {
		t.Fatalf("now-playing card still shows the no-timeline copy; texts: %q", tt.Texts())
	}
	if tt.HasText("推送") {
		t.Fatalf("now-playing card still shows the push toggle; texts: %q", tt.Texts())
	}
}

func TestNowPlayingCardHidesWithoutMedia(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	for _, gone := range []string{
		locales.FlyoutNowPlayingNoMedia(),
		locales.FlyoutNowPlayingSession(),
	} {
		if tt.HasText(gone) {
			t.Fatalf("empty now-playing card still shows %q; texts: %q", gone, tt.Texts())
		}
	}
}

func TestNowPlayingCardControlsSendActions(t *testing.T) {
	a := previewApp()
	a.nowSnap = nowplaying.Snapshot{
		Sessions: []nowplaying.Session{{
			AUMID: "test.session", AppName: "Test", Title: "夜曲", Artist: "周杰伦",
			Status: nowplaying.StatusPlaying, Playing: true,
			CanPlay: true, CanPause: true, CanPrev: true, CanNext: true,
		}},
		Current: 0,
	}
	a.nowMediaAUMID = "test.session"
	var actions []nowplaying.Action
	a.controlNow = func(aumid string, action nowplaying.Action) {
		if aumid != "test.session" {
			t.Errorf("control sent to %q, want test.session", aumid)
		}
		actions = append(actions, action)
	}

	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	pause, ok := tt.Find(locales.FlyoutPause())
	if !ok {
		t.Fatalf("pause control missing; texts: %q", tt.Texts())
	}
	tt.ClickAt(pause.X+pause.W/2, pause.Y+pause.H/2)
	if len(actions) != 1 || actions[0] != nowplaying.ActionPause {
		t.Fatalf("progress-row controls sent %v, want one pause", actions)
	}
}

func TestNowPlayingPlaceholderStandsOutFromTheCard(t *testing.T) {
	a := previewApp()
	a.nowSnap = nowplaying.Snapshot{
		Sessions: []nowplaying.Session{{
			AUMID: "loading.session", AppName: "Apple Music", Title: "正在连接…", Artist: "系统媒体会话",
			Status: nowplaying.StatusPlaying, Playing: true,
			CanPlay: true, CanPause: true, CanPrev: true,
		}},
		Current: 0,
	}
	a.nowMediaAUMID = "loading.session"
	// The entry pose fades the content in over time, and the tester's clock
	// stands still; settle it so the frame shows the card.
	a.flyoutEntered = true
	for _, dark := range []bool{false, true} {
		tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
		tt.SetDark(dark)
		tt.Frame()
		title, ok := tt.Find("正在连接…")
		if !ok {
			t.Fatalf("dark=%v title missing; texts: %q", dark, tt.Texts())
		}
		img := tt.Image()
		scale := float32(img.Bounds().Dx()) / float32(flyoutWidth)
		// The poster is 52 wide, 10 left of the copy; sample the tile's left
		// middle, away from the note glyph, where its fill is flat.
		x := int((title.X - 10 - 52 + 3) * scale)
		y := int((title.Y + 18.5) * scale)
		got := img.RGBAAt(x, y)
		pal := paletteFor(dark)
		near := func(a, b uint8) bool {
			d := int(a) - int(b)
			return d >= -1 && d <= 1
		}
		if !near(got.R, pal.card.R) || !near(got.G, pal.card.G) || !near(got.B, pal.card.B) {
			t.Fatalf("dark=%v placeholder pixel = %v, want the raised card surface %v", dark, got, pal.card)
		}
		if near(got.R, pal.pill.R) && near(got.G, pal.pill.G) && near(got.B, pal.pill.B) {
			t.Fatalf("dark=%v placeholder pixel = %v matches the media card's pill %v", dark, got, pal.pill)
		}
	}
}

func TestMeterAnimatesOnlyLivePlaybackInAWindow(t *testing.T) {
	tests := []struct {
		name             string
		animate, playing bool
		windowed, reduce bool
		want             bool
	}{
		{"live", true, true, true, false, true},
		{"feature off", false, true, true, false, false},
		{"paused", true, false, true, false, false},
		{"headless", true, true, false, false, false},
		{"reduced motion", true, true, true, true, false},
	}
	for _, tt := range tests {
		if got := meterAnimates(tt.animate, tt.playing, tt.windowed, func() bool { return tt.reduce }); got != tt.want {
			t.Fatalf("%s: meterAnimates = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestSettingsPreviewMeterFollowsLivePlaybackOnly(t *testing.T) {
	tests := []struct{ live, want bool }{
		{true, true},
		{false, false},
	}
	for _, tt := range tests {
		if got := previewMeterAnimates(tt.live); got != tt.want {
			t.Fatalf("previewMeterAnimates(live=%v) = %v, want %v", tt.live, got, tt.want)
		}
	}
}

func TestNowPlayingHeadShowsAppIconWhenAvailable(t *testing.T) {
	icon := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range icon.Pix {
		icon.Pix[i] = 0xff
	}
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, icon); err != nil {
		t.Fatal(err)
	}
	titleX := func(iconBytes []byte) (float32, *App) {
		a := previewApp()
		a.nowSnap = nowplaying.Snapshot{Sessions: []nowplaying.Session{{
			AUMID: "icon.session", AppName: "Icon App", Title: "夜曲", Artist: "周杰伦",
			Status: nowplaying.StatusPlaying, Playing: true, AppIcon: iconBytes,
		}}, Current: 0}
		a.nowMediaAUMID = "icon.session"
		a.refreshNowAppIcon()
		a.flyoutEntered = true
		tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
		tt.Frame()
		name, ok := tt.Find("Icon App")
		if !ok {
			t.Fatalf("app name missing; texts: %q", tt.Texts())
		}
		return name.X, a
	}
	with, a := titleX(pngBytes.Bytes())
	if a.nowIcon == nil {
		t.Fatal("the valid icon did not decode")
	}
	without, _ := titleX(nil)
	if with <= without {
		t.Fatalf("app name x with icon=%.1f, without=%.1f, want the icon to lead it", with, without)
	}

	// A broken icon is dropped and leaves the name alone.
	broken, a := titleX([]byte{0x89, 'P', 'N', 'G'})
	if a.nowIcon != nil {
		t.Fatal("a broken icon decoded")
	}
	if broken != without {
		t.Fatalf("broken icon shifted the name: %.1f vs %.1f", broken, without)
	}
}

func TestCoverFieldCompressesLightness(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: uint8(x * 4), B: 0, A: 255})
		}
	}
	for _, dark := range []bool{false, true} {
		f := coverField(img, dark)
		if f == nil || f.Bounds().Dx() != fieldW || f.Bounds().Dy() != fieldH {
			t.Fatalf("dark=%v field = %v, want %dx%d", dark, f, fieldW, fieldH)
		}
		for y := 0; y < fieldH; y++ {
			for x := 0; x < fieldW; x++ {
				c := f.RGBAAt(x, y)
				maxv := float64(max(c.R, max(c.G, c.B)))
				minv := float64(min(c.R, min(c.G, c.B)))
				l := (maxv + minv) / 2 / 255
				if l < 0.38 || l > 0.73 {
					t.Fatalf("dark=%v lightness %.3f at %d,%d outside the compressed range", dark, l, x, y)
				}
			}
		}
	}
	if coverField(nil, false) != nil {
		t.Fatal("a nil cover must not produce a field")
	}
}

func TestErrorStateShowsRetry(t *testing.T) {
	a := previewApp()
	a.snap.State = sender.StateIdle
	a.snap.Error = "接收器没有响应"
	a.snap.ConnectedName = ""
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	for _, want := range []string{"重试", "连接失败：接收器没有响应", "连接失败"} {
		if !tt.HasText(want) {
			t.Fatalf("error state is missing %q; texts: %q", want, tt.Texts())
		}
	}
	// The error bar takes its own height; it must not squeeze the device card
	// or clip the playback card's last row.
	if card, ok := tt.Find("接收器列表"); !ok || card.H+1 < deviceCardHeight(len(a.snap.Devices)) {
		t.Fatalf("error state squeezes the device card to %.1f DIP", card.H)
	}
	if seg, ok := tt.Find("自定义"); !ok || seg.H+1 < 26 {
		t.Fatalf("error state clips the latency row to %.1f DIP", seg.H)
	}
}

func TestEmptyStateOffersSearch(t *testing.T) {
	a := previewApp()
	a.snap.Devices = nil
	a.snap.SelectedID = ""
	a.snap.State = sender.StateIdle
	a.selID = ""
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	for _, want := range []string{"暂无", "正在搜索附近的 AirPlay 接收器…", "重新搜索"} {
		if !tt.HasText(want) {
			t.Fatalf("empty state is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestFlyoutHeightForDeviceRows(t *testing.T) {
	noMedia := map[int]int{0: 368, 1: 285, 2: 341, 3: 397, 5: 397}
	withMedia := map[int]int{0: 518, 1: 435, 2: 491, 3: 547, 5: 547}
	for _, tc := range []struct {
		media bool
		want  map[int]int
	}{{false, noMedia}, {true, withMedia}} {
		for devices, want := range tc.want {
			want += lampSectionHeight
			if got := flyoutHeightFor(devices, false, tc.media); got != want {
				t.Errorf("flyoutHeightFor(%d, false, media=%v) = %d, want %d", devices, tc.media, got, want)
			}
			if got := flyoutHeightFor(devices, true, tc.media); got != want+latencySwitchHeight {
				t.Errorf("flyoutHeightFor(%d, true, media=%v) = %d, want %d", devices, tc.media, got, want+latencySwitchHeight)
			}
		}
	}
}
func TestFlyoutKeepsPlaybackGap(t *testing.T) {
	var want float32
	for devices := 0; devices <= deviceRows; devices++ {
		a := previewApp()
		a.snap.Devices = a.snap.Devices[:devices]
		tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
		tt.Frame()
		card, ok := tt.Find("接收器列表")
		if !ok {
			t.Fatalf("%d devices: the device card is missing", devices)
		}
		head, ok := tt.Find("播放")
		if !ok {
			t.Fatalf("%d devices: the playback head is missing", devices)
		}
		// The playback section's head has 6 DIP of top padding.
		gap := head.Y - 6 - (card.Y + card.H)
		if devices == 0 {
			want = gap
			continue
		}
		if abs32(gap-want) > 1 {
			t.Fatalf("%d devices: gap %.1f DIP, want %.1f", devices, gap, want)
		}
	}
}

func TestFlyoutTargetHeightFitsPlayback(t *testing.T) {
	const lastRow = "自定义"
	find := func(devices int) ui.Rect {
		a := previewApp()
		a.snap.Devices = a.snap.Devices[:devices]
		tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
		tt.Frame()
		r, ok := tt.Find(lastRow)
		if !ok {
			t.Fatalf("%d devices: the latency row is missing", devices)
		}
		return r
	}
	full := find(deviceRows).H
	for devices := 0; devices < deviceRows; devices++ {
		if got := find(devices).H; got+1 < full {
			t.Fatalf("%d devices: the latency row is clipped to %.1f DIP, want %.1f", devices, got, full)
		}
	}
}

func TestRefreshNowPlayingPushesWhileStreaming(t *testing.T) {
	streaming := false
	track := nowplaying.Session{Title: "夜曲", Artist: "周杰伦", Album: "十一月的萧邦"}
	ok := true
	var pushes []string
	a := &App{
		readNowPlaying: func() (nowplaying.Snapshot, error) {
			if !ok {
				return nowplaying.Snapshot{}, nil
			}
			return nowplaying.Snapshot{Sessions: []nowplaying.Session{track}, Current: 0}, nil
		},
		pushNowPlaying: func(title, artist, album string, cover []byte, coverMIME string) {
			pushes = append(pushes, title+"\x00"+artist)
		},
		nowPlayingStreaming: func() bool { return streaming },
	}

	// The state updates without a stream; nothing is sent.
	a.refreshNowPlaying()
	if !a.nowOK || a.nowTrack.Title != "夜曲" || a.nowTrack.Artist != "周杰伦" {
		t.Fatalf("track = %+v ok=%v", a.nowTrack, a.nowOK)
	}
	if len(pushes) != 0 {
		t.Fatalf("pushed without a stream: %v", pushes)
	}

	// The first streaming poll sends the track once.
	streaming = true
	a.refreshNowPlaying()
	if len(pushes) != 1 {
		t.Fatalf("pushes while streaming = %d, want 1", len(pushes))
	}
	a.refreshNowPlaying()
	if len(pushes) != 1 {
		t.Fatalf("unchanged track pushed twice: %v", pushes)
	}

	// The card's display switch does not gate the receiver push.
	a.showNowPlayingPanel.Store(false)
	track = nowplaying.Session{Title: "稻香", Artist: "周杰伦"}
	a.refreshNowPlaying()
	if len(pushes) != 2 || pushes[1] != "稻香\x00周杰伦" {
		t.Fatalf("push with the panel hidden = %v", pushes)
	}

	// A new streaming session resends the same metadata after reconnect.
	streaming = false
	a.refreshNowPlaying()
	streaming = true
	a.refreshNowPlaying()
	if len(pushes) != 3 {
		t.Fatalf("pushes after reconnect = %d, want 3", len(pushes))
	}

	// No media clears both the state and the sent marker.
	ok = false
	a.refreshNowPlaying()
	if a.nowOK || a.nowTrack.Title != "" {
		t.Fatalf("track = %+v ok=%v", a.nowTrack, a.nowOK)
	}
	if a.nowSent != "" {
		t.Fatalf("sent marker kept without media: %q", a.nowSent)
	}
}

func TestRefreshNowPlayingPushesArtworkChanges(t *testing.T) {
	session := nowplaying.Session{
		Title: "夜曲", Artist: "周杰伦", Album: "十一月的萧邦",
		Artwork: []byte{0x89, 'P', 'N', 'G', 1}, ArtworkMIME: "image/png",
	}
	var covers [][]byte
	var mimes []string
	a := &App{
		readNowPlaying: func() (nowplaying.Snapshot, error) {
			return nowplaying.Snapshot{Sessions: []nowplaying.Session{session}, Current: 0}, nil
		},
		pushNowPlaying: func(title, artist, album string, cover []byte, coverMIME string) {
			covers = append(covers, append([]byte(nil), cover...))
			mimes = append(mimes, coverMIME)
		},
		nowPlayingStreaming: func() bool { return true },
	}
	a.refreshNowPlaying()
	if len(covers) != 1 || !bytes.Equal(covers[0], session.Artwork) || mimes[0] != "image/png" {
		t.Fatalf("covers = %v mimes = %v", covers, mimes)
	}

	// The same bytes in a rebuilt session do not resend.
	a.readNowPlaying = func() (nowplaying.Snapshot, error) {
		s := session
		s.Artwork = append([]byte(nil), session.Artwork...)
		return nowplaying.Snapshot{Sessions: []nowplaying.Session{s}, Current: 0}, nil
	}
	a.refreshNowPlaying()
	if len(covers) != 1 {
		t.Fatalf("unchanged artwork pushed again: %d", len(covers))
	}

	// SMTC can replace the thumbnail under the same text; the receiver gets
	// the new bytes.
	session.Artwork = []byte{0x89, 'P', 'N', 'G', 2}
	a.refreshNowPlaying()
	if len(covers) != 2 || !bytes.Equal(covers[1], session.Artwork) {
		t.Fatalf("covers after artwork change = %v", covers)
	}
}

func TestPinDialogRenders(t *testing.T) {
	a := previewApp()
	a.snap.PinRequired = true
	a.snap.PinDevice = "Apple TV"
	a.pinOpen = true
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	for _, want := range []string{
		"输入配对码", "查看 Apple TV 上显示的配对码。", "仅用于本次配对。", "取消", "配对",
	} {
		if !tt.HasText(want) {
			t.Fatalf("PIN dialog is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestPhaseOf(t *testing.T) {
	cases := []struct {
		name string
		snap engine.Snapshot
		want phase
	}{
		{"idle", engine.Snapshot{State: sender.StateIdle}, phaseIdle},
		{"connecting", engine.Snapshot{State: sender.StateConnecting}, phaseConnecting},
		{"pairing", engine.Snapshot{State: sender.StatePairing}, phaseConnecting},
		{"handshake", engine.Snapshot{State: sender.StateHandshake}, phaseConnecting},
		{"reconnecting", engine.Snapshot{State: sender.StateIdle, Reconnecting: true}, phaseConnecting},
		{"streaming", engine.Snapshot{State: sender.StateStreaming}, phaseStreaming},
		{"failed", engine.Snapshot{State: sender.StateIdle, Error: "boom"}, phaseError},
		{"streaming after an error", engine.Snapshot{State: sender.StateStreaming, Error: "boom"}, phaseStreaming},
	}
	for _, tc := range cases {
		if got := phaseOf(tc.snap); got != tc.want {
			t.Errorf("%s: phaseOf = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestDeviceRowSelectsOnClick(t *testing.T) {
	a := previewApp()
	a.selID = "AA:BB:CC:DD:EE:01"
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if err := tt.Click("HomePod mini"); err != nil {
		t.Fatal(err)
	}
	if a.selID != "AA:BB:CC:DD:EE:03" {
		t.Fatalf("selected %q, want the clicked device", a.selID)
	}
}

func TestSettingsNavKeyboardChoosesPage(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	// The first tab stop is 通用, the second 连接.
	for range 2 {
		tt.Key(0, ui.KeyTab)
		tt.Frame()
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.page != pageConnection {
		t.Fatalf("page = %d, want the connection page", a.page)
	}
}

func TestToggleTogglesWithKeyboard(t *testing.T) {
	on := false
	var calls []bool
	tt := ui.NewTester(func(c *ui.Context) {
		toggle(c, paletteFor(false), &on, "测试开关", func(v bool) { calls = append(calls, v) })
	}, 120, 40)
	tt.Frame()
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if !on || len(calls) != 1 || !calls[0] {
		t.Fatalf("after Space: on=%v calls=%v", on, calls)
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if on || len(calls) != 2 || calls[1] {
		t.Fatalf("after a second Space: on=%v calls=%v", on, calls)
	}
}

func TestSliderHasNameAndKeys(t *testing.T) {
	value := 50.0
	tt := ui.NewTester(func(c *ui.Context) {
		slider(c, paletteFor(false), &value, 0, 100, "延迟滑块", func(float64) {})
	}, 200, 40)
	tt.Frame()
	if _, ok := tt.Find("延迟滑块"); !ok {
		t.Fatal("the slider has no accessible name")
	}
	if err := tt.Click("延迟滑块"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	if value <= 50 {
		t.Fatalf("value = %v after Right, want above 50", value)
	}
}

func TestTrayAnimationFaces(t *testing.T) {
	for _, face := range []string{"stream", "connect"} {
		if !trayAnimatedFace(face) {
			t.Fatalf("%s should animate", face)
		}
	}
	for _, face := range []string{"smile", "error", ""} {
		if trayAnimatedFace(face) {
			t.Fatalf("%s should stay static", face)
		}
	}
}

func TestTrayAnimationSettingPersists(t *testing.T) {
	a := previewApp()
	a.sets = settings.NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	a.trayAnimation = true
	a.setTrayAnimation(false)
	if a.trayAnimation {
		t.Fatal("setting did not turn the animation off")
	}
	if a.sets.Load().TrayAnimation {
		t.Fatal("setting did not persist")
	}
}

func TestThemeSettingPersistsAndMapsToTheFramework(t *testing.T) {
	a := previewApp()
	a.sets = settings.NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	var applied []mygo.ThemeSource
	a.setThemeSource = func(source mygo.ThemeSource) { applied = append(applied, source) }
	a.setTheme(locales.ThemeDark())
	if a.cfg.Theme != settings.ThemeDark {
		t.Fatalf("setting did not choose dark: %q", a.cfg.Theme)
	}
	if a.sets.Load().Theme != settings.ThemeDark {
		t.Fatal("setting did not persist")
	}
	if got := a.themeLabel(); got != locales.ThemeDark() {
		t.Fatalf("themeLabel = %q, want %q", got, locales.ThemeDark())
	}
	if len(applied) != 1 || applied[0] != mygo.ThemeDark {
		t.Fatalf("applied appearances = %v, want [dark]", applied)
	}
	a.setTheme(locales.ThemeSystem())
	if a.sets.Load().Theme != settings.ThemeSystem {
		t.Fatal("follow system did not persist")
	}
	if len(applied) != 2 || applied[1] != mygo.ThemeSystem {
		t.Fatalf("applied appearances = %v, want [dark system]", applied)
	}
}

func TestFlyoutFooterFaceFollowsTray(t *testing.T) {
	snap := engine.Snapshot{State: sender.StateStreaming}
	if got := footerFace(snap); got != faceStream {
		t.Fatal("streaming footer face is not the stream face")
	}
	snap.AudioIdle = true
	if got := footerFace(snap); got != iconMark {
		t.Fatal("audio-idle footer face is not the smile")
	}
	snap.State = sender.StateConnecting
	if got := footerFace(snap); got != faceConnect {
		t.Fatal("connecting footer face is not the connect face")
	}
	snap.State = sender.StateIdle
	snap.Error = "boom"
	if got := footerFace(snap); got != faceError {
		t.Fatal("failed footer face is not the error face")
	}
	snap.Error = ""
	if got := footerFace(snap); got != faceIdleOpen {
		t.Fatal("idle footer face is not the open-eyed line face")
	}
}

func TestTrayIconFollowsPhase(t *testing.T) {
	a := previewApp()
	if got := a.face(); got != "stream" {
		t.Fatalf("streaming face = %q", got)
	}
	a.snap.AudioIdle = true
	if got := a.face(); got != "smile" {
		t.Fatalf("audio-idle face = %q", got)
	}
	a.snap.AudioIdle = false
	if got := a.face(); got != "stream" {
		t.Fatalf("resumed face = %q", got)
	}
	a.snap.State = sender.StateIdle
	a.snap.Error = "boom"
	if got := a.face(); got != "error" {
		t.Fatalf("error face = %q", got)
	}
	a.snap.Error = ""
	if got := a.face(); got != "idle-open" {
		t.Fatalf("idle face = %q", got)
	}
	a.snap.Devices = nil
	if got := a.face(); got != "idle-open" {
		t.Fatalf("empty face = %q", got)
	}
}

func TestDeviceIconFollowsModel(t *testing.T) {
	cases := []struct {
		model string
		want  *ui.SVG
	}{
		{"Mac14,2", iconMac},
		{"MacBookPro18,3", iconMac},
		{"AppleTV6,2", iconTV},
		{"AppleTV3,2", iconTV},
		{"AudioAccessory5,1", iconSpeaker},
		{"", iconSpeaker},
	}
	for _, tc := range cases {
		if got := deviceIcon(tc.model); got != tc.want {
			t.Errorf("deviceIcon(%q) is not the expected icon", tc.model)
		}
	}
}

func TestTrayAssetsDecode(t *testing.T) {
	for _, face := range []string{"stream", "connect", "error", "smile", "idle-open"} {
		for _, dark := range []bool{false, true} {
			img, err := png.Decode(bytes.NewReader(trayPNGs[trayIconKey{face: face, dark: dark}]))
			if err != nil {
				t.Fatalf("face %q dark %v: %v", face, dark, err)
			}
			if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
				t.Fatalf("face %q dark %v: %dx%d, want 32x32", face, dark, b.Dx(), b.Dy())
			}
		}
	}
}

func writePreview(t *testing.T, tt *ui.Tester, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestFlyoutLatencySwitchCanHide(t *testing.T) {
	a := previewApp()
	a.showLatencySwitch = false
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(len(a.snap.Devices), false, a.hasNowMedia()))
	if tt.HasText("低延迟") {
		t.Fatalf("hidden switch still shows the mode row: %q", tt.Texts())
	}
	if !tt.HasText("延迟 200 ms") {
		t.Fatalf("the ALAC line lost the latency: %q", tt.Texts())
	}
}

func TestFlyoutCustomLatencyOnALACLine(t *testing.T) {
	a := previewApp()
	a.latencyMode = "custom"
	a.latencyCustom = 700
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(len(a.snap.Devices), true, a.hasNowMedia()))
	if !tt.HasText("延迟 700 ms") || !tt.HasText("自定义") {
		t.Fatalf("custom latency not shown: %q", tt.Texts())
	}
}

func TestFlyoutLatencySegmentSelects(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(len(a.snap.Devices), true, a.hasNowMedia()))
	tt.Frame()
	if err := tt.Click("标准"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.latencyMode != "standard" || a.latency != 1500 {
		t.Fatalf("selected %q/%v, want standard/1500", a.latencyMode, a.latency)
	}
	if !tt.HasText("延迟 1.5 s") {
		t.Fatalf("the ALAC line did not follow: %q", tt.Texts())
	}
}

func TestAudioCustomModeRendersSlider(t *testing.T) {
	a := previewApp()
	a.page = pageAudio
	a.latencyMode = "custom"
	a.latencyCustom = 700
	tt := ui.NewTester(a.settingsView, 720, 500)
	for _, want := range []string{"自定义", "700 ms", "20 ms", "2 s"} {
		if !tt.HasText(want) {
			t.Fatalf("custom audio page is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestAudioModeTileSelects(t *testing.T) {
	a := previewApp()
	a.page = pageAudio
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	if err := tt.Click("标准 · 1.5 s"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.latencyMode != "standard" || a.latency != 1500 {
		t.Fatalf("selected %q/%v, want standard/1500", a.latencyMode, a.latency)
	}
}

func TestLatencyApplyHoldsStreamingSnapshot(t *testing.T) {
	a := previewApp()
	held := a.snap
	a.latencyApplying = true
	a.apply(engine.Snapshot{
		Devices: held.Devices, SelectedID: held.SelectedID,
		State: sender.StateHandshake, ConnectedName: held.ConnectedName,
	})
	if a.snap.State != held.State || a.snap.ConnectedName != held.ConnectedName {
		t.Fatalf("snapshot was not held: %+v", a.snap)
	}
	a.apply(engine.Snapshot{
		Devices: held.Devices, SelectedID: held.SelectedID,
		State: sender.StateStreaming, ConnectedName: held.ConnectedName,
	})
	if a.latencyApplying {
		t.Fatal("the hold did not finish on streaming")
	}
	if a.snap.State != sender.StateStreaming {
		t.Fatalf("final state = %v", a.snap.State)
	}
}

func TestLatencySpinnerKeepsALACLineAnchored(t *testing.T) {
	withLocale(t, "zh-CN")
	a := previewApp()
	const note = "ALAC · 44.1 kHz · PTP · 已同步 · 延迟 200 ms"
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(len(a.snap.Devices), true, a.hasNowMedia()))
	tt.Frame()
	before, ok := tt.Find(note)
	if !ok {
		t.Fatalf("ALAC line is missing: %q", tt.Texts())
	}
	a.latencyApplying = true
	tt.Frame()
	after, ok := tt.Find(note)
	if !ok {
		t.Fatalf("ALAC line is missing while applying: %q", tt.Texts())
	}
	if before.X != after.X || before.W != after.W {
		t.Fatalf("ALAC line moved from %.1f/%.1f to %.1f/%.1f", before.X, before.W, after.X, after.W)
	}
}

func TestSettingsAudioPageScrollsAtMinimumHeight(t *testing.T) {
	withLocale(t, "zh-CN")
	a := previewApp()
	a.page = pageAudio
	a.latencyMode = "custom"
	a.latencyCustom = 700
	tt := ui.NewTester(a.settingsView, 720, 420)
	tt.Frame()
	before, ok := tt.Find("同步方式")
	if !ok {
		t.Fatalf("the sync row is missing: %q", tt.Texts())
	}
	tt.Scroll(500, 300, 0, 160)
	tt.Frame()
	after, ok := tt.Find("同步方式")
	if !ok {
		t.Fatalf("the sync row is missing after scrolling: %q", tt.Texts())
	}
	if after.Y >= before.Y {
		t.Fatalf("the audio page did not scroll: sync row %.1f -> %.1f", before.Y, after.Y)
	}
}

// TestAudioPageKeepsLatencyTextInsideItsCards pins the two preset layouts:
// below latencyStackWidth the presets are full-width rows, at or above it three
// cards share a row. In both, every name, value, and description stays inside
// its radio; the fixed 99-DIP card used to let narrow descriptions spill into
// the gap and under the custom tile.
func TestAudioPageKeepsLatencyTextInsideItsCards(t *testing.T) {
	withLocale(t, "en")
	for _, width := range []int{640, latencyStackWidth, 720} {
		a := previewApp()
		a.page = pageAudio
		a.latencyMode = settings.LatencyLow
		tt := ui.NewTester(a.settingsView, width, 500)
		tt.Frame()
		custom, ok := tt.AccessByRole(ui.RoleRadio,
			locales.SettingsLatencyCustom()+" · "+settings.LatencyText(700))
		if !ok {
			t.Fatalf("at %d DIP the custom row is missing: %q", width, tt.Texts())
		}
		for _, preset := range latencyPresets() {
			card, ok := tt.AccessByRole(ui.RoleRadio, preset.name+" · "+preset.latency)
			if !ok {
				t.Fatalf("at %d DIP the %q card is missing: %q", width, preset.name, tt.Texts())
			}
			for _, text := range []string{preset.name, preset.latency, preset.desc} {
				box, ok := tt.Find(text)
				if !ok {
					t.Fatalf("at %d DIP %q is missing from %q: %q", width, preset.name, text, tt.Texts())
				}
				if box.X < card.Bounds.X-0.5 || box.Y < card.Bounds.Y-0.5 ||
					box.X+box.W > card.Bounds.X+card.Bounds.W+0.5 ||
					box.Y+box.H > card.Bounds.Y+card.Bounds.H+0.5 {
					t.Fatalf("at %d DIP, %q leaves its %q card: text (%.1f, %.1f, %.1f×%.1f), card (%.1f, %.1f, %.1f×%.1f)",
						width, text, preset.name, box.X, box.Y, box.W, box.H,
						card.Bounds.X, card.Bounds.Y, card.Bounds.W, card.Bounds.H)
				}
			}
			if width < latencyStackWidth && card.Bounds.W+0.5 < custom.Bounds.W {
				t.Fatalf("at %d DIP, %q is %.1f wide, not the custom row's %.1f: the presets did not stack",
					width, preset.name, card.Bounds.W, custom.Bounds.W)
			}
			if width >= latencyStackWidth && card.Bounds.W*2 > custom.Bounds.W {
				t.Fatalf("at %d DIP, %q is %.1f wide, not one of three cards: the presets did not share a row",
					width, preset.name, card.Bounds.W)
			}
		}
	}
}

// TestSettingsColumnCapsAndCenters pins the shared content column: the
// default window keeps the width it always had, and a wide window stops the
// header and the cards at 720 DIP and centers them instead of stretching every
// row across the screen.
func TestSettingsColumnCapsAndCenters(t *testing.T) {
	withLocale(t, "en")
	for _, tc := range []struct {
		width   int
		left    float32
		customW float32
	}{
		{720, 210, 458},
		{1440, 455, 688},
	} {
		a := previewApp()
		a.page = pageAudio
		tt := ui.NewTester(a.settingsView, tc.width, 900)
		tt.Frame()
		desc, ok := tt.Find(locales.PageAudioDesc())
		if !ok {
			t.Fatalf("at %d DIP the page description is missing: %q", tc.width, tt.Texts())
		}
		if abs32(desc.X-tc.left) > 1.5 {
			t.Fatalf("at %d DIP the column starts at %.1f, want %.1f", tc.width, desc.X, tc.left)
		}
		custom, ok := tt.AccessByRole(ui.RoleRadio,
			locales.SettingsLatencyCustom()+" · "+settings.LatencyText(700))
		if !ok {
			t.Fatalf("at %d DIP the custom row is missing: %q", tc.width, tt.Texts())
		}
		if abs32(custom.Bounds.W-tc.customW) > 1.5 {
			t.Fatalf("at %d DIP the card is %.1f wide, want %.1f", tc.width, custom.Bounds.W, tc.customW)
		}
	}
}

// TestLampTemplateTilesKeepTheirSize pins the six template swatches to one
// size at every window width; a narrow page wraps the strip instead of
// shrinking the swatches.
func TestLampTemplateTilesKeepTheirSize(t *testing.T) {
	withLocale(t, "en")
	for _, width := range []int{720, 1440} {
		a := previewApp()
		a.page = pageLamp
		a.lampPreset = "moon"
		tt := ui.NewTester(a.settingsView, width, 900)
		tt.Frame()
		for _, name := range []string{"Ember", "Dune", "Moss", "Tide", "Orchid", "Moon"} {
			tile, ok := tt.AccessByRole(ui.RoleButton, name)
			if !ok {
				t.Fatalf("at %d DIP the %q template is missing: %q", width, name, tt.Texts())
			}
			if abs32(tile.Bounds.W-lampTileWidth) > 0.5 {
				t.Fatalf("at %d DIP the %q template is %.1f wide, want %d",
					width, name, tile.Bounds.W, lampTileWidth)
			}
		}
	}
	a := previewApp()
	a.page = pageLamp
	tt := ui.NewTester(a.settingsView, 640, 900)
	tt.Frame()
	first, ok := tt.AccessByRole(ui.RoleButton, "Ember")
	if !ok {
		t.Fatalf("the narrow Ember template is missing: %q", tt.Texts())
	}
	last, ok := tt.AccessByRole(ui.RoleButton, "Moon")
	if !ok {
		t.Fatalf("the narrow Moon template is missing: %q", tt.Texts())
	}
	if last.Bounds.Y <= first.Bounds.Y {
		t.Fatalf("at 640 DIP the strip did not wrap: Ember y=%.1f Moon y=%.1f",
			first.Bounds.Y, last.Bounds.Y)
	}
}

// TestNowPlayingCoverOptionsSitInTheRow pins the cover control to the right of
// its description on one row, with no separate title: the three options name
// the cover themselves.
func TestNowPlayingCoverOptionsSitInTheRow(t *testing.T) {
	withLocale(t, "en")
	a := previewApp()
	a.page = pageNowPlaying
	tt := ui.NewTester(a.settingsView, 720, 900)
	tt.Frame()
	desc, ok := tt.Find(locales.SettingsCoverDesc())
	if !ok {
		t.Fatalf("the cover description is missing: %q", tt.Texts())
	}
	none, ok := tt.Find(locales.SettingsCoverNone())
	if !ok {
		t.Fatalf("the cover options are missing: %q", tt.Texts())
	}
	if none.X <= desc.X+desc.W {
		t.Fatalf("the cover options start at %.1f, not right of the description ending at %.1f",
			none.X, desc.X+desc.W)
	}
	if none.Y+none.H/2 < desc.Y-20 || none.Y+none.H/2 > desc.Y+desc.H+20 {
		t.Fatalf("the cover options sit at y=%.1f..%.1f, not on the description's row %.1f..%.1f",
			none.Y, none.Y+none.H, desc.Y, desc.Y+desc.H)
	}
}

// TestCaptureStatusRowKeepsItsTitle pins the capture status to the row's
// right-hand column: the device name follows the hardware, so the value is
// capped and ellipsized while the title keeps its width, and the full name is
// in the tooltip.
func TestCaptureStatusRowKeepsItsTitle(t *testing.T) {
	withLocale(t, "en")
	for _, tc := range []struct {
		width    int
		minTitle float32
	}{
		{640, 140},
		{720, 180},
		{1440, 300},
	} {
		a := previewApp()
		a.page = pageCapture
		a.snap.CaptureName = "Mi Monitor (NVIDIA High Definition Audio)"
		a.snap.CaptureRate = 48000
		tt := ui.NewTester(a.settingsView, tc.width, 900)
		tt.Frame()
		title, ok := tt.Find(locales.SettingsCaptureStatus())
		if !ok {
			t.Fatalf("at %d DIP the status title is missing: %q", tc.width, tt.Texts())
		}
		status, ok := tt.Find("Mi Monitor (NVIDIA High Definition Audio) · 48 kHz")
		if !ok {
			t.Fatalf("at %d DIP the capture status is missing: %q", tc.width, tt.Texts())
		}
		if title.W < tc.minTitle {
			t.Fatalf("at %d DIP the status title is only %.1f wide, want at least %.0f",
				tc.width, title.W, tc.minTitle)
		}
		if status.Y < title.Y-1 || status.Y > title.Y+40 {
			t.Fatalf("at %d DIP the status starts at %.1f, not on the title's row at %.1f",
				tc.width, status.Y, title.Y)
		}
	}
}

func TestNowPlayingSettingsPersist(t *testing.T) {
	a := previewApp()
	a.sets = settings.NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	a.page = pageNowPlaying
	a.nowSnap = nowplaying.Snapshot{
		Sessions: []nowplaying.Session{{
			AUMID: "test.session", AppName: "Test", Title: "夜曲", Artist: "周杰伦",
			Status: nowplaying.StatusPlaying, Playing: true,
			CanPlay: true, CanPause: true,
		}},
		Current: 0,
	}
	a.nowMediaAUMID = "test.session"
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	chip, ok := tt.Find(locales.SettingsShowInPanel())
	if !ok {
		t.Fatalf("display chip missing; texts: %q", tt.Texts())
	}
	// The display switch sits after the chip, 8 DIP gap, 32 wide.
	switchX := chip.X + chip.W + 8 + 16
	tt.ClickAt(switchX, chip.Y+chip.H/2)
	tt.Frame()
	if a.cfg.ShowNowPlayingPanel {
		t.Fatal("display switch did not turn off")
	}
	if a.showNowPlayingPanel.Load() {
		t.Fatal("display gate still on")
	}
	cfg := a.sets.Load()
	if cfg.ShowNowPlayingPanel {
		t.Fatal("display setting not saved")
	}
	if !cfg.PlaybackControls || !cfg.CoverColors || !cfg.AudioLevel {
		t.Fatalf("sibling defaults changed: %+v", cfg)
	}
	// A dependent row stays active while the card is hidden.
	row, ok := tt.Find(locales.SettingsPlaybackControls())
	if !ok {
		t.Fatalf("playback row missing; texts: %q", tt.Texts())
	}
	tt.ClickAt(switchX, row.Y+row.H/2)
	tt.Frame()
	if a.cfg.PlaybackControls {
		t.Fatal("active playback row did not change")
	}
}

func TestNowPlayingPanelHiddenWhenOff(t *testing.T) {
	a := previewApp()
	a.cfg.ShowNowPlayingPanel = false
	a.nowSnap = nowplaying.Snapshot{
		Sessions: []nowplaying.Session{{
			AUMID: "test.session", AppName: "Apple Music", Title: "夜曲", Artist: "周杰伦",
			Status: nowplaying.StatusPlaying, Playing: true,
			CanPlay: true, CanPause: true,
		}},
		Current: 0,
	}
	a.nowMediaAUMID = "test.session"
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	for _, gone := range []string{"Apple Music", "夜曲"} {
		if tt.HasText(gone) {
			t.Fatalf("panel still shows %q with the master off", gone)
		}
	}
	if got, want := visibleFlyoutHeight(a), flyoutHeightFor(len(a.snap.Devices), a.showLatencySwitch, false); got != want {
		t.Fatalf("flyout height = %d, want the no-card height %d", got, want)
	}
}

func TestNowPlayingControlsSetting(t *testing.T) {
	a := previewApp()
	a.cfg.PlaybackControls = false
	a.nowSnap = nowplaying.Snapshot{
		Sessions: []nowplaying.Session{{
			AUMID: "test.session", AppName: "Test", Title: "夜曲", Artist: "周杰伦",
			Status: nowplaying.StatusPlaying, Playing: true,
			CanPlay: true, CanPause: true,
		}},
		Current: 0,
	}
	a.nowMediaAUMID = "test.session"
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	tt.Frame()
	if _, ok := tt.Find(locales.FlyoutPause()); ok {
		t.Fatal("transport shown while the playback controls are off")
	}
	if !tt.HasText("夜曲") {
		t.Fatalf("card missing; texts: %q", tt.Texts())
	}
}

func TestNowPlayingArtworkRefreshes(t *testing.T) {
	a := previewApp()
	encode := func(r, g, b uint8) []byte {
		img := image.NewRGBA(image.Rect(0, 0, 2, 2))
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			}
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	a.nowSnap = nowplaying.Snapshot{Sessions: []nowplaying.Session{{
		AUMID: "test.session", AppName: "Test", Title: "夜曲", Artist: "周杰伦",
		Status: nowplaying.StatusPlaying, Playing: true,
		Artwork: encode(255, 0, 0),
	}}, Current: 0}
	a.refreshNowArtwork()
	first := a.nowArt
	if first == nil {
		t.Fatal("the first artwork did not decode")
	}
	a.nowSnap.Sessions[0].Artwork = encode(0, 0, 255)
	a.refreshNowArtwork()
	if a.nowArt == nil || a.nowArt == first {
		t.Fatal("the artwork did not refresh under the same title")
	}
}

func TestNowPlayingCoverModes(t *testing.T) {
	titleX := func(mode settings.CoverMode) (float32, int) {
		a := previewApp()
		a.cfg.CoverMode = mode
		a.nowSnap = nowplaying.Snapshot{Sessions: []nowplaying.Session{{
			AUMID: "test.session", AppName: "Test", Title: "夜曲", Artist: "周杰伦",
			Status: nowplaying.StatusPlaying, Playing: true,
		}}, Current: 0}
		a.nowMediaAUMID = "test.session"
		tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
		tt.Frame()
		title, ok := tt.Find("夜曲")
		if !ok {
			t.Fatalf("mode %q: title missing", mode)
		}
		return title.X, visibleFlyoutHeight(a)
	}
	none, hNone := titleX(settings.CoverNone)
	placeholder, hPlaceholder := titleX(settings.CoverPlaceholder)
	media, hMedia := titleX(settings.CoverMedia)
	if none >= placeholder || none >= media {
		t.Fatalf("title x none=%.1f placeholder=%.1f media=%.1f, want none furthest left", none, placeholder, media)
	}
	if placeholder != media {
		t.Fatalf("title x placeholder=%.1f media=%.1f, want the same poster width", placeholder, media)
	}
	if hNone != hMedia || hPlaceholder != hMedia {
		t.Fatalf("heights none=%d placeholder=%d media=%d, want a fixed card height", hNone, hPlaceholder, hMedia)
	}
}

func TestNowPlayingCoverModePersists(t *testing.T) {
	a := previewApp()
	a.sets = settings.NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	a.page = pageNowPlaying
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	if err := tt.Click(locales.SettingsCoverNone()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.cfg.CoverMode != settings.CoverNone {
		t.Fatalf("cover mode = %q, want none", a.cfg.CoverMode)
	}
	if got := a.sets.Load().CoverMode; got != settings.CoverNone {
		t.Fatalf("stored cover mode = %q, want none", got)
	}
}
