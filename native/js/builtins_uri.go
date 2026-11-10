package js

import (
	"strings"
	"unicode/utf8"
)

const uriUnreservedMarks = "-_.!~*'()"
const uriReserved = ";/?:@&=+$,#"

func uriEncode(s string, keep string) string {
	var b strings.Builder
	u := Units(s)
	for i := 0; i < len(u); i++ {
		c := u[i]
		if c < 0x80 && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte(uriUnreservedMarks, byte(c)) >= 0 || strings.IndexByte(keep, byte(c)) >= 0) {
			b.WriteByte(byte(c))
			continue
		}
		var r rune
		switch {
		case c >= 0xDC00 && c <= 0xDFFF:
			panic(URIErrorf("URI malformed"))
		case c >= 0xD800 && c <= 0xDBFF:
			if i+1 >= len(u) || u[i+1] < 0xDC00 || u[i+1] > 0xDFFF {
				panic(URIErrorf("URI malformed"))
			}
			r = (rune(c)-0xD800)<<10 + (rune(u[i+1]) - 0xDC00) + 0x10000
			i++
		default:
			r = rune(c)
		}
		var buf [4]byte
		n := utf8.EncodeRune(buf[:], r)
		for _, x := range buf[:n] {
			b.WriteByte('%')
			b.WriteByte("0123456789ABCDEF"[x>>4])
			b.WriteByte("0123456789ABCDEF"[x&15])
		}
	}
	return b.String()
}

func uriDecode(s string, reserved string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		start := i
		var bytes []byte
		for i < len(s) && s[i] == '%' {
			if i+2 >= len(s) || digitVal(rune(s[i+1])) < 0 || digitVal(rune(s[i+1])) > 15 || digitVal(rune(s[i+2])) < 0 || digitVal(rune(s[i+2])) > 15 {
				panic(URIErrorf("URI malformed"))
			}
			bytes = append(bytes, byte(digitVal(rune(s[i+1]))<<4|digitVal(rune(s[i+2]))))
			i += 3
			if utf8.FullRune(bytes) {
				break
			}
		}
		i--
		if len(bytes) == 1 && bytes[0] < 0x80 {
			if strings.IndexByte(reserved, bytes[0]) >= 0 {
				b.WriteString(s[start : i+1])
			} else {
				b.WriteByte(bytes[0])
			}
			continue
		}
		r, size := utf8.DecodeRune(bytes)
		if r == utf8.RuneError || size != len(bytes) {
			panic(URIErrorf("URI malformed"))
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (rt *Realm) setupURIFunctions(g *Object) {
	uriErr := rt.NewConstructor("URIError", 1, nil, nil, &Object{proto: rt.ErrorPrototype, Class: "Error"})
	uriErr.construct = func(args []Value, nt *Object) Value {
		o := OrdinaryCreateFromConstructor(nt, uriErr.Get("prototype").(*Object))
		o.Class = "Error"
		if m := Arg(args, 0); m != Undefined {
			o.DefineHidden("message", ToString(m))
		}
		return o
	}
	uriErr.Get("prototype").(*Object).DefineHidden("name", "URIError")
	uriErr.SetProto(rt.Error)
	g.DefineHidden("URIError", uriErr)
	rt.method(g, "encodeURIComponent", 1, func(_ Value, args []Value) Value { return uriEncode(ToString(Arg(args, 0)), "") })
	rt.method(g, "encodeURI", 1, func(_ Value, args []Value) Value { return uriEncode(ToString(Arg(args, 0)), uriReserved) })
	rt.method(g, "decodeURIComponent", 1, func(_ Value, args []Value) Value { return uriDecode(ToString(Arg(args, 0)), "") })
	rt.method(g, "decodeURI", 1, func(_ Value, args []Value) Value { return uriDecode(ToString(Arg(args, 0)), uriReserved) })
}
