package js

import (
	"unicode/utf16"
	"unicode/utf8"
)

// JavaScript strings are sequences of UTF-16 code units. Values keep them
// as UTF-8 Go strings; a lone surrogate, which UTF-8 can't hold, is kept as
// its 3-byte WTF-8 encoding. ASCII strings, the common case, take fast paths.

// Units decodes s into UTF-16 code units.
func Units(s string) []uint16 {
	out := make([]uint16, 0, len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			out = append(out, uint16(c))
			i++
			continue
		}
		// A WTF-8 surrogate: ED A0–BF xx.
		if c == 0xED && i+2 < len(s) && s[i+1] >= 0xA0 && s[i+1] <= 0xBF {
			out = append(out, 0xD000|uint16(s[i+1]&0x3F)<<6|uint16(s[i+2]&0x3F))
			i += 3
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r >= 0x10000 {
			a, b := utf16.EncodeRune(r)
			out = append(out, uint16(a), uint16(b))
		} else {
			out = append(out, uint16(r))
		}
		i += size
	}
	return out
}

// FromUnits encodes UTF-16 code units, pairing surrogates where they pair.
func FromUnits(u []uint16) string {
	b := make([]byte, 0, len(u))
	for i := 0; i < len(u); i++ {
		c := u[i]
		switch {
		case c < 0x80:
			b = append(b, byte(c))
		case c >= 0xD800 && c <= 0xDBFF && i+1 < len(u) && u[i+1] >= 0xDC00 && u[i+1] <= 0xDFFF:
			b = utf8.AppendRune(b, utf16.DecodeRune(rune(c), rune(u[i+1])))
			i++
		case c >= 0xD800 && c <= 0xDFFF:
			b = append(b, 0xED, byte(0xA0|(c>>6)&0x1F), byte(0x80|c&0x3F))
		default:
			b = utf8.AppendRune(b, rune(c))
		}
	}
	return string(b)
}

// Len16 is a string's length in UTF-16 code units.
func Len16(s string) int {
	if isASCII(s) {
		return len(s)
	}
	return len(Units(s))
}

// Slice16 is s.slice(start, end) after both are clamped to [0, len].
func Slice16(s string, start, end int) string {
	if start >= end {
		return ""
	}
	if isASCII(s) {
		return s[start:end]
	}
	return FromUnits(Units(s)[start:end])
}

// UnitAt is s.charCodeAt(i), or -1 out of range.
func UnitAt(s string, i int) int {
	if i < 0 {
		return -1
	}
	if isASCII(s) {
		if i >= len(s) {
			return -1
		}
		return int(s[i])
	}
	u := Units(s)
	if i >= len(u) {
		return -1
	}
	return int(u[i])
}

// CompareStrings orders strings by UTF-16 code units, as < does.
func CompareStrings(a, b string) int {
	if isASCII(a) && isASCII(b) {
		switch {
		case a < b:
			return -1
		case a > b:
			return 1
		}
		return 0
	}
	ua, ub := Units(a), Units(b)
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			if ua[i] < ub[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(ua) < len(ub):
		return -1
	case len(ua) > len(ub):
		return 1
	}
	return 0
}

// Index16 is s.indexOf(sub, from).
func Index16(s, sub string, from int) int {
	if isASCII(s) && isASCII(sub) {
		if from < 0 {
			from = 0
		}
		if from > len(s) {
			if sub == "" {
				return len(s)
			}
			return -1
		}
		for i := from; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return i
			}
		}
		return -1
	}
	u, v := Units(s), Units(sub)
	if from < 0 {
		from = 0
	}
	if from > len(u) {
		from = len(u)
	}
outer:
	for i := from; i+len(v) <= len(u); i++ {
		for j := range v {
			if u[i+j] != v[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}
