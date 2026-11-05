package sender

import (
	"bufio"
	"encoding/base64"
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
	"saundo/internal/lamp"
)

// TestLiveLampDataURL sends the artwork-command plist over one keep-alive
// connection, in the data-URL shapes the Unisson front end produces. It exists
// because the receiver's HTTP server stops accepting new connections after a
// burst, so each request must reuse one connection.
func TestLiveLampDataURL(t *testing.T) {
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

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, utoa(uint64(p))), 5*time.Second)
	if err != nil {
		t.Fatalf("dial artwork server: %v", err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	session, dacp, client := ls.s.rtspSession, ls.s.dacpID, ls.s.identity.Name

	send := func(label string, v map[string]any) {
		body, err := crypto.Encode(v)
		if err != nil {
			t.Fatal(err)
		}
		head := "POST /artwork/command HTTP/1.1\r\n" +
			"Host: " + net.JoinHostPort(host, utoa(uint64(p))) + "\r\n" +
			"Content-Type: application/x-apple-binary-plist\r\n" +
			"Content-Length: " + utoa(uint64(len(body))) + "\r\n" +
			"Session: " + session + "\r\n" +
			"DACP-ID: " + dacp + "\r\n" +
			"Active-Remote: " + utoa(uint64(ls.s.activeRemote)) + "\r\n" +
			"Client-Instance: " + dacp + "\r\n" +
			"X-Apple-Client-Name: " + client + "\r\n" +
			"X-Apple-ProtocolVersion: 1.1\r\n" +
			"User-Agent: AirPlay/550.10\r\n" +
			"Connection: keep-alive\r\n\r\n"
		if _, err := conn.Write([]byte(head)); err != nil {
			t.Logf("%s: write head: %v", label, err)
			return
		}
		if _, err := conn.Write(body); err != nil {
			t.Logf("%s: write body: %v", label, err)
			return
		}
		status, err := reader.ReadString('\n')
		if err != nil {
			t.Logf("%s: read status: %v", label, err)
			return
		}
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Logf("%s: read headers: %v", label, err)
				return
			}
			if line == "\r\n" || line == "\n" {
				break
			}
		}
		t.Logf("%s (%d bytes): %q", label, len(body), strings.TrimSpace(status))
		ls.pumpFor(4 * time.Second)
	}

	image, err := lamp.JPEG("dune")
	if err != nil {
		t.Fatal(err)
	}
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(image)
	send("jpeg=dataURL", map[string]any{"jpeg": dataURL})
	send("artwork+jpeg=dataURL", map[string]any{"artwork": "Lamp probe", "jpeg": dataURL})
	send("image=dataURL", map[string]any{"image": dataURL})
	send("image=base64", map[string]any{"image": base64.StdEncoding.EncodeToString(image)})

	ember, err := lamp.JPEG("ember")
	if err != nil {
		t.Fatal(err)
	}
	send("ember jpeg=dataURL", map[string]any{"jpeg": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(ember)})

	t.Log("dataurl variants done; holding the session for 20 s")
	ls.pumpFor(20 * time.Second)
}
