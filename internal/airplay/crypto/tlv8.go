// Package crypto implements the AirPlay and HomeKit wire primitives.
package crypto

import "fmt"

// TLVType is a HomeKit TLV8 type.
type TLVType byte

const (
	TLVMethod        TLVType = 0x00
	TLVIdentifier    TLVType = 0x01
	TLVSalt          TLVType = 0x02
	TLVPublicKey     TLVType = 0x03
	TLVProof         TLVType = 0x04
	TLVEncryptedData TLVType = 0x05
	TLVState         TLVType = 0x06
	TLVError         TLVType = 0x07
	TLVSignature     TLVType = 0x0A
	TLVPermissions   TLVType = 0x0B
	TLVName          TLVType = 0x11
	TLVFlags         TLVType = 0x13
)

// TLV is one HomeKit TLV8 item. Decode joins fragmented values, so a decoded
// item holds the complete value.
type TLV struct {
	Type  TLVType
	Value []byte
}

// EncodeTLV serializes items in order. Values longer than 255 bytes are split
// into consecutive same-type chunks, which HomeKit rejoins on read. An empty
// value still emits one zero-length record.
func EncodeTLV(items []TLV) []byte {
	size := 0
	for _, item := range items {
		size += 2 + len(item.Value)
		if len(item.Value) > 255 {
			size += 2 * ((len(item.Value) - 1) / 255)
		}
	}
	out := make([]byte, 0, size)
	for _, item := range items {
		value := item.Value
		for {
			n := len(value)
			if n > 255 {
				n = 255
			}
			out = append(out, byte(item.Type), byte(n))
			out = append(out, value[:n]...)
			value = value[n:]
			if len(value) == 0 {
				break
			}
		}
	}
	return out
}

// DecodeTLV parses HomeKit TLV8. A tag that immediately follows itself is the
// continuation of a fragmented value and is joined into one item.
func DecodeTLV(data []byte) ([]TLV, error) {
	var items []TLV
	for i := 0; i < len(data); {
		if len(data)-i < 2 {
			return nil, fmt.Errorf("tlv8: truncated header at offset %d", i)
		}
		typ := TLVType(data[i])
		n := int(data[i+1])
		i += 2
		if n > len(data)-i {
			return nil, fmt.Errorf("tlv8: truncated value for type 0x%02x", byte(typ))
		}
		chunk := data[i : i+n]
		i += n
		if len(items) > 0 && items[len(items)-1].Type == typ {
			items[len(items)-1].Value = append(items[len(items)-1].Value, chunk...)
			continue
		}
		items = append(items, TLV{Type: typ, Value: append([]byte(nil), chunk...)})
	}
	return items, nil
}

// GetTLV returns the first value for typ.
func GetTLV(items []TLV, typ TLVType) ([]byte, bool) {
	for _, item := range items {
		if item.Type == typ {
			return item.Value, true
		}
	}
	return nil, false
}
