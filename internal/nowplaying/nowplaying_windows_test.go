//go:build windows

package nowplaying

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
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

// TestSMTCLiveSessions reads every SMTC session, including controls, timeline,
// and artwork bytes. It needs live media; set SAUNDO_SMTC=1 to run it.
func TestSMTCLiveSessions(t *testing.T) {
	if os.Getenv("SAUNDO_SMTC") == "" {
		t.Skip("set SAUNDO_SMTC=1 to require live media sessions")
	}
	artworkDebug = t.Logf
	defer func() { artworkDebug = nil }()
	snap, err := Read()
	if err != nil {
		t.Fatalf("SMTC Read: %v", err)
	}
	if len(snap.Sessions) == 0 {
		t.Skip("no media sessions")
	}
	t.Logf("sessions=%d current=%d readAt=%s", len(snap.Sessions), snap.Current, snap.ReadAt.Format(time.RFC3339))
	for i, s := range snap.Sessions {
		t.Logf("[%d] %s aumid=%q status=%s playing=%v title=%q subtitle=%q artist=%q albumArtist=%q album=%q can(play=%v pause=%v next=%v prev=%v seek=%v) pos=%s dur=%s updated=%s art=%d %s",
			i, s.AppName, s.AUMID, s.Status, s.Playing, s.Title, s.Subtitle, s.Artist, s.AlbumArtist, s.Album,
			s.CanPlay, s.CanPause, s.CanNext, s.CanPrev, s.CanSeek, s.Position, s.Duration, s.Updated.Format(time.RFC3339), len(s.Artwork), s.ArtworkMIME)
		if dir := os.Getenv("SAUNDO_SMTC_ARTWORK_DIR"); dir != "" && len(s.Artwork) > 0 {
			ext := ".bin"
			switch s.ArtworkMIME {
			case "image/png":
				ext = ".png"
			case "image/jpeg":
				ext = ".jpg"
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Logf("artwork dir: %v", err)
				continue
			}
			path := filepath.Join(dir, fmt.Sprintf("session-%d%s", i, ext))
			if err := os.WriteFile(path, s.Artwork, 0o644); err != nil {
				t.Logf("artwork write: %v", err)
			} else {
				t.Logf("artwork saved: %s", path)
			}
		}
	}
}
