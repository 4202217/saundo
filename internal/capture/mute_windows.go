//go:build windows

package capture

import (
	"runtime"
	"unsafe"
)

// IAudioEndpointVolume, from endpointvolume.h. GetMute is vtable slot 15 and
// SetMute is slot 14, after IUnknown's three slots and the eleven methods
// before them.
var iidIAudioEndpointVolume = guid{0x5cdf2c82, 0x841e, 0x4546, [8]byte{0x97, 0x22, 0x0c, 0xf7, 0x40, 0x78, 0x22, 0x9a}}

// EndpointMuted reports the render endpoint's mute state. An empty id uses
// the default render endpoint.
//
// Muting the endpoint silences the local speakers. Whether it also silences a
// loopback capture is driver-dependent: the tap point sits upstream of the
// endpoint mute on some drivers and downstream on others. The settings copy
// warns about that; the probe under -tags muteprobe measures it on a machine.
func EndpointMuted(endpointID string) (bool, error) {
	var muted bool
	err := withEndpointVolume(endpointID, func(volume uintptr) error {
		var value int32
		if hr := comCall(volume, 15, uintptr(unsafe.Pointer(&value))); failed(hr) { // GetMute
			return hresult("GetMute", hr)
		}
		muted = value != 0
		return nil
	})
	return muted, err
}

// SetEndpointMuted sets the render endpoint's mute state. An empty id uses
// the default render endpoint.
func SetEndpointMuted(endpointID string, muted bool) error {
	return withEndpointVolume(endpointID, func(volume uintptr) error {
		var value uintptr
		if muted {
			value = 1
		}
		if hr := comCall(volume, 14, value, 0); failed(hr) { // SetMute
			return hresult("SetMute", hr)
		}
		return nil
	})
}

// withEndpointVolume opens the endpoint's IAudioEndpointVolume on a
// COM-initialized, pinned thread and releases it on return.
func withEndpointVolume(endpointID string, fn func(uintptr) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	balance, err := comInit()
	if err != nil {
		return err
	}
	defer balance()
	en, err := newEnumerator()
	if err != nil {
		return err
	}
	defer release(en)
	var device uintptr
	if endpointID == "" {
		device, err = defaultDevice(en)
	} else {
		device, err = deviceByID(en, endpointID)
	}
	if err != nil {
		return err
	}
	defer release(device)
	var volume uintptr
	if hr := comCall(device, 3, uintptr(unsafe.Pointer(&iidIAudioEndpointVolume)),
		clsctxAll, 0, uintptr(unsafe.Pointer(&volume))); failed(hr) { // IMMDevice.Activate
		return hresult("Activate(IAudioEndpointVolume)", hr)
	}
	defer release(volume)
	return fn(volume)
}
