package app

import (
	"testing"

	"saundo/internal/engine"
)

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
