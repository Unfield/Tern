package goscript

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sync"
	"time"

	"github.com/Unfield/Tern/internal/nilslices"
)

// Broker carries messages between server processes, so live components on
// every instance of an app see the same Shared values, Presence counts and
// published messages. Any realtime.Broker works, such as
// realtime.NewMemoryBroker or a Postgres broker from realtime/pgbroker;
// set it with kit.WithLiveBroker.
type Broker interface {
	// Publish delivers msg to the topic's subscribers in every process.
	Publish(ctx context.Context, topic string, msg []byte) error
	// Subscribe calls fn for each message on topic until ctx is canceled.
	Subscribe(ctx context.Context, topic string, fn func(msg []byte)) error
}

const topicPrefix = "tern.live."

var (
	brokerMu  sync.Mutex
	broker    Broker
	brokerCtx context.Context
	stopAll   context.CancelFunc
	replicaID = newReplicaID()
	shareds   []interface {
		connect(ctx context.Context, b Broker)
	}
	presences []*PresenceCounter
	topics    = map[string]*topicSubs{}
)

func newReplicaID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SetBroker connects this process's Shared values, Presence counts and
// topics to other processes through b. kit.WithLiveBroker calls it when
// the app starts; nil disconnects.
func SetBroker(b Broker) {
	brokerMu.Lock()
	defer brokerMu.Unlock()
	if broker != nil {
		// This process is leaving: tell the others its visitors are gone,
		// rather than leave them counted until the heartbeat expires.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		for _, p := range presences {
			_ = publishEnvelope(ctx, broker, p.topic(), envelope{Count: 0})
		}
		cancel()
	}
	if stopAll != nil {
		stopAll()
		stopAll = nil
	}
	broker = b
	if b == nil {
		brokerCtx = nil
		for _, t := range topics {
			t.remote = nil
		}
		return
	}
	brokerCtx, stopAll = context.WithCancel(context.Background())
	for _, s := range shareds {
		s.connect(brokerCtx, b)
	}
	for _, p := range presences {
		p.connect(brokerCtx, b)
	}
	for name, t := range topics {
		t.remote = subscribeRemote(brokerCtx, b, name)
	}
	go presenceHeartbeat(brokerCtx, b)
}

func currentBroker() (Broker, context.Context) {
	brokerMu.Lock()
	defer brokerMu.Unlock()
	return broker, brokerCtx
}

// envelope is a message between processes.
type envelope struct {
	Origin string          `json:"o"`
	Clock  uint64          `json:"c,omitempty"`
	Hello  bool            `json:"h,omitempty"`
	Count  int             `json:"n,omitempty"`
	Data   json.RawMessage `json:"d,omitempty"`
}

func publishEnvelope(ctx context.Context, b Broker, topic string, e envelope) error {
	e.Origin = replicaID
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return b.Publish(ctx, topic, raw)
}

// Topics: messages published to every component subscribed, in every
// process.

type topicSubs struct {
	subs   map[*topicSub]bool
	remote context.CancelFunc
}

type topicSub struct {
	c  *Component
	fn func([]byte)
}

// Subscribe calls fn as c, like a Func, for each message published to
// topic with [Publish], in this process or, with a broker, any other. It
// stops when c is destroyed, and does nothing while rendering.
func Subscribe[T any](c *Component, topic string, fn func(T)) {
	if c.rendering {
		return
	}
	sub := &topicSub{c: c, fn: func(raw []byte) {
		var v T
		if json.Unmarshal(raw, &v) == nil {
			fn(v)
		}
	}}
	brokerMu.Lock()
	t := topics[topic]
	if t == nil {
		t = &topicSubs{subs: map[*topicSub]bool{}}
		topics[topic] = t
		if broker != nil {
			t.remote = subscribeRemote(brokerCtx, broker, topic)
		}
	}
	t.subs[sub] = true
	brokerMu.Unlock()
	c.releases = append(c.releases, func() {
		brokerMu.Lock()
		defer brokerMu.Unlock()
		delete(t.subs, sub)
		if len(t.subs) == 0 {
			if t.remote != nil {
				t.remote()
			}
			delete(topics, topic)
		}
	})
}

func subscribeRemote(ctx context.Context, b Broker, topic string) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	_ = b.Subscribe(ctx, topicPrefix+"topic."+topic, func(msg []byte) {
		var e envelope
		if json.Unmarshal(msg, &e) != nil || e.Origin == replicaID {
			return
		}
		deliverLocal(topic, e.Data)
	})
	return cancel
}

func deliverLocal(topic string, raw []byte) {
	brokerMu.Lock()
	t := topics[topic]
	var subs []*topicSub
	if t != nil {
		for s := range t.subs {
			subs = append(subs, s)
		}
	}
	brokerMu.Unlock()
	for _, s := range subs {
		s.c.post(func() { s.fn(raw) })
	}
}

// Publish sends v to every component subscribed to topic with [Subscribe],
// in this process and, with a broker, every other. A common pattern: a
// Func saves a change to the database, then publishes that it happened, and
// every subscribed component reloads what it shows.
func Publish(ctx context.Context, topic string, v any) error {
	raw, err := json.Marshal(nilslices.Remove(v))
	if err != nil {
		return err
	}
	deliverLocal(topic, raw)
	if b, _ := currentBroker(); b != nil {
		return publishEnvelope(ctx, b, topicPrefix+"topic."+topic, envelope{Data: raw})
	}
	return nil
}

// SharedSignal is a value shared by every component in every process of an
// app, through the broker. When two processes set it at the same moment,
// one value wins everywhere (last writer wins), so Update is atomic only
// within a process: keep counts that must not lose updates in a database.
// Without a broker it behaves like a Global.
type SharedSignal[T any] struct {
	name  string
	g     *GlobalSignal[T]
	mu    sync.Mutex
	clock uint64
	owner string
}

// Shared declares a value shared across processes under name, which must
// be the same in every process and unique in the app. Declare it as a
// package variable.
func Shared[T any](name string, initial T) *SharedSignal[T] {
	s := &SharedSignal[T]{name: name, g: Global(initial)}
	brokerMu.Lock()
	shareds = append(shareds, s)
	if broker != nil {
		s.connect(brokerCtx, broker)
	}
	brokerMu.Unlock()
	return s
}

func (s *SharedSignal[T]) topic() string { return topicPrefix + "shared." + s.name }

func (s *SharedSignal[T]) connect(ctx context.Context, b Broker) {
	_ = b.Subscribe(ctx, s.topic(), func(msg []byte) {
		var e envelope
		if json.Unmarshal(msg, &e) != nil || e.Origin == replicaID {
			return
		}
		if e.Hello {
			// A process started: tell it the value, if it was ever set.
			s.mu.Lock()
			clock, data := s.clock, s.encode()
			s.mu.Unlock()
			if clock > 0 {
				go func() { _ = s.send(ctx, b, clock, data) }()
			}
			return
		}
		var v T
		if json.Unmarshal(e.Data, &v) != nil {
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if e.Clock > s.clock || (e.Clock == s.clock && e.Origin > s.owner) {
			s.clock, s.owner = e.Clock, e.Origin
			s.g.Set(v)
		}
	})
	go func() { _ = publishEnvelope(ctx, b, s.topic(), envelope{Hello: true}) }()
}

func (s *SharedSignal[T]) encode() json.RawMessage {
	b, _ := json.Marshal(nilslices.Remove(s.g.Get()))
	return b
}

func (s *SharedSignal[T]) send(ctx context.Context, b Broker, clock uint64, data json.RawMessage) error {
	return publishEnvelope(ctx, b, s.topic(), envelope{Clock: clock, Data: data})
}

// Get returns the current value without subscribing.
func (s *SharedSignal[T]) Get() T { return s.g.Get() }

// Set changes the value in every process.
func (s *SharedSignal[T]) Set(v T) { s.Update(func(T) T { return v }) }

// Update sets the value to fn of the current one, atomically within this
// process.
func (s *SharedSignal[T]) Update(fn func(T) T) {
	s.mu.Lock()
	old := s.g.Get()
	v := fn(old)
	if reflect.DeepEqual(old, v) {
		s.mu.Unlock()
		return
	}
	s.clock++
	s.owner = replicaID
	clock := s.clock
	s.g.Set(v)
	data, _ := json.Marshal(nilslices.Remove(v))
	s.mu.Unlock()
	if b, ctx := currentBroker(); b != nil {
		go func() { _ = s.send(ctx, b, clock, data) }()
	}
}

// Use returns a signal in c that follows the shared value.
func (s *SharedSignal[T]) Use(c *Component) *Signal[T] { return s.g.Use(c) }

// Show publishes the shared value in c's markup under name and returns the
// signal that follows it.
func (s *SharedSignal[T]) Show(c *Component, name string) *Signal[T] { return s.g.Show(c, name) }

// PresenceCounter counts the components that joined it, across every
// process of an app, such as the visitors on a page. Processes report their
// counts through the broker; a process that stops reporting, because it
// crashed, drops out of the total after half a minute.
type PresenceCounter struct {
	name   string
	mu     sync.Mutex
	local  int
	remote map[string]presenceEntry
	total  *GlobalSignal[int]
}

type presenceEntry struct {
	n    int
	seen time.Time
}

const (
	presenceEvery  = 10 * time.Second
	presenceExpiry = 30 * time.Second
)

// Presence declares a counter under name, which must be the same in every
// process. Declare it as a package variable.
func Presence(name string) *PresenceCounter {
	p := &PresenceCounter{name: name, remote: map[string]presenceEntry{}, total: Global(0)}
	brokerMu.Lock()
	presences = append(presences, p)
	if broker != nil {
		p.connect(brokerCtx, broker)
	}
	brokerMu.Unlock()
	return p
}

func (p *PresenceCounter) topic() string { return topicPrefix + "presence." + p.name }

func (p *PresenceCounter) connect(ctx context.Context, b Broker) {
	_ = b.Subscribe(ctx, p.topic(), func(msg []byte) {
		var e envelope
		if json.Unmarshal(msg, &e) != nil || e.Origin == replicaID {
			return
		}
		if e.Hello {
			// A process started: tell it our count rather than make it
			// wait for the next heartbeat.
			p.mu.Lock()
			n := p.local
			p.mu.Unlock()
			if n > 0 {
				go func() { _ = publishEnvelope(ctx, b, p.topic(), envelope{Count: n}) }()
			}
			return
		}
		p.mu.Lock()
		if e.Count == 0 {
			delete(p.remote, e.Origin)
		} else {
			p.remote[e.Origin] = presenceEntry{n: e.Count, seen: time.Now()}
		}
		p.recount()
		p.mu.Unlock()
	})
	go func() { _ = publishEnvelope(ctx, b, p.topic(), envelope{Hello: true}) }()
}

// recount updates the total; p.mu is held.
func (p *PresenceCounter) recount() {
	n := p.local
	for _, e := range p.remote {
		n += e.n
	}
	p.total.Set(n)
}

func (p *PresenceCounter) report() {
	if b, ctx := currentBroker(); b != nil {
		p.mu.Lock()
		n := p.local
		p.mu.Unlock()
		go func() { _ = publishEnvelope(ctx, b, p.topic(), envelope{Count: n}) }()
	}
}

// Join counts c until it is destroyed. It does nothing while rendering.
func (p *PresenceCounter) Join(c *Component) {
	if c.rendering {
		return
	}
	p.mu.Lock()
	p.local++
	p.recount()
	p.mu.Unlock()
	p.report()
	c.releases = append(c.releases, func() {
		p.mu.Lock()
		p.local--
		p.recount()
		p.mu.Unlock()
		p.report()
	})
}

// Count returns the current total.
func (p *PresenceCounter) Count() int { return p.total.Get() }

// Use returns a signal in c that follows the total.
func (p *PresenceCounter) Use(c *Component) *Signal[int] { return p.total.Use(c) }

// Show publishes the total in c's markup under name and returns the signal
// that follows it.
func (p *PresenceCounter) Show(c *Component, name string) *Signal[int] { return p.total.Show(c, name) }

// presenceHeartbeat reports every counter regularly and drops processes
// that stopped reporting.
func presenceHeartbeat(ctx context.Context, b Broker) {
	t := time.NewTicker(presenceEvery)
	defer t.Stop()
	for {
		brokerMu.Lock()
		list := append([]*PresenceCounter(nil), presences...)
		brokerMu.Unlock()
		for _, p := range list {
			p.mu.Lock()
			for id, e := range p.remote {
				if time.Since(e.seen) > presenceExpiry {
					delete(p.remote, id)
				}
			}
			p.recount()
			n := p.local
			p.mu.Unlock()
			_ = publishEnvelope(ctx, b, p.topic(), envelope{Count: n})
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
