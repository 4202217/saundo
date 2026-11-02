// SPDX-License-Identifier: Apache-2.0
//
// Ported from airplay2-sender-cpp (Apache-2.0), src/airplay_crypto.cpp,
// section "binary plist". Canonical Apple bplist00: "bplist00" header, object
// table, offset table, 32-byte trailer. Enough of the grammar to round-trip
// the AirPlay 2 SETUP request/response dictionaries; not a general libplist
// replacement.

package crypto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"unicode/utf16"
)

// Encode serializes v as bplist00. Accepted types: map[string]any,
// []any, string, []byte, bool, int, int64, float64.
func Encode(v any) ([]byte, error) {
	enc := &bpEncoder{}
	root, err := enc.add(v)
	if err != nil {
		return nil, err
	}
	if root != 0 {
		return nil, errors.New("crypto: bplist root must be object 0")
	}

	out := make([]byte, 0, 8+1024+32)
	out = append(out, "bplist00"...)

	offsets := make([]uint64, 0, len(enc.objects))
	for _, obj := range enc.objects {
		offsets = append(offsets, uint64(len(out)))
		out = append(out, obj...)
	}

	offsetTableStart := len(out)
	const offsetSize, refSize = 4, 4
	for _, off := range offsets {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(off))
		out = append(out, b[:]...)
	}

	trailer := make([]byte, 32)
	trailer[6] = offsetSize
	trailer[7] = refSize
	binary.BigEndian.PutUint64(trailer[8:16], uint64(len(enc.objects)))
	binary.BigEndian.PutUint64(trailer[16:24], 0) // top object
	binary.BigEndian.PutUint64(trailer[24:32], uint64(offsetTableStart))
	out = append(out, trailer...)
	return out, nil
}

type bpEncoder struct {
	objects [][]byte
}

// add flattens v into the object table and returns its index. The slot is
// reserved before recursing so children order after their parent.
func (e *bpEncoder) add(v any) (int, error) {
	idx := len(e.objects)
	e.objects = append(e.objects, nil)
	obj, err := e.encodeObject(v)
	if err != nil {
		return 0, err
	}
	e.objects[idx] = obj
	return idx, nil
}

func (e *bpEncoder) encodeObject(v any) ([]byte, error) {
	switch val := v.(type) {
	case bool:
		if val {
			return []byte{0x09}, nil
		}
		return []byte{0x08}, nil
	case int:
		return bpEncodeInt(nil, int64(val)), nil
	case int64:
		return bpEncodeInt(nil, val), nil
	case float64:
		obj := []byte{0x23}
		return binary.BigEndian.AppendUint64(obj, math.Float64bits(val)), nil
	case string:
		if bpIsASCII(val) {
			obj := make([]byte, 0, 1+len(val))
			obj = bpEncodeMarkerLen(obj, 0x50, len(val))
			return append(obj, val...), nil
		}
		// Non-ASCII strings are UTF-16BE, marker 0x60, length in code units.
		units := utf16.Encode([]rune(val))
		obj := make([]byte, 0, 1+2*len(units))
		obj = bpEncodeMarkerLen(obj, 0x60, len(units))
		for _, u := range units {
			obj = binary.BigEndian.AppendUint16(obj, u)
		}
		return obj, nil
	case []byte:
		obj := make([]byte, 0, 1+len(val))
		obj = bpEncodeMarkerLen(obj, 0x40, len(val))
		return append(obj, val...), nil
	case []any:
		refs := make([]int, 0, len(val))
		for _, child := range val {
			ref, err := e.add(child)
			if err != nil {
				return nil, err
			}
			refs = append(refs, ref)
		}
		obj := bpEncodeMarkerLen(nil, 0xA0, len(refs))
		for _, ref := range refs {
			obj = bpAppendRef(obj, ref)
		}
		return obj, nil
	case map[string]any:
		keys := make([]string, 0, len(val))
		for key := range val {
			keys = append(keys, key)
		}
		// Go map order is random; sort so encoding a given value is deterministic.
		sort.Strings(keys)
		krefs := make([]int, 0, len(keys))
		vrefs := make([]int, 0, len(keys))
		for _, key := range keys {
			kref, err := e.add(key)
			if err != nil {
				return nil, err
			}
			krefs = append(krefs, kref)
			vref, err := e.add(val[key])
			if err != nil {
				return nil, err
			}
			vrefs = append(vrefs, vref)
		}
		obj := bpEncodeMarkerLen(nil, 0xD0, len(keys))
		for _, ref := range krefs {
			obj = bpAppendRef(obj, ref)
		}
		for _, ref := range vrefs {
			obj = bpAppendRef(obj, ref)
		}
		return obj, nil
	default:
		return nil, fmt.Errorf("crypto: bplist unsupported type %T", v)
	}
}

func bpAppendRef(obj []byte, ref int) []byte {
	return binary.BigEndian.AppendUint32(obj, uint32(ref))
}

func bpIsASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func bpEncodeMarkerLen(obj []byte, marker byte, length int) []byte {
	if length < 15 {
		return append(obj, marker|byte(length))
	}
	obj = append(obj, marker|0x0F)
	return bpEncodeInt(obj, int64(length))
}

// bpEncodeInt picks the smallest power-of-two width that holds the value;
// negative values always use 8 bytes.
func bpEncodeInt(obj []byte, value int64) []byte {
	switch {
	case value >= 0 && value <= 0xFF:
		return append(obj, 0x10, byte(value))
	case value >= 0 && value <= 0xFFFF:
		return binary.BigEndian.AppendUint16(append(obj, 0x11), uint16(value))
	case value >= 0 && value <= 0xFFFFFFFF:
		return binary.BigEndian.AppendUint32(append(obj, 0x12), uint32(value))
	default:
		return binary.BigEndian.AppendUint64(append(obj, 0x13), uint64(value))
	}
}

// Decode parses a bplist00 document. Malformed or hostile input (bad
// magic, inconsistent size fields, counts that overrun the buffer, cyclic or
// over-deep references) yields an error, never a panic or a huge allocation.
func Decode(data []byte) (any, error) {
	d := &bpDecoder{data: data}
	if err := d.parseTrailer(); err != nil {
		return nil, err
	}
	return d.object(0, 0)
}

type bpDecoder struct {
	data             []byte
	offsetSize       int
	refSize          int
	numObjects       uint64
	offsetTableStart uint64
	offsets          []uint64
}

func (d *bpDecoder) parseTrailer() error {
	if len(d.data) < 8+32 {
		return errors.New("crypto: bplist too short")
	}
	if string(d.data[:8]) != "bplist00" {
		return errors.New("crypto: bplist bad magic")
	}
	tr := uint64(len(d.data) - 32)
	d.offsetSize = int(d.data[tr+6])
	d.refSize = int(d.data[tr+7])
	d.numObjects = d.readBE(tr+8, 8)
	topObject := d.readBE(tr+16, 8)
	d.offsetTableStart = d.readBE(tr+24, 8)

	validSize := func(s int) bool { return s == 1 || s == 2 || s == 4 || s == 8 }
	if !validSize(d.offsetSize) || !validSize(d.refSize) {
		return errors.New("crypto: bplist bad size fields")
	}
	if topObject != 0 {
		return errors.New("crypto: bplist top object is not 0")
	}
	// Bound numObjects before any sizing: each object needs at least one
	// byte, and the offset table must hold numObjects entries.
	if d.offsetTableStart > uint64(len(d.data)) {
		return errors.New("crypto: bplist offset table out of range")
	}
	if d.numObjects > uint64(len(d.data)) {
		return errors.New("crypto: bplist object count out of range")
	}
	if d.numObjects > (uint64(len(d.data))-d.offsetTableStart)/uint64(d.offsetSize) {
		return errors.New("crypto: bplist offset table too small")
	}
	d.offsets = make([]uint64, d.numObjects)
	for i := uint64(0); i < d.numObjects; i++ {
		d.offsets[i] = d.readBE(d.offsetTableStart+i*uint64(d.offsetSize), uint64(d.offsetSize))
	}
	return nil
}

func (d *bpDecoder) readBE(at, n uint64) uint64 {
	var v uint64
	for i := uint64(0); i < n; i++ {
		if at+i >= uint64(len(d.data)) {
			return 0
		}
		v = v<<8 | uint64(d.data[at+i])
	}
	return v
}

func (d *bpDecoder) readBEChecked(at, n uint64) (uint64, error) {
	if at+n > uint64(len(d.data)) || n == 0 {
		return 0, fmt.Errorf("crypto: bplist read out of range at %d+%d", at, n)
	}
	return d.readBE(at, n), nil
}

func (d *bpDecoder) object(ref uint64, depth int) (any, error) {
	if depth > 32 {
		return nil, errors.New("crypto: bplist nesting too deep")
	}
	if ref >= d.numObjects {
		return nil, fmt.Errorf("crypto: bplist object ref %d out of range", ref)
	}
	pos := d.offsets[ref]
	if pos >= uint64(len(d.data)) {
		return nil, fmt.Errorf("crypto: bplist object %d offset out of range", ref)
	}
	marker := d.data[pos]
	pos++
	hi := marker & 0xF0
	lo := marker & 0x0F

	readCount := func() (uint64, uint64, error) {
		// Returns (count, new pos).
		if lo != 0x0F {
			return uint64(lo), pos, nil
		}
		if pos >= uint64(len(d.data)) {
			return 0, 0, errors.New("crypto: bplist truncated extended count")
		}
		im := d.data[pos]
		pos++
		n := uint64(1) << (im & 0x0F)
		c, err := d.readBEChecked(pos, n)
		if err != nil {
			return 0, 0, err
		}
		pos += n
		return c, pos, nil
	}

	switch hi {
	case 0x00:
		// 0x08 false, 0x09 true; other singletons (null, fill) read as false.
		return marker == 0x09, nil
	case 0x10:
		n := uint64(1) << lo
		v, err := d.readBEChecked(pos, n)
		if err != nil {
			return nil, err
		}
		return int64(v), nil
	case 0x20:
		n := uint64(1) << lo
		bits, err := d.readBEChecked(pos, n)
		if err != nil {
			return nil, err
		}
		switch n {
		case 8:
			return math.Float64frombits(bits), nil
		case 4:
			return float64(math.Float32frombits(uint32(bits))), nil
		default:
			return nil, fmt.Errorf("crypto: bplist real width %d", n)
		}
	case 0x40:
		cnt, newPos, err := readCount()
		if err != nil {
			return nil, err
		}
		pos = newPos
		if cnt > uint64(len(d.data))-pos {
			return nil, errors.New("crypto: bplist data length out of range")
		}
		return append([]byte(nil), d.data[pos:pos+cnt]...), nil
	case 0x50:
		cnt, newPos, err := readCount()
		if err != nil {
			return nil, err
		}
		pos = newPos
		if cnt > uint64(len(d.data))-pos {
			return nil, errors.New("crypto: bplist string length out of range")
		}
		return string(d.data[pos : pos+cnt]), nil
	case 0x60:
		// UTF-16BE string; cnt is the number of code units.
		cnt, newPos, err := readCount()
		if err != nil {
			return nil, err
		}
		pos = newPos
		if pos > uint64(len(d.data)) || cnt > (uint64(len(d.data))-pos)/2 {
			return nil, errors.New("crypto: bplist utf16 length out of range")
		}
		units := make([]uint16, cnt)
		for i := range units {
			at := pos + 2*uint64(i)
			units[i] = binary.BigEndian.Uint16(d.data[at : at+2])
		}
		return string(utf16.Decode(units)), nil
	case 0xA0:
		cnt, newPos, err := readCount()
		if err != nil {
			return nil, err
		}
		pos = newPos
		// An array cannot reference more objects than exist, and its ref
		// table must fit.
		if cnt > d.numObjects || pos+cnt*uint64(d.refSize) > uint64(len(d.data)) {
			return nil, errors.New("crypto: bplist array out of range")
		}
		arr := make([]any, 0, cnt)
		for i := uint64(0); i < cnt; i++ {
			r := d.readBE(pos+i*uint64(d.refSize), uint64(d.refSize))
			child, err := d.object(r, depth+1)
			if err != nil {
				return nil, err
			}
			arr = append(arr, child)
		}
		return arr, nil
	case 0xD0:
		cnt, newPos, err := readCount()
		if err != nil {
			return nil, err
		}
		pos = newPos
		// Key table plus value table must both fit, and neither can
		// reference more objects than exist.
		if cnt > d.numObjects || pos+2*cnt*uint64(d.refSize) > uint64(len(d.data)) {
			return nil, errors.New("crypto: bplist dict out of range")
		}
		dict := make(map[string]any, cnt)
		for i := uint64(0); i < cnt; i++ {
			kr := d.readBE(pos+i*uint64(d.refSize), uint64(d.refSize))
			vr := d.readBE(pos+(cnt+i)*uint64(d.refSize), uint64(d.refSize))
			keyObj, err := d.object(kr, depth+1)
			if err != nil {
				return nil, err
			}
			key, _ := keyObj.(string)
			val, err := d.object(vr, depth+1)
			if err != nil {
				return nil, err
			}
			dict[key] = val
		}
		return dict, nil
	default:
		return nil, fmt.Errorf("crypto: bplist marker 0x%02x unsupported", marker)
	}
}
