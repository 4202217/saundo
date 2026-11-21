package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/engine"
	"saundo/internal/receivers"
)

// authPill is one receiver's authentication state for the 20-DIP chip on the
// row's title line. A password receiver says whether one is saved, or that
// the last attempt was refused; the other gates keep their own words.
type authPill struct {
	icon *ui.SVG
	text string
	tone chipTone
	ok   bool
}

// receiverAuthPill maps a receiver onto its status chip. A refused password
// outranks the saved bit: the engine's error names the receiver it could not
// open.
func (a *App) receiverAuthPill(v receivers.View) authPill {
	if a.passwordRejected(v) {
		return authPill{iconAlert, locales.DevicePasswordError(), chipDanger, true}
	}
	switch v.Device.Auth {
	case device.AuthHAPPIN:
		return authPill{iconKey, locales.DevicePinPairing(), chipNeutral, true}
	case device.AuthHAPPassword, device.AuthPassword:
		if v.PasswordSet {
			return authPill{iconKey, locales.DevicePasswordSaved(), chipNeutral, true}
		}
		return authPill{iconLock, locales.DeviceNeedsPassword(), chipAccent, true}
	case device.AuthSetup:
		return authPill{iconAlert, locales.DeviceAuthSetup(), chipNeutral, true}
	case device.AuthLegacyPin:
		return authPill{iconAlert, locales.DeviceUnsupported(), chipDanger, true}
	}
	return authPill{}
}

// passwordRejected reports whether the engine's last error is a refused
// password for this receiver. The error lives on the snapshot, so the red
// chip clears on the next connect attempt or a success.
func (a *App) passwordRejected(v receivers.View) bool {
	if a.snap.ErrorKey != engine.ErrPasswordRejected {
		return false
	}
	id := a.snap.PasswordDeviceID
	return a.snap.PasswordTargetID == v.Key || id == v.Key || id == v.Device.ID
}

// receiverOnlineChip is the stereo group's online count, left of the name:
// a dot and the member count, green when every member is online and red when
// one is down.
func (a *App) receiverOnlineChip(c *ui.Context, p palette, v receivers.View) {
	members := v.Device.Members
	if len(members) == 0 {
		return
	}
	online := 0
	for _, m := range members {
		if m.Online {
			online++
		}
	}
	label := locales.DeviceComboOnline(online)
	if online < len(members) {
		label = locales.DeviceComboOffline(len(members) - online)
	}
	countChip(c, p, fmt.Sprintf("%d/%d", online, len(members)), label, online == len(members), v.Hidden)
}

// receiverDetail is the settings row's second line: the address, the channel,
// the model and the system version. A stereo group starts at the model and
// lists both members' addresses; a combo lists its members in playback order.
func receiverDetail(v receivers.View) string {
	if v.Entry.IsCombo() {
		return comboDetail(v.Name, v.Device.Members)
	}
	if v.Stereo {
		// The glyph and the online count already say this is a stereo group;
		// the line names only the model and the two members' addresses.
		parts := make([]string, 0, len(v.Device.Members)+1)
		if model := stereoModelLabel(v); model != "" {
			parts = append(parts, model)
		}
		for _, m := range v.Device.Members {
			if addr := memberAddress(m); addr != "" {
				parts = append(parts, addr)
			}
		}
		return strings.Join(parts, " · ")
	}
	parts := make([]string, 0, 4)
	if addr := receiverAddress(v); addr != "" {
		parts = append(parts, addr)
	}
	parts = append(parts, channelFull(v.Channel))
	if model := receiverModelLabel(v); model != "" {
		parts = append(parts, model)
	}
	if os := receiverOSVersion(v); os != "" {
		parts = append(parts, os)
	}
	return strings.Join(parts, " · ")
}

// flyoutDetail is the flyout row's second line: the same facts without the
// address. A combo still lists its members in playback order.
func flyoutDetail(v receivers.View) string {
	if v.Entry.IsCombo() {
		return comboDetail(v.Name, v.Device.Members)
	}
	parts := make([]string, 0, 3)
	if !v.Stereo {
		parts = append(parts, channelFull(v.Channel))
	}
	if model := receiverModelLabel(v); model != "" {
		parts = append(parts, model)
	}
	if os := receiverOSVersion(v); os != "" {
		parts = append(parts, os)
	}
	return strings.Join(parts, " · ")
}

// comboDetail names a combo's two sides in playback order. A name that
// already carries both members is never repeated.
func comboDetail(name string, members []device.Device) string {
	names := make([]string, 0, len(members))
	for _, m := range members {
		if m.Name != "" {
			names = append(names, m.Name)
		}
	}
	out := strings.Join(names, " · ")
	if out == name {
		return ""
	}
	return out
}

func receiverAddress(v receivers.View) string {
	if v.Address != "" {
		return v.Address
	}
	if v.Device.Host != "" {
		return hostPort(v.Device.Host, v.Device.Port)
	}
	if v.Entry.Host != "" {
		return hostPort(v.Entry.Host, v.Entry.Port)
	}
	return ""
}

func memberAddress(m device.Device) string {
	if m.Host == "" {
		return ""
	}
	return hostPort(m.Host, m.Port)
}

func hostPort(host string, port uint16) string {
	return host + ":" + receivers.PortText(port)
}

// channelFull is the row's own channel word: 立体声, 左声道, or 右声道.
func channelFull(channel string) string {
	switch channel {
	case receivers.ChannelLeft:
		return locales.ComboFieldLeft()
	case receivers.ChannelRight:
		return locales.ComboFieldRight()
	}
	return locales.ChannelStereo()
}

// receiverModelLabel names the receiver's model for the detail line. A known
// HomePod, Apple TV, or Mac identifier gets its marketing name; an unknown
// identifier falls back to the family. A manual row carries no advertised
// family, so only the Express type has one worth naming.
func receiverModelLabel(v receivers.View) string {
	if v.IsManual() {
		if v.Type == receivers.TypeAirPortExpress {
			return locales.ReceiverTypeExpress()
		}
		return ""
	}
	if model := modelNameFor(v.Device.Model); model != "" {
		return model
	}
	switch v.Device.Family {
	case device.FamilyHomePod:
		return locales.DeviceFamilyHomePod()
	case device.FamilyMac:
		return locales.DeviceFamilyMac()
	case device.FamilyAppleTV:
		return locales.DeviceFamilyAppleTV()
	case device.FamilyAirPort:
		return locales.ReceiverTypeExpress()
	case device.FamilyOther:
		return locales.DeviceFamilyOther()
	}
	return ""
}

// stereoModelLabel names a stereo group's model when both members agree. A
// mixed pair or an unknown identifier falls back to the group's own model,
// then to the family.
func stereoModelLabel(v receivers.View) string {
	first, known := "", 0
	for _, m := range v.Device.Members {
		name := modelNameFor(m.Model)
		if name == "" {
			continue
		}
		known++
		if first == "" {
			first = name
		} else if name != first {
			return receiverModelLabel(v)
		}
	}
	if first != "" && known == len(v.Device.Members) {
		return first
	}
	return receiverModelLabel(v)
}

// modelNameFor maps a known HomePod, Apple TV, or Mac identifier to its
// marketing name; an unknown identifier falls through to the family.
func modelNameFor(model string) string {
	if name := device.HomePodModelName(model); name != "" {
		return name
	}
	if name := device.AppleTVModelName(model); name != "" {
		return name
	}
	return device.MacModelName(model)
}

func receiverOSVersion(v receivers.View) string {
	if v.Device.OSVersion != "" {
		return v.Device.OSVersion
	}
	return v.Entry.OSVersion
}
