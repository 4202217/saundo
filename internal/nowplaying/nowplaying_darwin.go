//go:build darwin

package nowplaying

// macOS reads the system Now Playing session through the vendored
// MediaRemote adapter (native/mediaremote-adapter, BSD-3). MediaRemote
// itself returns empty data to third-party processes since macOS 15.4, so
// the adapter loads its helper framework inside the Apple-signed
// /usr/bin/perl — a platform binary, which still passes MediaRemote's
// client check — and streams one JSON object per update on stdout. See
// docs/platforms.md and NOTICE.

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"saundo/internal/darwin"
)

// MRCommand ids from the adapter's private MediaRemote.h.
const (
	mrPlay          = "0"
	mrPause         = "1"
	mrNextTrack     = "4"
	mrPreviousTrack = "5"
)

// streamEvent is one adapter stdout line.
type streamEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// streamPayload is the metadata of one now playing session. Time keys are
// microseconds (the adapter's --micros option).
type streamPayload struct {
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	Album       string  `json:"album"`
	AlbumArtist string  `json:"albumArtist"`
	Playing     bool    `json:"playing"`
	Duration    float64 `json:"durationMicros"`
	Elapsed     float64 `json:"elapsedTimeMicros"`
	Timestamp   int64   `json:"timestampEpochMicros"`
	ArtworkData string  `json:"artworkData"`
	BundleID    string  `json:"bundleIdentifier"`
}

// Track is one media session's track metadata.
type Track struct {
	Title  string
	Artist string
	Album  string
}

// adapter holds one streaming adapter child and its latest payload.
type adapter struct {
	mu             sync.Mutex
	payload        *streamPayload
	updatedAt      time.Time
	cmd            *exec.Cmd
	exited         time.Time
	functionalTest error
	functionalOnce sync.Once
}

var shared = &adapter{}

// adapterPaths resolves the vendored adapter: the directory holding
// mediaremote-adapter.pl and the MediaRemoteAdapter.framework. The bundled
// layout carries both in the app's resources; a dev run falls back to the
// vendored source tree with its CMake build output. A bare executable still
// gets a main bundle from Foundation whose resource path is useless, so
// every candidate is verified before use.
func adapterPaths() (dir, framework string, err error) {
	var candidates [][2]string
	if res := darwin.BundleResourcePath(); res != "" {
		// The bundled layout mirrors the dev tree: bin holds the loader
		// script, build the CMake output.
		candidates = append(candidates, [2]string{
			filepath.Join(res, "adapter", "bin"),
			filepath.Join(res, "adapter", "MediaRemoteAdapter.framework"),
		})
	}
	if exe, exeErr := os.Executable(); exeErr == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, [2]string{
			filepath.Join(exeDir, "mediaremote-adapter"),
			filepath.Join(exeDir, "mediaremote-adapter", "MediaRemoteAdapter.framework"),
		})
	}
	candidates = append(candidates, [2]string{
		"native/mediaremote-adapter/bin",
		"native/mediaremote-adapter/build/MediaRemoteAdapter.framework",
	})
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c[0], "mediaremote-adapter.pl")); err != nil {
			continue
		}
		if _, err := os.Stat(c[1]); err != nil {
			continue
		}
		// The perl loader rejects relative framework paths.
		absDir, err := filepath.Abs(c[0])
		if err != nil {
			continue
		}
		absFramework, err := filepath.Abs(c[1])
		if err != nil {
			continue
		}
		return absDir, absFramework, nil
	}
	return "", "", errors.New("nowplaying: the MediaRemote adapter is not installed")
}

// functional checks the adapter's entitlement self-test once per process.
// A non-zero exit means MediaRemote rejects even the platform-binary trick
// on this system, and every read reports unsupported.
func (a *adapter) functional() error {
	a.functionalOnce.Do(func() {
		dir, framework, err := adapterPaths()
		if err != nil {
			a.functionalTest = err
			return
		}
		cmd := exec.Command("/usr/bin/perl", filepath.Join(dir, "mediaremote-adapter.pl"), framework, "test")
		if err := cmd.Run(); err != nil {
			a.functionalTest = fmt.Errorf("nowplaying: MediaRemote adapter is not functional here: %w", err)
		}
	})
	return a.functionalTest
}

// ensure starts the stream child if none is running and a respawn is due.
func (a *adapter) ensure() {
	if a.cmd != nil {
		return
	}
	if !a.exited.IsZero() && time.Since(a.exited) < 5*time.Second {
		return // a child that just died gets a short backoff
	}
	dir, framework, err := adapterPaths()
	if err != nil {
		return
	}
	cmd := exec.Command("/usr/bin/perl", filepath.Join(dir, "mediaremote-adapter.pl"), framework,
		"stream", "--no-diff", "--micros", "--debounce=200")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}
	a.cmd = cmd
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 4<<20), 32<<20) // artwork payloads get large
		for scanner.Scan() {
			p, err := parseStreamLine(scanner.Bytes())
			if err != nil {
				continue // a malformed line is dropped, the stream goes on
			}
			a.mu.Lock()
			a.payload = p
			a.updatedAt = time.Now()
			a.mu.Unlock()
		}
		_ = cmd.Wait()
		a.mu.Lock()
		a.cmd = nil
		a.exited = time.Now()
		a.mu.Unlock()
	}()
}

// parseStreamLine decodes one adapter stdout line into a payload. A nil
// payload (with a nil error) means "no session right now"; the adapter
// emits an empty payload object between sessions.
func parseStreamLine(line []byte) (*streamPayload, error) {
	var ev streamEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil, err
	}
	if ev.Type != "data" {
		return nil, nil
	}
	var p streamPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return nil, err
	}
	if p.Title == "" && !p.Playing && p.Duration == 0 {
		return nil, nil // an empty payload is "no session"
	}
	return &p, nil
}

// latest returns the newest payload, or nil when nothing is playing.
func (a *adapter) latest() *streamPayload {
	a.ensure()
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.payload
}

func artworkMIME(data []byte) string {
	switch {
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "image/jpeg"
	case len(data) >= 4 && string(data[:4]) == "\x89PNG":
		return "image/png"
	}
	return ""
}

func sessionFromPayload(p *streamPayload) Session {
	s := Session{
		Title:       p.Title,
		Artist:      p.Artist,
		AlbumArtist: p.AlbumArtist,
		Album:       p.Album,
		AUMID:       p.BundleID,
		AppName:     darwin.AppNameForBundleID(p.BundleID),
		AppIcon:     darwin.AppIconForBundleID(p.BundleID),
		Playing:     p.Playing,
		Status:      StatusPaused,
		CanPlay:     true,
		CanPause:    true,
		CanNext:     true,
		CanPrev:     true,
		CanSeek:     true,
		Duration:    time.Duration(p.Duration) * time.Microsecond,
		Position:    time.Duration(p.Elapsed) * time.Microsecond,
	}
	if p.Playing {
		s.Status = StatusPlaying
	}
	if p.Timestamp != 0 {
		s.Updated = time.UnixMicro(p.Timestamp)
	}
	if p.ArtworkData != "" {
		if raw, err := base64.StdEncoding.DecodeString(p.ArtworkData); err == nil && len(raw) > 0 {
			s.Artwork = raw
			s.ArtworkMIME = artworkMIME(raw)
		}
	}
	return s
}

// Read returns the system Now Playing session, or an empty snapshot when
// nothing is playing. An error means the adapter is missing or rejected on
// this system, not that the user paused.
func Read() (Snapshot, error) {
	if err := shared.functional(); err != nil {
		return Snapshot{}, err
	}
	p := shared.latest()
	if p == nil {
		return Snapshot{ReadAt: time.Now()}, nil
	}
	session := sessionFromPayload(p)
	return Snapshot{
		Sessions: []Session{session},
		Current:  0,
		ReadAt:   time.Now(),
	}, nil
}

// Current returns the shared session's track, or false when nothing plays.
func Current() (Track, bool, error) {
	snap, err := Read()
	if err != nil {
		return Track{}, false, err
	}
	if snap.Current < 0 || snap.Current >= len(snap.Sessions) {
		return Track{}, false, nil
	}
	s := snap.Sessions[snap.Current]
	return Track{Title: s.Title, Artist: s.Artist, Album: s.Album}, s.Playing, nil
}

// Control sends a transport command to the playing application through the
// adapter's send/seek functions. The session id is ignored: macOS exposes
// one system session.
func Control(aumid string, action Action, position time.Duration) error {
	if err := shared.functional(); err != nil {
		return err
	}
	dir, framework, err := adapterPaths()
	if err != nil {
		return err
	}
	var fn, param string
	switch action {
	case ActionPlay:
		fn, param = "send", mrPlay
	case ActionPause:
		fn, param = "send", mrPause
	case ActionNext:
		fn, param = "send", mrNextTrack
	case ActionPrevious:
		fn, param = "send", mrPreviousTrack
	case ActionSeek:
		fn, param = "seek", fmt.Sprintf("%d", position.Microseconds())
	default:
		return fmt.Errorf("nowplaying: unsupported action %d", int(action))
	}
	// The adapter takes the function and its parameter as separate argv
	// entries.
	cmd := exec.Command("/usr/bin/perl", filepath.Join(dir, "mediaremote-adapter.pl"), framework, fn, param)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nowplaying: adapter %s %s failed: %v: %s", fn, param, err, out)
	}
	return nil
}
