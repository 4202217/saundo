package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo"
)

const (
	trayAnimFrames   = 12
	trayAnimInterval = 100 * time.Millisecond
)

type trayAnimKey struct {
	face  string
	dark  bool
	frame int
}

var trayAnimPNGs = loadTrayAnimPNGs()

func loadTrayAnimPNGs() map[trayAnimKey][]byte {
	m := make(map[trayAnimKey][]byte, trayAnimFrames*4)
	for _, face := range []string{"stream", "connect"} {
		for _, theme := range []string{"light", "dark"} {
			for frame := 0; frame < trayAnimFrames; frame++ {
				name := fmt.Sprintf("assets/tray-%s-%s-%02d-32.png", theme, face, frame)
				png, err := trayAssets.ReadFile(name)
				if err != nil {
					panic(err)
				}
				m[trayAnimKey{face: face, dark: theme == "dark", frame: frame}] = png
			}
		}
	}
	return m
}

type trayAnimator struct {
	stop chan struct{}

	face      string
	frame     int
	frameKey  trayAnimKey
	frameOK   bool
	staticKey trayIconKey
	staticOK  bool
}

// syncTrayAnimator starts or stops the tray animation from the current
// preference.
func (a *App) syncTrayAnimator() {
	if a.tray == nil {
		return
	}
	if !a.trayAnimation {
		a.stopTrayAnimator()
		return
	}
	if a.trayAnim != nil {
		return
	}
	a.trayAnim = &trayAnimator{stop: make(chan struct{})}
	go a.trayAnim.run(a)
}

func (a *App) stopTrayAnimator() {
	if a.trayAnim == nil {
		return
	}
	close(a.trayAnim.stop)
	a.trayAnim = nil
	a.trayKey = trayIconKey{}
	a.refreshTrayIcon()
}

// trayAnimatedFace reports whether the tray face has frames to animate.
func trayAnimatedFace(face string) bool {
	return face == "stream" || face == "connect"
}

func (an *trayAnimator) run(a *App) {
	ticker := time.NewTicker(trayAnimInterval)
	defer ticker.Stop()
	for {
		select {
		case <-an.stop:
			return
		case <-a.quit:
			return
		case <-ticker.C:
		}
		mygo.RunOnMain(func() {
			if a.trayAnim != an {
				return
			}
			face := a.face()
			if face != an.face {
				an.face = face
				an.frame = 0
			}
			if !a.trayAnimation || !trayAnimatedFace(face) || mygo.Theme.Preferences().ReduceMotion {
				an.showStatic(a, face)
				return
			}
			an.showFrame(a, face)
		})
	}
}

func (an *trayAnimator) showFrame(a *App, face string) {
	key := trayAnimKey{face: face, dark: a.tray.Dark(), frame: an.frame}
	if !an.frameOK || key != an.frameKey {
		an.frameKey, an.frameOK = key, true
		an.staticOK = false
		if err := a.tray.SetIcon(trayAnimPNGs[key], false); err != nil {
			a.log.Printf("tray animation: %v", err)
		}
	}
	an.frame = (an.frame + 1) % trayAnimFrames
}

func (an *trayAnimator) showStatic(a *App, face string) {
	key := trayIconKey{face: face, dark: a.tray.Dark()}
	if an.staticOK && an.staticKey == key {
		return
	}
	an.staticKey, an.staticOK = key, true
	an.frameOK = false
	if err := a.tray.SetIcon(trayPNGs[key], false); err != nil {
		a.log.Printf("tray animation: %v", err)
	}
}
