// Package sinktest has helpers for the sinks' tests.
package sinktest

import (
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Unfield/Tern/logging"
)

// Request is a request a Server got, its body decompressed.
type Request struct {
	Path   string
	Header http.Header
	Body   []byte
}

// Server records requests and answers them with the next status from
// Statuses, then 200s.
type Server struct {
	*httptest.Server
	mu       sync.Mutex
	Requests []Request
	Statuses []int
}

// NewServer starts a Server, closed when t ends.
func NewServer(t *testing.T) *Server {
	s := &Server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			zr, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			body = zr
		}
		b, _ := io.ReadAll(body)
		s.mu.Lock()
		s.Requests = append(s.Requests, Request{Path: r.URL.Path, Header: r.Header.Clone(), Body: b})
		status := http.StatusOK
		if len(s.Statuses) > 0 {
			status, s.Statuses = s.Statuses[0], s.Statuses[1:]
		}
		s.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(s.Close)
	return s
}

// Last returns the last request.
func (s *Server) Last(t *testing.T) Request {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Requests) == 0 {
		t.Fatal("no requests")
	}
	return s.Requests[len(s.Requests)-1]
}

// Entries are a few entries covering the kinds of values.
func Entries() []logging.Entry {
	t := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	return []logging.Entry{
		{Time: t, Level: slog.LevelInfo, Message: "order placed", Attrs: []slog.Attr{
			slog.String(logging.RequestIDKey, "r1"),
			slog.String(logging.TraceIDKey, "4bf92f3577b34da6a3ce929d0e0e4736"),
			slog.String(logging.SpanIDKey, "00f067aa0ba902b7"),
			slog.Int("items", 3),
			slog.Float64("total", 9.5),
			slog.Bool("paid", true),
			slog.Duration("took", 1500*time.Millisecond),
			slog.Group("user", slog.String("id", "u1")),
		}},
		{Time: t.Add(time.Second), Level: slog.LevelError, Message: "payment failed", Attrs: []slog.Attr{
			slog.Any("error", io.ErrUnexpectedEOF),
		}, Source: &slog.Source{File: "/app/pay.go", Line: 42, Function: "main.pay"}},
	}
}
