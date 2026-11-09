package device

import "testing"

func TestFamilyFor(t *testing.T) {
	tests := []struct {
		model, am string
		want      Family
	}{
		{"AudioAccessory5,1", "", FamilyHomePod},
		{"AudioAccessory6,1", "", FamilyHomePod},
		{"", "AudioAccessory5,1", FamilyHomePod},
		{"Mac14,2", "", FamilyMac},
		{"AppleTV6,2", "", FamilyAppleTV},
		{"", "AirPort10,115", FamilyAirPort},
		{"AirPort Express", "", FamilyAirPort},
		{"Windows", "", FamilyOther},
		{"", "", FamilyUnknown},
	}
	for _, tt := range tests {
		if got := FamilyFor(tt.model, tt.am); got != tt.want {
			t.Errorf("FamilyFor(%q, %q) = %v, want %v", tt.model, tt.am, got, tt.want)
		}
	}
}
