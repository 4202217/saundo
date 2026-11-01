package sender

import (
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"saundo/internal/airplay/device"
	raopio "saundo/internal/airplay/io"
	"saundo/internal/airplay/ptp"
	"saundo/internal/audio/ring"
	"saundo/internal/discovery"
)

// TestLiveMacPassword streams to a password-protected macOS AirPlay receiver.
// It proves HAP pair-setup with the stored AirPlay password, pair-verify, the
// RFC 2069 digest challenge on the AP2 session SETUP, the PTP follower, and
// encrypted ALAC streaming.
//
// SAUNDO_MAC_TEST=1 enables it and SAUNDO_MAC_PASSWORD supplies the
// receiver password. SAUNDO_MAC_TONE=1 plays one second of 440 Hz at 20%
// instead of silence. It needs a Mac on the LAN with AirPlay Receiver enabled
// and Windows Firewall permission for inbound UDP 319/320 and mDNS.
func TestLiveMacPassword(t *testing.T) {
	if os.Getenv("SAUNDO_MAC_TEST") == "" {
		t.Skip("set SAUNDO_MAC_TEST=1 to run against a real Mac")
	}
	password := os.Getenv("SAUNDO_MAC_PASSWORD")
	if password == "" {
		t.Skip("set SAUNDO_MAC_PASSWORD to the receiver's AirPlay password")
	}
	tone := os.Getenv("SAUNDO_MAC_TONE") != ""

	disc := discovery.New(2*time.Second, 2*time.Second)
	disc.Start()
	defer disc.Stop()
	var target device.Device
	deadline := time.After(12 * time.Second)
wait:
	for {
		select {
		case devs := <-disc.Updates():
			for _, d := range devs {
				if d.Auth == device.AuthHAPPassword && strings.HasPrefix(d.Model, "Mac") {
					target = d
					break wait
				}
			}
		case <-deadline:
			t.Skip("no password-protected HAP receiver found")
		}
	}
	t.Logf("target: %q id=%s host=%s:%d model=%s auth=%s", target.Name, target.ID, target.Host, target.Port, target.Model, target.Auth)

	host := raopio.NewHost()
	defer host.Close()
	clock, err := ptp.New(target.Host)
	if err != nil {
		t.Fatal(err)
	}
	clock.Start()
	defer clock.Stop()

	var launched bool
	var launchedAt time.Time
	var lastErr string
	var closed bool
	s := New(host, Events{
		Launched: func(ok bool, err string) {
			launched, lastErr = ok, err
			if ok {
				launchedAt = time.Now()
			}
			t.Logf("launched=%v err=%q", ok, err)
		},
		Closed:       func() { closed = true; t.Log("receiver closed the session") },
		PinRequired:  func(name string) { t.Logf("unexpected PIN request from %s", name) },
		StateChanged: func(state State) { t.Logf("state: %s", state) },
	}, func(level Level, format string, args ...any) {
		t.Logf("[%d] "+format, append([]any{level}, args...)...)
	})
	s.SetIdentity(Identity{Name: "saundo-test", DeviceID: "02:00:00:00:00:06", Model: "iPhone14,3"})
	s.SetAuth(target.Auth, target.Auth.IsHAP(), target.ID, "", password)
	s.SetPTPClock(clock)
	rb := ring.New(44100 * 2 * 2)
	if tone {
		samples := make([]int16, 44100*2)
		for i := 0; i < 44100; i++ {
			v := int16(9000 * math.Sin(2*math.Pi*440*float64(i)/44100))
			samples[i*2], samples[i*2+1] = v, v
		}
		rb.Push(samples)
		s.SetVolume(20)
		t.Log("playing one second of 440 Hz at 20%")
	} else {
		s.SetVolume(0)
	}
	s.AttachRing(rb)

	timer := time.NewTimer(time.Hour)
	timer.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	s.Start(target.Host, target.Port, target.Name)
	stopAt := time.Now().Add(45 * time.Second)
	for {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		var timerC <-chan time.Time
		if dl := s.NextDeadline(); !dl.IsZero() {
			d := time.Until(dl)
			if d < 0 {
				d = 0
			}
			timer.Reset(d)
			timerC = timer.C
		}
		select {
		case ev := <-host.Events():
			s.HandleEvent(ev)
		case <-timerC:
			s.Tick()
		case <-tick.C:
		}
		if closed {
			t.Fatalf("receiver closed the session: %s", lastErr)
		}
		if launched && time.Since(launchedAt) > 3*time.Second {
			break
		}
		if time.Now().After(stopAt) {
			break
		}
	}
	if !launched {
		t.Fatalf("session did not launch: %s", lastErr)
	}
	if s.State() != StateStreaming || s.framesSent == 0 {
		t.Fatalf("no stream: state=%s frames=%d", s.State(), s.framesSent)
	}
	t.Logf("live session ok: dataPort=%d controlPort=%d frames=%d encrypted=%v ptp=%+v",
		s.serverPort, s.controlPort, s.framesSent, s.controlEncrypted, clock.Stats())
	s.Stop()
}
