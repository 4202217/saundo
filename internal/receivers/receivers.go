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
	// KindCombo marks a user-created stereo combo: two independent receivers
	// that play one input side each. Unlike a remembered stereo pair, a combo
	// is configuration, not an advertisement — it is never pruned for being
	// unseen, and its row stays manageable while a member is offline.
	KindCombo Kind = "combo"
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
	// base-plus-delay sum clamps into it. The sender uses the same constants.
	MinLatencyMS = device.MinLatencyMS
	MaxLatencyMS = device.MaxLatencyMS
)

const (
	manualKeyPrefix = "m:"
	groupKeyPrefix  = "stereo:"
	comboKeyPrefix  = "combo:"
)

// The stored channel preference. A stereo pair always plays stereo; a single
// receiver may play either input channel into both outputs.
const (
	ChannelStereo = "stereo"
	ChannelLeft   = "left"
	ChannelRight  = "right"
)

// ParseChannel normalizes a stored channel preference. Empty and unknown
// values read as stereo, because this is a preference, not a capability gate.
func ParseChannel(value string) string {
	switch value {
	case ChannelLeft:
		return ChannelLeft
	case ChannelRight:
		return ChannelRight
	default:
		return ChannelStereo
	}
}

// Member is one speaker of a remembered stereo pair, or one side of a
// stereo combo. Discovery supplies the addresses of a pair's speakers;
// keeping them lets the next launch connect the pair without waiting for
// mDNS. A combo member stores only the member's registry key in ID — the
// member's own entry is the source of truth for its endpoint and
// credentials — plus a display-name cache for the degenerate case where the
// member's entry is gone.
type Member struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Host      string `json:"host,omitempty"`
	Port      uint16 `json:"port,omitempty"`
	Model     string `json:"model,omitempty"`
	OSVersion string `json:"osVersion,omitempty"`
	AirPlay2  bool   `json:"airPlay2,omitempty"`
	Features  uint64 `json:"features,omitempty"`
	// FeaturesUnknown marks a cached value whose advertisement did not parse.
	FeaturesUnknown bool `json:"featuresUnknown,omitempty"`
	// PairedHost and PairedPort pin the endpoint this member was paired with.
	PairedHost string      `json:"pairedHost,omitempty"`
	PairedPort uint16      `json:"pairedPort,omitempty"`
	SF         uint64      `json:"sf,omitempty"`
	Auth       device.Auth `json:"auth,omitempty"`
}

// Entry is one receiver's persisted configuration. Key is the identity the
// engine, the UI rows, and the credential store share, and it never changes:
// the deviceid for a discovered receiver, "stereo:<tsid>" for a remembered
// stereo pair, "combo:<hex>" for a stereo combo, "m:<uuid>" for a manual
// receiver.
type Entry struct {
	Key       string `json:"key"`
	Kind      Kind   `json:"kind"`
	Type      Type   `json:"type,omitempty"`
	Name      string `json:"name,omitempty"`
	Host      string `json:"host,omitempty"`
	Port      uint16 `json:"port,omitempty"`
	Model     string `json:"model,omitempty"`
	OSVersion string `json:"osVersion,omitempty"`
	AirPlay2  bool   `json:"airPlay2,omitempty"`
	// Features and SF are the last advertised bitfields, cached so a launch
	// can route a remembered receiver to PTP or NTP without discovery.
	Features uint64 `json:"features,omitempty"`
	// FeaturesUnknown marks a cached features value that failed to parse.
	FeaturesUnknown bool `json:"featuresUnknown,omitempty"`
	// PairedHost and PairedPort pin the endpoint the receiver was paired
	// with. Discovery cannot authenticate an advertisement, so a device id
	// with stored credentials is never moved to a different address.
	PairedHost string      `json:"pairedHost,omitempty"`
	PairedPort uint16      `json:"pairedPort,omitempty"`
	SF         uint64      `json:"sf,omitempty"`
	Auth       device.Auth `json:"auth,omitempty"`
	// Members caches a stereo pair's speaker addresses so the next launch can
	// connect without waiting for discovery. A combo stores its two member
	// keys here in playback order: Members[0] plays the left side, Members[1]
	// the right, and swapping sides reorders them. Empty for a single
	// receiver.
	Members     []Member `json:"members,omitempty"`
	Hidden      bool     `json:"hidden,omitempty"`
	AutoConnect bool     `json:"autoConnect,omitempty"`
	DelayMS     int      `json:"delayMs,omitempty"`
	// Channel is the stereo side a single receiver plays: "stereo", "left",
	// or "right". A stereo pair and a combo always play stereo — a combo's
	// sides live in the member order, not here. Empty and unknown values read
	// as stereo, because this is a preference, not a capability gate.
	Channel string `json:"channel,omitempty"`
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
// reads means a newer build wrote it; unknown fields decode and stay. 2 added
// the per-receiver channel and the combo rows, so an older build treats a v2
// file as read-only instead of dropping those fields on its next save.
const SchemaVersion = 2

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

func (e Entry) IsManual() bool { return e.Kind == KindManual }

// IsCombo reports whether the entry is a user-created stereo combo. Settings
// on a combo row apply to the combo as a whole: one volume, one auto-connect
// flag, one hidden flag. Unlike a remembered pair, a combo is never pruned
// for being unseen and its members keep their own credentials.
func (e Entry) IsCombo() bool { return e.Kind == KindCombo }

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

// Endpoint is the address a session should use: the paired endpoint when one
// is pinned, else the last advertised or typed one.
func (e Entry) Endpoint() (string, uint16) {
	if e.PairedHost != "" && e.PairedPort != 0 {
		return e.PairedHost, e.PairedPort
	}
	return e.Host, e.Port
}

// CachedDevice builds the wire description of a remembered receiver from its
// persisted fields, so a launch can connect before discovery answers. The
// bool is false when the entry has no usable address.
func CachedDevice(e Entry, hasPassword bool) (device.Device, bool) {
	host, port := e.Endpoint()
	if host == "" || port == 0 {
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
		ID:        e.Key,
		Name:      e.DisplayName(),
		Host:      host,
		Port:      port,
		Model:     model,
		OSVersion: e.OSVersion,
		Features:  device.FeaturesFrom(e.Features, e.FeaturesUnknown),
		SF:        e.SF,
		AirPlay2:  e.AirPlay2,
		Auth:      auth,
		Family:    device.FamilyFor(model, ""),
		Online:    true,
	}
	if e.IsGroup() && len(e.Members) > 0 {
		d.StereoID = strings.TrimPrefix(e.Key, groupKeyPrefix)
		d.Members = make([]device.Device, 0, len(e.Members))
		for _, m := range e.Members {
			mm := m.Model
			if mm == "" {
				mm = model
			}
			mv := m.OSVersion
			if mv == "" {
				mv = e.OSVersion
			}
			ma := m.Auth
			if ma == device.AuthNone {
				ma = auth
			}
			d.Members = append(d.Members, device.Device{
				ID:        m.ID,
				Name:      m.Name,
				Host:      m.Host,
				Port:      m.Port,
				Model:     mm,
				OSVersion: mv,
				Features:  device.FeaturesFrom(m.Features, m.FeaturesUnknown),
				SF:        m.SF,
				AirPlay2:  m.AirPlay2,
				Auth:      ma,
				Family:    device.FamilyFor(mm, ""),
				Online:    true,
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

// NewComboKey mints a fresh stereo combo key.
func NewComboKey() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return comboKeyPrefix + "0000000000000000"
	}
	return comboKeyPrefix + hex.EncodeToString(b[:])
}
