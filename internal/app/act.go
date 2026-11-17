package app

// act runs a view's action, or the preview build's fake action when one is
// installed. The preview build installs one in RunPreview; production never
// does, so previewAct returns false and the real action runs.
func (a *App) act(name string, arg any, real func()) {
	if a.previewAct(name, arg) {
		return
	}
	real()
}
