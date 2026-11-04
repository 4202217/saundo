package app

import (
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"saundo/cmd/saundo/locales"
)

// openUpdateNotes shows the offered version's release notes in the settings
// window, with every version released since this build.
func (a *App) openUpdateNotes() {
	if a.upd.notes == "" {
		return
	}
	a.notesOpen = true
	a.notesEntered = false
	a.notesRange = true
	a.notesExpanded = map[string]bool{}
	a.loadReleaseHistory()
}

// updateNotesDialog is the read-only release notes dialog, drawn over the
// settings window like the diagnostics dialog.
func (a *App) updateNotesDialog(c *ui.Context, p palette) {
	if !a.notesOpen {
		return
	}
	modal := ui.DialogBase(c, &a.notesOpen, func(back, panel ui.Element) {
		back.Background(dialogScrim)
		panel.Width(600).Padding(18, 20, 18, 20).Radius(14).Background(p.card).Border(1, p.line).
			Shadow(0, 24, 60, 0, ui.RGBA(20, 22, 30, 0.28))
		dialogPose(c, panel, &a.notesEntered)
		ui.Row(c).Gap(12).Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, locales.UpdateNotesTitle(a.upd.version)).FontSize(17.5).FontWeight(650)
				ui.Text(c, a.notesRangeMeta()).FontSize(12).TextColor(p.muted).
					FontFeatures("tnum").SingleLine().Ellipsis("…")
			})
			choice := locales.UpdateRangeSince()
			if !a.notesRange {
				choice = locales.UpdateRangeLatest()
			}
			options := []string{locales.UpdateRangeSince(), locales.UpdateRangeLatest()}
			sel := ui.Select(c, &choice, options).Height(26).Radius(7).Padding(0, 8).FontSize(11.5).
				Label(locales.UpdateRangeSince())
			if sel.Changed() {
				a.notesRange = choice == locales.UpdateRangeSince()
			}
		})
		ui.Scroll(c).Height(232).Margin(12, 0, 0, 0).Children(func() {
			ui.Column(c).Gap(0).Children(func() { a.notesHistory(c, p) })
		})
		ui.Row(c).Justify(ui.End).Margin(12, 0, 0, 0).Children(func() {
			primaryButton(c, p, locales.LogClose(), func() { a.notesOpen = false })
		})
	})
	if modal.Dismissed() {
		a.notesOpen = false
	}
}

// notesTheme keeps release notes in the dialog's type: small block labels,
// tight spacing, and the dialog's colors.
func notesTheme(p palette) *markdown.Theme {
	base := &ui.Theme{
		Background: p.card,
		Text:       p.fg,
		TextMuted:  p.muted,
		Accent:     p.accent,
		Border:     p.line,
		Radius:     7,
		FontSize:   12,
	}
	t := markdown.DefaultTheme(base)
	t.FontSize = 12
	t.LineHeight = 1.5
	t.BlockSpacing = 0.45
	t.ItemSpacing = 0.18
	t.ListIndent = 1.35
	t.Headings = [6]markdown.HeadingStyle{
		{Size: 1.0, Weight: 650, Color: p.fg, Top: 0.6, Bottom: 0.2},
		{Size: 0.95, Weight: 650, Color: p.fg, Top: 0.6, Bottom: 0.2},
		{Size: 0.88, Weight: 600, Color: p.muted, Top: 0.6, Bottom: 0.2},
		{Size: 0.85, Weight: 600, Color: p.muted, Top: 0.5, Bottom: 0.2},
		{Size: 0.85, Weight: 600, Color: p.muted, Top: 0.5, Bottom: 0.2},
		{Size: 0.85, Weight: 600, Color: p.muted, Top: 0.5, Bottom: 0.2},
	}
	return t
}

// notesLoading reports whether the offered version's notes are still being
// fetched in a language other than the manifest's English, so showing the
// manifest first would flash the wrong language.
func (a *App) notesLoading() bool {
	return a.historyState == historyLoading && changelogURL(i18n.Locale()) != updateChangelog
}

// notesHistory renders the offered version in full, then one folded row per
// older version, newest first.
func (a *App) notesHistory(c *ui.Context, p palette) {
	a.md.Theme = notesTheme(p)
	entries := a.notesEntries()
	pending := a.notesLoading()
	a.notesHeading(c, p, entries[0])
	ui.Column(c).Padding(0, 0, 4, 0).Children(func() {
		if pending {
			ui.Row(c).Gap(8).Padding(8, 0).Children(func() {
				ui.Spinner(c).Size(12, 12)
				ui.Text(c, locales.UpdateHistoryLoading()).FontSize(11.5).TextColor(p.muted)
			})
			return
		}
		markdown.View(c, entries[0].Notes, &a.md)
	})
	if !a.notesRange {
		return
	}
	for _, e := range entries[1:] {
		ui.Box(c).Height(1).Background(p.line2)
		a.notesFoldedRow(c, p, e)
	}
	switch a.historyState {
	case historyLoading:
		if !pending {
			ui.Row(c).Gap(8).Padding(8, 0).Children(func() {
				ui.Spinner(c).Size(12, 12)
				ui.Text(c, locales.UpdateHistoryLoading()).FontSize(11.5).TextColor(p.muted)
			})
		}
	case historyFailed:
		ui.Row(c).Gap(8).Padding(8, 0).Children(func() {
			ui.Text(c, locales.UpdateHistoryFailed()).FontSize(11.5).TextColor(p.muted).Grow(1).SingleLine().Ellipsis("…")
			ui.Link(c, locales.UpdateHistoryLink(), "https://github.com/"+updateRepo+"/releases").FontSize(11.5)
		})
	}
}

// notesHeading is a version's number and date.
func (a *App) notesHeading(c *ui.Context, p palette, e releaseNote) {
	ui.Row(c).Height(28).Gap(8).Children(func() {
		ui.Text(c, e.Version).FontSize(13).FontWeight(650).FontFeatures("tnum")
		ui.Text(c, updateDate(e.Date)).FontSize(10.5).TextColor(p.muted).FontFeatures("tnum")
		ui.Spacer(c)
	})
}

// notesFoldedRow is an older version row: click it to expand its notes.
func (a *App) notesFoldedRow(c *ui.Context, p palette, e releaseNote) {
	open := a.notesExpanded[e.Version]
	row := ui.Row(c).Height(34).Gap(8).Cursor(ui.CursorPointer)
	row.OnClick(func() { a.notesExpanded[e.Version] = !open })
	row.Children(func() {
		ui.Text(c, e.Version).FontSize(12.5).FontWeight(600).FontFeatures("tnum")
		ui.Text(c, updateDate(e.Date)).FontSize(11).TextColor(p.muted).FontFeatures("tnum")
		ui.Spacer(c)
		kind, n := summarizeNotes(e.Notes)
		ui.Text(c, noteCountLabel(kind, n)).FontSize(11).TextColor(p.muted).SingleLine()
		chevron := ui.Icon(c, iconChevron).Size(11, 11).TextColor(p.muted)
		if open {
			chevron.Rotate(0)
		} else {
			chevron.Rotate(-90)
		}
	})
	if open {
		ui.Column(c).Padding(0, 0, 6, 0).Children(func() {
			markdown.View(c, e.Notes, &a.md)
		})
	}
}
