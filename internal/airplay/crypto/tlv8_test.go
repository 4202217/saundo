package crypto

import (
	"bytes"
	"testing"
)

func TestTLV8RoundTrip(t *testing.T) {
	publicKey := make([]byte, 384)
	for i := range publicKey {
		publicKey[i] = byte(i)
	}
	items := []TLV{
		{Type: TLVMethod, Value: []byte{0}},
		{Type: TLVState, Value: []byte{1}},
		{Type: TLVPublicKey, Value: publicKey},
		{Type: TLVName, Value: []byte("saundo")},
		{Type: TLVFlags, Value: []byte{0x10}},
		{Type: TLVProof, Value: nil},
	}

	encoded := EncodeTLV(items)
	decoded, err := DecodeTLV(encoded)
	if err != nil {
		t.Fatalf("DecodeTLV: %v", err)
	}
	if len(decoded) != len(items) {
		t.Fatalf("decoded %d items, want %d", len(decoded), len(items))
	}
	for i := range items {
		if decoded[i].Type != items[i].Type {
			t.Fatalf("item %d type = 0x%02x, want 0x%02x", i, decoded[i].Type, items[i].Type)
		}
		if !bytes.Equal(decoded[i].Value, items[i].Value) {
			t.Fatalf("item %d value = %x, want %x", i, decoded[i].Value, items[i].Value)
		}
	}
}

func TestTLV8FragmentationBoundaries(t *testing.T) {
	for _, length := range []int{0, 1, 254, 255, 256, 510, 511, 512, 1024} {
		value := bytes.Repeat([]byte{0xAB}, length)
		encoded := EncodeTLV([]TLV{{Type: TLVPublicKey, Value: value}})
		decoded, err := DecodeTLV(encoded)
		if err != nil {
			t.Fatalf("length %d: DecodeTLV: %v", length, err)
		}
		if len(decoded) != 1 {
			t.Fatalf("length %d: got %d items, want 1", length, len(decoded))
		}
		if !bytes.Equal(decoded[0].Value, value) {
			t.Fatalf("length %d: value mismatch", length)
		}
	}
}

func TestTLV8JoinsConsecutiveSameType(t *testing.T) {
	data := []byte{
		byte(TLVPublicKey), 2, 0xAA, 0xBB,
		byte(TLVPublicKey), 1, 0xCC,
		byte(TLVState), 1, 0x03,
	}
	items, err := DecodeTLV(data)
	if err != nil {
		t.Fatalf("DecodeTLV: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if !bytes.Equal(items[0].Value, []byte{0xAA, 0xBB, 0xCC}) {
		t.Fatalf("joined value = %x", items[0].Value)
	}
}

func TestTLV8Truncated(t *testing.T) {
	cases := [][]byte{
		{byte(TLVState)},
		{byte(TLVState), 2, 0x01},
	}
	for _, data := range cases {
		if _, err := DecodeTLV(data); err == nil {
			t.Fatalf("DecodeTLV(%x) succeeded, want error", data)
		}
	}
}

func TestTLV8EmptyInput(t *testing.T) {
	items, err := DecodeTLV(nil)
	if err != nil {
		t.Fatalf("DecodeTLV(nil): %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("got %d items, want 0", len(items))
	}
}
