package app

import (
	"strings"

	"saundo/internal/airplay/device"
	"saundo/internal/engine"
	"saundo/internal/receivers"
)

// sessionMember is one active target as the interface reads it: the device row
// the user sees plus the per-target controls the engine publishes.
type sessionMember struct {
	Device    device.Device
	Target    engine.TargetControl
	HasTarget bool
}

// sessionMembers lists the active targets in snapshot order, primary first.
// It is the one place the interface asks who is in the session; the tray, the
// connection page, the sidebar, and the latency line read it instead of
// assuming a single selected device.
func (a *App) sessionMembers() []sessionMember {
	if len(a.snap.ActiveTargets) == 0 {
		return nil
	}
	devices := make(map[string]device.Device, len(a.snap.Devices))
	for _, d := range a.snap.Devices {
		devices[d.ID] = d
	}
	out := make([]sessionMember, 0, len(a.snap.ActiveTargets))
	for _, id := range a.snap.ActiveTargets {
		d := devices[id]
		t, has := a.snap.Target(id)
		if d.ID == "" {
			// A member whose row is not in the visible list still belongs to
			// the session; its published name keeps the identity readable.
			name := t.Name
			if name == "" {
				name = id
			}
			d = device.Device{ID: id, Name: name, Online: true}
		}
		out = append(out, sessionMember{Device: d, Target: t, HasTarget: has})
	}
	return out
}

// sessionName names the whole session. It prefers the engine's joined name and
// otherwise joins the member names, so the connection page and the footer say
// the same thing while a connect is still opening.
func (a *App) sessionName() string {
	if a.snap.ConnectedName != "" {
		return a.snap.ConnectedName
	}
	members := a.sessionMembers()
	if len(members) == 0 {
		return ""
	}
	names := make([]string, 0, len(members))
	for _, m := range members {
		names = append(names, m.Device.Name)
	}
	return strings.Join(names, " + ")
}

// sessionPrimary is the target the interface speaks for: the selected member
// when it is in the session, else the first one.
func (a *App) sessionPrimary() (sessionMember, bool) {
	members := a.sessionMembers()
	if len(members) == 0 {
		return sessionMember{}, false
	}
	for _, m := range members {
		if m.Device.ID == a.snap.SelectedID || m.Device.ID == a.selID {
			return m, true
		}
	}
	return members[0], true
}

// sessionLatencyMS is the effective latency of the slowest session member.
// Every member gets its own base plus DSP/DAC delay, so one number for the
// session is the slowest of them.
func (a *App) sessionLatencyMS() int {
	base := a.latencyMS()
	slowest := base
	for _, m := range a.sessionMembers() {
		delay, ok := a.receiverDelay(m.Device.ID)
		if !ok {
			continue
		}
		if ms := receivers.EffectiveLatencyMS(base, delay); ms > slowest {
			slowest = ms
		}
	}
	return slowest
}
