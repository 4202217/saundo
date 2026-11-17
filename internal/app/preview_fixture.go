//go:build preview

package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/engine"
	"saundo/internal/receivers"
	"saundo/internal/settings"
)

var previewLocale = "zh-CN"

func init() {
	i18n.SetLocaleFunc(func() string { return previewLocale })
}

// PreviewState is one flyout state the wasm preview and the render share.
type PreviewState struct {
	Name  string
	Apply func(*App)
}

var PreviewStates = []PreviewState{
	{"streaming", func(a *App) {
		a.snap.State = sender.StateStreaming
		a.snap.ConnectedName = "HomePods"
		a.snap.SelectedID = "AA:BB:CC:DD:EE:01"
		a.snap.Error = ""
		a.selID = "AA:BB:CC:DD:EE:01"
	}},
	{"connecting", func(a *App) {
		a.snap.State = sender.StateHandshake
		a.snap.ConnectedName = ""
		a.snap.SelectedID = "AA:BB:CC:DD:EE:01"
		a.snap.Error = ""
		a.selID = "AA:BB:CC:DD:EE:01"
	}},
	{"error", func(a *App) {
		a.snap.State = sender.StateIdle
		a.snap.ConnectedName = ""
		a.snap.SelectedID = "AA:BB:CC:DD:EE:01"
		a.snap.Error = "The receiver did not respond."
		a.selID = "AA:BB:CC:DD:EE:01"
	}},
}

// PreviewApp is the fake state the preview and the render start from.
func PreviewApp() *App {
	capture := "扬声器"
	if previewLocale != "zh-CN" {
		capture = "Speakers"
	}
	a := &App{
		cfg:               settings.Default(),
		volume:            65,
		captureNames:      []string{locales.CaptureFollowSystem(), capture},
		captureIDs:        []string{"", "{endpoint}"},
		captureLabels:     []string{locales.CaptureFollowSystem(), capture},
		captureChoice:     locales.CaptureFollowSystem(),
		nameDraft:         locales.AppName(),
		latency:           200,
		latencyMode:       settings.LatencyLow,
		latencyCustom:     700,
		showLatencySwitch: true,
		lampPreset:        "dune",
		standbyEnabled:    true,
		standbyMinutes:    30,
		reconnect:         true,
		flyoutEntered:     true,
		setThemeSource:    mygo.Theme.SetSource,
	}
	a.cfg.Name = locales.AppName()
	a.cfg.Language = previewLocale
	a.selID = "AA:BB:CC:DD:EE:01"
	devices := []device.Device{
		{ID: "AA:BB:CC:DD:EE:01", Name: "HomePods", Host: "192.168.1.21", Port: 7000, Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true, Online: true},
		{ID: "AA:BB:CC:DD:EE:02", Name: "Apple TV", Host: "192.168.1.22", Port: 7000, Model: "AppleTV6,2", Auth: device.AuthHAPPIN, AirPlay2: true, Online: true},
		{ID: "AA:BB:CC:DD:EE:03", Name: "HomePod mini", Host: "192.168.1.23", Port: 7000, Model: "AudioAccessory5,1", Auth: device.AuthHAPTransient, AirPlay2: true},
	}
	a.snap = engine.Snapshot{
		Devices: devices,
		Receivers: []receivers.View{
			{Key: devices[0].ID, Name: devices[0].Name, Kind: receivers.KindDiscovered, Model: devices[0].Model, Online: true, AutoConnect: true, Device: devices[0], Entry: receivers.Entry{Key: devices[0].ID, Kind: receivers.KindDiscovered, Name: devices[0].Name, Host: devices[0].Host, Port: devices[0].Port, Model: devices[0].Model, AirPlay2: true, AutoConnect: true}},
			{Key: devices[1].ID, Name: devices[1].Name, Kind: receivers.KindDiscovered, Model: devices[1].Model, Online: true, AutoConnect: true, DelayMS: 120, Device: devices[1], Entry: receivers.Entry{Key: devices[1].ID, Kind: receivers.KindDiscovered, Name: devices[1].Name, Host: devices[1].Host, Port: devices[1].Port, Model: devices[1].Model, AirPlay2: true, AutoConnect: true, DelayMS: 120}},
			{Key: devices[2].ID, Name: devices[2].Name, Kind: receivers.KindDiscovered, Model: devices[2].Model, Device: devices[2], Entry: receivers.Entry{Key: devices[2].ID, Kind: receivers.KindDiscovered, Name: devices[2].Name, Host: devices[2].Host, Port: devices[2].Port, Model: devices[2].Model, AirPlay2: true}},
		},
		SelectedID:        "AA:BB:CC:DD:EE:01",
		State:             sender.StateStreaming,
		ConnectedName:     "HomePods",
		Volume:            65,
		CaptureName:       capture,
		CaptureRate:       48000,
		CaptureRunning:    true,
		Name:              locales.AppName(),
		ReceiverLatencyMS: 70,
		PTPActive:         true,
		PTPReady:          true,
		Reconnect:         true,
		LampPreset:        "dune",
		StandbyEnabled:    true,
		StandbyMinutes:    30,
	}
	return a
}

// previewHeight is the flyout height for the current state, as the window
// and the render composition ask for it.
func previewHeight(a *App) int {
	return flyoutHeightForState(len(a.snap.Devices), a.showLatencySwitch, a.hasNowMedia(), a.lampCover, a.lampExpanded, a.errorBarShown(), a.snap.CredentialReset)
}

// previewReduce is the fake controller: it changes the state the views read.
// The engine replaces it in production; the transitions are the spec.
func previewReduce(a *App, name string, arg any) bool {
	switch name {
	case "refresh":
		return true
	case "settings":
		a.showPreviewSettings()
	case "connect":
		id, _ := arg.(string)
		if id == "" {
			return true
		}
		a.selID = id
		a.snap.SelectedID = id
		a.snap.State = sender.StateHandshake
		a.snap.ConnectedName = ""
		a.snap.Error = ""
	case "disconnect":
		a.snap.State = sender.StateIdle
		a.snap.ConnectedName = ""
		a.snap.SelectedID = ""
		a.snap.Error = ""
	case "volume":
		v, _ := arg.(float64)
		a.volume = v
		a.snap.Volume = v
	case "name":
		name, _ := arg.(string)
		a.cfg.Name = name
		a.snap.Name = name
	case "launch":
		on, _ := arg.(bool)
		a.launchAtStart = on
		a.cfg.LaunchAtStart = on
	case "reconnect":
		on, _ := arg.(bool)
		a.reconnect = on
		a.cfg.Reconnect = on
	case "strict":
		on, _ := arg.(bool)
		a.strict = on
		a.cfg.StrictReceiverAuth = on
	case "language":
		label, _ := arg.(string)
		switch label {
		case locales.LanguageEnglish():
			previewLocale = "en"
			a.cfg.Language = "en"
		case locales.LanguageChinese():
			previewLocale = "zh-CN"
			a.cfg.Language = "zh-CN"
		default:
			a.cfg.Language = ""
		}
	case "receiver-add":
		spec, ok := arg.(receivers.Spec)
		if !ok {
			return false
		}
		previewReceiverAdd(a, spec)
	case "receiver-update":
		u, ok := arg.(receiverUpdateArg)
		if !ok {
			return false
		}
		previewReceiverUpdate(a, u.key, u.patch)
	case "receiver-remove":
		key, ok := arg.(string)
		if !ok {
			return false
		}
		previewReceiverRemove(a, key)
	case "receiver-password":
		p, ok := arg.(receiverPasswordArg)
		if !ok {
			return false
		}
		previewReceiverPassword(a, p.key, p.password)
	case "receiver-hidden":
		b, ok := arg.(receiverBoolArg)
		if !ok {
			return false
		}
		previewReceiverHidden(a, b.key, b.on)
	case "receiver-auto":
		b, ok := arg.(receiverBoolArg)
		if !ok {
			return false
		}
		previewReceiverAuto(a, b.key, b.on)
	case "receiver-delay":
		d, ok := arg.(receiverDelayArg)
		if !ok {
			return false
		}
		previewReceiverDelay(a, d.key, d.ms)
	default:
		return false
	}
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
	return true
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

// previewKeys switches the state with 1, 2, and 3, so a human can compare
// the same catalog the render writes.
func (a *App) previewKeys(c *ui.Context) {
	for i := range PreviewStates {
		i := i
		c.OnShortcut(0, ui.Key1+ui.Key(i), func() {
			PreviewStates[i].Apply(a)
			previewResize(a)
			if a.win != nil {
				a.win.Invalidate()
			}
		})
	}
}

// RunPreview opens the flyout with the fake state in a browser or a native
// window, driven by previewReduce.
func RunPreview() error {
	previewAction = previewReduce
	mygo.App.WhenReady(func() {
		a := PreviewApp()
		PreviewStates[0].Apply(a)
		var win *mygo.Window
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:  "saundo preview",
			Width:  flyoutWidth,
			Height: previewHeight(a),
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
