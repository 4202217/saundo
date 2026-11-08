//go:build darwin

package darwin

import "sync"

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
	defer iconMu.Unlock()
	if png, ok := iconCache[bundleID]; ok {
		return png
	}
	var png []byte
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
		icon := Send(ws, "iconForFile:", uintptr(Send(url, "path")))
		if icon == 0 {
			return
		}
		png = pngThumbnail(GoBytes(Send(icon, "TIFFRepresentation")))
	})
	iconCache[bundleID] = png
	return png
}

var (
	iconMu    sync.Mutex
	iconCache = map[string][]byte{}
)
