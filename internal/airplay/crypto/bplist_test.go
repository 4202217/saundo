package crypto

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

func TestBPListRoundTripAllTypes(t *testing.T) {
	// Encode accepts int and int64; Decode always returns int64, so the
	// expected map holds int64 everywhere.
	encodedValue := map[string]any{
		"string":  "Pair-Setup",
		"data":    []byte{0x01, 0x02, 0xFE, 0xFF},
		"boolT":   true,
		"boolF":   false,
		"int":     int(42),
		"int64":   int64(352),
		"big":     int64(1 << 40),
		"neg":     int64(-27),
		"float":   float64(-27.5),
		"empty":   "",
		"nested":  map[string]any{"timingProtocol": "NTP", "spf": int64(352)},
		"array":   []any{"a", int64(1), true, []byte{0x09}},
		"emptArr": []any{},
		"emptMap": map[string]any{},
	}
	want := map[string]any{
		"string":  "Pair-Setup",
		"data":    []byte{0x01, 0x02, 0xFE, 0xFF},
		"boolT":   true,
		"boolF":   false,
		"int":     int64(42),
		"int64":   int64(352),
		"big":     int64(1 << 40),
		"neg":     int64(-27),
		"float":   float64(-27.5),
		"empty":   "",
		"nested":  map[string]any{"timingProtocol": "NTP", "spf": int64(352)},
		"array":   []any{"a", int64(1), true, []byte{0x09}},
		"emptArr": []any{},
		"emptMap": map[string]any{},
	}

	encoded, err := Encode(encodedValue)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got %#v\nwant %#v", got, want)
	}
}

func TestBPListHeaderTrailerLayout(t *testing.T) {
	// One-entry dict: {"a": true}. Hand-computed layout:
	// object 0: 0xD1, keyref 00000001, valref 00000002 (9 bytes)
	// object 1: 0x51 'a' (2 bytes), object 2: 0x09 (1 byte)
	encoded, err := Encode(map[string]any{"a": true})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.HasPrefix(encoded, []byte("bplist00")) {
		t.Fatalf("missing bplist00 header: %x", encoded[:8])
	}
	if !bytes.Equal(encoded[8:17], []byte{0xD1, 0, 0, 0, 1, 0, 0, 0, 2}) {
		t.Fatalf("root dict bytes = %x", encoded[8:17])
	}
	if !bytes.Equal(encoded[17:19], []byte{0x51, 'a'}) {
		t.Fatalf("key bytes = %x", encoded[17:19])
	}
	if encoded[19] != 0x09 {
		t.Fatalf("bool byte = %x", encoded[19])
	}
	if len(encoded) != 8+9+2+1+12+32 {
		t.Fatalf("total %d bytes, want 64", len(encoded))
	}
	// Offset table at byte 20, three 4-byte big-endian offsets.
	offsets := encoded[20:32]
	if !bytes.Equal(offsets, []byte{
		0, 0, 0, 8,
		0, 0, 0, 17,
		0, 0, 0, 19,
	}) {
		t.Fatalf("offset table = %x", offsets)
	}
	trailer := encoded[len(encoded)-32:]
	if trailer[6] != 4 || trailer[7] != 4 {
		t.Fatalf("trailer sizes offsetSize=%d refSize=%d, want 4/4", trailer[6], trailer[7])
	}
	if n := binary.BigEndian.Uint64(trailer[8:16]); n != 3 {
		t.Fatalf("numObjects = %d, want 3", n)
	}
	if top := binary.BigEndian.Uint64(trailer[16:24]); top != 0 {
		t.Fatalf("topObject = %d, want 0", top)
	}
	if off := binary.BigEndian.Uint64(trailer[24:32]); off != 20 {
		t.Fatalf("offsetTableOffset = %d, want 20", off)
	}
}

func TestBPListIntWidths(t *testing.T) {
	cases := []struct {
		value    int64
		expected []byte
	}{
		{0, []byte{0x10, 0x00}},
		{255, []byte{0x10, 0xFF}},
		{256, []byte{0x11, 0x01, 0x00}},
		{65535, []byte{0x11, 0xFF, 0xFF}},
		{65536, []byte{0x12, 0x00, 0x01, 0x00, 0x00}},
		{-1, []byte{0x13, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}},
	}
	for _, tc := range cases {
		encoded, err := Encode(map[string]any{"n": tc.value})
		if err != nil {
			t.Fatalf("value %d: Encode: %v", tc.value, err)
		}
		if !bytes.Contains(encoded, tc.expected) {
			t.Fatalf("value %d: encoding %x lacks %x", tc.value, encoded, tc.expected)
		}
		got, err := Decode(encoded)
		if err != nil {
			t.Fatalf("value %d: Decode: %v", tc.value, err)
		}
		if n := got.(map[string]any)["n"]; n != tc.value {
			t.Fatalf("value %d decoded as %v", tc.value, n)
		}
	}
}

func TestBPListUTF16String(t *testing.T) {
	// Hand-built dict {"k": <UTF-16BE "Pair">}: UTF-16BE strings decode
	// like ASCII strings, including the full code unit.
	data := []byte{
		'b', 'p', 'l', 'i', 's', 't', '0', '0',
		0xD1, 0, 0, 0, 1, 0, 0, 0, 2, // object 0: dict, key ref 1, val ref 2
		0x51, 'k', // object 1: "k"
		0x64, 0x00, 'P', 0x00, 'a', 0x00, 'i', 0x00, 'r', // object 2: UTF-16BE "Pair"
		0, 0, 0, 8, 0, 0, 0, 17, 0, 0, 0, 19, // offset table
	}
	trailer := make([]byte, 32)
	trailer[6], trailer[7] = 4, 4
	binary.BigEndian.PutUint64(trailer[8:16], 3)
	// The offset table is the last 12 bytes before the trailer.
	binary.BigEndian.PutUint64(trailer[24:32], uint64(len(data)-12))
	data = append(data, trailer...)

	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	dict := got.(map[string]any)
	if s := dict["k"]; s != "Pair" {
		t.Fatalf("decoded UTF-16 string = %q, want %q", s, "Pair")
	}
}

func TestBPListNonASCIIString(t *testing.T) {
	cases := []string{"声渡", "声渡 🎵", "Über"}
	for _, name := range cases {
		encoded, err := Encode(name)
		if err != nil {
			t.Fatalf("%q: Encode: %v", name, err)
		}
		if marker := encoded[8] & 0xF0; marker != 0x60 {
			t.Fatalf("%q: root marker %#x, want a UTF-16BE string (0x60)", name, encoded[8])
		}
		got, err := Decode(encoded)
		if err != nil {
			t.Fatalf("%q: Decode: %v", name, err)
		}
		if got != name {
			t.Fatalf("decoded %q, want %q", got, name)
		}
	}
}

func TestBPListTruncatedRejected(t *testing.T) {
	encoded, err := Encode(map[string]any{
		"name":   "HomePod",
		"spf":    int64(352),
		"nested": []any{true, []byte{1, 2, 3}},
	})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	for cut := 0; cut < len(encoded); cut++ {
		if _, err := Decode(encoded[:cut]); err == nil {
			t.Fatalf("prefix of %d/%d bytes decoded without error", cut, len(encoded))
		}
	}
}

func TestBPListMalformedRejected(t *testing.T) {
	valid, err := Encode(map[string]any{"a": []byte("payload"), "b": int64(7)})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	cases := [][]byte{
		{},                             // empty
		bytes.Repeat([]byte{0x00}, 40), // no magic
		[]byte("bplist0" + "0"),        // magic only
		append([]byte(nil), valid[:len(valid)-1]...), // truncated trailer
	}
	// Corrupt numObjects to a huge value.
	huge := append([]byte(nil), valid...)
	trailer := len(huge) - 32
	binary.BigEndian.PutUint64(huge[trailer+8:trailer+16], ^uint64(0))
	cases = append(cases, huge)

	// Corrupt the offset size field.
	badSize := append([]byte(nil), valid...)
	badSize[trailer+6] = 3
	cases = append(cases, badSize)

	// Point the offset table out of range.
	badStart := append([]byte(nil), valid...)
	binary.BigEndian.PutUint64(badStart[trailer+24:trailer+32], ^uint64(0))
	cases = append(cases, badStart)

	// Data object claiming a huge length, hand-built: object 0 is 0x4F
	// (extended count) with an 8-byte count of 0xFFFFFFFFFFFFFFFF.
	hugeCount := []byte{
		'b', 'p', 'l', 'i', 's', 't', '0', '0',
		0x4F, 0x13,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	}
	tr := make([]byte, 32)
	tr[6], tr[7] = 4, 4
	binary.BigEndian.PutUint64(tr[8:16], 1)
	binary.BigEndian.PutUint64(tr[24:32], uint64(len(hugeCount)))
	hugeCount = append(hugeCount, 0, 0, 0, 8) // offset table
	hugeCount = append(hugeCount, tr...)
	cases = append(cases, hugeCount)

	for i, data := range cases {
		if _, err := Decode(data); err == nil {
			t.Fatalf("case %d: malformed input accepted", i)
		}
	}
}

func TestBPListSelfReferenceRejected(t *testing.T) {
	// A single array object whose only child is itself: the depth limit
	// must end the recursion with an error, not a stack overflow.
	data := []byte{
		'b', 'p', 'l', 'i', 's', 't', '0', '0',
		0xA1, 0, 0, 0, 0, // object 0: array of 1, self-referencing
		0, 0, 0, 8, // offset table
	}
	trailer := make([]byte, 32)
	trailer[6], trailer[7] = 4, 4
	binary.BigEndian.PutUint64(trailer[8:16], 1)
	// The offset table is the last 4 bytes before the trailer.
	binary.BigEndian.PutUint64(trailer[24:32], uint64(len(data)-4))
	data = append(data, trailer...)

	if _, err := Decode(data); err == nil {
		t.Fatal("self-referencing array accepted")
	}
}

func TestBPListUnsupportedEncodeType(t *testing.T) {
	if _, err := Encode(map[string]any{"bad": struct{}{}}); err == nil {
		t.Fatal("struct value accepted")
	}
	if _, err := Encode([]any{make(chan int)}); err == nil {
		t.Fatal("chan value accepted")
	}
	// A bare int root is a legal bplist top object.
	got, err := Decode(mustEncode(t, int64(352)))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != int64(352) {
		t.Fatalf("bare int root decoded as %v", got)
	}
}

func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	encoded, err := Encode(v)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return encoded
}
