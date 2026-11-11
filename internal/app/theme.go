package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"saundo/internal/settings"
)

// palette holds the design's tokens that the stock theme has no field for.
// It follows the system's appearance, as the theme does.
type palette struct {
	dark bool

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

// theme turns the platform theme into the design's, keeping the
// preferences the platform chose (contrast, text scale, reduced motion).
func (p palette) theme(base *ui.Theme) *ui.Theme {
	t := *base
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

// design applies the saundo theme to this frame and returns its palette.
func design(c *ui.Context) palette {
	p := paletteFor(c.Theme().Dark)
	c.SetTheme(p.theme(c.Theme()))
	return p
}
