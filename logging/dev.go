package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// devSink sends entries to tern dev's log view.
type devSink struct {
	url, token string
}

func (*devSink) Name() string { return "tern dev" }

func (d *devSink) Export(ctx context.Context, entries []Entry) error {
	body, err := json.Marshal(EncodeJSON(entries))
	if err != nil {
		return Permanent(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url, bytes.NewReader(body))
	if err != nil {
		return Permanent(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.token)
	resp, err := devClient.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return Permanent(fmt.Errorf("tern dev answered %s", resp.Status))
	}
	return nil
}

var devClient = &http.Client{Timeout: 2 * time.Second}

// JSONEntry is an entry as JSON: the shape tern dev's log view reads, and
// a plain one for sinks that send JSON objects.
type JSONEntry struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"msg"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Source  *slog.Source   `json:"source,omitempty"`
}

// EncodeJSON returns entries as JSONEntry values.
func EncodeJSON(entries []Entry) []JSONEntry {
	out := make([]JSONEntry, len(entries))
	for i, e := range entries {
		out[i] = JSONEntry{Time: e.Time, Level: e.Level.String(), Message: e.Message, Source: e.Source}
		if len(e.Attrs) > 0 {
			out[i].Attrs = AttrsMap(e.Attrs)
		}
	}
	return out
}

// AttrsMap returns attrs as a map for encoding/json: groups as nested maps,
// times in RFC 3339, durations as strings, errors as their messages.
// A key that appears twice keeps its last value.
func AttrsMap(attrs []slog.Attr) map[string]any {
	m := make(map[string]any, len(attrs))
	for _, a := range attrs {
		m[a.Key] = ValueOf(a.Value)
	}
	return m
}

// ValueOf returns v as a value for encoding/json.
func ValueOf(v slog.Value) any {
	switch v.Kind() {
	case slog.KindGroup:
		return AttrsMap(v.Group())
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return v.Int64()
	case slog.KindUint64:
		return v.Uint64()
	case slog.KindFloat64:
		return v.Float64()
	case slog.KindBool:
		return v.Bool()
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339Nano)
	case slog.KindLogValuer:
		return ValueOf(v.Resolve())
	}
	x := v.Any()
	switch x := x.(type) {
	case error:
		return x.Error()
	case json.Marshaler:
		return x
	case fmt.Stringer:
		return x.String()
	}
	if b, err := json.Marshal(x); err == nil {
		return json.RawMessage(b)
	}
	return fmt.Sprint(x)
}
