package receivers

import (
	"sort"
	"strings"

	"saundo/internal/airplay/device"
)

// Target is one row the engine manages: the wire description the sender
// needs plus the registry configuration attached to it.
type Target struct {
	Device device.Device
	Entry  Entry
}

// Merge joins the registry with discovery's current view. It returns every
// receiver the user can see or manage, sorted by display name then key:
//
//   - a remembered entry whose device is online takes the live advertisement;
//   - a remembered entry whose device is gone stays as an offline row, so its
//     settings and auto-connect flag survive a receiver that is powered off;
//   - a manual entry renders from its stored endpoint, always attemptable,
//     with the live advertisement's model and features when the same key is
//     seen (a manual row that was learned by the engine takes the
//     advertisement's identity fields);
//   - a first sighting is returned with a zero entry; the engine persists it.
//
// hasPassword reports whether a stored password exists for a key; it may be
// nil. Manual keys ("m:<uuid>") never collide with discovery keys (MACs),
// so one physical receiver shows as two rows only before the engine learns
// its manual row's deviceid.
func Merge(reg Registry, discovered []device.Device, hasPassword func(key string) bool) []Target {
	live := make(map[string]device.Device, len(discovered))
	for _, d := range discovered {
		live[d.ID] = d
	}
	out := make([]Target, 0, len(reg.Entries)+len(live))
	for _, e := range reg.Entries {
		if e.IsManual() {
			d := manualDevice(e, hasPassword != nil && hasPassword(e.Key))
			if seen, ok := live[e.Key]; ok {
				d = claimedDevice(d, seen)
				delete(live, seen.ID)
			}
			out = append(out, Target{Device: d, Entry: e})
			continue
		}
		if d, ok := live[e.Key]; ok {
			delete(live, d.ID)
			out = append(out, Target{Device: d, Entry: e})
			continue
		}
		out = append(out, Target{Device: offlineDevice(e), Entry: e})
	}
	for id, d := range live {
		// A first sighting. The engine persists it as a remembered entry;
		// the zero entry here keeps the pure function side-effect free.
		out = append(out, Target{Device: d, Entry: Entry{Key: id, Kind: KindDiscovered}})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Entry, out[j].Entry
		an, bn := a.DisplayName(), b.DisplayName()
		if an != bn {
			return an < bn
		}
		return a.Key < b.Key
	})
	return out
}

// manualDevice builds the wire description of a typed-in receiver. The
// address is attemptable without mDNS, so the row is Online by definition;
// a failed dial surfaces as a session error.
func manualDevice(e Entry, hasPassword bool) device.Device {
	name := e.DisplayName()
	model := e.Model
	if model == "" {
		model = manualModel(e.Type)
	}
	return device.Device{
		ID:       e.Key,
		Name:     name,
		Host:     e.Host,
		Port:     e.Port,
		Model:    model,
		Auth:     ManualAuth(e.Type, hasPassword),
		AirPlay2: e.Type == TypeAirPlay2,
		Online:   true,
	}
}

// claimedDevice folds a live advertisement into a manual row: the manual
// identity and name stay, everything the sender needs comes from the
// advertisement. It learns nothing on its own; the engine decides what to
// persist from the pair.
func claimedDevice(manual, seen device.Device) device.Device {
	d := seen
	d.ID = manual.ID
	if manual.Name != "" {
		d.Name = manual.Name
	}
	d.Online = true
	return d
}

// offlineDevice keeps a remembered receiver manageable while it is gone.
func offlineDevice(e Entry) device.Device {
	model := e.Model
	if model == "" {
		model = manualModel(TypeAirPlay2)
	}
	return device.Device{
		ID:       e.Key,
		Name:     e.DisplayName(),
		Model:    model,
		AirPlay2: e.AirPlay2,
		Online:   false,
	}
}

func manualModel(t Type) string {
	switch t {
	case TypeAirPortExpress:
		return "AirPort Express"
	case TypeAirPlay1:
		return "AirPlay 1"
	default:
		return "AirPlay 2"
	}
}

// Manageable is the list both surfaces show: manual entries always, online
// receivers, and the active session even when it just went offline so its
// controls stay reachable. A remembered receiver that is offline has nothing
// to configure or connect to; its entry stays in the registry and returns to
// the list when the device is seen again.
func Manageable(targets []Target, activeKey string) []Target {
	out := targets[:0]
	for _, t := range targets {
		if t.Entry.IsManual() || t.Device.Online || t.Entry.Key == activeKey {
			out = append(out, t)
		}
	}
	return out
}

// Visible is the flyout list: every target whose entry is shown, minus the
// hidden and the disabled. The active session's receiver stays even hidden,
// so its row, volume, and controls remain reachable.
func Visible(targets []Target, activeKey string) []device.Device {
	out := make([]device.Device, 0, len(targets))
	for _, t := range targets {
		if t.Entry.Key != activeKey && t.Entry.Hidden {
			continue
		}
		out = append(out, t.Device)
	}
	return out
}

// AutoTarget picks the receiver this launch connects to: the single entry
// carrying the auto-connect flag. It returns false when no entry asks to
// auto-connect.
func AutoTarget(reg Registry) (string, bool) {
	for _, e := range reg.Entries {
		if e.AutoConnect {
			return e.Key, true
		}
	}
	return "", false
}

// NormalizeHost lowercases and trims a manual host so "LivingRoom.local "
// and "livingroom.local" are one address.
func NormalizeHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}
