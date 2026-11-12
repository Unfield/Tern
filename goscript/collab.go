package goscript

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/internal/nilslices"
)

// Collaboration: who is looking at the same thing (Room), and a shared
// value that people change together without overwriting each other
// (Docs). Both work across server processes through the broker.

// collab is a room set or a doc set, for the broker's connect, heartbeat
// and leaving.
type collab interface {
	connect(ctx context.Context, b Broker)
	heartbeat(ctx context.Context, b Broker)
	leaving(ctx context.Context, b Broker)
}

var collabs []collab

func registerCollab(x collab) {
	brokerMu.Lock()
	collabs = append(collabs, x)
	if broker != nil {
		x.connect(brokerCtx, broker)
	}
	brokerMu.Unlock()
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Member is someone in a room: each component that joined, with the state
// it shares, such as a name, a color and where its cursor is.
type Member[M any] struct {
	// ID identifies the member: one per component that joined, so the same
	// user in two tabs is two members.
	ID    string    `json:"id"`
	State M         `json:"state"`
	Since time.Time `json:"since"`
}

// RoomSet is a kind of room, such as "board": one room per key, such as
// "board:42", with the members in it.
type RoomSet[M any] struct {
	name  string
	mu    sync.Mutex
	rooms map[string]*room[M]
}

type room[M any] struct {
	local   map[string]Member[M]
	remote  map[string]remoteMembers[M] // by process
	list    *GlobalSignal[[]Member[M]]
	refs    int
	pending bool // a broadcast is scheduled
}

type remoteMembers[M any] struct {
	members []Member[M]
	seen    time.Time
}

// Room declares a kind of room under name, which must be the same in every
// process. Declare it as a package variable:
//
//	type Here struct {
//		Name  string `json:"name"`
//		Field string `json:"field"` // the field they're editing
//	}
//
//	var boards = goscript.Room[Here]("board")
//
//	func Setup(c *goscript.Component) {
//		key := "board:" + id
//		me := boards.Join(c, key, Here{Name: user.Name})
//		boards.Show(c, key, "people")
//		goscript.State(c, "me", me.ID())
//		c.Func("focus", func(field string) { me.Update(func(h Here) Here { h.Field = field; return h }) })
//	}
func Room[M any](name string) *RoomSet[M] {
	r := &RoomSet[M]{name: name, rooms: map[string]*room[M]{}}
	registerCollab(r)
	return r
}

func (r *RoomSet[M]) topic() string { return topicPrefix + "room." + r.name }

// get returns the room for key, made if needed; r.mu is held.
func (r *RoomSet[M]) get(key string) *room[M] {
	rm := r.rooms[key]
	if rm == nil {
		rm = &room[M]{local: map[string]Member[M]{}, remote: map[string]remoteMembers[M]{}, list: Global([]Member[M]{})}
		r.rooms[key] = rm
	}
	return rm
}

// release drops a reference to the room, and the room once nothing uses
// it; r.mu is held.
func (r *RoomSet[M]) release(key string, rm *room[M]) {
	rm.refs--
	if rm.refs <= 0 && len(rm.local) == 0 && len(rm.remote) == 0 {
		delete(r.rooms, key)
	}
}

// relist updates the room's members, oldest first; r.mu is held.
func (rm *room[M]) relist() {
	var all []Member[M]
	for _, m := range rm.local {
		all = append(all, m)
	}
	for _, e := range rm.remote {
		all = append(all, e.members...)
	}
	slices.SortFunc(all, func(a, b Member[M]) int {
		if c := a.Since.Compare(b.Since); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	if all == nil {
		all = []Member[M]{}
	}
	rm.list.Set(all)
}

// RoomMember is the handle of a component that joined a room.
type RoomMember[M any] struct {
	r   *RoomSet[M]
	key string
	id  string
}

// Join adds c to the room key with state, until c is destroyed. While
// rendering it does nothing: the handle's methods do nothing then either.
func (r *RoomSet[M]) Join(c *Component, key string, state M) *RoomMember[M] {
	h := &RoomMember[M]{r: r, key: key, id: newID()}
	if c.rendering {
		return h
	}
	r.mu.Lock()
	rm := r.get(key)
	rm.refs++
	rm.local[h.id] = Member[M]{ID: h.id, State: state, Since: time.Now().UTC()}
	rm.relist()
	r.schedule(key, rm)
	r.mu.Unlock()
	c.releases = append(c.releases, func() {
		r.mu.Lock()
		delete(rm.local, h.id)
		rm.relist()
		r.schedule(key, rm)
		r.release(key, rm)
		r.mu.Unlock()
	})
	return h
}

// ID is the member's ID, to tell its own entry in the room's members.
func (h *RoomMember[M]) ID() string { return h.id }

// Set replaces the member's state.
func (h *RoomMember[M]) Set(state M) { h.Update(func(M) M { return state }) }

// Update changes the member's state. Updates reach other processes at
// most every 50 milliseconds, so a cursor can update on every move.
func (h *RoomMember[M]) Update(fn func(M) M) {
	h.r.mu.Lock()
	defer h.r.mu.Unlock()
	rm := h.r.rooms[h.key]
	if rm == nil {
		return
	}
	m, ok := rm.local[h.id]
	if !ok {
		return
	}
	m.State = fn(m.State)
	rm.local[h.id] = m
	rm.relist()
	h.r.schedule(h.key, rm)
}

// Members returns who is in the room key now.
func (r *RoomSet[M]) Members(key string) []Member[M] {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rm := r.rooms[key]; rm != nil {
		return rm.list.Get()
	}
	return []Member[M]{}
}

// Use returns a signal in c that follows the members of the room key.
func (r *RoomSet[M]) Use(c *Component, key string) *Signal[[]Member[M]] {
	return r.watch(c, key, func(g *GlobalSignal[[]Member[M]]) *Signal[[]Member[M]] { return g.Use(c) })
}

// Show publishes the members of the room key in c's markup under name,
// such as {#each people as p (p.id)}, and returns the signal.
func (r *RoomSet[M]) Show(c *Component, key, name string) *Signal[[]Member[M]] {
	return r.watch(c, key, func(g *GlobalSignal[[]Member[M]]) *Signal[[]Member[M]] { return g.Show(c, name) })
}

func (r *RoomSet[M]) watch(c *Component, key string, fn func(*GlobalSignal[[]Member[M]]) *Signal[[]Member[M]]) *Signal[[]Member[M]] {
	r.mu.Lock()
	rm := r.get(key)
	rm.refs++
	g := rm.list
	r.mu.Unlock()
	s := fn(g)
	c.releases = append(c.releases, func() {
		r.mu.Lock()
		r.release(key, rm)
		r.mu.Unlock()
	})
	return s
}

// roomMessage is a process's members of one room.
type roomMessage[M any] struct {
	Key     string      `json:"k"`
	Members []Member[M] `json:"m"`
}

// schedule sends the room's local members to other processes soon,
// gathering the changes of the next 50 milliseconds; r.mu is held.
func (r *RoomSet[M]) schedule(key string, rm *room[M]) {
	b, ctx := currentBroker()
	if b == nil || rm.pending {
		return
	}
	rm.pending = true
	time.AfterFunc(50*time.Millisecond, func() {
		r.mu.Lock()
		rm.pending = false
		msg := r.message(key, rm)
		r.mu.Unlock()
		_ = r.send(ctx, b, msg)
	})
}

// message is the room's local members; r.mu is held.
func (r *RoomSet[M]) message(key string, rm *room[M]) roomMessage[M] {
	msg := roomMessage[M]{Key: key, Members: []Member[M]{}}
	for _, m := range rm.local {
		msg.Members = append(msg.Members, m)
	}
	return msg
}

func (r *RoomSet[M]) send(ctx context.Context, b Broker, msg roomMessage[M]) error {
	data, err := json.Marshal(nilslices.Remove(msg))
	if err != nil {
		return err
	}
	return publishEnvelope(ctx, b, r.topic(), envelope{Data: data})
}

func (r *RoomSet[M]) connect(ctx context.Context, b Broker) {
	_ = b.Subscribe(ctx, r.topic(), func(raw []byte) {
		var e envelope
		if json.Unmarshal(raw, &e) != nil || e.Origin == replicaID {
			return
		}
		if e.Hello {
			// A process started: tell it who is here.
			r.mu.Lock()
			var msgs []roomMessage[M]
			for key, rm := range r.rooms {
				if len(rm.local) > 0 {
					msgs = append(msgs, r.message(key, rm))
				}
			}
			r.mu.Unlock()
			go func() {
				for _, m := range msgs {
					_ = r.send(ctx, b, m)
				}
			}()
			return
		}
		var msg roomMessage[M]
		if json.Unmarshal(e.Data, &msg) != nil {
			return
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		rm := r.rooms[msg.Key]
		if rm == nil {
			if len(msg.Members) == 0 {
				return
			}
			rm = r.get(msg.Key)
		}
		if len(msg.Members) == 0 {
			delete(rm.remote, e.Origin)
		} else {
			rm.remote[e.Origin] = remoteMembers[M]{members: msg.Members, seen: time.Now()}
		}
		rm.relist()
		if rm.refs <= 0 && len(rm.local) == 0 && len(rm.remote) == 0 {
			delete(r.rooms, msg.Key)
		}
	})
	go func() { _ = publishEnvelope(ctx, b, r.topic(), envelope{Hello: true}) }()
}

// heartbeat reports the rooms with members here, and drops the members of
// processes that stopped reporting.
func (r *RoomSet[M]) heartbeat(ctx context.Context, b Broker) {
	r.mu.Lock()
	var msgs []roomMessage[M]
	for key, rm := range r.rooms {
		changed := false
		for origin, e := range rm.remote {
			if time.Since(e.seen) > presenceExpiry {
				delete(rm.remote, origin)
				changed = true
			}
		}
		if changed {
			rm.relist()
		}
		if len(rm.local) > 0 {
			msgs = append(msgs, r.message(key, rm))
		}
	}
	r.mu.Unlock()
	for _, m := range msgs {
		_ = r.send(ctx, b, m)
	}
}

// leaving tells other processes this one's members are gone.
func (r *RoomSet[M]) leaving(ctx context.Context, b Broker) {
	r.mu.Lock()
	var keys []string
	for key, rm := range r.rooms {
		if len(rm.local) > 0 {
			keys = append(keys, key)
		}
	}
	r.mu.Unlock()
	for _, k := range keys {
		_ = r.send(ctx, b, roomMessage[M]{Key: k, Members: []Member[M]{}})
	}
}

// DocOptions configure a kind of shared document.
type DocOptions[T any] struct {
	// Load reads a document when a process opens it and no other process
	// has it open, such as from the database. Without it, a document starts
	// as T's zero value.
	Load func(ctx context.Context, key string) (T, error)
	// Save writes a document after it changes, in the process where it
	// changed, at most every SaveEvery. Errors are logged.
	Save func(ctx context.Context, key string, v T) error
	// SaveEvery is how long Save waits after a change, gathering the
	// changes meanwhile. Default: 1 second.
	SaveEvery time.Duration
}

// DocSet is a kind of shared document, such as "note": one document per
// key, such as "note:42", that everyone who opens it sees and changes.
type DocSet[T any] struct {
	name string
	opts DocOptions[T]
	mu   sync.Mutex
	docs map[string]*doc[T]
}

// doc is a document as its fields, each with the stamp of its last change.
type doc[T any] struct {
	fields map[string]json.RawMessage
	stamps map[string]stamp
	value  *GlobalSignal[T]
	refs   int
	saving bool
	err    error
}

// stamp orders changes to a field across processes: a Lamport clock, and
// the process, to break ties the same way everywhere.
type stamp struct {
	Clock  uint64 `json:"c"`
	Origin string `json:"o"`
}

func (s stamp) after(o stamp) bool {
	return s.Clock > o.Clock || s.Clock == o.Clock && s.Origin > o.Origin
}

var lamport struct {
	sync.Mutex
	n uint64
}

func tick(seen uint64) uint64 {
	lamport.Lock()
	defer lamport.Unlock()
	lamport.n = max(lamport.n, seen) + 1
	return lamport.n
}

// Docs declares a kind of shared document under name, which must be the
// same in every process. T must encode to a JSON object, such as a struct:
// its top-level fields merge separately, so two people changing different
// fields at once both keep their change; for the same field, the last
// change wins everywhere. Declare it as a package variable:
//
//	type Note struct {
//		Title string `json:"title"`
//		Body  string `json:"body"`
//	}
//
//	var notes = goscript.Docs("note", goscript.DocOptions[Note]{Load: loadNote, Save: saveNote})
//
//	func Setup(c *goscript.Component) {
//		note := notes.Open(c, "note:"+id)
//		note.Show(c, "note")
//		c.Func("title", func(s string) { note.Update(func(n Note) Note { n.Title = s; return n }) })
//	}
func Docs[T any](name string, opts DocOptions[T]) *DocSet[T] {
	if opts.SaveEvery <= 0 {
		opts.SaveEvery = time.Second
	}
	d := &DocSet[T]{name: name, opts: opts, docs: map[string]*doc[T]{}}
	registerCollab(d)
	return d
}

func (d *DocSet[T]) topic() string { return topicPrefix + "doc." + d.name }

// Doc is a component's handle on an open document.
type Doc[T any] struct {
	set *DocSet[T]
	key string
	doc *doc[T]
}

// Open opens the document key for c until c is destroyed, loading it if
// no component in this process has it open, and asking other processes
// for their copy. It works while rendering, so the page arrives with the
// document. A failed Load rejects the component.
func (d *DocSet[T]) Open(c *Component, key string) *Doc[T] {
	d.mu.Lock()
	x := d.docs[key]
	fresh := x == nil
	if fresh {
		x = &doc[T]{fields: map[string]json.RawMessage{}, stamps: map[string]stamp{}}
		var zero T
		x.value = Global(zero)
		d.docs[key] = x
	}
	x.refs++
	d.mu.Unlock()
	if fresh {
		if d.opts.Load != nil {
			v, err := d.opts.Load(c.Context(), key)
			if err != nil {
				x.err = err
			} else if err := d.apply(x, v, stamp{}); err != nil {
				x.err = err
			}
		}
		if b, ctx := currentBroker(); b != nil {
			hello, _ := json.Marshal(docMessage{Key: key})
			go func() { _ = publishEnvelope(ctx, b, d.topic(), envelope{Hello: true, Data: hello}) }()
		}
	}
	c.releases = append(c.releases, func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		x.refs--
		if x.refs <= 0 && !x.saving {
			delete(d.docs, key)
		}
	})
	if x.err != nil {
		slog.Error("goscript: loading a shared document failed", "doc", d.name, "key", key, "err", x.err)
		c.Reject("This couldn't be loaded.")
	}
	return &Doc[T]{set: d, key: key, doc: x}
}

// apply sets every field of v that was never set, with st.
func (d *DocSet[T]) apply(x *doc[T], v T, st stamp) error {
	fields, err := objectFields(v)
	if err != nil {
		return fmt.Errorf("goscript: shared document %s: %w", d.name, err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, raw := range fields {
		if _, ok := x.stamps[k]; !ok {
			x.fields[k] = raw
			x.stamps[k] = st
		}
	}
	return d.publish(x)
}

// publish rebuilds the document's value from its fields; d.mu is held.
func (d *DocSet[T]) publish(x *doc[T]) error {
	raw, err := json.Marshal(x.fields)
	if err != nil {
		return err
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	x.value.Set(v)
	return nil
}

func objectFields(v any) (map[string]json.RawMessage, error) {
	raw, err := json.Marshal(nilslices.Remove(v))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("its value must encode to a JSON object, not %s", raw)
	}
	return fields, nil
}

// Get returns the document now.
func (x *Doc[T]) Get() T { return x.doc.value.Get() }

// Update changes the document to fn of it. Only the fields fn changed are
// sent to other processes, each with its own stamp, so changes to other
// fields made meanwhile elsewhere are kept.
func (x *Doc[T]) Update(fn func(T) T) error {
	d := x.set
	d.mu.Lock()
	old := x.doc.value.Get()
	next := fn(old)
	before, err := objectFields(old)
	if err != nil {
		d.mu.Unlock()
		return err
	}
	after, err := objectFields(next)
	if err != nil {
		d.mu.Unlock()
		return err
	}
	changed := docMessage{Key: x.key, Fields: map[string]docField{}}
	for k, raw := range after {
		if !bytes.Equal(before[k], raw) {
			st := stamp{Clock: tick(0), Origin: replicaID}
			x.doc.fields[k], x.doc.stamps[k] = raw, st
			changed.Fields[k] = docField{Value: raw, Stamp: st}
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			st := stamp{Clock: tick(0), Origin: replicaID}
			delete(x.doc.fields, k)
			x.doc.stamps[k] = st
			changed.Fields[k] = docField{Stamp: st, Gone: true}
		}
	}
	if len(changed.Fields) == 0 {
		d.mu.Unlock()
		return nil
	}
	err = d.publish(x.doc)
	d.mu.Unlock()
	if err != nil {
		return err
	}
	if b, ctx := currentBroker(); b != nil {
		data, _ := json.Marshal(changed)
		go func() { _ = publishEnvelope(ctx, b, d.topic(), envelope{Data: data}) }()
	}
	d.scheduleSave(x.key, x.doc)
	return nil
}

// Use returns a signal in c that follows the document.
func (x *Doc[T]) Use(c *Component) *Signal[T] { return x.doc.value.Use(c) }

// Show publishes the document in c's markup under name and returns the signal.
func (x *Doc[T]) Show(c *Component, name string) *Signal[T] { return x.doc.value.Show(c, name) }

func (d *DocSet[T]) scheduleSave(key string, x *doc[T]) {
	if d.opts.Save == nil {
		return
	}
	d.mu.Lock()
	if x.saving {
		d.mu.Unlock()
		return
	}
	x.saving = true
	d.mu.Unlock()
	time.AfterFunc(d.opts.SaveEvery, func() {
		d.mu.Lock()
		x.saving = false
		v := x.value.Get()
		if x.refs <= 0 && d.docs[key] == x {
			delete(d.docs, key)
		}
		d.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := d.opts.Save(ctx, key, v); err != nil {
			slog.Error("goscript: saving a shared document failed", "doc", d.name, "key", key, "err", err)
		}
	})
}

// docMessage is changes to a document's fields, or, with Hello, a
// request for its fields from a process that just opened it.
type docMessage struct {
	Key    string              `json:"k"`
	Fields map[string]docField `json:"f,omitempty"`
}

type docField struct {
	Value json.RawMessage `json:"v,omitempty"`
	Stamp stamp           `json:"s"`
	Gone  bool            `json:"g,omitempty"`
}

func (d *DocSet[T]) connect(ctx context.Context, b Broker) {
	_ = b.Subscribe(ctx, d.topic(), func(raw []byte) {
		var e envelope
		if json.Unmarshal(raw, &e) != nil || e.Origin == replicaID {
			return
		}
		var msg docMessage
		if json.Unmarshal(e.Data, &msg) != nil {
			return
		}
		d.mu.Lock()
		defer d.mu.Unlock()
		x := d.docs[msg.Key]
		if x == nil {
			return // nobody here has it open
		}
		if e.Hello {
			// Another process opened it: send every field we know.
			all := docMessage{Key: msg.Key, Fields: map[string]docField{}}
			for k, st := range x.stamps {
				if st.Clock == 0 {
					continue // as loaded: the other process loaded it too
				}
				f := docField{Stamp: st}
				if v, ok := x.fields[k]; ok {
					f.Value = v
				} else {
					f.Gone = true
				}
				all.Fields[k] = f
			}
			if len(all.Fields) > 0 {
				data, _ := json.Marshal(all)
				go func() { _ = publishEnvelope(ctx, b, d.topic(), envelope{Data: data}) }()
			}
			return
		}
		changed := false
		for k, f := range msg.Fields {
			tick(f.Stamp.Clock)
			if !f.Stamp.after(x.stamps[k]) {
				continue
			}
			x.stamps[k] = f.Stamp
			if f.Gone {
				delete(x.fields, k)
			} else {
				x.fields[k] = f.Value
			}
			changed = true
		}
		if changed {
			_ = d.publish(x)
		}
	})
}

func (d *DocSet[T]) heartbeat(context.Context, Broker) {}
func (d *DocSet[T]) leaving(context.Context, Broker)   {}
