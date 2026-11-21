package device

import "testing"

func TestHomePodModelName(t *testing.T) {
	cases := map[string]string{
		"AudioAccessory1,1":  "HomePod",
		"AudioAccessory5,1":  "HomePod mini",
		"AudioAccessory6,1":  "HomePod (2nd gen)",
		"AudioAccessory11,1": "HomePod mini 2",
	}
	for model, want := range cases {
		if got := HomePodModelName(model); got != want {
			t.Errorf("HomePodModelName(%q) = %q, want %q", model, got, want)
		}
	}
	for _, unknown := range []string{"AudioAccessory9,9", "Mac14,2", "AppleTV6,2", ""} {
		if got := HomePodModelName(unknown); got != "" {
			t.Errorf("HomePodModelName(%q) = %q, want an empty fallback", unknown, got)
		}
	}
}
