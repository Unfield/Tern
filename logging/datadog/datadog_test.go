package datadog

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Unfield/Tern/logging/internal/sinktest"
)

func TestExport(t *testing.T) {
	srv := sinktest.NewServer(t)
	t.Setenv("DD_API_KEY", "key")
	t.Setenv("DD_ENV", "prod")
	s := New(Options{URL: srv.URL + "/api/v2/logs", Service: "shop", Hostname: "web-1", Tags: []string{"team:core"}})
	if err := s.Export(context.Background(), sinktest.Entries()); err != nil {
		t.Fatal(err)
	}
	req := srv.Last(t)
	if req.Header.Get("Dd-Api-Key") != "key" {
		t.Fatalf("header %v", req.Header)
	}
	var got []map[string]any
	if err := json.Unmarshal(req.Body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatal(string(req.Body))
	}
	a := got[0]
	want := map[string]any{"message": "order placed", "status": "info", "service": "shop", "hostname": "web-1", "ddsource": "go", "ddtags": "env:prod,team:core", "timestamp": float64(1794398400000), "request_id": "r1", "dd.trace_id": "11803532876627986230", "dd.span_id": "67667974448284343"}
	for k, v := range want {
		if a[k] != v {
			t.Errorf("%s = %v, want %v", k, a[k], v)
		}
	}
	if b := got[1]; b["status"] != "error" || b["error"] != "unexpected EOF" || b["logger"].(map[string]any)["line"] != float64(42) {
		t.Errorf("error entry %v", b)
	}
}
