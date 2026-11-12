package kit

import (
	"bufio"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type ask struct {
	Question string `json:"question" validate:"required,max=20"`
}

type delta struct {
	Text string `json:"text"`
}

func streamApp(t *testing.T, fn func(*Ctx, ask, *Stream[delta]) error) http.Handler {
	t.Helper()
	m := testManifest()
	m.Routes = append(m.Routes, RouteDef{ID: "/api/ask", Endpoint: map[string]HandlerFunc{
		"POST": StreamEndpoint("POST", fn),
		"GET": StreamOutputEndpoint(func(c *Ctx, out *Stream[delta]) error {
			return out.Send(delta{Text: "<hi> & bye"})
		}),
	}})
	a, _ := newTestApp(t, m)
	return a
}

func TestStream(t *testing.T) {
	app := streamApp(t, func(c *Ctx, in ask, out *Stream[delta]) error {
		switch in.Question {
		case "early":
			return Error(http.StatusTooManyRequests, "Slow down.")
		case "late":
			if err := out.Send(delta{Text: "Hello"}); err != nil {
				return err
			}
			return errors.New("the model went away")
		}
		for _, w := range strings.Fields("Hello there " + in.Question) {
			if err := out.Send(delta{Text: w}); err != nil {
				return err
			}
		}
		return nil
	})

	rec := do(t, app, "POST", "/api/ask", strings.NewReader(`{"question":"Ada"}`), "Content-Type", "application/json")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/event-stream" || rec.Header().Get("X-Accel-Buffering") != "no" {
		t.Fatalf("status %d, headers %v", rec.Code, rec.Header())
	}
	want := "data: {\"text\":\"Hello\"}\n\ndata: {\"text\":\"there\"}\n\ndata: {\"text\":\"Ada\"}\n\nevent: done\ndata: {}\n\n"
	if rec.Body.String() != want {
		t.Errorf("body = %q", rec.Body.String())
	}

	// Before the first value, an error is an ordinary response.
	rec = do(t, app, "POST", "/api/ask", strings.NewReader(`{"question":"early"}`), "Content-Type", "application/json")
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "Slow down.") {
		t.Errorf("early error: %d %s", rec.Code, rec.Body)
	}
	// After it, the stream ends with an error event.
	rec = do(t, app, "POST", "/api/ask", strings.NewReader(`{"question":"late"}`), "Content-Type", "application/json")
	if body := rec.Body.String(); rec.Code != 200 || !strings.HasPrefix(body, "data: {\"text\":\"Hello\"}\n\n") || !strings.Contains(body, "event: error\ndata: {\"message\":") || !strings.Contains(body, `"status":500`) {
		t.Errorf("late error: %d %q", rec.Code, body)
	}
	// The input is checked first.
	rec = do(t, app, "POST", "/api/ask", strings.NewReader(`{"question":""}`), "Content-Type", "application/json")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid input: %d %s", rec.Code, rec.Body)
	}
	// Without input; and values are JSON that can't end the event early.
	rec = do(t, app, "GET", "/api/ask", nil)
	if !strings.HasPrefix(rec.Body.String(), "data: {\"text\":\"\\u003chi\\u003e \\u0026 bye\"}\n\n") {
		t.Errorf("GET = %q", rec.Body.String())
	}
}

// Values arrive as they're sent, not when the endpoint returns, and a
// client that goes away stops the endpoint.
func TestStreamFlushesAndStops(t *testing.T) {
	stopped := make(chan error, 1)
	release := make(chan struct{})
	app := streamApp(t, func(c *Ctx, in ask, out *Stream[delta]) error {
		if err := out.Send(delta{Text: "first"}); err != nil {
			return err
		}
		<-release
		for {
			if err := out.Send(delta{Text: "more"}); err != nil {
				stopped <- err
				return err
			}
			time.Sleep(5 * time.Millisecond)
		}
	})
	srv := httptest.NewServer(app)
	defer srv.Close()
	res, err := http.Post(srv.URL+"/api/ask", "application/json", strings.NewReader(`{"question":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(res.Body).ReadString('\n')
	if err != nil || line != "data: {\"text\":\"first\"}\n" {
		t.Fatalf("first line %q, %v", line, err)
	}
	_ = res.Body.Close()
	close(release)
	select {
	case err := <-stopped:
		if err == nil {
			t.Error("Send succeeded after the client left")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the endpoint kept streaming to nobody")
	}
}
