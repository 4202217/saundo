package raopio

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ErrSlotClosed is returned when the sender writes to a closed socket.
var ErrSlotClosed = errors.New("raopio: socket is closed")

// Host is the Go net implementation of IO. It owns the sockets and delivers
// every read, connect, and close as an Event on Events. The engine drains that
// channel and drives the sender from one goroutine.
type Host struct {
	events chan Event

	mu      sync.Mutex
	tcp     [2]*tcpSlot
	udp     [3]*udpSlot
	closed  bool
	nextGen atomic.Uint64
}

// NewHost returns a host whose event channel buffers 256 notifications.
func NewHost() *Host {
	return &Host{events: make(chan Event, 256)}
}

// Events is the stream of host notifications.
func (h *Host) Events() <-chan Event { return h.events }

// Close releases every socket. It is safe to call more than once.
func (h *Host) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	tcp := h.tcp
	udp := h.udp
	h.tcp = [2]*tcpSlot{}
	h.udp = [3]*udpSlot{}
	h.mu.Unlock()

	for _, s := range tcp {
		if s != nil {
			s.abort()
		}
	}
	for _, s := range udp {
		if s != nil {
			s.close()
		}
	}
}

func (h *Host) emit(ev Event) {
	if ev.Data != nil {
		ev.Data = append([]byte(nil), ev.Data...)
	}
	select {
	case h.events <- ev:
	default:
		// The engine drains this channel promptly. If it cannot, the session
		// is already broken; dropping a datagram beats deadlocking a reader.
	}
}

// TCPConnect starts a non-blocking connect. Completion arrives as
// EventTCPConnected or EventTCPConnectFailed.
func (h *Host) TCPConnect(ch TCPChannel, host string, port uint16) error {
	if ch != TCPControl && ch != TCPEvent {
		return fmt.Errorf("raopio: bad TCP channel %d", ch)
	}
	hostPort := net.JoinHostPort(host, fmt.Sprint(port))
	gen := h.nextGen.Add(1)
	slot := &tcpSlot{ch: ch, host: h, gen: gen}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return ErrSlotClosed
	}
	old := h.tcp[ch]
	h.tcp[ch] = slot
	h.mu.Unlock()
	if old != nil {
		old.abort()
	}

	go func() {
		d := net.Dialer{Timeout: 5 * time.Second}
		conn, err := d.Dial("tcp4", hostPort)
		if err != nil {
			if slot.isAborted() {
				return
			}
			h.emit(Event{Kind: EventTCPConnectFailed, TCP: ch, Gen: gen, Err: err.Error()})
			return
		}
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetNoDelay(true)
		}
		if !slot.attach(conn) {
			_ = conn.Close()
			return
		}
		local := endpointOf(conn.LocalAddr())
		peer := endpointOf(conn.RemoteAddr())
		h.emit(Event{Kind: EventTCPConnected, TCP: ch, Gen: gen, Local: local, Peer: peer})
		slot.startWriter()
		go slot.readLoop()
	}()
	return nil
}

// TCPSend queues bytes for the writer goroutine.
func (h *Host) TCPSend(ch TCPChannel, data []byte) error {
	h.mu.Lock()
	slot := h.tcp[ch]
	h.mu.Unlock()
	if slot == nil {
		return ErrSlotClosed
	}
	return slot.send(data)
}

// TCPClose releases the socket. flush drains queued bytes before closing.
func (h *Host) TCPClose(ch TCPChannel, flush bool) {
	h.mu.Lock()
	slot := h.tcp[ch]
	h.tcp[ch] = nil
	h.mu.Unlock()
	if slot == nil {
		return
	}
	if flush {
		slot.drain()
	} else {
		slot.abort()
	}
}

// UDPBind binds an ephemeral IPv4 socket on all interfaces and returns its
// port.
func (h *Host) UDPBind(ch UDPChannel) (uint16, error) {
	if ch != UDPAudio && ch != UDPControl && ch != UDPTiming {
		return 0, fmt.Errorf("raopio: bad UDP channel %d", ch)
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return 0, err
	}
	slot := &udpSlot{ch: ch, host: h, conn: conn, done: make(chan struct{})}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		_ = conn.Close()
		return 0, ErrSlotClosed
	}
	old := h.udp[ch]
	h.udp[ch] = slot
	h.mu.Unlock()
	if old != nil {
		old.close()
	}
	go slot.readLoop()
	return uint16(conn.LocalAddr().(*net.UDPAddr).Port), nil
}

// UDPSend sends one datagram from the bound socket.
func (h *Host) UDPSend(ch UDPChannel, to Endpoint, data []byte) error {
	h.mu.Lock()
	slot := h.udp[ch]
	h.mu.Unlock()
	if slot == nil {
		return ErrSlotClosed
	}
	_, err := slot.conn.WriteToUDP(data, &net.UDPAddr{IP: net.ParseIP(to.IP), Port: int(to.Port)})
	return err
}

// UDPClose releases a bound UDP socket.
func (h *Host) UDPClose(ch UDPChannel) {
	h.mu.Lock()
	slot := h.udp[ch]
	h.udp[ch] = nil
	h.mu.Unlock()
	if slot != nil {
		slot.close()
	}
}

func endpointOf(addr net.Addr) Endpoint {
	switch a := addr.(type) {
	case *net.TCPAddr:
		return Endpoint{IP: a.IP.String(), Port: uint16(a.Port)}
	case *net.UDPAddr:
		return Endpoint{IP: a.IP.String(), Port: uint16(a.Port)}
	default:
		host, port, _ := net.SplitHostPort(addr.String())
		var p int
		_, _ = fmt.Sscanf(port, "%d", &p)
		return Endpoint{IP: host, Port: uint16(p)}
	}
}

type tcpSlot struct {
	ch   TCPChannel
	host *Host
	gen  uint64

	mu       sync.Mutex
	cond     *sync.Cond
	queue    [][]byte
	conn     net.Conn
	draining bool
	aborted  bool
}

func (s *tcpSlot) initCond() {
	s.mu.Lock()
	if s.cond == nil {
		s.cond = sync.NewCond(&s.mu)
	}
	s.mu.Unlock()
}

func (s *tcpSlot) attach(conn net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.aborted {
		return false
	}
	s.conn = conn
	if s.cond == nil {
		s.cond = sync.NewCond(&s.mu)
	}
	return true
}

func (s *tcpSlot) isAborted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.aborted
}

// isClosing reports whether this endpoint is being closed deliberately, in
// which case its read loop must stay quiet.
func (s *tcpSlot) isClosing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.aborted || s.draining
}

func (s *tcpSlot) send(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.aborted || s.draining || s.conn == nil && s.cond == nil {
		if s.conn == nil && !s.aborted && !s.draining {
			// Connect is still in flight; the sender only writes after the
			// connected event, so this is a programming error worth surfacing.
			return ErrSlotClosed
		}
		return ErrSlotClosed
	}
	s.queue = append(s.queue, append([]byte(nil), data...))
	if s.cond != nil {
		s.cond.Signal()
	}
	return nil
}

func (s *tcpSlot) startWriter() {
	s.initCond()
	go func() {
		for {
			s.mu.Lock()
			for s.cond != nil && len(s.queue) == 0 && !s.draining && !s.aborted {
				s.cond.Wait()
			}
			if s.aborted {
				s.mu.Unlock()
				return
			}
			if len(s.queue) == 0 && s.draining {
				conn := s.conn
				s.mu.Unlock()
				if conn != nil {
					_ = conn.Close()
				}
				return
			}
			data := s.queue[0]
			s.queue = s.queue[1:]
			conn := s.conn
			s.mu.Unlock()
			if conn == nil {
				return
			}
			if _, err := conn.Write(data); err != nil {
				_ = conn.Close()
				return
			}
		}
	}()
}

func (s *tcpSlot) readLoop() {
	buf := make([]byte, 64*1024)
	for {
		n, err := s.conn.Read(buf)
		if n > 0 {
			s.host.emit(Event{Kind: EventTCPData, TCP: s.ch, Gen: s.gen, Data: buf[:n]})
		}
		if err != nil {
			// A close we initiated is not a notification; its late event must
			// not reach a session that already reconnected.
			if s.isClosing() {
				return
			}
			reason := ""
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				reason = err.Error()
			}
			s.host.emit(Event{Kind: EventTCPClosed, TCP: s.ch, Gen: s.gen, Err: reason})
			return
		}
	}
}

func (s *tcpSlot) drain() {
	s.mu.Lock()
	s.draining = true
	if s.cond != nil {
		s.cond.Broadcast()
	}
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		// Connect still in flight; abort it so it does not open later.
		s.abort()
	}
}

func (s *tcpSlot) abort() {
	s.mu.Lock()
	if s.aborted {
		s.mu.Unlock()
		return
	}
	s.aborted = true
	if s.cond != nil {
		s.cond.Broadcast()
	}
	conn := s.conn
	s.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

type udpSlot struct {
	ch   UDPChannel
	host *Host
	conn *net.UDPConn
	once sync.Once
	done chan struct{}
}

func (s *udpSlot) readLoop() {
	buf := make([]byte, 64*1024)
	for {
		n, from, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		s.host.emit(Event{
			Kind: EventUDPDatagram,
			UDP:  s.ch,
			Peer: Endpoint{IP: from.IP.String(), Port: uint16(from.Port)},
			Data: buf[:n],
		})
	}
}

func (s *udpSlot) close() {
	s.once.Do(func() {
		close(s.done)
		_ = s.conn.Close()
	})
}
