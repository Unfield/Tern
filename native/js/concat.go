package js

import "unsafe"

// Concatenation. JavaScript code builds long strings with +=, which
// engines make cheap with ropes. Here the result of the last long
// concatenation keeps spare room after it: concatenating onto that same
// string appends in place, so building a page with += is linear, not
// quadratic. Strings already handed out never change, as they only cover
// bytes before the append point.

// concatMin is the length below which a plain Go concatenation is cheaper.
const concatMin = 128

// Concat is a + b for strings.
func (rt *Realm) Concat(a, b string) string {
	switch {
	case b == "":
		return a
	case a == "":
		return b
	}
	n := len(a) + len(b)
	if n < concatMin {
		return a + b
	}
	buf := rt.lastConcat
	if len(a) == len(buf) && unsafe.StringData(a) == unsafe.SliceData(buf) && cap(buf)-len(buf) >= len(b) {
		buf = append(buf, b...)
		rt.lastConcat = buf
		return unsafe.String(unsafe.SliceData(buf), n)
	}
	buf = make([]byte, n, n*2)
	copy(buf, a)
	copy(buf[len(a):], b)
	rt.lastConcat = buf
	return unsafe.String(unsafe.SliceData(buf), n)
}
