package kit

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Unfield/Tern/internal/jsonenc"
)

// Stream sends values to the client of a streaming endpoint as they come,
// such as the text of a model's answer as it's written:
//
//	type Ask struct {
//		Question string `json:"question" validate:"required,max=2000"`
//	}
//
//	type Delta struct {
//		Text string `json:"text"`
//	}
//
//	func POST(ctx *kit.Ctx, in Ask, out *kit.Stream[Delta]) error {
//		for text := range model.Answer(ctx, in.Question) {
//			if err := out.Send(Delta{Text: text}); err != nil {
//				return err // the client went away
//			}
//		}
//		return nil
//	}
//
// The values go out as server-sent events, which proxies know not to
// buffer, and $api's stream reads them in the browser, typed as T. An
// error returned before the first Send is an ordinary error response;
// after it, the client gets it as the stream's error. An endpoint without
// input is func(ctx *kit.Ctx, out *kit.Stream[T]) error.
type Stream[T any] struct {
	c       *Ctx
	rc      *http.ResponseController
	started bool
	sent    int
}

// Send writes v to the client and flushes it. It returns an error once
// the client has gone away: stop then, the request's context is done too.
func (s *Stream[T]) Send(v T) error {
	if err := s.c.Request.Context().Err(); err != nil {
		return err
	}
	b, err := jsonenc.Marshal(v)
	if err != nil {
		return err
	}
	s.start()
	w := s.c.Writer()
	if _, err := w.Write(append(append([]byte("data: "), b...), '\n', '\n')); err != nil {
		return err
	}
	s.sent++
	return s.rc.Flush()
}

// Sent is how many values have been sent.
func (s *Stream[T]) Sent() int { return s.sent }

func (s *Stream[T]) start() {
	if s.started {
		return
	}
	s.started = true
	h := s.c.Writer().Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	// Nginx and others buffer responses unless told not to.
	h.Set("X-Accel-Buffering", "no")
	// A stream lasts as long as it lasts: no write deadline from the server.
	_ = s.rc.SetWriteDeadline(time.Time{})
	s.c.Writer().WriteHeader(http.StatusOK)
}

// end finishes the stream: with a done event, or with err as an error
// event once values went out.
func (s *Stream[T]) end(err error) error {
	if !s.started {
		if err != nil {
			return err
		}
		s.start() // a stream with no values is still a stream
	}
	w := s.c.Writer()
	if err != nil {
		if s.c.Request.Context().Err() != nil {
			return nil //nolint:nilerr // the reader left: nobody is listening for the error
		}
		status, msg := s.c.UserError(err)
		var he *HTTPError
		if errors.As(err, &he) {
			status = he.Status
		}
		b, _ := json.Marshal(map[string]any{"status": status, "message": msg})
		_, _ = w.Write([]byte("event: error\ndata: "))
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n\n"))
	} else {
		_, _ = w.Write([]byte("event: done\ndata: {}\n\n"))
	}
	_ = s.rc.Flush()
	return nil
}

// StreamEndpoint adapts a streaming endpoint with input; generated code calls it.
func StreamEndpoint[In, T any](method string, fn func(*Ctx, In, *Stream[T]) error) HandlerFunc {
	return func(c *Ctx) error {
		var in In
		if err := decodeInput(c, method, &in); err != nil {
			return err
		}
		if err := checkTyped(c, in); err != nil {
			return err
		}
		if v, ok := any(&in).(Validator); ok {
			if err := v.Validate(); err != nil {
				return asBadRequest(err)
			}
		}
		s := &Stream[T]{c: c, rc: http.NewResponseController(c.Writer())}
		return s.end(fn(c, in, s))
	}
}

// StreamOutputEndpoint adapts a streaming endpoint without input.
func StreamOutputEndpoint[T any](fn func(*Ctx, *Stream[T]) error) HandlerFunc {
	return func(c *Ctx) error {
		s := &Stream[T]{c: c, rc: http.NewResponseController(c.Writer())}
		return s.end(fn(c, s))
	}
}
