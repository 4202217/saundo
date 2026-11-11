//go:build darwin

package nowplaying

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestFunctionalRetriesAfterTransientFailure pins the retry gate: a failed
// probe is returned for the backoff window, then retried, and success is
// cached. The old sync.Once returned the first error for the whole process.
func TestFunctionalRetriesAfterTransientFailure(t *testing.T) {
	cases := []struct {
		name string
		fail error
	}{
		{"deadline", context.DeadlineExceeded},
		{"exit 1", errors.New("exit status 1")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := &adapter{}
			now := time.Unix(0, 0)
			a.now = func() time.Time { return now }
			calls := 0
			a.probe = func() error {
				calls++
				if calls == 1 {
					return c.fail
				}
				return nil
			}

			if err := a.functional(); !errors.Is(err, c.fail) {
				t.Fatalf("first error = %v, want %v", err, c.fail)
			}
			if err := a.functional(); err == nil {
				t.Fatal("the cached error was not returned inside the backoff")
			}
			if calls != 1 {
				t.Fatalf("probes inside the backoff = %d, want 1", calls)
			}

			now = now.Add(functionalRetryMin + time.Second)
			if err := a.functional(); err != nil {
				t.Fatalf("retry error = %v, want success", err)
			}
			if calls != 2 {
				t.Fatalf("probes = %d, want 2", calls)
			}
			if err := a.functional(); err != nil {
				t.Fatalf("cached success error = %v", err)
			}
			if calls != 2 {
				t.Fatalf("probes after success = %d, want 2", calls)
			}
		})
	}
}

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

	if p, isData, err := parseStreamLine(lines[0]); err != nil || !isData || p != nil {
		t.Fatalf("empty payload = %v, %v, %v; want nil, true, nil", p, isData, err)
	}

	p, isData, err := parseStreamLine(lines[1])
	if err != nil || !isData || p == nil {
		t.Fatalf("session payload = %v, %v, %v; want a data payload", p, isData, err)
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
	p2, isData, err := parseStreamLine(lines[2])
	if err != nil || !isData || p2 == nil {
		t.Fatalf("paused payload = %v, %v, %v; want a data payload", p2, isData, err)
	}
	s2 := sessionFromPayload(p2)
	if s2.Status != StatusPaused || s2.Playing {
		t.Fatalf("paused session = %v playing=%v", s2.Status, s2.Playing)
	}
}

// TestParseStreamLineIgnoresNonDataEvents keeps a status or error line from
// clearing the current session.
func TestParseStreamLineIgnoresNonDataEvents(t *testing.T) {
	p, isData, err := parseStreamLine([]byte(`{"type":"error","payload":{}}`))
	if err != nil || isData || p != nil {
		t.Fatalf("non-data = %v, %v, %v; want nil, false, nil", p, isData, err)
	}
}
