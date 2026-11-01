//go:build windows

package nowplaying

import (
	"os"
	"testing"
)

// TestSMTCInfoLive reads the real SMTC session. It passes when SMTC answers
// even with nothing playing; set SAUNDO_SMTC=1 to require a track title,
// which needs something actually playing (browser, player, etc.).
func TestSMTCInfoLive(t *testing.T) {
	if os.Getenv("SAUNDO_SMTC") == "" {
		t.Skip("set SAUNDO_SMTC=1 to require a live media session")
	}
	track, ok, err := Current()
	if err != nil {
		t.Fatalf("SMTC Current: %v", err)
	}
	if !ok || track.Title == "" {
		t.Skip("nothing is playing")
	}
	t.Logf("now playing: %q by %q (album %q)", track.Title, track.Artist, track.Album)
}
