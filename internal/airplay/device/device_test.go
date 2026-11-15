package device

import "testing"

// TestUnknownFeaturesHideStaleBits pins the fail-closed read: the registry
// can carry an older bit field beside an unknown flag, and Bits must not
// surface it.
func TestUnknownFeaturesHideStaleBits(t *testing.T) {
	f := FeaturesFrom(0xF00, true)
	if f.Known() {
		t.Fatal("unknown features read as known")
	}
	if got := f.Bits(); got != 0 {
		t.Fatalf("Bits = %#x, want zero for an unknown value", got)
	}
	if f.Has(0x800) {
		t.Fatal("an unknown value reported a bit")
	}
	bits, unknown := f.Persisted()
	if bits != 0xF00 || !unknown {
		t.Fatalf("Persisted = %#x, %v, want the cached bits and unknown", bits, unknown)
	}
}

func TestTopDisplayCapable(t *testing.T) {
	cases := []struct {
		name string
		dev  Device
		want bool
	}{
		{"HomePod 2 at 27.2", Device{Model: "AudioAccessory6,1", OSVersion: "27.2"}, true},
		{"HomePod 2 above 27.2", Device{Model: "AudioAccessory6,1", OSVersion: "28.0"}, true},
		{"HomePod 2 with a patch version", Device{Model: "AudioAccessory6,1", OSVersion: "27.2.1"}, true},
		{"HomePod 2 below 27.2", Device{Model: "AudioAccessory6,1", OSVersion: "27.1.2"}, false},
		{"HomePod 2 with a missing minor", Device{Model: "AudioAccessory6,1", OSVersion: "27"}, false},
		{"HomePod 2 with no version", Device{Model: "AudioAccessory6,1"}, false},
		{"mini", Device{Model: "AudioAccessory5,1", OSVersion: "27.2"}, true},
		{"Mac", Device{Model: "Mac14,2", OSVersion: "27.2"}, false},
		{"Apple TV", Device{Model: "AppleTV6,2", OSVersion: "27.2"}, false},
		{"manual receiver", Device{OSVersion: "27.2"}, false},
	}
	for _, tc := range cases {
		if got := TopDisplayCapable(tc.dev); got != tc.want {
			t.Errorf("%s: TopDisplayCapable = %v, want %v", tc.name, got, tc.want)
		}
	}
}
