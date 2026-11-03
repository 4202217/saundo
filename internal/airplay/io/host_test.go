package raopio

import (
	"net"
	"strings"
	"testing"
	"time"
)

func waitEvent(t *testing.T, h *Host, kind EventKind) Event {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev := <-h.Events():
			if ev.Kind == kind {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for event kind %d", kind)
		}
	}
}

func TestTCPConnectSendReceiveClose(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	h := NewHost()
	defer h.Close()
	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	if err := h.TCPConnect(TCPControl, "127.0.0.1", port); err != nil {
		t.Fatal(err)
	}
	ev := waitEvent(t, h, EventTCPConnected)
	if ev.Peer.Port != port || ev.Local.IP == "" {
		t.Fatalf("bad connected event %+v", ev)
	}

	var server net.Conn
	select {
	case server = <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("accept timed out")
	}
	defer server.Close()

	if err := h.TCPSend(TCPControl, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	_ = server.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 5)
	if _, err := readFull(server, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "hello" {
		t.Fatalf("server read %q", buf)
	}

	if _, err := server.Write([]byte("world")); err != nil {
		t.Fatal(err)
	}
	data := waitEvent(t, h, EventTCPData)
	if string(data.Data) != "world" {
		t.Fatalf("client read %q", data.Data)
	}

	// A peer close is reported; a close we initiate is not.
	_ = server.Close()
	closed := waitEvent(t, h, EventTCPClosed)
	if closed.Err != "" || closed.Gen != data.Gen {
		t.Fatalf("bad close event %+v", closed)
	}
	h.TCPClose(TCPControl, false)
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// TestTeardownFlushIsBounded pins the flush bound: a writer blocked on a
// receiver that never reads must give up within the drain deadline instead
// of holding the connection open forever.
func TestTeardownFlushIsBounded(t *testing.T) {
	h := NewHost()
	defer h.Close()
	client, server := net.Pipe() // the pipe has no buffer: a write blocks until read
	defer server.Close()

	s := &tcpSlot{ch: TCPControl, host: h, gen: 1}
	if !s.attach(client) {
		t.Fatal("attach failed")
	}
	s.startWriter()
	if err := s.send([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond) // let the writer block inside Write

	s.drain() // arms the 500 ms write deadline
	start := time.Now()
	deadline := start.Add(3 * time.Second)
	for s.takeWriteErr() == nil {
		if time.Now().After(deadline) {
			t.Fatal("writer did not give up on the stalled receiver")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("flush took %s, want the %s deadline", elapsed, drainFlushDeadline)
	}
	// Giving up closes the connection on the receiver side too.
	buf := make([]byte, 16)
	_ = server.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := server.Read(buf); err == nil {
		t.Fatal("the connection stayed open after the flush gave up")
	}
}

// TestWriteFailureIsReportedAsSendFailed pins the reason fix: when the
// writer fails first, the close event carries the send error instead of a
// blank "clean close".
func TestWriteFailureIsReportedAsSendFailed(t *testing.T) {
	h := NewHost()
	defer h.Close()
	client, server := net.Pipe()
	_ = server.Close() // the peer is gone before anything is written

	s := &tcpSlot{ch: TCPControl, host: h, gen: 1}
	if !s.attach(client) {
		t.Fatal("attach failed")
	}
	s.startWriter()
	if err := s.send([]byte("x")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for s.takeWriteErr() == nil {
		if time.Now().After(deadline) {
			t.Fatal("writer did not record the send failure")
		}
		time.Sleep(5 * time.Millisecond)
	}

	go s.readLoop()
	ev := waitEvent(t, h, EventTCPClosed)
	if !strings.Contains(ev.Err, "send failed") {
		t.Fatalf("close reason = %q, want the send failure", ev.Err)
	}
}

// TestEmitCountsDropsWhenFull pins the drop counter: a full event channel
// drops the newest event and counts it instead of losing it silently.
func TestEmitCountsDropsWhenFull(t *testing.T) {
	h := NewHost()
	defer h.Close()
	for i := 0; i < 256; i++ {
		h.emit(Event{Kind: EventUDPDatagram})
	}
	if h.Dropped() != 0 {
		t.Fatalf("dropped = %d before the channel was full", h.Dropped())
	}
	h.emit(Event{Kind: EventUDPDatagram})
	h.emit(Event{Kind: EventUDPDatagram})
	if got := h.Dropped(); got != 2 {
		t.Fatalf("dropped = %d, want 2", got)
	}
}

func TestUDPBindSendReceive(t *testing.T) {
	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()

	h := NewHost()
	defer h.Close()
	port, err := h.UDPBind(UDPTiming)
	if err != nil || port == 0 {
		t.Fatalf("bind: port=%d err=%v", port, err)
	}
	peerAddr := peer.LocalAddr().(*net.UDPAddr)
	if err := h.UDPSend(UDPTiming, Endpoint{IP: "127.0.0.1", Port: uint16(peerAddr.Port)}, []byte("ping")); err != nil {
		t.Fatal(err)
	}
	_ = peer.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 32)
	n, from, err := peer.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "ping" || int(from.Port) != int(port) {
		t.Fatalf("peer got %q from %+v", buf[:n], from)
	}
	if _, err := peer.WriteToUDP([]byte("pong"), from); err != nil {
		t.Fatal(err)
	}
	ev := waitEvent(t, h, EventUDPDatagram)
	if string(ev.Data) != "pong" || ev.UDP != UDPTiming {
		t.Fatalf("event %+v", ev)
	}
}
