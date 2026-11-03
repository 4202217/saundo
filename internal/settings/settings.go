// Package settings persists the application's JSON configuration.
package settings

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// LatencyMode is the named latency preset the user picked.
type LatencyMode string

const (
	LatencyLow      LatencyMode = "low"
	LatencyStandard LatencyMode = "standard"
	LatencyStable   LatencyMode = "stable"
	LatencyCustom   LatencyMode = "custom"
)

const (
	LatencyLowMS           = 200
	LatencyStandardMS      = 1500
	LatencyStableMS        = 2000
	DefaultLatencyCustomMS = 700
)

// Settings is the user's configuration.
type Settings struct {
	Name                string             `json:"name"`
	DeviceID            string             `json:"deviceId"`
	Volume              float64            `json:"volume"`
	VolumeByDevice      map[string]float64 `json:"volumeByDevice"`
	Passwords           map[string]string  `json:"passwords"`
	LatencyMS           int                `json:"latencyMs"`
	LatencyMode         LatencyMode        `json:"latencyMode"`
	LatencyCustomMS     int                `json:"latencyCustomMs"`
	ShowLatencySwitch   bool               `json:"showLatencySwitch"`
	CaptureEndpointID   string             `json:"captureEndpointId"`
	CaptureEndpointName string             `json:"captureEndpointName"`
	CaptureProcessPID   uint32             `json:"captureProcessPid"`
	CaptureProcessName  string             `json:"captureProcessName"`
	LastDeviceID        string             `json:"lastDeviceId"`
	StrictReceiverAuth  bool               `json:"strictReceiverAuth"`
	AutoConnect         bool               `json:"autoConnect"`
	Reconnect           bool               `json:"reconnect"`
	ShowNowPlaying      bool               `json:"showNowPlaying"`
	TrayAnimation       bool               `json:"trayAnimation"`
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
		Name:              "声渡",
		DeviceID:          newDeviceID(),
		Volume:            80,
		AutoUpdate:        true,
		VolumeByDevice:    make(map[string]float64),
		Passwords:         make(map[string]string),
		LatencyMS:         LatencyLowMS,
		LatencyMode:       LatencyLow,
		LatencyCustomMS:   DefaultLatencyCustomMS,
		ShowLatencySwitch: true,
		Reconnect:         true,
		TrayAnimation:     true,
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
	// A missing latencyMode must stay distinguishable from the default, so a
	// legacy file's latencyMs can choose the mode.
	out.LatencyMode = ""
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
		out.LatencyMS = LatencyLowMS
	}
	if out.LatencyCustomMS < 20 || out.LatencyCustomMS > 2000 {
		out.LatencyCustomMS = DefaultLatencyCustomMS
	}
	switch out.LatencyMode {
	case LatencyLow, LatencyStandard, LatencyStable:
		out.LatencyMS = LatencyModeValue(out.LatencyMode, out.LatencyCustomMS)
	case LatencyCustom:
		out.LatencyCustomMS = out.LatencyMS
	default:
		out.LatencyMode = LatencyModeForMS(out.LatencyMS)
		if out.LatencyMode == LatencyCustom {
			out.LatencyCustomMS = out.LatencyMS
		}
	}
	return out
}

// LatencyModeForMS names the preset a stored millisecond value corresponds to.
func LatencyModeForMS(ms int) LatencyMode {
	switch ms {
	case LatencyLowMS:
		return LatencyLow
	case LatencyStandardMS:
		return LatencyStandard
	case LatencyStableMS:
		return LatencyStable
	}
	return LatencyCustom
}

// LatencyModeValue is the presentation latency a mode resolves to.
func LatencyModeValue(mode LatencyMode, customMS int) int {
	switch mode {
	case LatencyLow:
		return LatencyLowMS
	case LatencyStandard:
		return LatencyStandardMS
	case LatencyStable:
		return LatencyStableMS
	case LatencyCustom:
		return clampLatency(customMS)
	}
	return LatencyLowMS
}

// LatencyText renders a latency value for the UI: "200 ms", "1.5 s", "2 s".
func LatencyText(ms int) string {
	switch {
	case ms >= 1000 && ms%1000 == 0:
		return strconv.Itoa(ms/1000) + " s"
	case ms > 1000:
		return strconv.FormatFloat(float64(ms)/1000, 'f', 1, 64) + " s"
	default:
		return strconv.Itoa(ms) + " ms"
	}
}

func clampLatency(ms int) int {
	if ms < 20 {
		return 20
	}
	if ms > 2000 {
		return 2000
	}
	return ms
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
