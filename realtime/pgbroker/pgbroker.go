// Package pgbroker is a [realtime.Broker] on PostgreSQL's LISTEN and
// NOTIFY, so an app running several processes delivers realtime messages
// and live component updates to all of them without another service:
//
//	b, err := pgbroker.New(ctx, os.Getenv("DATABASE_URL"))
//	if err != nil { ... }
//	defer b.Close()
//	app := tern.App(kit.WithLiveBroker(b))
//	hub := realtime.NewHubWith[Update](b)
//
// Messages are delivered to processes connected at the time; a process that
// is reconnecting misses them. PostgreSQL limits a message to about 8000
// bytes, so send IDs or small updates rather than whole documents.
package pgbroker

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrTooLarge is returned by Publish for a message PostgreSQL can't carry.
var ErrTooLarge = errors.New("pgbroker: message too large for NOTIFY (about 8000 bytes)")

// maxPayload is PostgreSQL's NOTIFY payload limit, less a margin.
const maxPayload = 7900

// Options configure a Broker.
type Options struct {
	// Channel is the PostgreSQL channel all topics share. The default is
	// "tern_broker"; apps sharing a database use different channels.
	Channel string
}

// Broker delivers messages between processes through PostgreSQL. It is
// safe for concurrent use.
type Broker struct {
	pool    *pgxpool.Pool
	connStr string
	channel string
	cancel  context.CancelFunc
	done    chan struct{}

	mu    sync.RWMutex
	next  int
	subs  map[string]map[int]func([]byte)
	ready chan struct{} // closed once the first LISTEN is in place
}

// New connects to PostgreSQL and starts listening.
func New(ctx context.Context, connString string, opts ...Options) (*Broker, error) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.Channel == "" {
		o.Channel = "tern_broker"
	}
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	lctx, cancel := context.WithCancel(context.Background())
	b := &Broker{
		pool: pool, connStr: connString, channel: o.Channel, cancel: cancel,
		done: make(chan struct{}), subs: map[string]map[int]func([]byte){}, ready: make(chan struct{}),
	}
	go b.listen(lctx)
	select {
	case <-b.ready:
	case <-ctx.Done():
		b.Close()
		return nil, ctx.Err()
	}
	return b, nil
}

// Close stops listening and closes the connections.
func (b *Broker) Close() {
	b.cancel()
	<-b.done
	b.pool.Close()
}

// Publish implements realtime.Broker.
func (b *Broker) Publish(ctx context.Context, topic string, msg []byte) error {
	payload := topic + "\n" + string(msg)
	if len(payload) > maxPayload {
		return ErrTooLarge
	}
	_, err := b.pool.Exec(ctx, "SELECT pg_notify($1, $2)", b.channel, payload)
	return err
}

// Subscribe implements realtime.Broker.
func (b *Broker) Subscribe(ctx context.Context, topic string, fn func([]byte)) error {
	b.mu.Lock()
	id := b.next
	b.next++
	if b.subs[topic] == nil {
		b.subs[topic] = map[int]func([]byte){}
	}
	b.subs[topic][id] = fn
	b.mu.Unlock()
	context.AfterFunc(ctx, func() {
		b.mu.Lock()
		delete(b.subs[topic], id)
		if len(b.subs[topic]) == 0 {
			delete(b.subs, topic)
		}
		b.mu.Unlock()
	})
	return nil
}

// listen holds a connection that LISTENs, reconnecting with a backoff.
func (b *Broker) listen(ctx context.Context) {
	defer close(b.done)
	var once sync.Once
	delay := 100 * time.Millisecond
	for ctx.Err() == nil {
		err := b.listenOnce(ctx, func() { once.Do(func() { close(b.ready) }) })
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			delay = 100 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, 10*time.Second)
	}
}

func (b *Broker) listenOnce(ctx context.Context, ready func()) error {
	conn, err := pgx.Connect(ctx, b.connStr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{b.channel}.Sanitize()); err != nil {
		return err
	}
	ready()
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		topic, msg, ok := strings.Cut(n.Payload, "\n")
		if !ok {
			continue
		}
		b.mu.RLock()
		subs := make([]func([]byte), 0, len(b.subs[topic]))
		for _, fn := range b.subs[topic] {
			subs = append(subs, fn)
		}
		b.mu.RUnlock()
		for _, fn := range subs {
			fn([]byte(msg))
		}
	}
}
