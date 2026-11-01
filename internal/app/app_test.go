package app

import (
	"bytes"
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/engine"
	"saundo/internal/nowplaying"
)

func previewApp() *App {
	a := &App{
		volume:        65,
		captureNames:  []string{"跟随系统默认", "扬声器"},
		captureIDs:    []string{"", "{endpoint}"},
		captureChoice: "跟随系统默认",
		appNames:      []string{"整个系统", "Spotify"},
		appIDs:        []string{"0", "1234"},
		appChoice:     "整个系统",
		nameDraft:     "声渡",
		latency:       200,
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
		CaptureName:    "扬声器",
		CaptureRate:    48000,
		CaptureRunning: true,
		Name:           "声渡",
		DroppedSamples: 0,
		PTPActive:      true,
		PTPReady:       true,
		Reconnect:      true,
	}
	a.reconnect = true
	return a
}

func TestViewRendersDevicesAndStatus(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeight)
	for _, want := range []string{
		"声渡", "saundo", "设备", "HomePods", "Apple TV", "HomePod mini",
		"连接", "断开", "离线", "音量", "正在播放到 HomePods",
		"ALAC · 44.1 kHz · PTP · 已同步", "推送", "未检测到媒体", "诊断日志",
	} {
		if !tt.HasText(want) {
			t.Fatalf("view is missing %q; texts: %q", want, tt.Texts())
		}
	}
	if path := os.Getenv("SAUNDO_PREVIEW"); path != "" {
		writePreview(t, tt, path)
	}
}

func TestPasswordPromptRenders(t *testing.T) {
	a := previewApp()
	a.snap.PasswordRequired = true
	a.snap.PasswordDevice = "MacBook"
	a.snap.PasswordDeviceID = "AA:BB:CC:DD:EE:02"
	a.passwordOpen = true
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeight)
	for _, want := range []string{
		"输入接收端密码", "输入 MacBook 的 AirPlay 接收密码。",
		"密码保存在本机，只用于这台设备。", "连接", "取消",
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
		{pageGeneral, []string{"通用", "发送端身份、启动与版本。", "发送端名称", "保存", "开机自动启动", "关于", "声渡 · saundo"}},
		{pageConnection, []string{"连接", "配对、重连与验证策略", "HomePods", "已连接", "启动时自动连接上次设备", "断开后自动重连", "严格验证接收器", "接收端密码", "保存到 HomePods"}},
		{pageAudio, []string{"音频", "播放延迟", "200 ms", "音频格式", "ALAC · 44.1 kHz · 立体声", "同步方式", "PTP · 已同步"}},
		{pageCapture, []string{"采集", "系统输出", "按应用采集", "采集状态", "扬声器 · 48 kHz"}},
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

func TestFlyoutKeepsPlaybackControlsOnly(t *testing.T) {
	a := previewApp()
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeight)
	for _, moved := range []string{
		"开机自动启动", "启动时自动连接上次设备", "断开后自动重连",
		"严格验证接收器", "播放延迟", "接收端密码", "系统输出",
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
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeight)
	for _, want := range []string{"HomePods", "立体声组 2/2"} {
		if !tt.HasText(want) {
			t.Fatalf("group view is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestNowPlayingRowShowsTrack(t *testing.T) {
	a := previewApp()
	a.nowOK = true
	a.nowTrack = nowplaying.Track{Title: "夜曲", Artist: "周杰伦", Album: "十一月的萧邦"}
	a.showNowPlaying = true
	a.snap.ShowNowPlaying = true
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeight)
	for _, want := range []string{"夜曲 · 周杰伦", "正在播放 · 十一月的萧邦", "推送"} {
		if !tt.HasText(want) {
			t.Fatalf("now-playing row is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestErrorStateShowsRetry(t *testing.T) {
	a := previewApp()
	a.snap.State = sender.StateIdle
	a.snap.Error = "接收器没有响应"
	a.snap.ConnectedName = ""
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeight)
	for _, want := range []string{"重试", "连接失败：接收器没有响应", "连接失败"} {
		if !tt.HasText(want) {
			t.Fatalf("error state is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestEmptyStateOffersSearch(t *testing.T) {
	a := previewApp()
	a.snap.Devices = nil
	a.snap.SelectedID = ""
	a.snap.State = sender.StateIdle
	a.selID = ""
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeight)
	for _, want := range []string{"0 台", "正在搜索附近的 AirPlay 设备…", "重新搜索"} {
		if !tt.HasText(want) {
			t.Fatalf("empty state is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestFlyoutHeightForDeviceRows(t *testing.T) {
	cases := map[int]int{0: 470, 1: 388, 2: 444, 3: 500, 5: 500}
	for devices, want := range cases {
		if got := flyoutHeightFor(devices); got != want {
			t.Errorf("flyoutHeightFor(%d) = %d, want %d", devices, got, want)
		}
	}
}

func TestFlyoutKeepsPlaybackGap(t *testing.T) {
	var want float32
	for devices := 0; devices <= deviceRows; devices++ {
		a := previewApp()
		a.snap.Devices = a.snap.Devices[:devices]
		tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(devices))
		tt.Frame()
		card, ok := tt.Find("设备列表")
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
	const capture = "采集 扬声器 · 48 kHz · 丢帧 0"
	find := func(devices int) ui.Rect {
		a := previewApp()
		a.snap.Devices = a.snap.Devices[:devices]
		tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(devices))
		tt.Frame()
		r, ok := tt.Find(capture)
		if !ok {
			t.Fatalf("%d devices: the capture line is missing", devices)
		}
		return r
	}
	full := find(deviceRows).H
	for devices := 0; devices < deviceRows; devices++ {
		if got := find(devices).H; got+0.5 < full {
			t.Fatalf("%d devices: the capture line is clipped to %.1f DIP, want %.1f", devices, got, full)
		}
	}
}

func TestRefreshNowPlayingPushesOnlyWhenEnabled(t *testing.T) {
	enabled, streaming := false, true
	track := nowplaying.Track{Title: "夜曲", Artist: "周杰伦", Album: "十一月的萧邦"}
	ok := true
	var pushes []string
	a := &App{
		readNowPlaying: func() (nowplaying.Track, bool, error) {
			return track, ok, nil
		},
		pushNowPlaying: func(title, artist, album string) {
			pushes = append(pushes, title+"\x00"+artist)
		},
		nowPlayingSetting: func() (bool, bool) { return enabled, streaming },
	}

	// The row updates even while pushing is off.
	a.refreshNowPlaying()
	if !a.nowOK || a.nowPlayingText() != "夜曲 · 周杰伦" {
		t.Fatalf("display = %q ok=%v", a.nowPlayingText(), a.nowOK)
	}
	if len(pushes) != 0 {
		t.Fatalf("pushed while disabled: %v", pushes)
	}

	// Enabling while disconnected shows the track but sends nothing; the
	// first streaming poll sends it.
	enabled, streaming = true, false
	a.refreshNowPlaying()
	if len(pushes) != 0 {
		t.Fatalf("pushed without a stream: %v", pushes)
	}
	streaming = true
	a.refreshNowPlaying()
	if len(pushes) != 1 {
		t.Fatalf("pushes after enabling = %d, want 1", len(pushes))
	}
	a.refreshNowPlaying()
	if len(pushes) != 1 {
		t.Fatalf("unchanged track pushed twice: %v", pushes)
	}

	// A new track while off is not sent; re-enabling sends it.
	enabled = false
	track = nowplaying.Track{Title: "稻香", Artist: "周杰伦"}
	a.refreshNowPlaying()
	enabled = true
	a.refreshNowPlaying()
	if len(pushes) != 2 || pushes[1] != "稻香\x00周杰伦" {
		t.Fatalf("pushes = %v", pushes)
	}

	// A new streaming session resends the same metadata after reconnect.
	streaming = false
	a.refreshNowPlaying()
	streaming = true
	a.refreshNowPlaying()
	if len(pushes) != 3 {
		t.Fatalf("pushes after reconnect = %d, want 3", len(pushes))
	}

	// No media clears both the display and the sent marker.
	ok = false
	a.refreshNowPlaying()
	if a.nowOK || a.nowPlayingText() != "未检测到媒体" {
		t.Fatalf("display = %q ok=%v", a.nowPlayingText(), a.nowOK)
	}
	if a.nowSent != "" {
		t.Fatalf("sent marker kept without media: %q", a.nowSent)
	}
}

func TestPinDialogRenders(t *testing.T) {
	a := previewApp()
	a.snap.PinRequired = true
	a.snap.PinDevice = "Apple TV"
	a.pinOpen = true
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeight)
	for _, want := range []string{
		"输入配对码", "在 Apple TV 的屏幕上查看配对码。", "配对码只用于本次配对。", "取消", "配对",
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

func TestTrayIconFollowsPhase(t *testing.T) {
	a := previewApp()
	if got := a.face(); got != "stream" {
		t.Fatalf("streaming face = %q", got)
	}
	a.snap.State = sender.StateIdle
	a.snap.Error = "boom"
	if got := a.face(); got != "error" {
		t.Fatalf("error face = %q", got)
	}
	a.snap.Error = ""
	if got := a.face(); got != "smile" {
		t.Fatalf("idle face = %q", got)
	}
	a.snap.Devices = nil
	if got := a.face(); got != "smile" {
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
	for _, face := range []string{"stream", "connect", "error", "smile"} {
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
