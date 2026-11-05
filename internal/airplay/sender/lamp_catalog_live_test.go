package sender

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
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
)

type catalogBlob struct {
	c      color.RGBA
	x, y   float64
	radius float64
}

type catalogLight struct {
	id    string
	base  color.RGBA
	blobs []catalogBlob
}

func hexColor(s string) color.RGBA {
	var r, g, b uint8
	fmt := func(i int) uint8 {
		v := 0
		for _, ch := range s[i : i+2] {
			v *= 16
			switch {
			case ch >= '0' && ch <= '9':
				v += int(ch - '0')
			case ch >= 'a' && ch <= 'f':
				v += int(ch-'a') + 10
			case ch >= 'A' && ch <= 'F':
				v += int(ch-'A') + 10
			}
		}
		return uint8(v)
	}
	if len(s) == 7 && s[0] == '#' {
		r, g, b = fmt(1), fmt(3), fmt(5)
	}
	return color.RGBA{r, g, b, 255}
}

// renderCatalogLight fills a 600x600 canvas with the base color and blends the
// three radial blobs exactly as the Unisson catalog describes them.
func renderCatalogLight(l catalogLight) []byte {
	const size = 600
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			px, py := float64(x)/size, float64(y)/size
			c := l.base
			for _, b := range l.blobs {
				dx, dy := px-b.x, py-b.y
				d := math.Sqrt(dx*dx+dy*dy) / b.radius
				if d >= 1 {
					continue
				}
				a := 1 - d
				c.R = uint8(float64(b.c.R)*a + float64(c.R)*(1-a))
				c.G = uint8(float64(b.c.G)*a + float64(c.G)*(1-a))
				c.B = uint8(float64(b.c.B)*a + float64(c.B)*(1-a))
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88}); err != nil {
		return nil
	}
	return buf.Bytes()
}

func TestLiveLampCatalog(t *testing.T) {
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

	lights := []catalogLight{
		{id: "dawn", base: hexColor("#ff4d6d"), blobs: []catalogBlob{
			{c: hexColor("#ffb000"), x: 0.18, y: 0.12, radius: 0.72},
			{c: hexColor("#ff2e93"), x: 0.92, y: 0.86, radius: 0.7},
			{c: hexColor("#ffe14d"), x: 0.1, y: 0.95, radius: 0.48},
		}},
		{id: "lagoon", base: hexColor("#00b4d8"), blobs: []catalogBlob{
			{c: hexColor("#00f5a0"), x: 0.08, y: 0.88, radius: 0.72},
			{c: hexColor("#1e5bff"), x: 0.92, y: 0.1, radius: 0.72},
			{c: hexColor("#7df9ff"), x: 0.55, y: 0.5, radius: 0.36},
		}},
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, utoa(uint64(p))), 5*time.Second)
	if err != nil {
		t.Fatalf("dial artwork server: %v", err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	session, dacp, client := ls.s.rtspSession, ls.s.dacpID, ls.s.identity.Name
	ls.s.npTitle, ls.s.npArtist = "Lamp probe", "saundo"
	rtpInfo := "seq=" + utoa(uint64(ls.s.seq)) + ";rtptime=" + utoa(uint64(ls.s.rtpTime32()))
	var hdr []header
	if session != "" {
		hdr = append(hdr, header{"Session", session})
	}
	hdr = append(hdr, header{"RTP-Info", rtpInfo})
	dmap := dmapTag("mlit", dmapTag("minm", ls.s.npTitle)+dmapTag("asar", ls.s.npArtist))
	ls.s.sendRequest("SET_PARAMETER", ls.s.rtspURI(), "application/x-dmap-tagged", dmap, hdr)
	ls.pumpFor(2 * time.Second)

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

	for _, l := range lights {
		image := renderCatalogLight(l)
		if image == nil {
			t.Fatalf("render %s", l.id)
		}
		dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(image)
		send(l.id+" jpeg=dataURL", map[string]any{"jpeg": dataURL})
		send(l.id+" artwork+jpeg", map[string]any{"artwork": "Lamp probe", "jpeg": dataURL})
	}

	t.Log("catalog lights done; holding the session for 20 s")
	ls.pumpFor(20 * time.Second)
}
