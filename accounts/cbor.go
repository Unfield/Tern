package accounts

import (
	"encoding/binary"
	"errors"
	"math"
)

// A small CBOR decoder (RFC 8949), for what WebAuthn sends: attestation
// objects and COSE keys. Maps decode to map[any]any, integers to int64,
// byte strings to []byte and text to string.

var errCBOR = errors.New("accounts: malformed CBOR")

// decodeCBOR decodes one item and reports how many bytes it took.
func decodeCBOR(b []byte) (any, int, error) {
	d := &cborDecoder{b: b}
	v, err := d.item(0)
	return v, d.i, err
}

type cborDecoder struct {
	b []byte
	i int
}

func (d *cborDecoder) head() (major byte, arg uint64, indefinite bool, err error) {
	if d.i >= len(d.b) {
		return 0, 0, false, errCBOR
	}
	c := d.b[d.i]
	d.i++
	major, info := c>>5, c&0x1f
	switch {
	case info < 24:
		return major, uint64(info), false, nil
	case info == 31:
		return major, 0, true, nil
	case info > 27:
		return 0, 0, false, errCBOR
	}
	n := 1 << (info - 24) // 1, 2, 4 or 8 bytes
	if d.i+n > len(d.b) {
		return 0, 0, false, errCBOR
	}
	switch n {
	case 1:
		arg = uint64(d.b[d.i])
	case 2:
		arg = uint64(binary.BigEndian.Uint16(d.b[d.i:]))
	case 4:
		arg = uint64(binary.BigEndian.Uint32(d.b[d.i:]))
	case 8:
		arg = binary.BigEndian.Uint64(d.b[d.i:])
	}
	d.i += n
	return major, arg, false, nil
}

func (d *cborDecoder) item(depth int) (any, error) {
	if depth > 16 {
		return nil, errCBOR
	}
	start := d.i
	major, arg, indef, err := d.head()
	if err != nil {
		return nil, err
	}
	if indef && major != 7 {
		return nil, errCBOR // WebAuthn data is definite length
	}
	switch major {
	case 0:
		if arg > math.MaxInt64 {
			return nil, errCBOR
		}
		return int64(arg), nil
	case 1:
		if arg > math.MaxInt64 {
			return nil, errCBOR
		}
		return -1 - int64(arg), nil
	case 2, 3:
		if arg > uint64(len(d.b)-d.i) {
			return nil, errCBOR
		}
		s := d.b[d.i : d.i+int(arg)]
		d.i += int(arg)
		if major == 3 {
			return string(s), nil
		}
		return append([]byte(nil), s...), nil
	case 4:
		if arg > uint64(len(d.b)) {
			return nil, errCBOR
		}
		out := make([]any, 0, arg)
		for range arg {
			v, err := d.item(depth + 1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case 5:
		if arg > uint64(len(d.b)) {
			return nil, errCBOR
		}
		out := make(map[any]any, arg)
		for range arg {
			k, err := d.item(depth + 1)
			if err != nil {
				return nil, err
			}
			if _, ok := k.([]any); ok {
				return nil, errCBOR
			}
			if _, ok := k.(map[any]any); ok {
				return nil, errCBOR
			}
			if bk, ok := k.([]byte); ok {
				k = string(bk)
			}
			v, err := d.item(depth + 1)
			if err != nil {
				return nil, err
			}
			out[k] = v
		}
		return out, nil
	case 6:
		return d.item(depth + 1) // a tag: its content
	case 7:
		switch {
		case arg == 20:
			return false, nil
		case arg == 21:
			return true, nil
		case arg == 22, arg == 23:
			return nil, nil
		case d.i-start > 1: // half, single and double floats
			return float64(arg), nil
		}
		return nil, errCBOR
	}
	return nil, errCBOR
}
