package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/lamp"
	"saundo/internal/settings"
)

const (
	// lampSectionHeight is the collapsed flyout section: its head, the 44-DIP
	// row, and the body gap above it.
	lampSectionHeight = 75
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
	case "origin":
		return locales.LampPresetOrigin()
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
		return locales.LampPresetOrigin()
	}
}

// lampPreview paints a palette preset the way the receiver's JPEG does. The
// origin preset is a neutral tile, not the black image the sender puts on the
// wire. glyph adds the faint reset arrow for the preset grid; the large and
// collapsed previews keep the plain neutral tile.
func lampPreview(c *ui.Context, pal palette, preset string, radius float32, glyph bool) ui.Element {
	p, _ := lamp.ByID(lamp.Normalize(preset))
	if p.ID == "origin" {
		box := ui.Box(c).Radius(radius).Clip().Background(pal.pill)
		if glyph {
			box.Center().Children(func() {
				ui.Icon(c, iconReset).Size(radius*2.3, radius*2.3).TextColor(pal.faint)
			})
		}
		return box
	}
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

// setLampEnabled applies the switch on either surface. The switch controls
// whether the flyout shows the ambient card; the renderer falls back to the
// origin preset while it is off.
func (a *App) setLampEnabled(on bool) {
	a.lampEnabled = on
	a.snap.LampEnabled = on
	if !on {
		a.lampExpanded = false
	}
	a.cfg.Lamp = settings.LampSettings{Enabled: on, Preset: a.lampPreset}
	if a.eng != nil {
		a.eng.SetLamp(on, a.lampPreset)
	}
	a.fitFlyout()
}

func (a *App) chooseLampPreset(id string) {
	if !a.lampEnabled {
		return
	}
	a.lampPreset = lamp.Normalize(id)
	a.snap.LampPreset = a.lampPreset
	a.cfg.Lamp = settings.LampSettings{Enabled: true, Preset: a.lampPreset}
	if a.eng != nil {
		a.eng.SetLamp(true, a.lampPreset)
	}
}

func (a *App) toggleLampExpanded() {
	a.lampExpanded = !a.lampExpanded
	a.fitFlyout()
}

// lampHead is the flyout's section head: the label and the receiver-family
// tag, which stays visible whatever device is selected.
func lampHead(c *ui.Context, p palette) {
	ui.Row(c).WidthPercent(100).Padding(6, 2, 2, 2).Children(func() {
		ui.Text(c, locales.FlyoutLampSection()).FontSize(11).FontWeight(600).
			LetterSpacing(0.55).TextColor(p.muted)
		ui.Spacer(c)
		ui.Box(c).Height(18).Radius(9).Padding(0, 7).Center().Background(p.pill).Children(func() {
			ui.Text(c, locales.FlyoutLampHomePodOnly()).FontSize(10).FontWeight(500).
				TextColor(p.muted).SingleLine()
		})
	})
}

// lampSection is the ambient-light card under the playback card. It is drawn
// while the setting is on, with no session and with no capable receiver. The
// settings page switch hides it; the card itself has no switch.
func (a *App) lampSection(c *ui.Context, p palette) {
	if !a.lampEnabled {
		return
	}
	ui.Column(c).Children(func() {
		lampHead(c, p)
		card := ui.Column(c).Margin(2, 0, 0, 0).Radius(10).Background(p.card).Border(1, p.line).Clip()
		card.Children(func() {
			ui.Row(c).Height(lampRowHeight).Padding(0, 12).Gap(8).Children(func() {
				pick := ui.ButtonBase(c).Grow(1).Gap(8).AlignItems(ui.Center).Justify(ui.Start).
					Cursor(ui.CursorPointer).Label(locales.FlyoutLampSection())
				pick.OnClick(a.toggleLampExpanded)
				pick.Children(func() {
					lampPreview(c, p, a.lampPreset, 9, false).Size(32, 32).Shrink(0)
					ui.Text(c, lampPresetName(a.lampPreset)).FontSize(13).FontWeight(500).SingleLine()
				})

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
			if a.lampExpanded {
				rowLine(c, p)
				a.lampGrid(c, p)
			}
		})
	})
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
				a.lampSwatch(c, p, preset)
			}
		})
	})
}

func (a *App) lampSwatch(c *ui.Context, p palette, preset lamp.Preset) {
	selected := preset.ID == a.lampPreset
	swatch := ui.ButtonBase(c).Size(lampSwatchSize, lampSwatchSize).Radius(lampSwatchSize / 2).
		Clip().Cursor(ui.CursorPointer).Label(lampPresetName(preset.ID))
	swatch.OnClick(func() { a.chooseLampPreset(preset.ID) })
	if preset.ID == "origin" {
		swatch.Background(p.pill).Center().Children(func() {
			ui.Icon(c, iconReset).Size(12, 12).TextColor(p.faint)
		})
	} else {
		swatch.Draw(func(pt *ui.Painter, r ui.Rect) {
			drawLamp(pt, r, preset, lampSwatchSize/2)
		})
	}
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

// lampPage is the settings surface: one global switch, the current preview,
// and the six presets.
func (a *App) lampPage(c *ui.Context, p palette) {
	on := a.lampEnabled
	pageCard(c, p, func() {
		ui.Column(c).Padding(13, 14, 12, 14).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, locales.SettingsLampEnable()).FontSize(13).FontWeight(500)
				ui.Spacer(c)
				chip(c, p, locales.SettingsLampHomePodOnly(), chipNeutral)
				toggle(c, p, &a.lampEnabled, locales.LampToggle(), func(v bool) { a.setLampEnabled(v) })
			})

			ui.Row(c).Gap(14).Margin(12, 0, 0, 0).Children(func() {
				preview := lampPreview(c, p, a.lampPreset, 16, false).Size(72, 72).Shrink(0)
				if !on {
					preview.Opacity(0.35)
				}
				ui.Column(c).Gap(2).Children(func() {
					name := ui.Text(c, lampPresetName(a.lampPreset)).FontSize(14).FontWeight(550)
					if !on {
						name.TextColor(p.faint)
					}
					ui.Text(c, locales.SettingsLampTop()).FontSize(10.5).TextColor(p.muted)
				})
			})

			strip := ui.Row(c).Gap(8).Margin(12, 0, 0, 0)
			strip.Disabled(!on)
			if !on {
				strip.Opacity(0.35)
			}
			strip.Children(func() {
				for _, preset := range lamp.All() {
					a.lampTile(c, p, preset)
				}
			})

			hint := locales.SettingsLampHintOn()
			if !on {
				hint = locales.SettingsLampHintOff()
			}
			ui.Text(c, hint).FontSize(10.5).TextColor(p.muted).Margin(10, 0, 0, 0)
			ui.Text(c, locales.SettingsLampSoundGlowHint()).FontSize(10.5).TextColor(p.muted).Margin(4, 0, 0, 0)
		})
	})
}

func (a *App) lampTile(c *ui.Context, p palette, preset lamp.Preset) {
	selected := preset.ID == a.lampPreset
	name := lampPresetName(preset.ID)
	tile := ui.ButtonBase(c).Grow(1).Column().Height(64).Padding(5).Radius(9).
		Gap(5).AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).Label(name).
		Border(1, p.line).Background(p.card)
	if selected {
		tile.Background(p.accentSoft).BorderColor(p.accent)
	}
	tile.OnClick(func() { a.chooseLampPreset(preset.ID) })
	tile.Children(func() {
		lampPreview(c, p, preset.ID, 6, true).Height(lampSwatchSize).WidthPercent(100)
		ui.Text(c, name).FontSize(10.5).TextColor(p.fg2).SingleLine()
	})
}
