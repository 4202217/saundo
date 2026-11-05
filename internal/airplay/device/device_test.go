package device

import "testing"

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
