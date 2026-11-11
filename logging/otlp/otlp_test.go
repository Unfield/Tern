package otlp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"github.com/Unfield/Tern/logging"
	"github.com/Unfield/Tern/logging/internal/sinktest"
)

func TestExport(t *testing.T) {
	srv := sinktest.NewServer(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL+"/")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "x-api-key=k%201")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment=prod")
	s := New(Options{ServiceName: "shop"})
	if err := s.Export(context.Background(), sinktest.Entries()); err != nil {
		t.Fatal(err)
	}
	req := srv.Last(t)
	if req.Path != "/v1/logs" || req.Header.Get("X-Api-Key") != "k 1" || req.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("request %s %v", req.Path, req.Header)
	}
	var got exportRequest
	if err := json.Unmarshal(req.Body, &got); err != nil {
		t.Fatal(err)
	}
	res := got.ResourceLogs[0].Resource.Attributes
	if len(res) != 2 || res[0].Key != "deployment.environment" || res[1].Key != "service.name" || *res[1].Value.StringValue != "shop" {
		t.Fatalf("resource %s", body(res))
	}
	recs := got.ResourceLogs[0].ScopeLogs[0].LogRecords
	if len(recs) != 2 {
		t.Fatal(len(recs))
	}
	r := recs[0]
	if r.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" || r.SpanID != "00f067aa0ba902b7" || r.SeverityNumber != 9 || *r.Body.StringValue != "order placed" || r.TimeUnixNano != "1794398400000000000" {
		t.Fatalf("record %s", body(r))
	}
	want := `[{"key":"request_id","value":{"stringValue":"r1"}},{"key":"items","value":{"intValue":"3"}},{"key":"total","value":{"doubleValue":9.5}},{"key":"paid","value":{"boolValue":true}},{"key":"took","value":{"stringValue":"1.5s"}},{"key":"user","value":{"kvlistValue":{"values":[{"key":"id","value":{"stringValue":"u1"}}]}}}]`
	if g := body(r.Attributes); g != want {
		t.Fatalf("attributes\n got %s\nwant %s", g, want)
	}
	if e := recs[1]; e.SeverityNumber != 17 || body(e.Attributes) != `[{"key":"error","value":{"stringValue":"unexpected EOF"}},{"key":"code.file.path","value":{"stringValue":"/app/pay.go"}},{"key":"code.line.number","value":{"intValue":"42"}},{"key":"code.function.name","value":{"stringValue":"main.pay"}}]` {
		t.Fatalf("error record %s", body(e))
	}
}

func TestStatuses(t *testing.T) {
	srv := sinktest.NewServer(t)
	s := New(Options{Endpoint: srv.URL + "/v1/logs"})
	srv.Statuses = []int{http.StatusServiceUnavailable, http.StatusBadRequest}
	if err := s.Export(context.Background(), sinktest.Entries()); err == nil || logging.IsPermanent(err) {
		t.Fatalf("503: %v", err)
	}
	if err := s.Export(context.Background(), sinktest.Entries()); !logging.IsPermanent(err) {
		t.Fatalf("400: %v", err)
	}
}

func TestSeverity(t *testing.T) {
	for l, want := range map[int]int{-4: 5, 0: 9, 2: 11, 4: 13, 8: 17, 100: 24, -100: 1} {
		if got := Severity(levelOf(l)); got != want {
			t.Errorf("%d: %d, want %d", l, got, want)
		}
	}
}

func body(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func levelOf(n int) slog.Level { return slog.Level(n) }
