package app

import (
	"embed"
	"fmt"
	"log"
)

// The tray shows the state face of the moment in the ink of the taskbar:
// state faces are the app's smile with a mouth that says what it is doing.
// At rest it is the plain smile, the app's own mark.
//
//go:embed assets/tray-*.png
var trayAssets embed.FS

type trayIconKey struct {
	face string
	dark bool
}

var trayPNGs = loadTrayPNGs()

func loadTrayPNGs() map[trayIconKey][]byte {
	m := make(map[trayIconKey][]byte, 10)
	for _, face := range []string{"stream", "connect", "error", "smile"} {
		for _, theme := range []string{"light", "dark"} {
			name := fmt.Sprintf("assets/tray-%s-%s-32.png", theme, face)
			png, err := trayAssets.ReadFile(name)
			if err != nil {
				panic(err)
			}
			m[trayIconKey{face: face, dark: theme == "dark"}] = png
		}
	}
	return m
}

// refreshTrayIcon swaps the tray's face when the stream state or the
// taskbar's appearance changes.
func (a *App) refreshTrayIcon() {
	if a.tray == nil {
		return
	}
	key := trayIconKey{face: a.face(), dark: a.tray.Dark()}
	if key == a.trayKey {
		return
	}
	a.trayKey = key
	if err := a.tray.SetIcon(trayPNGs[key], false); err != nil {
		log.Printf("tray icon: %v", err)
	}
}
