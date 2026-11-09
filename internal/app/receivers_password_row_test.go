package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/airplay/device"
	"saundo/internal/receivers"
)

func TestHomePodRowHidesPasswordOption(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverOpen = map[string]bool{"AA:00": true}
	tt := ui.NewTester(a.settingsView, 720, 500)
	if tt.HasText(locales.ReceiverPasswordTitle()) {
		t.Fatalf("a PIN-paired HomePod shows the password row: %q", tt.Texts())
	}
}

func TestManualAndMacRowsKeepPasswordOption(t *testing.T) {
	a := receiverFixture()
	a.page = pageReceivers
	a.receiverOpen = map[string]bool{"m:ab12": true}
	tt := ui.NewTester(a.settingsView, 720, 500)
	if !tt.HasText(locales.ReceiverPasswordTitle()) {
		t.Fatalf("a manual receiver lost its password row: %q", tt.Texts())
	}
}

func TestReceiverPasswordEditable(t *testing.T) {
	tests := []struct {
		name string
		v    receivers.View
		want bool
	}{
		{
			name: "homepod transient",
			v:    receivers.View{Model: "AudioAccessory5,1", Device: device.Device{Auth: device.AuthHAPTransient}},
			want: false,
		},
		{
			name: "homepod hap password",
			v:    receivers.View{Model: "AudioAccessory5,1", Device: device.Device{Auth: device.AuthHAPPassword}},
			want: false,
		},
		{
			name: "apple tv pin",
			v:    receivers.View{Model: "AppleTV6,2", Device: device.Device{Auth: device.AuthHAPPIN}},
			want: false,
		},
		{
			name: "mac password",
			v:    receivers.View{Model: "Mac14,2", Device: device.Device{Auth: device.AuthHAPPassword}},
			want: true,
		},
		{
			name: "manual",
			v:    receivers.View{Kind: receivers.KindManual},
			want: true,
		},
		{
			name: "saved password",
			v:    receivers.View{PasswordSet: true, Model: "AudioAccessory5,1", Device: device.Device{Auth: device.AuthHAPTransient}},
			want: true,
		},
	}
	for _, tt := range tests {
		if got := receiverPasswordEditable(tt.v); got != tt.want {
			t.Errorf("%s: receiverPasswordEditable = %v, want %v", tt.name, got, tt.want)
		}
	}
}
