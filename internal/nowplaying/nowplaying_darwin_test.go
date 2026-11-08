//go:build darwin

package nowplaying

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The fixture carries one real adapter stream line (metadata from a Music
// session) plus the empty-session line the adapter emits between sessions.
func fixtureLines(t *testing.T) [][]byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "darwin-stream.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var lines [][]byte
	start := 0
	for i, b := range raw {
		if b == '\n' {
			if i > start {
				lines = append(lines, raw[start:i])
			}
			start = i + 1
		}
	}
	if len(lines) != 3 {
		t.Fatalf("fixture lines = %d, want 3", len(lines))
	}
	return lines
}

func TestParseStreamLineMapsTheSession(t *testing.T) {
	lines := fixtureLines(t)

	if p, err := parseStreamLine(lines[0]); err != nil || p != nil {
		t.Fatalf("empty payload = %v, %v; want nil, nil", p, err)
	}

	p, err := parseStreamLine(lines[1])
	if err != nil || p == nil {
		t.Fatalf("session payload = %v, %v; want a payload", p, err)
	}
	if p.Title != "Janice STFU" || p.Artist != "Drake" || p.Album != "ICEMAN" {
		t.Fatalf("metadata = %q %q %q", p.Title, p.Artist, p.Album)
	}
	if !p.Playing || p.Duration != 237388000 || p.Elapsed != 12000000 {
		t.Fatalf("playback = %v duration=%v elapsed=%v", p.Playing, p.Duration, p.Elapsed)
	}
	if p.BundleID != "com.apple.Music" {
		t.Fatalf("bundle = %q", p.BundleID)
	}
	art, err := base64.StdEncoding.DecodeString(p.ArtworkData)
	if err != nil || len(art) == 0 || artworkMIME(art) != "image/jpeg" {
		t.Fatalf("artwork decode failed: %v (%d bytes, %q)", err, len(art), artworkMIME(art))
	}

	s := sessionFromPayload(p)
	if s.Status != StatusPlaying || !s.Playing || !s.CanNext || !s.CanSeek {
		t.Fatalf("session state = %v playing=%v", s.Status, s.Playing)
	}
	if s.Duration != 237388*time.Millisecond || s.Position != 12*time.Second {
		t.Fatalf("times = %v %v", s.Duration, s.Position)
	}
	if s.Updated.IsZero() || s.AUMID != "com.apple.Music" {
		t.Fatalf("updated=%v aumid=%q", s.Updated, s.AUMID)
	}

	// A paused payload keeps the session and flips the status.
	p2, err := parseStreamLine(lines[2])
	if err != nil || p2 == nil {
		t.Fatalf("paused payload = %v, %v; want a payload", p2, err)
	}
	s2 := sessionFromPayload(p2)
	if s2.Status != StatusPaused || s2.Playing {
		t.Fatalf("paused session = %v playing=%v", s2.Status, s2.Playing)
	}
}
