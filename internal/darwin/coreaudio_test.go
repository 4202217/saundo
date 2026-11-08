//go:build darwin

package darwin

import (
	"testing"
	"unsafe"
)

// TestCoreAudioABI pins the C layouts the purego bindings pass across the
// boundary. The sizes were measured with clang against the macOS SDK; a
// short struct makes AudioObjectGetPropertyData read past the buffer.
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
