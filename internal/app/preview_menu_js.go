//go:build js && preview

package app

import "syscall/js"

func previewResizeMenu(h int) {
	fn := js.Global().Get("setPreviewSize")
	if fn.Type() != js.TypeFunction {
		return
	}
	fn.Invoke(previewMenuWidth, h)
}
