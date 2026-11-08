package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/lamp"
	"saundo/internal/settings"
)

const (
	// lampSectionHeight is the collapsed flyout section: its head with the
	// 26-DIP power action, the 44-DIP row, the card border and margin, the
	// body gap above it, and 4 DIP of body slack that keeps a one-row flyout
	// from compressing its cards.
	lampSectionHeight = 90
	lampRowHeight     = 44
	lampColumns       = 10
	lampSwatchSize    = 30
	lampSwatchGap     = 7.5
	lampSwatchGapY    = 8
	lampGridPadding   = 22
	lampRowsVisible   = 4
)

// lampPresetName is the localized name of a preset ID.
func lampPresetName(id string) string {
	switch lamp.Normalize(id) {
	case "ember":
		return locales.LampPresetEmber()
	case "dune":
		return locales.LampPresetDune()
	case "moss":
		return locales.LampPresetMoss()
	case "tide":
		return locales.LampPresetTide()
	case "orchid":
		return locales.LampPresetOrchid()
	case "moon":
		return locales.LampPresetMoon()
	default:
		return locales.LampPresetDune()
	}
}

// lampModeName is the name of what the top display is following: the media
// cover in cover priority, otherwise the selected template.
func (a *App) lampModeName() string {
	if a.lampCover {
		return locales.LampPresetCover()
	}
	return lampPresetName(a.lampPreset)
}

// lampPreview paints what the top display follows: the cover's extracted
// three-color glow, a neutral media-glyph tile when there is no cover yet,
// or the selected template.
func (a *App) lampPreview(c *ui.Context, p palette, radius float32) ui.Element {
	if a.lampCover && !a.nowLampOK {
		box := ui.Box(c).Radius(radius).Clip().Background(p.pill)
		box.Center().Children(func() {
			ui.Icon(c, iconImage).Size(radius*1.7, radius*1.7).TextColor(p.faint)
		})
		return box
	}
	box := ui.Box(c).Radius(radius).Clip()
	box.Draw(func(pt *ui.Painter, r ui.Rect) {
		if a.lampCover {
			drawLamp(pt, r, a.nowLamp, radius)
			return
		}
		preset, _ := lamp.ByID(a.lampPreset)
		drawLamp(pt, r, preset, radius)
	})
	return box
}

// lampTilePreview paints one template tile.
func lampTilePreview(c *ui.Context, preset string, radius float32) ui.Element {
	p, _ := lamp.ByID(lamp.Normalize(preset))
	return ui.Box(c).Radius(radius).Clip().Draw(func(pt *ui.Painter, r ui.Rect) {
		drawLamp(pt, r, p, radius)
	})
}

// lampGridRows is how many preset rows the expanded card shows before it
// scrolls: at most four.
func lampGridRows(count int) int {
	rows := (count + lampColumns - 1) / lampColumns
	return min(max(rows, 1), lampRowsVisible)
}

// lampGridScrollHeight is the scrolling part of the expanded card.
func lampGridScrollHeight(count int) int {
	rows := lampGridRows(count)
	return rows*lampSwatchSize + (rows-1)*lampSwatchGapY + lampGridPadding
}

// lampGridHeight adds the divider above the grid to its scrolling part.
func lampGridHeight(count int) int { return lampGridScrollHeight(count) + 1 }

// setLampOff applies the manual light-off action on either surface. The
// ambient feature stays; off turns the top display dark through the playback
// state.
func (a *App) setLampOff(off bool) {
	a.lampOff = off
	a.snap.LampOff = off
	a.lampExpanded = false
	a.cfg.Lamp = settings.LampSettings{Off: off, Cover: a.lampCover, Preset: a.lampPreset}
	if a.eng != nil {
		a.eng.SetLamp(off, a.lampCover, a.lampPreset)
	}
	a.fitFlyout()
}

// setLampCover chooses the media cover or the selected template. Either
// choice collapses the template list; the chevron opens it in template
// priority.
func (a *App) setLampCover(on bool) {
	if a.lampOff {
		return
	}
	a.lampCover = on
	a.snap.LampCover = on
	a.lampExpanded = false
	a.lampOff = false
	a.snap.LampOff = false
	a.cfg.Lamp = settings.LampSettings{Off: false, Cover: on, Preset: a.lampPreset}
	if a.eng != nil {
		a.eng.SetLamp(false, on, a.lampPreset)
	}
	a.fitFlyout()
}

// chooseLampPreset selects a template, turns cover priority off, and turns
// the light back on if it was off; picking a light is deliberate. The
// flyout list stays open after the pick; closing the flyout collapses it.
func (a *App) chooseLampPreset(id string) {
	if a.lampOff {
		return
	}
	a.lampPreset = lamp.Normalize(id)
	a.snap.LampPreset = a.lampPreset
	a.lampCover = false
	a.snap.LampCover = false
	a.lampOff = false
	a.snap.LampOff = false
	a.cfg.Lamp = settings.LampSettings{Off: false, Cover: false, Preset: a.lampPreset}
	if a.eng != nil {
		a.eng.SetLamp(false, false, a.lampPreset)
	}
	a.fitFlyout()
}

func (a *App) toggleLampExpanded() {
	a.lampExpanded = !a.lampExpanded
	a.fitFlyout()
}

// collapseLampList folds the template list when the flyout hides.
func (a *App) collapseLampList() {
	a.lampExpanded = false
	a.fitFlyout()
}

// lampPowerButton is the momentary 关灯/开灯 action. The color and the card
// dim carry the state; there is no toggle.
func (a *App) lampPowerButton(c *ui.Context, p palette) {
	label := locales.LampTurnOff()
	if a.lampOff {
		label = locales.LampTurnOn()
	}
	btn := ui.ButtonBase(c).Size(26, 26).Radius(7).Center().Shrink(0).
		Cursor(ui.CursorPointer).Label(label)
	btn.OnClick(func() { a.setLampOff(!a.lampOff) })
	if btn.Hovered() {
		btn.Background(p.pill)
	}
	color := p.fg
	if a.lampOff {
		color = p.accent
	}
	btn.Children(func() {
		ui.Icon(c, iconPower).Size(14, 14).TextColor(color)
	})
}

// lampSection is the permanent ambient-light card under the playback card.
// The row carries the priority segment; the template list appears only in
// template priority and expanded.
func (a *App) lampSection(c *ui.Context, p palette) {
	ui.Column(c).Children(func() {
		sectionHead(c, p, locales.FlyoutLampSection(), "", false, func() { a.lampPowerButton(c, p) })
		card := ui.Column(c).Margin(2, 0, 0, 0).Radius(10).Background(p.card).Border(1, p.line).Clip()
		card.Children(func() {
			ui.Row(c).Height(lampRowHeight).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
				if a.lampCover || a.lampOff {
					ui.Row(c).Grow(1).Gap(8).AlignItems(ui.Center).Children(func() {
						preview := a.lampPreview(c, p, 9).Size(32, 32).Shrink(0)
						if a.lampOff {
							preview.Opacity(0.35)
						}
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							name := ui.Text(c, a.lampModeName()).FontSize(13).FontWeight(500).SingleLine()
							if a.lampOff {
								name.TextColor(p.faint)
							}
							a.lampStatusChip(c, p)
						})
					})
					a.lampPrioritySegment(c, p, true)
					return
				}
				pick := ui.ButtonBase(c).Grow(1).Gap(8).AlignItems(ui.Center).Justify(ui.Start).
					Cursor(ui.CursorPointer).Label(locales.FlyoutLampSection())
				pick.OnClick(a.toggleLampExpanded)
				pick.Children(func() {
					a.lampPreview(c, p, 9).Size(32, 32).Shrink(0)
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, a.lampModeName()).FontSize(13).FontWeight(500).SingleLine()
						a.lampStatusChip(c, p)
					})
				})

				a.lampPrioritySegment(c, p, true)

				arrow := ui.ButtonBase(c).Size(26, 26).Radius(7).Center().Shrink(0).
					Cursor(ui.CursorPointer).Label(a.lampArrowLabel())
				arrow.OnClick(a.toggleLampExpanded)
				if arrow.Hovered() {
					arrow.Background(p.pill)
				}
				arrow.Children(func() {
					rot := arrow.Animate("lamp-open", boolFlag(a.lampExpanded), 160*time.Millisecond)
					ui.Icon(c, iconChevron).Size(12, 12).TextColor(p.muted).Rotate(180 * rot)
				})
			})
			if !a.lampCover && a.lampExpanded {
				rowLine(c, p)
				a.lampGrid(c, p)
			}
		})
	})
}

// lampStatusChip names the off or no-cover state next to the light's name.
func (a *App) lampStatusChip(c *ui.Context, p palette) {
	label := ""
	switch {
	case a.lampOff:
		label = locales.LampOffStatus()
	case a.lampCover && !a.nowLampOK:
		label = locales.LampNoMedia()
	}
	if label == "" {
		return
	}
	chip(c, p, label, chipNeutral)
}

func (a *App) lampArrowLabel() string {
	if a.lampExpanded {
		return locales.FlyoutLampCollapse()
	}
	return locales.FlyoutLampExpand()
}

func (a *App) lampGrid(c *ui.Context, p palette) {
	grid := ui.Scroll(c).Height(float32(lampGridScrollHeight(len(lamp.All())))).Padding(10, 12, 12, 12)
	grid.Children(func() {
		tracks := make([]ui.Track, lampColumns)
		for i := range tracks {
			tracks[i] = ui.Fixed(lampSwatchSize)
		}
		g := ui.Grid(c).ColumnTracks(tracks...).GapX(lampSwatchGap).GapY(lampSwatchGapY).
			JustifyItems(ui.Start).Label(locales.FlyoutLampColors())
		g.Children(func() {
			for _, preset := range lamp.All() {
				a.lampSwatch(c, p, preset.ID)
			}
		})
	})
}

func (a *App) lampSwatch(c *ui.Context, p palette, id string) {
	selected := id == a.lampPreset
	swatch := ui.ButtonBase(c).Size(lampSwatchSize, lampSwatchSize).Radius(lampSwatchSize / 2).
		Clip().Cursor(ui.CursorPointer).Disabled(a.lampOff).Label(lampPresetName(id))
	swatch.OnClick(func() { a.chooseLampPreset(id) })
	preset, _ := lamp.ByID(lamp.Normalize(id))
	swatch.Draw(func(pt *ui.Painter, r ui.Rect) {
		drawLamp(pt, r, preset, lampSwatchSize/2)
	})
	if selected {
		swatch.DrawOver(func(pt *ui.Painter, r ui.Rect) {
			pt.Stroke(r, p.accent, lampSwatchSize/2, 2)
		})
	}
}

// drawLamp is the shared painter for the preview, the settings swatches, and
// the flyout swatches.
func drawLamp(pt *ui.Painter, r ui.Rect, preset lamp.Preset, radius float32) {
	pt.Fill(r, ui.RGB(preset.Base.R, preset.Base.G, preset.Base.B), radius)
	for _, b := range preset.Blobs() {
		color := ui.RGB(b.Color.R, b.Color.G, b.Color.B)
		alpha := float32(b.Alpha)
		pt.FillRadialGradient(r, ui.RadialGradient{
			Center: ui.Point{X: float32(b.X) / 100, Y: float32(b.Y) / 100},
			Radius: float32(b.Radius) / 100 * r.W,
			Stops: []ui.GradientStop{
				{Color: color.Alpha(0.8 * alpha), At: 0},
				{Color: color.Alpha(0.33 * alpha), At: 0.38},
				{Color: ui.Transparent, At: 1},
			},
		}, radius)
	}
}

// lampPage is the settings surface: the desk-lamp header with the power
// action, the current preview, the priority segment, and the six templates.
func (a *App) lampPage(c *ui.Context, p palette) {
	on := !a.lampOff
	pageCard(c, p, func() {
		ui.Column(c).Padding(0, 14, 12, 14).Children(func() {
			cardHead(c, p, iconLamp, locales.SettingsLampCardTitle(), 13, 0, func() {
				chip(c, p, locales.SettingsLampHomePodOnly(), chipNeutral)
				a.lampPowerButton(c, p)
			})

			ui.Row(c).Gap(14).Margin(12, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				preview := a.lampPreview(c, p, 16).Size(72, 72).Shrink(0)
				if !on {
					preview.Opacity(0.35)
				}
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					name := ui.Text(c, a.lampModeName()).FontSize(13).FontWeight(550)
					if !on {
						name.TextColor(p.faint)
					}
					a.lampStatusChip(c, p)
				})
				ui.Spacer(c)
				a.lampPrioritySegment(c, p, false)
			})

			label := ui.Text(c, locales.SettingsLampTemplates()).FontSize(10.5).FontWeight(600).
				LetterSpacing(0.55).TextColor(p.muted).Margin(12, 0, 0, 0)
			if !on {
				label.TextColor(p.faint)
			}

			strip := ui.Row(c).Gap(8).Margin(8, 0, 0, 0)
			if !on {
				strip.Opacity(0.35)
			}
			strip.Children(func() {
				for _, preset := range lamp.All() {
					a.lampTile(c, p, preset.ID)
				}
			})

			hint := locales.SettingsLampHintOn()
			if !on {
				hint = locales.SettingsLampHintOff()
			}
			ui.Text(c, hint).FontSize(11.5).TextColor(p.muted).Margin(10, 0, 0, 0)
		})
	})
	ui.Column(c).Margin(8, 0, 0, 2).Children(func() {
		infoNote(c, p, locales.SettingsLampSoundGlowHint())
	})
}

// lampPrioritySegment is the two-way cover/template priority choice. The
// settings page uses the full labels; the flyout row uses the short ones.
func (a *App) lampPrioritySegment(c *ui.Context, p palette, compact bool) {
	seg := 0
	if !a.lampCover {
		seg = 1
	}
	width := float32(176)
	coverLabel, templateLabel := locales.LampPriorityCover(), locales.LampPriorityTemplate()
	if compact {
		width = 92
		coverLabel, templateLabel = locales.LampPriorityCoverShort(), locales.LampPriorityTemplateShort()
	}
	items := []segmentItem{{coverLabel, nil}, {templateLabel, nil}}
	if segmented(c, p, &seg, items, width, a.lampOff, locales.LampPriority()).Changed() {
		a.setLampCover(seg == 0)
	}
}

func (a *App) lampTile(c *ui.Context, p palette, id string) {
	selected := id == a.lampPreset
	name := lampPresetName(id)
	tile := ui.ButtonBase(c).Grow(1).Column().Height(64).Padding(5).Radius(9).
		Gap(5).AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).
		Disabled(a.lampOff).Label(name).Border(1, p.line).Background(p.card)
	if selected {
		tile.Background(p.accentSoft).BorderColor(p.accent)
	}
	tile.OnClick(func() { a.chooseLampPreset(id) })
	tile.Children(func() {
		lampTilePreview(c, id, 6).Height(lampSwatchSize).WidthPercent(100)
		ui.Text(c, name).FontSize(10.5).TextColor(p.fg2).SingleLine()
	})
}
