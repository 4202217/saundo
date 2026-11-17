//go:build preview

package app

// previewAction handles a view action and reports whether it did. RunPreview
// installs the fake reducer; the render command leaves it nil.
var previewAction func(a *App, name string, arg any) bool

func (a *App) previewAct(name string, arg any) bool {
	if previewAction == nil {
		return false
	}
	return previewAction(a, name, arg)
}
