//go:build js && preview

package app

import "syscall/js"

// previewResize asks the page to size the window's element to the flyout,
// because a browser window's size options do nothing.
func previewResize(a *App) {
	fn := js.Global().Get("setPreviewSize")
	if fn.Type() != js.TypeFunction {
		return
	}
	fn.Invoke(flyoutWidth, visibleFlyoutHeight(a))
}
