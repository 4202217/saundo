package app

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
)

func (a *App) logDialog(c *ui.Context, p palette) {
	if !a.showLog {
		return
	}
	lines := a.snap.LogLines
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	ui.DialogBase(c, &a.showLog, func(back, panel ui.Element) {
		back.Background(dialogScrim)
		panel.Width(430).Padding(20).Radius(14).Background(p.card).Border(1, p.line).
			Shadow(0, 24, 60, 0, ui.RGBA(20, 22, 30, 0.28))
		dialogPose(c, panel, &a.logEntered)
		dialogHead(c, p, iconDoc, locales.LogTitle(), locales.LogRecentLines(len(lines)), false, nil)
		ui.Scroll(c).Height(300).Margin(14, 0, 0, 0).Padding(10).Radius(10).Background(p.pill).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				if len(lines) == 0 {
					ui.Text(c, locales.LogEmpty()).FontSize(11.5).TextColor(p.muted)
				}
				for _, line := range lines {
					ui.Text(c, line).Font("Cascadia Mono").FontSize(10.5).TextColor(p.fg2)
				}
			})
		})
		ui.Row(c).Justify(ui.End).Gap(8).Margin(16, 0, 0, 0).Children(func() {
			quietButton(c, p, locales.LogCopy(), func() {
				c.Services().WriteClipboard(strings.Join(lines, "\n"))
			})
			primaryButton(c, p, locales.LogClose(), func() { a.showLog = false })
		})
	})
}
