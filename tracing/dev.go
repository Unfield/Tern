package tracing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/internal/batch"
	"github.com/Unfield/Tern/logging"
)

// Under tern dev, spans go to the dev server, which draws each request's
// as a waterfall in its log view. One shipper per process.
var dev struct {
	once sync.Once
	sh   *batch.Shipper[SpanData]
}

// DevSpan is a span as tern dev's log view reads it.
type DevSpan struct {
	TraceID   string         `json:"trace"`
	SpanID    string         `json:"span"`
	Parent    string         `json:"parent,omitempty"`
	Name      string         `json:"name"`
	Kind      string         `json:"kind"`
	Start     time.Time      `json:"start"`
	Micros    int64          `json:"us"`
	Error     string         `json:"error,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
	Attrs     map[string]any `json:"attrs,omitempty"`
}

func devShipper() *batch.Shipper[SpanData] {
	dev.once.Do(func() {
		u := os.Getenv("TERN_DEV_LOGS")
		if u == "" {
			return
		}
		u = strings.TrimSuffix(u, "/ingest") + "/ingest-spans"
		token := os.Getenv("TERN_DEV_LOGS_TOKEN")
		client := &http.Client{Timeout: 2 * time.Second}
		export := func(ctx context.Context, spans []SpanData) error {
			out := make([]DevSpan, len(spans))
			for i, s := range spans {
				out[i] = devSpan(s)
			}
			body, err := json.Marshal(out)
			if err != nil {
				return Permanent(err)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
			if err != nil {
				return Permanent(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := client.Do(req)
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
		dev.sh = batch.New(batch.Config{Name: "tern dev"}, export, batch.Options{Size: 256, Interval: 100 * time.Millisecond, MaxRetries: 2, Timeout: 2 * time.Second})
	})
	return dev.sh
}

func devSpan(s SpanData) DevSpan {
	d := DevSpan{TraceID: s.Context.TraceID.String(), SpanID: s.Context.SpanID.String(), Name: s.Name, Kind: s.Kind.String(), Start: s.Start, Micros: s.Duration().Microseconds()}
	if s.Parent.IsValid() {
		d.Parent = s.Parent.String()
	}
	if s.Status.Code == Error {
		d.Error = s.Status.Message
		if d.Error == "" {
			d.Error = "error"
		}
	}
	if len(s.Attrs) > 0 {
		d.Attrs = logging.AttrsMap(s.Attrs)
		if id, ok := d.Attrs[logging.RequestIDKey].(string); ok {
			d.RequestID = id
		}
	}
	return d
}

// DevOnly returns a tracer that records every span for tern dev's log
// view alone, or nil outside tern dev. kit uses it when the app has no
// tracer of its own.
func DevOnly() *Tracer {
	if devShipper() == nil {
		return nil
	}
	return &Tracer{rate: 1, ships: []*batch.Shipper[SpanData]{dev.sh}}
}
