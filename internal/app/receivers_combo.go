package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/receivers"
)

// The stereo combo's surfaces on the receivers page: the created combo's card
// row, its channel layout map, and the dashed create entry. The create and
// unlink dialogs live in receivers_dialogs.go.

// receiverPairAddRow is the dotted create-combo affordance: the empty
// section's rows are absent, so this row stands in for them.
func (a *App) receiverPairAddRow(c *ui.Context, p palette) {
	row := ui.ButtonBase(c).Margin(8, 0, 0, 0).MinHeight(44).Padding(9, 14).Gap(10).Shrink(0).
		Radius(12).Cursor(ui.CursorPointer).Justify(ui.Start).AlignItems(ui.Center).
		Label(locales.ReceiverPairAddTitle()).Border(1, p.line.Mix(p.muted, 0.2)).
		BorderStyle(ui.BorderDashed)
	row.Background(ui.Transparent)
	hoverFill(row, p.pill, 11, 1)
	row.OnClick(func() { a.openComboCreate() })
	row.Children(func() {
		tile := ui.Box(c).Size(24, 24).Radius(12).Center().Shrink(0).Background(p.accentSoft)
		tile.Draw(func(pt *ui.Painter, r ui.Rect) {
			if row.Hovered() {
				pt.Fill(r, p.accentSoft.Mix(p.accent, 0.2), 12)
			}
		})
		tile.Children(func() {
			ui.Icon(c, iconLink).Size(12, 12).TextColor(p.accent)
		})
		ui.Column(c).Gap(1).Grow(1).Children(func() {
			ui.Text(c, locales.ReceiverPairAddTitle()).FontSize(12.5).FontWeight(500).TextColor(p.accent)
			ui.Text(c, locales.ReceiverPairAddSub()).FontSize(10.5).TextColor(p.muted).SingleLine().Ellipsis("…")
		})
	})
}

// receiverComboRow is one combo card row: stereo glyph, name with the combo
// chip, the two member names as the sub, and the expanded body with the
// channel map, the auto-connect flag, and one volume. 解除组合 rides the
// head's tail cluster beside the switch while expanded, as the mockup pins
// it — the row is its own card section, so the switch's x may move here.
func (a *App) receiverComboRow(c *ui.Context, p palette, v receivers.View, hiddenGroup, divider bool) {
	open := a.receiverOpen[v.Key]
	row := ui.Column(c).Key(v.Key).Shrink(0).
		Transition(ui.ElementTransition{Colors: true, Duration: 140 * time.Millisecond})
	if divider {
		row.BorderWidth(0, 0, 1, 0).BorderColor(p.line2)
	}
	row.Children(func() {
		a.receiverComboHead(c, p, v, open, hiddenGroup)
		if open {
			a.receiverComboBody(c, p, v, hiddenGroup)
		}
	})
}

// receiverComboHead is the combo card's clickable head.
func (a *App) receiverComboHead(c *ui.Context, p palette, v receivers.View, open, hiddenGroup bool) {
	head := ui.ButtonBase(c.Key("head:"+v.Key)).MinHeight(56).Padding(9, 14).Gap(12).
		AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).
		Label(v.Name).FillWidth()
	switch {
	case open && !hiddenGroup:
		head.Background(p.sel)
	case open && hiddenGroup:
		head.Background(p.pill)
	default:
		hoverFill(head, p.cardHover, 0, 0)
	}
	head.OnClick(func() {
		if a.receiverOpen == nil {
			a.receiverOpen = map[string]bool{}
		}
		a.receiverOpen[v.Key] = !a.receiverOpen[v.Key]
	})
	head.Children(func() {
		a.receiverGlyph(c, p, v, open, hiddenGroup)
		a.receiverNameCol(c, p, v, false)
		ui.Row(c).Gap(8).Shrink(0).AlignItems(ui.Center).Children(func() {
			if open {
				color := p.red
				if hiddenGroup {
					color = p.muted
				}
				rowAction(c, p, "unlink:"+v.Key, locales.ComboUnlink(), iconUnlink, color, func() {
					a.receiverDialog = receiverDialogState{kind: receiverDialogComboUnlink, key: v.Key, entered: false}
					a.receiverDialogOpen = true
				})
			}
			shown := !v.Hidden
			sw := toggle(c, p, &shown, locales.ReceiverEnableSwitch(), func(next bool) {
				a.eng.SetReceiverHidden(v.Key, !next)
				if !next {
					a.receiverHiddenOpen = true
				}
			})
			if v.Invalid {
				sw.Disabled(true).Opacity(0.4)
			}
			chev := ui.Icon(c, iconChevron).Size(12, 12).TextColor(p.muted)
			if open {
				chev.TextColor(p.fg2)
			} else {
				chev.Rotate(-90)
			}
		})
	})
}

// receiverComboBody is the combo's expanded config: auto-connect, the
// channel layout map, an offline warning, and one volume for both members.
func (a *App) receiverComboBody(c *ui.Context, p palette, v receivers.View, hiddenGroup bool) {
	titleColor, subColor := p.fg, p.muted
	if hiddenGroup {
		titleColor, subColor = p.muted, p.faint
	}
	body := ui.Column(c).Padding(0, 14, 4, 14).BorderWidth(1, 0, 0, 0).BorderColor(p.line2).Shrink(0)
	body.Children(func() {
		if v.Invalid {
			// An unavailable combo has nothing left to configure: the head
			// names the reason, the layout says which devices it held, and
			// 解除组合 sits in the head's tail. Auto-connect, volume, and the
			// offline warning would all describe playback this combo cannot do.
			ui.Column(c).Padding(8, 14).Gap(6).Children(func() {
				ui.Text(c, locales.ComboMapLabel()).FontSize(13).FontWeight(500).TextColor(titleColor)
				a.comboChannelMap(c, p, v, hiddenGroup || v.Invalid)
			})
			return
		}
		auto := v.AutoConnect
		settingRowColors(c, p, locales.ReceiverAutoToggle(), locales.ReceiverAutoToggleDesc(),
			titleColor, subColor, func() {
				toggle(c, p, &auto, locales.ReceiverAutoToggle(), func(next bool) {
					a.eng.SetReceiverAutoConnect(v.Key, next)
				})
			})
		rowLine(c, p)
		// The map is a block row, not a settings row with a trailing control:
		// two members plus a swap button need the full width, and the title
		// keeps the page's row rhythm on its own line above them. The row
		// carries the same 14 inset as every setting row, so the title sits
		// on one x with 启动时自动连接 and 音量.
		ui.Column(c).Padding(8, 14).Gap(6).Children(func() {
			ui.Text(c, locales.ComboMapLabel()).FontSize(13).FontWeight(500).TextColor(titleColor)
			a.comboChannelMap(c, p, v, hiddenGroup)
		})
		if !v.Device.Online {
			// The bar rides the setting rows' inset, not the body's, so its
			// edges line up with the half-cards above and the volume row
			// below; the mockup keeps an 8px gap under the map.
			ui.Column(c).Padding(8, 14, 0, 14).Children(func() {
				statusBar(c, p, barWarning, iconAlert, locales.ComboOfflineWarn(comboOfflineMember(v.Device)), nil)
			})
		}
		rowLine(c, p)
		a.receiverVolumeRow(c, p, v, titleColor, subColor, hiddenGroup)
	})
}

// comboChannelMap is the combo's stereo layout: two soft half-cards with the
// side over the member, and a quiet swap button between them that flips the
// sides in place. The halves carry no border — the fill alone groups each
// side, and only an offline member's half turns dashed to read as a gap.
func (a *App) comboChannelMap(c *ui.Context, p palette, v receivers.View, hiddenGroup bool) {
	member := func(i int) device.Device {
		if i < len(v.Device.Members) {
			return v.Device.Members[i]
		}
		return device.Device{}
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		a.comboMapHalf(c, p, locales.ChannelLeft(), member(0), hiddenGroup)
		swap := ui.ButtonBase(c.Key("swap:"+v.Key)).Size(26, 26).Radius(13).Center().Shrink(0).
			Label(locales.ComboSwapTip()).Tooltip(locales.ComboSwapTip())
		if hiddenGroup {
			swap.Disabled(true).Opacity(0.4)
		} else {
			swap.Cursor(ui.CursorPointer)
			swap.OnClick(func() { a.eng.SwapComboSides(v.Key) })
			hoverFill(swap, p.pill, 13, 0)
		}
		swap.Children(func() {
			ui.Icon(c, iconSwap).Size(13, 13).TextColor(p.muted)
		})
		a.comboMapHalf(c, p, locales.ChannelRight(), member(1), hiddenGroup)
	})
}

// comboMapHalf is one side of the layout: a soft pill carrying the side
// label over the member's dot and name, both centered. An offline member's
// half turns dashed and its row dims.
func (a *App) comboMapHalf(c *ui.Context, p palette, side string, m device.Device, hiddenGroup bool) {
	name := m.Name
	if name == "" {
		name = locales.DeviceDefaultModel()
	}
	half := ui.Column(c).Grow(1).Padding(8, 10).Radius(10).Background(p.pill).Shrink(0)
	if !m.Online {
		half.Background(ui.Transparent).Border(1, p.line2).BorderStyle(ui.BorderDashed)
	}
	half.Children(func() {
		ui.Row(c).Justify(ui.Center).Children(func() {
			ui.Text(c, side).FontSize(10.5).FontWeight(600).LetterSpacing(0.4).TextColor(p.muted)
		})
		member := ui.Row(c).Margin(7, 0, 0, 0).Gap(8).AlignItems(ui.Center).Justify(ui.Center)
		if !m.Online {
			member.Opacity(0.45)
		}
		member.Children(func() {
			g := ui.Box(c).Size(20, 20).Radius(10).Center().Shrink(0).Background(p.card)
			g.Children(func() {
				ui.Icon(c, deviceIcon(m.Model, false)).Size(11, 11).TextColor(p.fg2)
				if m.Online && !hiddenGroup {
					ui.Box(c).Size(6, 6).Radius(3).Background(p.green).
						Shadow(0, 0, 0, 2, p.pill).
						Attach(ui.AnchorBottomRight, ui.AnchorBottomRight).Right(-1).Bottom(-1)
				}
			})
			ui.Text(c, name).FontSize(12.5).TextColor(p.fg2).SingleLine().Ellipsis("…")
		})
	})
}

// comboOfflineMember names the offline members of a combo for the warning
// bar under the channel map: one name, or the two joined the way the combo
// names itself.
func comboOfflineMember(d device.Device) string {
	names := make([]string, 0, 2)
	for _, m := range d.Members {
		if !m.Online && m.Name != "" {
			names = append(names, m.Name)
		}
	}
	if len(names) == 0 {
		return d.Name
	}
	return strings.Join(names, " + ")
}
