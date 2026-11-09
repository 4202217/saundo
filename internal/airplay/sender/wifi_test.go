package sender

import (
	"os"
	"testing"
)

func TestParseStreamSetupReadsWiFiMetrics(t *testing.T) {
	body, err := os.ReadFile("testdata/setup_wifi.bplist")
	if err != nil {
		t.Fatal(err)
	}
	setup, ok := parseStreamSetup(body)
	if !ok {
		t.Fatal("fixture did not parse")
	}
	if setup.DataPort != 6000 || setup.ControlPort != 6001 {
		t.Fatalf("ports = %d/%d, want 6000/6001", setup.DataPort, setup.ControlPort)
	}
	wifi := setup.WiFi
	if !wifi.Valid {
		t.Fatal("wiFiMetrics read as unknown")
	}
	if wifi.RSSI != -52 {
		t.Fatalf("rssi = %d, want -52", wifi.RSSI)
	}
	if wifi.SSID != "HomeNet" {
		t.Fatalf("ssid = %q, want HomeNet", wifi.SSID)
	}
	if wifi.Band != WiFiBand5 {
		t.Fatalf("band = %d, want 5 GHz", wifi.Band)
	}
}

func TestParseStreamSetupWithoutWiFiIsUnknown(t *testing.T) {
	body, err := os.ReadFile("testdata/setup_no_wifi.bplist")
	if err != nil {
		t.Fatal(err)
	}
	setup, ok := parseStreamSetup(body)
	if !ok {
		t.Fatal("fixture did not parse")
	}
	if setup.WiFi.Valid {
		t.Fatalf("missing wiFiMetrics read as valid: %+v", setup.WiFi)
	}
}

func TestParseStreamSetupMalformedIsUnknown(t *testing.T) {
	setup, ok := parseStreamSetup([]byte("not a plist"))
	if ok {
		t.Fatal("malformed body parsed")
	}
	if setup.WiFi.Valid {
		t.Fatalf("malformed body produced WiFi metrics: %+v", setup.WiFi)
	}
}

func TestWiFiBandFromRaw(t *testing.T) {
	cases := map[int64]WiFiBand{
		1: WiFiBand24,
		2: WiFiBand5,
		3: WiFiBand6,
		0: WiFiBandUnknown,
		4: WiFiBandUnknown,
	}
	for raw, want := range cases {
		if got := wifiBandFromRaw(raw); got != want {
			t.Errorf("band %d = %d, want %d", raw, got, want)
		}
	}
}
