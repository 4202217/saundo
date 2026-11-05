package sender

import (
	"encoding/base64"
	"math"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"saundo/internal/airplay/crypto"
	"saundo/internal/airplay/device"
	"saundo/internal/audio/ring"
	"saundo/internal/lamp"
)

// TestLiveLampRepeat sends the artwork-command plist repeatedly with several
// key names and presets, so the receiver's top display can be watched across
// attempts. SAUNDO_DEVICE_TEST=1 and SAUNDO_LAMP_TARGET=host:port enable it.
func TestLiveLampRepeat(t *testing.T) {
	if os.Getenv("SAUNDO_DEVICE_TEST") == "" {
		t.Skip("set SAUNDO_DEVICE_TEST=1 to run against a real receiver")
	}
	if testing.Short() {
		t.Skip("live device test")
	}
	addr := os.Getenv("SAUNDO_LAMP_TARGET")
	if addr == "" {
		t.Skip("set SAUNDO_LAMP_TARGET=host:port")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SAUNDO_LAMP_TARGET: %v", err)
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("SAUNDO_LAMP_TARGET port: %v", err)
	}
	target := device.Device{Host: host, Port: uint16(p), Name: "HomePod L", ID: "76:3D:7E:CA:67:71", Model: "AudioAccessory5,1", OSVersion: "27.2", Auth: device.AuthHAPTransient, AirPlay2: true}
	t.Logf("target: %s at %s:%d model=%s osvers=%s", target.Name, target.Host, target.Port, target.Model, target.OSVersion)

	ls := startLiveSession(t, target)
	defer ls.teardown()
	if !ls.pumpUntilLaunched(20 * time.Second) {
		t.Fatalf("session did not launch: %s", ls.lastErr)
	}
	tone := ring.New(44100 * 2 * 2)
	samples := make([]int16, 44100*2*2)
	for i := 0; i < 44100*2; i++ {
		v := int16(12000 * math.Sin(2*math.Pi*440*float64(i)/44100))
		samples[i*2], samples[i*2+1] = v, v
	}
	tone.Push(samples)
	ls.s.AttachRing(tone)
	ls.s.SetVolume(50)
	t.Log("launched; playing a 440 Hz tone at 50%; sends are 3 s apart")

	presets := []string{"dune", "ember", "moss", "tide", "orchid", "moon"}
	keys := []string{"image", "jpeg", "artworkData"}
	n := 0
	for _, id := range presets {
		image, err := lamp.JPEG(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range keys {
			body, err := crypto.Encode(map[string]any{key: image})
			if err != nil {
				t.Fatal(err)
			}
			n++
			t.Logf("send %d: /artwork/command key=%s preset=%s (%d bytes)", n, key, id, len(body))
			sendHTTPArtwork(t, target.Host, int(target.Port), "/artwork/command", "application/x-apple-binary-plist", body, ls.s.rtspSession, ls.s.dacpID, ls.s.identity.Name, ls.s.activeRemote, "")
			ls.pumpFor(3 * time.Second)
		}
	}

	// The Unisson front end hands the Rust core a data URL, not raw bytes.
	// Send the same image in that shape before and after the DMAP metadata.
	image, err := lamp.JPEG("dune")
	if err != nil {
		t.Fatal(err)
	}
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(image)
	sendPlist := func(label string, v map[string]any) {
		body, err := crypto.Encode(v)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("dataurl %s (%d bytes)", label, len(body))
		sendHTTPArtwork(t, target.Host, int(target.Port), "/artwork/command", "application/x-apple-binary-plist", body, ls.s.rtspSession, ls.s.dacpID, ls.s.identity.Name, ls.s.activeRemote, "")
		ls.pumpFor(4 * time.Second)
	}
	sendPlist("jpeg=dataURL", map[string]any{"jpeg": dataURL})
	sendPlist("artwork+jpeg=dataURL", map[string]any{"artwork": "Lamp probe", "jpeg": dataURL})
	sendPlist("image=base64", map[string]any{"image": base64.StdEncoding.EncodeToString(image)})

	ls.s.npTitle, ls.s.npArtist = "Lamp probe", "saundo"
	rtpInfo := "seq=" + utoa(uint64(ls.s.seq)) + ";rtptime=" + utoa(uint64(ls.s.rtpTime32()))
	var hdr []header
	if ls.s.rtspSession != "" {
		hdr = append(hdr, header{"Session", ls.s.rtspSession})
	}
	hdr = append(hdr, header{"RTP-Info", rtpInfo})
	dmap := dmapTag("mlit", dmapTag("minm", ls.s.npTitle)+dmapTag("asar", ls.s.npArtist))
	ls.s.sendRequest("SET_PARAMETER", ls.s.rtspURI(), "application/x-dmap-tagged", dmap, hdr)
	ls.pumpFor(2 * time.Second)
	sendPlist("after-dmap jpeg=dataURL", map[string]any{"jpeg": dataURL})

	t.Log("repeat done; holding the session for 20 s")
	ls.pumpFor(20 * time.Second)
}
