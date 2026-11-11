// Package datadog is a logging sink that sends records to Datadog's log
// intake (HTTP API v2):
//
//	datadog.New(datadog.Options{}) // DD_API_KEY, DD_SITE, DD_SERVICE, DD_ENV…
//
// Records logged in a request with a trusted traceparent carry its trace
// as dd.trace_id and dd.span_id, in the form Datadog's APM joins on.
package datadog

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Unfield/Tern/internal/telemetry"
	"github.com/Unfield/Tern/logging"
)

// Options configures the sink. Unset fields come from the environment
// variables Datadog's agents read.
type Options struct {
	// APIKey is the key logs are sent with. The default is DD_API_KEY's.
	APIKey string
	// Site is the Datadog site, such as datadoghq.eu. The default is
	// DD_SITE's, or datadoghq.com.
	Site string
	// URL replaces the intake address made from Site.
	URL string
	// Service, Env, Version, Hostname and Tags describe where the logs
	// come from. The defaults are DD_SERVICE (or the program's name),
	// DD_ENV, DD_VERSION, the host's name and DD_TAGS.
	Service, Env, Version, Hostname string
	Tags                            []string
	// Source is ddsource. The default is "go".
	Source string
	// NoGzip sends requests uncompressed.
	NoGzip bool
	// Client sends the requests. The default is http.DefaultClient.
	Client *http.Client
}

// Sink sends to Datadog. Make one with New.
type Sink struct {
	url                   string
	header                http.Header
	service, host, source string
	tags                  string
	gzip                  bool
	client                *http.Client
}

// New returns a Datadog sink.
func New(o Options) *Sink {
	env := func(v, name string) string {
		if v == "" {
			return os.Getenv(name)
		}
		return v
	}
	s := &Sink{url: o.URL, header: http.Header{}, gzip: !o.NoGzip, client: o.Client}
	if s.url == "" {
		site := env(o.Site, "DD_SITE")
		if site == "" {
			site = "datadoghq.com"
		}
		s.url = "https://http-intake.logs." + site + "/api/v2/logs"
	}
	s.header.Set("DD-API-KEY", env(o.APIKey, "DD_API_KEY"))
	s.service = env(o.Service, "DD_SERVICE")
	if s.service == "" {
		s.service = strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	}
	s.host = o.Hostname
	if s.host == "" {
		s.host, _ = os.Hostname()
	}
	s.source = o.Source
	if s.source == "" {
		s.source = "go"
	}
	var tags []string
	if e := env(o.Env, "DD_ENV"); e != "" {
		tags = append(tags, "env:"+e)
	}
	if v := env(o.Version, "DD_VERSION"); v != "" {
		tags = append(tags, "version:"+v)
	}
	if t := os.Getenv("DD_TAGS"); t != "" && o.Tags == nil {
		tags = append(tags, strings.FieldsFunc(t, func(r rune) bool { return r == ',' || r == ' ' })...)
	}
	tags = append(tags, o.Tags...)
	s.tags = strings.Join(tags, ",")
	return s
}

// Name implements the name logging reports errors with.
func (*Sink) Name() string { return "datadog" }

// maxBatch is the most entries the intake takes in one request.
const maxBatch = 1000

// Export implements logging.Sink.
func (s *Sink) Export(ctx context.Context, entries []logging.Entry) error {
	for len(entries) > 0 {
		n := min(len(entries), maxBatch)
		body, err := json.Marshal(s.items(entries[:n]))
		if err != nil {
			return logging.Permanent(err)
		}
		if err := telemetry.Do(ctx, telemetry.Request{Client: s.client, URL: s.url, ContentType: "application/json", Header: s.header, Body: body, Gzip: s.gzip}); err != nil {
			return err
		}
		entries = entries[n:]
	}
	return nil
}

func (s *Sink) items(entries []logging.Entry) []map[string]any {
	out := make([]map[string]any, len(entries))
	for i, e := range entries {
		m := logging.AttrsMap(e.Attrs)
		for _, k := range []string{"message", "status", "service", "hostname", "ddsource", "ddtags", "timestamp"} {
			if v, ok := m[k]; ok {
				delete(m, k)
				m["attrs."+k] = v
			}
		}
		m["message"] = e.Message
		m["status"] = status(e.Level.String())
		m["service"] = s.service
		m["ddsource"] = s.source
		if s.host != "" {
			m["hostname"] = s.host
		}
		if s.tags != "" {
			m["ddtags"] = s.tags
		}
		m["timestamp"] = e.Time.UnixMilli()
		if v, ok := e.Lookup(logging.TraceIDKey); ok {
			if t, ok := lower64(v.String()); ok {
				m["dd.trace_id"] = t
			}
		}
		if v, ok := e.Lookup(logging.SpanIDKey); ok {
			if sp, err := strconv.ParseUint(v.String(), 16, 64); err == nil {
				m["dd.span_id"] = strconv.FormatUint(sp, 10)
			}
		}
		if e.Source != nil {
			m["logger"] = map[string]any{"file": e.Source.File, "line": e.Source.Line, "method_name": e.Source.Function}
		}
		out[i] = m
	}
	return out
}

// lower64 is a W3C trace ID's lower 64 bits in decimal, as Datadog keys
// traces.
func lower64(trace string) (string, bool) {
	if len(trace) != 32 {
		return "", false
	}
	u, err := strconv.ParseUint(trace[16:], 16, 64)
	if err != nil {
		return "", false
	}
	return strconv.FormatUint(u, 10), true
}

func status(level string) string {
	switch {
	case strings.HasPrefix(level, "DEBUG"):
		return "debug"
	case strings.HasPrefix(level, "INFO"):
		return "info"
	case strings.HasPrefix(level, "WARN"):
		return "warning"
	}
	return "error"
}
