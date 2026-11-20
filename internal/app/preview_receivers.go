//go:build preview

package app

import (
	"strconv"

	"saundo/internal/airplay/device"
	"saundo/internal/airplay/sender"
	"saundo/internal/receivers"
)

// previewReceiverAdd is the fake engine's AddReceiver: it appends the row and
// the wire device the flyout also shows.
func previewReceiverAdd(a *App, spec receivers.Spec) {
	key := "m:" + spec.Host + ":" + strconv.Itoa(int(spec.Port))
	model := string(spec.Type)
	auth := device.AuthNone
	if spec.Type == receivers.TypeAirPlay2 {
		auth = device.AuthHAPTransient
	}
	if spec.Password != "" {
		auth = device.AuthHAPPassword
	}
	d := device.Device{
		ID:               key,
		Name:             spec.Name,
		Host:             spec.Host,
		Port:             spec.Port,
		Model:            model,
		AirPlay2:         spec.Type == receivers.TypeAirPlay2,
		Auth:             auth,
		Online:           true,
		PasswordRequired: spec.Password != "",
	}
	v := receivers.View{
		Key:         key,
		Name:        spec.Name,
		Kind:        receivers.KindManual,
		Type:        spec.Type,
		Address:     spec.Host + ":" + strconv.Itoa(int(spec.Port)),
		Model:       model,
		Online:      true,
		PasswordSet: spec.Password != "",
		Device:      d,
		Entry: receivers.Entry{
			Key:      key,
			Kind:     receivers.KindManual,
			Type:     spec.Type,
			Name:     spec.Name,
			Host:     spec.Host,
			Port:     spec.Port,
			Model:    model,
			AirPlay2: spec.Type == receivers.TypeAirPlay2,
			Auth:     auth,
		},
	}
	a.snap.Receivers = append(a.snap.Receivers, v)
	a.snap.Devices = append(a.snap.Devices, d)
}

func previewReceiverFind(a *App, key string) *receivers.View {
	for i := range a.snap.Receivers {
		if a.snap.Receivers[i].Key == key {
			return &a.snap.Receivers[i]
		}
	}
	return nil
}

func previewReceiverUpdate(a *App, key string, patch receivers.Patch) {
	v := previewReceiverFind(a, key)
	if v == nil {
		return
	}
	if patch.Name != nil {
		v.Name = *patch.Name
		v.Device.Name = *patch.Name
		v.Entry.Name = *patch.Name
	}
	if patch.Host != nil {
		v.Device.Host = *patch.Host
		v.Entry.Host = *patch.Host
	}
	if patch.Port != nil {
		v.Device.Port = *patch.Port
		v.Entry.Port = *patch.Port
	}
	if patch.Type != nil {
		v.Type = *patch.Type
		v.Entry.Type = *patch.Type
		v.Device.AirPlay2 = *patch.Type == receivers.TypeAirPlay2
	}
	if patch.Hidden != nil {
		v.Hidden = *patch.Hidden
		v.Entry.Hidden = *patch.Hidden
	}
	if patch.AutoConnect != nil {
		v.AutoConnect = *patch.AutoConnect
		v.Entry.AutoConnect = *patch.AutoConnect
	}
	if patch.DelayMS != nil {
		v.DelayMS = *patch.DelayMS
		v.Entry.DelayMS = *patch.DelayMS
	}
	if patch.Password != nil {
		v.PasswordSet = *patch.Password != ""
		v.Device.PasswordRequired = v.PasswordSet
		if v.PasswordSet {
			v.Device.Auth = device.AuthHAPPassword
			v.Entry.Auth = device.AuthHAPPassword
		}
	}
	v.Address = v.Device.Host + ":" + strconv.Itoa(int(v.Device.Port))
	previewReceiverSync(a, key, *v)
	if patch.Hidden != nil {
		previewReceiverVisible(a)
	}
}

func previewReceiverRemove(a *App, key string) {
	rows := a.snap.Receivers[:0]
	for _, v := range a.snap.Receivers {
		if v.Key != key {
			rows = append(rows, v)
		}
	}
	a.snap.Receivers = rows
	devices := a.snap.Devices[:0]
	for _, d := range a.snap.Devices {
		if d.ID != key {
			devices = append(devices, d)
		}
	}
	a.snap.Devices = devices
	if a.snap.SelectedID == key {
		a.snap.SelectedID = ""
		a.selID = ""
		a.snap.ConnectedName = ""
		a.snap.State = sender.StateIdle
		a.snap.Error = ""
	}
}

func previewReceiverPassword(a *App, key, password string) {
	v := previewReceiverFind(a, key)
	if v == nil {
		return
	}
	v.PasswordSet = password != ""
	v.Device.PasswordRequired = v.PasswordSet
	switch {
	case v.PasswordSet:
		v.Device.Auth = device.AuthHAPPassword
	case v.Device.AirPlay2:
		v.Device.Auth = device.AuthHAPTransient
	default:
		v.Device.Auth = device.AuthNone
	}
	v.Entry.Auth = v.Device.Auth
	previewReceiverSync(a, key, *v)
}

func previewReceiverHidden(a *App, key string, on bool) {
	v := previewReceiverFind(a, key)
	if v == nil {
		return
	}
	v.Hidden = on
	v.Entry.Hidden = on
	previewReceiverVisible(a)
}

// previewReceiverVisible rebuilds the flyout's list the way the engine's
// receivers.Visible does: a hidden row leaves it unless it is the active
// session's receiver.
func previewReceiverVisible(a *App) {
	devices := make([]device.Device, 0, len(a.snap.Receivers))
	for _, v := range a.snap.Receivers {
		if v.Key != a.snap.SelectedID && v.Hidden {
			continue
		}
		devices = append(devices, v.Device)
	}
	a.snap.Devices = devices
}

// previewReceiverAuto mirrors Registry.SingleAutoConnect: one target only, so
// enabling a row clears every other one.
func previewReceiverAuto(a *App, key string, on bool) {
	for i := range a.snap.Receivers {
		v := &a.snap.Receivers[i]
		switch {
		case v.Key == key:
			v.AutoConnect = on
			v.Entry.AutoConnect = on
		case on:
			v.AutoConnect = false
			v.Entry.AutoConnect = false
		}
	}
}

func previewReceiverDelay(a *App, key string, ms int) {
	if v := previewReceiverFind(a, key); v != nil {
		v.DelayMS = ms
		v.Entry.DelayMS = ms
	}
}

// previewReceiverSync writes the row's device back to the flyout's list.
func previewReceiverSync(a *App, key string, v receivers.View) {
	for i := range a.snap.Devices {
		if a.snap.Devices[i].ID == key {
			a.snap.Devices[i] = v.Device
			break
		}
	}
	if a.snap.SelectedID == key {
		a.snap.ConnectedName = v.Device.Name
	}
}
