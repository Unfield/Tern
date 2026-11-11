package otlp

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Unfield/Tern/tracing"
)

func TestExport(t *testing.T) {
	var body []byte
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		body, _ = io.ReadAll(zr)
	}))
	defer srv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)
	e := New(Options{ServiceName: "shop"})
	sc, _ := tracing.ParseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	start := time.Unix(1794398400, 0)
	err := e.Export(context.Background(), []tracing.SpanData{{
		Context: sc, Parent: tracing.SpanID{1, 2, 3, 4, 5, 6, 7, 8}, Name: "GET /blog/slug.param", Kind: tracing.Server,
		Start: start, End: start.Add(1500 * time.Microsecond),
		Attrs:  []slog.Attr{slog.Int("http.response.status_code", 500)},
		Events: []tracing.Event{{Name: "exception", Time: start, Attrs: []slog.Attr{slog.String("exception.message", "boom")}}},
		Status: tracing.Status{Code: tracing.Error, Message: "boom"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/traces" {
		t.Errorf("path %s", path)
	}
	var got exportRequest
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	s := got.ResourceSpans[0].ScopeSpans[0].Spans[0]
	if s.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" || s.SpanID != "00f067aa0ba902b7" || s.ParentSpanID != "0102030405060708" || s.Kind != 2 {
		t.Errorf("ids %+v", s)
	}
	if s.Start != "1794398400000000000" || s.End != "1794398400001500000" || s.Status.Code != 2 || s.Status.Message != "boom" {
		t.Errorf("times or status %+v", s)
	}
	if len(s.Events) != 1 || s.Events[0].Name != "exception" || *s.Attributes[0].Value.IntValue != "500" {
		t.Errorf("events or attributes %+v", s)
	}
	if res := got.ResourceSpans[0].Resource.Attributes; len(res) != 1 || *res[0].Value.StringValue != "shop" {
		t.Errorf("resource %+v", res)
	}
}

func TestRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer srv.Close()
	err := New(Options{Endpoint: srv.URL + "/v1/traces"}).Export(context.Background(), []tracing.SpanData{{Name: "x"}})
	if err == nil {
		t.Fatal("a 401 was accepted")
	}
}
