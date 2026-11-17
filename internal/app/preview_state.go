//go:build preview

package app

import (
	"reflect"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

// previewLocale is the language the wasm preview starts in. The language
// action updates it, and RunPreview installs the locale function once; the
// test entry point owns the locale in tests.
var previewLocale = "zh-CN"

// previewHost is the window a scenario shows: the settings window for a
// settings page or a dialog over it, the tray menu panel for a tray state,
// and the flyout for everything else.
func previewHost(sc Scenario) Surface {
	if sc.usesSettingsPage() {
		return SurfaceSettings
	}
	if sc.Surface == SurfaceTrayMenu {
		return SurfaceTrayMenu
	}
	return SurfaceFlyout
}

// previewShowHost reveals the scenario's host window. The state test replaces
// it to record the host without creating a native window.
var previewShowHost = func(a *App, host Surface) {
	if host == SurfaceSettings {
		a.showPreviewSettings()
		return
	}
	if host == SurfaceTrayMenu {
		a.openPreviewMenu()
		return
	}
	a.showPreviewFlyout()
}

// previewStateCount is the number of states the wasm preview can select.
func previewStateCount() int { return len(PreviewScenarios()) }

// previewStateAt applies scenario i on a, starting from the base fixture so a
// state never inherits the one before it. The host window is shown first: its
// OnShow hook resets the log and enter flags, and the scenario must set them
// after that. The page is set before the window opens so it paints the right
// one. The tray menu builds its rows from the applied state, so it opens after
// Apply.
func previewStateAt(a *App, i int) {
	sc := PreviewScenarios()[i]
	a.resetPreviewState()
	if sc.usesSettingsPage() {
		a.page = sc.Page
	}
	a.closePreviewMenu()
	if sc.Surface == SurfaceTrayMenu {
		sc.Apply(a)
		previewShowHost(a, SurfaceTrayMenu)
	} else {
		previewShowHost(a, previewHost(sc))
		sc.Apply(a)
		previewResize(a)
	}
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
}

// resetPreviewState replaces a's state with a fresh fixture. The flyout and
// settings window handles belong to the running preview and stay. App holds an
// atomic.Bool, which go vet forbids copying by assignment; the reflected copy
// is safe because fresh is a new value.
func (a *App) resetPreviewState() {
	win, settingsWin, theme := a.win, a.settingsWin, a.setThemeSource
	fresh := Fixture(previewLocale)
	fresh.win, fresh.settingsWin = win, settingsWin
	if theme != nil {
		fresh.setThemeSource = theme
	}
	reflect.ValueOf(a).Elem().Set(reflect.ValueOf(fresh).Elem())
}

// showPreviewSettings opens the real settings window over the flyout, as the
// gear does in the app.
func (a *App) showPreviewSettings() {
	a.closePreviewMenu()
	if a.settingsWin == nil {
		a.settingsWin = a.newSettingsWindow()
	}
	previewResizeSettings()
	a.showSettings()
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
}

// previewKeys switches among the first nine states with 1 through 9, so a
// human can compare the same catalog the render writes.
func (a *App) previewKeys(c *ui.Context) {
	for i := 0; i < previewStateCount() && i < 9; i++ {
		i := i
		c.OnShortcut(0, ui.Key1+ui.Key(i), func() {
			previewStateAt(a, i)
		})
	}
}

// RunPreview opens the first catalog state in a browser or a native window,
// driven by the typed preview handlers.
func RunPreview() error {
	i18n.SetLocaleFunc(func() string { return previewLocale })
	mygo.App.WhenReady(func() {
		a := Fixture(previewLocale)
		first := PreviewScenarios()[0]
		previewStateAt(a, 0)
		var win *mygo.Window
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:  "saundo preview",
			Width:  flyoutWidth,
			Height: visibleFlyoutHeight(a),
			Hidden: previewHost(first) == SurfaceSettings,
			Content: ui.View(func(c *ui.Context) {
				a.win = win
				a.previewKeys(c)
				a.view(c)
			}),
		})
		a.win = win
		previewResize(a)
		previewBridge(a)
	})
	return mygo.App.Run()
}
