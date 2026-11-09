package receivers

import "saundo/internal/airplay/device"

// View is the settings-facing row: everything the receiver page renders,
// and no secret — only whether one exists.
type View struct {
	Key         string
	Name        string
	Kind        Kind
	Type        Type
	Address     string
	Model       string
	Online      bool
	Hidden      bool
	Local       bool
	AutoConnect bool
	DelayMS     int
	Stereo      bool
	PasswordSet bool
	// Device is the merged wire description behind the row, for the
	// subtitle and icons.
	Device device.Device
	// Entry is the row's stored configuration. It carries no secret; the
	// password exists only as the PasswordSet bit.
	Entry Entry
}

// View projects a target onto the settings list.
func (t Target) View(hasPassword bool) View {
	e := t.Entry
	v := View{
		Key:         e.Key,
		Name:        e.DisplayName(),
		Kind:        e.Kind,
		Type:        e.Type,
		Model:       t.Device.Model,
		Online:      t.Device.Online,
		Hidden:      e.Hidden,
		Local:       t.Device.Local,
		AutoConnect: e.AutoConnect,
		DelayMS:     e.DelayMS,
		Stereo:      t.Device.IsStereoGroup() || e.IsGroup(),
		PasswordSet: hasPassword,
		Device:      t.Device,
		Entry:       e,
	}
	if e.IsManual() {
		v.Address = e.Host + ":" + PortText(e.Port)
	}
	return v
}

// IsManual reports whether the row is a manually added receiver, which may
// be edited and removed.
func (v View) IsManual() bool { return v.Kind == KindManual }
