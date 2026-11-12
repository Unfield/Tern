package js

import (
	"math"
	"unsafe"
)

// Boxer hands out the boxes of strings and numbers from slabs. Putting a
// string or a float64 in an interface allocates a box for it, 16 or 8
// bytes, which is most of a render's allocations; a Boxer makes one
// allocation per slab instead. The Values are ordinary interfaces, and a
// slab stays alive while any of its Values does, which during a render is
// the render.
//
// A Boxer belongs to one goroutine at a time, as a Realm does.
type Boxer struct {
	strs []string
	nums []float64
}

// slabSize is how many values a slab holds: 2 KiB of strings, 1 KiB of numbers.
const slabSize = 128

// eface is an interface's layout: its type, and a pointer to its value.
type eface struct {
	typ  unsafe.Pointer
	data unsafe.Pointer
}

var (
	stringType  = typeOf(any(""))
	float64Type = typeOf(any(float64(0)))
	emptyString = Value("")
)

func typeOf(v any) unsafe.Pointer { return (*eface)(unsafe.Pointer(&v)).typ }

// Str returns s as a Value.
func (b *Boxer) Str(s string) Value {
	if s == "" {
		return emptyString
	}
	if len(b.strs) == 0 {
		b.strs = make([]string, slabSize)
	}
	p := &b.strs[0]
	b.strs = b.strs[1:]
	*p = s
	var v Value
	e := (*eface)(unsafe.Pointer(&v))
	e.typ, e.data = stringType, unsafe.Pointer(p)
	return v
}

// Num returns f as a Value; small integers need no box at all.
func (b *Boxer) Num(f float64) Value {
	if i := int(f); float64(i) == f && i >= 0 && i < len(boxedInts) && (i != 0 || !math.Signbit(f)) {
		return boxedInts[i]
	}
	if len(b.nums) == 0 {
		b.nums = make([]float64, slabSize)
	}
	p := &b.nums[0]
	b.nums = b.nums[1:]
	*p = f
	var v Value
	e := (*eface)(unsafe.Pointer(&v))
	e.typ, e.data = float64Type, unsafe.Pointer(p)
	return v
}

// Str returns s as a Value, boxed from the realm's slabs.
func (rt *Realm) Str(s string) Value { return rt.box.Str(s) }

// Num returns f as a Value, boxed from the realm's slabs.
func (rt *Realm) Num(f float64) Value { return rt.box.Num(f) }
