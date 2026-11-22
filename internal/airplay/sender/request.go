package sender

// header is one RTSP header in wire order.
type header struct{ key, value string }

type pendingReq struct {
	method string
	// uri is the request path, so reply handling can tell /feedback from a
	// POST /command. Pairing requests leave it empty.
	uri string
	// cseq is the CSeq this request was sent with, 0 when the pairing POSTs
	// that carry no CSeq header queued it.
	cseq  int
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
