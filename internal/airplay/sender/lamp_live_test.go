package sender

import (
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"saundo/internal/airplay/crypto"
	"saundo/internal/airplay/device"
	"saundo/internal/audio/ring"
	"saundo/internal/discovery"
	"saundo/internal/lamp"
)

// TestLiveLampVariants sends a lamp image to a real receiver in the request
// shapes Unisson and saundo use, so the top display can be compared between
// them. SAUNDO_DEVICE_TEST=1 enables it.
func TestLiveLampVariants(t *testing.T) {
	if os.Getenv("SAUNDO_DEVICE_TEST") == "" {
		t.Skip("set SAUNDO_DEVICE_TEST=1 to run against a real receiver")
	}
	if testing.Short() {
		t.Skip("live device test")
	}

	disc := discovery.New(2*time.Second, 2*time.Second)
	disc.Start()
	defer disc.Stop()
	var target device.Device
	if addr := os.Getenv("SAUNDO_LAMP_TARGET"); addr != "" {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			t.Fatalf("SAUNDO_LAMP_TARGET: %v", err)
		}
		p, err := strconv.Atoi(port)
		if err != nil {
			t.Fatalf("SAUNDO_LAMP_TARGET port: %v", err)
		}
		target = device.Device{Host: host, Port: uint16(p), Name: "HomePod L", ID: "76:3D:7E:CA:67:71", Model: "AudioAccessory5,1", OSVersion: "27.2", Auth: device.AuthHAPTransient, AirPlay2: true}
	} else {
		deadline := time.After(45 * time.Second)
	wait:
		for {
			select {
			case devs := <-disc.Updates():
				for _, d := range devs {
					if d.Auth == device.AuthHAPTransient && d.AirPlay2 {
						target = d
						break wait
					}
				}
			case <-deadline:
				t.Skip("no transient AirPlay 2 receiver found")
			}
		}
	}
	t.Logf("target: %s (%s) at %s:%d model=%s osvers=%s", target.Name, target.ID, target.Host, target.Port, target.Model, target.OSVersion)

	image, err := lamp.JPEG("dune")
	if err != nil {
		t.Fatal(err)
	}

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
	t.Log("launched; playing a 440 Hz tone at 50%; each variant is held for 8 s, watch the HomePod top")

	ls.s.npTitle, ls.s.npArtist, ls.s.npAlbum = "Lamp probe", "saundo", ""
	rtpInfo := "seq=" + utoa(uint64(ls.s.seq)) + ";rtptime=" + utoa(uint64(ls.s.rtpTime32()))
	var hdr []header
	if ls.s.rtspSession != "" {
		hdr = append(hdr, header{"Session", ls.s.rtspSession})
	}
	hdr = append(hdr, header{"RTP-Info", rtpInfo})
	dmap := dmapTag("mlit", dmapTag("minm", ls.s.npTitle)+dmapTag("asar", ls.s.npArtist)+dmapTag("asal", ls.s.npAlbum))
	const progress = "progress: 0/10"

	send := func(label, method, uri, contentType, body string) {
		t.Logf("%s: %s %s %s (%d bytes)", label, method, uri, contentType, len(body))
		ls.s.sendRequest(method, uri, contentType, body, hdr)
		ls.pumpFor(8 * time.Second)
	}

	// Unisson shape, from its reverse-engineered sender: the DMAP metadata
	// travels by POST, the lamp image goes to artwork/command, the progress
	// goes by SET_PARAMETER.
	send("unisson dmap", "POST", ls.s.rtspURI(), "application/x-dmap-tagged", dmap)
	send("unisson image", "POST", "artwork/command", "image/jpeg", string(image))
	ls.s.sendRequest("SET_PARAMETER", ls.s.rtspURI(), "text/parameters", progress, hdr)
	ls.pumpFor(8 * time.Second)

	// Same shape with a leading slash in the artwork path.
	send("slash dmap", "POST", ls.s.rtspURI(), "application/x-dmap-tagged", dmap)
	send("slash image", "POST", "/artwork/command", "image/jpeg", string(image))
	ls.s.sendRequest("SET_PARAMETER", ls.s.rtspURI(), "text/parameters", progress, hdr)
	ls.pumpFor(8 * time.Second)

	// Absolute artwork URI.
	send("absolute image", "POST", "rtsp://"+ls.s.localIP+"/artwork/command", "image/jpeg", string(image))

	// The current saundo baseline: the DMAP by SET_PARAMETER, the image by
	// POST /artwork/command.
	send("baseline dmap", "SET_PARAMETER", ls.s.rtspURI(), "application/x-dmap-tagged", dmap)
	send("baseline image", "POST", "/artwork/command", "image/jpeg", string(image))

	// The MediaRemote now-playing plist, then the image.
	plist, err := crypto.Encode(map[string]any{
		"kMRMediaRemoteNowPlayingInfoTitle":       "Lamp probe",
		"kMRMediaRemoteNowPlayingInfoArtist":      "saundo",
		"kMRMediaRemoteNowPlayingInfoMediaType":   int64(1),
		"kMRMediaRemoteNowPlayingInfoElapsedTime": float64(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	send("mediaremote dmap", "POST", ls.s.rtspURI(), "application/x-apple-binary-plist", string(plist))
	send("mediaremote image", "POST", "artwork/command", "image/jpeg", string(image))

	// The AirPlay artwork endpoints on the receiver's HTTP server, which is
	// the standard cover path; the RTSP control channel rejects artwork POSTs.
	for _, ct := range []string{"image/jpeg", "application/octet-stream", "application/x-apple-binary-plist", "application/x-dmap-tagged", "text/parameters", "image/png"} {
		t.Logf("http artwork /artwork/command ct=%s", ct)
		sendHTTPArtwork(t, target.Host, int(target.Port), "/artwork/command", ct, image, ls.s.rtspSession, ls.s.dacpID, ls.s.identity.Name, ls.s.activeRemote, "")
	}
	t.Log("http artwork /artwork/command empty body")
	sendHTTPArtwork(t, target.Host, int(target.Port), "/artwork/command", "image/jpeg", nil, ls.s.rtspSession, ls.s.dacpID, ls.s.identity.Name, ls.s.activeRemote, "")
	t.Log("http artwork /artwork/command with CSeq")
	sendHTTPArtwork(t, target.Host, int(target.Port), "/artwork/command", "image/jpeg", image, ls.s.rtspSession, ls.s.dacpID, ls.s.identity.Name, ls.s.activeRemote, "1")
	wrapped, err := crypto.Encode(map[string]any{"image": image})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("http artwork /artwork/command bplist {image}")
	sendHTTPArtwork(t, target.Host, int(target.Port), "/artwork/command", "application/x-apple-binary-plist", wrapped, ls.s.rtspSession, ls.s.dacpID, ls.s.identity.Name, ls.s.activeRemote, "")

	t.Log("all variants sent; holding the session for 20 s")
	ls.pumpFor(20 * time.Second)
}

// sendHTTPArtwork posts the image to the receiver's HTTP server, the cover
// path AirPlay uses apart from the RTSP control channel.
func sendHTTPArtwork(t *testing.T, host string, port int, path, contentType string, body []byte, session, dacp, client string, remote uint32, cseq string) {
	t.Helper()
	addr := net.JoinHostPort(host, utoa(uint64(port)))
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		t.Logf("http %s: dial: %v", path, err)
		return
	}
	defer conn.Close()
	head := "POST " + path + " HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Content-Type: " + contentType + "\r\n" +
		"Content-Length: " + utoa(uint64(len(body))) + "\r\n" +
		"Session: " + session + "\r\n" +
		"DACP-ID: " + dacp + "\r\n" +
		"Active-Remote: " + utoa(uint64(remote)) + "\r\n" +
		"Client-Instance: " + dacp + "\r\n" +
		"X-Apple-Client-Name: " + client + "\r\n" +
		"X-Apple-ProtocolVersion: 1.1\r\n" +
		"User-Agent: AirPlay/550.10\r\n"
	if cseq != "" {
		head += "CSeq: " + cseq + "\r\n"
	}
	head += "Connection: close\r\n\r\n"
	conn.Write([]byte(head))
	conn.Write(body)
	buf := make([]byte, 512)
	n, _ := conn.Read(buf)
	t.Logf("http %s ct=%s len=%d: %q", path, contentType, len(body), strings.TrimSpace(string(buf[:n])))
}

// pumpFor keeps the session alive for d while the receiver processes the
// requests just written.
func (ls *liveSession) pumpFor(d time.Duration) {
	deadline := time.Now().Add(d)
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for time.Now().Before(deadline) {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		var timerC <-chan time.Time
		if dl := ls.s.NextDeadline(); !dl.IsZero() {
			dd := time.Until(dl)
			if dd < 0 {
				dd = 0
			}
			timer.Reset(dd)
			timerC = timer.C
		}
		select {
		case ev := <-ls.host.Events():
			ls.s.HandleEvent(ev)
		case <-timerC:
			ls.s.Tick()
		case <-time.After(20 * time.Millisecond):
		}
		if ls.closed {
			return
		}
	}
}
