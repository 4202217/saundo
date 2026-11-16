package app

import (
	"math"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/internal/settings"
)

// palette holds the design's tokens that the stock theme has no field for.
// It follows the system's appearance, as the theme does.
type palette struct {
	dark bool
	// scale is the desktop's text scale, carried by the window's content
	// zoom so text, icons and spacing grow together without clipping the
	// design's fixed rows.
	scale float32

	bg, panel, card, cardHover   ui.Color
	pill, pillHover              ui.Color
	line, line2                  ui.Color
	fg, fg2, ctl, muted, faint   ui.Color
	accent, accentSoft, accentFg ui.Color
	sel, green, greenSoft        ui.Color
	red, redSoft                 ui.Color
	warning, warningSoft         ui.Color
}

func paletteFor(dark bool) palette {
	if dark {
		return palette{
			dark:        true,
			bg:          ui.Hex("#1a1a1e"),
			panel:       ui.Hex("#1b1c20"),
			card:        ui.Hex("#232327"),
			cardHover:   ui.Hex("#26262b"),
			pill:        ui.Hex("#2d2d33"),
			pillHover:   ui.Hex("#35353c"),
			line:        ui.Hex("#333339"),
			line2:       ui.Hex("#2c2c32"),
			fg:          ui.Hex("#ededf1"),
			fg2:         ui.Hex("#b9b9c1"),
			ctl:         ui.Hex("#a1a1aa"),
			muted:       ui.Hex("#8b8b94"),
			faint:       ui.Hex("#56565f"),
			accent:      ui.Hex("#a5b4fc"),
			accentSoft:  ui.Hex("#2b2c47"),
			accentFg:    ui.Hex("#14142a"),
			sel:         ui.Hex("#33345a"),
			green:       ui.Hex("#6fd99b"),
			greenSoft:   ui.Hex("#1e3a2b"),
			red:         ui.Hex("#f28b82"),
			redSoft:     ui.Hex("#45272a"),
			warning:     ui.Hex("#e5a95c"),
			warningSoft: ui.Hex("#382b1b"),
		}
	}
	return palette{
		bg:          ui.Hex("#f4f4f6"),
		panel:       ui.Hex("#f2f2f5"),
		card:        ui.Hex("#ffffff"),
		cardHover:   ui.Hex("#f9f9fb"),
		pill:        ui.Hex("#f1f1f4"),
		pillHover:   ui.Hex("#e9e9ed"),
		line:        ui.Hex("#e3e3e8"),
		line2:       ui.Hex("#ececf0"),
		fg:          ui.Hex("#1c1c21"),
		fg2:         ui.Hex("#4b4b53"),
		ctl:         ui.Hex("#63636b"),
		muted:       ui.Hex("#85858d"),
		faint:       ui.Hex("#b9b9c2"),
		accent:      ui.Hex("#4f46e5"),
		accentSoft:  ui.Hex("#eeedfd"),
		accentFg:    ui.Hex("#ffffff"),
		sel:         ui.Hex("#ecebfb"),
		green:       ui.Hex("#189a58"),
		greenSoft:   ui.Hex("#e4f6ec"),
		red:         ui.Hex("#d64545"),
		redSoft:     ui.Hex("#fbe9e9"),
		warning:     ui.Hex("#b76a12"),
		warningSoft: ui.Hex("#fbf0e0"),
	}
}

// normalizeScale clamps the desktop's text scale for the window's content
// zoom. The platform reports 0 where it has no setting.
func normalizeScale(s float64) float32 {
	if s > 0 {
		return float32(min(max(s, 0.5), 3))
	}
	return 1
}

func scaleFor(prefs ui.Preferences) float32 {
	return normalizeScale(float64(prefs.TextScale))
}

// withPreferences carries the desktop's contrast setting into the design's
// own colors, as the theme does for the stock widgets.
func (p palette) withPreferences(prefs ui.Preferences) palette {
	if prefs.HighContrast {
		p.line = p.line.Mix(p.fg, 0.45)
		p.line2 = p.line2.Mix(p.fg, 0.35)
		p.muted = p.muted.Mix(p.fg, 0.4)
		p.faint = p.faint.Mix(p.fg, 0.4)
	}
	return p
}

// theme turns the platform theme into the design's. The window's content
// zoom carries the desktop's text scale, so the theme's own copy stays
// neutral and text is not scaled twice. The palette already carries the
// contrast setting, so the theme does not follow it again.
func (p palette) theme(base *ui.Theme, prefs ui.Preferences) *ui.Theme {
	t := *base
	// Undo the previous frame's follow first, then lay the design's values
	// on top. Following the contrast again here would mix colors that
	// withPreferences already mixed.
	followed := prefs
	followed.TextScale = 1
	followed.HighContrast = false
	t = *t.WithPreferences(followed)
	t.Background = p.bg
	t.Surface = p.pill
	t.SurfaceHover = p.pillHover
	t.SurfacePressed = p.line
	t.Border = p.line
	t.Text = p.fg
	t.TextMuted = p.muted
	t.Accent = p.accent
	t.AccentHover = p.accent.Mix(p.fg, 0.16)
	t.AccentPressed = p.accent.Mix(p.fg, 0.28)
	t.AccentText = p.accentFg
	t.Danger = p.red
	t.Warning = p.warning
	t.Success = ui.Hex("#189a58")
	if p.dark {
		t.Success = p.green
	}
	t.Selection = p.sel
	t.Focus = p.accent
	t.Radius = 7
	t.FontSize = 13
	if prefs.HighContrast {
		t.Focus.A = 255
	}
	return &t
}

// appearanceSource maps the saved appearance choice to the framework's.
func appearanceSource(mode settings.ThemeMode) mygo.ThemeSource {
	switch mode {
	case settings.ThemeLight:
		return mygo.ThemeLight
	case settings.ThemeDark:
		return mygo.ThemeDark
	}
	return mygo.ThemeSystem
}

// design applies the saundo theme and the desktop's text scale to this frame
// and returns the palette.
func design(c *ui.Context) palette {
	prefs := c.Preferences()
	p := paletteFor(c.Theme().Dark).withPreferences(prefs)
	p.scale = scaleFor(prefs)
	c.SetZoom(p.scale)
	c.SetTheme(p.theme(c.Theme(), prefs))
	return p
}

// scaled rounds a design size to the window DIPs the content zoom needs: the
// view keeps laying out at the design's size.
func scaled(px int, scale float32) int {
	return int(math.Round(float64(px) * float64(scale)))
}

func scaledSize(w, h int, scale float32) (int, int) {
	return scaled(w, scale), scaled(h, scale)
}
