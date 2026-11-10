package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestServe(t *testing.T) {
	s := &Server{
		Name: "t", Version: "1", Instructions: "be nice",
		Tools: []Tool{
			{Name: "echo", ReadOnly: true, Handler: func(_ context.Context, raw json.RawMessage) (string, error) {
				a, err := Args[struct {
					Say string `json:"say"`
				}](raw)
				return a.Say, err
			}},
			{Name: "fail", Handler: func(context.Context, json.RawMessage) (string, error) { return "", errors.New("nope") }},
			{Name: "boom", Handler: func(context.Context, json.RawMessage) (string, error) { panic("kaboom") }},
		},
		Resources: []Resource{{URI: "x://a", Name: "a", MimeType: "text/plain", Read: func() (string, error) { return "A", nil }}},
	}
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"say":"hi"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"fail"}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"boom"}}`,
		`{"jsonrpc":"2.0","id":6,"method":"resources/read","params":{"uri":"x://a"}}`,
		`{"jsonrpc":"2.0","id":7,"method":"nope"}`,
		`not json`,
		`{"jsonrpc":"2.0","id":"s","method":"ping"}`,
	}, "\n")
	var out strings.Builder
	if err := s.Serve(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	var res []map[string]any
	sc := bufio.NewScanner(strings.NewReader(out.String()))
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("bad line %q", sc.Text())
		}
		res = append(res, m)
	}
	if len(res) != 9 { // the notification gets no answer
		t.Fatalf("got %d responses:\n%s", len(res), out.String())
	}
	get := func(i int, path ...string) any {
		var v any = res[i]
		for _, p := range path {
			v = v.(map[string]any)[p]
		}
		return v
	}
	if get(0, "result", "protocolVersion") != "2025-03-26" || get(0, "result", "instructions") != "be nice" {
		t.Errorf("initialize: %v", res[0])
	}
	if tools := get(1, "result", "tools").([]any); len(tools) != 3 {
		t.Errorf("tools/list: %v", tools)
	}
	text := func(i int) string { return get(i, "result", "content").([]any)[0].(map[string]any)["text"].(string) }
	if text(2) != "hi" || text(3) != "nope" || get(3, "result", "isError") != true || !strings.Contains(text(4), "kaboom") {
		t.Errorf("calls: %v %v %v", res[2], res[3], res[4])
	}
	if get(5, "result", "contents").([]any)[0].(map[string]any)["text"] != "A" {
		t.Errorf("resource: %v", res[5])
	}
	if get(6, "error", "code") != float64(-32601) || get(7, "error", "code") != float64(-32700) || res[8]["id"] != "s" {
		t.Errorf("errors: %v %v %v", res[6], res[7], res[8])
	}
}
