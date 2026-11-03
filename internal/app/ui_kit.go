package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"saundo/internal/airplay/sender"
	"saundo/internal/engine"
)

// phase is the one state the whole interface speaks in, derived from the
// engine's finer-grained sender states.
type phase uint8

const (
	phaseIdle phase = iota
	phaseConnecting
	phaseStreaming
	phaseError
)

func phaseOf(s engine.Snapshot) phase {
	switch {
	case s.State == sender.StateStreaming:
		return phaseStreaming
	case s.State != sender.StateIdle || s.Reconnecting:
		return phaseConnecting
	case s.Error != "":
		return phaseError
	}
	return phaseIdle
}

func phaseColor(p phase, pal palette) ui.Color {
	switch p {
	case phaseStreaming:
		return pal.green
	case phaseConnecting:
		return pal.accent
	case phaseError:
		return pal.red
	}
	return pal.muted
}

// face is the tray's state face; at rest, or connected with no audible audio,
// it is the plain smile.
func (a *App) face() string { return faceOf(a.snap) }

// faceOf is the state face for a snapshot. At rest, or connected with no
// audible audio, it is the plain smile.
func faceOf(s engine.Snapshot) string {
	switch phaseOf(s) {
	case phaseStreaming:
		if s.AudioIdle {
			return "smile"
		}
		return "stream"
	case phaseConnecting:
		return "connect"
	case phaseError:
		return "error"
	}
	return "smile"
}

// sectionHead is the small label above a card.
func sectionHead(c *ui.Context, p palette, label, note string) {
	ui.Row(c).Padding(6, 2, 2, 2).Children(func() {
		ui.Text(c, label).FontSize(11).FontWeight(600).LetterSpacing(0.55).TextColor(p.muted)
		ui.Spacer(c)
		if note != "" {
			ui.Text(c, note).FontSize(11.5).TextColor(p.muted).SingleLine()
		}
	})
}

// iconButton is a borderless 26px square holding one icon.
func iconButton(c *ui.Context, p palette, icon *ui.SVG, label string, size float32, fn func()) {
	b := ui.ButtonBase(c).Size(26, 26).Radius(7).Center().Cursor(ui.CursorPointer).Label(label)
	b.OnClick(fn)
	if b.Hovered() {
		b.Background(p.pill)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(size, size).TextColor(p.muted)
	})
}

// textButton is a row action: an icon and a word, colored by its meaning. An
// action without a handler is a status, not a button, so it takes no focus.
func textButton(c *ui.Context, p palette, label string, icon *ui.SVG, color ui.Color, fn func()) {
	content := func() {
		if icon != nil {
			ui.Icon(c, icon).Size(13, 13).TextColor(color)
		}
		ui.Text(c, label).FontSize(12).FontWeight(500).TextColor(color).SingleLine()
	}
	if fn == nil {
		ui.Row(c).Gap(4).Padding(4, 7).Radius(6).Shrink(0).Children(content)
		return
	}
	b := ui.ButtonBase(c.Key("action:"+label)).Gap(4).Padding(4, 7).Radius(6).Shrink(0).
		Cursor(ui.CursorPointer).Label(label)
	b.OnClick(fn)
	if b.Hovered() {
		b.Background(p.pill)
	}
	b.Children(content)
}

// chipButton is the outlined pill that commits a field.
func chipButton(c *ui.Context, p palette, label string, fn func()) {
	b := ui.ButtonBase(c).Height(26).Radius(13).Padding(0, 10).Center().Cursor(ui.CursorPointer).Label(label)
	b.Background(p.card).Border(1, p.line)
	if b.Hovered() {
		b.Background(p.pill)
	}
	b.OnClick(fn)
	b.Children(func() {
		ui.Text(c, label).FontSize(11.5).TextColor(p.fg2).SingleLine()
	})
}

// dialogScrim darkens a window behind a dialog without hiding it.
var dialogScrim = ui.RGBA(20, 22, 30, 0.22)

// linkButton is a short text action inside a card, in the color it is given.
func linkButton(c *ui.Context, p palette, label string, color ui.Color, fn func()) {
	b := ui.ButtonBase(c).Height(30).Radius(15).Padding(0, 8).Center().Cursor(ui.CursorPointer).Label(label)
	if b.Hovered() {
		b.Background(p.pill)
	}
	b.OnClick(fn)
	b.Children(func() {
		ui.Text(c, label).FontSize(12.5).FontWeight(500).TextColor(color).SingleLine()
	})
}

// quietSmall is the card's 30-DIP cancel action.
func quietSmall(c *ui.Context, p palette, label string, fn func()) {
	linkButton(c, p, label, p.muted, fn)
}

// primaryButton is the ink pill that confirms a dialog.
func primaryButton(c *ui.Context, p palette, label string, fn func()) {
	b := ui.ButtonBase(c).Height(34).Radius(17).Padding(0, 16).Center().Cursor(ui.CursorPointer).Label(label)
	b.Background(p.fg)
	if b.Hovered() {
		b.Background(p.fg.Mix(p.bg, 0.12))
	}
	b.OnClick(fn)
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(p.bg).SingleLine()
	})
}

// quietButton is a dialog's cancel action, text only.
func quietButton(c *ui.Context, p palette, label string, fn func()) {
	b := ui.ButtonBase(c).Height(34).Radius(17).Padding(0, 12).Center().Cursor(ui.CursorPointer).Label(label)
	if b.Hovered() {
		b.Background(p.pill)
	}
	b.OnClick(fn)
	b.Children(func() {
		ui.Text(c, label).FontSize(13).TextColor(p.muted).SingleLine()
	})
}

type chipTone uint8

const (
	chipNeutral chipTone = iota
	chipOK
	chipAccent
)

func chip(c *ui.Context, p palette, label string, tone chipTone) {
	e := ui.Box(c).Height(22).Radius(11).Padding(0, 9).Center().Shrink(0)
	color := p.fg2
	switch tone {
	case chipOK:
		e.Background(p.greenSoft)
		color = p.green
	case chipAccent:
		e.Background(p.accentSoft)
		color = p.accent
	default:
		e.Background(p.card).Border(1, p.line)
	}
	e.Children(func() {
		ui.Text(c, label).FontSize(11.5).TextColor(color).SingleLine()
	})
}

// toggle is the design's switch: a 32x19 track that fills with ink, with a
// white knob sliding across it. It is a SwitchBase, so a click, Space and
// assistive technology toggle it; changed runs with the new value.
func toggle(c *ui.Context, p palette, on *bool, label string, changed func(bool)) {
	b := ui.SwitchBase(c.Key("switch:"+label), on).Size(32, 19).Radius(9.5).Shrink(0).
		Cursor(ui.CursorPointer).Label(label)
	pos := b.Animate("on", boolFlag(*on), 140*time.Millisecond)
	b.Draw(func(pt *ui.Painter, r ui.Rect) {
		pt.Fill(r, p.line.Mix(p.fg, pos), r.H/2)
		knob := ui.Rect{X: r.X + 2 + pos*(r.W-r.H), Y: r.Y + 2, W: 15, H: 15}
		pt.Shadow(knob, 7.5, 0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.25))
		pt.Fill(knob, ui.RGB(255, 255, 255).Mix(p.bg, pos), 7.5)
	})
	b.OnChange(func() { changed(*on) })
}

// slider is the design's slider: a hairline rail, ink below the knob and a
// white knob.
func slider(c *ui.Context, p palette, value *float64, lo, hi float64, label string, changed func(float64)) ui.Element {
	s := ui.SliderBase(c, value, lo, hi).Label(label).Height(20).PaddingX(8)
	frac := float32(0)
	if hi > lo {
		frac = float32((*value - lo) / (hi - lo))
	}
	frac = min(max(frac, 0), 1)
	s.Draw(func(pt *ui.Painter, r ui.Rect) {
		rail := ui.Rect{X: r.X, Y: r.Y + 9, W: r.W, H: 4}
		pt.Fill(rail, p.line, 2)
		x := r.X + r.W*frac
		if x > r.X {
			pt.Fill(ui.Rect{X: r.X, Y: rail.Y, W: x - r.X, H: 4}, p.fg, 2)
		}
		knob := ui.Rect{X: x - 8, Y: r.Y + 3, W: 16, H: 16}
		pt.Shadow(knob, 8, 0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.22))
		pt.Fill(knob, ui.RGB(255, 255, 255), 8)
	})
	if s.Changed() {
		changed(*value)
	}
	return s
}

// settingRow is a settings line: a title, a subtitle and its control.
func settingRow(c *ui.Context, p palette, title, sub string, control func()) {
	settingRowColors(c, p, title, sub, p.fg, p.muted, control)
}

// settingRowColors is settingRow with the two text colors chosen by the
// caller, for a row that sits in the greyed hidden group.
func settingRowColors(c *ui.Context, p palette, title, sub string, titleColor, subColor ui.Color, control func()) {
	ui.Row(c).MinHeight(56).Padding(8, 14).Gap(12).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(500).TextColor(titleColor).SingleLine().Ellipsis("…")
			ui.Text(c, sub).FontSize(11.5).TextColor(subColor).SingleLine().Ellipsis("…")
		})
		control()
	})
}

func rowLine(c *ui.Context, p palette) {
	ui.Box(c).Height(1).Background(p.line2)
}

// fieldInput is the 28px pill field of the settings pages.
func fieldInput(c *ui.Context, p palette, value *string, placeholder, label string) ui.Element {
	return ui.TextInput(c, value).Width(150).Height(28).Radius(7).PaddingX(9).FontSize(12).
		Placeholder(placeholder).Label(label).Border(1, p.pill)
}

func fieldSelect(c *ui.Context, p palette, value *string, options []string, label string) ui.Element {
	return ui.Select(c, value, options).Width(150).Height(28).Radius(7).Padding(0, 9).FontSize(12).Label(label)
}

// staticText is a read-only value at the end of a settings row.
func staticText(c *ui.Context, p palette, text string) {
	ui.Text(c, text).FontSize(12).TextColor(p.muted).SingleLine()
}

func boolFlag(v bool) float32 {
	if v {
		return 1
	}
	return 0
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// contentEnter is the motion of a window's content as the window appears.
var contentEnter = ui.Animation{Duration: 180 * time.Millisecond, Ease: ui.EaseOut}

// dialogEnter fades and scales a dialog's panel in. A dialog has no exit:
// its overlay and backdrop go with it in the same frame.
var dialogEnter = ui.Animation{Duration: 150 * time.Millisecond, Ease: ui.EaseOut}

// dialogPose shrinks a dialog's panel from 98% as it fades in, once per
// open. Its overlay is new in the same frame, so a Transition could not
// animate the panel in; the pose carries its own first frame.
func dialogPose(c *ui.Context, panel ui.Element, entered *bool) {
	if !*entered {
		*entered = true
		panel.AnimatePose(ui.Pose{Scale: 0.98, Fade: 1}, dialogEnter)
		c.Invalidate()
		return
	}
	panel.AnimatePose(ui.Pose{Scale: 1, Fade: 0}, dialogEnter)
}

// enterPose rises an element six points as it fades in, once per window
// show. The flag starts false; the build that poses the start asks for the
// frame that sends it home, so the animation does not wait for engine news.
func enterPose(c *ui.Context, e ui.Element, entered *bool) {
	if !*entered {
		*entered = true
		e.AnimatePose(ui.Pose{Y: 6, Fade: 1}, contentEnter)
		c.Invalidate()
		return
	}
	e.AnimatePose(ui.Pose{Fade: 0}, contentEnter)
}
