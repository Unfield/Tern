package js

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

func setupJSON() {
	j := NewPlainObject()
	Global.DefineHidden("JSON", j)
	j.DefineHidden(SymbolToStringTag, "JSON")
	method(j, "stringify", 3, func(_ Value, args []Value) Value {
		s, ok := Stringify(Arg(args, 0), Arg(args, 1), Arg(args, 2))
		if !ok {
			return Undefined
		}
		return s
	})
	method(j, "parse", 2, func(_ Value, args []Value) Value {
		v, err := ParseJSON(ToString(Arg(args, 0)))
		if err != nil {
			panic(newError(SyntaxError, err.Error()))
		}
		if r, ok := Arg(args, 1).(*Object); ok && r.call != nil {
			root := NewPlainObject()
			root.Define("", v)
			return internalize(r, root, "")
		}
		return v
	})
}

func internalize(reviver *Object, holder *Object, key string) Value {
	val := holder.Get(key)
	if o, ok := val.(*Object); ok {
		if o.array != nil {
			for i := range *o.array {
				k := Itoa(i)
				nv := internalize(reviver, o, k)
				if nv == Undefined {
					(*o.array)[i] = Undefined
				} else {
					(*o.array)[i] = nv
				}
			}
		} else {
			for _, k := range o.OwnKeys(true, false) {
				nv := internalize(reviver, o, k.(string))
				if nv == Undefined {
					o.Delete(k)
				} else {
					o.Define(k, nv)
				}
			}
		}
	}
	return reviver.call(holder, []Value{key, val})
}

// Stringify is JSON.stringify; ok is false when the result is undefined.
func Stringify(v, replacer, space Value) (string, bool) {
	st := &stringifier{indent: "", stack: nil}
	if r, ok := replacer.(*Object); ok {
		if r.call != nil {
			st.replacer = r
		} else if r.array != nil {
			seen := map[string]bool{}
			for _, e := range *r.array {
				var k string
				switch x := e.(type) {
				case string:
					k = x
				case float64:
					k = NumberToString(x)
				case *Object:
					if x.Class == "String" || x.Class == "Number" {
						k = ToString(x)
					} else {
						continue
					}
				default:
					continue
				}
				if !seen[k] {
					seen[k] = true
					st.allow = append(st.allow, k)
				}
			}
			if st.allow == nil {
				st.allow = []string{}
			}
		}
	}
	if o, ok := space.(*Object); ok && (o.Class == "Number" || o.Class == "String") {
		space = ToPrimitive(o, "default")
	}
	switch s := space.(type) {
	case float64:
		n := int(math.Max(0, math.Min(10, ToIntegerOrInfinity(s))))
		st.gap = strings.Repeat(" ", n)
	case string:
		u := Units(s)
		if len(u) > 10 {
			u = u[:10]
		}
		st.gap = FromUnits(u)
	}
	holder := NewPlainObject()
	holder.Define("", v)
	var b strings.Builder
	if !st.property(&b, holder, "", v) {
		return "", false
	}
	return b.String(), true
}

type stringifier struct {
	replacer    *Object
	allow       []string
	gap, indent string
	stack       []*Object
}

func (st *stringifier) property(b *strings.Builder, holder *Object, key string, v Value) bool {
	if o, ok := v.(*Object); ok {
		if f, ok := o.Get("toJSON").(*Object); ok && f.call != nil {
			v = f.call(o, []Value{key})
		}
	}
	if st.replacer != nil {
		v = st.replacer.call(holder, []Value{key, v})
	}
	if o, ok := v.(*Object); ok {
		switch o.Class {
		case "Number":
			v = ToNumber(o)
		case "String":
			v = ToString(o)
		case "Boolean":
			v = o.Internal.(bool)
		}
	}
	switch x := v.(type) {
	case nullType:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		QuoteJSON(b, x)
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			b.WriteString("null")
		} else {
			b.WriteString(NumberToString(x))
		}
	case *Object:
		if x.call != nil {
			return false
		}
		for _, s := range st.stack {
			if s == x {
				panic(TypeErrorf("Converting circular structure to JSON"))
			}
		}
		st.stack = append(st.stack, x)
		defer func() { st.stack = st.stack[:len(st.stack)-1] }()
		outer := st.indent
		st.indent += st.gap
		defer func() { st.indent = outer }()
		if x.array != nil {
			a := *x.array
			if len(a) == 0 {
				b.WriteString("[]")
				return true
			}
			b.WriteByte('[')
			for i, e := range a {
				if i > 0 {
					b.WriteByte(',')
				}
				if st.gap != "" {
					b.WriteString("\n" + st.indent)
				}
				if !st.property(b, x, Itoa(i), e) {
					b.WriteString("null")
				}
			}
			if st.gap != "" {
				b.WriteString("\n" + outer)
			}
			b.WriteByte(']')
			return true
		}
		keys := st.allow
		if keys == nil {
			for _, k := range x.OwnKeys(true, false) {
				keys = append(keys, k.(string))
			}
		}
		b.WriteByte('{')
		any := false
		for _, k := range keys {
			pv := x.Get(k)
			var part strings.Builder
			if !st.property(&part, x, k, pv) {
				continue
			}
			if any {
				b.WriteByte(',')
			}
			if st.gap != "" {
				b.WriteString("\n" + st.indent)
			}
			QuoteJSON(b, k)
			b.WriteByte(':')
			if st.gap != "" {
				b.WriteByte(' ')
			}
			b.WriteString(part.String())
			any = true
		}
		if any && st.gap != "" {
			b.WriteString("\n" + outer)
		}
		b.WriteByte('}')
	default:
		return false // undefined and symbols
	}
	return true
}

// QuoteJSON writes s as a JSON string, as JSON.stringify quotes it: lone
// surrogates become \u escapes.
func QuoteJSON(b *strings.Builder, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '"':
			b.WriteString(`\"`)
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\f':
			b.WriteString(`\f`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20:
			b.WriteString(`\u00`)
			b.WriteByte("0123456789abcdef"[c>>4])
			b.WriteByte("0123456789abcdef"[c&15])
		case c == 0xED && i+2 < len(s) && s[i+1] >= 0xA0 && s[i+1] <= 0xBF:
			u := 0xD000 | uint16(s[i+1]&0x3F)<<6 | uint16(s[i+2]&0x3F)
			b.WriteString(`\u` + strconv.FormatUint(uint64(u), 16))
			i += 3
			continue
		case c >= utf8.RuneSelf:
			_, size := utf8.DecodeRuneInString(s[i:])
			b.WriteString(s[i : i+size])
			i += size
			continue
		default:
			b.WriteByte(c)
		}
		i++
	}
	b.WriteByte('"')
}

type jsonError struct{ msg string }

func (e jsonError) Error() string { return e.msg }

// ParseJSON is JSON.parse without a reviver.
func ParseJSON(s string) (Value, error) {
	p := &jsonParser{s: s}
	p.space()
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	p.space()
	if p.i < len(p.s) {
		return nil, p.fail()
	}
	return v, nil
}

type jsonParser struct {
	s string
	i int
}

func (p *jsonParser) fail() error {
	if p.i >= len(p.s) {
		return jsonError{"Unexpected end of JSON input"}
	}
	return jsonError{"Unexpected token '" + string(p.s[p.i]) + "', in JSON at position " + strconv.Itoa(p.i)}
}

func (p *jsonParser) space() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *jsonParser) value() (Value, error) {
	if p.i >= len(p.s) {
		return nil, p.fail()
	}
	switch c := p.s[p.i]; {
	case c == '{':
		p.i++
		o := NewPlainObject()
		p.space()
		if p.i < len(p.s) && p.s[p.i] == '}' {
			p.i++
			return o, nil
		}
		for {
			p.space()
			if p.i >= len(p.s) || p.s[p.i] != '"' {
				return nil, p.fail()
			}
			k, err := p.str()
			if err != nil {
				return nil, err
			}
			p.space()
			if p.i >= len(p.s) || p.s[p.i] != ':' {
				return nil, p.fail()
			}
			p.i++
			p.space()
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			o.Define(k, v)
			p.space()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.i < len(p.s) && p.s[p.i] == '}' {
				p.i++
				return o, nil
			}
			return nil, p.fail()
		}
	case c == '[':
		p.i++
		var out []Value
		p.space()
		if p.i < len(p.s) && p.s[p.i] == ']' {
			p.i++
			return NewArray(out), nil
		}
		for {
			p.space()
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			p.space()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.i < len(p.s) && p.s[p.i] == ']' {
				p.i++
				return NewArray(out), nil
			}
			return nil, p.fail()
		}
	case c == '"':
		return p.str()
	case c == 't' && strings.HasPrefix(p.s[p.i:], "true"):
		p.i += 4
		return true, nil
	case c == 'f' && strings.HasPrefix(p.s[p.i:], "false"):
		p.i += 5
		return false, nil
	case c == 'n' && strings.HasPrefix(p.s[p.i:], "null"):
		p.i += 4
		return Null, nil
	case c == '-' || (c >= '0' && c <= '9'):
		start := p.i
		if c == '-' {
			p.i++
		}
		if p.i < len(p.s) && p.s[p.i] == '0' {
			p.i++
		} else {
			if p.i >= len(p.s) || p.s[p.i] < '1' || p.s[p.i] > '9' {
				return nil, p.fail()
			}
			for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
				p.i++
			}
		}
		if p.i < len(p.s) && p.s[p.i] == '.' {
			p.i++
			d := p.i
			for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
				p.i++
			}
			if p.i == d {
				return nil, p.fail()
			}
		}
		if p.i < len(p.s) && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
			p.i++
			if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
				p.i++
			}
			d := p.i
			for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
				p.i++
			}
			if p.i == d {
				return nil, p.fail()
			}
		}
		f, _ := strconv.ParseFloat(p.s[start:p.i], 64)
		return f, nil
	}
	return nil, p.fail()
}

func (p *jsonParser) str() (string, error) {
	p.i++ // the opening quote
	start := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '"' {
			s := p.s[start:p.i]
			p.i++
			return s, nil
		}
		if c == '\\' {
			break
		}
		if c < 0x20 {
			return "", p.fail()
		}
		p.i++
	}
	var u []uint16
	u = append(u, Units(p.s[start:p.i])...)
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == '"':
			p.i++
			return FromUnits(u), nil
		case c == '\\':
			p.i++
			if p.i >= len(p.s) {
				return "", p.fail()
			}
			switch e := p.s[p.i]; e {
			case '"', '\\', '/':
				u = append(u, uint16(e))
			case 'b':
				u = append(u, '\b')
			case 'f':
				u = append(u, '\f')
			case 'n':
				u = append(u, '\n')
			case 'r':
				u = append(u, '\r')
			case 't':
				u = append(u, '\t')
			case 'u':
				if p.i+4 >= len(p.s) {
					return "", p.fail()
				}
				n, err := strconv.ParseUint(p.s[p.i+1:p.i+5], 16, 16)
				if err != nil {
					return "", p.fail()
				}
				u = append(u, uint16(n))
				p.i += 4
			default:
				return "", p.fail()
			}
			p.i++
		case c < 0x20:
			return "", p.fail()
		default:
			r, size := utf8.DecodeRuneInString(p.s[p.i:])
			u = append(u, Units(string(r))...)
			p.i += size
		}
	}
	return "", p.fail()
}
