package app

import (
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
)

// credentialBar is the flyout's recovery notice: the saved pairing could not
// be read and was moved aside. It is text-only so it shares the connection
// error bar's inset and height; the chevron opens the explanation.
func (a *App) credentialBar(c *ui.Context, p palette) {
	bar := ui.ButtonBase(c).Margin(8, 0, 0, 0).Padding(8, 10).Radius(8).
		Background(p.warningSoft).Cursor(ui.CursorPointer).Label(locales.CredentialResetBar())
	bar.OnClick(func() {
		a.credentialOpen = true
		a.credentialEntered = false
	})
	bar.Children(func() {
		ui.Text(c, locales.CredentialResetBar()).FontSize(11.5).TextColor(p.warning).Grow(1).SingleLine().Ellipsis("…")
		ui.Icon(c, iconChevronRight).Size(12, 12).TextColor(p.warning)
	})
}

// credentialDialog explains the reset and offers the two ways out: dismiss
// the notice, or reconnect the current receiver so a fresh pairing can save
// its key into the rebuilt store.
func (a *App) credentialDialog(c *ui.Context, p palette) {
	if !a.snap.CredentialReset {
		a.credentialOpen = false
		return
	}
	if !a.credentialOpen {
		return
	}
	modal := ui.DialogBase(c, &a.credentialOpen, func(back, panel ui.Element) {
		back.Background(dialogScrim)
		panel.Width(380).Padding(20).Radius(14).Background(p.card).Border(1, p.line).
			Shadow(0, 24, 60, 0, ui.RGBA(20, 22, 30, 0.28))
		dialogPose(c, panel, &a.credentialEntered)
		dialogHead(c, p, iconKey, locales.CredentialResetTitle(), locales.CredentialResetSub(), false, nil)
		ui.Text(c, locales.CredentialResetBody()).FontSize(12.5).TextColor(p.fg2).Margin(14, 0, 0, 0)
		ui.Text(c, locales.CredentialResetHint()).FontSize(11.5).TextColor(p.muted).Margin(8, 0, 0, 0)
		ui.Row(c).Justify(ui.End).Gap(8).Margin(20, 0, 0, 0).Children(func() {
			quietButton(c, p, locales.CredentialResetDismiss(), a.dismissCredentialReset)
			primaryButton(c, p, locales.CredentialResetPair(), a.pairAgain)
		})
	})
	if modal.Dismissed() {
		a.credentialOpen = false
	}
}

func (a *App) dismissCredentialReset() {
	a.credentialOpen = false
	if a.eng != nil {
		a.eng.DismissCredentialReset()
	}
}

func (a *App) pairAgain() {
	a.credentialOpen = false
	if d, ok := a.currentDevice(); ok {
		a.connectDevice(d.ID)
		return
	}
	a.page = pageReceivers
	a.showSettings()
}
