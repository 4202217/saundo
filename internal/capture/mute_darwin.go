//go:build darwin

package capture

import (
	"fmt"
	"unsafe"

	"saundo/internal/darwin"
)

// Mute-local on macOS mutes the output device the settings selected, the
// default output when the stored endpoint is empty. The tap keeps capturing
// regardless: capture and local playback are independent decisions here,
// where the Windows endpoint mute is driver-dependent.

func muteDevice(endpointID string) (uint32, error) {
	if endpointID == "" {
		id, _, _, err := darwin.DefaultOutputDevice()
		return id, err
	}
	return darwin.DeviceForUID(endpointID)
}

// EndpointMuted reports the output device's mute state.
func EndpointMuted(endpointID string) (bool, error) {
	dev, err := muteDevice(endpointID)
	if err != nil {
		return false, err
	}
	var muted uint32
	size := uint32(4)
	if err := darwin.GetProperty(dev, darwin.PropertyAddress(darwin.PropDeviceMute, darwin.ScopeOutput), unsafe.Pointer(&muted), &size); err != nil {
		return false, err
	}
	return muted != 0, nil
}

// SetEndpointMuted mutes or unmutes the output device.
func SetEndpointMuted(endpointID string, muted bool) error {
	dev, err := muteDevice(endpointID)
	if err != nil {
		return err
	}
	var value uint32
	if muted {
		value = 1
	}
	size := uint32(4)
	if err := darwin.SetProperty(dev, darwin.PropertyAddress(darwin.PropDeviceMute, darwin.ScopeOutput), unsafe.Pointer(&value), size); err != nil {
		return fmt.Errorf("capture: set mute: %w", err)
	}
	return nil
}
