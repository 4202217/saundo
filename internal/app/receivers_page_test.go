package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/engine"
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
	a.snap.ActiveTargets = []string{"AA:00"}
	a.snap.Targets = []engine.TargetControl{{
		ID: "AA:00", Name: "客厅 HomePod", Volume: 65, Channel: receivers.ChannelStereo,
		State: engine.TargetMember,
	}}
	a.selID = "AA:00"
	a.snap.Devices = []device.Device{
		{ID: "AA:00", Name: "客厅 HomePod", Host: "192.168.1.20", Port: 7000, Model: "AudioAccessory5,1", Family: device.FamilyHomePod, Auth: device.AuthHAPTransient, AirPlay2: true, Online: true},
	}
	a.snap.Receivers = []receivers.View{
		{
			Key: "AA:00", Name: "客厅 HomePod", Kind: receivers.KindDiscovered,
			Model: "AudioAccessory5,1", Online: true, AutoConnect: true, DelayMS: 0,
			Channel: receivers.ChannelStereo,
			Device:  a.snap.Devices[0],
			Entry:   receivers.Entry{Key: "AA:00", Kind: receivers.KindDiscovered, Name: "客厅 HomePod", AutoConnect: true},
		},
		{
			// A second eligible AirPlay 2 receiver, so the combo section has two
			// candidates to offer.
			Key: "AA:09", Name: "卧室 HomePod", Kind: receivers.KindDiscovered,
			Model: "AudioAccessory5,1", Online: true,
			Channel: receivers.ChannelStereo,
			Device:  device.Device{ID: "AA:09", Name: "卧室 HomePod", Host: "192.168.1.21", Port: 7000, Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true, Online: true},
			Entry:   receivers.Entry{Key: "AA:09", Kind: receivers.KindDiscovered, Name: "卧室 HomePod"},
		},
		{
			Key: "AA:01", Name: "卧室 HomePod mini", Kind: receivers.KindDiscovered,
			Model: "AudioAccessory5,1", Hidden: true, DelayMS: 120,
			Channel: receivers.ChannelStereo,
			Device:  device.Device{ID: "AA:01", Name: "卧室 HomePod mini", Model: "AudioAccessory5,1"},
			Entry:   receivers.Entry{Key: "AA:01", Kind: receivers.KindDiscovered, Name: "卧室 HomePod mini", Hidden: true, DelayMS: 120},
		},
		{
			Key: "m:ab12", Name: "书房 AirPort Express", Kind: receivers.KindManual,
			Type: receivers.TypeAirPortExpress, Address: "192.168.1.32:5000",
			Model: "AirPort", Online: true, PasswordSet: true,
			Channel: receivers.ChannelLeft,
			Device:  device.Device{ID: "m:ab12", Name: "书房 AirPort Express", Model: "AirPort", Auth: device.AuthSetup, Online: true},
			Entry:   receivers.Entry{Key: "m:ab12", Kind: receivers.KindManual, Type: receivers.TypeAirPortExpress, Name: "书房 AirPort Express", Host: "192.168.1.32", Port: 5000},
		},
		{
			Key: "stereo:1", Name: "客厅立体声", Kind: receivers.KindDiscovered,
			Model: "AudioAccessory5,1", Online: true, Stereo: true,
			Channel: receivers.ChannelStereo,
			Device: device.Device{ID: "stereo:1", Name: "客厅立体声", Model: "AudioAccessory5,1", StereoID: "1",
				Members: []device.Device{{ID: "AA:10", Name: "L", Online: true}, {ID: "AA:11", Name: "R", Online: true}}, Online: true},
			Entry: receivers.Entry{Key: "stereo:1", Kind: receivers.KindDiscovered, Name: "客厅立体声"},
		},
	}
	return a
}

func TestReceiverPageRendersGroupsAndRows(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverHiddenOpen = true
	tt := ui.NewTester(a.settingsView, 720, 560)
	for _, want := range []string{
		"接收器", "管理接收器、自动连接、显示与声道。",
		locales.ReceiverGroupEnabled(), "客厅 HomePod", "书房 AirPort Express", "客厅立体声",
		"启动时连接", "手动添加", locales.ReceiverAddAction(), locales.ReceiverPairAddTitle(), "已隐藏", "卧室 HomePod mini",
		locales.DeviceAuthSetup(),
		"192.168.1.20:7000 · 立体声 · HomePod mini",
		"192.168.1.32:5000 · 左声道 · AirPort Express",
		"2/2",
	} {
		if !tt.HasText(want) {
			t.Fatalf("receiver page is missing %q; texts: %q", want, tt.Texts())
		}
	}
	if tt.HasText("AudioAccessory5,1") {
		t.Fatalf("a model code reached the receiver page; texts: %q", tt.Texts())
	}
}

// localReceiverFixture is the page with one receiver: this host's own
// AirPlay endpoint, already forced into the hidden group.
func localReceiverFixture() *App {
	a := previewApp()
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
	return a
}

// TestReceiverPageLocalRowIsDisabled pins the local row's shape: the 本机
// chip in the hidden group, with no enable switch and no expander, even when
// a stale open flag is set.
func TestReceiverPageLocalRowIsDisabled(t *testing.T) {
	a := localReceiverFixture()
	a.receiverOpen = map[string]bool{"02:8F:B8:25:3A:B2": true}
	tt := ui.NewTester(a.settingsView, 720, 560)
	for _, want := range []string{"MacBook Air", locales.ReceiverChipLocal(), locales.ReceiverGroupHidden()} {
		if !tt.HasText(want) {
			t.Fatalf("local row is missing %q; texts: %q", want, tt.Texts())
		}
	}
	if _, ok := tt.Find(locales.ReceiverEnableSwitch()); ok {
		t.Fatal("the local row still offers the enable switch")
	}
	if tt.HasText(locales.ReceiverAutoToggle()) {
		t.Fatal("the local row expanded its body")
	}
}

// TestReceiverPageEmptyListShowsTheAddRow pins the empty page: the singles
// head with no count pill and no head action, the add row as its content, and
// no combo section because there is nothing to group.
func TestReceiverPageEmptyListShowsTheAddRow(t *testing.T) {
	a := previewApp()
	a.page = pageReceivers
	a.snap.Receivers = nil
	a.snap.Devices = nil
	tt := ui.NewTester(a.settingsView, 720, 560)
	if !tt.HasText(locales.ReceiverGroupEnabled()) {
		t.Fatalf("the singles head is missing: %q", tt.Texts())
	}
	if !tt.HasText(locales.ReceiverAddTitle()) {
		t.Fatalf("add row is missing: %q", tt.Texts())
	}
	if got := countText(tt, locales.ReceiverAddAction()); got != 0 {
		t.Fatalf("the empty head offers a duplicate add action: %d", got)
	}
	if tt.HasText(locales.ReceiverGroupCombo()) || tt.HasText(locales.ReceiverPairAddTitle()) {
		t.Fatalf("the combo section rendered with no receivers: %q", tt.Texts())
	}
}

// TestReceiverPageAllHiddenShowsTheListHead pins the all-hidden page: the
// same head and add row as an empty page, over the hidden group.
func TestReceiverPageAllHiddenShowsTheListHead(t *testing.T) {
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
	tt := ui.NewTester(a.settingsView, 720, 560)
	if !tt.HasText(locales.ReceiverGroupEnabled()) {
		t.Fatalf("the singles head is missing: %q", tt.Texts())
	}
	if !tt.HasText(locales.ReceiverAddTitle()) {
		t.Fatalf("add row is missing: %q", tt.Texts())
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
	mac := device.Device{
		ID: "AA:02", Name: "MacBook", Host: "192.168.1.30", Port: 7000,
		Model: "Mac14,2", Auth: device.AuthHAPPassword, Family: device.FamilyMac,
		AirPlay2: true, Online: true,
	}
	a.snap.Devices = append(a.snap.Devices, mac)
	a.snap.Receivers = append(a.snap.Receivers, receivers.View{
		Key: "AA:02", Name: "MacBook", Kind: receivers.KindDiscovered,
		Model: "Mac14,2", Online: true, Device: mac,
		Entry: receivers.Entry{Key: "AA:02", Kind: receivers.KindDiscovered, Name: "MacBook"},
	})
	a.page = pageReceivers
	a.receiverOpen = map[string]bool{"m:ab12": true, "AA:00": true, "AA:02": true}
	tt := ui.NewTester(a.settingsView, 720, 560)
	for _, want := range []string{
		"启动时自动连接", locales.ReceiverAutoToggleDesc(),
		"接收器密码", "已保存 · 用于 AirPlay 密码认证", "更改", "••••••",
		"编辑",
	} {
		if !tt.HasText(want) {
			t.Fatalf("expanded receiver row is missing %q; texts: %q", want, tt.Texts())
		}
	}
	// The offline remembered row shows 未保存 for its password.
	if !tt.HasText("未保存 · 连接时按需填写") {
		t.Fatalf("unset password copy is missing; texts: %q", tt.Texts())
	}
}

// TestEnglishReceiverAuthLabelFitsOneLine keeps the English auth label from
// wrapping and pushing its select below the port input.
func TestEnglishReceiverAuthLabelFitsOneLine(t *testing.T) {
	withLocale(t, "en")
	a := receiverFixture()
	a.page = pageReceivers
	a.openAddReceiver()
	tt := ui.NewTester(a.settingsView, 720, 560)
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
	tt := ui.NewTester(a.settingsView, 720, 560)
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
	tt := ui.NewTester(a.settingsView, 720, 560)
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
	tt := ui.NewTester(a.settingsView, 720, 560)
	for _, want := range []string{
		"添加接收器", "填写名称、地址与认证方式。",
		"名称", "类型", "AirPort Express", "IP 地址或主机名", "端口", "5000 或 7000",
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
	tt := ui.NewTester(a.settingsView, 720, 560)
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
	tt := ui.NewTester(a.settingsView, 720, 560)
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
	tt := ui.NewTester(a.settingsView, 720, 560)
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
	tt := ui.NewTester(a.settingsView, 720, 560)
	for _, want := range []string{
		"移除接收器？", "书房 AirPort Express",
		"会从设置和托盘面板中移除；接收器本身不受影响。",
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
	tt := ui.NewTester(a.settingsView, 720, 560)
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
	tt := ui.NewTester(a.settingsView, 720, 700)
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

func TestFlyoutHeadShowsActiveReceiverDelay(t *testing.T) {
	a := previewApp()
	a.latencyMode = settings.LatencyLow
	a.latency = 200
	a.snap.SelectedID = "AA:BB:CC:DD:EE:01"
	a.snap.Receivers = []receivers.View{{Key: "AA:BB:CC:DD:EE:01", DelayMS: 120}}
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	if !tt.HasText("320 ms") {
		t.Fatalf("the playback head lacks the effective latency: %q", tt.Texts())
	}
}

func TestFlyoutFiltersHiddenReceivers(t *testing.T) {
	// The engine snapshot is already the filtered list; this pins the
	// contract the flyout draws: what snap.Devices carries is visible, what
	// it drops is not.
	a := receiverFixture()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	if !tt.HasText("客厅 HomePod") {
		t.Fatalf("visible receiver missing from the flyout: %q", tt.Texts())
	}
	if tt.HasText("卧室 HomePod mini") || tt.HasText("书房 AirPort Express") {
		t.Fatalf("hidden or manual rows appeared in the flyout: %q", tt.Texts())
	}
}

// comboFixture adds a created combo over the fixture's two receivers: the
// members drop from the list and the combo carries the synthesized group.
func comboFixture() *App {
	a := receiverFixture()
	// The fixture's row order is an implementation detail; find the members by
	// key so an added row cannot silently change which receivers link.
	var left, right, hidden receivers.View
	for _, v := range a.snap.Receivers {
		switch v.Key {
		case "AA:00":
			left = v
		case "m:ab12":
			right = v
		case "AA:01":
			hidden = v
		}
	}
	members := []device.Device{left.Device, right.Device}
	combo := receivers.View{
		Key: "combo:01", Name: "客厅立体声", Kind: receivers.KindCombo,
		Model: left.Device.Model, Online: true, Stereo: true,
		Device: device.Device{
			ID: "combo:01", Name: "客厅立体声", Model: left.Device.Model,
			StereoID: "combo:01", Members: members, Online: true,
		},
		Entry: receivers.Entry{
			Key: "combo:01", Kind: receivers.KindCombo, Name: "客厅立体声",
			Members: []receivers.Member{{ID: left.Key, Name: left.Name}, {ID: right.Key, Name: right.Name}},
		},
	}
	a.snap.SelectedID = "combo:01"
	a.snap.ConnectedName = "客厅立体声"
	a.snap.ActiveTargets = []string{"combo:01"}
	a.snap.Targets = []engine.TargetControl{{
		ID: "combo:01", Name: "客厅立体声", Volume: 65, Channel: receivers.ChannelStereo,
		State: engine.TargetMember, Split: true, Stereo: true,
	}}
	a.selID = "combo:01"
	a.snap.Receivers = []receivers.View{combo, hidden}
	return a
}

func TestReceiverComboRowRendersTheChannelMap(t *testing.T) {
	a := comboFixture()
	a.page = pageReceivers
	a.receiverOpen = map[string]bool{"combo:01": true}
	tt := ui.NewTester(a.settingsView, 720, 560)
	for _, want := range []string{
		locales.ReceiverGroupCombo(), locales.ComboTag(), locales.ComboMapLabel(),
		locales.ChannelLeft(), locales.ChannelRight(), locales.ReceiverEditButton(),
		locales.VolumeLabel(),
	} {
		if !tt.HasText(want) {
			t.Fatalf("combo row is missing %q; texts: %q", want, tt.Texts())
		}
	}
	// The row offers the edit action; the destructive unlink moved into the
	// dialog behind it.
	if tt.HasText(locales.ComboUnlink()) {
		t.Fatalf("the combo row still offers the unlink action: %q", tt.Texts())
	}
	// The combo replaces its members: the singles section has no rows, so its
	// head carries no action and the add row stands in for the card.
	if got := countText(tt, locales.ReceiverAddAction()); got != 0 {
		t.Fatalf("the empty singles head offers an add action beside the add row: %d", got)
	}
	if !tt.HasText(locales.ReceiverAddTitle()) {
		t.Fatal("the empty singles section lost its add row")
	}
}

func TestReceiverComboUnlinkOpensTheConfirmDialog(t *testing.T) {
	a := comboFixture()
	a.page = pageReceivers
	a.receiverOpen = map[string]bool{"combo:01": true}
	tt := ui.NewTester(a.settingsView, 720, 560)
	if err := tt.Click(locales.ReceiverEditButton()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText(locales.ComboEditTitle()) {
		t.Fatalf("the edit dialog did not open; texts: %q", tt.Texts())
	}
	if err := tt.Click(locales.ComboUnlink()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText(locales.ComboUnlinkTitle()) {
		t.Fatalf("the unlink dialog did not open; texts: %q", tt.Texts())
	}
}

func TestReceiverComboCreateDialogOpensAndNames(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	tt := ui.NewTester(a.settingsView, 720, 560)
	if err := tt.Click(locales.ReceiverPairAddTitle()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, want := range []string{locales.ComboFieldLeft(), locales.ComboFieldRight(), locales.ComboPickEmpty()} {
		if !tt.HasText(want) {
			t.Fatalf("the create dialog is missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestComboDetailUsesMemberNames(t *testing.T) {
	members := []device.Device{{Name: "HomePod L"}, {Name: "HomePod R"}}
	if got := comboDetail("客厅立体声", members); got != "HomePod L · HomePod R" {
		t.Fatalf("custom-name detail = %q, want the member names", got)
	}
	if got := comboDetail("HomePod L · HomePod R", members); got != "" {
		t.Fatalf("derived-name detail = %q, want no repeat", got)
	}
	if got := comboDetail("客厅立体声", nil); got != "" {
		t.Fatalf("memberless detail = %q, want a blank line", got)
	}
}

func TestReceiverDetailCarriesAddressChannelFamilyAndVersion(t *testing.T) {
	v := receivers.View{
		Key: "AA:00", Channel: receivers.ChannelLeft,
		Device: device.Device{ID: "AA:00", Host: "192.168.1.20", Port: 7000,
			Family: device.FamilyHomePod, OSVersion: "27.2", Online: true},
	}
	if got, want := receiverDetail(v, true), "192.168.1.20:7000 · 左声道 · HomePod · 27.2"; got != want {
		t.Fatalf("detail = %q, want %q", got, want)
	}
	v.Channel = receivers.ChannelRight
	if got, want := receiverDetail(v, true), "192.168.1.20:7000 · 右声道 · HomePod · 27.2"; got != want {
		t.Fatalf("right detail = %q, want %q", got, want)
	}
	v.Channel = receivers.ChannelStereo
	if got, want := receiverDetail(v, true), "192.168.1.20:7000 · 立体声 · HomePod · 27.2"; got != want {
		t.Fatalf("stereo detail = %q, want %q", got, want)
	}
	pair := receivers.View{Stereo: true, Device: device.Device{Members: []device.Device{
		{Host: "192.168.31.46", Port: 7000}, {Host: "192.168.31.164", Port: 7000},
	}}}
	if got, want := receiverDetail(pair, true), "192.168.31.46:7000 · 192.168.31.164:7000"; got != want {
		t.Fatalf("pair detail = %q, want the two member addresses", got)
	}
	mini := v
	mini.Device.Model = "AudioAccessory5,1"
	if got, want := receiverDetail(mini, true), "192.168.1.20:7000 · 立体声 · HomePod mini · 27.2"; got != want {
		t.Fatalf("HomePod mini detail = %q, want the marketing name", got)
	}
	if got, want := receiverDetail(mini, false), "192.168.1.20:7000 · 立体声 · HomePod mini · 27.2"; got != want {
		t.Fatalf("detail with model off = %q, want the base name", got)
	}
	gen2 := mini
	gen2.Device.Model = "AudioAccessory6,1"
	if got, want := receiverDetail(gen2, true), "192.168.1.20:7000 · 立体声 · HomePod (2nd gen) · 27.2"; got != want {
		t.Fatalf("second-generation HomePod detail = %q, want %q", got, want)
	}
	if got, want := receiverDetail(gen2, false), "192.168.1.20:7000 · 立体声 · HomePod · 27.2"; got != want {
		t.Fatalf("second-generation HomePod detail with model off = %q, want %q", got, want)
	}
	mac := mini
	mac.Device.Model = "Mac14,2"
	mac.Device.Family = device.FamilyMac
	if got, want := receiverDetail(mac, true), "192.168.1.20:7000 · 立体声 · MacBook Air (M2, 2022) · 27.2"; got != want {
		t.Fatalf("Mac detail = %q, want %q", got, want)
	}
	if got, want := receiverDetail(mac, false), "192.168.1.20:7000 · 立体声 · MacBook Air · 27.2"; got != want {
		t.Fatalf("Mac detail with model off = %q, want %q", got, want)
	}
	miniPair := receivers.View{Stereo: true, Device: device.Device{
		Model:   "AudioAccessory5,1",
		Members: []device.Device{{Host: "192.168.31.46", Port: 7000}, {Host: "192.168.31.164", Port: 7000}},
	}}
	if got, want := receiverDetail(miniPair, true), "HomePod mini · 192.168.31.46:7000 · 192.168.31.164:7000"; got != want {
		t.Fatalf("mini pair detail = %q, want the model before the addresses", got)
	}
}

func TestFlyoutDetailDropsTheAddress(t *testing.T) {
	single := receivers.View{Channel: receivers.ChannelLeft,
		Device: device.Device{Family: device.FamilyHomePod, OSVersion: "27.2", Online: true}}
	if got, want := flyoutDetail(single, true), "左声道 · HomePod · 27.2"; got != want {
		t.Fatalf("single detail = %q, want %q", got, want)
	}
	pair := receivers.View{Stereo: true, Channel: receivers.ChannelStereo,
		Device: device.Device{Model: "Mac14,2", Family: device.FamilyMac, OSVersion: "15.2", Online: true}}
	if got, want := flyoutDetail(pair, true), "MacBook Air (M2, 2022) · 15.2"; got != want {
		t.Fatalf("pair detail = %q, want %q", got, want)
	}
	if strings.Contains(flyoutDetail(pair, true), "Mac14,2") {
		t.Fatalf("the flyout detail kept the model identifier")
	}
	if got, want := flyoutDetail(pair, false), "MacBook Air · 15.2"; got != want {
		t.Fatalf("flyout detail with model off = %q, want the base name", got)
	}
	mini := receivers.View{Channel: receivers.ChannelLeft,
		Device: device.Device{Model: "AudioAccessory5,1", Family: device.FamilyHomePod, OSVersion: "27.3", Online: true}}
	if got, want := flyoutDetail(mini, true), "左声道 · HomePod mini · 27.3"; got != want {
		t.Fatalf("HomePod mini flyout detail = %q, want %q", got, want)
	}
	tv := receivers.View{Channel: receivers.ChannelStereo,
		Device: device.Device{Model: "AppleTV6,2", Family: device.FamilyAppleTV, Online: true}}
	if got, want := flyoutDetail(tv, true), "立体声 · Apple TV 4K (1st gen)"; got != want {
		t.Fatalf("Apple TV flyout detail = %q, want %q", got, want)
	}
	unknownTV := tv
	unknownTV.Device.Model = "AppleTV99,9"
	if got, want := flyoutDetail(unknownTV, true), "立体声 · Apple TV"; got != want {
		t.Fatalf("unknown Apple TV detail = %q, want the family fallback", got)
	}
	if got, want := flyoutDetail(tv, false), "立体声 · Apple TV 4K"; got != want {
		t.Fatalf("Apple TV detail with model off = %q, want the base name", got)
	}
}

func TestReceiverPageModelDetailSwitch(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	tt := ui.NewTester(a.settingsView, 720, 700)
	if !tt.HasText(locales.ReceiverModelDetailShort()) || !tt.HasText(locales.ReceiverModelDetail()) ||
		!tt.HasText(locales.ReceiverModelDetailDesc()) {
		t.Fatalf("the model-detail switch is missing: %q", tt.Texts())
	}
	if err := tt.Click(locales.ReceiverModelDetail()); err != nil {
		t.Fatalf("model-detail switch click: %v", err)
	}
	// The default is off, so the click turns the switch on.
	if !a.cfg.ModelDetail {
		t.Fatal("the model-detail switch did not turn on")
	}
	for i := range a.snap.Receivers {
		if a.snap.Receivers[i].Key == "AA:00" {
			a.snap.Receivers[i].Device.Model = "Mac14,2"
			a.snap.Receivers[i].Device.Family = device.FamilyMac
		}
	}
	a.cfg.ModelDetail = true
	tt = ui.NewTester(a.settingsView, 720, 700)
	if !hasExactText(tt, "192.168.1.20:7000 · 立体声 · MacBook Air (M2, 2022)") {
		t.Fatalf("the row lost the model detail: %q", tt.Texts())
	}
	a.cfg.ModelDetail = false
	tt = ui.NewTester(a.settingsView, 720, 700)
	if hasExactText(tt, "192.168.1.20:7000 · 立体声 · MacBook Air (M2, 2022)") {
		t.Fatalf("the row kept the parenthetical with the switch off: %q", tt.Texts())
	}
	if !hasExactText(tt, "192.168.1.20:7000 · 立体声 · MacBook Air") {
		t.Fatalf("the row lost the base name: %q", tt.Texts())
	}
}

func TestInvalidComboRowIsMarked(t *testing.T) {
	a := flyoutComboFixture()
	a.page = pageReceivers
	for i := range a.snap.Receivers {
		if a.snap.Receivers[i].Key == "combo:01" {
			a.snap.Receivers[i].Invalid = true
		}
	}
	tt := ui.NewTester(a.settingsView, 720, 700)
	if !tt.HasText(locales.ComboInvalid()) || !tt.HasText(locales.ComboInvalidReason()) {
		t.Fatalf("the invalid combo row is not marked: %q", tt.Texts())
	}
	if tt.HasText("2/2") {
		t.Fatalf("the invalid combo still shows an online count: %q", tt.Texts())
	}
	// The flyout lists only connectable targets, so the invalid combo is not
	// a row there at all. The engine's Visible projection drops it; model
	// that here and assert the other rows still render.
	a.snap.ActiveTargets = nil
	a.snap.Targets = nil
	a.snap.SelectedID = ""
	a.selID = ""
	kept := a.snap.Devices[:0]
	for _, d := range a.snap.Devices {
		if d.ID != "combo:01" {
			kept = append(kept, d)
		}
	}
	a.snap.Devices = kept
	tt = ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	if tt.HasText(locales.ComboInvalid()) || tt.HasText(locales.ComboInvalidReason()) || tt.HasText(locales.ComboTag()) {
		t.Fatalf("the flyout still lists the invalid combo: %q", tt.Texts())
	}
	if !tt.HasText("MacBook Air") || !tt.HasText("卧室") {
		t.Fatalf("the flyout dropped connectable rows with the invalid combo: %q", tt.Texts())
	}
}

func TestReceiverAuthPillStates(t *testing.T) {
	a := receiverFixture()
	needs := receivers.View{Key: "AA:00", Device: device.Device{ID: "AA:00", Auth: device.AuthHAPPassword}}
	if pill := a.receiverAuthPill(needs); !pill.ok || pill.text != locales.DeviceNeedsPassword() || pill.icon != iconLock {
		t.Fatalf("needs pill = %+v", pill)
	}
	saved := needs
	saved.PasswordSet = true
	if pill := a.receiverAuthPill(saved); !pill.ok || pill.text != locales.DevicePasswordSaved() || pill.icon != iconKey {
		t.Fatalf("saved pill = %+v", pill)
	}
	a.snap.ErrorKey = engine.ErrPasswordRejected
	a.snap.PasswordTargetID = "AA:00"
	if pill := a.receiverAuthPill(saved); !pill.ok || pill.text != locales.DevicePasswordError() || pill.icon != iconAlert {
		t.Fatalf("error pill = %+v", pill)
	}
}

// flyoutComboFixture renders the flyout with one created combo whose name is
// the derived default, mirroring a real linked pair of HomePods.
func flyoutComboFixture() *App {
	a := receiverFixture()
	combo := device.Device{
		ID: "combo:01", Name: "HomePod L + HomePod R", Model: "AudioAccessory5,1",
		StereoID: "combo:01", Online: true,
		Members: []device.Device{
			{ID: "AA:00", Name: "HomePod L", Model: "AudioAccessory5,1", Online: true},
			{ID: "AA:01", Name: "HomePod R", Model: "AudioAccessory5,1", Online: true},
		},
	}
	a.snap.Devices = []device.Device{combo,
		{ID: "mac:01", Name: "MacBook Air", Model: "Mac14,2", Online: true},
		{ID: "AA:02", Name: "卧室", Model: "AudioAccessory5,1", StereoID: "2", Online: true,
			Members: []device.Device{{ID: "AA:02", Name: "卧室"}, {ID: "AA:03", Name: "卧室 R"}}}}
	a.snap.Receivers = []receivers.View{{
		Key: "combo:01", Name: "HomePod L + HomePod R", Kind: receivers.KindCombo,
		Model: "AudioAccessory5,1", Online: true, Stereo: true, Device: combo,
		Entry: receivers.Entry{
			Key: "combo:01", Kind: receivers.KindCombo, Name: "HomePod L + HomePod R",
			Members: []receivers.Member{{ID: "AA:00", Name: "HomePod L"}, {ID: "AA:01", Name: "HomePod R"}},
		},
	}}
	// A live combo shows the mixer layout: one volume, no channel button.
	a.snap.ActiveTargets = []string{"combo:01"}
	a.snap.Targets = []engine.TargetControl{{
		ID: "combo:01", Name: "HomePod L + HomePod R", Volume: 40,
		Channel: receivers.ChannelStereo, Stereo: true, Split: true, State: engine.TargetMember,
	}}
	a.selID = "combo:01"
	a.snap.SelectedID = "combo:01"
	a.snap.ConnectedName = "HomePod L + HomePod R"
	return a
}

func TestFlyoutComboRowKeepsItsNameBesideTheLead(t *testing.T) {
	a := flyoutComboFixture()
	tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
	if !tt.HasText("HomePod L + HomePod R") {
		t.Fatalf("the combo name truncated beside the lead: %q", tt.Texts())
	}
	if tt.HasText(locales.ComboTag()) {
		t.Fatalf("the connected combo row still carries the combo chip: %q", tt.Texts())
	}
	if !tt.HasText(locales.VolumeFor("HomePod L + HomePod R")) {
		t.Fatalf("the combo mixer lost its volume: %q", tt.Texts())
	}
	if tt.HasText(locales.ChannelTooltip(locales.ChannelStereo(), locales.ChannelLeft())) {
		t.Fatalf("the combo row drew a channel button: %q", tt.Texts())
	}
}

func TestComboPickRowsExcludeTheOtherSide(t *testing.T) {
	candidates := []receivers.View{
		{Key: "AA:00", Name: "HomePod L"},
		{Key: "AA:01", Name: "HomePod R"},
		{Key: "m:02", Name: "书房"},
	}
	keys := func(views []receivers.View) []string {
		out := make([]string, 0, len(views))
		for _, v := range views {
			out = append(out, v.Key)
		}
		return out
	}
	left := keys(comboCandidatesExcept(candidates, "AA:01"))
	right := keys(comboCandidatesExcept(candidates, "AA:00"))
	if slices.Contains(left, "AA:01") {
		t.Fatalf("left side still offers the right side's receiver: %v", left)
	}
	if slices.Contains(right, "AA:00") {
		t.Fatalf("right side still offers the left side's receiver: %v", right)
	}
	if !slices.Contains(left, "AA:00") || !slices.Contains(right, "AA:01") {
		t.Fatalf("a side lost its own receiver: left=%v right=%v", left, right)
	}
	if len(comboCandidatesExcept(candidates, "")) != len(candidates) {
		t.Fatal("an empty exclusion dropped a receiver")
	}
}

// TestReceiverPageShowsSaveErrors pins the settings-side error surface: a
// refused save or link shows on the receivers page, because the dialog closes
// and the flyout is hidden while settings is open.
func TestReceiverPageShowsSaveErrors(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.snap.ErrorKey = engine.ErrSaveReceiver
	a.snap.ErrorArgs = []string{"disk full"}
	tt := ui.NewTester(a.settingsView, 720, 560)
	if !tt.HasText(locales.ErrorSaveReceiver("disk full")) {
		t.Fatalf("the save error is not on the page: %q", tt.Texts())
	}
	a.snap.ErrorKey = engine.ErrLatencyOffline
	tt = ui.NewTester(a.settingsView, 720, 560)
	if tt.HasText(locales.ErrorLatencyOffline()) {
		t.Fatal("an unrelated engine error rendered on the receivers page")
	}
}

// TestReceiverComboOfflineWarning pins the collapsed cue and the expanded
// bar: a combo with a member down reads 离线 on the row and names the member
// when opened.
func TestReceiverComboOfflineWarning(t *testing.T) {
	a := comboFixture()
	a.page = pageReceivers
	a.receiverOpen = map[string]bool{"combo:01": true}
	for i := range a.snap.Receivers {
		if a.snap.Receivers[i].Key != "combo:01" {
			continue
		}
		v := &a.snap.Receivers[i]
		v.Online = false
		v.Device.Online = false
		for j := range v.Device.Members {
			v.Device.Members[j].Online = false
		}
	}
	tt := ui.NewTester(a.settingsView, 720, 700)
	if !tt.HasText(locales.ActionOffline()) {
		t.Fatalf("the collapsed combo row has no offline cue: %q", tt.Texts())
	}
	if !tt.HasText(locales.ComboOfflineWarn("客厅 HomePod + 书房 AirPort Express")) {
		t.Fatalf("the expanded combo has no member warning: %q", tt.Texts())
	}
}

// TestComboCreateRefusesAnIncompleteForm pins the dialog's own validation:
// without both sides the commit stays in the dialog with the error line.
func TestComboCreateRefusesAnIncompleteForm(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverDialog = receiverDialogState{kind: receiverDialogComboCreate, entered: true, comboLeft: "AA:00"}
	a.receiverDialogOpen = true
	a.submitComboCreate()
	if !a.receiverDialogOpen {
		t.Fatal("the dialog closed without both sides")
	}
	if a.receiverDialog.comboError != locales.ComboErrorPick() {
		t.Fatalf("error = %q, want the pick hint", a.receiverDialog.comboError)
	}
}
