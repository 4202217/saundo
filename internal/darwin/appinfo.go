//go:build darwin

package darwin

import (
	"errors"
	"sync"
)

// RunningApp is one entry of NSWorkspace's running applications. Policy is
// NSApplicationActivationPolicy: 0 regular, 1 accessory, 2 prohibited.
type RunningApp struct {
	PID      uint32
	Name     string
	BundleID string
	Path     string
	Policy   int
}

// RunningApplications lists the apps LaunchServices knows are running,
// including background helpers. The capture picker groups them into app
// families; callers on other threads still get a snapshot.
func RunningApplications() ([]RunningApp, error) {
	if err := load(); err != nil {
		return nil, err
	}
	var out []RunningApp
	var listErr error
	WithPool(func() {
		ws := Send(mustClass("NSWorkspace"), "sharedWorkspace")
		if ws == 0 {
			listErr = errors.New("darwin: NSWorkspace is unavailable")
			return
		}
		arr := Send(ws, "runningApplications")
		if arr == 0 {
			listErr = errors.New("darwin: runningApplications is unavailable")
			return
		}
		n := SendInt(arr, "count")
		out = make([]RunningApp, 0, n)
		for i := 0; i < n; i++ {
			app := Send(arr, "objectAtIndex:", uintptr(i))
			if app == 0 {
				continue
			}
			a := RunningApp{
				PID:      uint32(SendInt(app, "processIdentifier")),
				Name:     GoString(Send(app, "localizedName")),
				BundleID: GoString(Send(app, "bundleIdentifier")),
				Policy:   SendInt(app, "activationPolicy"),
			}
			if u := Send(app, "bundleURL"); u != 0 {
				a.Path = GoString(Send(u, "path"))
			}
			out = append(out, a)
		}
	})
	return out, listErr
}

// App name resolution for media sessions: LaunchServices maps a bundle
// identifier to the app, whose bundle carries the localized display name.

var (
	appNameMu    sync.Mutex
	appNameCache = map[string]string{}
)

// AppNameForBundleID resolves the localized display name of the app with
// the given bundle identifier — "Music", not "com.apple.Music". It returns
// "" when LaunchServices does not know the identifier; callers then show
// their own unknown-app placeholder, the way the Windows snapshot does.
func AppNameForBundleID(bundleID string) string {
	if bundleID == "" {
		return ""
	}
	appNameMu.Lock()
	defer appNameMu.Unlock()
	if n, ok := appNameCache[bundleID]; ok {
		return n
	}
	n := ""
	WithPool(func() {
		ws := Send(mustClass("NSWorkspace"), "sharedWorkspace")
		url := Send(ws, "URLForApplicationWithBundleIdentifier:", uintptr(NSString(bundleID)))
		if url == 0 {
			return
		}
		path := Send(url, "path")
		if path == 0 {
			return
		}
		// localizedName is absent on current NSBundle; the file manager's
		// display name is the localized name Finder shows.
		name := Send(Send(mustClass("NSFileManager"), "defaultManager"),
			"displayNameAtPath:", uintptr(NSString(GoString(path))))
		if name == 0 {
			return
		}
		n = GoString(name)
	})
	appNameCache[bundleID] = n
	return n
}

// AppIconForBundleID renders the app's icon as a 128 px PNG — the same
// shape the Windows snapshot carries in Session.AppIcon. It returns nil
// when LaunchServices does not know the identifier.
func AppIconForBundleID(bundleID string) []byte {
	if bundleID == "" {
		return nil
	}
	iconMu.Lock()
	png, ok := iconCache[bundleID]
	iconMu.Unlock()
	if ok {
		return png
	}
	var path string
	WithPool(func() {
		ws := Send(mustClass("NSWorkspace"), "sharedWorkspace")
		url := Send(ws, "URLForApplicationWithBundleIdentifier:", uintptr(NSString(bundleID)))
		if url == 0 {
			return
		}
		if p := Send(url, "path"); p != 0 {
			path = GoString(p)
		}
	})
	png = AppIconForPath(path)
	iconMu.Lock()
	iconCache[bundleID] = png
	iconMu.Unlock()
	return png
}

// AppIconForPath renders a file or app bundle's icon as a 128 px PNG. It
// returns nil when the path has no icon.
func AppIconForPath(path string) []byte {
	if path == "" {
		return nil
	}
	iconMu.Lock()
	defer iconMu.Unlock()
	if png, ok := iconCache[path]; ok {
		return png
	}
	var png []byte
	WithPool(func() {
		ws := Send(mustClass("NSWorkspace"), "sharedWorkspace")
		icon := Send(ws, "iconForFile:", uintptr(NSString(path)))
		if icon == 0 {
			return
		}
		png = pngThumbnail(GoBytes(Send(icon, "TIFFRepresentation")))
	})
	iconCache[path] = png
	return png
}

var (
	iconMu    sync.Mutex
	iconCache = map[string][]byte{}
)
