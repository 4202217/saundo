// Package raopio is the sender's outbound I/O seam. The sender owns no
// socket: it asks the host for the six operations below and receives
// everything that happens as Event values on one channel.
package raopio

// TCPChannel identifies one of the session's two TCP sockets.
type TCPChannel uint8

const (
	// TCPControl carries RTSP and HTTP, plaintext before pair-verify and
	// encrypted after it.
	TCPControl TCPChannel = iota
	// TCPEvent carries the reverse AirPlay 2 event channel.
	TCPEvent
	// TCPArtwork carries the plaintext HTTP artwork channel to the receiver's
	// HTTP server, separate from the encrypted RTSP control connection.
	TCPArtwork
)

// UDPChannel identifies one of the session's three UDP sockets.
type UDPChannel uint8

const (
	// UDPAudio carries RTP audio to the receiver.
	UDPAudio UDPChannel = iota
	// UDPControl carries sync packets and receives retransmit requests.
	UDPControl
	// UDPTiming receives the receiver's NTP requests.
	UDPTiming
)

// Endpoint is an IPv4 address and port.
type Endpoint struct {
	IP   string
	Port uint16
}

// IO is the six-method seam. Every method is safe to call from the sender's
// event-loop goroutine while the host is reading its sockets.
type IO interface {
	TCPConnect(ch TCPChannel, host string, port uint16) error
	TCPSend(ch TCPChannel, data []byte) error
	TCPClose(ch TCPChannel, flush bool)

	UDPBind(ch UDPChannel) (port uint16, err error)
	UDPSend(ch UDPChannel, to Endpoint, data []byte) error
	UDPClose(ch UDPChannel)
}

// EventKind tells the sender what happened.
type EventKind uint8

const (
	EventTCPConnected EventKind = iota
	EventTCPConnectFailed
	EventTCPData
	EventTCPClosed
	EventUDPDatagram
)

// Event is one host notification. Data is owned by the receiver. Gen numbers
// TCP events by connection attempt so a sender can ignore a closed socket's
// late bytes or close notification after it reconnects.
type Event struct {
	Kind  EventKind
	TCP   TCPChannel
	UDP   UDPChannel
	Gen   uint64
	Local Endpoint
	Peer  Endpoint
	Data  []byte
	Err   string
}
