// Package device defines the receiver description shared by discovery, the
// sender, and the UI.
package device

import (
	"strconv"
	"strings"
)

// Auth is how a receiver wants its sender authenticated. The values follow the
// airplay2-sender-cpp recipe.
type Auth uint8

const (
	AuthNone Auth = iota
	AuthPassword
	AuthSetup
	AuthLegacyPin
	AuthHAPTransient
	AuthHAPPIN
	AuthHAPPassword
)

func (a Auth) String() string {
	switch a {
	case AuthPassword:
		return "password"
	case AuthSetup:
		return "auth-setup"
	case AuthLegacyPin:
		return "legacy-pin"
	case AuthHAPTransient:
		return "hap-transient"
	case AuthHAPPIN:
		return "hap-pin"
	case AuthHAPPassword:
		return "hap-password"
	default:
		return "none"
	}
}

// IsHAP reports whether the pairing runs the HomeKit Accessory Protocol.
// Every HAP pairing speaks AirPlay 2; the legacy paths do not.
func (a Auth) IsHAP() bool {
	switch a {
	case AuthHAPTransient, AuthHAPPIN, AuthHAPPassword:
		return true
	default:
		return false
	}
}

// Family is the receiver family. It is independent of Auth: a HomePod can
// pair transiently or with a password and keeps the HomePod key and latency
// profile. A Mac is the only family that uses the Events-Write audio key and
// the wide latency range.
type Family uint8

const (
	FamilyUnknown Family = iota
	FamilyHomePod
	FamilyMac
	FamilyAppleTV
	FamilyAirPort
	FamilyOther
)

func (f Family) String() string {
	switch f {
	case FamilyHomePod:
		return "homepod"
	case FamilyMac:
		return "mac"
	case FamilyAppleTV:
		return "appletv"
	case FamilyAirPort:
		return "airport"
	case FamilyOther:
		return "other"
	default:
		return "unknown"
	}
}

// FamilyFor derives the receiver family from the `model` and `am` TXT values.
// A missing model is unknown, not a guessed family; the pairing method is a
// separate axis and never decides this.
func FamilyFor(model, am string) Family {
	m := strings.TrimSpace(model)
	am = strings.TrimSpace(am)
	if m == "" {
		m = am
	}
	switch {
	case strings.HasPrefix(am, "AirPort"), strings.HasPrefix(m, "AirPort"):
		return FamilyAirPort
	case strings.HasPrefix(m, "AudioAccessory"):
		return FamilyHomePod
	case strings.HasPrefix(m, "Mac"):
		return FamilyMac
	case strings.HasPrefix(m, "AppleTV"):
		return FamilyAppleTV
	case m == "":
		return FamilyUnknown
	default:
		return FamilyOther
	}
}

// Device is one AirPlay receiver as discovery sees it.
type Device struct {
	// ID is the `deviceid` TXT value, a colon-separated MAC address.
	ID string
	// Name is the friendly name from the service instance or TXT record.
	Name string
	// Host and Port are the RAOP control endpoint.
	Host string
	Port uint16
	// Model is the `model` TXT value, for example AppleTV6,2 or AudioAccessory5,1.
	Model string
	// OSVersion is the `osvers` TXT value, for example 27.2.
	OSVersion string
	// Features and SF are the parsed `features` and `sf` bit fields.
	Features uint64
	SF       uint64
	// AM is the `am` TXT value.
	AM string
	// PasswordRequired reports that the receiver gates AirPlay with a
	// password, from `pw=true` or `sf` bit 7. `pw` is the AP1 digest gate;
	// `sf` bit 7 is the HAP gate.
	PasswordRequired bool
	// AirPlay2 is true when the device advertises an AirPlay 2 service.
	AirPlay2 bool
	// Auth is the pairing path chosen from the advertisement.
	Auth Auth
	// Family is the receiver family derived from model/am. It is orthogonal
	// to Auth: a HomePod may pair transiently or with a password.
	Family Family
	// GroupName is the AirPlay group the device belongs to, when the
	// advertisement carries one. Members of a group receive audio only when
	// the whole group is addressed.
	GroupName string
	// GroupLeader reports the `igl` TXT field: this member is the group's
	// leader. `gcgl` means the group contains a leader, not that this member
	// is one.
	GroupLeader bool
	// StereoID is the `tsid` TXT value. Members of a stereo pair share it,
	// and the stream SETUP sends it as `groupUUID`.
	StereoID string
	// Members is set only on a synthetic stereo-group device. The group
	// carries the leader's endpoint and connection fields; Members holds the
	// two physical receivers, leader first.
	Members []Device
	// Online reports whether discovery saw the device recently. Discovery
	// fills it in snapshots; a stored device reads as offline. A stereo group
	// is online only when both members are online.
	Online bool
	// Local reports that the advertised address belongs to the machine
	// running saundo. Discovery fills it in; a stored device reads as false.
	// A local receiver is never a target: the sender and its own receiver
	// share one audio path, so the stream would loop, and macOS refuses the
	// session SETUP anyway.
	Local bool
}

// IsStereoGroup reports whether d is a synthesized stereo-pair target.
func (d Device) IsStereoGroup() bool { return d.StereoID != "" && len(d.Members) > 0 }

// StereoGroupID is the stable ID of the stereo pair that carries tsid.
func StereoGroupID(tsid string) string { return "stereo:" + tsid }

// TopDisplayCapable reports whether the receiver can show the ambient-light
// artwork on its top display. The HomePod family at software 27.2 or later
// qualifies once Sound Glow is enabled on the HomePod; an unknown model or
// version, and every other family, fails closed.
func TopDisplayCapable(d Device) bool {
	if !strings.HasPrefix(d.Model, "AudioAccessory") {
		return false
	}
	major, minor, ok := parseOSVersion(d.OSVersion)
	if !ok {
		return false
	}
	return major > 27 || major == 27 && minor >= 2
}

// parseOSVersion reads the dotted major.minor an `osvers` TXT value carries.
// A missing minor or a non-numeric component is unknown, not zero.
func parseOSVersion(v string) (major, minor int, ok bool) {
	parts := strings.Split(strings.TrimSpace(v), ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return 0, 0, false
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil || minor < 0 {
		return 0, 0, false
	}
	return major, minor, true
}
