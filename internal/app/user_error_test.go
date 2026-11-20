package app

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/sender"
	"saundo/internal/engine"
)

// TestClockConflictCopyFitsTheErrorBar pins the shortened copy and what it
// must say. The bar lays out 376 DIP at 11.5 px, so a longer sentence
// ellipsizes its remedy. The message now names the one refused mix: a stereo
// pair beside standalone PTP receivers.
func TestClockConflictCopyFitsTheErrorBar(t *testing.T) {
	budget := map[string]int{"en": 60, "zh-CN": 32}
	wants := map[string][]string{
		"en":    {"stereo pair", "PTP"},
		"zh-CN": {"立体声对", "PTP"},
	}
	for _, locale := range []string{"en", "zh-CN"} {
		withLocale(t, locale)
		got := locales.ErrorClockConflict()
		if n := utf8.RuneCountInString(got); n > budget[locale] {
			t.Errorf("%s: %q is %d runes, over the %d-rune one-line budget", locale, got, n, budget[locale])
		}
		for _, want := range wants[locale] {
			if !strings.Contains(got, want) {
				t.Errorf("%s: %q does not name %q", locale, got, want)
			}
		}
	}
}

// TestErrorBarShowsOnlyConnectionFailuresWhileStreaming pins the narrowed
// predicate: a healthy stream shows the bar for a refused connect or add, but
// not for a save or latency error that belongs to another surface.
func TestErrorBarShowsOnlyConnectionFailuresWhileStreaming(t *testing.T) {
	cases := []struct {
		name string
		key  engine.UserErrorKey
		want bool
	}{
		{"latency offline", engine.ErrLatencyOffline, false},
		{"save receiver", engine.ErrSaveReceiver, false},
		{"save password", engine.ErrSavePassword, false},
		{"clock conflict", engine.ErrClockConflict, true},
		{"device missing", engine.ErrDeviceMissing, true},
		{"local receiver", engine.ErrLocalReceiver, true},
		{"offline stereo pair", engine.ErrStereoMembersOffline, true},
		{"fairplay", engine.ErrFairPlay, true},
		{"moved endpoint", engine.ErrEndpointMoved, true},
		{"session open", engine.ErrSessionOpen, true},
	}
	for _, tc := range cases {
		a := previewApp()
		a.snap.State = sender.StateStreaming
		a.snap.Error = string(tc.key)
		a.snap.ErrorKey = tc.key
		if got := a.errorBarShown(); got != tc.want {
			t.Errorf("%s: errorBarShown = %v, want %v", tc.name, got, tc.want)
		}
	}

	// The error phase still shows any failure, keyed or raw.
	a := previewApp()
	a.snap.State = sender.StateIdle
	a.snap.Error = "raw connection failure"
	if !a.errorBarShown() {
		t.Fatal("the error phase hid a keyless failure")
	}

	// A takeover is the error phase with no message; it must not draw an
	// empty bar.
	a = previewApp()
	a.snap.State = sender.StateIdle
	a.snap.TakenOver = true
	if a.errorBarShown() {
		t.Fatal("a takeover with no message drew an empty error bar")
	}
}

// TestConnectionErrorKeysAreCatalogKeys keeps the bar's key list honest: every
// key it admits resolves to catalog copy.
func TestConnectionErrorKeysAreCatalogKeys(t *testing.T) {
	for _, key := range []engine.UserErrorKey{
		engine.ErrClockConflict, engine.ErrDeviceMissing, engine.ErrLocalReceiver,
		engine.ErrStereoMembersOffline, engine.ErrComboInvalid, engine.ErrFairPlay,
		engine.ErrEndpointMoved, engine.ErrSessionOpen, engine.ErrPasswordRejected,
	} {
		if !connectionErrorKey(key) {
			t.Errorf("%q is not in the connection key list", key)
		}
		if !slices.Contains(engine.UserErrorKeys, key) {
			t.Errorf("%q is not an engine user error key", key)
		}
	}
}

// TestEveryEngineErrorKeyHasACatalogMessage keeps the engine's key list and
// the app's resolver in step. A new key without a catalog message would show
// its internal name in the flyout.
func TestEveryEngineErrorKeyHasACatalogMessage(t *testing.T) {
	for _, locale := range []string{"en", "zh-CN"} {
		withLocale(t, locale)
		for _, key := range engine.UserErrorKeys {
			got := userErrorText(key, []string{"device", "1"}, "fallback")
			if got == "" || got == "fallback" {
				t.Errorf("%s: %q has no catalog message", locale, key)
			}
		}
	}
}
