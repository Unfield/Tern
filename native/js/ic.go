package js

import "math/big"

// Inline caches. Each property access in compiled code with a fixed key,
// like obj.name or obj.method(…), has an IC: where it last found the
// property, for objects of one shape. While objects of that shape come
// by, a read is a pointer comparison and a slot load.
//
// A cache is valid while the receiver has the cached shape and each
// prototype walked to reach the holder still has its cached shape: shapes
// fix both an object's own keys and its prototype, so nothing else can
// have changed. Dictionary shapes change in place, so they're never
// cached.

// maxDepth is how far up the prototype chain a cache reaches.
const maxDepth = 4

// IC is one access site's cache.
type IC struct {
	shape *Shape
	// protos are the shapes of the prototypes walked; the last is the
	// holder's. depth is how many; 0 for an own property.
	protos [maxDepth]*Shape
	depth  uint8
	// accessor: the slot holds an *accessor, whose getter or setter runs.
	accessor bool
	// absent: no object on the chain has the key.
	absent bool
	// add caches adding a property: the object goes from shape to next.
	add   bool
	next  *Shape
	index int32
}

// lazyFnShape stands for functions whose name, length and prototype
// haven't been made yet, for any other key: they have no other own
// properties.
func lazyFnShape(proto *Object) *Shape {
	r := rootShape(proto)
	if r.lazyFn == nil {
		r.lazyFn = &Shape{proto: proto}
	}
	return r.lazyFn
}

// receiverShape is what a cache compares for a receiver; nil if nothing.
func (rt *Realm) receiverShape(v Value) *Shape {
	switch x := v.(type) {
	case *Object:
		if x.shape == nil && x.fnLazy != 0 {
			return lazyFnShape(x.proto)
		}
		return x.shape
	case string:
		return rt.stringShape
	case float64:
		return rt.numberShape
	case bool:
		return rt.booleanShape
	case *big.Int:
		return rt.bigintShape
	}
	return nil
}

// GetIC is obj.key with a cache.
func (rt *Realm) GetIC(c *IC, v Value, key string) Value {
	s := rt.receiverShape(v)
	if s != c.shape || s == nil || c.add {
		return rt.getMiss(c, v, key)
	}
	var holder *Object
	if c.depth == 0 {
		if c.absent {
			return Undefined
		}
		holder = v.(*Object)
	} else {
		p := s.proto
		for d := uint8(0); ; d++ {
			if p.shape != c.protos[d] {
				return rt.getMiss(c, v, key)
			}
			if d+1 == c.depth {
				break
			}
			p = p.proto
		}
		if c.absent {
			return Undefined
		}
		holder = p
	}
	slot := holder.slots[c.index]
	if c.accessor {
		if g := slot.(*accessor).get; g != nil {
			return g.invoke(v)
		}
		return Undefined
	}
	return slot
}

// getMiss looks key up and fills the cache.
func (rt *Realm) getMiss(c *IC, v Value, key string) Value {
	c.shape = nil
	if !cacheable(key) {
		return rt.Get(v, key)
	}
	var s *Shape
	if x, ok := v.(*Object); ok {
		if x.fnLazy != 0 && (key == "name" || key == "prototype") {
			x.sh() // those are the lazy properties: make them
		}
		s = rt.receiverShape(x)
		if s == nil {
			s = x.sh()
		}
	} else if s = rt.receiverShape(v); s == nil {
		return rt.Get(v, key)
	}
	rt.fill(c, v, s, key)
	return rt.Get(v, key)
}

// fill caches where key is for receivers of shape s: on the receiver
// itself, on a prototype, or nowhere.
func (rt *Realm) fill(c *IC, v Value, s *Shape, key string) {
	if s.dict || v == Value(rt.Global) {
		return
	}
	if o, ok := v.(*Object); ok && o.shape == s {
		if i := s.findString(key); i >= 0 {
			c.shape, c.depth, c.index, c.absent = s, 0, int32(i), false
			c.accessor, c.add = s.attrs[i]&attrAccessor != 0, false
			return
		}
	}
	depth := 0
	for p := s.proto; p != nil; p = p.proto {
		ps := p.sh()
		if ps.dict || depth == maxDepth {
			return
		}
		c.protos[depth] = ps
		depth++
		if i := ps.findString(key); i >= 0 {
			c.shape, c.depth, c.index, c.absent = s, uint8(depth), int32(i), false
			c.accessor, c.add = ps.attrs[i]&attrAccessor != 0, false
			return
		}
	}
	// Nowhere: reading it gives undefined while the shapes stay.
	c.shape, c.depth, c.absent, c.add = s, uint8(depth), true, false
}

// cacheable reports whether a key is one that lives in shapes on every
// object: array elements, an array's length and typed arrays' numeric
// keys don't.
func cacheable(key string) bool {
	if key == "length" {
		return false
	}
	_, numeric, _ := canonicalIndex(key)
	return !numeric
}

// CallIC is obj.key(…args) with a cache.
func (rt *Realm) CallIC(c *IC, obj Value, key string, args ...Value) Value {
	f := rt.GetIC(c, obj, key)
	o, ok := f.(*Object)
	if !ok || o.call == nil {
		panic(TypeErrorf("%s.%s is not a function", describeForError(obj), key))
	}
	return o.invoke(obj, args...)
}

// SetIC is obj.key = v with a cache, for a writable own property or for
// adding one.
func (rt *Realm) SetIC(c *IC, obj Value, key string, v Value) {
	if o, ok := obj.(*Object); ok && o.shape == c.shape && o.shape != nil {
		if !c.add {
			if c.depth == 0 && !c.accessor {
				o.slots[c.index] = v
				return
			}
		} else if protosValid(c, o.shape.proto) {
			o.shape = c.next
			o.slots = append(o.slots, v)
			return
		}
	}
	rt.setMiss(c, obj, key, v)
}

// protosValid reports whether the prototype chain still has the cached
// shapes, all the way to its end.
func protosValid(c *IC, p *Object) bool {
	for d := uint8(0); d < c.depth; d++ {
		if p == nil || p.shape != c.protos[d] {
			return false
		}
		p = p.proto
	}
	return p == nil
}

func (rt *Realm) setMiss(c *IC, obj Value, key string, v Value) {
	o, ok := obj.(*Object)
	if !ok || !cacheable(key) || o.frozen || o == rt.Global {
		Set(obj, key, v)
		return
	}
	before := o.sh()
	c.shape = nil
	if before.dict {
		o.Set(key, v)
		return
	}
	if i := before.findString(key); i >= 0 {
		o.Set(key, v)
		if a := before.attrs[i]; a&attrAccessor == 0 && a&attrWritable != 0 && o.shape == before {
			c.shape, c.depth, c.index, c.accessor, c.add = before, 0, int32(i), false, false
		}
		return
	}
	// Adding: cacheable if no prototype has the key (a setter or a read-only
	// property there would change what = does) and the chain is short.
	depth := 0
	var protos [maxDepth]*Shape
	for p := o.proto; p != nil; p = p.proto {
		ps := p.sh()
		if depth == maxDepth || ps.dict || ps.findString(key) >= 0 {
			o.Set(key, v)
			return
		}
		protos[depth] = ps
		depth++
	}
	o.Set(key, v)
	if after := o.shape; !after.dict && len(after.keys) == len(before.keys)+1 && after.keys[len(before.keys)] == PropKey(key) {
		c.shape, c.next, c.add, c.depth, c.protos, c.accessor = before, after, true, uint8(depth), protos, false
	}
}
