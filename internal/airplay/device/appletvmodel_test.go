package device

import "testing"

func TestAppleTVModelName(t *testing.T) {
	cases := map[string]string{
		"AppleTV1,1":  "Apple TV (1st gen)",
		"AppleTV2,1":  "Apple TV (2nd gen)",
		"AppleTV3,1":  "Apple TV (3rd gen)",
		"AppleTV3,2":  "Apple TV (3rd gen)",
		"AppleTV5,3":  "Apple TV HD",
		"AppleTV6,2":  "Apple TV 4K (1st gen)",
		"AppleTV11,1": "Apple TV 4K (2nd gen)",
		"AppleTV14,1": "Apple TV 4K (3rd gen)",
		"AppleTV18,1": "Apple TV 4K (4th gen)",
	}
	for model, want := range cases {
		if got := AppleTVModelName(model); got != want {
			t.Errorf("AppleTVModelName(%q) = %q, want %q", model, got, want)
		}
	}
	for _, unknown := range []string{"AppleTV99,9", "AudioAccessory5,1", "Mac14,2", ""} {
		if got := AppleTVModelName(unknown); got != "" {
			t.Errorf("AppleTVModelName(%q) = %q, want an empty fallback", unknown, got)
		}
	}
}
