package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/logging"
)

// devLogs is tern dev's log view: the app's log records, which the logging
// package sends to an ingest endpoint on a loopback port of its own, and
// the requests tern dev proxies to the app, which carry the request ID the
// records have. The browser gets them at /__tern/dev/logs.
type devLogs struct {
	token string
	url   string // the ingest endpoint, for the app's environment

	mu      sync.Mutex
	items   []devLogItem // the newest max, oldest first
	seq     int64
	max     int
	clients map[chan struct{}]bool
}

// devLogItem is one row of the view.
type devLogItem struct {
	Seq   int64          `json:"seq"`
	Kind  string         `json:"kind"` // log, request or event
	Time  time.Time      `json:"time"`
	Level string         `json:"level,omitempty"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs,omitempty"`
	// Requests only.
	Method    string  `json:"method,omitempty"`
	Path      string  `json:"path,omitempty"`
	Status    int     `json:"status,omitempty"`
	Millis    float64 `json:"ms,omitempty"`
	RequestID string  `json:"request_id,omitempty"`
}

func newDevLogs() *devLogs {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return &devLogs{token: hex.EncodeToString(b[:]), max: 5000, clients: map[chan struct{}]bool{}}
}

// listen starts the ingest endpoint on a free loopback port.
func (l *devLogs) listen() (stop func(), err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	l.url = "http://" + ln.Addr().String() + "/ingest"
	srv := &http.Server{Handler: http.HandlerFunc(l.ingest), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return func() { _ = srv.Close() }, nil
}

// env is what the app needs to send its records here.
func (l *devLogs) env() []string {
	if l == nil || l.url == "" {
		return nil
	}
	return []string{"TERN_DEV_LOGS=" + l.url, "TERN_DEV_LOGS_TOKEN=" + l.token}
}

func (l *devLogs) ingest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/ingest" {
		http.NotFound(w, r)
		return
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(l.token)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var entries []struct {
		Time    time.Time      `json:"time"`
		Level   string         `json:"level"`
		Message string         `json:"msg"`
		Attrs   map[string]any `json:"attrs"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(&entries); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, e := range entries {
		item := devLogItem{Kind: "log", Time: e.Time, Level: e.Level, Msg: e.Message, Attrs: e.Attrs}
		if id, ok := e.Attrs["request_id"].(string); ok {
			item.RequestID = id
		}
		l.add(item)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (l *devLogs) add(item devLogItem) {
	l.mu.Lock()
	l.seq++
	item.Seq = l.seq
	if len(l.items) >= l.max {
		l.items = append(l.items[:0], l.items[len(l.items)-l.max+1:]...)
	}
	l.items = append(l.items, item)
	for ch := range l.clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	l.mu.Unlock()
}

// event adds a row about tern dev itself, such as a restart.
func (l *devLogs) event(msg string) {
	if l == nil {
		return
	}
	l.add(devLogItem{Kind: "event", Time: time.Now(), Msg: msg})
}

// since returns the items after seq.
func (l *devLogs) since(seq int64) []devLogItem {
	l.mu.Lock()
	defer l.mu.Unlock()
	i := len(l.items)
	for i > 0 && l.items[i-1].Seq > seq {
		i--
	}
	return append([]devLogItem(nil), l.items[i:]...)
}

func (l *devLogs) clear() {
	l.mu.Lock()
	l.items = nil
	l.mu.Unlock()
}

// recorder notes a proxied response's status.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// proxy serves r through next and records it.
func (l *devLogs) proxy(next http.Handler, w http.ResponseWriter, r *http.Request) {
	if l == nil {
		next.ServeHTTP(w, r)
		return
	}
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w}
	next.ServeHTTP(rec, r)
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path = logging.RedactURL(&url.URL{Path: r.URL.Path, RawQuery: r.URL.RawQuery})
	}
	l.add(devLogItem{
		Kind:      "request",
		Time:      start,
		Msg:       r.Method + " " + path,
		Method:    r.Method,
		Path:      path,
		Status:    rec.status,
		Millis:    float64(time.Since(start).Microseconds()) / 1000,
		RequestID: w.Header().Get("X-Request-Id"),
	})
}

// serve answers /__tern/dev/logs and what is under it.
func (l *devLogs) serve(w http.ResponseWriter, r *http.Request) {
	if l == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	switch strings.TrimPrefix(r.URL.Path, "/__tern/dev/logs") {
	case "", "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, devLogsPage)
	case "/stream":
		l.stream(w, r)
	case "/clear":
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		l.clear()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// stream sends the items as server-sent events: those after Last-Event-ID
// (or all), then each new one.
func (l *devLogs) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	var seq int64
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		seq, _ = strconv.ParseInt(v, 10, 64)
	}
	ch := make(chan struct{}, 1)
	l.mu.Lock()
	l.clients[ch] = true
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		delete(l.clients, ch)
		l.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		items := l.since(seq)
		if len(items) > 0 {
			b, _ := json.Marshal(items)
			seq = items[len(items)-1].Seq
			_, _ = io.WriteString(w, "id: "+strconv.FormatInt(seq, 10)+"\ndata: ")
			_, _ = w.Write(b)
			_, _ = io.WriteString(w, "\n\n")
		} else {
			_, _ = io.WriteString(w, ": idle\n\n")
		}
		flusher.Flush()
		select {
		case <-ch:
			// Gather what comes close together into one event.
			time.Sleep(30 * time.Millisecond)
		case <-ping.C:
		case <-r.Context().Done():
			return
		}
	}
}
