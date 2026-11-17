//go:build !preview

package app

func (a *App) previewAct(string, any) bool { return false }
