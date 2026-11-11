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
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// adapter holds one streaming adapter child and its latest payload. mu
// guards payload, cmd and exited; ensure and the reader both touch them.
type adapter struct {
	mu             sync.Mutex
	payload        *streamPayload
	cmd            *exec.Cmd
	exited         time.Time
	functionalTest error
	functionalNext time.Time
	functionalBack time.Duration
	functionalOK   bool
	// probe and now are injectable for the darwin tests.
	probe func() error
	now   func() time.Time
}

const (
	functionalRetryMin = 30 * time.Second
	functionalRetryMax = 5 * time.Minute
	// The adapter's own test can spend 3 s waiting for its helper plus two
	// 2 s gets; 5 s cut a healthy slow probe short.
	functionalProbeTimeout = 10 * time.Second
)

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

// functional checks the adapter's entitlement self-test. A failed probe is
// retried on a 30 s to 5 min backoff rather than cached for the process: a
// launch-time failure, a slow first probe, or a transient MediaRemote state
// must not disable Now Playing until restart.
func (a *adapter) functional() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if a.now != nil {
		now = a.now()
	}
	if a.functionalOK {
		return nil
	}
	if !a.functionalNext.IsZero() && now.Before(a.functionalNext) {
		return a.functionalTest
	}
	probe := a.probe
	if probe == nil {
		probe = a.probeAdapter
	}
	err := probe()
	if err == nil {
		a.functionalOK = true
		a.functionalTest = nil
		a.functionalNext = time.Time{}
		a.functionalBack = 0
		return nil
	}
	a.functionalTest = err
	if a.functionalBack < functionalRetryMin {
		a.functionalBack = functionalRetryMin
	} else {
		a.functionalBack *= 2
		if a.functionalBack > functionalRetryMax {
			a.functionalBack = functionalRetryMax
		}
	}
	a.functionalNext = now.Add(a.functionalBack)
	return err
}

// adapterTestArgs builds the perl loader command for the self-test. The
// loader reads TEST_CLIENT_PATH only when the argument after the framework
// contains a slash, so the staged client goes before the "test" function.
func adapterTestArgs(dir, framework string) []string {
	args := []string{filepath.Join(dir, "mediaremote-adapter.pl"), framework}
	client := filepath.Join(dir, "MediaRemoteAdapterTestClient")
	if _, err := os.Stat(client); err == nil {
		args = append(args, client)
	}
	return append(args, "test")
}

// probeAdapter runs the vendored adapter's entitlement self-test.
func (a *adapter) probeAdapter() error {
	dir, framework, err := adapterPaths()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), functionalProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/perl", adapterTestArgs(dir, framework)...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nowplaying: MediaRemote adapter is not functional here: %w", err)
	}
	return nil
}

// ensure starts the stream child if none is running and a respawn is due.
func (a *adapter) ensure() {
	a.mu.Lock()
	defer a.mu.Unlock()
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
	go a.read(cmd, stdout)
}

// read drains the child's stdout. A data line replaces the payload; a
// non-data line leaves the current session alone. A malformed line is
// dropped and the stream goes on.
func (a *adapter) read(cmd *exec.Cmd, stdout io.ReadCloser) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 4<<20), 32<<20) // artwork payloads get large
	for scanner.Scan() {
		p, isData, err := parseStreamLine(scanner.Bytes())
		if err != nil || !isData {
			continue
		}
		a.mu.Lock()
		a.payload = p
		a.mu.Unlock()
	}
	_ = cmd.Wait()
	a.mu.Lock()
	a.cmd = nil
	a.exited = time.Now()
	a.mu.Unlock()
}

// parseStreamLine decodes one adapter stdout line. isData reports whether
// the line carried session state at all; a data event with a nil payload
// means "no session right now", which the adapter emits between sessions.
func parseStreamLine(line []byte) (p *streamPayload, isData bool, err error) {
	var ev streamEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil, false, err
	}
	if ev.Type != "data" {
		return nil, false, nil
	}
	var payload streamPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return nil, true, err
	}
	if payload.Title == "" && !payload.Playing && payload.Duration == 0 {
		return nil, true, nil // an empty payload is "no session"
	}
	return &payload, true, nil
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
	// entries. A hung MediaRemote call must not hang the UI.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/perl", filepath.Join(dir, "mediaremote-adapter.pl"), framework, fn, param)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nowplaying: adapter %s %s failed: %v: %s", fn, param, err, out)
	}
	return nil
}
