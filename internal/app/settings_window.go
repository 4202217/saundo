package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/internal/airplay/device"
	"saundo/internal/engine"
)

type settingsPage uint8

const (
	pageGeneral settingsPage = iota
	pageConnection
	pageAudio
	pageCapture
)

var settingsNav = []struct {
	page  settingsPage
	label string
	icon  *ui.SVG
}{
	{pageGeneral, "通用", iconSliders},
	{pageConnection, "连接", iconLink},
	{pageAudio, "音频", iconWave},
	{pageCapture, "采集", iconScreen},
}

// showSettings opens the configuration window. The flyout keeps the playback
// controls, so it hides while the user works in this window.
func (a *App) showSettings() {
	if a.win != nil {
		a.win.Hide()
	}
	if a.settingsWin == nil {
		return
	}
	a.settingsWin.Show()
	a.settingsWin.Focus()
}

// newSettingsWindow builds the configuration window hidden; showSettings
// reveals it. Closing it hides it, and the app keeps running in the tray.
func (a *App) newSettingsWindow() *mygo.Window {
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     "设置 · 声渡",
		Width:     720,
		Height:    500,
		MinWidth:  640,
		MinHeight: 420,
		Hidden:    true,
		// The view draws the title bar so it blends with the window's
		// background; the system keeps its buttons over it.
		TitleBarStyle: mygo.TitleBarHidden,
		Content:       ui.View(a.settingsView),
	})
	w.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		e.PreventDefault()
		w.Hide()
	})
	w.OnShow(func() { a.settingsEntered = false })
	return w
}

func (a *App) settingsView(c *ui.Context) {
	p := design(c)
	ui.Column(c).Fill().Background(p.bg).Children(func() {
		a.settingsTitleBar(c, p)
		ui.Row(c).Grow(1).Children(func() {
			a.settingsSidebar(c, p)
			a.settingsBody(c, p)
		})
	})
}

// settingsTitleBar draws the window's title bar under the system's window
// buttons, which take the band TitleBar reports. It is the window's handle.
func (a *App) settingsTitleBar(c *ui.Context, p palette) {
	bar := c.TitleBar()
	title := ui.Row(c).Height(max(bar.Height, 32)).
		Padding(0, bar.Right, 0, 12+bar.Left).
		Gap(8).Background(p.panel).BorderWidth(0, 0, 1, 0).BorderColor(p.line2).
		DragWindow()
	title.Children(func() {
		ui.Icon(c, iconMark).Size(14, 14).TextColor(p.fg)
		ui.Text(c, "设置 · 声渡").FontSize(11.5).TextColor(p.fg2).SingleLine()
	})
}

func (a *App) settingsSidebar(c *ui.Context, p palette) {
	name, sub, dot := a.sidebarIdentity(p)
	ui.Column(c).Width(190).FillHeight().Background(p.panel).Padding(12, 10, 10, 10).
		BorderWidth(0, 1, 0, 0).BorderColor(p.line2).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			for _, item := range settingsNav {
				a.sidebarItem(c, p, item.page, item.icon, item.label)
			}
		})
		ui.Spacer(c)
		ui.Box(c).Height(1).Background(p.line2)
		ui.Row(c).Gap(8).Margin(10, 6, 0, 6).Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Shrink(0).Background(dot)
			ui.Column(c).Gap(1).Grow(1).Children(func() {
				ui.Text(c, name).FontSize(11.5).SingleLine().Ellipsis("…")
				ui.Text(c, sub).FontSize(10.5).TextColor(p.muted).SingleLine().Ellipsis("…")
			})
		})
	})
}

func (a *App) sidebarItem(c *ui.Context, p palette, page settingsPage, icon *ui.SVG, label string) {
	on := a.page == page
	e := ui.Row(c).Height(32).Padding(0, 9).Gap(8).Radius(7).Cursor(ui.CursorPointer).
		Key(page).Transition(ui.ElementTransition{Colors: true, Duration: 140 * time.Millisecond})
	switch {
	case on && p.dark:
		e.Background(ui.RGBA(255, 255, 255, 0.15))
	case on:
		e.Background(p.card)
		e.Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.14))
	case e.Hovered():
		e.Background(p.pill)
	}
	e.OnClick(func() { a.page = page })
	e.Children(func() {
		ic := ui.Icon(c, icon).Size(15, 15)
		tx := ui.Text(c, label).FontSize(12.5).SingleLine()
		if on {
			ic.TextColor(p.fg)
			tx.TextColor(p.fg).FontWeight(500)
		} else {
			ic.TextColor(p.muted)
			tx.TextColor(p.ctl)
		}
	})
}

func (a *App) settingsBody(c *ui.Context, p palette) {
	title, desc := pageCopy(a.page)
	body := ui.Column(c).Grow(1).FillHeight().Padding(14, 20, 10, 20)
	enterPose(c, body, &a.settingsEntered)
	body.Children(func() {
		ui.Text(c, title).FontSize(18).FontWeight(650)
		ui.Text(c, desc).FontSize(13).TextColor(p.muted).Margin(4, 0, 0, 0)
		content := ui.Column(c).Key(a.page).Grow(1).Transition(ui.ElementTransition{
			Duration: 160 * time.Millisecond,
			Enter:    &ui.Motion{X: -6, Opacity: 0},
			Exit:     &ui.Motion{X: 6, Opacity: 0},
		})
		content.Children(func() {
			switch a.page {
			case pageGeneral:
				a.generalPage(c, p)
			case pageConnection:
				a.connectionPage(c, p)
			case pageAudio:
				a.audioPage(c, p)
			case pageCapture:
				a.capturePage(c, p)
			}
		})
	})
}

func pageCopy(page settingsPage) (title, desc string) {
	switch page {
	case pageConnection:
		return "连接", "配对、重连与验证策略"
	case pageAudio:
		return "音频", "播放延迟与传输格式。"
	case pageCapture:
		return "采集", "系统声音或单个应用。"
	}
	return "通用", "发送端身份、启动与版本。"
}

func pageCard(c *ui.Context, p palette, body func()) {
	card := ui.Column(c).Margin(12, 0, 0, 0).Radius(12).Background(p.card).Border(1, p.line).Clip()
	card.Children(body)
}

func (a *App) generalPage(c *ui.Context, p palette) {
	pageCard(c, p, func() {
		settingRow(c, p, "发送端名称", "接收设备上显示的名字", func() {
			ui.Row(c).Gap(6).Children(func() {
				if fieldInput(c, p, &a.nameDraft, "声渡", "发送端名称").Submitted() {
					a.eng.SetName(a.nameDraft)
				}
				chipButton(c, p, "保存", func() { a.eng.SetName(a.nameDraft) })
			})
		})
		rowLine(c, p)
		settingRow(c, p, "开机自动启动", "登录 Windows 后静默驻留托盘", func() {
			toggle(c, p, &a.launchAtStart, func(v bool) { a.setLaunchAtStart(v) })
		})
		rowLine(c, p)
		settingRow(c, p, "关于", "声渡 · saundo", func() {
			staticText(c, p, Version)
		})
	})
}

func (a *App) connectionPage(c *ui.Context, p palette) {
	if d, ok := a.currentDevice(); ok {
		a.deviceSummary(c, p, d)
	}
	pageCard(c, p, func() {
		settingRow(c, p, "启动时自动连接上次设备", "上次成功播放的设备", func() {
			toggle(c, p, &a.autoConn, func(v bool) { a.eng.SetAutoConnect(v) })
		})
		rowLine(c, p)
		settingRow(c, p, "断开后自动重连", "1s / 2s / 5s / 10s / 15s 退避，最多 5 次", func() {
			toggle(c, p, &a.reconnect, func(v bool) { a.eng.SetReconnect(v) })
		})
		rowLine(c, p)
		settingRow(c, p, "严格验证接收器", "配对证明或签名不通过时立即断开", func() {
			toggle(c, p, &a.strict, func(v bool) { a.eng.SetStrictAuth(v) })
		})
		rowLine(c, p)
		settingRow(c, p, "接收端密码", a.passwordTargetText(), func() {
			ui.Row(c).Gap(6).Children(func() {
				field := fieldInput(c, p, &a.passwordDraft, "为所选设备保存", "接收端密码").Password()
				if field.Submitted() {
					a.saveReceiverPassword()
				}
				chipButton(c, p, "保存", func() { a.saveReceiverPassword() })
			})
		})
	})
}

func (a *App) audioPage(c *ui.Context, p palette) {
	pageCard(c, p, func() {
		ui.Column(c).Padding(9, 14, 10, 14).Children(func() {
			ui.Row(c).Children(func() {
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, "播放延迟").FontSize(13).FontWeight(500)
					ui.Text(c, "重连后生效，数值越大音画同步余量越大").FontSize(11.5).
						TextColor(p.muted).SingleLine().Ellipsis("…")
				})
				ui.Textf(c, "%.0f ms", a.latency).FontSize(12).TextColor(p.muted)
			})
			slider(c, p, &a.latency, 20, 2000, func(v float64) {
				a.eng.SetLatency(int(v))
			}).Margin(4, 0, 0, 0)
		})
		rowLine(c, p)
		settingRow(c, p, "音频格式", "固定格式，兼容所有 AirPlay 2 接收器", func() {
			staticText(c, p, "ALAC · 44.1 kHz · 立体声")
		})
		rowLine(c, p)
		settingRow(c, p, "同步方式", "按接收器能力选择 PTP 或 NTP", func() {
			mode := "自动"
			if phaseOf(a.snap) == phaseStreaming {
				mode = a.syncText()
			}
			staticText(c, p, mode)
		})
	})
}

func (a *App) capturePage(c *ui.Context, p palette) {
	pageCard(c, p, func() {
		settingRow(c, p, "系统输出", "Windows 默认播放设备", func() {
			if fieldSelect(c, p, &a.captureChoice, a.captureNames, "系统输出").Changed() {
				a.applyCaptureEndpoint()
			}
		})
		rowLine(c, p)
		settingRow(c, p, "按应用采集", "只串流所选程序的声音", func() {
			if fieldSelect(c, p, &a.appChoice, a.appNames, "按应用采集").Changed() {
				a.applyCaptureProcess()
			}
		})
		rowLine(c, p)
		settingRow(c, p, "采集状态", "当前采集源", func() {
			ui.Row(c).Gap(6).Shrink(0).Children(func() {
				dot := p.faint
				switch {
				case a.snap.CaptureError != "":
					dot = p.red
				case a.snap.CaptureRunning:
					dot = p.green
				}
				ui.Box(c).Size(6, 6).Radius(3).Shrink(0).Background(dot)
				ui.Text(c, captureStatusText(a.snap)).FontSize(12).TextColor(p.muted).SingleLine()
			})
		})
	})
}

func (a *App) deviceSummary(c *ui.Context, p palette, d device.Device) {
	ph := phaseOf(a.snap)
	e := ui.Row(c).Height(54).Margin(12, 0, 0, 0).Padding(0, 14).Gap(10).
		Radius(12).Background(p.card).Border(1, p.line)
	e.Children(func() {
		deviceGlyph(c, p, d, false)
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, d.Name).FontSize(13).FontWeight(500).SingleLine().Ellipsis("…")
			ui.Text(c, summarySub(d)).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
		})
		switch ph {
		case phaseStreaming:
			chip(c, p, "已连接", chipOK)
		case phaseConnecting:
			chip(c, p, "连接中", chipAccent)
		case phaseError:
			chip(c, p, "连接失败", chipNeutral)
		default:
			chip(c, p, "未连接", chipNeutral)
		}
	})
}

func summarySub(d device.Device) string {
	if d.IsStereoGroup() {
		return fmt.Sprintf("立体声组 · %d/2 · 当前设备", len(d.Members))
	}
	if d.Model != "" {
		return d.Model + " · 当前设备"
	}
	return "当前设备"
}

func (a *App) sidebarIdentity(p palette) (name, sub string, dot ui.Color) {
	d, ok := a.currentDevice()
	name = "未选择设备"
	if ok {
		name = d.Name
	}
	group := ok && d.IsStereoGroup()
	switch phaseOf(a.snap) {
	case phaseStreaming:
		sub, dot = "已连接", p.green
		if group {
			sub = "已连接 · 立体声组"
		}
	case phaseConnecting:
		sub, dot = a.statusText(), p.accent
	case phaseError:
		sub, dot = "连接失败", p.red
	default:
		sub, dot = "未连接", p.faint
	}
	return name, sub, dot
}

// applyCaptureEndpoint switches capture to the chosen output endpoint.
func (a *App) applyCaptureEndpoint() {
	for i, name := range a.captureNames {
		if name == a.captureChoice && i < len(a.captureIDs) {
			a.eng.SetCaptureEndpoint(a.captureIDs[i], a.captureChoice)
			return
		}
	}
}

// applyCaptureProcess narrows capture to one application.
func (a *App) applyCaptureProcess() {
	for i, name := range a.appNames {
		if name == a.appChoice && i < len(a.appIDs) {
			var pid uint32
			fmt.Sscanf(a.appIDs[i], "%d", &pid)
			a.eng.SetCaptureProcess(pid, name)
			return
		}
	}
}

func (a *App) saveReceiverPassword() {
	if a.selID == "" {
		return
	}
	a.eng.SetReceiverPassword(a.selID, strings.TrimSpace(a.passwordDraft))
}

// passwordTargetText names the device the receiver password is saved to.
func (a *App) passwordTargetText() string {
	for _, d := range a.snap.Devices {
		if d.ID == a.selID {
			return "保存到 " + d.Name
		}
	}
	return "先在控制面板选择设备"
}

func captureStatusText(s engine.Snapshot) string {
	if s.CaptureError != "" {
		return "采集错误"
	}
	if !s.CaptureRunning {
		return "未运行"
	}
	name := s.CaptureName
	if name == "" {
		name = "系统默认输出"
	}
	if s.CaptureRate > 0 {
		return name + " · " + formatRate(s.CaptureRate)
	}
	return name
}
