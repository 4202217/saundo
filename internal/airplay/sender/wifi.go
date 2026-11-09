package sender

import "saundo/internal/airplay/crypto"

// WiFiBand is the band a receiver reports in its SETUP response. The raw
// values are the receiver's own enumeration, not a guess: 1, 2, and 3 map to
// 2.4, 5, and 6 GHz; anything else is unknown.
type WiFiBand uint8

const (
	WiFiBandUnknown WiFiBand = iota
	WiFiBand24
	WiFiBand5
	WiFiBand6
)

// WiFiMetrics is the receiver's Wi-Fi snapshot from the stream SETUP
// response. It is a device-side sample that can be up to 30 minutes old, not
// a live signal reading. Valid is false when the response carried no
// wiFiMetrics object or no rssi; a missing value is unknown, never 0 dBm.
type WiFiMetrics struct {
	Valid bool
	RSSI  int64
	SSID  string
	Band  WiFiBand
}

// streamSetup is the part of the stream SETUP response the sender uses.
type streamSetup struct {
	DataPort          uint16
	ControlPort       uint16
	ReceiverLatencyMS int
	WiFi              WiFiMetrics
}

// parseStreamSetup reads the first entry of the response's streams array. A
// malformed response reports ok false; the caller fails the session on a
// missing data port instead of guessing one.
func parseStreamSetup(body []byte) (streamSetup, bool) {
	root, err := crypto.Decode(body)
	if err != nil {
		return streamSetup{}, false
	}
	m, ok := root.(map[string]any)
	if !ok {
		return streamSetup{}, false
	}
	streams, ok := m["streams"].([]any)
	if !ok || len(streams) == 0 {
		return streamSetup{}, false
	}
	s0, ok := streams[0].(map[string]any)
	if !ok {
		return streamSetup{}, false
	}
	setup := streamSetup{
		DataPort:          uint16(plistInt(s0, "dataPort")),
		ControlPort:       uint16(plistInt(s0, "controlPort")),
		ReceiverLatencyMS: int(plistInt(s0, "arrivalToRenderLatencyMs")),
		WiFi:              parseWiFiMetrics(s0),
	}
	return setup, true
}

// parseWiFiMetrics reads the capital-F wiFiMetrics object. rssi is required:
// an object without it is collection-off, which reads as unknown.
func parseWiFiMetrics(stream map[string]any) WiFiMetrics {
	m, ok := stream["wiFiMetrics"].(map[string]any)
	if !ok {
		return WiFiMetrics{}
	}
	rssi, ok := plistIntOK(m, "rssi")
	if !ok {
		return WiFiMetrics{}
	}
	wifi := WiFiMetrics{Valid: true, RSSI: rssi}
	if ssid, ok := m["ssid"].(string); ok {
		wifi.SSID = ssid
	}
	if band, ok := plistIntOK(m, "wifiBand"); ok {
		wifi.Band = wifiBandFromRaw(band)
	}
	return wifi
}

func wifiBandFromRaw(raw int64) WiFiBand {
	switch raw {
	case 1:
		return WiFiBand24
	case 2:
		return WiFiBand5
	case 3:
		return WiFiBand6
	default:
		return WiFiBandUnknown
	}
}
