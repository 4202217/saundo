package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/receivers"
	"saundo/internal/settings"
)

// applyReceiverFixtures swaps the preview App's device list for the
// receiver fixture's rows, so the preview shows the page with realistic
// receivers.
func (a *App) applyReceiverFixtures() {
	fixed := receiverFixture()
	a.snap = fixed.snap
}

// receiverFixture is an App with one visible remembered HomePod, one hidden
// HomePod mini, one online manual AirPort Express, and one stereo group,
// mirroring the mockup's list.
func receiverFixture() *App {
	a := previewApp()
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
	return a
}

func TestReceiverPageRendersGroupsAndRows(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverHiddenOpen = true
	tt := ui.NewTester(a.settingsView, 720, 500)
	for _, want := range []string{
		"接收器", "管理接收器、自动连接、可见性与每台接收器的音画同步。",
		"已启用", "客厅 HomePod", "书房 AirPort Express", "客厅立体声",
		"自动连接", "手动添加", "添加接收器", "已隐藏", "卧室 HomePod mini",
		"AudioAccessory5,1 · 192.168.1.20:7000",
		"AirPort Express · 192.168.1.32:5000",
		"AudioAccessory5,1 · 立体声组 2/2",
		"AudioAccessory5,1 · 离线",
	} {
		if !tt.HasText(want) {
			t.Fatalf("receiver page is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

// TestReceiverPageEmptyListShowsDiscoveryHint pins the empty page to plain
// prose: one discovery line where the list would be, the add row, and no
// 已启用 head at zero rows.
func TestReceiverPageEmptyListShowsDiscoveryHint(t *testing.T) {
	a := previewApp()
	a.page = pageReceivers
	a.snap.Receivers = nil
	a.snap.Devices = nil
	tt := ui.NewTester(a.settingsView, 720, 500)
	if !tt.HasText(locales.ReceiverDiscoveryHint()) {
		t.Fatalf("discovery hint is missing: %q", tt.Texts())
	}
	if !tt.HasText(locales.ReceiverAddTitle()) {
		t.Fatalf("add row is missing: %q", tt.Texts())
	}
	if tt.HasText(locales.ReceiverGroupEnabled()) {
		t.Fatalf("已启用 head rendered with zero rows: %q", tt.Texts())
	}
	if tt.HasText("没有可见的接收器") {
		t.Fatalf("old empty card copy is on the page: %q", tt.Texts())
	}
}

// TestReceiverPageAllHiddenShowsProse pins the all-hidden page to two prose
// lines where the list would be, then the add row and the hidden group.
func TestReceiverPageAllHiddenShowsProse(t *testing.T) {
	a := previewApp()
	a.page = pageReceivers
	a.snap.Receivers = []receivers.View{
		{
			Key: "AA:01", Name: "卧室 HomePod mini", Kind: receivers.KindDiscovered,
			Model: "AudioAccessory5,1", Hidden: true, DelayMS: 120,
			Device: device.Device{ID: "AA:01", Name: "卧室 HomePod mini", Model: "AudioAccessory5,1"},
			Entry:  receivers.Entry{Key: "AA:01", Kind: receivers.KindDiscovered, Name: "卧室 HomePod mini", Hidden: true, DelayMS: 120},
		},
	}
	a.snap.Devices = nil
	tt := ui.NewTester(a.settingsView, 720, 500)
	if !tt.HasText(locales.ReceiverEmptyAllHiddenTitle()) {
		t.Fatalf("all-hidden title is missing: %q", tt.Texts())
	}
	if !tt.HasText(locales.ReceiverEmptyHiddenHint()) {
		t.Fatalf("hidden hint is missing: %q", tt.Texts())
	}
	if !tt.HasText(locales.ReceiverAddTitle()) {
		t.Fatalf("add row is missing: %q", tt.Texts())
	}
	if tt.HasText(locales.ReceiverGroupEnabled()) {
		t.Fatalf("已启用 head rendered with zero visible rows: %q", tt.Texts())
	}
	if !tt.HasText(locales.ReceiverGroupHidden()) {
		t.Fatalf("hidden group is missing: %q", tt.Texts())
	}
}

func TestReceiverPageScrollsAtMinimumHeight(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverOpen = map[string]bool{"AA:00": true}
	tt := ui.NewTester(a.settingsView, 720, 420)
	tt.Frame()
	before, ok := tt.Find("已隐藏")
	if !ok {
		t.Fatalf("the hidden group head is missing: %q", tt.Texts())
	}
	tt.Scroll(500, 300, 0, 200)
	tt.Frame()
	after, ok := tt.Find("已隐藏")
	if !ok {
		t.Fatalf("the hidden group head is missing after scrolling: %q", tt.Texts())
	}
	if after.Y >= before.Y {
		t.Fatalf("the receiver page did not scroll: %q %.1f -> %.1f", "已隐藏", before.Y, after.Y)
	}
}

func TestReceiverExpandedRowShowsConfig(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverOpen = map[string]bool{"m:ab12": true, "AA:00": true}
	tt := ui.NewTester(a.settingsView, 720, 500)
	for _, want := range []string{
		"启动时自动连接", "同一时间仅一台",
		"DSP/DAC 延迟", "补偿接收器的硬件音频延迟", "0 ms", "2000 ms",
		"接收器密码", "已保存 · 用于 AirPlay 密码认证", "更改", "••••••",
		"编辑",
	} {
		if !tt.HasText(want) {
			t.Fatalf("expanded receiver row is missing %q; texts: %q", want, tt.Texts())
		}
	}
	// The offline remembered row shows 未设置 for its password.
	if !tt.HasText("未设置 · 仅在这台接收器要求密码时填写") {
		t.Fatalf("unset password copy is missing; texts: %q", tt.Texts())
	}
}

// TestReceiverDelayRowAlignsWithSettingRows pins the delay row to the same
// left inset as the auto-connect and password rows.
func TestReceiverDelayRowAlignsWithSettingRows(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverOpen = map[string]bool{"AA:00": true}
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	auto, ok := tt.Find("启动时自动连接")
	if !ok {
		t.Fatalf("auto-connect row is missing: %q", tt.Texts())
	}
	delay, ok := tt.Find("DSP/DAC 延迟")
	if !ok {
		t.Fatalf("delay row is missing: %q", tt.Texts())
	}
	if delay.X != auto.X {
		t.Fatalf("delay row is not aligned with setting rows: auto x=%.1f delay x=%.1f", auto.X, delay.X)
	}
}

// TestEnglishReceiverAuthLabelFitsOneLine keeps the English auth label from
// wrapping and pushing its select below the port input.
func TestEnglishReceiverAuthLabelFitsOneLine(t *testing.T) {
	withLocale(t, "en")
	a := receiverFixture()
	a.page = pageReceivers
	a.openAddReceiver()
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	label, ok := tt.Find(locales.ReceiverFieldAuth())
	if !ok {
		t.Fatalf("auth label is missing: %q", tt.Texts())
	}
	if label.H > 18 {
		t.Fatalf("english auth label wraps: height %.1f, want one line", label.H)
	}
}

// TestReceiverHiddenExpansionKeepsEnabledCardHeight pins the enabled card
// against the hidden group and hidden row expansion animations, which used to
// squeeze it for a frame before the scroll content settled.
func TestReceiverHiddenExpansionKeepsEnabledCardHeight(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.SetDark(false)
	tt.Frame()
	row, ok := tt.Find("客厅 HomePod")
	if !ok {
		t.Fatalf("enabled receiver row is missing: %q", tt.Texts())
	}
	x := int(row.X) - 6
	before := enabledCardHeight(t, tt, x)
	if err := tt.Click(locales.ReceiverGroupHidden()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 16; i++ {
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
		if got := enabledCardHeight(t, tt, x); got != before {
			t.Fatalf("hidden group animation squeezed the enabled card: %d -> %d at frame %d", before, got, i)
		}
	}
	if err := tt.Click("卧室 HomePod mini"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 16; i++ {
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
		if got := enabledCardHeight(t, tt, x); got != before {
			t.Fatalf("hidden row animation squeezed the enabled card: %d -> %d at frame %d", before, got, i)
		}
	}
}

// enabledCardHeight measures the first run of card-colored pixels at x. The
// enabled receiver card is the first card below the group head.
func enabledCardHeight(t *testing.T, tt *ui.Tester, x int) int {
	t.Helper()
	p := paletteFor(false)
	card := p.card
	top, bottom := -1, -1
	img := tt.Image()
	for y := 0; y < img.Bounds().Dy(); y++ {
		c := img.RGBAAt(x, y)
		if c.R == card.R && c.G == card.G && c.B == card.B {
			if top < 0 {
				top = y
			}
			bottom = y
			continue
		}
		if top >= 0 {
			break
		}
	}
	if top < 0 {
		t.Fatalf("enabled card is not visible at x=%d", x)
	}
	return bottom - top + 1
}

// TestCollapsingHiddenGroupCollapsesHiddenRows pins the group's collapse to
// also dropping the expanded state of the hidden rows inside it.
func TestCollapsingHiddenGroupCollapsesHiddenRows(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverHiddenOpen = true
	a.receiverOpen = map[string]bool{"AA:01": true}
	tt := ui.NewTester(a.settingsView, 720, 500)
	tt.Frame()
	if !tt.HasText("卧室 HomePod mini") {
		t.Fatalf("hidden receiver is missing: %q", tt.Texts())
	}
	if err := tt.Click(locales.ReceiverGroupHidden()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.receiverHiddenOpen {
		t.Fatal("hidden group did not collapse")
	}
	if a.receiverOpen["AA:01"] {
		t.Fatal("hidden receiver stayed expanded after collapsing the group")
	}
}

func TestReceiverAddDialogRenders(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.openAddReceiver()
	tt := ui.NewTester(a.settingsView, 720, 500)
	for _, want := range []string{
		"添加接收器", "用于无法通过 mDNS 发现的接收器。",
		"名称", "类型", "AirPort Express", "IP 地址或主机名", "端口", "常用 5000 或 7000",
		"密码认证", "无", "取消", "添加",
	} {
		if !tt.HasText(want) {
			t.Fatalf("add dialog is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestReceiverAddDialogValidatesInPlace(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.openAddReceiver()
	tt := ui.NewTester(a.settingsView, 720, 500)
	a.receiverDraft.name = "书房"
	if got := a.receiverDraft.errorMsg; got != "" {
		t.Fatal("a fresh draft carries no error")
	}
	// Empty host fails without closing the dialog.
	a.submitReceiverDialog()
	if !a.receiverDialogOpen {
		t.Fatal("the dialog closed despite the invalid draft")
	}
	if a.receiverDraft.errorMsg == "" {
		t.Fatalf("no error surfaced: %q", tt.Texts())
	}
}

func TestReceiverEditDialogFillsAndSaves(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.beginEditReceiver(a.snap.Receivers[2]) // the manual AirPort Express
	tt := ui.NewTester(a.settingsView, 720, 500)
	for _, want := range []string{"编辑接收器", "修改手动添加的接收器信息。", "书房 AirPort Express", "保存", "移除接收器"} {
		if !tt.HasText(want) {
			t.Fatalf("edit dialog is missing %q; texts: %q", want, tt.Texts())
		}
	}
	if a.receiverDraft.port != "5000" {
		t.Fatalf("draft port = %q", a.receiverDraft.port)
	}
}

func TestReceiverPasswordDialogRenders(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverDialog = receiverDialogState{kind: receiverDialogPassword, key: "m:ab12", entered: false}
	a.receiverDialogOpen = true
	tt := ui.NewTester(a.settingsView, 720, 500)
	for _, want := range []string{
		"更改接收器密码", "书房 AirPort Express · AirPlay 密码认证",
		"仅保存在本机；连接这台接收器时使用，不会写入诊断日志。",
		"清除已保存密码", "保存",
	} {
		if !tt.HasText(want) {
			t.Fatalf("password dialog is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestReceiverRemoveDialogRenders(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.openRemoveReceiver(a.snap.Receivers[2])
	tt := ui.NewTester(a.settingsView, 720, 500)
	for _, want := range []string{
		"移除接收器？", "书房 AirPort Express",
		"会从设置和设备列表中移除这台接收器；接收器本身不会被更改。",
		"移除",
	} {
		if !tt.HasText(want) {
			t.Fatalf("remove dialog is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestReceiverEditButtonOpensEditWithoutCollapsing(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverOpen = map[string]bool{"m:ab12": true}
	tt := ui.NewTester(a.settingsView, 720, 500)
	if err := tt.Click("编辑"); err != nil {
		t.Fatalf("click the edit action: %v", err)
	}
	if !a.receiverDialogOpen || a.receiverDialog.kind != receiverDialogEdit {
		t.Fatalf("edit dialog state = %+v open=%v", a.receiverDialog, a.receiverDialogOpen)
	}
	if !a.receiverOpen["m:ab12"] {
		t.Fatal("the edit click also collapsed the row")
	}
}

func TestReceiverPasswordRowOpensItsDialog(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverOpen = map[string]bool{"m:ab12": true}
	tt := ui.NewTester(a.settingsView, 720, 500)
	if err := tt.Click("更改"); err != nil {
		t.Fatalf("click the password action: %v", err)
	}
	if !a.receiverDialogOpen || a.receiverDialog.kind != receiverDialogPassword {
		t.Fatalf("password dialog state = %+v open=%v", a.receiverDialog, a.receiverDialogOpen)
	}
	if !a.receiverOpen["m:ab12"] {
		t.Fatal("the password click also collapsed the row")
	}
	tt.Frame()
	if !tt.HasText("更改接收器密码") {
		t.Fatalf("password dialog did not render: %q", tt.Texts())
	}
}

func TestReceiverDelayDraftFeedsTheSliderAndALACLine(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverOpen = map[string]bool{"AA:00": true}
	a.receiverDelayPend = map[string]int{"AA:00": 120}
	tt := ui.NewTester(a.settingsView, 720, 500)
	if !tt.HasText("120 ms") {
		t.Fatalf("delay draft is not on the row: %q", tt.Texts())
	}
	a.latencyMode = settings.LatencyLow
	a.latency = 200
	a.snap.State = sender.StateStreaming
	a.snap.SelectedID = "AA:00"
	if got := a.latencyDisplay(); got != "320 ms" {
		t.Fatalf("ALAC latency = %q, want 320 ms", got)
	}
	a.receiverDelayPend = nil
	a.snap.State = sender.StateIdle
	if got := a.latencyDisplay(); got != "200 ms" {
		t.Fatalf("idle ALAC latency = %q, want the base 200 ms", got)
	}
}

func TestFlyoutALACLineShowsActiveReceiverDelay(t *testing.T) {
	a := previewApp()
	a.latencyMode = settings.LatencyLow
	a.latency = 200
	a.snap.SelectedID = "AA:BB:CC:DD:EE:01"
	a.snap.Receivers = []receivers.View{{Key: "AA:BB:CC:DD:EE:01", DelayMS: 120}}
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(len(a.snap.Devices), a.showLatencySwitch))
	if !tt.HasText("延迟 320 ms") {
		t.Fatalf("ALAC line lacks the effective latency: %q", tt.Texts())
	}
}

func TestFlyoutFiltersHiddenReceivers(t *testing.T) {
	// The engine snapshot is already the filtered list; this pins the
	// contract the flyout draws: what snap.Devices carries is visible, what
	// it drops is not.
	a := receiverFixture()
	tt := ui.NewTester(a.view, flyoutWidth, flyoutHeightFor(len(a.snap.Devices), a.showLatencySwitch))
	if !tt.HasText("客厅 HomePod") {
		t.Fatalf("visible receiver missing from the flyout: %q", tt.Texts())
	}
	if tt.HasText("卧室 HomePod mini") || tt.HasText("书房 AirPort Express") {
		t.Fatalf("hidden or manual rows appeared in the flyout: %q", tt.Texts())
	}
}
