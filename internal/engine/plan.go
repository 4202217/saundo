package engine

import (
	"errors"
	"fmt"
	"strings"

	"saundo/internal/airplay/creds"
	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/receivers"
)

// sessionPlan is the immutable description of one session: the targets the
// user selected, every sender that serves them, and the one clock role they
// share. Build it with buildPlan and never mutate it afterwards. The zero
// value is no session. The plan carries member credentials; never log it.
type sessionPlan struct {
	targets []plannedTarget
	// members is flat, in target order then member order. A target addresses
	// its window with first/count, so member data has one home.
	members []plannedMember
	clock   clockPlan
	// joinKey is the target the user's connect started or added. It stays set
	// until the cohort streams, so the flyout can offer cancel on that row
	// alone; a rebuild carries it only while that target stays planned.
	joinKey string
}

// plannedTarget is one selected target: a single receiver, a remembered
// stereo pair whose two senders are its members, or a stereo combo whose two
// member receivers play one input side each.
type plannedTarget struct {
	key       string // registry key: deviceid, "stereo:<tsid>", "combo:<hex>", or "m:<uuid>"
	name      string // display name; the pair's or combo's name for a stereo target
	groupName string // AirPlay group name for diagnostics; "" when none
	stereo    bool
	// combo marks a user-created stereo combo: two independent receivers, so
	// no group UUID or SETPEERS travels to them, each member plays its own
	// input side, and each member times against its own grandmaster instead
	// of the sender.
	combo bool
	ptp   bool // this target's receivers need a sender-side PTP clock
	first int  // window into sessionPlan.members
	count int
	// volume is the target's own volume in percent. volumeSet false means the
	// user never set one, so the receiver's reported initial volume is adopted
	// when the session launches.
	volume    float64
	volumeSet bool
	// channel is the target's stereo map. A stereo pair always plays stereo.
	channel sender.ChannelMode
}

// plannedMember is one sender's immutable configuration.
type plannedMember struct {
	// key is the registry key whose settings govern this member: the target
	// key. A pair's two members share the pair's key because the registry
	// stores one delay per pair; a per-member delay would change only
	// planTarget, not the transport.
	key string
	// targetKey is the target this member belongs to; the volume and channel
	// callbacks address it, so a pair shares one value.
	targetKey  string
	device     device.Device
	auth       memberAuth
	latencyMS  int    // Engine.latencyFor(key) at plan build
	duplicates int    // receivers.DuplicatesFor(owning entry)
	groupUUID  string // pair tsid sent as groupUUID; "" for a standalone receiver
	volume     float64
	volumeSet  bool
	channel    sender.ChannelMode
	// timingPeers is the pair's own member hosts for SETPEERS. Empty for a
	// standalone receiver; SETPEERS is a no-op on a PTP session and an NTP
	// receiver ignores it.
	timingPeers []string
	// ptpTimed is true when this member's target needs PTP timing. A cohort
	// may mix timed and NTP members; only timed members get a clock.
	ptpTimed bool
}

// memberAuth is everything a sender needs to reach its receiver before Start.
// It replaces the SetAuth, SetFamily, and SetMemberAuth setters: a transport
// cannot be constructed without it, and one path carries pair and single auth.
// The credential and password fields are creds.Secret, so a format verb prints
// [redacted] instead of leaking key material.
type memberAuth struct {
	method    device.Auth
	airplay2  bool
	deviceID  string
	credsJSON creds.Secret
	password  creds.Secret
	family    device.Family
}

// clockPlan is the cohort's PTP role. At most one role family per session: the
// follower set and the master both bind UDP 319/320. A follower set serves any
// number of standalone PTP receivers over one shared socket pair. The zero
// value is "every member uses NTP".
type clockPlan struct {
	role          ptpRole
	followerHosts []string // ptpFollowers: one host per standalone PTP receiver
	masterPeers   []string // ptpMaster: one host per timed pair member
}

type ptpRole uint8

const (
	ptpNone ptpRole = iota
	ptpFollowers
	ptpMaster
)

// planError is a fail-closed plan rejection carrying the catalog key the UI
// resolves. buildPlan and validateTarget return it before any transport opens.
type planError struct {
	key  UserErrorKey
	args []string
}

func (e *planError) Error() string { return string(e.key) }

// errAwaitInput reports that the plan needs a credential the user must supply;
// the prompt is already set. It is not a failure and must not stop a session.
var errAwaitInput = errors.New("awaiting receiver input")

func (p sessionPlan) live() bool { return len(p.targets) > 0 }

// target returns the plan's target for a key.
func (p sessionPlan) target(key string) (plannedTarget, bool) {
	for _, t := range p.targets {
		if t.key == key {
			return t, true
		}
	}
	return plannedTarget{}, false
}

// keys returns the plan's target keys in plan order; the first is primary.
func (p sessionPlan) keys() []string {
	keys := make([]string, len(p.targets))
	for i, t := range p.targets {
		keys[i] = t.key
	}
	return keys
}

func (p sessionPlan) has(key string) bool {
	for _, t := range p.targets {
		if t.key == key {
			return true
		}
	}
	return false
}

// devices returns every member's device in plan order, for credential pinning.
func (p sessionPlan) devices() []device.Device {
	out := make([]device.Device, len(p.members))
	for i, m := range p.members {
		out[i] = m.device
	}
	return out
}

// containsAnyMember reports whether any receiver behind t is already a cohort
// member. A pair member seen under its own key is the common case; it makes
// such a Connect an idempotent no-op instead of a duplicate stream.
func (p sessionPlan) containsAnyMember(t receivers.Target) bool {
	d := t.Device
	if d.IsStereoGroup() {
		for _, m := range d.Members {
			if p.containsDevice(m) {
				return true
			}
		}
		return false
	}
	return p.containsDevice(d)
}

func (p sessionPlan) containsDevice(d device.Device) bool {
	for _, m := range p.members {
		if d.ID != "" && m.device.ID == d.ID {
			return true
		}
		if d.Host != "" && m.device.Host == d.Host && m.device.Port == d.Port {
			return true
		}
	}
	return false
}

// connectedName is the status line's name: one target keeps its own name;
// several join with " + ", the separator the capture label already uses.
func (p sessionPlan) connectedName() string {
	names := make([]string, len(p.targets))
	for i, t := range p.targets {
		names[i] = t.name
	}
	return strings.Join(names, " + ")
}

// targetMembers returns one target's window into the flat member list.
func (p sessionPlan) targetMembers(i int) []plannedMember {
	t := p.targets[i]
	return p.members[t.first : t.first+t.count]
}

// buildPlan derives a session plan from selected target keys. The engine
// goroutine is the only caller: it reads the registry, credentials, and
// settings that goroutine owns. It is the only place a session's shape is
// decided, so one target and many cannot diverge.
func (e *Engine) buildPlan(keys []string) (sessionPlan, error) {
	p := sessionPlan{}
	// seen rejects the same receiver entering twice under two keys (a single
	// row plus a pair that contains it): one host cannot run two streams.
	seen := map[string]string{} // member device ID or host:port -> target key
	for _, key := range keys {
		t, ok := e.target(key)
		if !ok {
			return sessionPlan{}, &planError{key: ErrDeviceMissing}
		}
		t.Device = e.pinGroupMembers(t.Device) // paired endpoints win
		target, members, err := e.planTarget(t, len(p.members), seen)
		if err != nil {
			return sessionPlan{}, err
		}
		p.targets = append(p.targets, target)
		p.members = append(p.members, members...)
	}
	clock, err := clockPlanFor(p)
	if err != nil {
		return sessionPlan{}, err
	}
	p.clock = clock
	return p, nil
}

// planTarget turns one merged target into plan data: one member for a single
// receiver, the pair's two members for a stereo group or combo. first is the
// offset the target's window will occupy in the plan's flat member list. A
// remembered pair's two members carry the pair's groupUUID and the pair's two
// hosts as timing peers. A combo's two members are independent receivers:
// no groupUUID, no timing peers, one input side each, and each member times
// against its own grandmaster. The registry stores one delay per stereo
// target, so all members resolve latency through the target's key.
func (e *Engine) planTarget(t receivers.Target, first int, seen map[string]string) (plannedTarget, []plannedMember, error) {
	d := t.Device
	if d.Name == "" {
		d.Name = t.Entry.DisplayName()
	}
	entry, _ := e.reg.Get(t.Entry.Key)
	combo := t.Entry.IsCombo()
	target := plannedTarget{
		key:       t.Entry.Key,
		name:      d.Name,
		groupName: d.GroupName,
		stereo:    d.IsStereoGroup(),
		combo:     combo,
		ptp:       usesPTP(d),
		first:     first,
		channel:   sender.ParseChannelMode(entry.Channel),
	}
	target.volume, target.volumeSet = e.setsValue.VolumeByDevice[t.Entry.Key]
	if target.stereo {
		target.channel = sender.ChannelStereo
	}
	latency := e.latencyFor(t.Entry.Key)
	duplicates := receivers.DuplicatesFor(entry)
	authFor := func(m device.Device) memberAuth {
		return memberAuth{
			method:    m.Auth,
			airplay2:  m.Auth.IsHAP(),
			deviceID:  m.ID,
			credsJSON: creds.Secret(e.creds.Load(m.ID)),
			password:  creds.Secret(e.memberPassword(m)),
			family:    m.Family,
		}
	}
	if target.stereo {
		if len(d.Members) != 2 {
			return plannedTarget{}, nil, &planError{key: ErrStereoMembersOffline}
		}
		peers := make([]string, 0, len(d.Members))
		for _, m := range d.Members {
			peers = append(peers, m.Host)
		}
		// A combo is not a native pair: its members learn nothing about each
		// other, so neither the group UUID nor SETPEERS travels to them.
		groupUUID, timingPeers := d.StereoID, peers
		if combo {
			groupUUID, timingPeers = "", nil
		}
		members := make([]plannedMember, 0, len(d.Members))
		for i, m := range d.Members {
			if m.Host == "" || m.Port == 0 {
				return plannedTarget{}, nil, addressless(m)
			}
			if err := seenMember(seen, m, t.Entry.Key); err != nil {
				return plannedTarget{}, nil, err
			}
			mode, ptpTimed := target.channel, target.ptp
			if combo {
				mode = sender.ChannelLeft
				if i == 1 {
					mode = sender.ChannelRight
				}
				ptpTimed = usesPTP(m)
				if ptpTimed {
					target.ptp = true
				}
			}
			members = append(members, plannedMember{
				key:         t.Entry.Key,
				targetKey:   t.Entry.Key,
				device:      m,
				auth:        authFor(m),
				latencyMS:   latency,
				duplicates:  duplicates,
				groupUUID:   groupUUID,
				timingPeers: timingPeers,
				ptpTimed:    ptpTimed,
				volume:      target.volume,
				volumeSet:   target.volumeSet,
				channel:     mode,
			})
		}
		target.count = len(members)
		return target, members, nil
	}
	if d.Host == "" || d.Port == 0 {
		return plannedTarget{}, nil, addressless(d)
	}
	if err := seenMember(seen, d, t.Entry.Key); err != nil {
		return plannedTarget{}, nil, err
	}
	m := plannedMember{
		key:         t.Entry.Key,
		targetKey:   t.Entry.Key,
		device:      d,
		auth:        authFor(d),
		latencyMS:   latency,
		duplicates:  duplicates,
		groupUUID:   d.StereoID,
		timingPeers: nil,
		ptpTimed:    target.ptp,
		volume:      target.volume,
		volumeSet:   target.volumeSet,
		channel:     target.channel,
	}
	target.count = 1
	return target, []plannedMember{m}, nil
}

// addressless rejects a target the transport would dial as :0.
func addressless(d device.Device) error {
	return &planError{key: ErrSessionOpen, args: []string{fmt.Sprintf("receiver %q has no address", d.Name)}}
}

// seenMember rejects the same receiver entering a plan twice. Both identities
// matter: the same device id under two keys, and the same endpoint under a
// single row and a pair member.
func seenMember(seen map[string]string, d device.Device, key string) error {
	// The endpoint enters under its host alone. The cohort routes host events
	// by peer address, so two members on one host at different ports cannot be
	// told apart even though each dials a distinct port.
	ids := []string{"host:" + d.Host}
	if d.ID != "" {
		ids = append(ids, "id:"+d.ID)
	}
	for _, id := range ids {
		if prev, ok := seen[id]; ok {
			return &planError{key: ErrSessionOpen, args: []string{
				fmt.Sprintf("%s already plays in this session as %s", d.Name, prev),
			}}
		}
	}
	for _, id := range ids {
		seen[id] = key
	}
	return nil
}

// clockPlanFor derives the one PTP role the plan's targets demand. A standalone
// PTP receiver joins a follower set, which serves any number of them over one
// shared socket pair; a combo's PTP members each keep their own grandmaster,
// so both hosts join the same follower set. A remembered stereo pair needs the
// sender as grandmaster. Both role families bind UDP 319/320, so a follower
// set beside a master cannot run and is rejected here, before a transport
// opens. The returned error is always a *planError when non-nil.
func clockPlanFor(p sessionPlan) (clockPlan, error) {
	var (
		followerHosts []string
		followerName  string
		masterPeers   []string
		masterName    string
	)
	for i, t := range p.targets {
		if !t.ptp {
			continue // NTP receiver: needs no role
		}
		members := p.targetMembers(i)
		if t.stereo && !t.combo {
			for _, m := range members {
				masterPeers = append(masterPeers, m.device.Host)
			}
			if masterName == "" {
				masterName = t.name
			}
			continue
		}
		if followerName == "" {
			followerName = t.name
		}
		for _, m := range members {
			if m.ptpTimed {
				followerHosts = append(followerHosts, m.device.Host)
			}
		}
	}
	switch {
	case len(followerHosts) > 0 && len(masterPeers) > 0:
		// A follower set and a master would bind the same UDP ports twice.
		// Fail closed and name one target from each family.
		return clockPlan{}, &planError{key: ErrClockConflict, args: []string{followerName, masterName}}
	case len(masterPeers) > 0:
		return clockPlan{role: ptpMaster, masterPeers: masterPeers}, nil
	case len(followerHosts) > 0:
		return clockPlan{role: ptpFollowers, followerHosts: followerHosts}, nil
	}
	return clockPlan{}, nil
}

// connectAction is what a Connect means for the current session.
type connectAction uint8

const (
	// connectNoop is a click on a target already in the live cohort.
	connectNoop connectAction = iota
	// connectStart starts a one-target session while idle.
	connectStart
	// connectAdd adds a target to a streaming cohort and rebuilds it.
	connectAdd
	// connectReplace replaces a cohort that is still handshaking, the
	// pre-existing switch behavior while a connection is in flight.
	connectReplace
	// connectRetry retries every wanted target after a failure.
	connectRetry
)

// connectIntent is the whole Connect policy. A key already in a live cohort is
// a no-op; a new key joins a streaming cohort or replaces one that has not
// started yet; with no live cohort, a key in the wanted set retries every
// wanted target and a new key starts a one-target session.
func (e *Engine) connectIntent(key string) (connectAction, []string) {
	live := e.plan.live() && e.tr != nil && e.tr.State() != sender.StateIdle
	if live {
		if e.plan.has(key) {
			return connectNoop, nil
		}
		if e.tr.State() == sender.StateStreaming {
			return connectAdd, append(e.plan.keys(), key)
		}
		return connectReplace, []string{key}
	}
	for _, k := range e.wantedKeys {
		if k == key {
			return connectRetry, append([]string(nil), e.wantedKeys...)
		}
	}
	return connectStart, []string{key}
}

// activeKeys is the row-visibility and action set: the live plan's keys while a
// transport exists, the wanted keys while a failure or retry waits. The first
// key is the primary target.
func (e *Engine) activeKeys() []string {
	if e.tr != nil {
		return e.plan.keys()
	}
	if len(e.wantedKeys) > 0 {
		return append([]string(nil), e.wantedKeys...)
	}
	return nil
}

// pruneWanted drops wanted keys that no longer resolve, with the reconnect
// drop line. Survivors keep their order and the caller's set is untouched.
func (e *Engine) pruneWanted(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, ok := e.target(key); ok {
			out = append(out, key)
			continue
		}
		e.logf("reconnect: dropping %s; it is no longer discovered", key)
	}
	return out
}

// validateWanted applies the command guards to every key. The first failing
// key is returned with its error; errAwaitInput means the prompt is already
// set. A key that no longer resolves is skipped, as pruneWanted already
// reported it.
func (e *Engine) validateWanted(keys []string) (string, error) {
	for _, key := range keys {
		t, ok := e.target(key)
		if !ok {
			continue
		}
		if err := e.validateTarget(t); err != nil {
			return key, err
		}
	}
	return "", nil
}

// validateTarget applies command-time guards: local receiver, stereo members
// structural, legacy PIN, pinned endpoint, receiver password. A missing
// password sets the prompt and returns errAwaitInput. A failure returns a
// *planError for failPlan; it never runs for an already-running target.
func (e *Engine) validateTarget(t receivers.Target) error {
	d := t.Device
	// A receiver on this host cannot be a target: the sender and receiver
	// would share one audio path, and the receiver refuses the session anyway.
	if d.Local {
		e.logf("refusing the local receiver %s", d.Name)
		return &planError{key: ErrLocalReceiver}
	}
	if d.IsStereoGroup() && (!d.Online || len(d.Members) != 2) {
		return &planError{key: ErrStereoMembersOffline}
	}
	if d.Auth == device.AuthLegacyPin {
		e.logf("FairPlay receiver %s is not supported", d.Name)
		return &planError{key: ErrFairPlay, args: []string{d.Name}}
	}
	// A device id with stored credentials is pinned to the endpoint it was
	// paired with. Discovery cannot authenticate an advertisement, so a
	// different advertised address is refused before any key material is
	// handed to the sender.
	if entry, ok := e.reg.Get(d.ID); ok && entry.PairedHost != "" && entry.PairedPort != 0 &&
		(d.Host != entry.PairedHost || d.Port != entry.PairedPort) {
		e.logf("refusing %s: advertised %s:%d, paired %s:%d", d.Name, d.Host, d.Port, entry.PairedHost, entry.PairedPort)
		return &planError{key: ErrEndpointMoved, args: []string{d.Name}}
	}
	if d.IsStereoGroup() {
		// A member that needs a receiver password and has none would burn the
		// reconnect budget on 401 digest challenges; prompt for it the same way
		// a single receiver would.
		for _, m := range d.Members {
			// memberPassword falls back to the pair's own key, so a password
			// stored once for both members already satisfies this member.
			if !needsReceiverPassword(m.Auth) || e.memberPassword(m) != "" {
				continue
			}
			e.passwordRequired = true
			e.passwordDevice = m.Name
			e.passwordDeviceID = m.ID
			e.passwordTargetID = t.Entry.Key
			e.logf("AirPlay password required for %s (stereo member)", m.Name)
			return errAwaitInput
		}
		return nil
	}
	if !needsReceiverPassword(d.Auth) {
		return nil
	}
	if password := e.passwordFor(d.ID); password != "" {
		return nil
	}
	// No stored password, or one the DPAPI file could not decrypt: prompt
	// instead of attempting the permissive path.
	if e.hasPassword(d.ID) {
		e.logf("stored receiver password for %s is unreadable; prompting again", d.ID)
	}
	e.passwordRequired = true
	e.passwordDevice = d.Name
	e.passwordDeviceID = d.ID
	e.passwordTargetID = d.ID
	e.logf("AirPlay password required for %s", d.Name)
	return errAwaitInput
}

// failPlan maps a plan error to its catalog key.
func (e *Engine) failPlan(err error) {
	var pe *planError
	if errors.As(err, &pe) {
		e.fail(pe.key, pe.args...)
		return
	}
	e.fail(ErrSessionOpen, err.Error())
}

// ptpFeatureBit is the receiver feature that advertises PTP support.
const ptpFeatureBit = 1 << 41

// usesPTP reports whether the receiver needs the sender as a PTP peer. HomePod
// software 27 and macOS AirPlay receivers advertise the PTP bit and silently
// ignore an NTP-timed stream; both use the same follower.
func usesPTP(d device.Device) bool {
	if !d.Auth.IsHAP() {
		return false
	}
	if !strings.HasPrefix(d.Model, "AudioAccessory") && !strings.HasPrefix(d.Model, "Mac") {
		return false
	}
	// An unknown features value fails closed: these families keep their own
	// PTP grandmaster, and an NTP-timed stream is silently ignored.
	return !d.Features.Known() || d.Features.Has(ptpFeatureBit)
}
