package kit

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func realtimeApp(t *testing.T) (*App, *httptest.Server) {
	t.Helper()
	m := &Manifest{
		AppHTML: testHTML,
		Routes: []RouteDef{
			{ID: "/events", Endpoint: map[string]HandlerFunc{"GET": func(c *Ctx) error {
				return c.EventStream(func(ctx context.Context, s *EventStream) error {
					if err := s.SendEvent(Event{Name: "hello", ID: "7", Data: map[string]int{"n": 1}, Retry: 2 * time.Second}); err != nil {
						return err
					}
					if err := s.Send("", "line one\nline two"); err != nil {
						return err
					}
					if err := s.Send("resume", s.LastEventID()); err != nil {
						return err
					}
					if s.SendEvent(Event{ID: "bad\nid"}) == nil {
						return errors.New("want an error for a newline in an ID")
					}
					<-ctx.Done() // until the client leaves or the server stops
					return nil
				})
			}}},
			{ID: "/ws", Endpoint: map[string]HandlerFunc{"GET": func(c *Ctx) error {
				return c.WebSocket(&WebSocketOptions{Origins: []string{"trusted.example"}}, func(ctx context.Context, ws *WebSocket) error {
					for {
						var msg map[string]string
						if err := ws.Receive(&msg); err != nil {
							return err
						}
						if err := ws.Send(map[string]string{"echo": msg["text"]}); err != nil {
							return err
						}
					}
				})
			}}},
		},
	}
	a, _ := newTestApp(t, m)
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	return a, srv
}

func TestEventStream(t *testing.T) {
	_, srv := realtimeApp(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/events", nil)
	req.Header.Set("Last-Event-ID", "41")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}

	want := "id: 7\nevent: hello\nretry: 2000\ndata: {\"n\":1}\n\ndata: line one\ndata: line two\n\nevent: resume\ndata: 41\n\n"
	r := bufio.NewReader(res.Body)
	var got strings.Builder
	for got.Len() < len(want) {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read: %v (got so far %q)", err, got.String())
		}
		got.WriteString(line)
	}
	if got.String() != want {
		t.Errorf("stream =\n%q\nwant\n%q", got.String(), want)
	}
}

func TestEventStreamEndsOnShutdown(t *testing.T) {
	a, srv := realtimeApp(t)
	res, err := http.Get(srv.URL + "/events") //nolint:bodyclose // closed below
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	done := make(chan struct{})
	go func() {
		_, _ = bufio.NewReader(res.Body).ReadString(0) // reads until the stream ends
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	_ = a.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the stream is still open after Close")
	}
}

func TestWebSocket(t *testing.T) {
	_, srv := realtimeApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	conn, _, err := websocket.Dial(ctx, url, nil) //nolint:bodyclose // the connection owns it
	if err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Write(ctx, conn, map[string]string{"text": "hi"}); err != nil {
		t.Fatal(err)
	}
	var reply map[string]string
	if err := wsjson.Read(ctx, conn, &reply); err != nil || reply["echo"] != "hi" {
		t.Fatalf("reply = %v, %v", reply, err)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")

	// Other sites cannot connect, unless they are allowed.
	h := http.Header{"Origin": {"https://evil.example"}}
	_, res, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: h}) //nolint:bodyclose // closed below
	if res != nil && res.Body != nil {
		_ = res.Body.Close()
	}
	if err == nil || res == nil || res.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin dial: %v %v", res, err)
	}
	h.Set("Origin", "https://trusted.example")
	trusted, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: h}) //nolint:bodyclose // the connection owns it
	if err != nil {
		t.Fatalf("allowed origin: %v", err)
	}
	_ = trusted.Close(websocket.StatusNormalClosure, "")
}

func TestWebSocketClosedOnShutdown(t *testing.T) {
	a, srv := realtimeApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil) //nolint:bodyclose // the connection owns it
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	_ = a.Close()
	_, _, err = conn.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusGoingAway {
		t.Errorf("close status = %v (%v), want going away", websocket.CloseStatus(err), err)
	}
}

func TestDevProxiesOnlyViteWebSockets(t *testing.T) {
	d := &devServer{}
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Upgrade", "websocket")
	if d.handles(r) {
		t.Error("the app's own WebSocket went to Vite")
	}
	r.Header.Set("Sec-WebSocket-Protocol", "vite-hmr")
	if !d.handles(r) {
		t.Error("Vite's HMR WebSocket did not go to Vite")
	}
}
