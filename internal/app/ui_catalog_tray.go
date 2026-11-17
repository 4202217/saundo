//go:build preview

package app

// The tray menu's catalog row. The shipped menu is native and stays outside
// the render paths; this row draws the preview panel, which uses the same
// trayModel and trayMenuItems the native menu is built from.

var trayScenarios = []Scenario{
	{ID: "tray.menu", Name: "tray-menu", Dark: "tray-menu-dark", Surface: SurfaceTrayMenu, Size: previewMenuSize, Theme: ThemeBoth, Apply: applyStreamingCard},
}
