package sender

// header is one RTSP header in wire order.
type header struct{ key, value string }

// pendingReq is a request awaiting its reply. Replies arrive in request
// order; the CSeq makes the match explicit when the reply carries one.
type pendingReq struct {
	method string
	// uri is the request path, so reply handling can tell /feedback from a
	// POST /command. Pairing requests leave it empty.
	uri string
	// cseq is the CSeq this request was sent with, 0 when the pairing POSTs
	// that carry no CSeq header queued it.
	cseq int
	// label names the request in the reply log: a POST path, content type,
	// and body size. Pairing requests leave it empty.
	label string
	http  bool
	// replay resends the identical AP2 request with a fresh CSeq after a 401
	// digest challenge.
	replay func()
	// authRetry marks a request sent in answer to a challenge; a second 401
	// fails instead of looping.
	authRetry bool
}

func (p pendingReq) describe() string {
	if p.label != "" {
		return p.label
	}
	return p.method
}
