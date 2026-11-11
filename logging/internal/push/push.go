// Package push sends a sink's batch over HTTP, the way the sinks that
// talk to services share: gzip, status codes mapped to retries, and
// Retry-After.
package push

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/logging"
)

// Request is one push.
type Request struct {
	Client      *http.Client // nil: http.DefaultClient
	URL         string
	ContentType string
	Header      http.Header
	Body        []byte
	Gzip        bool
}

// Do sends r. Errors that retrying can fix (connection errors, 408, 429
// and 5xx) are returned as they are, or with [logging.RetryAfter]; the
// rest are [logging.Permanent].
func Do(ctx context.Context, r Request) error {
	body := r.Body
	if r.Gzip {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(body); err != nil {
			return logging.Permanent(err)
		}
		if err := zw.Close(); err != nil {
			return logging.Permanent(err)
		}
		body = buf.Bytes()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, bytes.NewReader(body))
	if err != nil {
		return logging.Permanent(err)
	}
	for k, vs := range r.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Content-Type", r.ContentType)
	if r.Gzip {
		req.Header.Set("Content-Encoding", "gzip")
	}
	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode/100 == 2 {
		return nil
	}
	err = fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(msg)))
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode >= 500 {
		if d := retryAfter(resp.Header.Get("Retry-After")); d > 0 {
			return logging.RetryAfter(err, d)
		}
		return err
	}
	return logging.Permanent(err)
}

func retryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil && s > 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}

// Pair is a key and its value.
type Pair struct{ Key, Value string }

// ParsePairs reads "k=v,k2=v2", as OTEL_EXPORTER_OTLP_HEADERS and
// OTEL_RESOURCE_ATTRIBUTES have them, with URL-encoded values.
func ParsePairs(s string) []Pair {
	var out []Pair
	for _, part := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(part, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			continue
		}
		v = strings.TrimSpace(v)
		if uv, err := url.PathUnescape(v); err == nil {
			v = uv
		}
		out = append(out, Pair{k, v})
	}
	return out
}

// ParseHeaders reads ParsePairs' format as headers.
func ParseHeaders(s string) http.Header {
	h := http.Header{}
	for _, p := range ParsePairs(s) {
		h.Add(p.Key, p.Value)
	}
	return h
}
