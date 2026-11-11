// Package redis is a cache.Store in Redis, or anything that speaks its
// protocol (Valkey, KeyDB, Dragonfly), shared by every process of the app.
// It speaks RESP itself, with the four commands the cache needs, so it
// adds no dependency.
//
//	store, err := redis.New(os.Getenv("REDIS_URL")) // redis://:password@host:6379/0
//	c := cache.New(store)
package redis

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Store is a cache.Store in Redis.
type Store struct {
	addr     string
	user     string
	password string
	db       int
	tls      bool
	timeout  time.Duration
	pool     chan *conn
}

// New connects to the server at rawURL, redis://[user:password@]host:port[/db]
// or rediss:// for TLS. Connections are made as needed, up to 16 idle.
func New(rawURL string) (*Store, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("redis: %w", err)
	}
	if u.Scheme != "redis" && u.Scheme != "rediss" {
		return nil, fmt.Errorf("redis: %q isn't a redis:// or rediss:// URL", u.Redacted())
	}
	s := &Store{addr: u.Host, tls: u.Scheme == "rediss", timeout: 5 * time.Second, pool: make(chan *conn, 16)}
	if !strings.Contains(s.addr, ":") {
		s.addr += ":6379"
	}
	if u.User != nil {
		s.user = u.User.Username()
		s.password, _ = u.User.Password()
	}
	if p := strings.Trim(u.Path, "/"); p != "" {
		if s.db, err = strconv.Atoi(p); err != nil {
			return nil, fmt.Errorf("redis: database %q isn't a number", p)
		}
	}
	return s, nil
}

// Ping checks that the server answers.
func (s *Store) Ping(ctx context.Context) error {
	_, err := s.do(ctx, "PING")
	return err
}

// Close closes the idle connections.
func (s *Store) Close() error {
	for {
		select {
		case c := <-s.pool:
			_ = c.Close()
		default:
			return nil
		}
	}
}

// Get implements cache.Store.
func (s *Store) Get(ctx context.Context, key string) ([]byte, bool, error) {
	v, err := s.do(ctx, "GET", key)
	if err != nil {
		return nil, false, err
	}
	if v == nil {
		return nil, false, nil
	}
	b, ok := v.([]byte)
	return b, ok, nil
}

// Set implements cache.Store.
func (s *Store) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	args := []any{"SET", key, value}
	if ttl > 0 {
		args = append(args, "PX", strconv.FormatInt(max(ttl.Milliseconds(), 1), 10))
	}
	_, err := s.do(ctx, args...)
	return err
}

// Delete implements cache.Store.
func (s *Store) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	args := []any{"DEL"}
	for _, k := range keys {
		args = append(args, k)
	}
	_, err := s.do(ctx, args...)
	return err
}

// Incr implements cache.Store.
func (s *Store) Incr(ctx context.Context, key string) (int64, error) {
	v, err := s.do(ctx, "INCR", key)
	if err != nil {
		return 0, err
	}
	n, ok := v.(int64)
	if !ok {
		return 0, errors.New("redis: INCR didn't answer a number")
	}
	return n, nil
}

type conn struct {
	net.Conn
	r *bufio.Reader
}

// Error is an error the server answered with.
type Error string

func (e Error) Error() string { return "redis: " + string(e) }

func (s *Store) get(ctx context.Context) (*conn, error) {
	select {
	case c := <-s.pool:
		return c, nil
	default:
	}
	d := net.Dialer{Timeout: s.timeout}
	var nc net.Conn
	var err error
	if s.tls {
		host, _, _ := net.SplitHostPort(s.addr)
		nc, err = (&tls.Dialer{NetDialer: &d, Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", s.addr)
	} else {
		nc, err = d.DialContext(ctx, "tcp", s.addr)
	}
	if err != nil {
		return nil, fmt.Errorf("redis: %w", err)
	}
	c := &conn{Conn: nc, r: bufio.NewReader(nc)}
	if s.password != "" {
		args := []any{"AUTH", s.password}
		if s.user != "" {
			args = []any{"AUTH", s.user, s.password}
		}
		if _, err := c.roundTrip(ctx, s.timeout, args); err != nil {
			_ = c.Close()
			return nil, err
		}
	}
	if s.db != 0 {
		if _, err := c.roundTrip(ctx, s.timeout, []any{"SELECT", strconv.Itoa(s.db)}); err != nil {
			_ = c.Close()
			return nil, err
		}
	}
	return c, nil
}

func (s *Store) do(ctx context.Context, args ...any) (any, error) {
	c, err := s.get(ctx)
	if err != nil {
		return nil, err
	}
	v, err := c.roundTrip(ctx, s.timeout, args)
	var re Error
	if err != nil && !errors.As(err, &re) {
		_ = c.Close() // broken: don't reuse
		return nil, err
	}
	select {
	case s.pool <- c:
	default:
		_ = c.Close()
	}
	return v, err
}

func (c *conn) roundTrip(ctx context.Context, timeout time.Duration, args []any) (any, error) {
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = c.SetDeadline(deadline)
	var b []byte
	b = append(b, '*')
	b = strconv.AppendInt(b, int64(len(args)), 10)
	b = append(b, '\r', '\n')
	for _, a := range args {
		var v []byte
		switch x := a.(type) {
		case string:
			v = []byte(x)
		case []byte:
			v = x
		default:
			v = []byte(fmt.Sprint(x))
		}
		b = append(b, '$')
		b = strconv.AppendInt(b, int64(len(v)), 10)
		b = append(b, '\r', '\n')
		b = append(b, v...)
		b = append(b, '\r', '\n')
	}
	if _, err := c.Write(b); err != nil {
		return nil, fmt.Errorf("redis: %w", err)
	}
	return c.read()
}

// read reads one RESP2 reply.
func (c *conn) read() (any, error) {
	line, err := c.r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("redis: %w", err)
	}
	line = strings.TrimSuffix(line, "\r\n")
	if line == "" {
		return nil, errors.New("redis: empty reply")
	}
	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return nil, Error(line[1:])
	case ':':
		return strconv.ParseInt(line[1:], 10, 64)
	case '$':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, fmt.Errorf("redis: bad reply %q", line)
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(c.r, buf); err != nil {
			return nil, fmt.Errorf("redis: %w", err)
		}
		return buf[:n], nil
	case '*':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, fmt.Errorf("redis: bad reply %q", line)
		}
		if n < 0 {
			return nil, nil
		}
		out := make([]any, n)
		for i := range out {
			if out[i], err = c.read(); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("redis: unknown reply %q", line)
}
