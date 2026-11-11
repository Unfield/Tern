package loki

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Unfield/Tern/logging"
	"github.com/Unfield/Tern/logging/internal/sinktest"
)

func TestExport(t *testing.T) {
	srv := sinktest.NewServer(t)
	s := New(Options{URL: srv.URL, Labels: map[string]string{"app": "shop"}, Username: "u", Password: "p", TenantID: "t1", Metadata: []string{logging.RequestIDKey}})
	if err := s.Export(context.Background(), sinktest.Entries()); err != nil {
		t.Fatal(err)
	}
	req := srv.Last(t)
	if req.Path != "/loki/api/v1/push" || req.Header.Get("X-Scope-Orgid") != "t1" || req.Header.Get("Authorization") != "Basic dTpw" {
		t.Fatalf("request %s %v", req.Path, req.Header)
	}
	var got struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
			Values [][]any           `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(req.Body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Streams) != 2 || got.Streams[0].Stream["level"] != "error" || got.Streams[1].Stream["level"] != "info" || got.Streams[1].Stream["app"] != "shop" {
		t.Fatalf("streams %s", req.Body)
	}
	info := got.Streams[1].Values[0]
	if info[0] != "1794398400000000000" || len(info) != 3 || info[2].(map[string]any)["request_id"] != "r1" {
		t.Fatalf("value %v", info)
	}
	var line map[string]any
	if err := json.Unmarshal([]byte(info[1].(string)), &line); err != nil {
		t.Fatal(err)
	}
	if line["msg"] != "order placed" || line["items"] != float64(3) || line["user"].(map[string]any)["id"] != "u1" {
		t.Fatalf("line %v", line)
	}
	errLine := got.Streams[0].Values[0]
	if len(errLine) != 2 { // no request_id: no metadata
		t.Fatalf("error value %v", errLine)
	}
}
