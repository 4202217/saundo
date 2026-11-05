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
	// PasswordRequired is the `pw` TXT value.
	PasswordRequired bool
	// AirPlay2 is true when the device advertises an AirPlay 2 service.
	AirPlay2 bool
	// Auth is the pairing path chosen from the advertisement.
	Auth Auth
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
