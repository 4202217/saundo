// Package settings persists the application's JSON configuration.
package settings

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Settings is the user's configuration.
type Settings struct {
	Name                string             `json:"name"`
	DeviceID            string             `json:"deviceId"`
	Volume              float64            `json:"volume"`
	VolumeByDevice      map[string]float64 `json:"volumeByDevice"`
	Passwords           map[string]string  `json:"passwords"`
	LatencyMS           int                `json:"latencyMs"`
	CaptureEndpointID   string             `json:"captureEndpointId"`
	CaptureEndpointName string             `json:"captureEndpointName"`
	CaptureProcessPID   uint32             `json:"captureProcessPid"`
	CaptureProcessName  string             `json:"captureProcessName"`
	LastDeviceID        string             `json:"lastDeviceId"`
	StrictReceiverAuth  bool               `json:"strictReceiverAuth"`
	AutoConnect         bool               `json:"autoConnect"`
	Reconnect           bool               `json:"reconnect"`
	ShowNowPlaying      bool               `json:"showNowPlaying"`
	LaunchAtStart       bool               `json:"launchAtStart"`
	Language            string             `json:"language"`
	Beta                bool               `json:"beta"`
	AutoUpdate          bool               `json:"autoUpdate"`
	AutoDownload        bool               `json:"autoDownload"`
	SkippedVersion      string             `json:"skippedVersion"`
	LastCheck           string             `json:"lastCheck"`
}

// Default returns the first-run configuration.
func Default() Settings {
	return Settings{
		Name:           "声渡",
		DeviceID:       newDeviceID(),
		Volume:         80,
		AutoUpdate:     true,
		VolumeByDevice: make(map[string]float64),
		Passwords:      make(map[string]string),
		LatencyMS:      200,
		Reconnect:      true,
	}
}

// Store reads and writes settings.json.
type Store struct {
	mu   sync.Mutex
	path string
}

// NewStore returns the per-user settings store.
func NewStore() *Store {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return &Store{path: filepath.Join(dir, "saundo", "settings.json")}
}

// NewStoreAt is the testable form of NewStore.
func NewStoreAt(path string) *Store { return &Store{path: path} }

// Load returns the stored settings, filling defaults for missing fields. A
// missing or corrupt file yields the defaults.
func (s *Store) Load() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Default()
	data, err := os.ReadFile(s.path)
	if err != nil {
		return out
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return Default()
	}
	if out.Name == "" {
		out.Name = "声渡"
	}
	if out.DeviceID == "" {
		out.DeviceID = newDeviceID()
	}
	if out.Volume < 0 || out.Volume > 100 {
		out.Volume = 80
	}
	if out.VolumeByDevice == nil {
		out.VolumeByDevice = make(map[string]float64)
	}
	if out.Passwords == nil {
		out.Passwords = make(map[string]string)
	}
	if out.LatencyMS < 20 || out.LatencyMS > 2000 {
		out.LatencyMS = 200
	}
	return out
}

// Save writes the settings atomically.
func (s *Store) Save(v Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.DeviceID == "" {
		v.DeviceID = newDeviceID()
	}
	if v.Name == "" {
		v.Name = "声渡"
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newDeviceID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "02:00:00:00:00:01"
	}
	b[0] = (b[0] & 0xFE) | 0x02 // locally administered, unicast
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", b[0], b[1], b[2], b[3], b[4], b[5])
}
