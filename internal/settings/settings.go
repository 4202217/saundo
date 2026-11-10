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

	"saundo/internal/lamp"
)

// ThemeMode is the user's light or dark appearance choice. System follows
// Windows.
type ThemeMode string

const (
	ThemeSystem ThemeMode = "system"
	ThemeLight  ThemeMode = "light"
	ThemeDark   ThemeMode = "dark"
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

// LampSettings is the global ambient-light choice. The light is always part
// of the app; Off is the manual "关灯" state. Cover chooses the media cover;
// when it is off, Preset renders one of the six templates. LegacyEnabled
// decodes the old on/off switch so an existing file keeps its state.
type LampSettings struct {
	Off           bool   `json:"off"`
	Cover         bool   `json:"cover"`
	Preset        string `json:"preset"`
	LegacyEnabled *bool  `json:"enabled,omitempty"`
}

// UnmarshalJSON tracks whether the file had a cover field. An old template
// file has no cover field and must stay cover off, not inherit the default
// cover-on value from the pre-populated settings struct.
func (l *LampSettings) UnmarshalJSON(data []byte) error {
	var raw struct {
		Off           *bool  `json:"off"`
		Cover         *bool  `json:"cover"`
		Preset        string `json:"preset"`
		LegacyEnabled *bool  `json:"enabled"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	l.Off = raw.Off != nil && *raw.Off
	l.Cover = raw.Cover != nil && *raw.Cover
	l.Preset = raw.Preset
	l.LegacyEnabled = raw.LegacyEnabled
	return nil
}

// Settings is the user's configuration.
// CoverMode is what the Now Playing card shows on its left.
type CoverMode string

const (
	CoverNone        CoverMode = "none"
	CoverPlaceholder CoverMode = "placeholder"
	CoverMedia       CoverMode = "media"
)

type Settings struct {
	Name                string             `json:"name"`
	NameMigrated        bool               `json:"nameMigrated"`
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
	ShowNowPlayingPanel bool               `json:"showNowPlayingPanel"`
	PlaybackControls    bool               `json:"playbackControls"`
	CoverColors         bool               `json:"coverColors"`
	CoverMode           CoverMode          `json:"coverMode"`
	AudioLevel          bool               `json:"audioLevel"`
	MuteLocalOnConnect  bool               `json:"muteLocalOnConnect"`
	Lamp                LampSettings       `json:"lamp"`
	TrayAnimation       bool               `json:"trayAnimation"`
	LaunchAtStart       bool               `json:"launchAtStart"`
	Language            string             `json:"language"`
	Theme               ThemeMode          `json:"theme"`
	Beta                bool               `json:"beta"`
	AutoUpdate          bool               `json:"autoUpdate"`
	AutoDownload        bool               `json:"autoDownload"`
	SkippedVersion      string             `json:"skippedVersion"`
	LastCheck           string             `json:"lastCheck"`
}

// Default returns the first-run configuration.
func Default() Settings {
	return Settings{
		Name:                "声渡",
		DeviceID:            newDeviceID(),
		Volume:              80,
		AutoUpdate:          true,
		VolumeByDevice:      make(map[string]float64),
		Passwords:           make(map[string]string),
		LatencyMS:           LatencyLowMS,
		LatencyMode:         LatencyLow,
		LatencyCustomMS:     DefaultLatencyCustomMS,
		ShowLatencySwitch:   true,
		ShowNowPlayingPanel: true,
		PlaybackControls:    true,
		CoverColors:         true,
		CoverMode:           CoverMedia,
		AudioLevel:          true,
		MuteLocalOnConnect:  true,
		Reconnect:           true,
		TrayAnimation:       true,
		Theme:               ThemeSystem,
		Lamp:                LampSettings{Cover: true, Preset: lamp.DefaultID},
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

// Dir returns the directory holding settings.json and the engine's guard
// files. A test store points it at the test's temporary directory.
func (s *Store) Dir() string { return filepath.Dir(s.path) }

// Load returns the stored settings, filling defaults for missing fields. A
// missing or corrupt file yields the defaults.
func (s *Store) Load() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

// Update loads the stored settings, calls fn with them, and saves the result,
// all under the store lock. fn runs under that lock and must not call the
// store. Each writer reads the file as it is now, so it cannot clobber fields
// another writer changed since this process started.
func (s *Store) Update(fn func(*Settings)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.loadLocked()
	fn(&v)
	return s.saveLocked(v)
}

func (s *Store) loadLocked() Settings {
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
	switch out.Theme {
	case ThemeLight, ThemeDark:
	default:
		out.Theme = ThemeSystem
	}
	switch out.CoverMode {
	case CoverNone, CoverPlaceholder, CoverMedia:
	default:
		out.CoverMode = CoverMedia
	}
	// The old switch stored "enabled"; invert it once and drop the field.
	if out.Lamp.LegacyEnabled != nil {
		out.Lamp.Off = !*out.Lamp.LegacyEnabled
		out.Lamp.LegacyEnabled = nil
	}
	// The old model stored cover mode as the "media" preset. A legacy media,
	// empty, or unknown value turns cover on and resets to the default
	// template. A valid template keeps the stored cover flag, which is absent
	// (false) in every old template file.
	switch out.Lamp.Preset {
	case "media", "":
		out.Lamp.Cover = true
		out.Lamp.Preset = lamp.DefaultID
	default:
		if _, ok := lamp.ByID(out.Lamp.Preset); !ok {
			out.Lamp.Cover = true
			out.Lamp.Preset = lamp.DefaultID
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
	return s.saveLocked(v)
}

func (s *Store) saveLocked(v Settings) error {
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
