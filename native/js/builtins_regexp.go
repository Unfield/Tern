package js

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dlclark/regexp2/v2"
)

type regexpData struct {
	re     *regexp2.Regexp
	source string
	flags  string
	// set is the bytes a regexp that's one class of ASCII characters, such
	// as /[&"<]/g, matches: those are found by scanning bytes.
	set *[256]bool
}

// asciiClass recognizes a regexp that is one class of plain ASCII
// characters, with no flags that change what it matches.
func asciiClass(source, flags string) *[256]bool {
	if strings.Trim(flags, "gyd") != "" || len(source) < 3 || source[0] != '[' || source[len(source)-1] != ']' {
		return nil
	}
	var set [256]bool
	for _, c := range []byte(source[1 : len(source)-1]) {
		if c < 0x20 || c >= 0x7f || c == '\\' || c == ']' || c == '[' || c == '^' || c == '-' {
			return nil
		}
		set[c] = true
	}
	return &set
}

// NewRegExp compiles a regular expression literal or new RegExp(source, flags).
func (rt *Realm) NewRegExp(source, flags string) *Object {
	opts := regexp2.ECMAScript
	for _, f := range flags {
		switch f {
		case 'i':
			opts |= regexp2.IgnoreCase
		case 'm':
			opts |= regexp2.Multiline
		case 's':
			opts |= regexp2.Singleline
		case 'u', 'v':
			opts |= regexp2.Unicode
		case 'g', 'y', 'd':
		default:
			panic(SyntaxErrorf("Invalid regular expression flags"))
		}
	}
	if strings.ContainsRune(flags, 's') {
		opts &^= regexp2.ECMAScript // regexp2's ECMAScript mode has no dotAll
	}
	if strings.ContainsRune(flags, 'u') && strings.ContainsRune(flags, 'v') {
		panic(SyntaxErrorf("Invalid regular expression flags"))
	}
	re, err := regexp2.Compile(source, opts)
	if err != nil && strings.ContainsRune(flags, 'v') {
		// Set operations and string classes: the engine has them.
		panic(NotSupported("the v flag's class syntax in /%s/", source))
	}
	if err != nil {
		panic(SyntaxErrorf("Invalid regular expression: /%s/: %s", source, err.Error()))
	}
	o := NewObject(rt.RegExpPrototype)
	o.Class, o.Internal = "RegExp", &regexpData{re: re, source: source, flags: sortFlags(flags), set: asciiClass(source, flags)}
	o.add(prop{key: "lastIndex", value: 0.0, writable: true})
	return o
}

func sortFlags(f string) string {
	var b strings.Builder
	for _, c := range "dgimsuvy" {
		if strings.ContainsRune(f, c) {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func (o *Object) regexpFlags() string { return o.Internal.(*regexpData).flags }

func (rt *Realm) toRegExp(v Value, flags string) *Object {
	if o, ok := v.(*Object); ok && o.Class == "RegExp" {
		return o
	}
	src := "(?:)"
	if v != Undefined {
		src = regexpEscapeSource(ToString(v))
	}
	return rt.NewRegExp(src, flags)
}

// regexpEscapeSource keeps a string's meaning as a pattern: match("a.b")
// treats the string as a regexp source, so nothing is escaped.
func regexpEscapeSource(s string) string { return s }

// utf16Offset converts a rune index into s to a UTF-16 index.
func (rt *Realm) utf16Offset(s string, runeIndex int) int {
	if rt.isASCII(s) {
		return runeIndex
	}
	n, r := 0, 0
	for _, c := range s {
		if r == runeIndex {
			break
		}
		if c >= 0x10000 {
			n += 2
		} else {
			n++
		}
		r++
	}
	return n
}

// runeOffset converts a UTF-16 index into s to a rune index.
func (rt *Realm) runeOffset(s string, unitIndex int) int {
	if rt.isASCII(s) {
		return unitIndex
	}
	n, r := 0, 0
	for _, c := range s {
		if n >= unitIndex {
			break
		}
		if c >= 0x10000 {
			n += 2
		} else {
			n++
		}
		r++
	}
	return r
}

type match struct {
	index  int // UTF-16
	end    int
	groups []Value // the whole match, then each group (undefined if it didn't take part)
	names  map[string]Value
	// spans are each group's UTF-16 start and end, for the d flag; -1 when
	// the group didn't take part.
	spans      [][2]int
	nameSpans  map[string][2]int
	hasIndices bool
}

// exec runs the regexp once, honoring lastIndex for g and y.
func (rt *Realm) exec(r *Object, s string) *match {
	d := r.Internal.(*regexpData)
	global := strings.ContainsAny(d.flags, "gy")
	start := 0
	if global {
		start = int(ToIntegerOrInfinity(r.Get("lastIndex")))
		if start > rt.Len16(s) {
			r.Set("lastIndex", 0.0)
			return nil
		}
	}
	if d.set != nil {
		return rt.execClass(r, d, s, start, global)
	}
	m, err := d.re.FindStringMatchStartingAt(s, rt.runeOffset(s, start))
	if err != nil || m == nil || (strings.ContainsRune(d.flags, 'y') && rt.utf16Offset(s, m.RuneIndex) != start) {
		if global {
			r.Set("lastIndex", 0.0)
		}
		return nil
	}
	res := &match{index: rt.utf16Offset(s, m.RuneIndex), hasIndices: strings.ContainsRune(d.flags, 'd')}
	res.end = res.index + rt.Len16(m.String())
	gs := m.Groups()
	span := func(g *regexp2.Group) [2]int {
		if len(g.Captures) == 0 {
			return [2]int{-1, -1}
		}
		return [2]int{rt.utf16Offset(s, g.RuneIndex), rt.utf16Offset(s, g.RuneIndex+g.RuneLength)}
	}
	for i := range gs {
		g := &gs[i]
		if len(g.Captures) == 0 {
			res.groups = append(res.groups, Undefined)
		} else {
			res.groups = append(res.groups, g.String())
		}
		if res.hasIndices {
			res.spans = append(res.spans, span(g))
		}
	}
	for _, name := range d.re.GetGroupNames() {
		if _, err := strconv.Atoi(name); err == nil {
			continue
		}
		if res.names == nil {
			res.names = map[string]Value{}
		}
		if g := m.GroupByName(name); g != nil && len(g.Captures) > 0 {
			res.names[name] = g.String()
		} else {
			res.names[name] = Undefined
		}
		if res.hasIndices {
			if res.nameSpans == nil {
				res.nameSpans = map[string][2]int{}
			}
			if g := m.GroupByName(name); g != nil {
				res.nameSpans[name] = span(g)
			} else {
				res.nameSpans[name] = [2]int{-1, -1}
			}
		}
	}
	if global {
		r.Set("lastIndex", float64(res.end))
	}
	return res
}

// execClass is exec for a regexp that is one class of ASCII characters.
// ASCII bytes never occur inside UTF-8 sequences, so scanning bytes finds
// exactly the code units the class matches.
func (rt *Realm) execClass(r *Object, d *regexpData, s string, start int, global bool) *match {
	ascii := rt.isASCII(s)
	from := start
	if !ascii {
		from = byteOffset16(s, start)
	}
	sticky := strings.ContainsRune(d.flags, 'y')
	for i := from; i < len(s); i++ {
		if d.set[s[i]] {
			idx := i
			if !ascii {
				idx = rt.Len16(s[:i])
			}
			if global {
				r.Set("lastIndex", float64(idx+1))
			}
			return &match{index: idx, end: idx + 1, groups: []Value{s[i : i+1]}, hasIndices: strings.ContainsRune(d.flags, 'd'), spans: [][2]int{{idx, idx + 1}}}
		}
		if sticky {
			break
		}
	}
	if global {
		r.Set("lastIndex", 0.0)
	}
	return nil
}

// byteOffset16 is the byte offset of a UTF-16 index into s.
func byteOffset16(s string, unitIndex int) int {
	n := 0
	for i, c := range s {
		if n >= unitIndex {
			return i
		}
		if c >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return len(s)
}

func (m *match) array(rt *Realm, s string) *Object {
	a := rt.NewArray(append([]Value(nil), m.groups...))
	a.Define("index", float64(m.index))
	a.Define("input", s)
	if m.names != nil {
		g := NewObject(nil)
		for _, k := range sortedNames(m.names) {
			g.Define(k, m.names[k])
		}
		a.Define("groups", g)
	} else {
		a.Define("groups", Undefined)
	}
	if m.hasIndices {
		pair := func(sp [2]int) Value {
			if sp[0] < 0 {
				return Undefined
			}
			return rt.NewArray([]Value{float64(sp[0]), float64(sp[1])})
		}
		ind := make([]Value, len(m.spans))
		for i, sp := range m.spans {
			ind[i] = pair(sp)
		}
		ia := rt.NewArray(ind)
		if m.names != nil {
			g := NewObject(nil)
			for _, k := range sortedNames(m.names) {
				g.Define(k, pair(m.nameSpans[k]))
			}
			ia.Define("groups", g)
		} else {
			ia.Define("groups", Undefined)
		}
		a.Define("indices", ia)
	}
	return a
}

func sortedNames(m map[string]Value) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Group order is the order they appear in the pattern; regexp2 gives
	// them in that order, so keep it stable by name as a fallback.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func (rt *Realm) setupRegExp() {
	ctor := rt.NewConstructor("RegExp", 2, nil, func(args []Value, nt *Object) Value {
		pat, flags := Arg(args, 0), Arg(args, 1)
		src := "(?:)"
		fl := ""
		if o, ok := pat.(*Object); ok && o.Class == "RegExp" {
			d := o.Internal.(*regexpData)
			src, fl = d.source, d.flags
		} else if pat != Undefined {
			src = ToString(pat)
		}
		if flags != Undefined {
			fl = ToString(flags)
		}
		return rt.NewRegExp(src, fl)
	}, rt.RegExpPrototype)
	ctor.call = func(_ Value, args []Value) Value { return ctor.construct(args, ctor) }
	rt.Global.DefineHidden("RegExp", ctor)
	p := rt.RegExpPrototype
	this := func(v Value) *Object {
		if o, ok := v.(*Object); ok && o.Class == "RegExp" {
			return o
		}
		panic(TypeErrorf("RegExp method called on incompatible receiver"))
	}
	rt.method(p, "exec", 1, func(t Value, args []Value) Value {
		r := this(t)
		s := ToString(Arg(args, 0))
		m := rt.exec(r, s)
		if m == nil {
			return Null
		}
		return m.array(rt, s)
	})
	rt.method(p, "test", 1, func(t Value, args []Value) Value { return rt.exec(this(t), ToString(Arg(args, 0))) != nil })
	rt.method(p, "toString", 0, func(t Value, _ []Value) Value {
		d := this(t).Internal.(*regexpData)
		return "/" + d.source + "/" + d.flags
	})
	getter := func(name string, fn func(d *regexpData) Value) {
		p.DefineAccessor(name, rt.NewFunction("get "+name, 0, func(t Value, _ []Value) Value {
			if t == Value(rt.RegExpPrototype) {
				return Undefined
			}
			return fn(this(t).Internal.(*regexpData))
		}), nil, false)
	}
	getter("source", func(d *regexpData) Value { return d.source })
	getter("flags", func(d *regexpData) Value { return d.flags })
	for name, f := range map[string]rune{"global": 'g', "ignoreCase": 'i', "multiline": 'm', "dotAll": 's', "unicode": 'u', "sticky": 'y', "hasIndices": 'd', "unicodeSets": 'v'} {
		getter(name, func(d *regexpData) Value { return strings.ContainsRune(d.flags, f) })
	}
}

func (rt *Realm) regexpMatch(r *Object, s string) Value {
	if !strings.ContainsRune(r.regexpFlags(), 'g') {
		m := rt.exec(r, s)
		if m == nil {
			return Null
		}
		return m.array(rt, s)
	}
	r.Set("lastIndex", 0.0)
	var out []Value
	for {
		m := rt.exec(r, s)
		if m == nil {
			break
		}
		out = append(out, m.groups[0])
		if m.end == m.index {
			r.Set("lastIndex", float64(rt.advance(s, m.end, strings.ContainsRune(r.regexpFlags(), 'u'))))
		}
	}
	if out == nil {
		return Null
	}
	return rt.NewArray(out)
}

func (rt *Realm) regexpMatchAll(r *Object, s string) Value {
	clone := rt.NewRegExp(r.Internal.(*regexpData).source, r.regexpFlags())
	clone.Set("lastIndex", r.Get("lastIndex"))
	done := false
	return rt.NewIterator(func() (Value, bool) {
		if done {
			return Undefined, false
		}
		m := rt.exec(clone, s)
		if m == nil {
			done = true
			return Undefined, false
		}
		if m.end == m.index {
			clone.Set("lastIndex", float64(rt.advance(s, m.end, strings.ContainsRune(clone.regexpFlags(), 'u'))))
		}
		return m.array(rt, s), true
	})
}

func (rt *Realm) regexpSearch(r *Object, s string) Value {
	saved := r.Get("lastIndex")
	r.Set("lastIndex", 0.0)
	d := r.Internal.(*regexpData)
	m, err := d.re.FindStringMatch(s)
	r.Set("lastIndex", saved)
	if err != nil || m == nil {
		return -1.0
	}
	return float64(rt.utf16Offset(s, m.RuneIndex))
}

// advance moves past an empty match by one code unit, or one code point
// with the u flag.
func (rt *Realm) advance(s string, i int, unicode bool) int {
	if unicode {
		if c := rt.UnitAt(s, i); c >= 0xD800 && c <= 0xDBFF {
			if d := rt.UnitAt(s, i+1); d >= 0xDC00 && d <= 0xDFFF {
				return i + 2
			}
		}
	}
	return i + 1
}

func (rt *Realm) regexpSplit(r *Object, s string, limitArg Value) Value {
	limit := 1<<32 - 1
	if limitArg != Undefined {
		limit = int(ToUint32(limitArg))
	}
	var out []Value
	if limit == 0 {
		return rt.NewArray(nil)
	}
	d := r.Internal.(*regexpData)
	if s == "" {
		if m, _ := d.re.FindStringMatch(s); m != nil {
			return rt.NewArray(nil)
		}
		return rt.NewArray([]Value{s})
	}
	size := rt.Len16(s)
	p, q := 0, 0
	for q < size {
		m, err := d.re.FindStringMatchStartingAt(s, rt.runeOffset(s, q))
		if err != nil || m == nil {
			break
		}
		start := rt.utf16Offset(s, m.RuneIndex)
		end := start + rt.Len16(m.String())
		if end == start && start == p || start >= size {
			if start >= size {
				break
			}
			q = start + 1
			if end == start && start == p {
				continue
			}
		}
		if end == p {
			q = start + 1
			continue
		}
		out = append(out, rt.Slice16(s, p, start))
		if len(out) == limit {
			return rt.NewArray(out)
		}
		for _, g := range m.Groups()[1:] {
			if len(g.Captures) == 0 {
				out = append(out, Undefined)
			} else {
				out = append(out, g.String())
			}
			if len(out) == limit {
				return rt.NewArray(out)
			}
		}
		p = end
		q = end
		if end == start {
			q = end + 1
		}
	}
	out = append(out, rt.Slice16(s, p, size))
	return rt.NewArray(out)
}

// replace is String.prototype.replace and replaceAll.
func (rt *Realm) replace(s string, pattern, replacement Value, all bool) Value {
	fn, isFn := replacement.(*Object)
	isFn = isFn && fn.call != nil
	var repl string
	if !isFn {
		repl = ToString(replacement)
	}
	var b strings.Builder
	if r, ok := pattern.(*Object); ok && r.Class == "RegExp" {
		global := strings.ContainsRune(r.regexpFlags(), 'g')
		if all && !global {
			panic(TypeErrorf("replaceAll must be called with a global RegExp"))
		}
		if global {
			r.Set("lastIndex", 0.0)
		}
		last := 0
		for {
			m := rt.exec(r, s)
			if m == nil {
				break
			}
			b.WriteString(rt.Slice16(s, last, m.index))
			if isFn {
				args := append([]Value(nil), m.groups...)
				args = append(args, float64(m.index), s)
				if m.names != nil {
					g := NewObject(nil)
					for _, k := range sortedNames(m.names) {
						g.Define(k, m.names[k])
					}
					args = append(args, g)
				}
				b.WriteString(ToString(fn.call(Undefined, args)))
			} else {
				b.WriteString(rt.expandReplacement(repl, s, m))
			}
			last = m.end
			if !global {
				break
			}
			if m.end == m.index {
				r.Set("lastIndex", float64(rt.advance(s, m.end, strings.ContainsRune(r.regexpFlags(), 'u'))))
			}
		}
		b.WriteString(rt.Slice16(s, last, rt.Len16(s)))
		return b.String()
	}
	pat := ToString(pattern)
	last := 0
	from := 0
	for {
		i := rt.Index16(s, pat, from)
		if i < 0 {
			break
		}
		b.WriteString(rt.Slice16(s, last, i))
		m := &match{index: i, end: i + rt.Len16(pat), groups: []Value{pat}}
		if isFn {
			b.WriteString(ToString(fn.invoke(Undefined, pat, float64(i), s)))
		} else {
			b.WriteString(rt.expandReplacement(repl, s, m))
		}
		last = m.end
		if !all {
			break
		}
		from = m.end
		if pat == "" {
			from++
			if from > rt.Len16(s) {
				break
			}
		}
	}
	b.WriteString(rt.Slice16(s, last, rt.Len16(s)))
	return b.String()
}

// expandReplacement applies $$, $&, $`, $', $n and $<name>.
func (rt *Realm) expandReplacement(repl, s string, m *match) string {
	if !strings.Contains(repl, "$") {
		return repl
	}
	var b strings.Builder
	for i := 0; i < len(repl); i++ {
		c := repl[i]
		if c != '$' || i+1 >= len(repl) {
			b.WriteByte(c)
			continue
		}
		n := repl[i+1]
		switch {
		case n == '$':
			b.WriteByte('$')
			i++
		case n == '&':
			b.WriteString(ToString(m.groups[0]))
			i++
		case n == '`':
			b.WriteString(rt.Slice16(s, 0, m.index))
			i++
		case n == '\'':
			b.WriteString(rt.Slice16(s, m.end, rt.Len16(s)))
			i++
		case n >= '0' && n <= '9':
			// One or two digits, preferring two if that group exists.
			idx := int(n - '0')
			width := 1
			if i+2 < len(repl) && repl[i+2] >= '0' && repl[i+2] <= '9' {
				two := idx*10 + int(repl[i+2]-'0')
				if two >= 1 && two < len(m.groups) {
					idx, width = two, 2
				}
			}
			if idx >= 1 && idx < len(m.groups) {
				if g := m.groups[idx]; g != Undefined {
					b.WriteString(ToString(g))
				}
				i += width
			} else {
				b.WriteByte('$')
			}
		case n == '<' && m.names != nil:
			end := strings.IndexByte(repl[i:], '>')
			if end < 0 {
				b.WriteByte('$')
				continue
			}
			name := repl[i+2 : i+end]
			if v, ok := m.names[name]; ok && v != Undefined {
				b.WriteString(ToString(v))
			}
			i += end
		default:
			b.WriteByte('$')
		}
	}
	return b.String()
}

var _ = utf8.RuneSelf
