package sender

import (
	"fmt"
	"strconv"
	"strings"
)

func trimASCII(s string) string {
	return strings.Trim(s, " \t\n\v\f\r")
}

func lowerASCII(s string) string { return strings.ToLower(s) }
func upperASCII(s string) string { return strings.ToUpper(s) }

func headerValue(h map[string]string, key string) string { return h[key] }

func toInt(s string) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return v
}

// toIntOK parses an integer header value and reports whether it parsed. A
// header that is present but unreadable is unknown, not zero.
func toIntOK(s string) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return v, true
}

// contentLengthOf reads a response's Content-Length. A missing header is a
// zero-length body; a negative value parses but lies about the frame, and
// clamping it to zero keeps the stream parseable. A header that is present
// but unreadable is unknown and reports false, which the frame splitters
// treat as a broken stream.
func contentLengthOf(h map[string]string) (int, bool) {
	raw := headerValue(h, "content-length")
	if raw == "" {
		return 0, true
	}
	n, ok := toIntOK(raw)
	if !ok {
		return 0, false
	}
	if n < 0 {
		return 0, true
	}
	return n, true
}

func toUInt16(s string) uint16 {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 16)
	if err != nil {
		return 0
	}
	return uint16(v)
}

func fixed6(v float64) string { return strconv.FormatFloat(v, 'f', 6, 64) }

func stripCRLF(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// ntp2ts and ts2ntp are pyatv's NTP-to-RTP-timestamp conversions.
func ntp2ts(ntp uint64, rate uint32) uint64 { return ((ntp >> 16) * uint64(rate)) >> 16 }

func ts2ntp(ts uint64, rate uint32) uint64 { return ((ts << 16) / uint64(rate)) << 16 }

// dmapTag builds a DMAP item: four ASCII bytes, a big-endian length, and the
// payload.
func dmapTag(code, payload string) string {
	return code + string([]byte{
		byte(len(payload) >> 24), byte(len(payload) >> 16),
		byte(len(payload) >> 8), byte(len(payload)),
	}) + payload
}

func fmtError(format string, args ...any) string { return fmt.Sprintf(format, args...) }
