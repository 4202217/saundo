//go:build darwin && muteprobe

package capture

import "testing"

// TestDarwinEndpointMute proves the Core Audio output mute on real hardware.
// It mutes the default output for a moment and restores the previous state.
// Run it by hand:
//
//	go test -tags muteprobe ./internal/capture -run TestDarwinEndpointMute -v
func TestDarwinEndpointMute(t *testing.T) {
	previous, err := EndpointMuted("")
	if err != nil {
		t.Fatalf("read the default output mute: %v", err)
	}
	t.Cleanup(func() {
		if err := SetEndpointMuted("", previous); err != nil {
			t.Errorf("restore the output mute: %v", err)
		}
	})
	if err := SetEndpointMuted("", true); err != nil {
		t.Fatalf("mute the default output: %v", err)
	}
	if muted, err := EndpointMuted(""); err != nil || !muted {
		t.Fatalf("after mute: muted=%v err=%v, want true", muted, err)
	}
	if err := SetEndpointMuted("", false); err != nil {
		t.Fatalf("unmute the default output: %v", err)
	}
	if muted, err := EndpointMuted(""); err != nil || muted {
		t.Fatalf("after unmute: muted=%v err=%v, want false", muted, err)
	}
}
