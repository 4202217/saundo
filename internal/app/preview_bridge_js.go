//go:build js && preview

package app

import (
	"syscall/js"

	"saundo/internal/receivers"
)

// previewBridge exposes the state catalog and the settings window to the
// page, so a human can switch states from a button and a tool can switch
// them without trusted key events.
func previewBridge(a *App) {
	js.Global().Set("setPreviewState", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		i := args[0].Int()
		if i < 0 || i >= len(PreviewStates) {
			return nil
		}
		a.closePreviewMenu()
		if a.settingsWin != nil {
			a.settingsWin.Hide()
		}
		if a.win != nil {
			a.win.Show()
		}
		PreviewStates[i].Apply(a)
		previewResize(a)
		if a.win != nil {
			a.win.Invalidate()
		}
		return nil
	}))
	js.Global().Set("openPreviewSettings", js.FuncOf(func(this js.Value, args []js.Value) any {
		a.showPreviewSettings()
		return nil
	}))
	js.Global().Set("openPreviewMenu", js.FuncOf(func(this js.Value, args []js.Value) any {
		a.openPreviewMenu()
		return nil
	}))
	js.Global().Set("setPreviewName", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		a.nameDraft = args[0].String()
		a.actName()
		if a.settingsWin != nil {
			a.settingsWin.Invalidate()
		}
		return nil
	}))
	js.Global().Set("addPreviewReceiver", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 2 {
			return nil
		}
		a.actReceiverAdd(receivers.Spec{
			Name: args[0].String(),
			Type: receivers.TypeAirPlay2,
			Host: args[1].String(),
			Port: 7000,
		})
		if a.settingsWin != nil {
			a.settingsWin.Invalidate()
		}
		return nil
	}))
}
