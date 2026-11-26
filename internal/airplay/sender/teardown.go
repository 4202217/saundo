package sender

import raopio "saundo/internal/airplay/io"

// fail ends a session that cannot continue and reports it. The session is
// marked torn down so the pacer burst or the prefill loop it was called from
// stops sending instead of failing the same dead key or socket again.
func (s *Sender) fail(why string) {
	s.warn("RAOP error: %s", why)
	wasStarting := s.state != StateStreaming
	s.setState(StateIdle)
	s.tornDown = true
	s.stopTimers()
	s.resetStartState()
	s.io.TCPClose(raopio.TCPControl, false)
	s.rtspConnected = false
	s.releaseSockets()
	s.wipeSecrets()
	if wasStarting && s.events.Launched != nil {
		s.events.Launched(false, why)
	}
	if s.events.Closed != nil {
		s.events.Closed()
	}
}

// preempt ends a streaming session that the receiver handed to another
// sender. The receiver closes the displaced control channel cleanly, so this
// is a terminal outcome, not a retryable failure.
func (s *Sender) preempt(why string) {
	s.warn("session taken over: %s", why)
	s.setState(StateIdle)
	s.tornDown = true
	s.stopTimers()
	s.resetStartState()
	s.io.TCPClose(raopio.TCPControl, false)
	s.rtspConnected = false
	s.releaseSockets()
	s.wipeSecrets()
	for i := range s.backlog {
		s.backlog[i] = nil
	}
	for i := range s.backlogSeq {
		s.backlogSeq[i] = -1
	}
	if s.events.Preempted != nil {
		s.events.Preempted(why)
	}
	if s.events.Closed != nil {
		s.events.Closed()
	}
}
