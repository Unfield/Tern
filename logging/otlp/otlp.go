// Package otlp is a logging sink that sends records to an OpenTelemetry
// collector, or any service that takes OTLP over HTTP (Grafana Cloud,
// Honeycomb, New Relic, SigNoz, Axiom…), as OTLP/HTTP JSON:
//
//	logging.New(logging.Options{Sinks: []logging.Sink{otlp.New(otlp.Options{})}})
//
// With no options it reads the standard environment variables:
// OTEL_EXPORTER_OTLP_LOGS_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT,
// OTEL_EXPORTER_OTLP_LOGS_HEADERS or OTEL_EXPORTER_OTLP_HEADERS,
// OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES. Records logged in a
// request with a trusted traceparent carry its trace and span.
package otlp

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/logging"
	"github.com/Unfield/Tern/logging/internal/push"
)

// Options configures the sink. Unset fields come from the environment.
type Options struct {
	// Endpoint is the URL logs are posted to, ending in /v1/logs. The
	// default is OTEL_EXPORTER_OTLP_LOGS_ENDPOINT's, or
	// OTEL_EXPORTER_OTLP_ENDPOINT's with /v1/logs added, or
	// http://localhost:4318/v1/logs.
	Endpoint string
	// Headers are sent with each request, such as an API key.
	Headers http.Header
	// ServiceName is the service.name resource attribute. The default is
	// OTEL_SERVICE_NAME's, or the program's name.
	ServiceName string
	// Resource adds resource attributes, after OTEL_RESOURCE_ATTRIBUTES'.
	Resource map[string]string
	// NoGzip sends requests uncompressed.
	NoGzip bool
	// Client sends the requests. The default is http.DefaultClient.
	Client *http.Client
}

// Sink sends records over OTLP/HTTP. Make one with New.
type Sink struct {
	endpoint string
	headers  http.Header
	resource []keyValue
	gzip     bool
	client   *http.Client
}

// New returns an OTLP sink.
func New(o Options) *Sink {
	s := &Sink{endpoint: o.Endpoint, headers: http.Header{}, gzip: !o.NoGzip, client: o.Client}
	if s.endpoint == "" {
		s.endpoint = os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT")
	}
	if s.endpoint == "" {
		if base := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); base != "" {
			s.endpoint = strings.TrimRight(base, "/") + "/v1/logs"
		}
	}
	if s.endpoint == "" {
		s.endpoint = "http://localhost:4318/v1/logs"
	}
	env := os.Getenv("OTEL_EXPORTER_OTLP_LOGS_HEADERS")
	if env == "" {
		env = os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")
	}
	for k, vs := range push.ParseHeaders(env) {
		s.headers[k] = vs
	}
	for k, vs := range o.Headers {
		s.headers[k] = vs
	}

	res := map[string]string{}
	var order []string
	set := func(k, v string) {
		if _, ok := res[k]; !ok {
			order = append(order, k)
		}
		res[k] = v
	}
	for _, p := range push.ParsePairs(os.Getenv("OTEL_RESOURCE_ATTRIBUTES")) {
		set(p.Key, p.Value)
	}
	keys := make([]string, 0, len(o.Resource))
	for k := range o.Resource {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		set(k, o.Resource[k])
	}
	name := o.ServiceName
	if name == "" {
		name = os.Getenv("OTEL_SERVICE_NAME")
	}
	if name == "" {
		name = res["service.name"]
	}
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	}
	set("service.name", name)
	slices.Sort(order)
	for _, k := range order {
		s.resource = append(s.resource, keyValue{Key: k, Value: anyValue{StringValue: ptr(res[k])}})
	}
	return s
}

// Name implements the name logging reports errors with.
func (*Sink) Name() string { return "otlp" }

// Export implements logging.Sink.
func (s *Sink) Export(ctx context.Context, entries []logging.Entry) error {
	body, err := json.Marshal(s.request(entries))
	if err != nil {
		return logging.Permanent(err)
	}
	return push.Do(ctx, push.Request{Client: s.client, URL: s.endpoint, ContentType: "application/json", Header: s.headers, Body: body, Gzip: s.gzip})
}

func (s *Sink) request(entries []logging.Entry) exportRequest {
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	recs := make([]logRecord, len(entries))
	for i, e := range entries {
		r := logRecord{
			TimeUnixNano:         strconv.FormatInt(e.Time.UnixNano(), 10),
			ObservedTimeUnixNano: now,
			SeverityNumber:       Severity(e.Level),
			SeverityText:         e.Level.String(),
			Body:                 anyValue{StringValue: ptr(e.Message)},
		}
		if e.Time.IsZero() {
			r.TimeUnixNano = "0"
		}
		for _, a := range e.Attrs {
			switch a.Key {
			case logging.TraceIDKey:
				if v := a.Value.String(); len(v) == 32 {
					r.TraceID = v
					continue
				}
			case logging.SpanIDKey:
				if v := a.Value.String(); len(v) == 16 {
					r.SpanID = v
					continue
				}
			}
			r.Attributes = append(r.Attributes, keyValue{Key: a.Key, Value: value(a.Value)})
		}
		if src := e.Source; src != nil {
			r.Attributes = append(r.Attributes,
				keyValue{Key: "code.file.path", Value: anyValue{StringValue: ptr(src.File)}},
				keyValue{Key: "code.line.number", Value: anyValue{IntValue: ptr(strconv.Itoa(src.Line))}},
				keyValue{Key: "code.function.name", Value: anyValue{StringValue: ptr(src.Function)}},
			)
		}
		recs[i] = r
	}
	return exportRequest{ResourceLogs: []resourceLogs{{
		Resource:  resource{Attributes: s.resource},
		ScopeLogs: []scopeLogs{{Scope: scope{Name: "github.com/Unfield/Tern/logging"}, LogRecords: recs}},
	}}}
}

// Severity is OpenTelemetry's severity number for a slog level: DEBUG is
// 5, INFO 9, WARN 13 and ERROR 17, with the levels between in between.
func Severity(l slog.Level) int {
	return min(max(int(l)+9, 1), 24)
}

func value(v slog.Value) anyValue {
	switch v.Kind() {
	case slog.KindString:
		return anyValue{StringValue: ptr(v.String())}
	case slog.KindInt64:
		return anyValue{IntValue: ptr(strconv.FormatInt(v.Int64(), 10))}
	case slog.KindUint64:
		u := v.Uint64()
		if u > math.MaxInt64 {
			return anyValue{StringValue: ptr(strconv.FormatUint(u, 10))}
		}
		return anyValue{IntValue: ptr(strconv.FormatUint(u, 10))}
	case slog.KindFloat64:
		f := v.Float64()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return anyValue{StringValue: ptr(strconv.FormatFloat(f, 'g', -1, 64))}
		}
		return anyValue{DoubleValue: &f}
	case slog.KindBool:
		b := v.Bool()
		return anyValue{BoolValue: &b}
	case slog.KindDuration:
		return anyValue{StringValue: ptr(v.Duration().String())}
	case slog.KindTime:
		return anyValue{StringValue: ptr(v.Time().Format(time.RFC3339Nano))}
	case slog.KindGroup:
		var kv []keyValue
		for _, a := range v.Group() {
			kv = append(kv, keyValue{Key: a.Key, Value: value(a.Value)})
		}
		return anyValue{KvlistValue: &kvList{Values: kv}}
	}
	switch x := logging.ValueOf(v).(type) {
	case string:
		return anyValue{StringValue: &x}
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return anyValue{StringValue: ptr(v.String())}
		}
		return anyValue{StringValue: ptr(string(b))}
	}
}

func ptr[T any](v T) *T { return &v }

// The OTLP/HTTP JSON shapes, from opentelemetry-proto's logs.proto.
type (
	exportRequest struct {
		ResourceLogs []resourceLogs `json:"resourceLogs"`
	}
	resourceLogs struct {
		Resource  resource    `json:"resource"`
		ScopeLogs []scopeLogs `json:"scopeLogs"`
	}
	resource struct {
		Attributes []keyValue `json:"attributes,omitempty"`
	}
	scopeLogs struct {
		Scope      scope       `json:"scope"`
		LogRecords []logRecord `json:"logRecords"`
	}
	scope struct {
		Name string `json:"name"`
	}
	logRecord struct {
		TimeUnixNano         string     `json:"timeUnixNano"`
		ObservedTimeUnixNano string     `json:"observedTimeUnixNano"`
		SeverityNumber       int        `json:"severityNumber"`
		SeverityText         string     `json:"severityText"`
		Body                 anyValue   `json:"body"`
		Attributes           []keyValue `json:"attributes,omitempty"`
		TraceID              string     `json:"traceId,omitempty"`
		SpanID               string     `json:"spanId,omitempty"`
	}
	keyValue struct {
		Key   string   `json:"key"`
		Value anyValue `json:"value"`
	}
	anyValue struct {
		StringValue *string  `json:"stringValue,omitempty"`
		IntValue    *string  `json:"intValue,omitempty"`
		DoubleValue *float64 `json:"doubleValue,omitempty"`
		BoolValue   *bool    `json:"boolValue,omitempty"`
		KvlistValue *kvList  `json:"kvlistValue,omitempty"`
	}
	kvList struct {
		Values []keyValue `json:"values"`
	}
)
