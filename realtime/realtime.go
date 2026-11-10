// Package realtime is a publish/subscribe hub for pushing updates to
// browsers over server-sent events or WebSockets:
//
//	var hub = realtime.NewHub[StatusUpdate]()
//
//	// Wherever something changes:
//	hub.Publish("monitors", update)
//
//	// In an endpoint:
//	func GET(ctx *kit.Ctx) error {
//		return realtime.Stream(ctx, hub, "monitors")
//	}
//
// The hub is in memory, so it reaches the browsers connected to this
// process. Apps running several instances can implement [Broker] on top of
// Redis, NATS or Postgres LISTEN/NOTIFY and pass it to [NewHubWith].
package realtime

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Unfield/Tern/kit"
)

// Hub fans out messages of type T to subscribers of a topic. It is safe for
// concurrent use.
type Hub[T any] struct {
	broker Broker
	// Buffer is how many messages a slow subscriber may fall behind before
	// messages to it are dropped. The default is 64.
	Buffer int
}

// Broker carries encoded messages between publishers and subscribers. The
// default broker is in memory.
type Broker interface {
	// Publish delivers msg to the topic's subscribers.
	Publish(ctx context.Context, topic string, msg []byte) error
	// Subscribe calls fn for each message on topic until ctx is canceled.
	// fn must not block for long.
	Subscribe(ctx context.Context, topic string, fn func(msg []byte)) error
}

// NewHub returns a hub backed by an in-memory broker.
func NewHub[T any]() *Hub[T] { return NewHubWith[T](NewMemoryBroker()) }

// NewHubWith returns a hub backed by b.
func NewHubWith[T any](b Broker) *Hub[T] { return &Hub[T]{broker: b} }

// Publish sends v to everyone subscribed to topic.
func (h *Hub[T]) Publish(topic string, v T) error {
	return h.PublishContext(context.Background(), topic, v)
}

// PublishContext is Publish with a context for the broker.
func (h *Hub[T]) PublishContext(ctx context.Context, topic string, v T) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return h.broker.Publish(ctx, topic, b)
}

// Subscribe returns a channel of the messages published to topic until ctx
// is canceled, when the channel is closed. A subscriber that falls more
// than Buffer messages behind misses messages rather than slowing down
// publishers.
func (h *Hub[T]) Subscribe(ctx context.Context, topic string) (<-chan T, error) {
	size := h.Buffer
	if size <= 0 {
		size = 64
	}
	ch := make(chan T, size)
	var mu sync.Mutex
	closed := false
	err := h.broker.Subscribe(ctx, topic, func(msg []byte) {
		var v T
		if json.Unmarshal(msg, &v) != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return
		}
		select {
		case ch <- v:
		default: // full: drop rather than block the publisher
		}
	})
	if err != nil {
		return nil, err
	}
	context.AfterFunc(ctx, func() {
		mu.Lock()
		closed = true
		close(ch)
		mu.Unlock()
	})
	return ch, nil
}

// Stream responds to an endpoint request with a server-sent event stream
// of topic's messages, each as an event named after the topic. Read it in
// the browser with stream() from @ternkit/tern.
func Stream[T any](ctx *kit.Ctx, h *Hub[T], topic string) error {
	return ctx.EventStream(func(sctx context.Context, s *kit.EventStream) error {
		msgs, err := h.Subscribe(sctx, topic)
		if err != nil {
			return err
		}
		for v := range msgs {
			if err := s.Send(topic, v); err != nil {
				return err
			}
		}
		return nil
	})
}

// MemoryBroker delivers messages within one process.
type MemoryBroker struct {
	mu     sync.RWMutex
	next   int
	topics map[string]map[int]func([]byte)
}

// NewMemoryBroker returns an empty in-memory broker.
func NewMemoryBroker() *MemoryBroker {
	return &MemoryBroker{topics: map[string]map[int]func([]byte){}}
}

// Publish implements [Broker].
func (b *MemoryBroker) Publish(_ context.Context, topic string, msg []byte) error {
	b.mu.RLock()
	subs := make([]func([]byte), 0, len(b.topics[topic]))
	for _, fn := range b.topics[topic] {
		subs = append(subs, fn)
	}
	b.mu.RUnlock()
	for _, fn := range subs {
		fn(msg)
	}
	return nil
}

// Subscribe implements [Broker].
func (b *MemoryBroker) Subscribe(ctx context.Context, topic string, fn func([]byte)) error {
	b.mu.Lock()
	id := b.next
	b.next++
	if b.topics[topic] == nil {
		b.topics[topic] = map[int]func([]byte){}
	}
	b.topics[topic][id] = fn
	b.mu.Unlock()
	context.AfterFunc(ctx, func() {
		b.mu.Lock()
		delete(b.topics[topic], id)
		if len(b.topics[topic]) == 0 {
			delete(b.topics, topic)
		}
		b.mu.Unlock()
	})
	return nil
}

// Subscribers returns how many subscriptions topic has, for metrics and
// tests.
func (b *MemoryBroker) Subscribers(topic string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.topics[topic])
}
