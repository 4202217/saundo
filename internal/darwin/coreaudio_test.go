//go:build darwin

package darwin

import (
	"testing"
	"unsafe"
)

// TestCoreAudioABI pins the C layouts the purego bindings pass across the
// boundary. The sizes were measured with clang against the macOS SDK; a
// short struct makes AudioObjectGetPropertyData read past the buffer.
// TestCoreAudioOutputScope pins the output scope. The macOS port shipped
// 'out ' (0x6f757420) instead of 'outp' (0x6f757470), so every device mute
// and stream query failed with kAudioHardwareUnknownPropertyError ('who?').
func TestCoreAudioOutputScope(t *testing.T) {
	if ScopeOutput != 0x6f757470 {
		t.Fatalf("ScopeOutput = %#x, want 0x6f757470 ('outp')", ScopeOutput)
	}
}

func TestCoreAudioABI(t *testing.T) {
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"AudioStreamBasicDescription", unsafe.Sizeof(AudioStreamBasicDescription{}), 40},
		{"AudioTimeStamp", unsafe.Sizeof(AudioTimeStamp{}), 64},
		{"SMPTETime", unsafe.Sizeof(SMPTETime{}), 24},
		{"AudioBuffer", unsafe.Sizeof(AudioBuffer{}), 16},
		{"AudioBufferList", unsafe.Sizeof(AudioBufferList{}), 24},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d bytes, want %d", c.name, c.got, c.want)
		}
	}
}
