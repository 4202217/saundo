package app

import (
	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
)

// deviceSubtitle is the flyout row's second line: the model identifier plus
// the receiver's stereo or authentication label. The flyout is the one
// surface that shows a model identifier; the settings page never does.
func deviceSubtitle(d device.Device) string {
	model := d.Model
	if model == "" {
		model = locales.DeviceDefaultModel()
	}
	line := func(label string) string { return model + " · " + label }
	if d.IsStereoGroup() {
		return line(locales.DeviceStereoGroup(len(d.Members)))
	}
	switch d.Auth {
	case device.AuthHAPPIN:
		return line(locales.DevicePinPairing())
	case device.AuthHAPPassword, device.AuthPassword:
		return line(locales.DeviceNeedsPassword())
	case device.AuthLegacyPin:
		return line(locales.DeviceUnsupported())
	case device.AuthSetup:
		return line(locales.DeviceAuthSetup())
	}
	if d.AirPlay2 {
		return line(locales.DeviceAirPlay2())
	}
	return line(locales.DeviceAirPlay1())
}

// authSubtitle is one receiver's authentication gate in the row's own words.
// A password receiver only says whether one is saved; whether the receiver
// accepts it belongs to the password dialog and the error bar.
func authSubtitle(auth device.Auth, passwordSet bool) string {
	switch auth {
	case device.AuthHAPPIN:
		return locales.DevicePinPairing()
	case device.AuthHAPPassword, device.AuthPassword:
		if passwordSet {
			return locales.DevicePasswordSaved()
		}
		return locales.DeviceNeedsPassword()
	case device.AuthSetup:
		return locales.DeviceAuthSetup()
	case device.AuthLegacyPin:
		return locales.DeviceUnsupported()
	}
	return ""
}
