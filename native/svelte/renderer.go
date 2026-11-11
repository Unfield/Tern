package svelte

import (
	"strings"

	"github.com/Unfield/Tern/native/js"
)

// Svelte's server Renderer collects a page as a tree: each renderer keeps
// #out, a list of strings and child renderers, and #collect_content joins
// them into content.head and content.body with +=, one piece at a time.
// These Go versions of push and #collect_content do the same work with
// fewer steps: push appends to #out directly, and collecting walks the
// whole tree once and joins each part's pieces into one string of the
// right size. What they don't handle they leave to the compiled methods
// (orig), so the result is always what the JavaScript computes.

// RendererPush is Renderer.prototype.push:
//
//	push(content) {
//		if (typeof content === 'function') {
//			this.child(async (renderer) => renderer.push(await content()));
//		} else {
//			this.#out.push(content);
//		}
//	}
func RendererPush(rt *js.Realm, out *js.PrivateName, orig js.Value) js.Value {
	var push js.IC
	return rt.NewFunction("push", 1, func(this js.Value, args []js.Value) js.Value {
		content := js.Arg(args, 0)
		if o, ok := content.(*js.Object); ok && o.IsCallable() {
			return js.Call(orig, this, args...)
		}
		if !js.HasPrivate(this, out) {
			return js.Call(orig, this, args...) // the TypeError the method throws
		}
		list := js.PrivateGet(this, out)
		if !rt.PushPlain(&push, list, content) {
			rt.CallIC(&push, list, "push", content)
		}
		return js.Undefined
	})
}

// RendererCollect is Renderer.prototype.#collect_content:
//
//	#collect_content(content = { head: '', body: '' }) {
//		for (const item of this.#out) {
//			if (typeof item === 'string') {
//				content[this.type] += item;
//			} else if (item instanceof Renderer) {
//				item.#collect_content(content);
//			}
//		}
//		return content;
//	}
//
// ctor is the class, set after its methods are made.
func RendererCollect(rt *js.Realm, out, collect *js.PrivateName, ctor **js.Object, orig js.Value) js.Value {
	var self *js.Object
	self = js.AsObject(rt.NewFunction("#collect_content", 0, func(this js.Value, args []js.Value) js.Value {
		content := js.Arg(args, 0)
		if content == js.Undefined {
			content = rt.Obj("head", "", "body", "")
		}
		c := &collector{rt: rt, out: out, collect: collect, ctor: *ctor, self: self, content: content}
		c.walk(this)
		c.flush()
		return content
	}))
	return self
}

type collector struct {
	rt      *js.Realm
	out     *js.PrivateName
	collect *js.PrivateName
	ctor    *js.Object
	self    *js.Object
	content js.Value
	// parts holds the pieces for each key of content not yet added, in the
	// order the keys first came: head and body, so a list beats a map.
	parts []part
	last  int // the part the last piece went to
}

type part struct {
	key    js.PropKey
	pieces []string
	n      int
}

func (c *collector) add(key js.PropKey, s string) {
	if c.last < len(c.parts) && c.parts[c.last].key == key {
		p := &c.parts[c.last]
		p.pieces, p.n = append(p.pieces, s), p.n+len(s)
		return
	}
	for i := range c.parts {
		if c.parts[i].key == key {
			c.last = i
			c.parts[i].pieces, c.parts[i].n = append(c.parts[i].pieces, s), c.parts[i].n+len(s)
			return
		}
	}
	c.last = len(c.parts)
	c.parts = append(c.parts, part{key: key, pieces: append(make([]string, 0, 64), s), n: len(s)})
}

// flush does content[key] += the pieces, for each key: the same strings
// the one-at-a-time += makes, as each key's pieces keep their order.
func (c *collector) flush() {
	for _, p := range c.parts {
		var b strings.Builder
		b.Grow(p.n)
		for _, s := range p.pieces {
			b.WriteString(s)
		}
		js.Set(c.content, p.key, c.rt.Add(c.rt.Get(c.content, p.key), b.String()))
	}
	c.parts, c.last = c.parts[:0], 0
}

func (c *collector) walk(r js.Value) {
	if !js.HasPrivate(r, c.out) {
		js.PrivateGet(r, c.out) // throws the TypeError the method would
	}
	var key js.PropKey
	keyRead := false
	next := c.rt.Iterate(js.PrivateGet(r, c.out))
	for {
		item, ok := next()
		if !ok {
			return
		}
		if s, ok := item.(string); ok {
			if !keyRead {
				// this.type, read as the loop's first string does.
				key, keyRead = js.ToPropertyKey(c.rt.Get(r, "type")), true
			}
			c.add(key, s)
			continue
		}
		if !c.rt.InstanceOf(item, c.ctor) {
			continue
		}
		if m, ok := js.PrivateGet(item, c.collect).(*js.Object); ok && m == c.self {
			c.walk(item)
			continue
		}
		// Not this method (another realm's class, say): call it, after
		// adding what came before, so the order stays.
		c.flush()
		js.Call(js.PrivateGet(item, c.collect), item, c.content)
	}
}
