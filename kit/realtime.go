package kit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// EventStream is a server-sent events connection, opened with
// [Ctx.EventStream]. Browsers read it with EventSource, or with stream()
// from @ternkit/tern.
type EventStream struct {
	ctx  context.Context
	w    http.ResponseWriter
	rc   *http.ResponseController
	mu   sync.Mutex
	last string
}

// Event is one server-sent event.
type Event struct {
	// Name is the event type; "" is the default "message" event.
	Name string
	// ID is stored by the browser and sent back as Last-Event-ID when it
	// reconnects, so a stream can resume.
	ID string
	// Data is sent as JSON, unless it is a string.
	Data any
	// Retry tells the browser how long to wait before reconnecting.
	Retry time.Duration
}

// EventStream responds with a stream of server-sent events. fn runs until
// it returns, the client disconnects, or the server shuts down; its ctx is
// canceled in the last two cases. A comment is sent every 20 seconds to
// keep proxies from closing an idle connection.
//
//	func GET(ctx *kit.Ctx) error {
//		updates := hub.Subscribe(ctx, "status")
//		return ctx.EventStream(func(ctx context.Context, s *kit.EventStream) error {
//			for {
//				select {
//				case <-ctx.Done():
//					return nil
//				case u := <-updates:
//					if err := s.Send("status", u); err != nil {
//						return err
//					}
//				}
//			}
//		})
//	}
func (c *Ctx) EventStream(fn func(ctx context.Context, s *EventStream) error) error {
	w := c.Writer()
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // nginx: don't buffer the stream
	w.WriteHeader(http.StatusOK)

	ctx, cancel := c.app.streamContext(c)
	defer cancel()
	s := &EventStream{ctx: ctx, w: w, rc: http.NewResponseController(w), last: c.Request.Header.Get("Last-Event-ID")}
	if err := s.flush(); err != nil {
		return nil //nolint:nilerr // the client is gone
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				if s.write(": keep-alive\n\n") != nil {
					cancel()
					return
				}
			}
		}
	}()

	err := fn(ctx, s)
	if ctx.Err() != nil {
		return nil // the client left or the server is stopping: not an error
	}
	return err
}

// LastEventID returns the ID of the last event the browser received before
// it reconnected, or "".
func (s *EventStream) LastEventID() string { return s.last }

// Send sends an event with a name and data, which is encoded as JSON unless
// it is a string.
func (s *EventStream) Send(name string, data any) error {
	return s.SendEvent(Event{Name: name, Data: data})
}

// SendEvent sends an event with all its fields.
func (s *EventStream) SendEvent(e Event) error {
	var b strings.Builder
	if e.ID != "" {
		if strings.ContainsAny(e.ID, "\r\n\x00") {
			return errors.New("kit: an event ID cannot contain newlines or NUL")
		}
		fmt.Fprintf(&b, "id: %s\n", e.ID)
	}
	if e.Name != "" {
		if strings.ContainsAny(e.Name, "\r\n") {
			return errors.New("kit: an event name cannot contain newlines")
		}
		fmt.Fprintf(&b, "event: %s\n", e.Name)
	}
	if e.Retry > 0 {
		fmt.Fprintf(&b, "retry: %d\n", e.Retry.Milliseconds())
	}
	var data string
	switch v := e.Data.(type) {
	case string:
		data = v
	case nil:
	default:
		j, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("kit: encoding event data: %w", err)
		}
		data = string(j)
	}
	for _, line := range strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n") {
		fmt.Fprintf(&b, "data: %s\n", line)
	}
	b.WriteString("\n")
	return s.write(b.String())
}

func (s *EventStream) write(p string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if _, err := s.w.Write([]byte(p)); err != nil {
		return err
	}
	return s.rc.Flush()
}

func (s *EventStream) flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rc.Flush()
}

// WebSocket is a WebSocket connection, opened with [Ctx.WebSocket].
type WebSocket struct {
	conn *websocket.Conn
	ctx  context.Context
}

// WebSocketOptions configure [Ctx.WebSocket].
type WebSocketOptions struct {
	// Origins lists other origins allowed to connect, as host patterns
	// such as "app.example.com" or "*.example.com". Pages on the app's own
	// origin can always connect; browsers on other sites cannot, unless
	// they are listed here.
	Origins []string
	// Subprotocols the server accepts, in order of preference.
	Subprotocols []string
	// ReadLimit caps the size of a received message, in bytes. The default
	// is 1 MiB.
	ReadLimit int64
}

// WebSocket upgrades the request to a WebSocket connection and runs fn
// until it returns, the client disconnects, or the server shuts down; its
// ctx is canceled in the last two cases. Connections from other sites are
// refused unless their origin is allowed in opts.
//
//	func GET(ctx *kit.Ctx) error {
//		return ctx.WebSocket(nil, func(ctx context.Context, ws *kit.WebSocket) error {
//			for {
//				var msg Message
//				if err := ws.Receive(&msg); err != nil {
//					return err
//				}
//				if err := ws.Send(Reply{Text: msg.Text}); err != nil {
//					return err
//				}
//			}
//		})
//	}
func (c *Ctx) WebSocket(opts *WebSocketOptions, fn func(ctx context.Context, ws *WebSocket) error) error {
	if opts == nil {
		opts = &WebSocketOptions{}
	}
	c.applyHeaders()
	conn, err := websocket.Accept(c.writer, c.Request, &websocket.AcceptOptions{
		OriginPatterns: opts.Origins,
		Subprotocols:   opts.Subprotocols,
	})
	if err != nil {
		return nil //nolint:nilerr // Accept has already written the error response
	}
	limit := opts.ReadLimit
	if limit <= 0 {
		limit = 1 << 20
	}
	conn.SetReadLimit(limit)

	// Reads and writes use the request's context. Canceling a read would
	// drop the connection without a close frame, so on shutdown the
	// connection is closed with "going away" instead, which ends a pending
	// Receive with a close error.
	ws := &WebSocket{conn: conn, ctx: c.Request.Context()}
	goingAway := context.AfterFunc(c.app.shutdownCtx, func() {
		_ = conn.Close(websocket.StatusGoingAway, "server shutting down")
	})
	defer goingAway()
	ctx, cancel := c.app.streamContext(c)
	defer cancel()
	err = fn(ctx, ws)

	switch {
	case c.app.stopping():
		return nil
	case err == nil, ws.closed(err):
		_ = conn.Close(websocket.StatusNormalClosure, "")
		return nil
	default:
		_ = conn.Close(websocket.StatusInternalError, "internal error")
		return err
	}
}

// Send sends v as a JSON text message.
func (ws *WebSocket) Send(v any) error { return wsjson.Write(ws.ctx, ws.conn, v) }

// Receive reads the next message, which must be JSON, into v. It returns
// an error when the connection closes; [WebSocket] treats a normal close
// as the end of the connection, not a failure.
func (ws *WebSocket) Receive(v any) error { return wsjson.Read(ws.ctx, ws.conn, v) }

// SendText sends a text message as is.
func (ws *WebSocket) SendText(s string) error {
	return ws.conn.Write(ws.ctx, websocket.MessageText, []byte(s))
}

// ReceiveText reads the next message as text.
func (ws *WebSocket) ReceiveText() (string, error) {
	_, b, err := ws.conn.Read(ws.ctx)
	return string(b), err
}

// Subprotocol returns the subprotocol the client and server agreed on.
func (ws *WebSocket) Subprotocol() string { return ws.conn.Subprotocol() }

// Conn returns the underlying github.com/coder/websocket connection, for
// binary messages, pings and other details.
func (ws *WebSocket) Conn() *websocket.Conn { return ws.conn }

// closed reports whether err means the client closed the connection.
func (ws *WebSocket) closed(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	switch websocket.CloseStatus(err) {
	case websocket.StatusNormalClosure, websocket.StatusGoingAway, websocket.StatusNoStatusRcvd:
		return true
	}
	return ws.ctx.Err() != nil
}

// streamContext is canceled when the request ends or the server begins
// shutting down, so long-lived connections don't hold up a graceful stop.
func (a *App) streamContext(c *Ctx) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(c.Request.Context())
	stop := context.AfterFunc(a.shutdownCtx, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

func (a *App) stopping() bool { return a.shutdownCtx.Err() != nil }
