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
//   - a first sighting is returned with a zero entry; the engine persists it;
//   - a combo renders as one synthesized stereo-group row built from its two
//     members' own merged rows.
//
// hasPassword reports whether a stored password exists for a key; it may be
// nil. Manual keys ("m:<uuid>") never collide with discovery keys (MACs),
// so one physical receiver shows as two rows only before the engine learns
// its manual row's deviceid. The member rows a combo replaces stay in the
// output: DropLinked removes them at the projection point, so lookups by a
// member key keep working while its combo exists.
func Merge(reg Registry, discovered []device.Device, hasPassword func(key string) bool) []Target {
	live := make(map[string]device.Device, len(discovered))
	for _, d := range discovered {
		live[d.ID] = d
	}
	out := make([]Target, 0, len(reg.Entries)+len(live))
	rows := make(map[string]Target, len(reg.Entries))
	for _, e := range reg.Entries {
		if e.IsCombo() {
			continue
		}
		var t Target
		if e.IsManual() {
			d := manualDevice(e, hasPassword != nil && hasPassword(e.Key))
			if seen, ok := live[e.Key]; ok {
				d = claimedDevice(d, seen)
				delete(live, seen.ID)
			}
			if d.Local {
				e = localEntry(e)
			}
			t = Target{Device: d, Entry: e}
		} else if d, ok := live[e.Key]; ok {
			delete(live, d.ID)
			d = pinnedDevice(e, d)
			if d.Local {
				e = localEntry(e)
			}
			t = Target{Device: d, Entry: e}
		} else {
			t = Target{Device: offlineDevice(e, hasPassword), Entry: e}
		}
		rows[e.Key] = t
		out = append(out, t)
	}
	for _, e := range reg.Entries {
		if !e.IsCombo() {
			continue
		}
		out = append(out, ComboTarget(e, rows))
	}
	for id, d := range live {
		// A first sighting. The engine persists it as a remembered entry;
		// the zero entry here keeps the pure function side-effect free.
		entry := Entry{Key: id, Kind: KindDiscovered}
		if d.Local {
			entry = localEntry(entry)
		}
		out = append(out, Target{Device: d, Entry: entry})
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

// ComboTarget synthesizes a combo's merged row: one stereo-group device over
// the two members' own rows, in the entry's member order (left, then right).
// A member whose entry is gone renders from its cached name as an offline
// row, so the combo stays visible and unlinkable instead of vanishing.
func ComboTarget(e Entry, rows map[string]Target) Target {
	members := make([]device.Device, 0, len(e.Members))
	for _, m := range e.Members {
		if t, ok := rows[m.ID]; ok {
			members = append(members, t.Device)
			continue
		}
		members = append(members, device.Device{ID: m.ID, Name: m.Name})
	}
	return Target{Device: ComboDevice(e, members), Entry: e}
}

// ComboDevice builds the wire description of a stereo combo: one synthesized
// stereo-group device over its two members. StereoID carries the combo key
// so IsStereoGroup gates the pair plumbing; the pair's own protocol fields
// (group UUID, timing peers) stay empty because a combo's members are
// independent receivers, not a native pair.
func ComboDevice(e Entry, members []device.Device) device.Device {
	online := len(members) > 0
	model := ""
	for _, m := range members {
		if !m.Online {
			online = false
		}
		if model == "" {
			model = m.Model
		}
	}
	d := device.Device{
		ID:       e.Key,
		Name:     e.DisplayName(),
		Model:    model,
		Family:   device.FamilyFor(model, ""),
		AirPlay2: true,
		StereoID: e.Key,
		Members:  members,
		Online:   online,
	}
	if len(members) > 0 {
		d.Host, d.Port = members[0].Host, members[0].Port
	}
	return d
}

// localEntry is the row a receiver on this host gets: the hidden group and
// no auto-connect. The sender cannot target its own host, so the row exists
// only to show that the receiver was seen and is not usable.
func localEntry(e Entry) Entry {
	e.Hidden = true
	e.AutoConnect = false
	return e
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
		Family:   device.FamilyFor(model, ""),
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

// pinnedDevice keeps a paired receiver at the endpoint it was paired with.
// The rest of the advertisement still updates: name, model, feature bits.
func pinnedDevice(e Entry, d device.Device) device.Device {
	if e.PairedHost == "" || e.PairedPort == 0 {
		return d
	}
	d.Host, d.Port = e.PairedHost, e.PairedPort
	return d
}

// offlineDevice keeps a remembered receiver manageable while it is gone. The
// cached endpoint, auth path, and feature bits travel with it: a connect that
// lands on this row must not fall back to AP1 or NTP, and the row still shows
// the address the next launch probes.
func offlineDevice(e Entry, hasPassword func(string) bool) device.Device {
	// The registry projection and the cold-start connect must describe the
	// same receiver: an offline pair keeps its cached members and their
	// software versions, or the ambient-light capability gate fails closed
	// while the session itself already has both members.
	if d, ok := CachedDevice(e, hasPassword != nil && hasPassword(e.Key)); ok {
		d.Online = false
		return d
	}
	model := e.Model
	if model == "" {
		model = manualModel(TypeAirPlay2)
	}
	host, port := e.Endpoint()
	return device.Device{
		ID:       e.Key,
		Name:     e.DisplayName(),
		Host:     host,
		Port:     port,
		Model:    model,
		Features: device.FeaturesFrom(e.Features, e.FeaturesUnknown),
		SF:       e.SF,
		AirPlay2: e.AirPlay2,
		Auth:     e.Auth,
		Family:   device.FamilyFor(model, ""),
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

// Manageable is the list both surfaces show: manual entries and combos
// always (both are user-created configuration), online receivers, and every
// active target even when it just went offline so its controls stay
// reachable. A remembered receiver that is offline has nothing to configure
// or connect to; its entry stays in the registry and returns to the list
// when the device is seen again.
func Manageable(targets []Target, active map[string]bool) []Target {
	out := targets[:0]
	for _, t := range targets {
		if t.Entry.IsManual() || t.Entry.IsCombo() || t.Device.Online || active[t.Entry.Key] {
			out = append(out, t)
		}
	}
	return out
}

// DropLinked removes the member rows a combo replaces. While the combo
// exists its members play through the combo row; their own rows return when
// the combo is unlinked. The engine keeps the unfiltered merge for lookups
// by member key, so this runs at the projection point, not inside Merge.
func DropLinked(targets []Target) []Target {
	linked := make(map[string]bool)
	for _, t := range targets {
		if t.Entry.IsCombo() {
			for _, m := range t.Entry.Members {
				linked[m.ID] = true
			}
		}
	}
	if len(linked) == 0 {
		return targets
	}
	out := targets[:0]
	for _, t := range targets {
		if !linked[t.Entry.Key] {
			out = append(out, t)
		}
	}
	return out
}

// Visible is the flyout list: every target whose entry is shown, minus the
// hidden and the disabled. Every active target stays even when hidden, so its
// row, volume, and controls remain reachable.
func Visible(targets []Target, active map[string]bool) []device.Device {
	out := make([]device.Device, 0, len(targets))
	for _, t := range targets {
		if !active[t.Entry.Key] && t.Entry.Hidden {
			continue
		}
		out = append(out, t.Device)
	}
	return out
}

// AutoTargets lists the flagged launch set in registry order: every entry
// carrying the auto-connect flag, minus any receiver linked into a combo. A
// linked member's row is hidden, and the combo carries the launch intent, so
// a stale member flag never launches that receiver on its own.
func AutoTargets(reg Registry) []string {
	linked := make(map[string]bool)
	for _, e := range reg.Entries {
		if !e.IsCombo() {
			continue
		}
		for _, m := range e.Members {
			linked[m.ID] = true
		}
	}
	var keys []string
	for _, e := range reg.Entries {
		if e.AutoConnect && !linked[e.Key] {
			keys = append(keys, e.Key)
		}
	}
	return keys
}

// NormalizeHost lowercases and trims a manual host so "LivingRoom.local "
// and "livingroom.local" are one address.
func NormalizeHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}
