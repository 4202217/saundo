package sender

import raopio "saundo/internal/airplay/io"

// staleDial reports whether a completion belongs to a dial the sender already
// replaced. The host numbers every dial from one, so a completion that does not
// carry the current token is stale: unknown never means current.
func staleDial(gen, dial uint64) bool {
	return gen != dial
}

// HandleEvent feeds one host notification into the sender.
func (s *Sender) HandleEvent(ev raopio.Event) {
	// Ignore late bytes or a close from a socket the sender already replaced.
	switch ev.TCP {
	case raopio.TCPControl:
		switch ev.Kind {
		case raopio.EventTCPConnected:
			if staleDial(ev.Gen, s.controlDialGen) {
				return
			}
			s.controlGen = ev.Gen
		case raopio.EventTCPConnectFailed:
			if staleDial(ev.Gen, s.controlDialGen) {
				return
			}
		case raopio.EventTCPData, raopio.EventTCPClosed:
			if ev.Gen != s.controlGen {
				return
			}
		}
	case raopio.TCPEvent:
		switch ev.Kind {
		case raopio.EventTCPConnected:
			if staleDial(ev.Gen, s.eventDialGen) {
				return
			}
			s.eventGen = ev.Gen
		case raopio.EventTCPConnectFailed:
			if staleDial(ev.Gen, s.eventDialGen) {
				return
			}
		case raopio.EventTCPData, raopio.EventTCPClosed:
			if ev.Gen != s.eventGen {
				return
			}
		}
	}
	switch ev.Kind {
	case raopio.EventTCPConnected:
		if ev.TCP == raopio.TCPEvent {
			s.eventChannelUp = true
			if s.state != StateIdle {
				s.info("event channel open (%s:%d)", ev.Peer.IP, ev.Peer.Port)
			}
			// RECORD waits for this result, so a refused event port fails
			// the handshake instead of a stream that dies about 30 s later.
			if s.pairStage == pairAP2Event && s.state != StateIdle {
				s.sendAP2Record()
			}
			return
		}
		if s.state != StateConnecting {
			return
		}
		s.rtspConnected = true
		s.localIP = ev.Local.IP
		s.hostIP = ev.Peer.IP
		s.info("RTSP connected to %s:%d", s.host, ev.Peer.Port)
		// The socket itself proves the target is this host, even when
		// discovery could not classify it (a manual row or a failed
		// interface read). Refuse before pairing; the receiver rejects the
		// session SETUP anyway, and a stream to its own host would loop.
		if s.localIP != "" && s.localIP == s.hostIP {
			s.fail("the local receiver cannot be a target")
			return
		}
		s.beginAuthChain()
	case raopio.EventTCPConnectFailed:
		if ev.TCP == raopio.TCPEvent {
			s.eventChannelUp = false
			if s.state != StateIdle {
				why := ev.Err
				if why == "" {
					why = "connection failed"
				}
				s.fail(fmtError("The receiver's event channel did not connect: %s", why))
			}
			return
		}
		if s.state != StateIdle {
			why := ev.Err
			if why == "" {
				why = "Connection failed"
			}
			s.fail(why)
		}
	case raopio.EventTCPData:
		if s.state == StateIdle {
			// A stopped session keeps no pending requests or keys. Late
			// bytes from its socket must not re-enter the state machine and
			// fail the session a second time.
			return
		}
		switch ev.TCP {
		case raopio.TCPControl:
			s.onRTSPData(ev.Data)
		default:
			s.onEventData(ev.Data)
		}
	case raopio.EventTCPClosed:
		if ev.TCP == raopio.TCPEvent {
			// The receiver tears a session without a working event channel
			// down after roughly 30 s. Clear the health flag so a later
			// clean control close reads as that retryable failure, not as a
			// takeover by another sender.
			s.eventChannelUp = false
			if s.state == StateStreaming || s.state == StateHandshake {
				if ev.Err != "" {
					s.warn("event channel closed: %s", ev.Err)
				} else {
					s.warn("event channel closed")
				}
			}
			return
		}
		s.rtspConnected = false
		if s.state == StateIdle {
			return
		}
		if ev.Err != "" {
			s.fail(ev.Err)
			return
		}
		if s.state == StateStreaming {
			if s.eventChannelUp {
				s.preempt("receiver closed the session")
			} else {
				// The event channel never came up this session: a receiver
				// tears a session without one down after roughly 30 s. That
				// is a connect failure, not a takeover, and stays retryable.
				s.fail("The receiver closed the session without a working event channel")
			}
			return
		}
		wasStarting := s.state != StateStreaming
		s.setState(StateIdle)
		s.stopTimers()
		s.resetStartState()
		s.releaseSockets()
		for i := range s.backlog {
			s.backlog[i] = nil
		}
		for i := range s.backlogSeq {
			s.backlogSeq[i] = -1
		}
		s.info("connection closed by the device")
		if wasStarting && s.events.Launched != nil {
			s.events.Launched(false, "Connection closed by the device")
		}
		if s.events.Closed != nil {
			s.events.Closed()
		}
	case raopio.EventUDPDatagram:
		if s.state == StateIdle {
			return
		}
		switch ev.UDP {
		case raopio.UDPControl:
			s.onControlDatagram(ev.Data, ev.Peer)
		case raopio.UDPTiming:
			s.onTimingDatagram(ev.Data, ev.Peer)
		}
	}
}
