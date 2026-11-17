//go:build preview

package app

var dialogScenarios = []Scenario{
	{ID: "dialog.credential", Name: "dialog-credential", Dark: "dialog-credential-dark", Surface: SurfaceDialog, Size: flyoutSize, Theme: ThemeBoth, Apply: applyCredentialDialog},
	{ID: "dialog.receiver.add", Name: "dialog-receiver-add", Surface: SurfaceDialog, Host: SurfaceSettings, Page: pageReceivers, Size: sizeSettings, Apply: applyAddReceiverDialog},
	{ID: "dialog.receiver.edit", Name: "dialog-receiver-edit", Surface: SurfaceDialog, Host: SurfaceSettings, Page: pageReceivers, Size: sizeSettings, Apply: applyEditReceiverDialog},
	{ID: "dialog.receiver.remove", Name: "dialog-receiver-remove", Surface: SurfaceDialog, Host: SurfaceSettings, Page: pageReceivers, Size: sizeSettings, Apply: applyRemoveReceiverDialog},
	{ID: "dialog.receiver.password", Name: "dialog-receiver-password", Surface: SurfaceDialog, Host: SurfaceSettings, Page: pageReceivers, Size: sizeSettings, Apply: applyReceiverPasswordDialog},
	{ID: "dialog.pin", Name: "dialog-pin", Surface: SurfaceDialog, Size: flyoutSize, Apply: applyPinDialog},
	{ID: "dialog.password", Name: "dialog-password", Surface: SurfaceDialog, Size: flyoutSize, Apply: applyPasswordDialog},
	{ID: "dialog.log", Name: "dialog-log", Surface: SurfaceDialog, Host: SurfaceSettings, Page: pageAbout, Size: sizeSettings, Apply: applyLogDialog},
	{ID: "probe.pin.enter", Name: "probe-pin-enter", Surface: SurfaceDialog, Size: flyoutSize, Frames: 3, Apply: applyPinDialog},
}

func applyCredentialDialog(a *App) {
	a.snap.CredentialReset = true
	a.credentialOpen = true
}

// applyReceiverDialogPage is the receivers page the add, edit, remove, and
// password dialogs open over: the expanded manual row and the open hidden
// group.
func applyReceiverDialogPage(a *App) {
	applyReceiverRows(a)
	a.receiverOpen = map[string]bool{"m:ab12": true}
	a.receiverHiddenOpen = true
}

func applyAddReceiverDialog(a *App) {
	applyReceiverDialogPage(a)
	a.openAddReceiver()
}

func applyEditReceiverDialog(a *App) {
	applyReceiverDialogPage(a)
	if v, ok := a.receiverViewFor("m:ab12"); ok {
		a.beginEditReceiver(v)
	}
}

func applyRemoveReceiverDialog(a *App) {
	applyReceiverDialogPage(a)
	if v, ok := a.receiverViewFor("m:ab12"); ok {
		a.beginEditReceiver(v)
		a.openRemoveReceiver(v)
	}
}

func applyReceiverPasswordDialog(a *App) {
	applyReceiverDialogPage(a)
	a.receiverDialog = receiverDialogState{kind: receiverDialogPassword, key: "m:ab12", entered: false}
	a.receiverDialogOpen = true
}

func applyPinDialog(a *App) {
	a.snap.PinRequired = true
	a.snap.PinDevice = "Apple TV"
	a.pinOpen = true
	a.pinCode = "3939"
}

func applyPasswordDialog(a *App) {
	a.snap.PasswordRequired = true
	a.snap.PasswordDevice = "MacBook"
	a.passwordOpen = true
	a.passwordPrompt = "hunter2"
}

func applyLogDialog(a *App) {
	a.showLog = true
	a.snap.LogLines = []string{
		"2026-11-01 12:00:01 saundo 0.3.0",
		"2026-11-01 12:00:02 discovery: 3 devices",
		"2026-11-01 12:00:03 connect AA:BB:CC:DD:EE:01",
		"2026-11-01 12:00:03 pair-verify ok",
		"2026-11-01 12:00:04 ptp: follower ready",
		"2026-11-01 12:00:05 capture open: PHL 27E2F7901 (NVIDIA High Definition Audio) (48000 Hz)",
	}
}
