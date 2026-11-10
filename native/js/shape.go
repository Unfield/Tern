package js

// Shapes. Objects built the same way share a Shape: the list of their own
// properties' keys and attributes, and their prototype. An object keeps
// just its values, in slots. Since a shape says exactly which own
// properties an object has and what its prototype is, compiled code can
// remember where it found a property for objects of a shape (an inline
// cache, see ic.go) and skip the lookup next time.
//
// A shape is never changed once objects use it, except a dictionary
// shape: an object gets its own one when it deletes a property, changes
// one's attributes or grows very large, and then changes it in place.
// Inline caches never remember dictionary shapes.

const (
	attrEnumerable = 1 << iota
	attrWritable
	attrConfigurable
	attrAccessor
)

// attrsOf packs a prop's attributes.
func attrsOf(p *prop) uint8 {
	var a uint8
	if p.enumerable {
		a |= attrEnumerable
	}
	if p.writable {
		a |= attrWritable
	}
	if p.configurable {
		a |= attrConfigurable
	}
	if p.accessor {
		a |= attrAccessor
	}
	return a
}

// Shape is the layout of objects' own properties.
type Shape struct {
	proto *Object
	keys  []PropKey
	attrs []uint8
	// index finds a string key's slot in large shapes; built when needed.
	index map[string]int
	trans []transition
	// extended is set once a child shape shares keys' and attrs' arrays,
	// so a second child must copy them.
	extended bool
	dict     bool
	// lazyFn, on a root shape, stands for lazy functions with this
	// prototype (see lazyFnShape).
	lazyFn *Shape
}

type transition struct {
	key   PropKey
	attrs uint8
	to    *Shape
}

// dictAt is the number of properties past which an object gets a
// dictionary shape: big objects are rarely built the same way twice.
const dictAt = 256

// indexAt is the number of keys from which a shape finds keys with a map.
const indexAt = 10

// rootShape is the shape of objects with no own properties and prototype p.
func rootShape(p *Object) *Shape {
	if p == nil {
		// Objects without a prototype are mostly used as maps: each gets
		// its own dictionary shape.
		return &Shape{dict: true}
	}
	if p.childRoot == nil {
		p.childRoot = &Shape{proto: p}
	}
	return p.childRoot
}

// find returns the slot of key, or -1.
func (s *Shape) find(key PropKey) int {
	if k, ok := key.(string); ok {
		return s.findString(k)
	}
	for i, x := range s.keys {
		if x == key {
			return i
		}
	}
	return -1
}

func (s *Shape) findString(k string) int {
	if len(s.keys) >= indexAt {
		if s.index == nil {
			s.buildIndex()
		}
		if i, ok := s.index[k]; ok {
			return i
		}
		return -1
	}
	for i, x := range s.keys {
		if str, ok := x.(string); ok && len(str) == len(k) && str == k {
			return i
		}
	}
	return -1
}

func (s *Shape) buildIndex() {
	s.index = make(map[string]int, len(s.keys))
	for i, x := range s.keys {
		if str, ok := x.(string); ok {
			s.index[str] = i
		}
	}
}

// with returns the shape with one more property.
func (s *Shape) with(key PropKey, attrs uint8) *Shape {
	if k, ok := key.(string); ok {
		for _, t := range s.trans {
			if tk, ok := t.key.(string); ok && t.attrs == attrs && len(tk) == len(k) && tk == k {
				return t.to
			}
		}
	} else {
		for _, t := range s.trans {
			if t.key == key && t.attrs == attrs {
				return t.to
			}
		}
	}
	n := &Shape{proto: s.proto}
	if !s.extended {
		// Share the arrays: this shape never looks past its own length.
		n.keys = append(s.keys, key)     //nolint:gocritic // sharing is the point
		n.attrs = append(s.attrs, attrs) //nolint:gocritic // sharing is the point
		s.extended = true
	} else {
		n.keys = append(append(make([]PropKey, 0, len(s.keys)+4), s.keys...), key)
		n.attrs = append(append(make([]uint8, 0, len(s.attrs)+4), s.attrs...), attrs)
	}
	s.trans = append(s.trans, transition{key: key, attrs: attrs, to: n})
	return n
}

// ownCopy is a dictionary shape with the same properties, for one object.
func (s *Shape) ownCopy() *Shape {
	return &Shape{
		proto: s.proto,
		keys:  append([]PropKey(nil), s.keys...),
		attrs: append([]uint8(nil), s.attrs...),
		dict:  true,
	}
}

// reshape gives o a dictionary shape, to change its properties in place.
func (o *Object) reshape() {
	if !o.shape.dict {
		o.shape = o.shape.ownCopy()
	}
}

// accessor is the value of an accessor property's slot.
type accessor struct{ get, set *Object }
