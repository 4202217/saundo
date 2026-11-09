package discovery

import (
	"net"
	"testing"

	"github.com/hashicorp/mdns"
	"saundo/internal/airplay/device"
)

func TestParseEntrySetsFamily(t *testing.T) {
	tests := []struct {
		name string
		am   string
		want device.Family
	}{
		{"AudioAccessory5,1", "", device.FamilyHomePod},
		{"Mac14,2", "", device.FamilyMac},
		{"AppleTV6,2", "", device.FamilyAppleTV},
		{"", "AirPort10,115", device.FamilyAirPort},
		{"Windows", "", device.FamilyOther},
	}
	for _, tt := range tests {
		fields := []string{"deviceid=AA:BB:CC:DD:EE:FF"}
		if tt.name != "" {
			fields = append(fields, "model="+tt.name)
		}
		if tt.am != "" {
			fields = append(fields, "am="+tt.am)
		}
		e := &mdns.ServiceEntry{
			Name:       "Receiver._airplay._tcp.local.",
			AddrV4:     net.IPv4(192, 0, 2, 1),
			Port:       7000,
			InfoFields: fields,
		}
		d, ok := parseEntry(e, airplayService)
		if !ok {
			t.Fatalf("parseEntry(%q, %q) rejected the entry", tt.name, tt.am)
		}
		if d.Family != tt.want {
			t.Errorf("parseEntry(%q, %q) family = %v, want %v", tt.name, tt.am, d.Family, tt.want)
		}
	}
}
