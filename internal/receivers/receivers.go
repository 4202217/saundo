// Package receivers is the single source of truth for the receivers the user
// manages: discovered ones it remembers, and manual ones the user typed in.
// The engine goroutine is the only production writer; the UI reads copies in
// the engine snapshot. The merge of registry and discovery, the flyout
// filter, the auto-connect choice, and the latency arithmetic are pure
// functions here, so the rules they encode are testable without sockets.
package receivers

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"saundo/internal/airplay/device"
)

// Kind says how an entry entered the registry. A discovered entry is
// remembered from an advertisement and cannot be removed from the UI; a
// manual one was typed in and keeps its key for life.
type Kind string

const (
	KindManual     Kind = "manual"
	KindDiscovered Kind = "discovered"
)

// Type is the family a manual entry declares. A discovered entry never uses
// it: its advertisement supplies the real auth path and feature bits.
type Type string

const (
	TypeAirPortExpress Type = "airport-express"
	TypeAirPlay1       Type = "airplay-1"
	TypeAirPlay2       Type = "airplay-2"
)

// The per-receiver delay compensates a receiver's DSP/DAC buffering. It is
// added to the global base latency at connect, and the sum is clamped to the
// sender's 20..4000 ms window.
const (
	MaxDelayMS = 2000
	// MinLatencyMS and MaxLatencyMS are the sender's latency window; the
	// base-plus-delay sum clamps into it.
	MinLatencyMS = 20
	MaxLatencyMS = 4000
)

const (
	manualKeyPrefix = "m:"
	groupKeyPrefix  = "stereo:"
)

// Entry is one receiver's persisted configuration. Key is the identity the
// engine, the UI rows, and the credential store share, and it never changes:
// the deviceid for a discovered receiver, "stereo:<tsid>" for a remembered
// stereo pair, "m:<uuid>" for a manual receiver.
type Entry struct {
	Key         string `json:"key"`
	Kind        Kind   `json:"kind"`
	Type        Type   `json:"type,omitempty"`
	Name        string `json:"name,omitempty"`
	Host        string `json:"host,omitempty"`
	Port        uint16 `json:"port,omitempty"`
	Model       string `json:"model,omitempty"`
	AirPlay2    bool   `json:"airPlay2,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
	AutoConnect bool   `json:"autoConnect,omitempty"`
	DelayMS     int    `json:"delayMs,omitempty"`
}

// Registry is the document stored in receivers.json.
type Registry struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// SchemaVersion stamps every save. A higher number in a file this build
// reads means a newer build wrote it; unknown fields decode and stay.
const SchemaVersion = 1

// Get returns the entry with the key.
func (r *Registry) Get(key string) (Entry, bool) {
	for _, e := range r.Entries {
		if e.Key == key {
			return e, true
		}
	}
	return Entry{}, false
}

// Put replaces the entry with the same key, or appends it.
func (r *Registry) Put(e Entry) {
	for i := range r.Entries {
		if r.Entries[i].Key == e.Key {
			r.Entries[i] = e
			return
		}
	}
	r.Entries = append(r.Entries, e)
}

// Delete removes the entry with the key and reports whether it was there.
func (r *Registry) Delete(key string) bool {
	for i := range r.Entries {
		if r.Entries[i].Key == key {
			r.Entries = append(r.Entries[:i], r.Entries[i+1:]...)
			return true
		}
	}
	return false
}

// SingleAutoConnect keeps the one-target invariant: every entry but key
// loses AutoConnect, and key gains it when on. The engine calls it inside
// one Store.Update, so the file never holds two auto-connect targets.
func (r *Registry) SingleAutoConnect(key string, on bool) {
	for i := range r.Entries {
		if r.Entries[i].Key == key {
			r.Entries[i].AutoConnect = on
		} else if on {
			r.Entries[i].AutoConnect = false
		}
	}
}

func (e Entry) IsManual() bool { return e.Kind == KindManual }

// IsGroup reports whether the entry is a remembered stereo pair. Settings on
// a group row apply to the pair as a whole, and its password may serve both
// members.
func (e Entry) IsGroup() bool { return strings.HasPrefix(e.Key, groupKeyPrefix) }

// DisplayName is the user's name, or the last seen one, or the host.
func (e Entry) DisplayName() string {
	if strings.TrimSpace(e.Name) != "" {
		return e.Name
	}
	if e.Host != "" {
		return e.Host
	}
	return e.Key
}

// ClampedDelay brings a stored or user-entered delay into 0..MaxDelayMS.
func ClampedDelay(ms int) int {
	if ms < 0 {
		return 0
	}
	if ms > MaxDelayMS {
		return MaxDelayMS
	}
	return ms
}

// EffectiveLatencyMS is the presentation latency one receiver gets: the
// global base plus that receiver's DSP/DAC delay, clamped to the sender's
// 20..4000 ms window. The flyout shows this value for the active receiver.
func EffectiveLatencyMS(globalMS, delayMS int) int {
	ms := globalMS + ClampedDelay(delayMS)
	if ms < MinLatencyMS {
		return MinLatencyMS
	}
	if ms > MaxLatencyMS {
		return MaxLatencyMS
	}
	return ms
}

// ManualAuth is the pairing path for a manual receiver. A saved password
// moves an AirPlay 2 receiver from transient pairing to HAP password
// pairing; without one the receiver must accept transient pairing. A manual
// receiver never uses PTP: its advertisement is unavailable, so PTP
// receivers must be discovered.
func ManualAuth(t Type, hasPassword bool) device.Auth {
	switch t {
	case TypeAirPortExpress:
		return device.AuthSetup
	case TypeAirPlay2:
		if hasPassword {
			return device.AuthHAPPassword
		}
		return device.AuthHAPTransient
	default:
		if hasPassword {
			return device.AuthPassword
		}
		return device.AuthNone
	}
}

// NewManualKey mints a fresh manual receiver key.
func NewManualKey() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return manualKeyPrefix + "0000000000000000"
	}
	return manualKeyPrefix + hex.EncodeToString(b[:])
}
