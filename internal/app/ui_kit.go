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
	case s.TakenOver:
		return phaseError
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

// faceOf is the state face for a snapshot. Connected with no audible audio
// is the plain smile; disconnected is the open-eyed line face, so the tray
// separates an idle app from a silent stream.
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
	return "idle-open"
}

// sectionHead is a flyout section header. All sections use it, so their labels
// share one left edge. busy shows the latency spinner after the label; action
// draws an optional trailing control.
func sectionHead(c *ui.Context, p palette, label, note string, busy bool, action func()) {
	ui.Row(c).WidthPercent(100).Padding(6, 2, 2, 2).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(11.5).FontWeight(600).LetterSpacing(0.55).TextColor(p.muted)
		if busy {
			ui.Spinner(c).Size(10, 10).TextColor(p.accent)
		}
		ui.Spacer(c)
		if note != "" {
			ui.Text(c, note).FontSize(11.5).TextColor(p.muted).SingleLine()
		}
		if action != nil {
			action()
		}
	})
}

// groupHead is a settings page's section label above a card: the 11.5 / 600 /
// letter-spaced muted group style, an optional count badge, and an optional
// trailing row action. The row is one fixed 26 DIP whether or not it carries
// an action, so the label and the card below it keep one baseline on every
// page; without the fixed height the action would stretch the row and push
// its centered label down.
func groupHead(c *ui.Context, p palette, label string, count int, top float32, action func()) {
	ui.Row(c).Margin(top, 0, 0, 0).FillWidth().Height(26).Gap(7).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, label).FontSize(11.5).FontWeight(600).LetterSpacing(0.55).TextColor(p.muted)
		if count > 0 {
			countBadge(c, p, count)
		}
		if action != nil {
			ui.Spacer(c)
			action()
		}
	})
}

// iconButton is a borderless 26px square holding one icon.
func iconButton(c *ui.Context, p palette, icon *ui.SVG, label string, size float32, fn func()) {
	b := ui.ButtonBase(c).Size(26, 26).Radius(7).Center().Cursor(ui.CursorPointer).Label(label)
	b.OnClick(fn)
	hoverFill(b, p.pill, 7, 0)
	b.Children(func() {
		ui.Icon(c, icon).Size(size, size).TextColor(p.muted)
	})
}

// rowAction is a row-level text action: a 26-DIP pill with an icon and a word.
// An action without a handler is a status, not a button, so it takes no focus.
func rowAction(c *ui.Context, p palette, key, label string, icon *ui.SVG, color ui.Color, fn func()) ui.Element {
	content := func() {
		if icon != nil {
			ui.Icon(c, icon).Size(12, 12).TextColor(color)
		}
		ui.Text(c, label).FontSize(11.5).FontWeight(500).TextColor(color).SingleLine()
	}
	if fn == nil {
		return ui.Row(c).Height(26).Gap(5).Padding(0, 8).Radius(13).Shrink(0).Children(content)
	}
	b := ui.ButtonBase(c.Key(key)).Height(26).Gap(5).Padding(0, 8).Radius(13).Shrink(0).
		Cursor(ui.CursorPointer).Label(label)
	b.OnClick(fn)
	hoverFill(b, p.pill, 13, 0)
	b.Children(content)
	return b
}

// textButton is the flyout's row action, keyed by its label.
func textButton(c *ui.Context, p palette, label string, icon *ui.SVG, color ui.Color, fn func()) {
	rowAction(c, p, "action:"+label, label, icon, color, fn)
}

// chipButton is the outlined pill that commits a field.
func chipButton(c *ui.Context, p palette, label string, fn func()) {
	b := ui.ButtonBase(c).Height(28).Radius(14).Padding(0, 10).Center().Cursor(ui.CursorPointer).Label(label)
	b.Background(p.card).Border(1, p.line)
	hoverFill(b, p.pill, 13, 1)
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
	hoverFill(b, p.pill, 15, 0)
	b.OnClick(fn)
	b.Children(func() {
		ui.Text(c, label).FontSize(12.5).FontWeight(500).TextColor(color).SingleLine()
	})
}

// primaryButton is the ink pill that confirms a dialog.
func primaryButton(c *ui.Context, p palette, label string, fn func()) {
	b := ui.ButtonBase(c).Height(34).Radius(17).Padding(0, 16).Center().Cursor(ui.CursorPointer).Label(label)
	b.Background(p.fg)
	hoverFill(b, p.fg.Mix(p.bg, 0.12), 17, 0)
	b.OnClick(fn)
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(p.bg).SingleLine()
	})
}

// quietButton is a dialog's cancel action, text only.
func quietButton(c *ui.Context, p palette, label string, fn func()) {
	b := ui.ButtonBase(c).Height(34).Radius(17).Padding(0, 12).Center().Cursor(ui.CursorPointer).Label(label)
	hoverFill(b, p.pill, 17, 0)
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
	chipDanger
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
	case chipDanger:
		e.Background(p.redSoft)
		color = p.red
	default:
		e.Background(p.card).Border(1, p.line)
	}
	e.Children(func() {
		ui.Text(c, label).FontSize(11.5).TextColor(color).SingleLine()
	})
}

// inlineChip is the 20-DIP chip beside a row's name. dimmed greys it in the
// hidden group.
func inlineChip(c *ui.Context, p palette, label string, tone chipTone, dimmed bool) {
	e := ui.Box(c).Height(20).Radius(10).Padding(0, 8).Center().Shrink(0)
	color := p.fg2
	switch {
	case dimmed:
		e.Background(p.pill)
		color = p.muted
	case tone == chipOK:
		e.Background(p.greenSoft)
		color = p.green
	case tone == chipAccent:
		e.Background(p.accentSoft)
		color = p.accent
	default:
		e.Background(p.card).Border(1, p.line)
	}
	e.Children(func() {
		ui.Text(c, label).FontSize(10.5).TextColor(color).SingleLine()
	})
}

// iconChip is the 20-DIP status chip with a leading glyph: the settings and
// flyout rows use it for a password or auth gate. dimmed greys it in the
// hidden group.
func iconChip(c *ui.Context, p palette, ic *ui.SVG, label string, tone chipTone, dimmed bool) {
	e := ui.Row(c).Height(20).Radius(10).Padding(0, 7).Gap(4).AlignItems(ui.Center).Shrink(0).
		Label(label)
	color := p.fg2
	switch {
	case dimmed:
		e.Background(p.pill)
		color = p.muted
	case tone == chipAccent:
		e.Background(p.accentSoft)
		color = p.accent
	case tone == chipDanger:
		e.Background(p.redSoft)
		color = p.red
	case tone == chipOK:
		e.Background(p.greenSoft)
		color = p.green
	default:
		e.Background(p.pill)
		color = p.muted
	}
	e.Children(func() {
		ui.Icon(c, ic).Size(12, 12).TextColor(color)
		ui.Text(c, label).FontSize(10.5).TextColor(color).SingleLine()
	})
}

// countChip is the stereo group's online count: a dot and the member count,
// green when every member is online and red when one is down.
func countChip(c *ui.Context, p palette, text, label string, full bool, dimmed bool) {
	bg, color := p.greenSoft, p.green
	switch {
	case dimmed:
		bg, color = p.pill, p.muted
	case !full:
		bg, color = p.redSoft, p.red
	}
	e := ui.Row(c).Height(20).Radius(10).Padding(0, 8).Gap(5).AlignItems(ui.Center).Shrink(0).
		Background(bg).Label(label).Tooltip(label)
	e.Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(color)
		ui.Text(c, text).FontSize(10.5).TextColor(color).FontFeatures("tnum").SingleLine()
	})
}

// countBadge is the small count beside a group's label.
func countBadge(c *ui.Context, p palette, n int) {
	ui.Box(c).Height(18).MinWidth(18).Radius(9).Padding(0, 6).Center().Shrink(0).
		Background(p.pill).Children(func() {
		ui.Textf(c, "%d", n).FontSize(10.5).TextColor(p.muted)
	})
}

// cardAction is the 30-DIP filled action at a card's tail.
func cardAction(c *ui.Context, p palette, label string, fn func()) {
	b := ui.ButtonBase(c).Height(30).Radius(15).Padding(0, 14).Center().Cursor(ui.CursorPointer).Label(label)
	b.Background(p.fg)
	hoverFill(b, p.fg.Mix(p.bg, 0.12), 15, 0)
	b.OnClick(fn)
	b.Children(func() {
		ui.Text(c, label).FontSize(12.5).FontWeight(500).TextColor(p.bg).SingleLine()
	})
}

// cardHead is a settings card's header: a fixed 22-DIP leading tile, the title,
// and a trailing cluster. The tile stays even without an icon, so every card's
// title starts at the same x. padX lets a card that already pads its body keep
// one 14-DIP inset.
func cardHead(c *ui.Context, p palette, icon *ui.SVG, title string, size, padX float32, trailing func()) {
	ui.Row(c).Height(44).Padding(0, padX).Gap(8).AlignItems(ui.Center).Children(func() {
		tile := ui.Box(c).Size(22, 22).Radius(6).Shrink(0).Center()
		if icon != nil {
			tile.Background(p.pill).Children(func() {
				ui.Icon(c, icon).Size(13, 13).TextColor(p.ctl)
			})
		}
		ui.Text(c, title).FontSize(size).FontWeight(600).Grow(1).SingleLine().Ellipsis("…")
		if trailing != nil {
			trailing()
		}
	})
}

type segmentItem struct {
	label string
	icon  *ui.SVG
}

// segmentAlign places a segment's content. segmentLeading puts it on the
// card's content column instead of centering it in the segment.
type segmentAlign uint8

const (
	segmentCenter segmentAlign = iota
	segmentLeading
)

// segmented is the shared 30-DIP segmented control. width 0 fills the parent.
// dimmed greys every label and disables the track.
func segmented(c *ui.Context, p palette, seg *int, items []segmentItem, width float32, dimmed bool, align segmentAlign, label string) ui.Element {
	parts := ui.SegmentedBase(c.Key(label), seg, len(items))
	track := parts.Track.Height(30).Radius(8).Background(p.pill).Padding(2).Gap(2).
		Label(label).Disabled(dimmed)
	if width > 0 {
		track.Width(width)
	}
	track.Children(func() {
		for i, item := range items {
			s := parts.Segment(i).Grow(1).Height(26).Radius(6).Gap(4).
				Cursor(ui.CursorPointer).Label(item.label)
			if align == segmentLeading {
				s.Justify(ui.Start).AlignItems(ui.Center).Padding(0, 8)
			} else {
				s.Center()
			}
			if i == *seg {
				if p.dark {
					s.Background(p.pillHover)
				} else {
					s.Background(p.card)
				}
				s.Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.14))
			}
			s.Children(func() {
				var ic ui.Element
				if item.icon != nil {
					ic = ui.Icon(c, item.icon).Size(14, 14)
				}
				tx := ui.Text(c, item.label).FontSize(11.5).SingleLine()
				switch {
				case dimmed:
					tx.TextColor(p.faint)
					if item.icon != nil {
						ic.TextColor(p.faint)
					}
				case i == *seg:
					tx.TextColor(p.fg)
					if item.icon != nil {
						ic.TextColor(p.accent)
					}
				default:
					tx.TextColor(p.muted)
					if item.icon != nil {
						ic.TextColor(p.muted)
					}
				}
			})
		}
	})
	return track
}

// infoNote is a muted caveat with an info glyph. It sits under a card, where
// the glyph marks something outside the card that can stop the feature. Each
// line is one paragraph; the note wraps instead of truncating.
func infoNote(c *ui.Context, p palette, lines ...string) {
	ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
		ui.Box(c).Size(16, 16).Shrink(0).Center().Children(func() {
			ui.Icon(c, iconInfo).Size(13, 13).TextColor(p.muted)
		})
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			for _, line := range lines {
				ui.Text(c, line).FontSize(11.5).TextColor(p.muted)
			}
		})
	})
}

// emptyCopy is an empty state's text block. center is for the flyout's card;
// a settings page uses the same type at the page's own inset.
func emptyCopy(c *ui.Context, p palette, title, hint string, center bool) {
	col := ui.Column(c).Gap(3)
	if center {
		col.AlignItems(ui.Center)
	}
	col.Children(func() {
		t := ui.Text(c, title).FontSize(12.5).FontWeight(500).TextColor(p.fg2).SingleLine()
		h := ui.Text(c, hint).FontSize(11.5).TextColor(p.muted)
		if center {
			t.TextAlign(ui.Center)
			h.TextAlign(ui.Center)
		}
	})
}

// toggle is the design's switch: a 32x19 track that fills with ink, with a
// white knob sliding across it. It is a SwitchBase, so a click, Space and
// assistive technology toggle it; changed runs with the new value.
func toggle(c *ui.Context, p palette, on *bool, label string, changed func(bool)) ui.Element {
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
	return b
}

// slider is the design's slider: a hairline rail, ink below the knob and a
// white knob.
func slider(c *ui.Context, p palette, value *float64, lo, hi float64, label string, changed func(float64)) ui.Element {
	s := ui.SliderBase(c.Key(label), value, lo, hi).Label(label).Height(20).PaddingX(8)
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
// caller, for a row that sits in the greyed hidden group. An empty title or
// subtitle leaves that line out, for rows a wide control labels by itself.
func settingRowColors(c *ui.Context, p palette, title, sub string, titleColor, subColor ui.Color, control func()) {
	ui.Row(c).MinHeight(56).Padding(8, 14).Gap(12).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			if title != "" {
				ui.Text(c, title).FontSize(13).FontWeight(500).TextColor(titleColor).SingleLine().Ellipsis("…")
			}
			if sub != "" {
				ui.Text(c, sub).FontSize(11.5).TextColor(subColor).SingleLine().Ellipsis("…")
			}
		})
		control()
	})
}

func rowLine(c *ui.Context, p palette) {
	ui.Box(c).Height(1).Background(p.line2)
}

// hoverFill paints a control's hover look in its Draw phase, so the pointer
// entering or leaving repaints instead of rebuilding the view. inset keeps a
// border's pixels clear; radius is the fill's corner radius.
func hoverFill(e ui.Element, c ui.Color, radius, inset float32) {
	e.Draw(func(pt *ui.Painter, r ui.Rect) {
		if !e.Hovered() || c.A == 0 {
			return
		}
		r = ui.Rect{X: r.X + inset, Y: r.Y + inset, W: r.W - 2*inset, H: r.H - 2*inset}
		if r.W > 0 && r.H > 0 {
			pt.Fill(r, c, radius)
		}
	})
}

// fieldInput is the 28px pill field of the settings pages.
func fieldInput(c *ui.Context, p palette, value *string, placeholder, label string) ui.Element {
	return ui.TextInput(c.Key(label), value).Width(150).Height(28).Radius(7).PaddingX(9).FontSize(12.5).
		Placeholder(placeholder).Label(label).Border(1, p.pill)
}

func fieldSelect(c *ui.Context, p palette, value *string, options []string, label string) ui.Element {
	return ui.Select(c.Key(label), value, options).Width(150).Height(28).Radius(7).Padding(0, 9).FontSize(12.5).Label(label)
}

// staticText is a read-only value at the end of a settings row.
func staticText(c *ui.Context, p palette, text string) {
	ui.Text(c, text).FontSize(12.5).TextColor(p.muted).SingleLine()
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

// barTone is the status-bar family shared by the session notice, the
// connection error, and the credential notice.
type barTone uint8

const (
	barAccent barTone = iota
	barNeutral
	barDanger
	barWarning
)

// statusBar is one 44-DIP line under the device card: a 16-DIP glyph, one
// single-line message, and an optional trailing action cluster. Every bar in
// that slot uses this shape, so they read as one family.
func statusBar(c *ui.Context, p palette, tone barTone, glyph *ui.SVG, text string, trailing func()) {
	bg, iconColor, textColor := p.accentSoft, p.accent, p.fg2
	switch tone {
	case barNeutral:
		bg, iconColor = p.pill, p.muted
	case barDanger:
		bg, iconColor, textColor = p.redSoft, p.red, p.red
	case barWarning:
		bg, iconColor, textColor = p.warningSoft, p.warning, p.warning
	}
	bar := ui.Row(c).MinHeight(44).Padding(8, 10).Radius(9).Gap(9).
		AlignItems(ui.Center).Background(bg).Border(1, p.line)
	bar.Children(func() {
		ui.Icon(c, glyph).Size(16, 16).TextColor(iconColor)
		ui.Text(c, text).FontSize(11.5).TextColor(textColor).Grow(1).SingleLine().Ellipsis("…")
		if trailing != nil {
			trailing()
		}
	})
}
