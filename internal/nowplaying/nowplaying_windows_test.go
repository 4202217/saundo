//go:build windows

package nowplaying

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppNameResolvesUnpackagedExecutable(t *testing.T) {
	tests := []struct{ aumid, want string }{
		{"MyGo Music.exe", "MyGo Music"},
		{`C:\Program Files\MyGo\MyGo Music.EXE`, "MyGo Music"},
		{`C:/Tools/vlc.exe`, "vlc"},
		{"Spotify", "Spotify"},
		{"Microsoft.ZuneMusic_8wekyb3d8bbwe!Microsoft.ZuneMusic", "Media Player"},
		{"Some.Package_abc!App", ""},
		{".exe", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := appName(tt.aumid); got != tt.want {
			t.Fatalf("appName(%q) = %q, want %q", tt.aumid, got, tt.want)
		}
	}
}

func TestMatchInstalledApp(t *testing.T) {
	apps := []shellApp{
		{name: "Google Chrome", id: "Chrome", path: `shell:AppsFolder\Chrome`},
		{name: "MyGo Music", id: `C:\Apps\MyGo Music.exe`, path: `shell:AppsFolder\C:\Apps\MyGo Music.exe`},
		{name: "Movie Player", id: "Microsoft.ZuneMusic_abc!Microsoft.ZuneMusic"},
	}
	tests := []struct{ aumid, want string }{
		{"Chrome.UserData.Profile1", "Google Chrome"},
		{"MyGo Music.exe", "MyGo Music"},
		{"Microsoft.ZuneMusic_abc!Microsoft.ZuneMusic", "Movie Player"},
		{"Unknown.App!Viewer", ""},
	}
	for _, tt := range tests {
		app, ok := matchInstalledApp(tt.aumid, apps)
		if tt.want == "" {
			if ok {
				t.Fatalf("matchInstalledApp(%q) = %q, want no match", tt.aumid, app.name)
			}
			continue
		}
		if !ok || app.name != tt.want {
			t.Fatalf("matchInstalledApp(%q) = %q ok=%v, want %q", tt.aumid, app.name, ok, tt.want)
		}
	}
}

func TestExeFileAndStem(t *testing.T) {
	tests := []struct{ aumid, file, stem string }{
		{`C:\Tools\MyGo Music.EXE`, "MyGo Music.EXE", "MyGo Music"},
		{"vlc.exe", "vlc.exe", "vlc"},
		{"Chrome.UserData.Profile1", "", ""},
		{".exe", "", ""},
	}
	for _, tt := range tests {
		if got := exeFile(tt.aumid); got != tt.file {
			t.Fatalf("exeFile(%q) = %q, want %q", tt.aumid, got, tt.file)
		}
		if got := exeStem(tt.aumid); got != tt.stem {
			t.Fatalf("exeStem(%q) = %q, want %q", tt.aumid, got, tt.stem)
		}
	}
}

func TestAppCandidates(t *testing.T) {
	tests := []struct {
		aumid string
		want  []string
	}{
		{"Chrome.UserData.Profile1", []string{"Chrome.UserData.Profile1", "Chrome"}},
		{"MyGo Music.exe", []string{"MyGo Music.exe", "MyGo Music"}},
		{"Some.Package_abc!App", []string{"Some.Package_abc!App", "Some"}},
		{"Chrome", []string{"Chrome"}},
	}
	for _, tt := range tests {
		got := appCandidates(tt.aumid)
		if len(got) != len(tt.want) {
			t.Fatalf("appCandidates(%q) = %v, want %v", tt.aumid, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("appCandidates(%q) = %v, want %v", tt.aumid, got, tt.want)
			}
		}
	}
}

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
		t.Logf("[%d] %s icon=%d aumid=%q status=%s playing=%v title=%q subtitle=%q artist=%q albumArtist=%q album=%q can(play=%v pause=%v next=%v prev=%v seek=%v) pos=%s dur=%s updated=%s art=%d %s",
			i, s.AppName, len(s.AppIcon), s.AUMID, s.Status, s.Playing, s.Title, s.Subtitle, s.Artist, s.AlbumArtist, s.Album,
			s.CanPlay, s.CanPause, s.CanNext, s.CanPrev, s.CanSeek, s.Position, s.Duration, s.Updated.Format(time.RFC3339), len(s.Artwork), s.ArtworkMIME)
		if dir := os.Getenv("SAUNDO_SMTC_ARTWORK_DIR"); dir != "" && len(s.AppIcon) > 0 {
			if err := os.MkdirAll(dir, 0o755); err == nil {
				path := filepath.Join(dir, fmt.Sprintf("session-%d-icon.png", i))
				if err := os.WriteFile(path, s.AppIcon, 0o644); err == nil {
					t.Logf("icon saved: %s", path)
				}
			}
		}
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
