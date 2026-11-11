// Package loki is a logging sink that pushes records to Grafana Loki:
//
//	loki.New(loki.Options{URL: "http://localhost:3100", Labels: map[string]string{"app": "shop"}})
//
// Each record is a JSON line in a stream with the sink's labels and the
// record's level. Unset options come from LOKI_URL, LOKI_USERNAME,
// LOKI_PASSWORD and LOKI_TENANT_ID.
package loki

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Unfield/Tern/internal/telemetry"
	"github.com/Unfield/Tern/logging"
)

// Options configures the sink.
type Options struct {
	// URL is Loki's address, such as http://localhost:3100; the push path
	// is added. The default is LOKI_URL's, or http://localhost:3100.
	URL string
	// Labels are every stream's labels. Keep them few and their values
	// fixed: Loki indexes streams by them. The default is service, the
	// program's name. "level" is added for each record.
	Labels map[string]string
	// Username and Password are sent as basic authentication, as Grafana
	// Cloud wants them.
	Username, Password string
	// TenantID is sent as X-Scope-OrgID, for multi-tenant Loki.
	TenantID string
	// Metadata names top-level attributes sent as structured metadata
	// (Loki 3 and newer) as well as in the line, such as
	// logging.RequestIDKey or logging.TraceIDKey.
	Metadata []string
	// NoGzip sends requests uncompressed.
	NoGzip bool
	// Client sends the requests. The default is http.DefaultClient.
	Client *http.Client
}

// Sink pushes to Loki. Make one with New.
type Sink struct {
	url    string
	labels map[string]string
	header http.Header
	meta   []string
	gzip   bool
	client *http.Client
}

// New returns a Loki sink.
func New(o Options) *Sink {
	s := &Sink{url: o.URL, labels: o.Labels, header: http.Header{}, meta: o.Metadata, gzip: !o.NoGzip, client: o.Client}
	if s.url == "" {
		s.url = os.Getenv("LOKI_URL")
	}
	if s.url == "" {
		s.url = "http://localhost:3100"
	}
	s.url = strings.TrimRight(s.url, "/")
	if !strings.HasSuffix(s.url, "/loki/api/v1/push") {
		s.url += "/loki/api/v1/push"
	}
	if len(s.labels) == 0 {
		s.labels = map[string]string{"service": strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")}
	}
	user, pass := o.Username, o.Password
	if user == "" {
		user, pass = os.Getenv("LOKI_USERNAME"), os.Getenv("LOKI_PASSWORD")
	}
	if user != "" {
		r, _ := http.NewRequest(http.MethodPost, s.url, nil)
		r.SetBasicAuth(user, pass)
		s.header.Set("Authorization", r.Header.Get("Authorization"))
	}
	tenant := o.TenantID
	if tenant == "" {
		tenant = os.Getenv("LOKI_TENANT_ID")
	}
	if tenant != "" {
		s.header.Set("X-Scope-OrgID", tenant)
	}
	return s
}

// Name implements the name logging reports errors with.
func (*Sink) Name() string { return "loki" }

type stream struct {
	Stream map[string]string `json:"stream"`
	Values [][]any           `json:"values"`
}

// Export implements logging.Sink.
func (s *Sink) Export(ctx context.Context, entries []logging.Entry) error {
	body, err := s.body(entries)
	if err != nil {
		return logging.Permanent(err)
	}
	return telemetry.Do(ctx, telemetry.Request{Client: s.client, URL: s.url, ContentType: "application/json", Header: s.header, Body: body, Gzip: s.gzip})
}

func (s *Sink) body(entries []logging.Entry) ([]byte, error) {
	byLevel := map[string]*stream{}
	var order []string
	for _, e := range entries {
		level := strings.ToLower(e.Level.String())
		st := byLevel[level]
		if st == nil {
			labels := make(map[string]string, len(s.labels)+1)
			for k, v := range s.labels {
				labels[k] = v
			}
			labels["level"] = level
			st = &stream{Stream: labels}
			byLevel[level] = st
			order = append(order, level)
		}
		line := map[string]any{"msg": e.Message}
		for k, v := range logging.AttrsMap(e.Attrs) {
			if k == "msg" {
				k = "attrs.msg"
			}
			line[k] = v
		}
		if e.Source != nil {
			line["source"] = e.Source.File + ":" + strconv.Itoa(e.Source.Line)
		}
		b, err := json.Marshal(line)
		if err != nil {
			return nil, err
		}
		v := []any{strconv.FormatInt(e.Time.UnixNano(), 10), string(b)}
		if len(s.meta) > 0 {
			md := map[string]string{}
			for _, k := range s.meta {
				if val, ok := e.Lookup(k); ok {
					md[k] = val.String()
				}
			}
			if len(md) > 0 {
				v = append(v, md)
			}
		}
		st.Values = append(st.Values, v)
	}
	slices.Sort(order)
	streams := make([]*stream, len(order))
	for i, l := range order {
		streams[i] = byLevel[l]
	}
	return json.Marshal(map[string]any{"streams": streams})
}
