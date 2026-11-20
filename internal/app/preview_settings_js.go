//go:build js && preview

package app

import "syscall/js"

// previewResizeSettings sizes the page to the settings window.
func previewResizeSettings() {
	fn := js.Global().Get("setPreviewSize")
	if fn.Type() != js.TypeFunction {
		return
	}
	fn.Invoke(settingsContentWidth, settingsContentHeight)
}
