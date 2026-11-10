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
	"time"

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

// Member is one speaker of a remembered stereo pair. Discovery supplies the
// addresses; keeping them lets the next launch connect the pair without
// waiting for mDNS.
type Member struct {
	ID       string      `json:"id,omitempty"`
	Name     string      `json:"name,omitempty"`
	Host     string      `json:"host,omitempty"`
	Port     uint16      `json:"port,omitempty"`
	Model    string      `json:"model,omitempty"`
	AirPlay2 bool        `json:"airPlay2,omitempty"`
	Features uint64      `json:"features,omitempty"`
	SF       uint64      `json:"sf,omitempty"`
	Auth     device.Auth `json:"auth,omitempty"`
}

// Entry is one receiver's persisted configuration. Key is the identity the
// engine, the UI rows, and the credential store share, and it never changes:
// the deviceid for a discovered receiver, "stereo:<tsid>" for a remembered
// stereo pair, "m:<uuid>" for a manual receiver.
type Entry struct {
	Key      string `json:"key"`
	Kind     Kind   `json:"kind"`
	Type     Type   `json:"type,omitempty"`
	Name     string `json:"name,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     uint16 `json:"port,omitempty"`
	Model    string `json:"model,omitempty"`
	AirPlay2 bool   `json:"airPlay2,omitempty"`
	// Features and SF are the last advertised bitfields, cached so a launch
	// can route a remembered receiver to PTP or NTP without discovery.
	Features uint64      `json:"features,omitempty"`
	SF       uint64      `json:"sf,omitempty"`
	Auth     device.Auth `json:"auth,omitempty"`
	// Members caches a stereo pair's speaker addresses so the next launch can
	// connect without waiting for discovery. Empty for a single receiver.
	Members     []Member `json:"members,omitempty"`
	Hidden      bool     `json:"hidden,omitempty"`
	AutoConnect bool     `json:"autoConnect,omitempty"`
	DelayMS     int      `json:"delayMs,omitempty"`
	// LastRetransmits and LastSentBlocks drive the next session's duplicate
	// count: clean sessions get one copy, lossy ones two or three.
	LastRetransmits uint64 `json:"lastRetransmits,omitempty"`
	LastSentBlocks  uint64 `json:"lastSentBlocks,omitempty"`
	// LastSeen is the last time discovery saw this receiver. A remembered
	// stereo group unseen for GroupGrace is pruned because a split and
	// re-merge mints a new tsid, so the old key never returns.
	LastSeen time.Time `json:"lastSeen,omitempty"`
}

// GroupGrace is how long a remembered stereo group may stay unseen before its
// row is pruned. A day covers a receiver that is merely powered off for a
// while; a group gone longer than that has been split or re-merged, which
// produces a new tsid under a new key.
const GroupGrace = 24 * time.Hour

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

// PruneStaleGroups removes remembered stereo groups discovery has not seen for
// grace and returns their keys. A group with no LastSeen is kept: the caller
// stamps it first so an upgraded registry gets a full grace period.
func PruneStaleGroups(reg *Registry, seen map[string]bool, now time.Time, grace time.Duration) []string {
	if reg == nil || grace <= 0 {
		return nil
	}
	var removed []string
	kept := reg.Entries[:0]
	for _, e := range reg.Entries {
		if e.IsGroup() && !seen[e.Key] && !e.LastSeen.IsZero() && now.Sub(e.LastSeen) > grace {
			removed = append(removed, e.Key)
			continue
		}
		kept = append(kept, e)
	}
	reg.Entries = kept
	return removed
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

// CachedDevice builds the wire description of a remembered receiver from its
// persisted fields, so a launch can connect before discovery answers. The
// bool is false when the entry has no usable address.
func CachedDevice(e Entry, hasPassword bool) (device.Device, bool) {
	if e.Host == "" || e.Port == 0 {
		return device.Device{}, false
	}
	if e.IsGroup() && len(e.Members) == 0 {
		// The first sighting of a pair has only the leader's address. Wait
		// for discovery instead of connecting a lone speaker by mistake.
		return device.Device{}, false
	}
	model := e.Model
	if model == "" {
		model = manualModel(TypeAirPlay2)
	}
	auth := e.Auth
	if auth == device.AuthNone {
		switch {
		case hasPassword:
			auth = device.AuthHAPPassword
		case e.AirPlay2:
			auth = device.AuthHAPTransient
		}
	}
	d := device.Device{
		ID:       e.Key,
		Name:     e.DisplayName(),
		Host:     e.Host,
		Port:     e.Port,
		Model:    model,
		Features: e.Features,
		SF:       e.SF,
		AirPlay2: e.AirPlay2,
		Auth:     auth,
		Family:   device.FamilyFor(model, ""),
		Online:   true,
	}
	if e.IsGroup() && len(e.Members) > 0 {
		d.StereoID = strings.TrimPrefix(e.Key, groupKeyPrefix)
		d.Members = make([]device.Device, 0, len(e.Members))
		for _, m := range e.Members {
			mm := m.Model
			if mm == "" {
				mm = model
			}
			ma := m.Auth
			if ma == device.AuthNone {
				ma = auth
			}
			d.Members = append(d.Members, device.Device{
				ID:       m.ID,
				Name:     m.Name,
				Host:     m.Host,
				Port:     m.Port,
				Model:    mm,
				Features: m.Features,
				SF:       m.SF,
				AirPlay2: m.AirPlay2,
				Auth:     ma,
				Family:   device.FamilyFor(mm, ""),
				Online:   true,
			})
		}
	}
	return d, true
}

// DuplicatesFor picks the extra audio copies the next session should send.
// A first session or a lossy one gets one extra copy, a clean one none, and
// a session above 3% retransmits gets two.
func DuplicatesFor(e Entry) int {
	if e.LastSentBlocks == 0 {
		return 1
	}
	ratio := float64(e.LastRetransmits) / float64(e.LastSentBlocks)
	switch {
	case ratio > 0.03:
		return 2
	case ratio > 0:
		return 1
	default:
		return 0
	}
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
